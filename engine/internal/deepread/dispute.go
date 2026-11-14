package deepread

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"tns65/internal/cast"
	"tns65/internal/db"
	"tns65/internal/journal"
)

// Спорные вопросы слоту (W-24, T-14-10): очередь спорных мест разбора, топ-K по
// бюджету, батчи вопросов с проверяемыми цитатами и применение решений. Слот —
// локальный :5001 (запаска) через инфраструктуру мастерской, в тестах — фейк
// testkit.

// EventDeepreadSlot — счётчик вызовов слота спорных вопросов (W-24.5): событие
// журнала на каждый вызов, включая повторы после провала проверки.
const EventDeepreadSlot = "deepread.slot"

// DisputeSlotRole — роль слота вопросов разбора: отдельный маршрут от разбора
// первой реплики (book.DeepReadRole), в тестах — ответ фейка по этой роли.
const DisputeSlotRole = "deepread.dispute"

const (
	// defaultSlotBudget — глубокий бюджет вопросов на книгу по умолчанию (W-24.1).
	defaultSlotBudget = 20
	// defaultSlotBatch — вопросов в одном вызове слота по умолчанию (W-24.1).
	defaultSlotBatch = 5
	// disputeRetries — повторы батча при провале проверки (W-24.3): всего не
	// более трёх вызовов на батч.
	disputeRetries = 2
	// maxDisputeSignals — нормировка силы противоречия для ранжирования (шаг 1).
	maxDisputeSignals = 3
)

// DisputeKind — тип спорного вопроса (W-24.2): выбор из канона, не свободное
// суждение.
type DisputeKind string

const (
	DisputeRole     DisputeKind = "role"     // роль персонажа по отрезку (P11)
	DisputeCategory DisputeKind = "category" // категория чанка из ключей словарей
	DisputeMerge    DisputeKind = "merge"    // «тот ли это персонаж»
	DisputeBoundary DisputeKind = "boundary" // граница сцен
)

// Канон ответов на вопросы с двумя вариантами (W-24.2): значения фиксированы,
// выдумывать варианты нельзя.
const (
	AnswerMergeSame      = "same"
	AnswerMergeDifferent = "different"
	AnswerBoundarySplit  = "split"
	AnswerBoundaryJoin   = "join"
)

// DisputeState — судьба спорного вопроса после вопросов слоту.
type DisputeState string

const (
	// DisputeUncertain — ответа нет: сверх бюджета, без слота или при пустом
	// ответе спор остаётся с полосой «спорно» (W-24.1, W-24.4).
	DisputeUncertain DisputeState = "uncertain"
	// DisputeAnswered — проверенный ответ получен.
	DisputeAnswered DisputeState = "answered"
	// DisputeEmpty — слот ответил пустой строкой: спор остаётся пустым (W-24.4).
	DisputeEmpty DisputeState = "empty"
	// DisputeFailed — проверка не прошла после всех повторов (W-24.3).
	DisputeFailed DisputeState = "failed"
)

// DisputeConfig — бюджет вопросов книге и размер батча (W-24.1): значения
// deepread.slot_budget и deepread.slot_batch конфигурации.
type DisputeConfig struct {
	Budget int // вопросов на книгу; 0 — слот не спрашивают
	Batch  int // вопросов в одном вызове слота
}

// DefaultDisputeConfig возвращает значения, совпадающие с config.yaml.
func DefaultDisputeConfig() DisputeConfig {
	return DisputeConfig{Budget: defaultSlotBudget, Batch: defaultSlotBatch}
}

// LoadDisputeConfig читает бюджет и батч из файла конфигурации.
func LoadDisputeConfig(path string) (DisputeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DisputeConfig{}, err
	}
	return parseDisputeSection(string(data))
}

// parseDisputeSection разбирает плоские ключи секции deepread
// (slot_budget, slot_batch); вложенные блоки (thresholds) не читаются.
func parseDisputeSection(data string) (DisputeConfig, error) {
	cfg := DefaultDisputeConfig()

	inDeepread := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inDeepread = trimmed == "deepread:"
			continue
		}
		if !inDeepread || indent != 2 {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		n, err := strconv.Atoi(value)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(key) {
		case "slot_budget":
			cfg.Budget = n
		case "slot_batch":
			cfg.Batch = n
		}
	}
	if err := sc.Err(); err != nil {
		return DisputeConfig{}, err
	}
	if err := validateDisputeConfig(cfg); err != nil {
		return DisputeConfig{}, err
	}
	return cfg, nil
}

