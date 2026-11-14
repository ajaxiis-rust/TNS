package deepread_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"tns65/internal/deepread"
)

// Пределы гейта золотых отрывков (W-23.4, W-23.5).
const (
	// goldenDefaultMinPrecision — дефолт deepread.golden_min_precision, если
	// ключа нет в config.yaml.
	goldenDefaultMinPrecision = 0.7
	// goldenMismatchLimit — предел строк разбора несовпадений на отрывок.
	goldenMismatchLimit = 10
)

// goldenMarkup — разметка отрывка: флаг утверждения владельцем и чанки с
// категориями (W-23.4).
type goldenMarkup struct {
	Approved bool          `json:"approved"`
	Chunks   []goldenChunk `json:"chunks"`
}

// goldenChunk — чанк разметки: границы абзацев, число слов и категории.
type goldenChunk struct {
	ID         int                       `json:"id"`
	ParaFrom   int                       `json:"para_from"`
	ParaTo     int                       `json:"para_to"`
	Words      int                       `json:"words"`
	Categories map[string]goldenCategory `json:"categories"`
}

// goldenCategory — разметка категории чанка.
type goldenCategory struct {
	Present bool `json:"present"`
}

// categoryScore — доли полос и попадания одной категории отрывка.
type categoryScore struct {
	Key       string
	Present   int
	Confident int
	Uncertain int
	Reject    int
	Matched   int
}

// goldenScore — precision отрывка: pooled-счёт по confident-тегам, разбивка по
// категориям, пропущенные ключи и несовпадения.
type goldenScore struct {
	Chunks     int
	Confident  int
	Matched    int
	Categories []categoryScore
	Skipped    []string
	Mismatches []string
}

// precision — доля confident-тегов, совпавших с present=true разметки.
func (s goldenScore) precision() float64 {
	if s.Confident == 0 {
		return 0
	}
	return float64(s.Matched) / float64(s.Confident)
}

