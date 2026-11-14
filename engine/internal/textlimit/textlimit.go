// Package textlimit — общая оценка токенов вводимых текстов (IMP-7).
// Формула одна для движка и счётчика в браузере (web/static/tokenlimit.js):
// вес руны 2 для кириллицы (U+0400–U+04FF, включая ё) и 1 для остальных,
// tokens = (units + 3) / 4. Кириллица на той же длине даёт вдвое больше единиц,
// поэтому русский текст упирается в лимит раньше латиницы.
package textlimit

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	// DefaultLimit — потолок по умолчанию для реплики игрока и описания мира.
	DefaultLimit = 2000

	// CyrillicWeight и OtherWeight — вес руны в единицах оценки.
	CyrillicWeight = 2
	OtherWeight    = 1

	// UnitsPerToken — сколько единиц приходится на один токен.
	UnitsPerToken = 4

	// KindPlayerMessage и KindWorldDescription — названия текстов в подсказке.
	KindPlayerMessage    = "Реплика игрока"
	KindWorldDescription = "Описание мира"
)

// Estimate возвращает оценку числа токенов текста с поправкой на язык.
func Estimate(text string) int {
	units := 0
	for _, r := range text {
		units += RuneWeight(r)
	}
	if units == 0 {
		return 0
	}
	return (units + UnitsPerToken - 1) / UnitsPerToken
}

// RuneWeight возвращает вес руны: 2 для кириллицы, 1 для остальных.
func RuneWeight(r rune) int {
	if r >= 0x0400 && r <= 0x04FF {
		return CyrillicWeight
	}
	return OtherWeight
}

// Hint собирает подсказку игроку: что длиннее, насколько и что делать.
func Hint(kind string, tokens, limit int) string {
	return fmt.Sprintf("%s длиннее %d токенов (≈%d): сократите текст", kind, limit, tokens)
}

// TooLongError — текст длиннее лимита. Message хранит готовую подсказку
// сервера, когда ошибка пришла из его ответа (Telegram-канал).
type TooLongError struct {
	Kind    string
	Tokens  int
	Limit   int
	Message string
}

func (e *TooLongError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return Hint(e.Kind, e.Tokens, e.Limit)
}

// Check проверяет текст против лимита; limit ≤ 0 — DefaultLimit.
func Check(kind, text string, limit int) error {
	if limit <= 0 {
		limit = DefaultLimit
	}
	tokens := Estimate(text)
	if tokens <= limit {
		return nil
	}
	return &TooLongError{Kind: kind, Tokens: tokens, Limit: limit}
}

// Limits — потоки вводимых текстов из config.yaml (секция limits).
type Limits struct {
	PlayerMessage    int
	WorldDescription int
}

// DefaultLimits возвращает лимиты, совпадающие со значениями config.yaml.
func DefaultLimits() Limits {
	return Limits{PlayerMessage: DefaultLimit, WorldDescription: DefaultLimit}
}

// LoadConfig читает секцию limits из файла конфигурации.
func LoadConfig(path string) (Limits, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Limits{}, err
	}
	return parseLimits(string(data)), nil
}

// parseLimits разбирает топ-ключ `limits:` с целыми полями
// player_message_tokens и world_description_tokens; нет секции или ключа —
// умолчание, нечисловое значение пропускается.
func parseLimits(data string) Limits {
	cfg := DefaultLimits()

	inLimits := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inLimits = trimmed == "limits:"
			continue
		}
		if !inLimits {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			continue
		}
		switch strings.TrimSpace(key) {
		case "player_message_tokens":
			cfg.PlayerMessage = n
		case "world_description_tokens":
			cfg.WorldDescription = n
		}
	}
	return cfg
}