// validateDisputeConfig проверяет бюджет и батч: budget ≥ 0, batch ≥ 1.
func validateDisputeConfig(cfg DisputeConfig) error {
	if cfg.Budget < 0 {
		return fmt.Errorf("deepread: slot_budget %d < 0", cfg.Budget)
	}
	if cfg.Batch < 1 {
		return fmt.Errorf("deepread: slot_batch %d < 1", cfg.Batch)
	}
	return nil
}

// DisputeSlot — слот вопросов разбора: testkit.FakeSlot в тестах, локальный
// :5001 (запаска) в производстве (W-24.5).
type DisputeSlot interface {
	Generate(ctx context.Context, role, prompt string) (string, error)
}

// DisputeQuestion — один спорный вопрос слоту (W-24.2): тип, канон вариантов,
// текст окна и ожидаемый эффект для ранжирования. Поля привязки заполняет
// источник вопроса; по ним решение применяется к записям карты (шаг 5).
type DisputeQuestion struct {
	Key     string      // стабильный ключ спора в пределах книги
	Kind    DisputeKind // тип вопроса
	ChunkID int64       // чанк окна
	Window  string      // текст окна: цитата обязана быть его подстрокой
	Options []string    // каноничный список вариантов
	Impact  float64     // ожидаемый эффект 0..1 для ранжирования
	Score   float64     // скор тега категории (правка book_chunk_tag)

	TagKey      string // ключ спорного тега категории
	CharacterID int64  // персонаж спорной роли
	FromChunk   int64  // начало отрезка (роль, граница)
	ToChunk     int64  // конец отрезка (роль, граница)
	SeniorID    int64  // старшая карточка слияния
	JuniorID    int64  // младшая карточка слияния
	JuniorName  string // имя младшей карточки для алиаса
}

// DisputedRole — спорный отрезок роли (W-25.4): маркеры дают разные роли или
// только слабый сигнал; отрезок и окно задаёт извлекатель ролей карты.
type DisputedRole struct {
	CharacterID int64
	FromChunk   int64
	ToChunk     int64
	Signals     []string
	Window      string
}

// DisputeInput — источники очереди спорных (шаг 1): полосы uncertain тегов,
// пограничные слияния (T-14-07.4), противоречивые границы сцен (T-14-08.3),
// спорные отрезки ролей (W-25.4) и тексты чанков для окон.
type DisputeInput struct {
	BookID     int64
	Tags       []ChunkTags
	Merges     []DisputedAlias
	Boundaries []DisputedBoundary
	Roles      []DisputedRole
	Windows    map[int64]string
	TagKeys    []string // канон ключей словарей; пусто — ключи тегов книги
}

// DisputeOutcome — итог по одному спорному вопросу.
type DisputeOutcome struct {
	Question   DisputeQuestion
	State      DisputeState
	Value      string
	Quote      string
	Confidence float64
	Attempts   int // вызовов слота на батч, ответивший вопросом
}