// TestGoldenPrecision — гейт калибровки по золотым отрывкам (W-23.4, W-23.5,
// приёмка T-14-06): обход fine/golden/ru*/ с разметкой, нарезка excerpt.txt по
// границам para_from/para_to, прогон salience-слоя и precision категорий
// против deepread.golden_min_precision по каждому утверждённому отрывку.
// Отрывок без "approved": true пропускается с причиной.
func TestGoldenPrecision(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Skip("корень движка не найден — гейт золотых отрывков недоступен")
	}

	minPrecision := loadGoldenMinPrecision(t, filepath.Join(root, "config.yaml"))
	salienceCfg, err := deepread.LoadSalienceConfig(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatalf("LoadSalienceConfig(engine/config.yaml): %v", err)
	}
	dicts, err := deepread.LoadDictionaries(filepath.Join(root, "packs", "ru"))
	if err != nil {
		t.Fatalf("LoadDictionaries: %v", err)
	}
	packDir := filepath.Join(root, "packs", "ru")

	markupRoot := filepath.Join(root, "..", "fine", "golden")
	entries, err := os.ReadDir(markupRoot)
	if err != nil {
		t.Fatalf("каталог золотых отрывков %s: %v", markupRoot, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "ru") {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		t.Fatalf("в %s нет отрывков ru*", markupRoot)
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(markupRoot, name)
			markup := loadGoldenMarkup(t, filepath.Join(dir, "golden.json"))
			if !markup.Approved {
				t.Skipf("отрывок %s не утверждён владельцем", name)
			}

			raw, err := os.ReadFile(filepath.Join(dir, "excerpt.txt"))
			if err != nil {
				t.Fatalf("отрывок %s: excerpt.txt: %v", name, err)
			}
			chunks, err := goldenChunkTexts(excerptParagraphs(string(raw)), markup)
			if err != nil {
				t.Fatalf("отрывок %s: нарезка: %v", name, err)
			}

			if residentBinary(t, packDir) == "" {
				t.Skipf("отрывок %s: бинарник резидента лемматизатора недоступен", name)
			}
			run, err := deepread.OpenRun(packDir)
			if err != nil {
				t.Fatalf("отрывок %s: OpenRun: %v", name, err)
			}
			lemmas := run.ChunkLemmas(chunks)
			if err := run.Err(); err != nil {
				_ = run.Close()
				t.Fatalf("отрывок %s: ChunkLemmas: %v", name, err)
			}
			if err := run.Close(); err != nil {
				t.Fatalf("отрывок %s: Close: %v", name, err)
			}

			salienceChunks := make([]deepread.SalienceChunk, len(chunks))
			for i := range chunks {
				salienceChunks[i] = deepread.SalienceChunk{ID: int64(i + 1), Text: chunks[i], Lemmas: lemmas[i]}
			}
			tags, err := deepread.TagChunks(salienceChunks, dicts, salienceCfg)
			if err != nil {
				t.Fatalf("отрывок %s: TagChunks: %v", name, err)
			}
			score, err := scoreGolden(tags, markup)
			if err != nil {
				t.Fatalf("отрывок %s: счёт precision: %v", name, err)
			}
			if score.Confident == 0 {
				t.Fatalf("отрывок %s: нет confident-тегов по ключам разметки — precision не определена", name)
			}

			lines := score.report(name, minPrecision)
			if score.precision() < minPrecision {
				for _, line := range lines {
					t.Error(line)
				}
				return
			}
			for _, line := range lines {
				t.Log(line)
			}
		})
	}

	t.Run("нарезка по границам", func(t *testing.T) {
		// Заголовки глав и разделители пропусков абзацами не считаются.
		text := "### ГЛАВА I\n\nПервый абзац.\n\nВторой абзац.\n\n=== ПРОПУСК ===\n\n### ГЛАВА II\n\nТретий абзац."
		paras := excerptParagraphs(text)
		if len(paras) != 3 {
			t.Fatalf("абзацев %d, ожидалось 3: %q", len(paras), paras)
		}
		markup := &goldenMarkup{Chunks: []goldenChunk{
			{ID: 0, ParaFrom: 0, ParaTo: 1, Words: 4},
			{ID: 1, ParaFrom: 2, ParaTo: 2, Words: 2},
		}}
		chunks, err := goldenChunkTexts(paras, markup)
		if err != nil {
			t.Fatalf("goldenChunkTexts: %v", err)
		}
		if chunks[0] != "Первый абзац.\n\nВторой абзац." {
			t.Errorf("чанк 0 = %q", chunks[0])
		}
		if chunks[1] != "Третий абзац." {
			t.Errorf("чанк 1 = %q", chunks[1])
		}

		// Расхождение числа слов разметки и нарезки — ошибка, а не тихий пропуск.
		bad := &goldenMarkup{Chunks: []goldenChunk{{ID: 0, ParaFrom: 0, ParaTo: 0, Words: 3}}}
		if _, err := goldenChunkTexts(paras, bad); err == nil {
			t.Error("goldenChunkTexts со словами разметки 3: ошибки нет")
		}
		// Границы за пределами отрывка — ошибка.
		out := &goldenMarkup{Chunks: []goldenChunk{{ID: 0, ParaFrom: 2, ParaTo: 3}}}
		if _, err := goldenChunkTexts(paras, out); err == nil {
			t.Error("goldenChunkTexts с para_to за пределами: ошибки нет")
		}
	})

	t.Run("счёт precision", func(t *testing.T) {
		markup := &goldenMarkup{Chunks: []goldenChunk{
			{ID: 0, Categories: map[string]goldenCategory{
				"nature": {Present: true}, "battle": {Present: false}, "erotic": {Present: false},
				"speech": {Present: true},
			}},
			{ID: 1, Categories: map[string]goldenCategory{
				"nature": {Present: false}, "battle": {Present: true}, "erotic": {Present: false},
				"speech": {Present: true},
			}},
			{ID: 2, Categories: map[string]goldenCategory{
				"nature": {Present: true}, "battle": {Present: false}, "erotic": {Present: false},
				"speech": {Present: true},
			}},
		}}
		tags := []deepread.ChunkTags{
			{ChunkID: 1, Tags: []deepread.ChunkTag{
				{Key: "nature", Band: deepread.BandConfident},
				{Key: "battle", Band: deepread.BandReject},
				{Key: "erotic", Band: deepread.BandConfident},
				{Key: "social", Band: deepread.BandReject},
			}},
			{ChunkID: 2, Tags: []deepread.ChunkTag{
				{Key: "nature", Band: deepread.BandConfident},
				{Key: "battle", Band: deepread.BandConfident},
				{Key: "erotic", Band: deepread.BandUncertain},
				{Key: "social", Band: deepread.BandReject},
			}},
			{ChunkID: 3, Tags: []deepread.ChunkTag{
				{Key: "nature", Band: deepread.BandConfident},
				{Key: "battle", Band: deepread.BandConfident},
				{Key: "erotic", Band: deepread.BandReject},
				{Key: "social", Band: deepread.BandReject},
			}},
		}
		score, err := scoreGolden(tags, markup)
		if err != nil {
			t.Fatalf("scoreGolden: %v", err)
		}
		// nature: 3 confident, 2 попадания; battle: 2 confident, 1 попадание.
		if score.Confident != 5 || score.Matched != 3 {
			t.Fatalf("confident %d, matched %d, ожидалось 5/3", score.Confident, score.Matched)
		}
		if got := score.precision(); got != 0.6 {
			t.Errorf("precision = %g, ожидалось 0.6", got)
		}
		byKey := make(map[string]categoryScore, len(score.Categories))
		for _, cat := range score.Categories {
			byKey[cat.Key] = cat
		}
		if cat := byKey["nature"]; cat.Confident != 3 || cat.Matched != 2 || cat.Present != 2 {
			t.Errorf("nature: %+v, ожидалось 3 confident, 2 matched, 2 present", cat)
		}
		if cat := byKey["battle"]; cat.Confident != 2 || cat.Matched != 1 {
			t.Errorf("battle: %+v, ожидалось 2 confident, 1 matched", cat)
		}
		if _, ok := byKey["erotic"]; ok {
			t.Error("erotic: ключ без present=true попал в счёт")
		}
		if _, ok := byKey["speech"]; ok {
			t.Error("speech: ключ без словаря salience попал в счёт")
		}
		skipped := strings.Join(score.Skipped, "; ")
		for _, want := range []string{"erotic (present=false во всех чанках)", "speech (нет словаря salience)"} {
			if !strings.Contains(skipped, want) {
				t.Errorf("пропущенные ключи %q, нет %q", skipped, want)
			}
		}
		if len(score.Mismatches) != 2 {
			t.Errorf("несовпадений %d, ожидалось 2: %q", len(score.Mismatches), score.Mismatches)
		}
	})
}

