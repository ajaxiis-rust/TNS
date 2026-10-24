package turn

import (
	"context"
	"time"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/models"
)

// Stage — одна стадия конвейера хода (T-03-01, шаг 2).
type Stage interface {
	Name() string
	Run(ctx context.Context, st *TurnState) error
}

// TurnState — состояние хода, проходящее по стадиям конвейера.
type TurnState struct {
	Input     TurnInput
	Command   *Command // не nil для слеш-команды.
	StartedAt time.Time
	Path      string // выбранный путь хода.
	Reply     string // накапливаемый ответ игроку.
	Degraded  bool
	LatencyMS int64

	// TurnID — номер хода в памяти: связывает векторы резки (T-03-02, шаг 2).
	TurnID int64
	// Embedder — эмбеддер векторов смысла (T-02-07); nil — резка без векторов.
	Embedder models.Embedder
	// Lemmatizer — клиент лемматизации (T-01-09); nil — резка без лемм.
	Lemmatizer Lemmatizer
	// Cut — результат резки реплики (T-03-02) в памяти хода.
	Cut Cut
}

// fallbackReply — нейтральная запись fallback из банка (T-03-01, шаг 5): до путей
// (T-03-07/08) это заглушка, отвечающая непустым текстом с первого дня.
const fallbackReply = "Мир слушает, но пока не находит ответа. Скажи иначе."

// stages — упорядоченный реестр стадий (шаг 2): резка → предварительный путь →
// подтверждение → выбор пути → исполнение → страховка → коммит. Каждая задача P2
// добавляет свою стадию отдельным файлом и одной строкой реестра.
func (p *Pipeline) stages() []Stage {
	return []Stage{
		&cutStage{logger: p.Logger},
		&prepathStage{},
		&confirmStage{},
		&selectStage{},
		&executeStage{},
		&safetyStage{},
		&commitStage{db: p.DB},
	}
}

// cutStage — резка реплики (T-03-02): сегменты по знакам препинания, лемматизация
// и векторизация — вектор целой реплики, по вектору на сегмент и ранние векторы
// по словам. Писатель отсутствует: векторы живут в st.Cut до подтверждения
// (T-03-05).
type cutStage struct {
	logger *journal.Logger
}

func (*cutStage) Name() string { return "cut" }

func (s *cutStage) Run(ctx context.Context, st *TurnState) error {
	if st.TurnID == 0 {
		st.TurnID = nextTurnID()
	}
	cut := Cut{TurnID: st.TurnID}

	segments := cutReplica(st.Input.PlayerText)
	embedder := st.Embedder
	lemmatizer := st.Lemmatizer

	// Вектор целой реплики (шаг 2): контроль — по точке целой реплики (LT-02.3).
	if embedder != nil {
		if v, err := embedder.Embed(ctx, st.Input.PlayerText); err != nil {
			s.degrade(st, err)
		} else {
			cut.Whole = v
		}
	}

	for _, text := range segments {
		seg := Segment{Text: text}
		if lemmatizer != nil {
			if toks, err := lemmatizer.Lemmatize(text); err == nil {
				for _, tok := range toks {
					seg.Lemmas = append(seg.Lemmas, tok.Lemma)
				}
			}
			// Ошибка лемматизатора оставляет леммы пустыми; событие lemma.fail
			// пишет клиент (T-01-09).
		}
		if embedder != nil {
			if v, err := embedder.Embed(ctx, text); err != nil {
				s.degrade(st, err)
			} else {
				seg.Vector = v
			}
		}
		cut.Segments = append(cut.Segments, seg)
	}

	// Ранние векторы по словам (шаг 3): идут на входе без слота генерации,
	// промежуточные векторы коммит не снимает.
	if embedder != nil {
		for _, w := range splitWords(st.Input.PlayerText) {
			if v, err := embedder.Embed(ctx, w); err != nil {
				s.degrade(st, err)
			} else {
				cut.Words = append(cut.Words, WordVector{Word: w, Vector: v})
			}
		}
	}

	st.Cut = cut
	return nil
}

// degrade — ошибка эмбеддера: событие embed.fail в журнал и пометка деградации
// хода (деградацию обрабатывает T-03-08).
func (s *cutStage) degrade(st *TurnState, err error) {
	st.Degraded = true
	if s.logger != nil {
		_ = s.logger.Event("turn", journal.EventEmbedFail,
			journal.Field{Key: "err", Value: err.Error()})
	}
}

// prepathStage — предварительный путь (заполняется T-03-03).
type prepathStage struct{}

func (*prepathStage) Name() string                          { return "prepath" }
func (*prepathStage) Run(context.Context, *TurnState) error { return nil }

// confirmStage — подтверждение смысла (заполняется T-03-04).
type confirmStage struct{}

func (*confirmStage) Name() string                          { return "confirm" }
func (*confirmStage) Run(context.Context, *TurnState) error { return nil }

// selectStage — выбор пути (заполняется T-03-09); до него путь фиксируется
// заглушкой: слеш-команда — "command", проза — "fallback".
type selectStage struct{}

func (*selectStage) Name() string { return "select" }

func (*selectStage) Run(_ context.Context, st *TurnState) error {
	if st.Command != nil {
		st.Path = "command"
	} else {
		st.Path = "fallback"
	}
	return nil
}

// executeStage — исполнение выбранного пути (заполняется T-03-07/08). До путей
// отвечает нейтральной записью fallback из банка, слеш-команда — служебным
// текстом; reply_text непуст с первого дня.
type executeStage struct{}

func (*executeStage) Name() string { return "execute" }

func (*executeStage) Run(_ context.Context, st *TurnState) error {
	if st.Command != nil {
		st.Reply = commandReply(st.Command.Name)
		return nil
	}
	st.Reply = fallbackReply
	return nil
}

// safetyStage — страховка хода (заполняется T-03-08).
type safetyStage struct{}

func (*safetyStage) Name() string                          { return "safety" }
func (*safetyStage) Run(context.Context, *TurnState) error { return nil }

// commitStage — коммит хода (заполняется T-03-05): строка в turns после стадий.
type commitStage struct {
	db *db.Handle
}

func (*commitStage) Name() string { return "commit" }

func (s *commitStage) Run(_ context.Context, st *TurnState) error {
	st.LatencyMS = time.Since(st.StartedAt).Milliseconds()
	if s.db == nil {
		return nil
	}
	degraded := 0
	if st.Degraded {
		degraded = 1
	}
	_, err := s.db.Exec("turns",
		`INSERT INTO turns (session_id, player_text, reply_text, path, saw_recommendation, trust_level, latency_ms, degraded) VALUES (?, ?, ?, ?, 0, 0, ?, ?)`,
		st.Input.SessionID, st.Input.PlayerText, st.Reply, st.Path, st.LatencyMS, degraded)
	return err
}