// BuildDisputes собирает очередь спорных (шаг 1) и ранжирует её по убыванию
// ожидаемого эффекта: категория — скор тега (вклад в карточку чанка), слияние —
// косинус центроидов, роль и граница — число противоречивых сигналов. Бюджет
// здесь не режется: его применяет ResolveDisputes (шаг 2).
func BuildDisputes(in DisputeInput) ([]DisputeQuestion, error) {
	canon := tagCanon(in)
	out := make([]DisputeQuestion, 0, len(in.Tags)+len(in.Merges)+len(in.Boundaries)+len(in.Roles))
	seen := make(map[string]struct{}, cap(out))

	add := func(q DisputeQuestion) {
		if _, dup := seen[q.Key]; dup {
			return
		}
		seen[q.Key] = struct{}{}
		out = append(out, q)
	}

	for _, chunk := range in.Tags {
		if chunk.ChunkID <= 0 {
			return nil, fmt.Errorf("deepread: спор категории: chunk_id %d", chunk.ChunkID)
		}
		for _, tag := range chunk.Tags {
			if tag.Band != BandUncertain || tag.Key == "" {
				continue
			}
			add(DisputeQuestion{
				Key:     fmt.Sprintf("category:%d:%s", chunk.ChunkID, tag.Key),
				Kind:    DisputeCategory,
				ChunkID: chunk.ChunkID,
				TagKey:  tag.Key,
				Window:  in.Windows[chunk.ChunkID],
				Options: canon,
				Impact:  clamp01(tag.Score),
				Score:   tag.Score,
			})
		}
	}

	for _, merge := range in.Merges {
		if merge.SeniorID <= 0 || merge.JuniorID <= 0 || merge.SeniorID == merge.JuniorID {
			return nil, fmt.Errorf("deepread: спор слияния: карточки %d/%d", merge.SeniorID, merge.JuniorID)
		}
		chunkID, window := windowForName(in.Windows, merge.JuniorName)
		add(DisputeQuestion{
			Key:        fmt.Sprintf("merge:%d:%d", merge.SeniorID, merge.JuniorID),
			Kind:       DisputeMerge,
			ChunkID:    chunkID,
			Window:     window,
			Options:    []string{AnswerMergeSame, AnswerMergeDifferent},
			Impact:     clamp01(merge.Cosine),
			SeniorID:   merge.SeniorID,
			JuniorID:   merge.JuniorID,
			JuniorName: merge.JuniorName,
		})
	}

	for _, boundary := range in.Boundaries {
		if boundary.FromChunk <= 0 || boundary.ToChunk <= boundary.FromChunk {
			return nil, fmt.Errorf("deepread: спор границы: %d→%d", boundary.FromChunk, boundary.ToChunk)
		}
		add(DisputeQuestion{
			Key:       fmt.Sprintf("boundary:%d:%d", boundary.FromChunk, boundary.ToChunk),
			Kind:      DisputeBoundary,
			ChunkID:   boundary.FromChunk,
			Window:    windowPair(in.Windows, boundary.FromChunk, boundary.ToChunk),
			Options:   []string{AnswerBoundarySplit, AnswerBoundaryJoin},
			Impact:    signalImpact(len(boundary.Signals)),
			FromChunk: boundary.FromChunk,
			ToChunk:   boundary.ToChunk,
		})
	}

	roles := roleCanon()
	for _, role := range in.Roles {
		if role.CharacterID <= 0 || role.FromChunk <= 0 || role.ToChunk < role.FromChunk {
			return nil, fmt.Errorf("deepread: спор роли: персонаж %d, отрезок %d..%d",
				role.CharacterID, role.FromChunk, role.ToChunk)
		}
		window := role.Window
		if strings.TrimSpace(window) == "" {
			window = windowRange(in.Windows, role.FromChunk, role.ToChunk)
		}
		add(DisputeQuestion{
			Key:         fmt.Sprintf("role:%d:%d:%d", role.CharacterID, role.FromChunk, role.ToChunk),
			Kind:        DisputeRole,
			ChunkID:     role.FromChunk,
			Window:      window,
			Options:     roles,
			Impact:      signalImpact(len(role.Signals)),
			CharacterID: role.CharacterID,
			FromChunk:   role.FromChunk,
			ToChunk:     role.ToChunk,
		})
	}

	RankDisputes(out)
	return out, nil
}

// RankDisputes сортирует очередь по убыванию ожидаемого эффекта; равные эффекты
// разводятся порядком типов (категория, слияние, роль, граница) и ключом спора,
// чтобы порядок был воспроизводим.
func RankDisputes(questions []DisputeQuestion) {
	sort.SliceStable(questions, func(i, j int) bool {
		a, b := questions[i], questions[j]
		if a.Impact != b.Impact {
			return a.Impact > b.Impact
		}
		if ra, rb := disputeKindRank(a.Kind), disputeKindRank(b.Kind); ra != rb {
			return ra < rb
		}
		return a.Key < b.Key
	})
}

// disputeKindRank — порядок типов при равном эффекте.
func disputeKindRank(kind DisputeKind) int {
	switch kind {
	case DisputeCategory:
		return 0
	case DisputeMerge:
		return 1
	case DisputeRole:
		return 2
	case DisputeBoundary:
		return 3
	}
	return 4
}

