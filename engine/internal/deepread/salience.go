package deepread

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"tns65/internal/senses"
)

// Salience чанка (W-23): IDF лемм по книге, скор ключа на чанк, полоса и
// якорные цитаты. Пороги полос — данные конфига (секция deepread).

// Полосы тега чанка (W-23.3); выбор полосы — по порогам thresholds конфига.
const (
	BandConfident = "confident"
	BandUncertain = "uncertain"
	BandReject    = "reject"
)

const (
	// maxAnchors — предел якорных цитат одного тега (W-23.3).
	maxAnchors = 3
	// anchorWindow — окно якоря: столько же предложений слева и справа (W-23.3).
	anchorWindow = 2
)

// SalienceConfig — пороги полос salience из config.yaml (секция deepread,
// thresholds, §3 плана волны 14).
type SalienceConfig struct {
	Confident float64 // score ≥ — полоса «уверенно».
	Reject    float64 // score ≤ — полоса «брак».
	// SensesMargin — порог отрыва лучшего смысла омонима от второго
	// (deepread.senses_margin, T-15-02); 0 — константа T-15-01.
	SensesMargin float64
	// SensesEmbedWeight — вес векторного счёта смысла
	// (deepread.senses_embed_weight, T-20-03); 0 — только триггеры.
	SensesEmbedWeight float64
}

// DefaultSalienceConfig возвращает пороги, совпадающие со значениями config.yaml.
func DefaultSalienceConfig() SalienceConfig {
	return SalienceConfig{
		Confident:         0.6,
		Reject:            0.15,
		SensesMargin:      defaultSensesMargin,
		SensesEmbedWeight: senses.DefaultEmbedWeight,
	}
}

// LoadSalienceConfig читает пороги полос из файла конфигурации.
func LoadSalienceConfig(path string) (SalienceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SalienceConfig{}, err
	}
	return parseSalienceSection(string(data))
}

// parseSalienceSection разбирает вложенный блок deepread.thresholds с полями
// confident и reject; прочие ключи секции не читаются.
func parseSalienceSection(data string) (SalienceConfig, error) {
	cfg := DefaultSalienceConfig()

	inDeepread, inThresholds := false, false
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
			inThresholds = false
			continue
		}
		if !inDeepread {
			continue
		}
		if indent <= 2 {
			inThresholds = trimmed == "thresholds:"
			key, value, ok := strings.Cut(trimmed, ":")
			if ok {
				value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
				switch strings.TrimSpace(key) {
				case "senses_margin":
					f, err := strconv.ParseFloat(value, 64)
					if err != nil {
						return SalienceConfig{}, fmt.Errorf("deepread: senses_margin %q: %v", value, err)
					}
					cfg.SensesMargin = f
				case "senses_embed_weight":
					f, err := strconv.ParseFloat(value, 64)
					if err != nil {
						return SalienceConfig{}, fmt.Errorf("deepread: senses_embed_weight %q: %v", value, err)
					}
					cfg.SensesEmbedWeight = f
				}
			}
			continue
		}
		if !inThresholds {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(key) {
		case "confident":
			cfg.Confident = f
		case "reject":
			cfg.Reject = f
		}
	}
	if err := sc.Err(); err != nil {
		return SalienceConfig{}, err
	}
	if err := validateSalienceConfig(cfg); err != nil {
		return SalienceConfig{}, err
	}
	return cfg, nil
}

