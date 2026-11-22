package turn

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"unicode"
)

// Пределы словаря при пустых meta: порог владельца 1.0, окно контраста в 6
// токенов и «короткая реплика» до 10 слов.
const (
	defaultSarcasmThreshold      = 1.0
	defaultSarcasmContrastWindow = 6
	defaultSarcasmShortMax       = 10
)

// SarcasmDictionary — словарь сарказма языкового пакета (T-18-02, D-11):
// фразовые маркеры по категориям, коэффициенты позиций и кавычек, контраст
// гиперболы с бытовым негативом. Одиночные слова дают ложные срабатывания,
// поэтому вес набирают n-граммы 2–4 слова, позиция и контраст в окне.
type SarcasmDictionary struct {
	Meta         SarcasmMeta         `json:"meta"`
	Coefficients SarcasmCoefficients `json:"coefficients"`
	Patterns     SarcasmPatterns     `json:"patterns"`

	compiled *sarcasmCompiled
}

// SarcasmMeta — порог срабатывания и окна детектора в токенах.
type SarcasmMeta struct {
	Language              string  `json:"language"`
	SarcasmThreshold      float64 `json:"sarcasm_threshold"`
	ContrastWindowTokens  int     `json:"contrast_window_tokens"`
	ShortMessageMaxTokens int     `json:"short_message_max_tokens"`
}

// SarcasmCoefficients — множители детектора: позиции, кавычки, указательный
// префикс, многоточие, скобки и контраст-пара.
type SarcasmCoefficients struct {
	PositionStartFirst3           float64 `json:"position_start_first_3_tokens"`
	PositionEndLast3              float64 `json:"position_end_last_3_tokens"`
	ShortMessageMultiplier        float64 `json:"short_message_multiplier"`
	InQuotesMultiplier            float64 `json:"in_quotes_multiplier"`
	DemonstrativePrefixMultiplier float64 `json:"demonstrative_prefix_multiplier"`
	TrailingEllipsisMultiplier    float64 `json:"trailing_ellipsis_multiplier"`
	NegativeWithExcessParensBonus float64 `json:"negative_with_excess_parens_bonus"`
	ContrastPairBaseBonus         float64 `json:"contrast_pair_base_bonus"`
}

// SarcasmPatterns — категории маркеров словаря.
type SarcasmPatterns struct {
	DemonstrativePrefixes []string                `json:"demonstrative_prefixes"`
	ExplicitMarkers       SarcasmPhraseCategory   `json:"explicit_markers"`
	PseudoAgreement       SarcasmPhraseCategory   `json:"pseudo_agreement"`
	FalseSurprise         SarcasmPhraseCategory   `json:"false_surprise"`
	Dismissive            SarcasmPhraseCategory   `json:"dismissive"`
	IronicCliches         SarcasmPhraseCategory   `json:"ironic_cliches"`
	HyperbolicPositive    SarcasmContrastCategory `json:"hyperbolic_positive"`
	TechnicalNegative     SarcasmContrastCategory `json:"technical_everyday_negative"`
	IronicLabels          SarcasmContrastCategory `json:"ironic_labels"`
}

// SarcasmPhraseCategory — категория фраз с весами: позиционные множители
// применяются только к категориям со своим флагом (псевдо-согласие — начало,
// обесценивание — конец, ложное удивление — короткая реплика).
type SarcasmPhraseCategory struct {
	ApplyStartMultiplier bool               `json:"apply_start_multiplier"`
	ApplyEndMultiplier   bool               `json:"apply_end_multiplier"`
	ApplyShortMultiplier bool               `json:"apply_short_message_multiplier"`
	DefaultWeight        float64            `json:"default_weight"`
	Items                map[string]float64 `json:"items"`
}

