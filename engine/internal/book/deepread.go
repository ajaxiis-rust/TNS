package book

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"tns65/internal/db"
	"tns65/internal/journal"
)

// DeepReadRole — роль слота разбора первой реплики (CB-02.2): событие
// deepread.call отличает этот вызов от прочих вызовов слота.
const DeepReadRole = "deepread"

// deepReadSource — источник строк карты игрока и фактов Ткача, пополненных
// разбором первой реплики.
const deepReadSource = "deepread"

// deepReadFactKind — вид события фактов Ткача в events (CB-02.5).
const deepReadFactKind = "deepread_fact"

// DeepReadField — одно поле результата разбора (CB-02.4): значение, цитата из
// реплики игрока и доверие 0..1. Поле ниже порога при разносе игнорируется,
// но в deep_reads хранится.
type DeepReadField struct {
	Value      string  `json:"value"`
	Quote      string  `json:"quote"`
	Confidence float64 `json:"confidence"`
}

// DeepReadResult — JSON-контракт разбора первой реплики (CB-02.4): четыре поля,
// каждое с цитатой и доверием.
type DeepReadResult struct {
	Intent     DeepReadField `json:"intent"`
	Plan       DeepReadField `json:"plan"`
	Foreboding DeepReadField `json:"foreboding"`
	OpenThread DeepReadField `json:"open_thread"`
}

// namedField — поле результата с именем контракта.
type namedField struct {
	Name  string
	Field DeepReadField
}

// fields — четыре поля результата в порядке контракта CB-02.4.
func (r DeepReadResult) fields() []namedField {
	return []namedField{
		{Name: "intent", Field: r.Intent},
		{Name: "plan", Field: r.Plan},
		{Name: "foreboding", Field: r.Foreboding},
		{Name: "open_thread", Field: r.OpenThread},
	}
}

// citations — строки citations_json: имя поля и его цитата из реплики.
func (r DeepReadResult) citations() []DeepReadCitation {
	out := make([]DeepReadCitation, 0, 4)
	for _, f := range r.fields() {
		out = append(out, DeepReadCitation{Field: f.Name, Quote: f.Field.Quote})
	}
	return out
}

// DeepReadCitation — строка citations_json: имя поля и цитата из реплики.
type DeepReadCitation struct {
	Field string `json:"field"`
	Quote string `json:"quote"`
}

// DeepReadConfig — настройки разбора (config.yaml, секция deepread): порог
// доверия, ниже которого поле не идёт в разнос (CB-02.4).
type DeepReadConfig struct {
	ConfidenceThreshold float64
}

// DefaultDeepReadConfig — значения по умолчанию: порог доверия 0.6.
func DefaultDeepReadConfig() DeepReadConfig {
	return DeepReadConfig{ConfidenceThreshold: 0.6}
}

// LoadDeepReadConfig читает секцию deepread из файла конфигурации;
// отсутствующая секция или поле оставляют дефолт.
func LoadDeepReadConfig(path string) (DeepReadConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return DeepReadConfig{}, err
	}
	return parseDeepReadSection(string(b)), nil
}

// parseDeepReadSection разбирает топ-ключ `deepread:` со скалярным полем
// confidence_threshold (например, `confidence_threshold: 0.6`).
func parseDeepReadSection(data string) DeepReadConfig {
	cfg := DefaultDeepReadConfig()

	inDeep := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inDeep = trimmed == "deepread:"
			continue
		}
		if !inDeep {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		if strings.TrimSpace(key) == "confidence_threshold" {
			if v, err := strconv.ParseFloat(value, 64); err == nil && v > 0 {
				cfg.ConfidenceThreshold = v
			}
		}
	}
	return cfg
}

// ErrNoDeepRead — разбор главы не найден.
var ErrNoDeepRead = errors.New("book: разбор главы не найден")

// DeepRead выполняет глубокий разбор реплики главы (CB-02): ровно один дешёвый
// вызов слота на главу — на первую реплику; повторная реплика той же главы
// разбора не порождает и возвращает сохранённый результат. Событие deepread.call
// маркирует каждый фактический вызов разбора.
func DeepRead(h *db.Handle, ch Chapter, replica string, cfg DeepReadConfig, slot Slot, logger *journal.Logger) (DeepReadResult, error) {
	if h == nil {
		return DeepReadResult{}, errors.New("book: нет хэндла мира")
	}
	if ch.ID <= 0 {
		return DeepReadResult{}, errors.New("book: id главы не задан")
	}
	if strings.TrimSpace(replica) == "" {
		return DeepReadResult{}, errors.New("book: реплика для разбора пуста")
	}
	if cfg.ConfidenceThreshold <= 0 {
		cfg.ConfidenceThreshold = DefaultDeepReadConfig().ConfidenceThreshold
	}

	// Разбор выполняется один раз на главу (CB-02.1): уже разобранная глава
	// возвращает сохранённый результат без нового вызова слота.
	if got, err := loadDeepRead(h, ch.ID); err == nil {
		return got, nil
	} else if !errors.Is(err, ErrNoDeepRead) {
		return DeepReadResult{}, err
	}

	if slot == nil {
		return DeepReadResult{}, errors.New("book: слот разбора не подключён")
	}

	if logger != nil {
		_ = logger.Event("book", journal.EventDeepreadCall,
			journal.Field{Key: "chapter_id", Value: strconv.FormatInt(ch.ID, 10)})
	}

	raw, err := slot.Generate(context.Background(), DeepReadRole, deepReadPrompt(ch, replica))
	if err != nil {
		return DeepReadResult{}, err
	}

	var res DeepReadResult
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return DeepReadResult{}, fmt.Errorf("book: выход разбора не JSON: %w", err)
	}
	if err := validateDeepRead(res, replica); err != nil {
		return DeepReadResult{}, err
	}

	if err := writeDeepRead(h, ch, res, raw, cfg); err != nil {
		return DeepReadResult{}, err
	}
	return res, nil
}

