package birth

import (
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

//go:embed data/ancketa.json
var ancketaFS embed.FS

// Fields — ключи анкеты в порядке шага 1 (T-02-01): эталон для world_meta,
// интерфейса и экспорта.
var Fields = []string{
	"slug",
	"title",
	"description",
	"synopsis",
	"prologue",
	"lang",
	"travel_mode",
	"genres",
	"world_rules",
	"social_order",
	"economy_mods",
	"magic",
	"hero_birth",
	"censorship",
}

// Ancketa — анкета рождения: ровно 14 смысловых полей (B-01.2).
type Ancketa struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Synopsis    string   `json:"synopsis"`
	Prologue    string   `json:"prologue"`
	Lang        string   `json:"lang"`
	TravelMode  string   `json:"travel_mode"`
	Genres      []string `json:"genres"`
	WorldRules  []string `json:"world_rules"`
	SocialOrder string   `json:"social_order"`
	EconomyMods []string `json:"economy_mods"`
	Magic       string   `json:"magic"`
	HeroBirth   string   `json:"hero_birth"`
	Censorship  string   `json:"censorship"`
}

// Data — перечни анкеты из data/ancketa.json (шаг 2).
type Data struct {
	Genres       []string `json:"genres"`
	SocialOrders []string `json:"social_orders"`
	TravelModes  []string `json:"travel_modes"`
	EconomyMods  []string `json:"economy_mods"`
	Ratings      []string `json:"ratings"`
}

var (
	dataOnce sync.Once
	dataVal  Data
	dataErr  error
)

// LoadData возвращает перечни анкеты из встроенного data/ancketa.json.
func LoadData() (Data, error) {
	dataOnce.Do(func() {
		b, err := ancketaFS.ReadFile("data/ancketa.json")
		if err != nil {
			dataErr = err
			return
		}
		dataErr = json.Unmarshal(b, &dataVal)
	})
	return dataVal, dataErr
}

// FieldError — причина отказа по одному полю анкеты.
type FieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// ValidationError — перечень полей с причинами отказа (шаг 4).
type ValidationError struct {
	Fields []FieldError `json:"fields"`
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, f.Field+": "+f.Reason)
	}
	return "ancketa: " + strings.Join(parts, "; ")
}

var slugRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// Validate проверяет анкету и возвращает *ValidationError с перечнем полей и
// причин (шаги 3–4). Валидация ничего не пишет — результат в память вызывающего.
func Validate(a *Ancketa) error {
	data, err := LoadData()
	if err != nil {
		return err
	}

	var fields []FieldError

	if a.Slug == "" {
		fields = append(fields, FieldError{Field: "slug", Reason: "пустой слаг"})
	} else if !slugRe.MatchString(a.Slug) {
		fields = append(fields, FieldError{Field: "slug", Reason: "допустимы только [a-z0-9-]"})
	}

	if n := countSentences(a.Synopsis); n == 0 {
		fields = append(fields, FieldError{Field: "synopsis", Reason: "синопсис пуст"})
	} else if n > 3 {
		fields = append(fields, FieldError{Field: "synopsis", Reason: fmt.Sprintf("синопсис должен быть 1–3 предложениями, получено %d", n)})
	}

	for _, g := range a.Genres {
		if !contains(data.Genres, g) {
			fields = append(fields, FieldError{Field: "genres", Reason: fmt.Sprintf("жанр вне канонического списка: %q", g)})
		}
	}

	if a.SocialOrder == "" {
		fields = append(fields, FieldError{Field: "social_order", Reason: "строй не указан (нужен ровно один)"})
	} else if !contains(data.SocialOrders, a.SocialOrder) {
		fields = append(fields, FieldError{Field: "social_order", Reason: fmt.Sprintf("строй вне канонического списка: %q", a.SocialOrder)})
	}

	for i, r := range a.WorldRules {
		if !validWorldRule(r) {
			fields = append(fields, FieldError{Field: "world_rules", Reason: fmt.Sprintf("строка %d не в формате «название: описание»: %q", i+1, r)})
		}
	}

	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

// countSentences считает предложения по завершающим знакам «.!?…», за которыми
// идёт конец строки, пробел или закрывающая кавычка/скобка. Непустой текст без
// завершающего знака считается одним предложением.
func countSentences(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	runes := []rune(s)
	count := 0
	i := 0
	for i < len(runes) {
		c := runes[i]
		if c == '.' || c == '!' || c == '?' || c == '…' {
			j := i
			for j < len(runes) && (runes[j] == '.' || runes[j] == '…') {
				j++
			}
			for j < len(runes) && (runes[j] == '"' || runes[j] == '\'' || runes[j] == '»' || runes[j] == ')' || runes[j] == ']') {
				j++
			}
			if j == len(runes) || runes[j] == ' ' || runes[j] == '\t' || runes[j] == '\n' || runes[j] == '\r' {
				count++
			}
			i = j
			continue
		}
		i++
	}
	if count == 0 {
		return 1
	}
	return count
}

// validWorldRule проверяет строку «название: описание»: непустое название до
// двоеточия и непустое описание после него.
func validWorldRule(s string) bool {
	idx := strings.IndexByte(s, ':')
	if idx <= 0 {
		return false
	}
	return strings.TrimSpace(s[idx+1:]) != ""
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
