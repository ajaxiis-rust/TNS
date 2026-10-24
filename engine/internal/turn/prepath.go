package turn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Lexicon — лексикон языкового пакета для предпути: намерения из
// lexicon_intents.jsonl и настроение из detectors_mood.json (T-01-08, шаг 6).
// Гипотеза намерения и настроение реплики считаются кодом, без LLM (LT-03.7).
type Lexicon struct {
	Intents []IntentEntry
	Moods   []MoodEntry
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
	moods, err := loadMoodEntries(filepath.Join(dir, "detectors_mood.json"))
	if err != nil {
		return nil, err
	}
	return &Lexicon{Intents: intents, Moods: moods}, nil
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

func loadMoodEntries(path string) ([]MoodEntry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m MoodEntry
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.Mood == "" && len(m.Lemmas) == 0 {
		return nil, nil
	}
	return []MoodEntry{m}, nil
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

// detectMood подбирает настроение реплики по лексикону настроений; пусто, если
// ни одна лемма настроения не найдена.
func detectMood(lemmas []string, lex *Lexicon) string {
	if lex == nil {
		return ""
	}
	for _, m := range lex.Moods {
		if countOverlap(lemmas, m.Lemmas) > 0 {
			return m.Mood
		}
	}
	return ""
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
// биндера, Prompt — закрытый биндером текст без заменителей.
type Prepath struct {
	Intent    Intent
	Mood      string
	Frame     SceneFrame
	Preselect Preselect
	Template  string
	Prompt    string
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
