package turn

import (
	"context"
	"time"

	"tns65/internal/db"
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
}

// fallbackReply — нейтральная запись fallback из банка (T-03-01, шаг 5): до путей
// (T-03-07/08) это заглушка, отвечающая непустым текстом с первого дня.
const fallbackReply = "Мир слушает, но пока не находит ответа. Скажи иначе."

// stages — упорядоченный реестр стадий (шаг 2): резка → предварительный путь →
// подтверждение → выбор пути → исполнение → страховка → коммит. Каждая задача P2
// добавляет свою стадию отдельным файлом и одной строкой реестра.
func (p *Pipeline) stages() []Stage {
	return []Stage{
		&cutStage{},
		&prepathStage{},
		&confirmStage{},
		&selectStage{},
		&executeStage{},
		&safetyStage{},
		&commitStage{db: p.DB},
	}
}

// cutStage — резка реплики (заполняется T-03-02).
type cutStage struct{}

func (*cutStage) Name() string                          { return "cut" }
func (*cutStage) Run(context.Context, *TurnState) error { return nil }

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
