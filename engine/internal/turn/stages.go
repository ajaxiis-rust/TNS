package turn

import (
	"context"
	"time"

	"tns65/internal/bank"
	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/models"
	"tns65/internal/selector"
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
	// Prepath — результат предварительного пути (T-03-03): намерение, настроение,
	// рамка сцены, предвыбор и собранный промпт без токенов-заменителей.
	Prepath Prepath
	// HypothesisLemmas — леммы гипотезы намерения (T-03-04): основа словарного
	// голоса подтверждения. Заполняет предпуть из лексикона.
	HypothesisLemmas []string
	// Confirm — результат двухголосия подтверждения (T-03-04) в памяти хода.
	Confirm Confirmation
	// Bindings — привязки токенов биндера до подтверждения (T-03-03); на коммите
	// хода пишутся в token_bindings (T-03-05).
	Bindings []Binding
	// SawRecommendation — код рекомендации Пилы (T-03-06) для turns.saw_recommendation:
	// 0 — нет, 1 — Ткач, 2 — Челнок. Заполняет Селектор (T-03-09).
	SawRecommendation int
	// SelectorDecision — решение Селектора (T-03-09): номер сработавшего правила
	// и его имя, пишется в turns.selector_decision.
	SelectorDecision string
	// Candidates — ветки, не вошедшие в ход (T-03-10, шаг 3): после закрытия
	// сцены ставятся заказом sediment в workshop_queue, в игровые таблицы не
	// пишутся. Заполняет стадия исполнения.
	Candidates []SedimentCandidate
	// Roster — труппа и места сцены (T-03-11): наполняет труппа до предпути,
	// предпуть закрывает токены биндера по этим лицам.
	Roster *Roster
	// Troupe — лица мира в живом ходе (T-03-11): появление персонажа и память
	// реакций; nil — труппа не подключена.
	Troupe *Troupe
	// TimeJump — декларация времени в реплике (T-03-12): детектор времени на
	// входе и скачок; пустой Span — декларации не было.
	TimeJump TimeJump
	// Absence — итог прогона паузы на возврате (T-03-13): заполняет returnStage,
	// сводку и вести из него строит T-03-14.
	Absence AbsenceReport
	// AbsentFor — длительность отсутствия до текущего хода (T-03-14, шаг 1).
	AbsentFor time.Duration
	// DigestSummary — сводка «пока тебя не было» (T-03-14); пусто — сводки нет.
	DigestSummary string
	// DigestNews — запаздывающие вести, накопленные в отсутствие (T-03-14, шаг 2).
	DigestNews []string
}

// fallbackReply — нейтральная запись fallback из банка (T-03-01, шаг 5): до путей
// (T-03-07/08) это заглушка, отвечающая непустым текстом с первого дня.
const fallbackReply = "Мир слушает, но пока не находит ответа. Скажи иначе."

// reaskReply — переспрос при неподтверждённом смысле (T-03-04, шаг 2):
// dict_match ниже порога — ход уходит на переспрос нейтральной записью.
const reaskReply = "Я не уверен, что верно понял тебя. Скажи иначе."

// stages — упорядоченный реестр стадий (шаг 2): возврат → резка → предварительный
// путь → подтверждение → выбор пути → исполнение → время 1:1 → страховка →
// сводка/вести → коммит. Каждая задача P2 добавляет свою стадию отдельным файлом
// и одной строкой реестра.
func (p *Pipeline) stages() []Stage {
	return []Stage{
		&returnStage{db: p.DB},
		&cutStage{logger: p.Logger},
		&troupeStage{db: p.DB},
		&prepathStage{db: p.DB},
		&confirmStage{configPath: "config.yaml"},
		&selectStage{},
		&executeStage{},
		&timejumpStage{db: p.DB},
		&safetyStage{},
		&digestStage{db: p.DB},
		&commitStage{db: p.DB},
	}
}

// returnStage — возврат игрока (T-03-13, «Правится» stages.go): на входе нового
// хода мир продвигается за время отсутствия той же скоростью, что и при игре
// (пауза ≠ заморозка). Отсутствие отсчитывается от lastActive; в реестре стадий
// lastActive пуст — проводку последней активности ведут сервер хода (T-08-15) и
// книга (T-07-01), здесь — крючок без ложного продвижения.
type returnStage struct {
	db         *db.Handle
	clock      ClockSource
	lastActive time.Time
}

func (*returnStage) Name() string { return "return" }