// validateSalienceConfig проверяет порядок порогов: 0 ≤ reject < confident ≤ 1,
// senses_margin — NaN или вне 0..1.
func validateSalienceConfig(cfg SalienceConfig) error {
	if math.IsNaN(cfg.Confident) || math.IsNaN(cfg.Reject) {
		return fmt.Errorf("deepread: пороги salience: NaN")
	}
	if math.IsNaN(cfg.SensesMargin) || cfg.SensesMargin < 0 || cfg.SensesMargin > 1 {
		return fmt.Errorf("deepread: senses_margin %g: нужно 0 ≤ senses_margin ≤ 1", cfg.SensesMargin)
	}
	if math.IsNaN(cfg.SensesEmbedWeight) || cfg.SensesEmbedWeight < 0 {
		return fmt.Errorf("deepread: senses_embed_weight %g: нужно senses_embed_weight ≥ 0", cfg.SensesEmbedWeight)
	}
	if cfg.Reject < 0 || cfg.Confident > 1 || cfg.Reject >= cfg.Confident {
		return fmt.Errorf("deepread: пороги salience: confident %g, reject %g: нужно 0 ≤ reject < confident ≤ 1",
			cfg.Confident, cfg.Reject)
	}
	return nil
}

// SalienceChunk — чанк книги для разбора: строка book_chunk, текст чанка и его
// леммы из прогона лемма-слоя (W-23.6).
type SalienceChunk struct {
	ID     int64
	Text   string
	Lemmas []Lemma
}

// ChunkTag — тег категории на чанк (W-23.3): скор ключа, полоса и якорные
// цитаты. Якорь — подстрока текста чанка.
type ChunkTag struct {
	Key     string
	Score   float64
	Band    string
	Anchors []string
}

// ChunkTags — теги одного чанка: по тегу на каждый ключ словарей.
type ChunkTags struct {
	ChunkID int64
	Tags    []ChunkTag
}

// IDF считает idf лемм книги (W-23.1): idf = log(1 + N/(1+df)), N — чанков,
// df — чанков, где лемма встречается хотя бы раз среди разборов токенов (IMP-10).
func IDF(chunks []SalienceChunk) map[string]float64 {
	n := len(chunks)
	df := make(map[string]int)
	for _, chunk := range chunks {
		seen := make(map[string]struct{}, len(chunk.Lemmas))
		for _, lemma := range chunk.Lemmas {
			for _, parse := range lemma.parses() {
				if parse.Lemma == "" {
					continue
				}
				if _, dup := seen[parse.Lemma]; dup {
					continue
				}
				seen[parse.Lemma] = struct{}{}
				df[parse.Lemma]++
			}
		}
	}
	idf := make(map[string]float64, len(df))
	for lemma, d := range df {
		idf[lemma] = math.Log(1 + float64(n)/float64(1+d))
	}
	return idf
}

// TagChunks считает теги всех ключей словарей для каждого чанка книги без
// смыслового слоя: омонимы дают вклад словарю любой своей категорией
// (T-14, T-15-02 шаг 1 — senses.json нет).
func TagChunks(chunks []SalienceChunk, dicts map[string]*Dictionary, cfg SalienceConfig) ([]ChunkTags, error) {
	return TagChunksWithSenses(chunks, dicts, nil, cfg)
}

