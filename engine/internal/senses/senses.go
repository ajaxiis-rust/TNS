// Package senses — смыслы слов-омонимов (T-15-01): словарь смыслов языкового
// пакета и триггерный матчер. Разбор живёт в памяти прогона, в базу ничего не
// пишется.
package senses

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Пороги смысла (шаг 4): лучший вес не ниже MinScore, отрыв от второго места не
// ниже MinMargin — иначе смысл не решён и тег остаётся консервативным.
const (
	MinScore  = 1.0
	MinMargin = 0.5
)

// WindowRadius — окно контекста омонима в токенах (шаг 4): ±12 в пределах
// абзаца.
const WindowRadius = 12

// Границы веса триггера и минимум триггеров у смысла (шаг 2).
const (
	minTriggerWeight = 0.5
	maxTriggerWeight = 1.5
	minTriggers      = 3
)

// floatEps — допуск сравнения дробных весов: 1.2-0.7 в float64 даёт
// 0.4999999999999999.
const floatEps = 1e-9

// Categories — ключи словаря разбора (шаг 1), допустимые в поле category;
// пустая строка — смысл вне категорий.
var Categories = []string{
	"architecture",
	"appearance",
	"nature",
	"social",
	"action_dramatic",
	"erotic",
	"violence",
	"battle",
}

// Token — токен разбора: форма, лемма (нижний регистр), часть речи и позиция.
// Line — номер абзаца с 1, Index — позиция токена в строке с 0.
type Token struct {
	Form  string
	Lemma string
	POS   string
	Line  int
	Index int
}

// Trigger — слово-признак смысла: лемма и вес в счёте смысла.
type Trigger struct {
	Lemma  string  `json:"lemma"`
	Weight float64 `json:"weight"`
}

// Sense — смысл омонима: лемма и часть речи омонима, имя смысла, ключ словаря
// разбора (пусто — смысл в категорию не попадает) и триггеры контекста.
type Sense struct {
	Lemma    string    `json:"lemma"`
	POS      string    `json:"pos"`
	Sense    string    `json:"sense"`
	Category string    `json:"category"`
	Triggers []Trigger `json:"triggers"`
}

// Decision — решение по токену-омониму: Index — позиция токена в срезе,
// переданном в Resolve; Evidence — сработавшие триггеры лучшего смысла. У
// нерешённого смысла Sense и Category пусты.
type Decision struct {
	Index    int
	Sense    string
	Category string
	Score    float64
	Margin   float64
	Evidence []string
	Decided  bool
}

// Dictionary — словарь смыслов пака: смыслы, сгруппированные по ключу
// (лемма, часть речи) омонима.
type Dictionary struct {
	groups map[string][]Sense
}

// sensesFile — схема senses.json (шаг 1).
type sensesFile struct {
	Version int     `json:"version"`
	Senses  []Sense `json:"senses"`
}

// Load читает и валидирует senses.json пака. Ошибка — файл не разобрался, у
// смысла пустое обязательное поле, категория вне канона Categories, вес триггера
// вне 0.5–1.5, триггеров меньше трёх или ключ (lemma, pos, sense) повторяется.
func Load(path string) (*Dictionary, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	var f sensesFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("senses: %s: %w", name, err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("senses: %s: версия схемы %d, ожидалась 1", name, f.Version)
	}
	if len(f.Senses) == 0 {
		return nil, fmt.Errorf("senses: %s: словарь смыслов пуст", name)
	}

	d := &Dictionary{groups: make(map[string][]Sense, len(f.Senses))}
	seen := make(map[string]struct{}, len(f.Senses))
	for i := range f.Senses {
		s := f.Senses[i]
		s.Lemma = normalizeLemma(s.Lemma)
		s.POS = strings.ToUpper(strings.TrimSpace(s.POS))
		s.Sense = strings.TrimSpace(s.Sense)
		s.Category = strings.ToLower(strings.TrimSpace(s.Category))
		if s.Lemma == "" {
			return nil, fmt.Errorf("senses: %s: смысл %d: пустая лемма", name, i)
		}
		if s.POS == "" {
			return nil, fmt.Errorf("senses: %s: %s/%s: пустая часть речи", name, s.Lemma, s.Sense)
		}
		if s.Sense == "" {
			return nil, fmt.Errorf("senses: %s: %s: пустое имя смысла", name, s.Lemma)
		}
		if s.Category != "" && !validCategory(s.Category) {
			return nil, fmt.Errorf("senses: %s: %s/%s: категория %q вне канона",
				name, s.Lemma, s.Sense, s.Category)
		}
		if len(s.Triggers) < minTriggers {
			return nil, fmt.Errorf("senses: %s: %s/%s: триггеров %d, нужно не меньше %d",
				name, s.Lemma, s.Sense, len(s.Triggers), minTriggers)
		}
		triggers := make([]Trigger, 0, len(s.Triggers))
		trigSeen := make(map[string]struct{}, len(s.Triggers))
		for _, tr := range s.Triggers {
			tr.Lemma = normalizeLemma(tr.Lemma)
			if tr.Lemma == "" {
				return nil, fmt.Errorf("senses: %s: %s/%s: триггер без леммы", name, s.Lemma, s.Sense)
			}
			if tr.Weight < minTriggerWeight || tr.Weight > maxTriggerWeight {
				return nil, fmt.Errorf("senses: %s: %s/%s: триггер %q: вес %g вне %g..%g",
					name, s.Lemma, s.Sense, tr.Lemma, tr.Weight, minTriggerWeight, maxTriggerWeight)
			}
			if _, dup := trigSeen[tr.Lemma]; dup {
				return nil, fmt.Errorf("senses: %s: %s/%s: триггер %q повторяется",
					name, s.Lemma, s.Sense, tr.Lemma)
			}
			trigSeen[tr.Lemma] = struct{}{}
			triggers = append(triggers, tr)
		}
		s.Triggers = triggers
		key := groupKey(s.Lemma, s.POS)
		if _, dup := seen[key+"\x00"+s.Sense]; dup {
			return nil, fmt.Errorf("senses: %s: смысл %s/%s/%s повторяется",
				name, s.Lemma, s.POS, s.Sense)
		}
		seen[key+"\x00"+s.Sense] = struct{}{}
		d.groups[key] = append(d.groups[key], s)
	}
	return d, nil
}

