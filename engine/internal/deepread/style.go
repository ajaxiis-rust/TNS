package deepread

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"tns65/internal/db"
)

// Стилевой профиль книги (W-27, T-14-09): признаки стиля по пунктуации и
// морфологии лемма-слоя, ярлыки канона styles.json, драматичность чанков и
// автор из метаданных парсера. Разбор только измеряет текст, прозу не правит
// (GLO-8).

// Веса драматичности чанка (W-27.3, шаг 3).
const (
	dramaWeightAction  = 0.35
	dramaWeightCombat  = 0.25
	dramaWeightEmotion = 0.20
	dramaWeightPunct   = 0.20

	// shortSentenceWords — верхняя граница короткого предложения в словах:
	// серия считается по предложениям короче пяти слов.
	shortSentenceWords = 5
	// shortSentenceRun — длина серии коротких предложений, дающая сигнал
	// краткости.
	shortSentenceRun = 3
	// punctBrevityPunct — доля пунктуационного сигнала в блоке «пунктуация и
	// краткость»; остаток — сигнал серии коротких предложений.
	punctBrevityPunct = 0.5
)

// histogramBuckets — корзины распределения длины предложений в словах.
var histogramBuckets = []string{"1-5", "6-10", "11-15", "16-20", "21-30", "31+"}

// StyleChunk — чанк для расчёта драматичности: текст и скоры salience
// категорий action_dramatic, violence, battle и emotion в 0..1 (W-27.3).
// Значения вне 0..1 при расчёте прижимаются к границам.
type StyleChunk struct {
	ID       int64
	Text     string
	Salience map[string]float64
}

// ChunkDrama — драматичность чанка (W-27.3): значение 0..1 в
// book_style.drama_profile как {chunk, drama}.
type ChunkDrama struct {
	Chunk int64   `json:"chunk"`
	Drama float64 `json:"drama"`
}

// BookStyle — стилевой профиль книги (W-27.1): признаки стиля, распределение
// длины предложений, частоты частей речи, ярлыки канона и драматичность
// чанков. AvgSentenceLen не ограничен единицей, остальные признаки лежат в
// 0..1.
type BookStyle struct {
	Author    string
	Features  map[string]float64
	Histogram map[string]int
	POS       map[string]float64
	Labels    []string
	Drama     []ChunkDrama
}

// StyleInput — вход сборки профиля: автор из Meta парсера книги (W-27.5),
// полный текст тела, леммы прогона, чанки со скорами salience и канон ярлыков
// styles.json.
type StyleInput struct {
	Author string
	Text   string
	Lemmas []Lemma
	Chunks []StyleChunk
	Styles *Styles
}

// styleSentence — предложение для профиля: длина в словах, пунктуационные
// маркеры и признак реплики.
type styleSentence struct {
	words     int
	exclaims  int
	questions int
	ellipsis  int
	dialogue  bool
}

// BuildBookStyle собирает стилевой профиль книги (W-27.1–W-27.3): признаки по
// предложениям и леммам, ярлыки сверкой с порогами styles.json, драматичность
// каждого чанка. Пустой текст или отсутствие лемм — ошибка: тихий нулевой
// профиль исказил бы ярлыки и лексическое разнообразие.
func BuildBookStyle(in StyleInput) (*BookStyle, error) {
	if in.Styles == nil {
		return nil, fmt.Errorf("deepread: стиль книги: канон styles.json не загружен")
	}
	if len(in.Lemmas) == 0 {
		return nil, fmt.Errorf("deepread: стиль книги: нет лемм прогона")
	}
	sentences := styleSentences(in.Text)
	if len(sentences) == 0 {
		return nil, fmt.Errorf("deepread: стиль книги: в тексте нет предложений")
	}

	features := styleFeatures(sentences, in.Lemmas)
	labels, err := styleLabelsFor(features, in.Styles)
	if err != nil {
		return nil, err
	}

	style := &BookStyle{
		Author:    strings.TrimSpace(in.Author),
		Features:  features,
		Histogram: sentenceHistogram(sentences),
		POS:       posFrequencies(in.Lemmas),
		Labels:    labels,
		Drama:     make([]ChunkDrama, 0, len(in.Chunks)),
	}
	for _, chunk := range in.Chunks {
		style.Drama = append(style.Drama, ChunkDrama{
			Chunk: chunk.ID,
			Drama: DramaScore(chunk.Text, chunk.Salience),
		})
	}
	if err := validateBookStyle(style); err != nil {
		return nil, err
	}
	return style, nil
}