// ResolveDisputes задаёт слоту топ бюджета вопросов батчами (шаги 2–4) и
// возвращает итог по каждому вопросу в порядке ранжирования. Споры сверх бюджета
// остаются uncertain; без слота разбор завершается без запросов (приёмка W-24).
func ResolveDisputes(questions []DisputeQuestion, cfg DisputeConfig, slot DisputeSlot, logger *journal.Logger) ([]DisputeOutcome, error) {
	if cfg == (DisputeConfig{}) {
		cfg = DefaultDisputeConfig()
	}
	if err := validateDisputeConfig(cfg); err != nil {
		return nil, err
	}

	queue := append([]DisputeQuestion(nil), questions...)
	RankDisputes(queue)
	out := make([]DisputeOutcome, len(queue))
	for i := range queue {
		out[i] = DisputeOutcome{Question: queue[i], State: DisputeUncertain}
	}

	limit := cfg.Budget
	if limit > len(queue) {
		limit = len(queue)
	}
	if limit <= 0 || slot == nil {
		return out, nil
	}

	for start := 0; start < limit; start += cfg.Batch {
		end := start + cfg.Batch
		if end > limit {
			end = limit
		}
		copy(out[start:end], askBatch(queue[start:end], start/cfg.Batch+1, slot, logger))
	}
	return out, nil
}

// askBatch задаёт один батч вопросов: вызов слота, проверка ответа, повтор при
// провале не более disputeRetries раз. Каждый вызов помечается событием
// deepread.slot (W-24.5); исчерпанные повторы помечают батч failed.
func askBatch(batch []DisputeQuestion, num int, slot DisputeSlot, logger *journal.Logger) []DisputeOutcome {
	prompt := disputePrompt(batch)
	attempts := 0
	var lastErr error
	for {
		attempts++
		if logger != nil {
			_ = logger.Event("deepread", EventDeepreadSlot,
				journal.Field{Key: "batch", Value: strconv.Itoa(num)},
				journal.Field{Key: "questions", Value: strconv.Itoa(len(batch))})
		}
		raw, err := slot.Generate(context.Background(), DisputeSlotRole, prompt)
		if err == nil {
			out, parseErr := parseDisputeAnswers(batch, raw)
			if parseErr == nil {
				for i := range out {
					out[i].Attempts = attempts
				}
				return out
			}
			err = parseErr
		}
		lastErr = err
		if attempts > disputeRetries {
			break
		}
	}

	if logger != nil {
		_ = logger.Event("deepread", journal.EventSlotDegraded,
			journal.Field{Key: "kind", Value: "dispute"},
			journal.Field{Key: "batch", Value: strconv.Itoa(num)},
			journal.Field{Key: "questions", Value: strconv.Itoa(len(batch))},
			journal.Field{Key: "reason", Value: lastErr.Error()})
	}

	out := make([]DisputeOutcome, len(batch))
	for i, q := range batch {
		out[i] = DisputeOutcome{Question: q, State: DisputeFailed, Attempts: attempts}
	}
	return out
}

// disputePrompt строит вопрос слота (W-24.2): тип, каноничный список вариантов и
// текст окна по каждому вопросу батча. Ответ — JSON-массив объектов
// {"value", "quote", "confidence"} в том же порядке.
func disputePrompt(batch []DisputeQuestion) string {
	var b strings.Builder
	b.WriteString("Ты отвечаешь на спорные вопросы разбора книги. Ответ — JSON-массив объектов ")
	b.WriteString(`{"value": ..., "quote": ..., "confidence": 0..1}, по одному на вопрос в том же порядке. `)
	b.WriteString("value — только из списка вариантов вопроса или пустая строка; ")
	b.WriteString("quote — дословная цитата из окна; confidence — 0..1.\n")
	for i, q := range batch {
		options, _ := json.Marshal(q.Options)
		fmt.Fprintf(&b, "\nВопрос %d (тип %s): варианты %s.\nОкно: %s\n", i+1, q.Kind, options, q.Window)
	}
	return b.String()
}

// disputeAnswer — объект ответа слота по одному вопросу (W-24.3).
type disputeAnswer struct {
	Value      string  `json:"value"`
	Quote      string  `json:"quote"`
	Confidence float64 `json:"confidence"`
}