// TagChunksWithSenses считает теги всех ключей словарей для каждого чанка книги
// (W-23.1–W-23.3): скор ключа чанка — сумма «вес словаря × idf» по леммам
// словаря, найденным в чанке (ключ с частью речи — только разбором той же пары
// «лемма, POS», IMP-10), нормированная по максимуму скора книги (единый максимум
// по всем ключам и чанкам — полосы категорий сравнимы между собой); полоса — по
// порогам cfg; якоря — до трёх подстрок чанка вокруг опорных лемм ключа.
// Смысловой слой layer (T-15-02) снимает вклад омонима, решённого в другую
// категорию; nil-слой — счёт как в T-14.
func TagChunksWithSenses(chunks []SalienceChunk, dicts map[string]*Dictionary, layer *SenseLayer, cfg SalienceConfig) ([]ChunkTags, error) {
	if cfg == (SalienceConfig{}) {
		cfg = DefaultSalienceConfig()
	}
	if err := validateSalienceConfig(cfg); err != nil {
		return nil, err
	}
	if len(dicts) == 0 {
		return nil, fmt.Errorf("deepread: нет словарей категорий для тегов чанков")
	}

	keys := make([]string, 0, len(dicts))
	for key, dict := range dicts {
		if dict == nil {
			return nil, fmt.Errorf("deepread: словарь ключа %q не загружен", key)
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	idf := IDF(chunks)
	type chunkKey struct {
		raw     float64
		support []string            // леммы ключа в чанке по убыванию вклада
		forms   map[string][]string // лемма → формы токенов чанка
	}
	scores := make([][]chunkKey, len(chunks))
	maxRaw := 0.0
	for i, chunk := range chunks {
		present := chunkParses(chunk.Lemmas)
		forms := lemmaForms(chunk.Lemmas)
		marks := senseMarks(chunk, layer)
		scores[i] = make([]chunkKey, len(keys))
		for ki, key := range keys {
			var raw float64
			var support []string
			for lemma, weight := range dicts[key].Lemmas {
				pos := dicts[key].Pos[lemma]
				if !keyMatches(present, lemma, pos) {
					continue
				}
				if !senseAllows(marks, lemma, pos, key) {
					continue
				}
				raw += float64(weight) * idf[lemma]
				support = append(support, lemma)
			}
			sort.Slice(support, func(a, b int) bool {
				wa := float64(dicts[key].Lemmas[support[a]]) * idf[support[a]]
				wb := float64(dicts[key].Lemmas[support[b]]) * idf[support[b]]
				if wa != wb {
					return wa > wb
				}
				return support[a] < support[b]
			})
			scores[i][ki] = chunkKey{raw: raw, support: support, forms: forms}
			if raw > maxRaw {
				maxRaw = raw
			}
		}
	}

	out := make([]ChunkTags, len(chunks))
	for i, chunk := range chunks {
		tags := make([]ChunkTag, 0, len(keys))
		for ki, key := range keys {
			score := 0.0
			if maxRaw > 0 {
				score = scores[i][ki].raw / maxRaw
			}
			tags = append(tags, ChunkTag{
				Key:     key,
				Score:   score,
				Band:    bandFor(score, cfg),
				Anchors: chunkAnchors(chunk.Text, scores[i][ki].support, scores[i][ki].forms),
			})
		}
		out[i] = ChunkTags{ChunkID: chunk.ID, Tags: tags}
	}
	return out, nil
}

// bandFor выбирает полосу по скору и порогам (W-23.3): уверенно при score ≥
// confident, брак при score ≤ reject, иначе неуверенно.
func bandFor(score float64, cfg SalienceConfig) string {
	switch {
	case score >= cfg.Confident:
		return BandConfident
	case score <= cfg.Reject:
		return BandReject
	default:
		return BandUncertain
	}
}

// chunkParses строит карту «лемма → части речи разборов» по токенам чанка:
// одно слово может дать несколько лемм с разными POS (IMP-10).
func chunkParses(lemmas []Lemma) map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{}, len(lemmas))
	for _, lemma := range lemmas {
		for _, parse := range lemma.parses() {
			if parse.Lemma == "" {
				continue
			}
			if out[parse.Lemma] == nil {
				out[parse.Lemma] = make(map[string]struct{}, 2)
			}
			out[parse.Lemma][parse.Pos] = struct{}{}
		}
	}
	return out
}

// keyMatches сообщает, что ключ словаря присутствует среди разборов чанка:
// ключ с частью речи — только парой (лемма, POS), ключ без POS — леммой любого
// разбора (IMP-10).
func keyMatches(present map[string]map[string]struct{}, lemma, pos string) bool {
	poses, ok := present[lemma]
	if !ok {
		return false
	}
	if pos == "" {
		return true
	}
	_, hit := poses[pos]
	return hit
}

