package workshop

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	"tns65/internal/db"
	"tns65/internal/sediment"
	"tns65/internal/structural"
)

// StyleRole — роль слота фоновой стилизации под классика (W-21.4).
const StyleRole = "style"

// styleKind — kind стилизованной заготовки в bank_pool.
const styleKind = "drama"

// styleLang — язык стилизованной заготовки.
const styleLang = "ru"

// styleTag — тег стилизованной заготовки в bank_pool.
const styleTag = "style"

// styleOriginPrefix — префикс origin стилизованной заготовки.
const styleOriginPrefix = "style:"

// StyleConfig — параметры селектора образцов стиля из config.yaml (секция style,
// W-21.1): число реплик и суммарный лимит токенов образцов.
type StyleConfig struct {
	SampleCount      int
	SampleTokenLimit int
}

// DefaultStyleConfig возвращает параметры, совпадающие со значениями config.yaml.
func DefaultStyleConfig() StyleConfig {
	return StyleConfig{SampleCount: 8, SampleTokenLimit: 2000}
}

// LoadStyleConfig читает секцию style из файла конфигурации.
func LoadStyleConfig(path string) (StyleConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return StyleConfig{}, err
	}
	return parseStyleSection(string(data))
}

// parseStyleSection разбирает топ-ключ `style:` со скалярными полями
// sample_count и sample_token_limit.
func parseStyleSection(data string) (StyleConfig, error) {
	cfg := DefaultStyleConfig()

	inStyle := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inStyle = trimmed == "style:"
			continue
		}
		if !inStyle {
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
		case "sample_count":
			cfg.SampleCount = n
		case "sample_token_limit":
			cfg.SampleTokenLimit = n
		}
	}
	if err := sc.Err(); err != nil {
		return StyleConfig{}, err
	}
	return cfg, nil
}

// StyleSample — образец стиля игрока из журнала ходов (W-21.1): одна реплика
// игрока и её оценочное число токенов.
type StyleSample struct {
	Text   string
	Tokens int
}