// SarcasmContrastCategory — список слов контраст-пары или ярлыков: собственный
// вес низкий, вес набирается правилом контраста и кавычками.
type SarcasmContrastCategory struct {
	ContrastRole       string   `json:"contrast_role"`
	StandaloneWeight   float64  `json:"standalone_weight"`
	ContrastBoost      float64  `json:"contrast_boost"`
	ApplyQuoteOrPrefix bool     `json:"apply_quote_or_prefix_multiplier"`
	Items              []string `json:"items"`
}

// SarcasmVerdict — итог детектора: скор, решение по порогу и сработавшие
// маркеры в порядке начисления.
type SarcasmVerdict struct {
	Score     float64
	Sarcastic bool
	Markers   []string
}

// sarcasmCompiled — словарь, разрезанный на слова один раз при первом скоринге.
type sarcasmCompiled struct {
	categories []sarcasmCategory
	positive   []sarcasmPhrase
	negative   []sarcasmPhrase
	labels     []sarcasmPhrase
	prefixes   map[string]bool
}

type sarcasmCategory struct {
	applyStart    bool
	applyEnd      bool
	applyShort    bool
	defaultWeight float64
	phrases       []sarcasmPhrase
}

// sarcasmPhrase — фраза словаря словами в сравнимом виде.
type sarcasmPhrase struct {
	words  []string
	weight float64
}

