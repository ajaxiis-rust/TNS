package cast

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"tns65/internal/lemmarpc"
)

// DescribeAxis — ось словаря описаний (P12 §5.9): значения и леммы-маркеры.
type DescribeAxis struct {
	// Verbs — true: ось привычек, принимает глагольные маркеры; иначе маркеры
	// оси — прилагательные и существительные.
	Verbs bool `json:"verbs,omitempty"`
	// Values — каноническое значение оси → леммы-маркеры.
	Values map[string][]string `json:"values"`
}

// DescribeDict — словарь описаний из packs/<lang>/descriptions.json: оси,
// значения и леммы. Данные, не код — расширяется файлом без правки извлечения.
type DescribeDict struct {
	Version int                     `json:"version"`
	Axes    map[string]DescribeAxis `json:"axes"`

	markers map[string][]marker // лемма → маркеры (строится при загрузке).
}

// marker — один маркер словаря: ось и значение, к которому ведёт лемма.
type marker struct {
	axis  string
	value string
	verb  bool
}

// TraitEvidence — основание выбора значения: сырая форма маркера и цитата.
type TraitEvidence struct {
	Axis  string
	Value string
	Word  string
	Quote string
}

// Described — собранное по осям описание героя (P12 §5.9, шаг 4).
type Described struct {
	// Name — имя героя (как в кандидате).
	Name string
	// Traits — ось → каноническое значение, набравшее порог.
	Traits map[string]string
	// Evidence — основания выбора значений.
	Evidence []TraitEvidence
}

// DescribeOptions — параметры извлечения описаний.
type DescribeOptions struct {
	// Lemmatizer — клиент лемматизации; nil — деградация по формам без лемм.
	Lemmatizer Lemmatizer
	// SentenceWindow — окно предложений вокруг упоминания (±N); 0 — дефолт.
	SentenceWindow int
	// NegWindow — окно токенов отрицания «не» перед маркером; 0 — дефолт.
	NegWindow int
}

const (
	defaultSentenceWindow = 2
	defaultNegWindow      = 2
)

// LoadDescriptions читает словарь описаний из файла и строит индекс лемм.
func LoadDescriptions(path string) (*DescribeDict, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d DescribeDict
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("cast: словарь описаний %s: %w", path, err)
	}
	if len(d.Axes) == 0 {
		return nil, fmt.Errorf("cast: словарь описаний %s: нет осей", path)
	}
	d.buildMarkers()
	return &d, nil
}

// buildMarkers строит индекс «лемма → маркеры»; леммы в нижнем регистре, пустые
// записи пропускаются.
func (d *DescribeDict) buildMarkers() {
	d.markers = map[string][]marker{}
	for axis, a := range d.Axes {
		for value, lemmas := range a.Values {
			for _, l := range lemmas {
				key := strings.ToLower(strings.TrimSpace(l))
				if key == "" {
					continue
				}
				d.markers[key] = append(d.markers[key], marker{axis: axis, value: value, verb: a.Verbs})
			}
		}
	}
}

// descSentence — одно предложение куска: текст и токены лемматизатора.
type descSentence struct {
	text   string
	tokens []lemmarpc.Token
}

// Describe собирает описания героев словарём, без слота (P12 §5.9, шаг 4).
// Вокруг каждого упоминания берётся окно ±SentenceWindow предложений, леммы
// сверяются со словарём; значение оси засчитывается при двух и более попаданиях,
// отрицание «не» в окне NegWindow токенов гасит маркер. Результат детерминирован:
// повторный прогон по тем же кускам даёт то же описание.
func Describe(chunks []Chunk, cands []Candidate, dict *DescribeDict, opts DescribeOptions) ([]Described, error) {
	if dict == nil {
		return nil, fmt.Errorf("cast: словарь описаний не задан")
	}
	if dict.markers == nil {
		dict.buildMarkers()
	}
	if opts.SentenceWindow <= 0 {
		opts.SentenceWindow = defaultSentenceWindow
	}
	if opts.NegWindow <= 0 {
		opts.NegWindow = defaultNegWindow
	}

	var sents []descSentence
	for _, ch := range chunks {
		for _, s := range splitSentences(ch.Text) {
			toks, err := scanTokens(opts.Lemmatizer, s)
			if err != nil {
				return nil, err
			}
			sents = append(sents, descSentence{text: s, tokens: toks})
		}
	}

	out := make([]Described, 0, len(cands))
	for _, c := range cands {
		out = append(out, describeOne(sents, c, dict, opts))
	}
	return out, nil
}

