package turn

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Lexicon — лексикон языкового пакета для предпути: намерения из
// lexicon_intents.jsonl и настроение из detectors_mood.json (T-01-08, шаг 6).
// Гипотеза намерения и настроение реплики считаются кодом, без LLM (LT-03.7).
// Весовые оси настроения, модификаторы и сигналы — словарь тональности T-18-01;
// словарь сарказма — детектор издёвки T-18-02.
type Lexicon struct {
	Intents []IntentEntry
	Moods   []MoodEntry

	Meta      MoodMeta
	Modifiers MoodModifiers
	Signals   MoodSignals
	MoodAxes  map[string]MoodAxis
	Sarcasm   *SarcasmDictionary
	axisOrder []string
}

// MoodMeta — параметры скоринга настроения: мёртвая зона нейтральной реплики и
// окна модификаторов в леммах.
type MoodMeta struct {
	Language                string  `json:"language"`
	ItemForm                string  `json:"item_form"`
	NeutralDeadzone         float64 `json:"neutral_deadzone"`
	NegationWindowTokens    int     `json:"negation_window_tokens"`
	IntensifierWindowTokens int     `json:"intensifier_window_tokens"`
}

// MoodAxis — ось настроения весового словаря: имя настроения, valence и веса
// элементов. Элемент — лемма или фраза из подряд идущих лемм.
type MoodAxis struct {
	Mood    string             `json:"mood"`
	Valence float64            `json:"valence"`
	Items   map[string]float64 `json:"items"`
}

// MoodScale — шкала усилителей или ослабителей: множитель по леммам.
type MoodScale struct {
	DefaultMultiplier float64            `json:"default_multiplier"`
	Tokens            map[string]float64 `json:"tokens"`
}

// MoodModifiers — слой модификаторов тональности (D-13): отрицание, усилители,
// ослабители. Отрицание в скоринге ловит только лемма «не» (negationLemma).
type MoodModifiers struct {
	NegationMultiplier float64   `json:"negation_multiplier"`
	NegationTokens     []string  `json:"negation_tokens"`
	Intensifiers       MoodScale `json:"intensifiers"`
	Diminishers        MoodScale `json:"diminishers"`
}

// MoodSignals — текстовые сигналы тональности без графических эмодзи: CAPS,
// повторы знаков, тянущиеся гласные, ASCII-смайлы и многоточие.
type MoodSignals struct {
	CapsLockWordMultiplier        float64             `json:"caps_lock_word_multiplier"`
	MultipleExclamationMultiplier float64             `json:"multiple_exclamation_multiplier"`
	MultipleQuestionMultiplier    float64             `json:"multiple_question_multiplier"`
	ElongatedVowelMultiplier      float64             `json:"elongated_vowel_multiplier"`
	TrailingEllipsisSadnessBonus  float64             `json:"trailing_ellipsis_sadness_bonus"`
	EmoticonBonus                 float64             `json:"emoticon_bonus"`
	TextEmoticons                 map[string][]string `json:"text_emoticons"`
}

// moodDictionary — настроение из detectors_mood.json: плоский лексикон T-01-08
// (mood/lemmas) и весовой словарь T-18-01 (mood_vectors, modifiers, signals).
type moodDictionary struct {
	Moods     []MoodEntry
	Meta      MoodMeta
	Modifiers MoodModifiers
	Signals   MoodSignals
	Axes      map[string]MoodAxis
	Order     []string
}

// IntentEntry — строка лексикона намерений: имя намерения, леммы и глаголы.
type IntentEntry struct {
	Intent string   `json:"intent"`
	Lemmas []string `json:"lemmas"`
	Verbs  []string `json:"verbs"`
}

// MoodEntry — запись лексикона настроений: имя настроения и его леммы.
type MoodEntry struct {
	Mood   string   `json:"mood"`
	Lemmas []string `json:"lemmas"`
}