// DramaScore считает драматичность чанка (W-27.3):
// 0.35×salience(action_dramatic) + 0.25×max(salience(violence), salience(battle))
// + 0.20×salience(emotion) + 0.20×пунктуация_и_краткость; результат в 0..1.
func DramaScore(text string, salience map[string]float64) float64 {
	shortness := punctuationBrevity(styleSentences(text))
	combat := math.Max(saliencePart(salience, KeyViolence), saliencePart(salience, KeyBattle))
	drama := dramaWeightAction*saliencePart(salience, KeyActionDramatic) +
		dramaWeightCombat*combat +
		dramaWeightEmotion*saliencePart(salience, KeyEmotion) +
		dramaWeightPunct*shortness
	return clamp01(drama)
}

// saliencePart читает скор категории: отсутствующее и нечисловое значение —
// ноль, значение вне 0..1 прижимается к границам.
func saliencePart(salience map[string]float64, key string) float64 {
	value, ok := salience[key]
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return clamp01(value)
}

// clamp01 прижимает значение к 0..1.
func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	}
	return value
}

// punctuationBrevity — блок «пунктуация и краткость» (шаг 3): плотность ! и ?
// на предложение (не выше 1) и серия из ≥ 3 предложений короче пяти слов.
// Обе половины весят поровну.
func punctuationBrevity(sentences []styleSentence) float64 {
	if len(sentences) == 0 {
		return 0
	}
	marks := 0
	longest, run := 0, 0
	for _, sentence := range sentences {
		marks += sentence.exclaims + sentence.questions
		if sentence.words < shortSentenceWords {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	density := float64(marks) / float64(len(sentences))
	if density > 1 {
		density = 1
	}
	brevity := 0.0
	if longest >= shortSentenceRun {
		brevity = 1
	}
	return punctBrevityPunct*density + (1-punctBrevityPunct)*brevity
}

// styleSentences делит текст на предложения терминаторами .!?… и переводами
// строк; предложения без слов (одни знаки) в профиль не входят.
func styleSentences(text string) []styleSentence {
	runes := []rune(text)
	spans, _ := splitSentences(runes)
	out := make([]styleSentence, 0, len(spans))
	for _, span := range spans {
		raw := strings.TrimSpace(string(runes[span.start:span.end]))
		if raw == "" {
			continue
		}
		words := wordsOf([]rune(raw))
		if len(words) == 0 {
			continue
		}
		out = append(out, styleSentence{
			words:     len(words),
			exclaims:  strings.Count(raw, "!"),
			questions: strings.Count(raw, "?"),
			ellipsis:  countEllipsis(raw),
			dialogue:  isDialogueSentence(raw),
		})
	}
	return out
}

// countEllipsis считает многоточия предложения: знак … и три точки подряд.
func countEllipsis(sentence string) int {
	return strings.Count(sentence, "…") + strings.Count(sentence, "...")
}

// isDialogueSentence — реплика по тире или кавычкам (шаг 1): начало с тире или
// открывающей кавычки либо цитата внутри предложения.
func isDialogueSentence(sentence string) bool {
	sentence = strings.TrimSpace(sentence)
	if sentence == "" {
		return false
	}
	switch []rune(sentence)[0] {
	case '—', '–', '―', '-', '«', '"', '“', '„':
		return true
	}
	return strings.Contains(sentence, "«") && strings.Contains(sentence, "»")
}

// styleFeatures считает признаки стиля (шаг 1): средняя длина предложения в
// словах, доля диалогов, плотности !, ? и … на предложение, лексическое
// разнообразие уникальных лемм ко всем леммам.
func styleFeatures(sentences []styleSentence, lemmas []Lemma) map[string]float64 {
	features := map[string]float64{
		FeatureAvgSentenceLen:   0,
		FeatureDialogueShare:    0,
		FeatureExclaimDensity:   0,
		FeatureQuestionDensity:  0,
		FeatureEllipsisDensity:  0,
		FeatureLexicalDiversity: 0,
	}
	if len(sentences) > 0 {
		totalWords, dialogue, exclaims, questions, ellipsis := 0, 0, 0, 0, 0
		for _, sentence := range sentences {
			totalWords += sentence.words
			exclaims += sentence.exclaims
			questions += sentence.questions
			ellipsis += sentence.ellipsis
			if sentence.dialogue {
				dialogue++
			}
		}
		n := float64(len(sentences))
		features[FeatureAvgSentenceLen] = float64(totalWords) / n
		features[FeatureDialogueShare] = float64(dialogue) / n
		features[FeatureExclaimDensity] = float64(exclaims) / n
		features[FeatureQuestionDensity] = float64(questions) / n
		features[FeatureEllipsisDensity] = float64(ellipsis) / n
	}

	if len(lemmas) > 0 {
		unique := make(map[string]struct{}, len(lemmas))
		for _, lemma := range lemmas {
			unique[lemma.Lemma] = struct{}{}
		}
		features[FeatureLexicalDiversity] = float64(len(unique)) / float64(len(lemmas))
	}
	return features
}

// sentenceHistogram — распределение длины предложений по корзинам в словах.
func sentenceHistogram(sentences []styleSentence) map[string]int {
	out := make(map[string]int, len(histogramBuckets))
	for _, sentence := range sentences {
		out[sentenceBucket(sentence.words)]++
	}
	return out
}

// sentenceBucket выбирает корзину гистограммы для длины предложения.
func sentenceBucket(words int) string {
	switch {
	case words <= 5:
		return "1-5"
	case words <= 10:
		return "6-10"
	case words <= 15:
		return "11-15"
	case words <= 20:
		return "16-20"
	case words <= 30:
		return "21-30"
	}
	return "31+"
}

// posFrequencies — доли частей речи лемма-слоя: часть речи токена, делённая на
// число токенов с известной частью речи.
func posFrequencies(lemmas []Lemma) map[string]float64 {
	counts := make(map[string]int, 8)
	total := 0
	for _, lemma := range lemmas {
		pos := strings.TrimSpace(lemma.POS)
		if pos == "" {
			continue
		}
		counts[pos]++
		total++
	}
	out := make(map[string]float64, len(counts))
	if total == 0 {
		return out
	}
	for pos, n := range counts {
		out[pos] = float64(n) / float64(total)
	}
	return out
}

// styleLabelsFor сверяет признаки с порогами styles.json (шаг 2): ярлык
// присваивается при выполнении всех его min/max и только из канона
// StyleLabels. Ярлык файла вне канона — ошибка пака.
func styleLabelsFor(features map[string]float64, styles *Styles) ([]string, error) {
	canon := make(map[string]struct{}, len(StyleLabels))
	for _, name := range StyleLabels {
		canon[name] = struct{}{}
	}

	labels := make([]string, 0, len(styles.Labels))
	for _, label := range styles.Labels {
		name := strings.TrimSpace(label.Name)
		if _, ok := canon[name]; !ok {
			return nil, fmt.Errorf("deepread: styles.json: ярлык %q вне канона", name)
		}
		if styleLabelMatches(features, label) {
			labels = append(labels, name)
		}
	}
	return labels, nil
}

// styleLabelMatches сообщает, попадают ли признаки в пороги ярлыка.
func styleLabelMatches(features map[string]float64, label StyleLabel) bool {
	for feature, min := range label.Min {
		if features[feature] < min {
			return false
		}
	}
	for feature, max := range label.Max {
		if features[feature] > max {
			return false
		}
	}
	return true
}

// WriteBookStyle пишет профиль стиля книги в book_style (W-27.1–W-27.3, писатель
// forge): признаки с гистограммой и частотами частей речи — в features, ярлыки
// канона — в labels, драматичность — в drama_profile. Повторная запись
// обновляет строку книги, а не плодит её. Пустой now заменяется текущим UTC.
func WriteBookStyle(lit *db.Handle, bookID int64, style *BookStyle, now string) error {
	if lit == nil {
		return ErrNoStore
	}
	if bookID <= 0 {
		return fmt.Errorf("deepread: запись стиля: нет книги")
	}
	if err := validateBookStyle(style); err != nil {
		return err
	}
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}

	features, err := styleFeaturesJSON(style)
	if err != nil {
		return err
	}
	labels := []byte("[]")
	if len(style.Labels) > 0 {
		if labels, err = json.Marshal(style.Labels); err != nil {
			return fmt.Errorf("deepread: ярлыки стиля: %w", err)
		}
	}
	drama := []byte("[]")
	if len(style.Drama) > 0 {
		if drama, err = json.Marshal(style.Drama); err != nil {
			return fmt.Errorf("deepread: драматичность стиля: %w", err)
		}
	}

	return lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		if _, err := tx.Exec("book_style",
			`INSERT INTO book_style (book_id, author, features, labels, drama_profile, computed_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(book_id) DO UPDATE SET
			   author = excluded.author,
			   features = excluded.features,
			   labels = excluded.labels,
			   drama_profile = excluded.drama_profile,
			   computed_at = excluded.computed_at`,
			bookID, style.Author, string(features), string(labels), string(drama), now); err != nil {
			return fmt.Errorf("deepread: стиль книги %d: %w", bookID, err)
		}
		return nil
	})
}