// Resolve разбирает токены и выдаёт решение только по омонимам — токенам, у
// которых (лемма, часть речи) имеет два и более смысла. Окно контекста —
// ±WindowRadius токенов в пределах абзаца (Line); триггер считается один раз
// независимо от числа вхождений, сам токен-омоним триггером не бывает.
func (d *Dictionary) Resolve(tokens []Token) []Decision {
	return d.resolve(tokens, "", nil)
}

// resolve — общий путь Resolve и ResolveWithVectors (T-15-03): к триггерному
// счёту смысла прибавляется векторный вклад слоя layer, text — текст чанка для
// сборки окна эмбеддинга.
func (d *Dictionary) resolve(tokens []Token, text string, layer *VectorLayer) []Decision {
	if d == nil || len(tokens) == 0 {
		return nil
	}
	var out []Decision
	for i := range tokens {
		entries := d.groups[groupKey(tokens[i].Lemma, tokens[i].POS)]
		if len(entries) < 2 {
			continue
		}
		var bonus []float64
		if layer != nil {
			bonus = layer.scores(entries, WindowText(text, tokens, i))
		}
		out = append(out, decide(i, entries, window(tokens, i), bonus))
	}
	return out
}

// window — леммы абзаца в окне омонима tokens[at], без самого омонима.
func window(tokens []Token, at int) map[string]struct{} {
	lo := at - WindowRadius
	if lo < 0 {
		lo = 0
	}
	hi := at + WindowRadius
	if hi > len(tokens)-1 {
		hi = len(tokens) - 1
	}
	out := make(map[string]struct{}, hi-lo+1)
	for j := lo; j <= hi; j++ {
		if j == at || tokens[j].Line != tokens[at].Line {
			continue
		}
		out[normalizeLemma(tokens[j].Lemma)] = struct{}{}
	}
	return out
}

// decide считает смыслы по сработавшим триггерам окна и решает лучший; bonus —
// векторный вклад смыслов (T-15-03), прибавляется к триггерному счёту.
func decide(index int, entries []Sense, window map[string]struct{}, bonus []float64) Decision {
	scores := make([]float64, len(entries))
	evidence := make([][]string, len(entries))
	for i, s := range entries {
		for _, tr := range s.Triggers {
			if _, ok := window[tr.Lemma]; !ok {
				continue
			}
			scores[i] += tr.Weight
			evidence[i] = append(evidence[i], tr.Lemma)
		}
		if i < len(bonus) {
			scores[i] += bonus[i]
		}
		sort.Strings(evidence[i])
	}

	best := 0
	for i := 1; i < len(entries); i++ {
		if scores[i] > scores[best] {
			best = i
		}
	}
	second := 0.0
	for i := range entries {
		if i != best && scores[i] > second {
			second = scores[i]
		}
	}

	d := Decision{
		Index:    index,
		Score:    scores[best],
		Margin:   scores[best] - second,
		Evidence: evidence[best],
	}
	if d.Score+floatEps >= MinScore && d.Margin+floatEps >= MinMargin {
		d.Decided = true
		d.Sense = entries[best].Sense
		d.Category = entries[best].Category
	}
	return d
}

// groupKey — ключ группы смыслов: лемма и часть речи омонима.
func groupKey(lemma, pos string) string {
	return normalizeLemma(lemma) + "\x00" + strings.ToUpper(strings.TrimSpace(pos))
}

// normalizeLemma приводит лемму к виду словаря: нижний регистр без пробелов.
func normalizeLemma(lemma string) string {
	return strings.ToLower(strings.TrimSpace(lemma))
}

// validCategory сообщает, что key — ключ словаря разбора из канона.
func validCategory(key string) bool {
	for _, c := range Categories {
		if c == key {
			return true
		}
	}
	return false
}
