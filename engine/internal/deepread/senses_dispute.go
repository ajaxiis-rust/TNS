package deepread

// senses_dispute.go — спорные смыслы омонимов (T-15-04): омоним с нерешённым
// смыслом уходит вопросом слоту в общем бюджете спорных (W-24), ответ меняет
// теги текущего прогона и пишется в forge_log kind='deepread_sense'.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/senses"
	"tns65/internal/version"
)

// DisputeSense — тип спорного вопроса: смысл омонима (W-24.6, T-15-04).
const DisputeSense DisputeKind = "sense"

// KindSenseDispute — kind строк forge_log с ответами по спорным смыслам
// (писатель forge): отдельный kind, чтобы строки не путались с ключом разбора.
const KindSenseDispute = "deepread_sense"

// senseMarginFactor — множитель порога отрыва спорного смысла (шаг 1): смысл
// спорен, если отрыв не выше senses_margin × 2.
const senseMarginFactor = 2

// SenseGroup — ключ словаря смыслов: лемма и часть речи омонима.
type SenseGroup struct {
	Lemma string
	POS   string
}

// SenseOption — смысл в вопросе слота (шаг 2): имя, категория словаря разбора
// (пусто — смысл вне категорий) и триггеры-свидетели.
type SenseOption struct {
	Sense    string   `json:"sense"`
	Category string   `json:"category,omitempty"`
	Triggers []string `json:"triggers"`
}

// SenseCatalog — смыслы пака по группам омонимов: канон имён смыслов вопроса.
type SenseCatalog map[SenseGroup][]SenseOption

// SenseQuestion — спорный смысл омонима (шаг 2): лемма, часть речи, номер
// токена в чанке, цитата-подстрока чанка и смыслы с триггерами. Window — текст
// чанка, по нему проверяется цитата ответа.
type SenseQuestion struct {
	Key     string        `json:"key"`
	ChunkID int64         `json:"chunk_id"`
	Token   int           `json:"token"`
	Lemma   string        `json:"lemma"`
	POS     string        `json:"pos"`
	Quote   string        `json:"quote"`
	Senses  []SenseOption `json:"senses"`
	Options []string      `json:"options"`
	Score   float64       `json:"score"`
	Margin  float64       `json:"margin"`
	Window  string        `json:"-"`
}

// SenseOutcome — итог по одному смысловому вопросу: состояние, выбранный смысл
// и его категория (по ней считается тег), цитата ответа.
type SenseOutcome struct {
	Question   SenseQuestion `json:"question"`
	State      DisputeState  `json:"state"`
	Value      string        `json:"value,omitempty"`
	Category   string        `json:"category,omitempty"`
	Quote      string        `json:"quote,omitempty"`
	Confidence float64       `json:"confidence,omitempty"`
	Attempts   int           `json:"attempts,omitempty"`
}

// SenseDisputeRecord — запись forge_log о смысловых вопросах прогона: ключ
// пересчёта и итог по каждому вопросу (шаг 3). Повторный прогон с тем же ключом
// вопросы не повторяет (шаг 4).
type SenseDisputeRecord struct {
	Key     Key            `json:"key"`
	Results []SenseOutcome `json:"sense_disputes"`
}