// CollectSamples собирает образцы стиля из журнала ходов (turns, писатель core):
// последние реплики игрока, не более cfg.SampleCount, суммарно не более
// cfg.SampleTokenLimit токенов. Селектор детерминированный: от самой свежей
// реплики к более старым, следующая реплика не влезает по лимиту — выбор
// останавливается. Результат возвращается в порядке ходов (старые раньше).
func CollectSamples(world *db.Handle, cfg StyleConfig) ([]StyleSample, error) {
	if world == nil {
		return nil, errors.New("workshop: нет хэндла мира для сбора образцов стиля")
	}
	cfg = normalizeStyleConfig(cfg)

	rows, err := world.Query(`SELECT player_text FROM turns ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var newest []StyleSample
	total := 0
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		if text = strings.TrimSpace(text); text == "" {
			continue
		}
		if len(newest) >= cfg.SampleCount {
			break
		}
		tokens := countTokens(text)
		if total+tokens > cfg.SampleTokenLimit {
			break
		}
		total += tokens
		newest = append(newest, StyleSample{Text: text, Tokens: tokens})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// В промпт образцы подаются в порядке ходов — от старых к свежим.
	out := make([]StyleSample, len(newest))
	for i, s := range newest {
		out[len(newest)-1-i] = s
	}
	return out, nil
}

// countTokens — детерминированная оценка числа токенов текста: число слов по
// пробелам. Внешнего токенизатора нет, лимит считается одной мерой везде.
func countTokens(text string) int {
	return len(strings.Fields(text))
}

// normalizeStyleConfig заменяет неположительные параметры дефолтами.
func normalizeStyleConfig(cfg StyleConfig) StyleConfig {
	d := DefaultStyleConfig()
	if cfg.SampleCount <= 0 {
		cfg.SampleCount = d.SampleCount
	}
	if cfg.SampleTokenLimit <= 0 {
		cfg.SampleTokenLimit = d.SampleTokenLimit
	}
	return cfg
}

// StyleBlockHeader — заголовок блока примеров стиля игрока в промпте (W-21.1).
const StyleBlockHeader = "Стиль игрока (примеры):"

// StyleBlock собирает блок примеров стиля для промпта (W-21.1): заголовок и по
// реплике на строку. Пустой набор образцов — пустой блок.
func StyleBlock(samples []StyleSample) string {
	if len(samples) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(StyleBlockHeader)
	for _, s := range samples {
		b.WriteString("\n- ")
		b.WriteString(s.Text)
	}
	return b.String()
}

// StylePrompt собирает промпт генерации со стилем примером (W-21.1): базовый
// промпт и блок образцов стиля игрока. Без образцов возвращает базовый промпт.
func StylePrompt(base string, samples []StyleSample) string {
	block := StyleBlock(samples)
	if block == "" {
		return base
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return block
	}
	return base + "\n\n" + block
}

// MoralMarkers — маркеры начала оценочного блока (W-21.4): вывод движка не
// содержит морали или интерпретации главы в конце ответа.
var MoralMarkers = []string{"итак", "вывод", "мораль", "мысль главы"}

// HasMoralBlock сообщает, начинается ли конец ответа с оценочного блока (W-21.4):
// первый значащий токен последнего абзаца совпадает с одним из маркеров.
func HasMoralBlock(reply string) bool {
	tail := strings.ToLower(strings.TrimSpace(lastParagraph(reply)))
	for _, m := range MoralMarkers {
		if startsWithMarker(tail, m) {
			return true
		}
	}
	return false
}

// lastParagraph возвращает последний абзац ответа: текст после последнего
// разрыва абзаца, либо весь ответ, если разрывов нет.
func lastParagraph(s string) string {
	if i := strings.LastIndex(s, "\n\n"); i >= 0 {
		return s[i+2:]
	}
	return s
}

// startsWithMarker сообщает, начинается ли s маркером как отдельным словом:
// после маркера конец строки или небуквенный знак.
func startsWithMarker(s, marker string) bool {
	if !strings.HasPrefix(s, marker) {
		return false
	}
	rest := s[len(marker):]
	if rest == "" {
		return true
	}
	r := rune(rest[0])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// styleForgePrompt строит промпт фоновой стилизации под классика (W-21.4).
func styleForgePrompt(draft string) string {
	raw, _ := json.Marshal(map[string]any{
		"task":  StyleRole,
		"draft": draft,
	})
	return string(raw)
}

// StylizeClassic — фоновая стилизация черновика под классика внешней моделью
// (W-21.4): слот варит стилизованную заготовку, она пишется в bank_pool хэндлом
// forge. В живом ходе не вызывается.
func StylizeClassic(ctx context.Context, lit *db.Handle, draft string) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if draft = strings.TrimSpace(draft); draft == "" {
		return 0, errors.New("workshop: стилизация без черновика")
	}
	slot := slotForProvider(DefaultProvider)
	if slot == nil {
		return 0, errors.New("workshop: слот стилизации не подключён")
	}
	text, err := slot.Generate(ctx, StyleRole, styleForgePrompt(draft))
	if err != nil {
		return 0, err
	}
	if text = strings.TrimSpace(text); text == "" {
		return 0, errors.New("workshop: пустой выход стилизации")
	}
	return writeStyleRecord(lit, text, structural.Composition{})
}

// writeStyleRecord пишет стилизованную заготовку в bank_pool хэндлом forge
// (W-21.4): доверие как у банка-ресурса. Композиция пишется, если извлечена
// (T-10-12); стилизация композицию не извлекает — колонки остаются пустыми.
func writeStyleRecord(lit *db.Handle, text string, comp structural.Composition) (int64, error) {
	var id int64
	err := lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		inversion := 0
		if comp.Inversion {
			inversion = 1
		}
		res, err := tx.Exec("bank_pool",
			`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at, family_primary, pattern_secondary, technique, moral_stance, inversion_flag, usage_count) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			styleKind, text, styleLang, styleTag, sediment.TrustLevelVerified, styleOrigin(text), nowUTC(),
			comp.FamilyPrimary, comp.PatternSecondary, comp.Technique, comp.MoralStance, inversion, 0)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// styleOrigin — origin записи стилизованной заготовки: одна стилизация — одна запись.
func styleOrigin(text string) string {
	sum := sha256.Sum256([]byte(text))
	return styleOriginPrefix + fmt.Sprintf("%x", sum)
}