// parseDisputeAnswers проверяет ответ батча (W-24.3): число ответов, value из
// канона (пустое допустимо), цитата — подстрока окна после нормализации,
// confidence в 0..1. Батч из одного вопроса принимает и одиночный объект.
func parseDisputeAnswers(batch []DisputeQuestion, raw string) ([]DisputeOutcome, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, errors.New("deepread: пустой ответ слота на спорные вопросы")
	}

	var answers []disputeAnswer
	if strings.HasPrefix(text, "[") {
		if err := json.Unmarshal([]byte(text), &answers); err != nil {
			return nil, fmt.Errorf("deepread: ответ слота не JSON-массив: %w", err)
		}
	} else {
		var one disputeAnswer
		if err := json.Unmarshal([]byte(text), &one); err != nil {
			return nil, fmt.Errorf("deepread: ответ слота не JSON: %w", err)
		}
		answers = []disputeAnswer{one}
	}
	if len(answers) != len(batch) {
		return nil, fmt.Errorf("deepread: ответов %d, вопросов %d", len(answers), len(batch))
	}

	out := make([]DisputeOutcome, len(batch))
	for i, q := range batch {
		answer := answers[i]
		value := strings.TrimSpace(answer.Value)
		if value != "" && !hasOption(q.Options, value) {
			return nil, fmt.Errorf("deepread: вопрос %q: значение %q вне канона", q.Key, value)
		}
		if math.IsNaN(answer.Confidence) || answer.Confidence < 0 || answer.Confidence > 1 {
			return nil, fmt.Errorf("deepread: вопрос %q: confidence %g вне 0..1", q.Key, answer.Confidence)
		}
		if !quoteInWindow(answer.Quote, q.Window) {
			return nil, fmt.Errorf("deepread: вопрос %q: цитата %q не является подстрокой окна", q.Key, answer.Quote)
		}

		state := DisputeAnswered
		if value == "" {
			// Слот не добавляет сущностей: пустой ответ оставляет спор пустым (W-24.4).
			state = DisputeEmpty
		}
		out[i] = DisputeOutcome{
			Question:   q,
			State:      state,
			Value:      value,
			Quote:      answer.Quote,
			Confidence: answer.Confidence,
		}
	}
	return out, nil
}

// quoteInWindow — проверка цитаты (W-24.3): после нормализации цитата обязана
// быть подстрокой окна; пустая цитата допустима.
func quoteInWindow(quote, window string) bool {
	quote = normalizeQuote(quote)
	if quote == "" {
		return true
	}
	return strings.Contains(normalizeQuote(window), quote)
}