// LoadSenseCatalog читает senses.json пака в каталог смыслов для вопросов слота
// (шаг 2); файла нет — nil без ошибки, вопросов не будет.
func LoadSenseCatalog(packDir string) (SenseCatalog, error) {
	path := filepath.Join(packDir, sensesFileName)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var f struct {
		Version int            `json:"version"`
		Senses  []senses.Sense `json:"senses"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("deepread: смыслы пака: %s: %w", filepath.Base(path), err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("deepread: смыслы пака: %s: версия схемы %d, ожидалась 1",
			filepath.Base(path), f.Version)
	}

	cat := make(SenseCatalog, len(f.Senses))
	for _, s := range f.Senses {
		lemma := senseLemma(s.Lemma)
		pos := strings.ToUpper(strings.TrimSpace(s.POS))
		name := strings.TrimSpace(s.Sense)
		if lemma == "" || pos == "" || name == "" {
			continue
		}
		option := SenseOption{
			Sense:    name,
			Category: strings.ToLower(strings.TrimSpace(s.Category)),
			Triggers: make([]string, 0, len(s.Triggers)),
		}
		for _, tr := range s.Triggers {
			if trigger := senseLemma(tr.Lemma); trigger != "" {
				option.Triggers = append(option.Triggers, trigger)
			}
		}
		group := SenseGroup{Lemma: lemma, POS: pos}
		cat[group] = append(cat[group], option)
	}
	if len(cat) == 0 {
		return nil, nil
	}
	return cat, nil
}

// senseLemma приводит лемму к виду словаря смыслов: нижний регистр без
// окружающих пробелов.
func senseLemma(lemma string) string {
	return strings.ToLower(strings.TrimSpace(lemma))
}

// SenseDisputeInput — источники смысловых вопросов (шаг 1): чанки книги с
// разборами, смысловой слой прогона и каталог смыслов пака.
type SenseDisputeInput struct {
	Chunks  []SalienceChunk
	Layer   *SenseLayer
	Catalog SenseCatalog
}

// BuildSenseDisputes собирает очередь спорных смыслов (шаг 1): омоним, чей смысл
// не решён слоем, но у которого сработал хотя бы один триггер либо отрыв не выше
// senses_margin × 2, уходит вопросом с цитатой, номером токена и смыслами с
// триггерами. Очередь ранжируется по убыванию скора (шаг 2). nil-слой или пустой
// каталог — вопросов нет.
func BuildSenseDisputes(in SenseDisputeInput) ([]SenseQuestion, error) {
	if in.Layer == nil || in.Layer.Dict == nil || len(in.Catalog) == 0 {
		return nil, nil
	}
	margin := in.Layer.margin()

	var out []SenseQuestion
	for _, chunk := range in.Chunks {
		if chunk.ID <= 0 {
			return nil, fmt.Errorf("deepread: спорный смысл: chunk_id %d", chunk.ID)
		}
		tokens, lemmaAt, keys := senseTokenList(chunk)
		if len(tokens) == 0 {
			continue
		}
		occurrences := formOccurrences(chunk.Lemmas)
		for _, d := range in.Layer.Dict.Resolve(tokens) {
			key := keys[d.Index]
			sensesOf := in.Catalog[SenseGroup{Lemma: key.lemma, POS: key.pos}]
			if len(sensesOf) < 2 {
				continue
			}
			if d.Decided && d.Margin+senseEps >= margin {
				continue
			}
			if len(d.Evidence) == 0 && d.Margin > senseMarginFactor*margin+senseEps {
				continue
			}

			at := lemmaAt[d.Index]
			token := at + 1
			out = append(out, SenseQuestion{
				Key:     fmt.Sprintf("sense:%d:%d:%s:%s", chunk.ID, token, key.lemma, key.pos),
				ChunkID: chunk.ID,
				Token:   token,
				Lemma:   key.lemma,
				POS:     key.pos,
				Quote:   senseQuote(chunk.Text, chunk.Lemmas[at].Form, occurrences[at]),
				Senses:  sensesOf,
				Options: senseOptions(sensesOf),
				Score:   d.Score,
				Margin:  d.Margin,
				Window:  chunk.Text,
			})
		}
	}

	RankSenseDisputes(out)
	return out, nil
}

// RankSenseDisputes сортирует очередь смысловых вопросов: выше скор — сильнее
// ожидаемый эффект; равные скоры разводит меньший отрыв, затем ключ вопроса.
func RankSenseDisputes(questions []SenseQuestion) {
	sort.SliceStable(questions, func(i, j int) bool {
		a, b := questions[i], questions[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Margin != b.Margin {
			return a.Margin < b.Margin
		}
		return a.Key < b.Key
	})
}

// senseOptions — канон имён смыслов вопроса в порядке словаря.
func senseOptions(sensesOf []SenseOption) []string {
	out := make([]string, 0, len(sensesOf))
	for _, option := range sensesOf {
		out = append(out, option.Sense)
	}
	return out
}

// senseTokenList строит токены чанка для смыслового разбора: по токену на разбор
// леммы; lemmaAt — индекс леммы чанка, keys — ключ группы (лемма, часть речи).
func senseTokenList(chunk SalienceChunk) ([]senses.Token, []int, []senseKey) {
	tokens := make([]senses.Token, 0, len(chunk.Lemmas))
	lemmaAt := make([]int, 0, len(chunk.Lemmas))
	keys := make([]senseKey, 0, len(chunk.Lemmas))
	for i, lemma := range chunk.Lemmas {
		for _, parse := range lemma.parses() {
			if parse.Lemma == "" {
				continue
			}
			tokens = append(tokens, senses.Token{
				Form:  lemma.Form,
				Lemma: parse.Lemma,
				POS:   parse.Pos,
				Line:  lemma.Line,
				Index: lemma.Index,
			})
			lemmaAt = append(lemmaAt, i)
			keys = append(keys, senseKey{lemma: parse.Lemma, pos: parse.Pos})
		}
	}
	return tokens, lemmaAt, keys
}

// formOccurrences — сколько раз форма леммы уже встречалась в чанке до неё: по
// нему выбирается предложение-цитата нужного вхождения.
func formOccurrences(lemmas []Lemma) []int {
	out := make([]int, len(lemmas))
	seen := make(map[string]int, len(lemmas))
	for i, lemma := range lemmas {
		form := senseLemma(lemma.Form)
		out[i] = seen[form]
		seen[form]++
	}
	return out
}

// senseQuote — цитата-подстрока чанка для токена (шаг 2): предложение с
// occurrence-м (с нуля) вхождением формы; не нашлось — текст чанка.
func senseQuote(text, form string, occurrence int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	want := senseLemma(form)
	runes := []rune(text)
	spans, _ := splitSentences(runes)
	n := 0
	for _, span := range spans {
		for _, word := range wordsOf(runes[span.start:span.end]) {
			if word != want {
				continue
			}
			if n == occurrence {
				return strings.TrimSpace(string(runes[span.start:span.end]))
			}
			n++
		}
	}
	return text
}

// AskSenseDisputes задаёт слоту спорные смыслы прогона (шаги 2–4): вопросы
// уходят в общем бюджете deepread.slot_budget, принятые ответы пишутся в
// forge_log kind='deepread_sense'. Прогон с тем же ключом пересчёта вопросов не
// повторяет и возвращает записанные ответы; без базы или ключа запись не
// пишется.
func AskSenseDisputes(lit *db.Handle, key Key, questions []SenseQuestion, cfg DisputeConfig,
	slot DisputeSlot, logger *journal.Logger, now string) ([]SenseOutcome, error) {
	if lit != nil && key.Valid() {
		recorded, found, err := findSenseDisputes(lit, key)
		if err != nil {
			return nil, err
		}
		if found {
			return recorded, nil
		}
	}

	out, err := resolveSenseQuestions(questions, cfg, slot, logger)
	if err != nil {
		return nil, err
	}
	if lit != nil && key.Valid() && senseAsked(out) {
		if err := writeSenseDisputes(lit, key, out, now); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// senseAsked сообщает, уходил ли хоть один вопрос слоту: без слота и вне бюджета
// запись в forge_log не пишется, чтобы не заслонить будущие вопросы.
func senseAsked(outcomes []SenseOutcome) bool {
	for _, out := range outcomes {
		if out.Attempts > 0 {
			return true
		}
	}
	return false
}

// RemainingDisputeConfig — бюджет спорных мест после пакета смысловых вопросов
// (шаг 2): смыслы и места делят один deepread.slot_budget.
func RemainingDisputeConfig(cfg DisputeConfig, senseQuestions int) DisputeConfig {
	asked := senseQuestions
	if asked > cfg.Budget {
		asked = cfg.Budget
	}
	if asked < 0 {
		asked = 0
	}
	rest := cfg
	rest.Budget = cfg.Budget - asked
	return rest
}

// resolveSenseQuestions задаёт слоту топ бюджета вопросов батчами: споры сверх
// бюджета остаются uncertain, без слота запросов нет.
func resolveSenseQuestions(questions []SenseQuestion, cfg DisputeConfig,
	slot DisputeSlot, logger *journal.Logger) ([]SenseOutcome, error) {
	if cfg == (DisputeConfig{}) {
		cfg = DefaultDisputeConfig()
	}
	if err := validateDisputeConfig(cfg); err != nil {
		return nil, err
	}

	queue := append([]SenseQuestion(nil), questions...)
	RankSenseDisputes(queue)
	out := make([]SenseOutcome, len(queue))
	for i := range queue {
		out[i] = SenseOutcome{Question: queue[i], State: DisputeUncertain}
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
		copy(out[start:end], askSenseBatch(queue[start:end], start/cfg.Batch+1, slot, logger))
	}
	return out, nil
}

// askSenseBatch задаёт один батч смысловых вопросов: вызов слота, проверка
// ответа, повтор при провале не более disputeRetries раз. Каждый вызов
// помечается событием deepread.slot; исчерпанные повторы помечают батч failed.
func askSenseBatch(batch []SenseQuestion, num int, slot DisputeSlot, logger *journal.Logger) []SenseOutcome {
	prompt := senseDisputePrompt(batch)
	attempts := 0
	var lastErr error
	for {
		attempts++
		if logger != nil {
			_ = logger.Event("deepread", EventDeepreadSlot,
				journal.Field{Key: "batch", Value: strconv.Itoa(num)},
				journal.Field{Key: "questions", Value: strconv.Itoa(len(batch))},
				journal.Field{Key: "kind", Value: string(DisputeSense)})
		}
		raw, err := slot.Generate(context.Background(), DisputeSlotRole, prompt)
		if err == nil {
			out, parseErr := parseSenseAnswers(batch, raw)
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
			journal.Field{Key: "kind", Value: string(DisputeSense)},
			journal.Field{Key: "batch", Value: strconv.Itoa(num)},
			journal.Field{Key: "questions", Value: strconv.Itoa(len(batch))},
			journal.Field{Key: "reason", Value: lastErr.Error()})
	}

	out := make([]SenseOutcome, len(batch))
	for i, q := range batch {
		out[i] = SenseOutcome{Question: q, State: DisputeFailed, Attempts: attempts}
	}
	return out
}

// senseDisputePrompt строит вопрос слота (шаг 2): лемма, часть речи, номер
// токена, цитата чанка и смыслы с триггерами. Ответ — JSON-массив объектов
// {"value", "quote", "confidence"} в том же порядке.
func senseDisputePrompt(batch []SenseQuestion) string {
	var b strings.Builder
	b.WriteString("Ты разбираешь смысл слова-омонима в книге. Ответ — JSON-массив объектов ")
	b.WriteString(`{"value": ..., "quote": ..., "confidence": 0..1}, по одному на вопрос в том же порядке. `)
	b.WriteString("value — только имя смысла из списка вопроса или пустая строка; ")
	b.WriteString("quote — дословная цитата из окна; confidence — 0..1.\n")
	for i, q := range batch {
		fmt.Fprintf(&b, "\nВопрос %d (смысл): лемма %q, часть речи %s, токен %d.\n", i+1, q.Lemma, q.POS, q.Token)
		fmt.Fprintf(&b, "Цитата: %s\nСмыслы: ", q.Quote)
		for j, option := range q.Senses {
			if j > 0 {
				b.WriteString("; ")
			}
			fmt.Fprintf(&b, "%q", option.Sense)
			if option.Category != "" {
				fmt.Fprintf(&b, " → %s", option.Category)
			}
			fmt.Fprintf(&b, " (триггеры: %s)", strings.Join(option.Triggers, ", "))
		}
		fmt.Fprintf(&b, "\nОкно: %s\n", q.Window)
	}
	return b.String()
}

// parseSenseAnswers проверяет ответ батча (шаг 3): число ответов, смысл из
// канона вопроса (пустой допустим), confidence в 0..1, цитата — подстрока окна
// после нормализации. Батч из одного вопроса принимает и одиночный объект.
func parseSenseAnswers(batch []SenseQuestion, raw string) ([]SenseOutcome, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, errors.New("deepread: пустой ответ слота на спорные смыслы")
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

	out := make([]SenseOutcome, len(batch))
	for i, q := range batch {
		answer := answers[i]
		value := strings.TrimSpace(answer.Value)
		if value != "" && !hasOption(q.Options, value) {
			return nil, fmt.Errorf("deepread: вопрос %q: смысл %q вне канона", q.Key, value)
		}
		if math.IsNaN(answer.Confidence) || answer.Confidence < 0 || answer.Confidence > 1 {
			return nil, fmt.Errorf("deepread: вопрос %q: confidence %g вне 0..1", q.Key, answer.Confidence)
		}
		if !quoteInWindow(answer.Quote, q.Window) {
			return nil, fmt.Errorf("deepread: вопрос %q: цитата %q не является подстрокой чанка", q.Key, answer.Quote)
		}

		state := DisputeAnswered
		if value == "" {
			state = DisputeEmpty
		}
		out[i] = SenseOutcome{
			Question:   q,
			State:      state,
			Value:      value,
			Category:   senseCategory(q.Senses, value),
			Quote:      answer.Quote,
			Confidence: answer.Confidence,
		}
	}
	return out, nil
}

// senseCategory — категория выбранного смысла (пусто — смысл вне категорий или
// ответа нет).
func senseCategory(sensesOf []SenseOption, value string) string {
	if value == "" {
		return ""
	}
	for _, option := range sensesOf {
		if option.Sense == value {
			return option.Category
		}
	}
	return ""
}

// SenseDictionaries применяет принятые ответы к словарям прогона (шаг 3):
// лемма-омоним с решённым смыслом даёт вклад только категории этого смысла, а
// смысл вне категорий убирает её из словарей. Отклонённый вопрос, пустой или
// противоречивый ответ словари не меняют — тег остаётся консервативным.
func SenseDictionaries(dicts map[string]*Dictionary, outcomes []SenseOutcome) map[string]*Dictionary {
	type verdict struct {
		category string
		resolved bool
	}
	verdicts := make(map[string]verdict, len(outcomes))
	for _, out := range outcomes {
		lemma := out.Question.Lemma
		if lemma == "" {
			continue
		}
		prev, seen := verdicts[lemma]
		if out.State != DisputeAnswered || out.Value == "" {
			verdicts[lemma] = verdict{}
			continue
		}
		if seen && !prev.resolved {
			// Уже решено консервативно: вопрос без ответа важнее ответа.
			continue
		}
		if seen && prev.category != out.Category {
			// Вхождения омонима разошлись смыслами — словарь не трогаем.
			verdicts[lemma] = verdict{}
			continue
		}
		verdicts[lemma] = verdict{category: out.Category, resolved: true}
	}
	if len(verdicts) == 0 {
		return dicts
	}

	out := make(map[string]*Dictionary, len(dicts))
	for key, dict := range dicts {
		if dict == nil {
			out[key] = dict
			continue
		}
		clone := &Dictionary{
			Key:      dict.Key,
			Lemmas:   make(map[string]int, len(dict.Lemmas)),
			Pos:      dict.Pos,
			Phrases:  dict.Phrases,
			Severity: dict.Severity,
		}
		for lemma, weight := range dict.Lemmas {
			if v, seen := verdicts[lemma]; seen && v.resolved && v.category != key {
				continue
			}
			clone.Lemmas[lemma] = weight
		}
		out[key] = clone
	}
	return out
}

// findSenseDisputes ищет в журнале ковки ответы по спорным смыслам для ключа
// пересчёта (шаг 4): последняя подходящая строка kind='deepread_sense'.
func findSenseDisputes(lit *db.Handle, key Key) ([]SenseOutcome, bool, error) {
	rows, err := lit.Query(
		`SELECT task FROM forge_log WHERE kind = ? AND status = ? ORDER BY id DESC`,
		KindSenseDispute, statusDone)
	if err != nil {
		return nil, false, fmt.Errorf("deepread: журнал спорных смыслов: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var task string
		if err := rows.Scan(&task); err != nil {
			return nil, false, fmt.Errorf("deepread: журнал спорных смыслов: %w", err)
		}
		var rec SenseDisputeRecord
		if err := json.Unmarshal([]byte(task), &rec); err != nil {
			continue
		}
		if rec.Key.Equal(key) {
			return rec.Results, true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("deepread: журнал спорных смыслов: %w", err)
	}
	return nil, false, nil
}

// writeSenseDisputes пишет строку forge_log kind='deepread_sense' с итогом
// смысловых вопросов прогона (шаг 3).
func writeSenseDisputes(lit *db.Handle, key Key, outcomes []SenseOutcome, now string) error {
	task, err := json.Marshal(SenseDisputeRecord{Key: key, Results: outcomes})
	if err != nil {
		return fmt.Errorf("deepread: запись спорных смыслов: %w", err)
	}
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	return lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		_, err := tx.Exec("forge_log",
			`INSERT INTO forge_log (kind, task, status, started_at, finished_at, log_path, package_version) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			KindSenseDispute, string(task), statusDone, now, now, "", version.Version)
		return err
	})
}