// styleFeaturesJSON собирает features колонки: признаки канона и рядом
// гистограмма длины предложений и частоты частей речи.
func styleFeaturesJSON(style *BookStyle) ([]byte, error) {
	histogram := style.Histogram
	if histogram == nil {
		histogram = map[string]int{}
	}
	pos := style.POS
	if pos == nil {
		pos = map[string]float64{}
	}
	payload := make(map[string]any, len(style.Features)+2)
	for feature, value := range style.Features {
		payload[feature] = value
	}
	payload["sentence_histogram"] = histogram
	payload["pos_frequencies"] = pos
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("deepread: признаки стиля: %w", err)
	}
	return raw, nil
}

// validStyleFeature — признак входит в канон styles.json features.
func validStyleFeature(feature string) bool {
	switch feature {
	case FeatureAvgSentenceLen, FeatureDialogueShare, FeatureExclaimDensity,
		FeatureQuestionDensity, FeatureEllipsisDensity, FeatureLexicalDiversity:
		return true
	}
	return false
}

// validateBookStyle проверяет профиль перед записью: признаки канона и границы,
// ярлыки только из канона и без повторов, гистограмма по корзинам, частоты
// частей речи и драматичность чанков в 0..1.
func validateBookStyle(style *BookStyle) error {
	if style == nil {
		return fmt.Errorf("deepread: стиль книги: профиль не задан")
	}
	for feature, value := range style.Features {
		if !validStyleFeature(feature) {
			return fmt.Errorf("deepread: стиль книги: признак %q вне канона styles.json", feature)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return fmt.Errorf("deepread: стиль книги: признак %q = %g", feature, value)
		}
		if feature != FeatureAvgSentenceLen && value > 1 {
			return fmt.Errorf("deepread: стиль книги: признак %q = %g вне 0..1", feature, value)
		}
	}

	canon := make(map[string]struct{}, len(StyleLabels))
	for _, name := range StyleLabels {
		canon[name] = struct{}{}
	}
	seenLabels := make(map[string]struct{}, len(style.Labels))
	for _, label := range style.Labels {
		if _, ok := canon[label]; !ok {
			return fmt.Errorf("deepread: стиль книги: ярлык %q вне канона", label)
		}
		if _, dup := seenLabels[label]; dup {
			return fmt.Errorf("deepread: стиль книги: ярлык %q повторяется", label)
		}
		seenLabels[label] = struct{}{}
	}

	for bucket, count := range style.Histogram {
		known := false
		for _, b := range histogramBuckets {
			if b == bucket {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("deepread: стиль книги: корзина %q вне канона", bucket)
		}
		if count < 0 {
			return fmt.Errorf("deepread: стиль книги: корзина %q = %d", bucket, count)
		}
	}

	for pos, share := range style.POS {
		if pos == "" || math.IsNaN(share) || share < 0 || share > 1 {
			return fmt.Errorf("deepread: стиль книги: частота части речи %q = %g вне 0..1", pos, share)
		}
	}

	seenChunks := make(map[int64]struct{}, len(style.Drama))
	for _, item := range style.Drama {
		if item.Chunk <= 0 {
			return fmt.Errorf("deepread: стиль книги: драматичность чанка без id")
		}
		if _, dup := seenChunks[item.Chunk]; dup {
			return fmt.Errorf("deepread: стиль книги: чанк %d повторяется", item.Chunk)
		}
		seenChunks[item.Chunk] = struct{}{}
		if math.IsNaN(item.Drama) || item.Drama < 0 || item.Drama > 1 {
			return fmt.Errorf("deepread: стиль книги: драматичность чанка %d = %g вне 0..1",
				item.Chunk, item.Drama)
		}
	}
	return nil
}