// normalizeQuote — нормализация для сверки цитаты с окном (образец T-07-03.4):
// нижний регистр и схлопывание пробелов.
func normalizeQuote(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// ApplyDecision применяет принятое решение к записям карты (шаг 5): роль — строка
// book_character_role (P11), слияние — имя младшего в алиасах старшего. Решение
// категории и границы пишут писатели T-14-05 (AppliedTag) и T-14-08
// (ApplyBoundaries); пустой или отсутствующий ответ записей не меняет.
func ApplyDecision(lit *db.Handle, out DisputeOutcome) error {
	if out.State != DisputeAnswered || out.Value == "" {
		return nil
	}
	q := out.Question
	switch q.Kind {
	case DisputeRole:
		if lit == nil {
			return ErrNoStore
		}
		if q.CharacterID <= 0 || q.FromChunk <= 0 || q.ToChunk < q.FromChunk {
			return fmt.Errorf("deepread: решение роли: персонаж %d, отрезок %d..%d",
				q.CharacterID, q.FromChunk, q.ToChunk)
		}
		evidence := strings.TrimSpace(out.Quote)
		if evidence == "" {
			evidence = EventDeepreadSlot
		}
		if _, err := lit.Exec("book_character_role",
			`INSERT INTO book_character_role (character_id, from_chunk, to_chunk, role, confidence, evidence) VALUES (?, ?, ?, ?, ?, ?)`,
			q.CharacterID, q.FromChunk, q.ToChunk, out.Value, out.Confidence, evidence); err != nil {
			return fmt.Errorf("deepread: роль персонажа %d: %w", q.CharacterID, err)
		}
		return nil
	case DisputeMerge:
		if out.Value != AnswerMergeSame {
			// «Разные» — кандидаты остаются раздельными, записи не меняются.
			return nil
		}
		if lit == nil {
			return ErrNoStore
		}
		return applyMerge(lit, q)
	}
	return nil
}

// applyMerge записывает решение о слиянии (T-14-07): имя младшей карточки уходит
// в алиасы старшей, младшая остаётся в карте со статусом merged.
func applyMerge(lit *db.Handle, q DisputeQuestion) error {
	if q.SeniorID <= 0 || q.JuniorID <= 0 || strings.TrimSpace(q.JuniorName) == "" {
		return fmt.Errorf("deepread: слияние: карточки %d/%d без имени младшей", q.SeniorID, q.JuniorID)
	}

	var raw string
	err := lit.QueryRow(`SELECT aliases FROM book_character WHERE id = ?`, q.SeniorID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("deepread: слияние: персонаж %d не найден в карте", q.SeniorID)
	}
	if err != nil {
		return fmt.Errorf("deepread: слияние: алиасы персонажа %d: %w", q.SeniorID, err)
	}
	var aliases []string
	if err := json.Unmarshal([]byte(raw), &aliases); err != nil {
		return fmt.Errorf("deepread: слияние: алиасы персонажа %d: %w", q.SeniorID, err)
	}
	aliases = appendAlias(aliases, strings.TrimSpace(q.JuniorName))
	data, err := json.Marshal(aliases)
	if err != nil {
		return fmt.Errorf("deepread: слияние: алиасы персонажа %d: %w", q.SeniorID, err)
	}

	if _, err := lit.Exec("book_character",
		`UPDATE book_character SET aliases = ? WHERE id = ?`, string(data), q.SeniorID); err != nil {
		return fmt.Errorf("deepread: слияние: персонаж %d: %w", q.SeniorID, err)
	}
	if _, err := lit.Exec("book_character",
		`UPDATE book_character SET status = ? WHERE id = ?`, "merged", q.JuniorID); err != nil {
		return fmt.Errorf("deepread: слияние: персонаж %d: %w", q.JuniorID, err)
	}
	return nil
}

// AppliedTag отдаёт решение категории тегом чанка с полосой confident: строку
// пишет T-14-05 (WriteChunkTags), цитата ответа становится якорем. ok=false —
// решения нет.
func (o DisputeOutcome) AppliedTag() (ChunkTag, bool) {
	if o.State != DisputeAnswered || o.Question.Kind != DisputeCategory || o.Value == "" {
		return ChunkTag{}, false
	}
	tag := ChunkTag{Key: o.Value, Score: o.Question.Score, Band: BandConfident}
	if quote := strings.TrimSpace(o.Quote); quote != "" {
		tag.Anchors = []string{quote}
	}
	return tag, true
}

// ApplyBoundaries применяет решения о границах к набору сцен до записи T-14-08:
// «одна сцена» склеивает смежные сцены по границе, «разрез» оставляет как есть.
func ApplyBoundaries(set *BookScenes, outcomes []DisputeOutcome) error {
	if set == nil {
		return fmt.Errorf("deepread: границы: нет набора сцен")
	}
	for _, out := range outcomes {
		if out.State != DisputeAnswered || out.Question.Kind != DisputeBoundary {
			continue
		}
		if out.Value != AnswerBoundaryJoin {
			continue
		}
		scenes, err := joinScenes(set.Scenes, out.Question.FromChunk, out.Question.ToChunk)
		if err != nil {
			return err
		}
		set.Scenes = scenes
	}
	return nil
}

// joinScenes склеивает сцены по стыку from→to: диапазон и участники объединяются,
// доли эмоций смешиваются с весом по числу чанков, ord перенумеровывается.
func joinScenes(scenes []Scene, from, to int64) ([]Scene, error) {
	for i := 0; i+1 < len(scenes); i++ {
		if scenes[i].ToChunk != from || scenes[i+1].FromChunk != to {
			continue
		}
		first, second := scenes[i], scenes[i+1]
		firstChunks := first.ToChunk - first.FromChunk + 1
		secondChunks := second.ToChunk - second.FromChunk + 1
		first.ToChunk = second.ToChunk
		first.Participants = unionStrings(first.Participants, second.Participants)
		first.Tags = unionStrings(first.Tags, second.Tags)
		first.Emotion = mergeEmotion(first.Emotion, firstChunks, second.Emotion, secondChunks)

		out := make([]Scene, 0, len(scenes)-1)
		out = append(out, scenes[:i]...)
		out = append(out, first)
		out = append(out, scenes[i+2:]...)
		for j := range out {
			out[j].Ord = j
		}
		return out, nil
	}
	return nil, fmt.Errorf("deepread: граница %d→%d не найдена среди сцен", from, to)
}

// mergeEmotion смешивает доли эмоций двух сцен с весом по числу чанков.
func mergeEmotion(first map[string]float64, firstChunks int64, second map[string]float64, secondChunks int64) map[string]float64 {
	if len(first) == 0 && len(second) == 0 {
		return nil
	}
	out := make(map[string]float64, len(first)+len(second))
	for _, key := range unionStrings(mapKeys(first), mapKeys(second)) {
		out[key] = (first[key]*float64(firstChunks) + second[key]*float64(secondChunks)) /
			float64(firstChunks+secondChunks)
	}
	return out
}

// mapKeys — ключи карты в отсортированном порядке.
func mapKeys(m map[string]float64) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// unionStrings объединяет списки без повторов, сохраняя порядок первого списка.
func unionStrings(first, second []string) []string {
	seen := make(map[string]struct{}, len(first)+len(second))
	out := make([]string, 0, len(first)+len(second))
	for _, list := range [][]string{first, second} {
		for _, item := range list {
			if item == "" {
				continue
			}
			if _, dup := seen[item]; dup {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

// hasOption проверяет значение в каноне вариантов.
func hasOption(options []string, value string) bool {
	for _, option := range options {
		if option == value {
			return true
		}
	}
	return false
}

// tagCanon — каноничный список ключей категорий (W-24.2): список словарей от
// извлекателя, без него — ключи тегов книги в лексикографическом порядке.
func tagCanon(in DisputeInput) []string {
	if len(in.TagKeys) > 0 {
		out := make([]string, 0, len(in.TagKeys))
		seen := make(map[string]struct{}, len(in.TagKeys))
		for _, key := range in.TagKeys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, key)
		}
		if len(out) > 0 {
			return out
		}
	}

	seen := make(map[string]struct{})
	out := make([]string, 0, 8)
	for _, chunk := range in.Tags {
		for _, tag := range chunk.Tags {
			if tag.Key == "" {
				continue
			}
			if _, dup := seen[tag.Key]; dup {
				continue
			}
			seen[tag.Key] = struct{}{}
			out = append(out, tag.Key)
		}
	}
	sort.Strings(out)
	return out
}

// roleCanon — закрытый список ролей карты (P11): варианты вопроса берутся из
// канона cast, выдумывать роль нельзя.
func roleCanon() []string {
	roles := cast.RoleList()
	out := make([]string, len(roles))
	for i, role := range roles {
		out[i] = string(role)
	}
	return out
}

// sortedWindowIDs — ключи окон по возрастанию ord-подобного порядка.
func sortedWindowIDs(windows map[int64]string) []int64 {
	out := make([]int64, 0, len(windows))
	for id := range windows {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// windowForName ищет первое по порядку чанков окно с именем младшей карточки.
func windowForName(windows map[int64]string, name string) (int64, string) {
	name = normalizeQuote(name)
	if name == "" {
		return 0, ""
	}
	for _, id := range sortedWindowIDs(windows) {
		if strings.Contains(normalizeQuote(windows[id]), name) {
			return id, windows[id]
		}
	}
	return 0, ""
}

// windowPair — окно границы: тексты обоих чанков через перевод строки.
func windowPair(windows map[int64]string, from, to int64) string {
	parts := make([]string, 0, 2)
	for _, id := range []int64{from, to} {
		if text := strings.TrimSpace(windows[id]); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// windowRange — окно отрезка: тексты чанков диапазона по порядку.
func windowRange(windows map[int64]string, from, to int64) string {
	parts := make([]string, 0, int(to-from)+1)
	for _, id := range sortedWindowIDs(windows) {
		if id < from || id > to {
			continue
		}
		if text := strings.TrimSpace(windows[id]); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// signalImpact — ожидаемый эффект спора по числу противоречивых сигналов (шаг 1):
// больше сигналов — сильнее влияние на карточку или сцену.
func signalImpact(signals int) float64 {
	if signals <= 0 {
		return 0
	}
	return clamp01(float64(signals) / maxDisputeSignals)
}