// describeOne собирает описание одного героя по окнам его упоминаний.
func describeOne(sents []descSentence, c Candidate, dict *DescribeDict, opts DescribeOptions) Described {
	lemma := strings.ToLower(strings.TrimSpace(c.Lemma))
	if lemma == "" {
		lemma = strings.ToLower(strings.TrimSpace(c.Name))
	}

	inWindow := make([]bool, len(sents))
	anyMention := false
	for i, s := range sents {
		if mentions(s.tokens, lemma) {
			anyMention = true
			lo := i - opts.SentenceWindow
			if lo < 0 {
				lo = 0
			}
			hi := i + opts.SentenceWindow
			if hi >= len(sents) {
				hi = len(sents) - 1
			}
			for j := lo; j <= hi; j++ {
				inWindow[j] = true
			}
		}
	}

	counts := map[string]map[string]int{}
	var evidence []TraitEvidence
	if anyMention {
		for i, s := range sents {
			if !inWindow[i] {
				continue
			}
			for j, tok := range s.tokens {
				for _, m := range dict.markers[tokenLemma(tok)] {
					if !markerPOS(tok.Pos, m.verb) {
						continue
					}
					if negatedBefore(s.tokens, j, opts.NegWindow) {
						continue
					}
					if counts[m.axis] == nil {
						counts[m.axis] = map[string]int{}
					}
					counts[m.axis][m.value]++
					evidence = append(evidence, TraitEvidence{
						Axis:  m.axis,
						Value: m.value,
						Word:  cleanForm(tok.Form),
						Quote: strings.TrimSpace(s.text),
					})
				}
			}
		}
	}

	return Described{
		Name:     c.Name,
		Traits:   selectTraits(counts),
		Evidence: sortEvidence(evidence),
	}
}

// mentions сообщает, есть ли в токенах предложения лемма героя.
func mentions(tokens []lemmarpc.Token, lemma string) bool {
	for _, tok := range tokens {
		if tokenLemma(tok) == lemma {
			return true
		}
	}
	return false
}

// tokenLemma возвращает лемму токена в нижнем регистре; пустая лемма падает на
// форму.
func tokenLemma(tok lemmarpc.Token) string {
	l := strings.ToLower(cleanForm(tok.Lemma))
	if l == "" {
		l = strings.ToLower(cleanForm(tok.Form))
	}
	return l
}

// markerPOS — фильтр части речи маркера (P12 §5.9): прилагательное или
// существительное; глаголы — только для оси привычек.
func markerPOS(pos string, verb bool) bool {
	if verb {
		return pos == "VERB" || pos == "INFN"
	}
	return pos == "ADJF" || pos == "ADJS" || pos == "PRTF" || pos == "NOUN"
}

// negatedBefore сообщает, есть ли «не» в окне window токенов перед маркером.
func negatedBefore(tokens []lemmarpc.Token, i, window int) bool {
	for j := i - 1; j >= 0 && j >= i-window; j-- {
		if tokenLemma(tokens[j]) == "не" {
			return true
		}
	}
	return false
}

// selectTraits отбирает по одному значению на ось: порог два попадания, при
// равенстве побеждает первое по алфавиту значение — для детерминизма.
func selectTraits(counts map[string]map[string]int) map[string]string {
	traits := map[string]string{}
	axes := make([]string, 0, len(counts))
	for a := range counts {
		axes = append(axes, a)
	}
	sort.Strings(axes)
	for _, a := range axes {
		best := ""
		bestN := 0
		values := make([]string, 0, len(counts[a]))
		for v := range counts[a] {
			values = append(values, v)
		}
		sort.Strings(values)
		for _, v := range values {
			n := counts[a][v]
			if n >= 2 && n > bestN {
				best = v
				bestN = n
			}
		}
		if best != "" {
			traits[a] = best
		}
	}
	return traits
}

// sortEvidence упорядочивает основания для детерминированного результата.
func sortEvidence(ev []TraitEvidence) []TraitEvidence {
	sort.Slice(ev, func(i, j int) bool {
		if ev[i].Axis != ev[j].Axis {
			return ev[i].Axis < ev[j].Axis
		}
		if ev[i].Value != ev[j].Value {
			return ev[i].Value < ev[j].Value
		}
		if ev[i].Word != ev[j].Word {
			return ev[i].Word < ev[j].Word
		}
		return ev[i].Quote < ev[j].Quote
	})
	return ev
}

// splitSentences режет текст на предложения по `. ! ? … ; :` (резка T-03-02) и
// отбрасывает пустые куски.
func splitSentences(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case '.', '!', '?', '…', ';', ':':
			return true
		}
		return false
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