func (s *returnStage) Run(_ context.Context, st *TurnState) error {
	if s.lastActive.IsZero() {
		return nil
	}
	rep, err := advanceAbsence(s.db, s.clock, s.lastActive)
	if err != nil {
		return err
	}
	now := time.Now()
	if s.clock != nil {
		now = s.clock.Now()
	}
	st.Absence = rep
	st.AbsentFor = now.Sub(s.lastActive)
	return nil
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

// troupeStage — труппа (T-03-11): наполняет лиц мира из карточек characters до
// предпути, чтобы биндер слотов видел появившихся персонажей. Реакция игрока на
// встречу фиксируется предпутём по названным именам (память реакций — в
// memory_topics).
type troupeStage struct {
	db *db.Handle
}

func (*troupeStage) Name() string { return "troupe" }

func (s *troupeStage) Run(_ context.Context, st *TurnState) error {
	if st.Troupe == nil {
		st.Troupe = NewTroupe(s.db)
	}
	if s.db == nil {
		return nil
	}
	roster, err := st.Troupe.LoadRoster()
	if err != nil {
		return err
	}
	st.Roster = &roster
	return nil
}

// prepathStage — предварительный путь (T-03-03): гипотеза намерения лексиконом
// языкового пакета без LLM, настроение реплики, рамка сцены из механики и
// предвыбор; биндер закрывает токены-заменители, привязки живут в памяти до
// подтверждения.
type prepathStage struct {
	db      *db.Handle
	lexicon *Lexicon
	binder  *SlotBinder
}

func (*prepathStage) Name() string { return "prepath" }

func (s *prepathStage) Run(_ context.Context, st *TurnState) error {
	lex := s.lexicon
	if lex == nil {
		lex = loadDefaultLexicon()
	}
	binder := s.binder
	if binder == nil {
		roster := Roster{}
		if st.Roster != nil {
			roster = *st.Roster
		}
		binder = NewSlotBinder(roster)
	}

	lemmas := collectLemmas(st.Cut)
	intent := hypothesizeIntent(lemmas, lex)
	mood := detectMood(lemmas, lex)
	frame := s.assembleFrame(st)
	sel := preselect(intent, mood)
	template := promptTemplate(sel)

	mentions := binder.DetectMentions(st.Input.PlayerText)
	prompt, bindings := binder.Bind(template, mentions, 0, 0)

	// Память реакций (T-03-11, LT-16.2): каждая встреча названного лица
	// фиксируется как реакция «не заметил» — механика, не генерация; повторная
	// встреча читает запись в полосе отношения.
	if st.Troupe != nil {
		for _, n := range detectMentions(binder.Roster(), st.Input.PlayerText) {
			_ = st.Troupe.RecordMeeting(n.ID, ReactionIgnored)
		}
	}

	st.Prepath = Prepath{
		Intent:        intent,
		Mood:          mood,
		Frame:         frame,
		Preselect:     sel,
		Template:      template,
		Prompt:        prompt,
		LexiconLoaded: lex != nil,
	}
	// Леммы гипотезы намерения — словарный голос подтверждения (T-03-04).
	st.HypothesisLemmas = intentLemmas(lex, intent.Name)
	st.Bindings = append(st.Bindings, bindings...)
	return nil
}

// intentLemmas возвращает леммы и глаголы намерения из лексикона — гипотезу,
// которую подтверждает словарный голос (T-03-04, шаг 1). Пусто, если лексикона
// нет или намерение не определено.
func intentLemmas(lex *Lexicon, name string) []string {
	if lex == nil || name == "" {
		return nil
	}
	for _, e := range lex.Intents {
		if e.Intent != name {
			continue
		}
		out := make([]string, 0, len(e.Lemmas)+len(e.Verbs))
		out = append(out, e.Lemmas...)
		out = append(out, e.Verbs...)
		return out
	}
	return nil
}

// assembleFrame собирает рамку сцены из механики: место и настроение героя из
// character_state, от слов реплики не зависят.
func (s *prepathStage) assembleFrame(_ *TurnState) SceneFrame {
	var f SceneFrame
	if s.db == nil {
		return f
	}
	_ = s.db.QueryRow(`SELECT place_id, mood FROM character_state WHERE character_id = 0`).
		Scan(&f.HeroPlaceID, &f.HeroMood)
	return f
}

// loadDefaultLexicon загружает лексикон языкового пакета по умолчанию из
// packs/ru; при отсутствии пакета гипотеза намерения и настроение не считаются,
// ход при этом не прерывается.
func loadDefaultLexicon() *Lexicon {
	lex, err := LoadLexicon("packs/ru")
	if err != nil {
		return nil
	}
	return lex
}

// confirmStage — подтверждение смысла (T-03-04): словарный и векторный голоса,
// решение по таблице с числами из config.yaml. Писатель отсутствует: результат
// живёт в st.Confirm до подтверждения; при неподтверждении ход уходит на переспрос.
type confirmStage struct {
	Config     ConfirmConfig // пороги; нулевые значения заменяются дефолтами.
	configPath string        // путь к config.yaml; пусто — не читать файл.
}

func (*confirmStage) Name() string { return "confirm" }

func (s *confirmStage) Run(ctx context.Context, st *TurnState) error {
	// Слеш-команда не имеет гипотезы смысла — подтверждать нечего.
	if st.Command != nil {
		return nil
	}
	cfg := s.Config
	if cfg.DictThreshold <= 0 && s.configPath != "" {
		if loaded, err := LoadConfirmConfig(s.configPath); err == nil {
			cfg = loaded
		}
	}
	st.Confirm = confirm(ctx, st, cfg)
	return nil
}

// selectStage — выбор пути (T-03-09): Селектор сводит сигналы хода в единую
// таблицу решений, первое сработавшее правило выигрывает. Рекомендация Пилы и
// решение Селектора живут в состоянии хода и пишутся в turns на коммите (T-03-05).
// bank и weaver опциональны: без них жёсткое ограничение уводит ход на страховку;
// saw опционален — без Пилы рекомендации нет.
type selectStage struct {
	bank      *bank.Bank    // Челнок (T-03-07); nil — банк не подключён.
	weaver    ProseSlot     // Ткач (T-03-08); nil — слот мёртв.
	saw       *selector.Saw // Пила (T-03-06); nil — рекомендации нет.
	sawThread int64         // нить отношений для шага Пилы (0 — дефолт).
	sawLink   selector.MoodLink
}

func (*selectStage) Name() string { return "select" }

func (s *selectStage) Run(_ context.Context, st *TurnState) error {
	rec := s.recommendation()
	in := selector.Input{
		Command:      s.commandName(st),
		Stop:         st.Confirm.Decision == DecisionReask,
		HardFallback: s.slotDead() && s.bankEmpty(),
		Repeatable:   isRepeatable(st),
		Emotional:    isEmotional(st),
		Saw:          rec,
		BankEmpty:    s.bankEmpty(),
	}
	dec := selector.Select(in)
	st.Path = string(dec.Path)
	st.SawRecommendation = selector.RecommendationCode(rec)
	st.SelectorDecision = dec.Encode()
	return nil
}

// recommendation — рекомендация Пилы на ход; без Пилы её нет (T-03-06, шаг 3).
func (s *selectStage) recommendation() selector.Recommendation {
	if s.saw == nil {
		return selector.RecommendNone
	}
	rec, err := s.saw.Step(s.sawThread, s.sawLink)
	if err != nil {
		return selector.RecommendNone
	}
	return rec
}

// commandName — имя слеш-команды; пусто для прозы.
func (*selectStage) commandName(st *TurnState) string {
	if st.Command == nil {
		return ""
	}
	return st.Command.Name
}

// slotDead — слот Ткача не подключён (жёсткое ограничение, T-03-09, шаг 1.3).
func (s *selectStage) slotDead() bool { return s.weaver == nil }

// bankEmpty — банк не подключён (пул недоступен).
func (s *selectStage) bankEmpty() bool { return s.bank == nil }

// isEmotional — эмоция или уникальность реплики (LT-05.4): настроение из лексикона
// либо намерение, требующее уникальной генерации (речь, перемещение).
func isEmotional(st *TurnState) bool {
	if st.Prepath.Mood != "" {
		return true
	}
	return st.Prepath.Intent.Name == "речь" || st.Prepath.Intent.Name == "перемещение"
}

// isRepeatable — сцена повторяема (T-03-07): рутинное намерение без эмоции и
// уникальности — банк обслуживает её записью из пула.
func isRepeatable(st *TurnState) bool {
	if isEmotional(st) {
		return false
	}
	return st.Prepath.Intent.Name != ""
}

// executeStage — исполнение выбранного пути (T-03-07/08). Путь Челнока отвечает
// записью из банка рутины (T-03-07), путь Ткача — генерацией прозы с порогом
// тишины (T-03-08). До путей и при пустом банке — нейтральная запись fallback;
// reply_text непуст с первого дня.
type executeStage struct {
	bank   *bank.Bank // Челнок (T-03-07); nil — банк не подключён, ответ заглушкой.
	weaver ProseSlot  // Ткач (T-03-08); nil — слот не подключён, ответ заглушкой.
	clock  Clock      // часы порога тишины Ткача; nil — реальное время.
}

func (*executeStage) Name() string { return "execute" }

func (s *executeStage) Run(ctx context.Context, st *TurnState) error {
	if st.Command != nil {
		st.Reply = commandReply(st.Command.Name)
		return nil
	}
	// Неподтверждённый смысл — переспрос нейтральной записью (T-03-04, шаг 2).
	if st.Confirm.Decision == DecisionReask {
		st.Reply = reaskReply
		return nil
	}
	// Путь Ткача (T-03-08): генерация прозы со стримингом и порогом тишины.
	if st.Path == pathWeaver {
		reply, degraded := s.weaverReply(ctx, st)
		st.Reply = reply
		if degraded {
			st.Degraded = true
			return nil
		}
		// Отрицательная ветка — готовая запись из БД — не вошла в ход и
		// становится кандидатом в оседание (T-03-10, шаги 2–3).
		st.Candidates = append(st.Candidates, SedimentCandidate{
			TurnID: st.TurnID, SessionID: st.Input.SessionID, Kind: BranchBank,
		})
		return nil
	}
	// Путь Челнока (T-03-07): запись из банка рутины по тегам сцены.
	if s.bank != nil {
		if rec, err := s.bank.Pick(sceneTags(st)); err == nil {
			st.Reply = rec.Text
			// Положительная ветка — живой ответ модели — не вошла в ход и
			// становится кандидатом в оседание (T-03-10, шаги 2–3).
			st.Candidates = append(st.Candidates, SedimentCandidate{
				TurnID: st.TurnID, SessionID: st.Input.SessionID, Kind: BranchWeaver,
			})
			return nil
		}
	}
	st.Reply = fallbackReply
	return nil
}

// weaverReply исполняет путь Ткача: собирает промпт из фактов сцены и инструкций
// движка без токенов-заменителей и доставляет сцену порциями — мгновенная
// подкладка, затем живой поток с порогом тишины (T-03-10, шаг 1). Возвращает
// ответ и признак деградации слота (молчание, отказ или незакрытый токен).
func (s *executeStage) weaverReply(ctx context.Context, st *TurnState) (string, bool) {
	clock := s.clock
	if clock == nil {
		clock = wallClock{}
	}
	prompt := models.BuildPrompt(st.Prepath.Prompt, models.WeaverInstruction)
	if models.HasReplacementTokens(prompt) {
		return fallbackReply, true
	}
	var underlay, reply string
	degraded := false
	for p := range streamScene(ctx, st, s.weaver, prompt, clock) {
		switch p.Kind {
		case portionUnderlay:
			underlay += p.Text
		case portionWeaver:
			reply = p.Text
			degraded = p.Degraded
		}
	}
	if degraded {
		return reply, true
	}
	return underlay + reply, false
}

// sceneTags — теги сцены для выбора записи банка (T-03-07, шаг 1): намерение и
// настроение реплики из предпути. Пусто — банк выберет запись по умолчанию.
func sceneTags(st *TurnState) []string {
	var tags []string
	if st.Prepath.Intent.Name != "" {
		tags = append(tags, st.Prepath.Intent.Name)
	}
	if st.Prepath.Mood != "" {
		tags = append(tags, st.Prepath.Mood)
	}
	return tags
}

// commitStage — коммит хода (T-03-05): после полной подтверждённой реплики пишет
// ход одной транзакцией (стоп-кран в stopcrane.go); неподтверждённая ветка
// записей в мир не оставляет.
type commitStage struct {
	db        *db.Handle
	FailTable string // тестовый крючок отката (T-03-05, «Проверка»); пусто в проде.
}

func (*commitStage) Name() string { return "commit" }

func (s *commitStage) Run(_ context.Context, st *TurnState) error {
	st.LatencyMS = time.Since(st.StartedAt).Milliseconds()
	if err := commitTurn(st, s.db, s.FailTable); err != nil {
		return err
	}
	if s.db == nil {
		return nil
	}
	// После закрытия сцены невошедшие ветки уходят заказом sediment в очередь
	// мастерской (T-03-10, шаг 3); в игровые таблицы кандидат не пишется.
	return queueSedimentCandidates(s.db, st.Candidates)
}