// loadDeepRead читает сохранённый результат разбора главы.
func loadDeepRead(h *db.Handle, chapterID int64) (DeepReadResult, error) {
	var intentRaw, planRaw, forebodingRaw, openThreadRaw, raw string
	err := h.QueryRow(`SELECT intent_json, plan_json, foreboding_json, open_thread_json, raw_json FROM deep_reads WHERE chapter_id = ?`, chapterID).
		Scan(&intentRaw, &planRaw, &forebodingRaw, &openThreadRaw, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return DeepReadResult{}, ErrNoDeepRead
	}
	if err != nil {
		return DeepReadResult{}, err
	}
	var res DeepReadResult
	if err := json.Unmarshal([]byte(intentRaw), &res.Intent); err != nil {
		return DeepReadResult{}, err
	}
	if err := json.Unmarshal([]byte(planRaw), &res.Plan); err != nil {
		return DeepReadResult{}, err
	}
	if err := json.Unmarshal([]byte(forebodingRaw), &res.Foreboding); err != nil {
		return DeepReadResult{}, err
	}
	if err := json.Unmarshal([]byte(openThreadRaw), &res.OpenThread); err != nil {
		return DeepReadResult{}, err
	}
	return res, nil
}

// normalizeText — нормализация для сверки цитаты с репликой (CB-02.4): нижний
// регистр и схлопывание пробелов; это единственная проверяемая форма «не пересказ».
func normalizeText(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// validateDeepRead проверяет контракт результата (CB-02.4): каждая непустая
// цитата поля обязана быть подстрокой реплики после нормализации — иначе это
// пересказ, и ответ отвергается.
func validateDeepRead(res DeepReadResult, replica string) error {
	normReplica := normalizeText(replica)
	for _, f := range res.fields() {
		quote := normalizeText(f.Field.Quote)
		if quote == "" {
			continue
		}
		if !strings.Contains(normReplica, quote) {
			return fmt.Errorf("book: цитата поля %q не является подстрокой реплики", f.Name)
		}
	}
	return nil
}

// deepReadFactEvent — payload события фактов Ткача (events.kind='deepread_fact').
type deepReadFactEvent struct {
	Field string `json:"field"`
	Value string `json:"value"`
	Quote string `json:"quote"`
}

// writeDeepRead хранит результат разбора в deep_reads и разносит поля выше порога
// в карту игрока Пифии (player_atlas) и факты Ткача (events). Поля ниже порога в
// разнос не идут, но в deep_reads сохраняются (CB-02.4).
func writeDeepRead(h *db.Handle, ch Chapter, res DeepReadResult, raw string, cfg DeepReadConfig) error {
	intentJSON, err := json.Marshal(res.Intent)
	if err != nil {
		return err
	}
	planJSON, err := json.Marshal(res.Plan)
	if err != nil {
		return err
	}
	forebodingJSON, err := json.Marshal(res.Foreboding)
	if err != nil {
		return err
	}
	openThreadJSON, err := json.Marshal(res.OpenThread)
	if err != nil {
		return err
	}
	citationsJSON, err := json.Marshal(res.citations())
	if err != nil {
		return err
	}

	return h.Tx(db.WriterCore, func(tx *db.Tx) error {
		if _, err := tx.Exec("deep_reads",
			`INSERT INTO deep_reads (chapter_id, intent_json, plan_json, foreboding_json, open_thread_json, citations_json, raw_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			ch.ID, string(intentJSON), string(planJSON), string(forebodingJSON), string(openThreadJSON), string(citationsJSON), raw, nowUTC()); err != nil {
			return err
		}

		for _, f := range res.fields() {
			if f.Field.Confidence < cfg.ConfidenceThreshold {
				continue
			}

			// Карта игрока Пифии: фасет грани — имя поля, значение — в interest.
			if _, err := tx.Exec("player_atlas",
				`INSERT INTO player_atlas (day, theme, mood, interest, source) VALUES (?, ?, '', ?, ?)`,
				ch.Day, f.Name, f.Field.Value, deepReadSource); err != nil {
				return err
			}

			// Факты Ткача: событие мира, которое читает Ткач и Селектор (CB-02.5).
			payload, err := json.Marshal(deepReadFactEvent{Field: f.Name, Value: f.Field.Value, Quote: f.Field.Quote})
			if err != nil {
				return err
			}
			if _, err := tx.Exec("events",
				`INSERT INTO events (day, kind, payload_json, source) VALUES (?, ?, ?, ?)`,
				ch.Day, deepReadFactKind, string(payload), deepReadSource); err != nil {
				return err
			}
		}
		return nil
	})
}

// deepReadPrompt строит промпт разбора: реплика игрока и рамка главы.
func deepReadPrompt(ch Chapter, replica string) string {
	raw, _ := json.Marshal(map[string]any{
		"task":       DeepReadRole,
		"chapter_id": ch.ID,
		"day":        ch.Day,
		"replica":    replica,
		"fields":     []string{"intent", "plan", "foreboding", "open_thread"},
	})
	return string(raw)
}