// loadGoldenMarkup читает разметку отрывка из golden.json.
func loadGoldenMarkup(t *testing.T, path string) *goldenMarkup {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden.json %s: %v", path, err)
	}
	var markup goldenMarkup
	if err := json.Unmarshal(raw, &markup); err != nil {
		t.Fatalf("golden.json %s: %v", path, err)
	}
	if len(markup.Chunks) == 0 {
		t.Fatalf("golden.json %s: нет чанков разметки", path)
	}
	return &markup
}

// loadGoldenMinPrecision читает deepread.golden_min_precision из config.yaml;
// отсутствие ключа — дефолт 0.7, значение вне (0..1] — ошибка.
func loadGoldenMinPrecision(t *testing.T, path string) float64 {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("config.yaml %s: %v", path, err)
	}
	min := goldenDefaultMinPrecision
	inDeepread := false
	for _, rawLine := range strings.Split(string(raw), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		indent := len(rawLine) - len(strings.TrimLeft(rawLine, " \t"))
		if indent == 0 {
			inDeepread = line == "deepread:"
			continue
		}
		if !inDeepread || indent != 2 {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != "golden_min_precision" {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			t.Fatalf("deepread.golden_min_precision %q: %v", value, err)
		}
		min = f
	}
	if min <= 0 || min > 1 {
		t.Fatalf("deepread.golden_min_precision = %g, нужно 0 < p ≤ 1", min)
	}
	return min
}

// excerptParagraphs делит текст отрывка на абзацы по пустой строке; заголовки
// глав «### …» и разделители пропусков «=== …» абзацами не считаются.
func excerptParagraphs(text string) []string {
	var out []string
	for _, block := range strings.Split(text, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, "###") || strings.HasPrefix(block, "===") {
			continue
		}
		out = append(out, block)
	}
	return out
}

// goldenChunkTexts нарезает абзацы по границам разметки: para_from/para_to
// фиксированы разметкой, чанкер их не пересчитывает; words разметки — проверка
// нарезки.
func goldenChunkTexts(paras []string, markup *goldenMarkup) ([]string, error) {
	out := make([]string, len(markup.Chunks))
	for i, chunk := range markup.Chunks {
		if chunk.ParaFrom < 0 || chunk.ParaTo < chunk.ParaFrom || chunk.ParaTo >= len(paras) {
			return nil, fmt.Errorf("чанк %d: границы абзацев %d..%d вне 0..%d",
				chunk.ID, chunk.ParaFrom, chunk.ParaTo, len(paras)-1)
		}
		text := strings.Join(paras[chunk.ParaFrom:chunk.ParaTo+1], "\n\n")
		if chunk.Words > 0 {
			if words := len(strings.Fields(text)); words != chunk.Words {
				return nil, fmt.Errorf("чанк %d: слов %d, в разметке %d", chunk.ID, words, chunk.Words)
			}
		}
		out[i] = text
	}
	return out, nil
}