// lemmaForms строит карту «лемма → формы токенов» в порядке появления; лемма
// берётся из любого разбора токена (IMP-10), повторы схлопываются.
func lemmaForms(lemmas []Lemma) map[string][]string {
	out := make(map[string][]string)
	seen := make(map[string]map[string]struct{})
	for _, lemma := range lemmas {
		form := strings.ToLower(lemma.Form)
		if form == "" {
			continue
		}
		for _, parse := range lemma.parses() {
			if parse.Lemma == "" {
				continue
			}
			if seen[parse.Lemma] == nil {
				seen[parse.Lemma] = make(map[string]struct{})
			}
			if _, dup := seen[parse.Lemma][form]; dup {
				continue
			}
			seen[parse.Lemma][form] = struct{}{}
			out[parse.Lemma] = append(out[parse.Lemma], form)
		}
	}
	return out
}

// sentence — предложение чанка: полуинтервал [start, end) в рунах текста.
type sentence struct {
	start int
	end   int
}

// chunkAnchors возвращает до трёх якорных цитат чанка (W-23.3): подстроку с
// предложением опорной леммы и окном ±2 предложения. Повторы подстрок
// схлопываются, леммы без формы в тексте пропускаются.
func chunkAnchors(text string, support []string, forms map[string][]string) []string {
	if len(support) == 0 || strings.TrimSpace(text) == "" {
		return nil
	}
	runes := []rune(text)
	spans, words := splitSentences(runes)

	var out []string
	seen := make(map[string]struct{}, maxAnchors)
	for _, lemma := range support {
		if len(out) == maxAnchors {
			break
		}
		at := -1
		for _, form := range forms[lemma] {
			if idx, ok := words[form]; ok {
				at = idx
				break
			}
		}
		if at < 0 {
			continue
		}
		from := at - anchorWindow
		if from < 0 {
			from = 0
		}
		to := at + anchorWindow + 1
		if to > len(spans) {
			to = len(spans)
		}
		anchor := strings.TrimSpace(string(runes[spans[from].start:spans[to-1].end]))
		if anchor == "" {
			continue
		}
		if _, dup := seen[anchor]; dup {
			continue
		}
		seen[anchor] = struct{}{}
		out = append(out, anchor)
	}
	return out
}

// splitSentences делит текст чанка на предложения по терминаторам .!?… и
// переводам строк, а заодно строит указатель «слово → номер предложения»
// (первое вхождение) для поиска опорных лемм.
func splitSentences(runes []rune) ([]sentence, map[string]int) {
	var spans []sentence
	i := 0
	for i < len(runes) {
		for i < len(runes) && unicode.IsSpace(runes[i]) {
			i++
		}
		if i >= len(runes) {
			break
		}
		start := i
		for i < len(runes) && !isSentenceTerminator(runes[i]) && runes[i] != '\n' {
			i++
		}
		if i >= len(runes) {
			spans = append(spans, sentence{start: start, end: len(runes)})
			break
		}
		if runes[i] == '\n' {
			spans = append(spans, sentence{start: start, end: i})
			i++
			continue
		}
		for i < len(runes) && isSentenceTerminator(runes[i]) {
			i++
		}
		for i < len(runes) && isClosingMark(runes[i]) {
			i++
		}
		spans = append(spans, sentence{start: start, end: i})
	}

	words := make(map[string]int)
	for idx, span := range spans {
		for _, word := range wordsOf(runes[span.start:span.end]) {
			if _, ok := words[word]; !ok {
				words[word] = idx
			}
		}
	}
	return spans, words
}

// isSentenceTerminator — конец предложения.
func isSentenceTerminator(r rune) bool {
	return r == '.' || r == '!' || r == '?' || r == '…'
}

// isClosingMark — закрывающий знак, остающийся у предложения.
func isClosingMark(r rune) bool {
	switch r {
	case '"', '»', '“', '”', ')', ']':
		return true
	}
	return false
}

// wordsOf выделяет из отрезка текста слова в нижнем регистре.
func wordsOf(runes []rune) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return out
}