// LoadLexicon читает лексикон намерений и настроений из каталога языкового
// пакета dir. Ошибка — если файлы лексикона отсутствуют или биты.
func LoadLexicon(dir string) (*Lexicon, error) {
	intents, err := loadIntentEntries(filepath.Join(dir, "lexicon_intents.jsonl"))
	if err != nil {
		return nil, err
	}
	dict, err := loadMoodDictionary(filepath.Join(dir, "detectors_mood.json"))
	if err != nil {
		return nil, err
	}
	sarcasm, err := loadSarcasmDictionary(filepath.Join(dir, "sarcasm.json"))
	if err != nil {
		return nil, err
	}
	lex := &Lexicon{Intents: intents, Sarcasm: sarcasm}
	if dict != nil {
		lex.Moods = dict.Moods
		lex.Meta = dict.Meta
		lex.Modifiers = dict.Modifiers
		lex.Signals = dict.Signals
		lex.MoodAxes = dict.Axes
		lex.axisOrder = dict.Order
	}
	return lex, nil
}

func loadIntentEntries(path string) ([]IntentEntry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []IntentEntry
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e IntentEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// loadMoodDictionary читает detectors_mood.json. Плоские mood/lemmas остаются
// контрактом T-01-08, весовой словарь T-18-01 читается теми же полями файла.
// Пустой файл без обеих частей — пустой словарь (nil) без ошибки.
func loadMoodDictionary(path string) (*moodDictionary, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file struct {
		Mood        string              `json:"mood"`
		Lemmas      []string            `json:"lemmas"`
		Meta        MoodMeta            `json:"meta"`
		Modifiers   MoodModifiers       `json:"modifiers"`
		Signals     MoodSignals         `json:"signals"`
		MoodVectors map[string]MoodAxis `json:"mood_vectors"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		return nil, err
	}
	if file.Mood == "" && len(file.Lemmas) == 0 && len(file.MoodVectors) == 0 {
		return nil, nil
	}
	dict := &moodDictionary{
		Meta:      file.Meta,
		Modifiers: file.Modifiers,
		Signals:   file.Signals,
		Axes:      file.MoodVectors,
	}
	if file.Mood != "" || len(file.Lemmas) > 0 {
		dict.Moods = []MoodEntry{{Mood: file.Mood, Lemmas: file.Lemmas}}
	}
	// Порядок осей фиксирован сортировкой: карта JSON не задаёт его, а выбор
	// настроения при равных весах должен быть детерминированным.
	for name := range file.MoodVectors {
		dict.Order = append(dict.Order, name)
	}
	sort.Strings(dict.Order)
	return dict, nil
}

// Intent — гипотеза намерения реплики: имя из лексикона и совпавшая лемма-цитата.
type Intent struct {
	Name  string
	Quote string
}

// hypothesizeIntent подбирает намерение по леммам реплики без LLM: побеждает
// намерение с наибольшим числом совпадений лемм и глаголов; при равенстве —
// первое в файле.
func hypothesizeIntent(lemmas []string, lex *Lexicon) Intent {
	if lex == nil {
		return Intent{}
	}
	var best Intent
	bestScore := 0
	for _, e := range lex.Intents {
		score := countOverlap(lemmas, e.Lemmas) + countOverlap(lemmas, e.Verbs)
		if score > bestScore {
			bestScore = score
			best = Intent{Name: e.Intent, Quote: firstOverlap(lemmas, e.Lemmas, e.Verbs)}
		}
	}
	return best
}

// negationLemma — единственная лемма отрицания в скоринге (D-13): остальные
// формы из negation_tokens — справочный слой словаря, а не правило разбора.
const negationLemma = "не"

// detectMood подбирает настроение реплики по весовому словарю: сумма весов
// элементов оси с модификаторами и сигналами; пусто, если реплика нейтральна.
// Словарь без mood_vectors работает по плоскому контракту T-01-08.
func detectMood(text string, lemmas []string, lex *Lexicon) string {
	if lex == nil {
		return ""
	}
	if len(lex.MoodAxes) == 0 {
		for _, m := range lex.Moods {
			if countOverlap(lemmas, m.Lemmas) > 0 {
				return m.Mood
			}
		}
		return ""
	}
	sig := lex.analyzeSignals(text)
	best, bestScore := "", 0.0
	for _, name := range lex.axisOrder {
		score := lex.moodAxisScore(name, lemmas, sig)
		if score > bestScore {
			best, bestScore = lex.MoodAxes[name].Mood, score
		}
	}
	if bestScore < lex.Meta.NeutralDeadzone {
		return ""
	}
	return best
}

// moodAxisScore — вес оси: вхождения элементов словаря, помноженные на
// модификаторы окна, общий множитель сигналов, |valence| и бонус сигналов оси.
// |valence| — вес оси: слабая ось (удивление 0.3) требует более сильного
// свидетельства, чем сильная (радость 1.0); знак valence в выбор не входит.
func (lex *Lexicon) moodAxisScore(name string, lemmas []string, sig moodSignals) float64 {
	axis := lex.MoodAxes[name]
	items := make([]string, 0, len(axis.Items))
	for item := range axis.Items {
		items = append(items, item)
	}
	sort.Strings(items) // детерминированный порядок суммирования весов
	total := 0.0
	for _, item := range items {
		for _, pos := range matchMoodItem(lemmas, item) {
			weight := axis.Items[item]
			if negatedMood(lemmas, pos, lex.Meta.NegationWindowTokens) {
				weight *= lex.Modifiers.NegationMultiplier
			} else {
				weight *= lex.modifierMultiplier(lemmas, pos)
			}
			total += weight
		}
	}
	total *= sig.multiplier
	if valence := math.Abs(axis.Valence); valence > 0 {
		total *= valence
	}
	return total + sig.bonus[name]
}

// matchMoodItem ищет вхождения элемента словаря в леммы реплики: одно слово —
// по равенству леммы, фраза — по подряд идущим леммам.
func matchMoodItem(lemmas []string, item string) []int {
	words := strings.Fields(item)
	if len(words) == 0 {
		return nil
	}
	var out []int
	for i := 0; i+len(words) <= len(lemmas); i++ {
		ok := true
		for j, w := range words {
			if lemmas[i+j] != w {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, i)
		}
	}
	return out
}

// negatedMood — перед вхождением в окне стоит лемма «не» (D-13).
func negatedMood(lemmas []string, pos, window int) bool {
	if window <= 0 {
		return false
	}
	from := pos - window
	if from < 0 {
		from = 0
	}
	for i := from; i < pos; i++ {
		if lemmas[i] == negationLemma {
			return true
		}
	}
	return false
}

// modifierMultiplier — множитель ближайшего усилителя или ослабителя перед
// вхождением; 1, если в окне их нет.
func (lex *Lexicon) modifierMultiplier(lemmas []string, pos int) float64 {
	window := lex.Meta.IntensifierWindowTokens
	if window <= 0 {
		return 1
	}
	from := pos - window
	if from < 0 {
		from = 0
	}
	for i := pos - 1; i >= from; i-- {
		if m, ok := lex.Modifiers.Intensifiers.Tokens[lemmas[i]]; ok {
			return m
		}
		if m, ok := lex.Modifiers.Diminishers.Tokens[lemmas[i]]; ok {
			return m
		}
	}
	return 1
}

// moodSignals — итог текстовых сигналов реплики: общий множитель интонации и
// бонусы осей по имени оси словаря.
type moodSignals struct {
	multiplier float64
	bonus      map[string]float64
}

// analyzeSignals считает сигналы чисто текстового чата: CAPS, повторы ! и ?,
// тянущиеся гласные, многоточие и ASCII-смайлы. Отсутствующий множитель
// словаря (0) сигнал не даёт, а не обнуляет вес оси.
func (lex *Lexicon) analyzeSignals(text string) moodSignals {
	sig := moodSignals{multiplier: 1, bonus: map[string]float64{}}
	if text == "" {
		return sig
	}
	if m := lex.Signals.MultipleExclamationMultiplier; m > 0 && strings.Count(text, "!") >= 2 {
		sig.multiplier *= m
	}
	if m := lex.Signals.MultipleQuestionMultiplier; m > 0 && strings.Count(text, "?") >= 2 {
		sig.multiplier *= m
	}
	if m := lex.Signals.CapsLockWordMultiplier; m > 0 && hasCapsWord(text) {
		sig.multiplier *= m
	}
	if m := lex.Signals.ElongatedVowelMultiplier; m > 0 && hasElongatedVowel(text) {
		sig.multiplier *= m
	}
	if hasTrailingEllipsis(text) {
		sig.bonus["sadness_fatigue"] += lex.Signals.TrailingEllipsisSadnessBonus
	}
	lower := strings.ToLower(text)
	for axis, marks := range lex.Signals.TextEmoticons {
		for _, mark := range marks {
			if containsEmoticon(lower, strings.ToLower(mark)) {
				sig.bonus[axis] += lex.Signals.EmoticonBonus
				break
			}
		}
	}
	return sig
}

// hasCapsWord — в реплике есть слово из двух и более букв целиком в верхнем
// регистре (крик без графических эмодзи).
func hasCapsWord(text string) bool {
	for _, word := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) }) {
		runeCount := 0
		caps := true
		for _, r := range word {
			runeCount++
			if !unicode.IsUpper(r) {
				caps = false
			}
		}
		if runeCount >= 2 && caps {
			return true
		}
	}
	return false
}

// hasElongatedVowel — три и более одинаковых гласных подряд («круууто»,
// «нееет»): удлинение фиксируется как усилитель эмоции.
func hasElongatedVowel(text string) bool {
	run, prev := 0, rune(0)
	for _, r := range strings.ToLower(text) {
		if r == prev && isVowel(r) {
			run++
			if run >= 3 {
				return true
			}
			continue
		}
		prev, run = r, 1
	}
	return false
}

func isVowel(r rune) bool {
	return strings.ContainsRune("аеёиоуыэюяaeiouy", r)
}

// hasTrailingEllipsis — реплика заканчивается многоточием.
func hasTrailingEllipsis(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.HasSuffix(trimmed, "…") || strings.HasSuffix(trimmed, "...")
}

// containsEmoticon — ASCII-смайл или реплика-междометие из signals словаря:
// буквенные маркеры ищутся целым словом, символьные — подстрокой.
func containsEmoticon(text, mark string) bool {
	if mark == "" {
		return false
	}
	letters := true
	for _, r := range mark {
		if !unicode.IsLetter(r) {
			letters = false
			break
		}
	}
	if !letters {
		return strings.Contains(text, mark)
	}
	for _, word := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) }) {
		if word == mark {
			return true
		}
	}
	return false
}

// SceneFrame — рамка сцены из механики: состояние героя, граф и физиология от
// слов реплики не зависят (T-03-03, шаг 1).
type SceneFrame struct {
	HeroPlaceID int64
	HeroMood    string
}

// Preselect — решение предвыбора: запись из пула банка или преднабор промпта
// Ткача. Окончательное решение пути — за Селектором (T-03-09), предвыбор лишь
// готовит заготовку.
type Preselect string

const (
	PreselectBank   Preselect = "bank"
	PreselectWeaver Preselect = "weaver"
)

// Prepath — результат предварительного пути: гипотеза намерения, настроение,
// рамка сцены, предвыбор и собранный шаблон ответа. Template — с токенами
// биндера, Prompt — закрытый биндером текст без заменителей. До подтверждения
// смысла предпуть только читает и готовит: всё это живёт в памяти хода, а в базу
// попадает на коммите стоп-крана (T-03-05, LT-07.1).
type Prepath struct {
	Intent        Intent
	Mood          string
	Frame         SceneFrame
	Preselect     Preselect
	Template      string
	Prompt        string
	LexiconLoaded bool // лексикон загружен: стоп-крану есть что подтверждать (T-03-05).
}

// preselect выбирает заготовку пути по намерению и настроению детерминированно,
// без LLM: движение и речь — Ткач (нужна генерация), прочее и пустое — банк.
func preselect(intent Intent, mood string) Preselect {
	if intent.Name == "перемещение" || intent.Name == "речь" || mood != "" {
		return PreselectWeaver
	}
	return PreselectBank
}

// promptTemplate — преднабор промпта Ткача или запись пула банка с токенами,
// которые закрывает биндер слотов (T-03-03, шаг 2).
func promptTemplate(sel Preselect) string {
	if sel == PreselectWeaver {
		return weaverPreset
	}
	return bankPreset
}

const weaverPreset = "[NPCname] смотрит, как ты входишь в [PLACE]. [NPCroleDeity] молчит."
const bankPreset = "Ты у [PLACE]. Всё идёт своим чередом."

// collectLemmas собирает леммы сегментов резки в плоский список.
func collectLemmas(c Cut) []string {
	var out []string
	for _, seg := range c.Segments {
		out = append(out, seg.Lemmas...)
	}
	return out
}

func countOverlap(haystack, needles []string) int {
	n := 0
	for _, h := range haystack {
		for _, needle := range needles {
			if h == needle {
				n++
				break
			}
		}
	}
	return n
}

func firstOverlap(haystack []string, groups ...[]string) string {
	for _, h := range haystack {
		for _, group := range groups {
			for _, needle := range group {
				if h == needle {
					return needle
				}
			}
		}
	}
	return ""
}