// loadSarcasmDictionary читает sarcasm.json. Файла может не быть в пакете —
// тогда детектор не взводится (nil) без ошибки.
func loadSarcasmDictionary(path string) (*SarcasmDictionary, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var d SarcasmDictionary
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// DetectSarcasm считает скор сарказма реплики по словарю пакета (T-18-02,
// D-11). Словаря в пакете нет или он пуст — вердикт пуст, ход не прерывается.
func (lex *Lexicon) DetectSarcasm(text string) SarcasmVerdict {
	if lex == nil || lex.Sarcasm == nil {
		return SarcasmVerdict{}
	}
	return lex.Sarcasm.score(text)
}

func (d *SarcasmDictionary) score(text string) SarcasmVerdict {
	var v SarcasmVerdict
	tokens := tokenizeSarcasm(text)
	if len(tokens) == 0 {
		return v
	}
	d.compile()
	c := d.compiled
	coef := d.Coefficients

	shortMax := d.Meta.ShortMessageMaxTokens
	if shortMax <= 0 {
		shortMax = defaultSarcasmShortMax
	}

	// Категории фраз: вес фразы × позиция × кавычки × указательный префикс.
	for _, cat := range c.categories {
		for _, ph := range cat.phrases {
			for _, pos := range matchSarcasmPhrase(tokens, ph.words) {
				w := ph.weight
				if cat.applyStart && pos < 3 {
					w *= coef.PositionStartFirst3
				}
				if cat.applyEnd && pos+len(ph.words) > len(tokens)-3 {
					w *= coef.PositionEndLast3
				}
				if cat.applyShort && len(tokens) <= shortMax {
					w *= coef.ShortMessageMultiplier
				}
				if tokens[pos].quoted {
					w *= coef.InQuotesMultiplier
				}
				if c.demonstrativePrefix(tokens, pos) {
					w *= coef.DemonstrativePrefixMultiplier
				}
				v.Score += w
				v.Markers = append(v.Markers, strings.Join(ph.words, " "))
			}
		}
	}

	// Контраст-пара: гипербола и бытовой негатив в одном окне — самый сильный
	// признак издёвки; окно пары повторно не пересчитывается.
	window := d.Meta.ContrastWindowTokens
	if window <= 0 {
		window = defaultSarcasmContrastWindow
	}
	if window > len(tokens) {
		window = len(tokens)
	}
	for i := 0; i+window <= len(tokens); i++ {
		if !matchSarcasmWindow(tokens, i, i+window, c.positive) {
			continue
		}
		if !matchSarcasmWindow(tokens, i, i+window, c.negative) {
			continue
		}
		v.Score += coef.ContrastPairBaseBonus +
			d.Patterns.HyperbolicPositive.ContrastBoost +
			d.Patterns.TechnicalNegative.ContrastBoost
		v.Markers = append(v.Markers, "contrast_pair")
		i += window - 1
	}

	// Одиночные веса: гипербола считается отдельно — многоточие после похвалы
	// («Гениально...») усиливает диссонанс.
	positive := 0.0
	for _, ph := range c.positive {
		positive += float64(len(matchSarcasmPhrase(tokens, ph.words))) *
			d.Patterns.HyperbolicPositive.StandaloneWeight
	}
	if positive > 0 && hasTrailingEllipsis(text) {
		positive *= coef.TrailingEllipsisMultiplier
	}
	v.Score += positive

	negativeSeen := false
	for _, ph := range c.negative {
		if n := len(matchSarcasmPhrase(tokens, ph.words)); n > 0 {
			negativeSeen = true
			v.Score += float64(n) * d.Patterns.TechnicalNegative.StandaloneWeight
		}
	}
	if negativeSeen && strings.Contains(text, ")))") {
		v.Score += coef.NegativeWithExcessParensBonus
	}

	// Ярлыки («эксперт», «гений») весят мало, но в кавычках и после
	// указательного префикса («наш эксперт») множатся.
	for _, ph := range c.labels {
		for _, pos := range matchSarcasmPhrase(tokens, ph.words) {
			w := d.Patterns.IronicLabels.StandaloneWeight
			if d.Patterns.IronicLabels.ApplyQuoteOrPrefix {
				switch {
				case tokens[pos].quoted:
					w *= coef.InQuotesMultiplier
				case c.demonstrativePrefix(tokens, pos):
					w *= coef.DemonstrativePrefixMultiplier
				}
			}
			v.Score += w
			v.Markers = append(v.Markers, strings.Join(ph.words, " "))
		}
	}

	// Эксплицитные маркеры сети («/s», «(нет)») — почти точное указание.
	lower := strings.ToLower(text)
	for _, marker := range sortedSarcasmKeys(d.Patterns.ExplicitMarkers.Items) {
		if !strings.Contains(lower, strings.ToLower(marker)) {
			continue
		}
		w := d.Patterns.ExplicitMarkers.Items[marker]
		if w == 0 {
			w = d.Patterns.ExplicitMarkers.DefaultWeight
		}
		v.Score += w
		v.Markers = append(v.Markers, marker)
	}

	threshold := d.Meta.SarcasmThreshold
	if threshold <= 0 {
		threshold = defaultSarcasmThreshold
	}
	v.Sarcastic = v.Score >= threshold
	return v
}

// compile разрезает фразы словаря на слова один раз: по карте JSON порядок не
// задан, а сумма весов должна быть детерминированной.
func (d *SarcasmDictionary) compile() {
	if d.compiled != nil {
		return
	}
	c := &sarcasmCompiled{prefixes: map[string]bool{}}
	for _, src := range []SarcasmPhraseCategory{
		d.Patterns.PseudoAgreement,
		d.Patterns.FalseSurprise,
		d.Patterns.Dismissive,
		d.Patterns.IronicCliches,
	} {
		cat := sarcasmCategory{
			applyStart:    src.ApplyStartMultiplier,
			applyEnd:      src.ApplyEndMultiplier,
			applyShort:    src.ApplyShortMultiplier,
			defaultWeight: src.DefaultWeight,
		}
		for _, key := range sortedSarcasmKeys(src.Items) {
			words := sarcasmWords(key)
			if len(words) == 0 {
				continue
			}
			w := src.Items[key]
			if w == 0 {
				w = src.DefaultWeight
			}
			cat.phrases = append(cat.phrases, sarcasmPhrase{words: words, weight: w})
		}
		c.categories = append(c.categories, cat)
	}
	c.positive = sarcasmPhrases(d.Patterns.HyperbolicPositive.Items)
	c.negative = sarcasmPhrases(d.Patterns.TechnicalNegative.Items)
	c.labels = sarcasmPhrases(d.Patterns.IronicLabels.Items)
	for _, prefix := range d.Patterns.DemonstrativePrefixes {
		if words := sarcasmWords(prefix); len(words) > 0 {
			c.prefixes[words[0]] = true
		}
	}
	d.compiled = c
}

// demonstrativePrefix — перед вхождением стоит указательный префикс («наш
// эксперт», «очередной гений»).
func (c *sarcasmCompiled) demonstrativePrefix(tokens []sarcasmToken, pos int) bool {
	return pos > 0 && c.prefixes[tokens[pos-1].word]
}

func sortedSarcasmKeys(items map[string]float64) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sarcasmPhrases(items []string) []sarcasmPhrase {
	out := make([]sarcasmPhrase, 0, len(items))
	for _, item := range items {
		if words := sarcasmWords(item); len(words) > 0 {
			out = append(out, sarcasmPhrase{words: words})
		}
	}
	return out
}

// sarcasmWords режет фразу словаря тем же резаком, что и реплику: запись
// словаря и текст сравниваются в одной поверхностной форме.
func sarcasmWords(phrase string) []string {
	tokens := tokenizeSarcasm(phrase)
	words := make([]string, len(tokens))
	for i, t := range tokens {
		words[i] = t.word
	}
	return words
}

// sarcasmToken — слово реплики в сравнимом виде; quoted — слово внутри кавычек
// «…» или "…" (кавычки вокруг обычного слова — признак издёвки).
type sarcasmToken struct {
	word   string
	quoted bool
}

// tokenizeSarcasm режет реплику на слова в нижнем регистре: буквы и цифры,
// дефис внутри слова; кавычки помечают вложенные слова цитатой.
func tokenizeSarcasm(text string) []sarcasmToken {
	var out []sarcasmToken
	var cur []rune
	quoted := false
	flush := func() {
		for len(cur) > 0 && cur[len(cur)-1] == '-' {
			cur = cur[:len(cur)-1]
		}
		if len(cur) > 0 {
			out = append(out, sarcasmToken{word: sarcasmNormalize(string(cur)), quoted: quoted})
		}
		cur = cur[:0]
	}
	for _, r := range text {
		switch {
		case r == '«' || r == '»' || r == '"' || r == '„' || r == '“' || r == '”':
			flush()
			quoted = !quoted
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			cur = append(cur, r)
		case r == '-':
			if len(cur) > 0 {
				cur = append(cur, r)
			}
		default:
			flush()
		}
	}
	flush()
	return out
}

// sarcasmNormalize приводит слово к сравнимому виду: нижний регистр, «ё»→«е».
func sarcasmNormalize(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "ё", "е")
}

// matchSarcasmPhrase возвращает позиции вхождений фразы в токены.
func matchSarcasmPhrase(tokens []sarcasmToken, words []string) []int {
	if len(words) == 0 {
		return nil
	}
	var out []int
	for i := 0; i+len(words) <= len(tokens); i++ {
		if sarcasmPhraseAt(tokens, i, words) {
			out = append(out, i)
		}
	}
	return out
}

// matchSarcasmWindow — в окне [from, to) есть хотя бы одна фраза списка.
func matchSarcasmWindow(tokens []sarcasmToken, from, to int, phrases []sarcasmPhrase) bool {
	for _, ph := range phrases {
		for i := from; i+len(ph.words) <= to; i++ {
			if sarcasmPhraseAt(tokens, i, ph.words) {
				return true
			}
		}
	}
	return false
}

func sarcasmPhraseAt(tokens []sarcasmToken, pos int, words []string) bool {
	if pos+len(words) > len(tokens) {
		return false
	}
	for j, w := range words {
		if tokens[pos+j].word != w {
			return false
		}
	}
	return true
}