// tagsOf раскладывает теги чанка по ключам.
func tagsOf(chunk deepread.ChunkTags) map[string]deepread.ChunkTag {
	out := make(map[string]deepread.ChunkTag, len(chunk.Tags))
	for _, tag := range chunk.Tags {
		out[tag.Key] = tag
	}
	return out
}

// scoreGolden считает precision категорий отрывка (W-23.5): confident-тег,
// совпавший с present=true, идёт в числитель и знаменатель, confident-тег с
// present=false — только в знаменатель. Ключ без present=true во всех чанках
// на отрывке не калибруется (W-23.4), ключ без словаря salience не оценивается.
func scoreGolden(tags []deepread.ChunkTags, markup *goldenMarkup) (goldenScore, error) {
	if len(tags) != len(markup.Chunks) {
		return goldenScore{}, fmt.Errorf("тегов %d, чанков разметки %d", len(tags), len(markup.Chunks))
	}

	dictKeys := make(map[string]struct{})
	if len(tags) > 0 {
		for _, tag := range tags[0].Tags {
			dictKeys[tag.Key] = struct{}{}
		}
	}

	keySet := make(map[string]struct{})
	for _, chunk := range markup.Chunks {
		for key := range chunk.Categories {
			keySet[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	score := goldenScore{Chunks: len(tags)}
	for _, key := range keys {
		if _, ok := dictKeys[key]; !ok {
			score.Skipped = append(score.Skipped, key+" (нет словаря salience)")
			continue
		}
		present := 0
		for _, chunk := range markup.Chunks {
			if chunk.Categories[key].Present {
				present++
			}
		}
		if present == 0 {
			score.Skipped = append(score.Skipped, key+" (present=false во всех чанках)")
			continue
		}

		cat := categoryScore{Key: key, Present: present}
		for i := range markup.Chunks {
			tag := tagsOf(tags[i])[key]
			isPresent := markup.Chunks[i].Categories[key].Present
			switch tag.Band {
			case deepread.BandConfident:
				cat.Confident++
				if isPresent {
					cat.Matched++
				} else {
					score.Mismatches = append(score.Mismatches,
						fmt.Sprintf("чанк %d, %s: confident, разметка present=false", markup.Chunks[i].ID, key))
				}
			case deepread.BandUncertain:
				cat.Uncertain++
			case deepread.BandReject:
				cat.Reject++
			default:
				return goldenScore{}, fmt.Errorf("чанк %d, ключ %s: полоса %q вне confident/uncertain/reject",
					markup.Chunks[i].ID, key, tag.Band)
			}
		}
		score.Categories = append(score.Categories, cat)
		score.Confident += cat.Confident
		score.Matched += cat.Matched
	}
	return score, nil
}

// report собирает строки отчёта отрывка: precision и порог, доли полос по
// категориям, пропущенные ключи и до goldenMismatchLimit несовпадений.
func (s goldenScore) report(name string, minPrecision float64) []string {
	lines := []string{fmt.Sprintf("отрывок %s: чанков %d, precision %.3f (confident %d, совпало %d), порог %.2f",
		name, s.Chunks, s.precision(), s.Confident, s.Matched, minPrecision)}
	for _, cat := range s.Categories {
		stat := "нет confident"
		if cat.Confident > 0 {
			stat = fmt.Sprintf("%.3f", float64(cat.Matched)/float64(cat.Confident))
		}
		lines = append(lines, fmt.Sprintf("  %s: confident %d, uncertain %d, reject %d, precision %s",
			cat.Key, cat.Confident, cat.Uncertain, cat.Reject, stat))
	}
	if len(s.Skipped) > 0 {
		lines = append(lines, "  пропущены ключи: "+strings.Join(s.Skipped, ", "))
	}
	limit := goldenMismatchLimit
	if len(s.Mismatches) < limit {
		limit = len(s.Mismatches)
	}
	for _, mismatch := range s.Mismatches[:limit] {
		lines = append(lines, "  "+mismatch)
	}
	return lines
}
