package turn

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// moodFixture — весовой словарь T-18-01 для герметичных тестов скоринга:
// пять осей с valence, модификаторы и текстовые сигналы.
const moodFixture = `{
  "meta": {"language": "ru", "item_form": "lemma", "neutral_deadzone": 0.25,
           "negation_window_tokens": 3, "intensifier_window_tokens": 2},
  "modifiers": {
    "negation_multiplier": -0.75,
    "negation_tokens": ["не", "ни", "нет"],
    "intensifiers": {"default_multiplier": 1.4, "tokens": {"очень": 1.35, "крайне": 1.5}},
    "diminishers": {"default_multiplier": 0.6, "tokens": {"немного": 0.6}}
  },
  "signals": {
    "caps_lock_word_multiplier": 1.3,
    "multiple_exclamation_multiplier": 1.25,
    "multiple_question_multiplier": 1.2,
    "elongated_vowel_multiplier": 1.25,
    "trailing_ellipsis_sadness_bonus": 0.2,
    "emoticon_bonus": 0.3,
    "text_emoticons": {
      "joy_approval": [":)", "))"],
      "sadness_fatigue": ["хнык", "(("],
      "surprise_interest": ["?!"]
    }
  },
  "mood_vectors": {
    "joy_approval": {"mood": "радость", "valence": 1.0,
      "items": {"отлично": 0.8, "нравиться": 0.7, "круто": 0.8, "в точку": 0.75}},
    "anger_irritation": {"mood": "злость", "valence": -1.0,
      "items": {"бесить": 0.95}},
    "sadness_fatigue": {"mood": "уныние", "valence": -0.8,
      "items": {"грустно": 0.85, "лень": 0.55}},
    "fear_anxiety": {"mood": "тревога", "valence": -0.7,
      "items": {"бояться": 0.9}},
    "surprise_interest": {"mood": "удивление", "valence": 0.3,
      "items": {"ого": 0.8}}
  }
}`

// loadMoodFixture пишет словарь во временный каталог пакета и читает лексикон.
func loadMoodFixture(t *testing.T) *Lexicon {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lexicon_intents.jsonl"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "detectors_mood.json"), []byte(moodFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	lex, err := LoadLexicon(dir)
	if err != nil {
		t.Fatalf("LoadLexicon: %v", err)
	}
	return lex
}

// TestDetectMoodVectors проверяет скоринг осей, модификаторы и сигналы T-18-01.
func TestDetectMoodVectors(t *testing.T) {
	lex := loadMoodFixture(t)

	cases := []struct {
		name   string
		text   string
		lemmas []string
		want   string
	}{
		{"вес оси", "отлично", []string{"отлично"}, "радость"},
		{"усилитель", "мне очень нравится", []string{"я", "очень", "нравиться"}, "радость"},
		{"ослабитель", "мне немного грустно", []string{"я", "немного", "грустно"}, "уныние"},
		{"отрицание не", "не боюсь", []string{"не", "бояться"}, ""},
		{"отрицание ловит только лемма не", "нисколько боюсь", []string{"нисколько", "бояться"}, "тревога"},
		{"отрицание действует в своём окне", "не боюсь я этого, отлично", []string{"не", "бояться", "я", "этого", "отлично"}, "радость"},
		{"caps", "КРУТО", []string{"круто"}, "радость"},
		{"повтор восклицаний", "бесит!!!", []string{"бесить"}, "злость"},
		{"тянущиеся гласные", "грууустно", []string{"грустно"}, "уныние"},
		{"многоточие", "лень...", []string{"лень"}, "уныние"},
		{"смайл", "хнык", []string{"хнык"}, "уныние"},
		{"смайл не ищется внутри слова", "круги", []string{"круг"}, ""},
		{"фраза из лемм", "в точку", []string{"в", "точку"}, "радость"},
		{"мёртвая зона", "ого", []string{"ого"}, ""},
		{"сигнал выводит из мёртвой зоны", "ого!!!", []string{"ого"}, "удивление"},
		{"нейтральная реплика", "иду домой", []string{"идти", "домой"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detectMood(c.text, c.lemmas, lex); got != c.want {
				t.Fatalf("detectMood(%q, %v) = %q, want %q", c.text, c.lemmas, got, c.want)
			}
		})
	}
}

// TestPrepathDetectMoodSignals проверяет, что предпуть передаёт текст реплики в
// скоринг: без сигналов «ого!!!» осталось бы в мёртвой зоне.
func TestPrepathDetectMoodSignals(t *testing.T) {
	lex := loadMoodFixture(t)
	st := &TurnState{
		Input: TurnInput{SessionID: 1, PlayerText: "ого!!!"},
		Cut:   Cut{Segments: []Segment{{Text: "ого", Lemmas: []string{"ого"}}}},
	}
	stage := &prepathStage{lexicon: lex, binder: NewSlotBinder(Roster{})}
	if err := stage.Run(context.Background(), st); err != nil {
		t.Fatalf("prepath: %v", err)
	}
	if st.Prepath.Mood != "удивление" {
		t.Fatalf("настроение = %q, want удивление", st.Prepath.Mood)
	}
	if st.Prepath.Preselect != PreselectWeaver {
		t.Fatalf("предвыбор = %q, want %q", st.Prepath.Preselect, PreselectWeaver)
	}
}

// TestDetectMoodPack проверяет словарь пакета: пять осей, плоские mood/lemmas
// для оси emotion разбора книг (W-22.2) и скоринг живых реплик.
func TestDetectMoodPack(t *testing.T) {
	lex, err := LoadLexicon(filepath.Join("..", "..", "packs", "ru"))
	if err != nil {
		t.Fatalf("LoadLexicon(packs/ru): %v", err)
	}
	if len(lex.MoodAxes) != 5 {
		t.Fatalf("осей настроения = %d, want 5", len(lex.MoodAxes))
	}
	if len(lex.Moods) != 1 || lex.Moods[0].Mood != "тревога" || len(lex.Moods[0].Lemmas) == 0 {
		t.Fatalf("плоский словарь T-01-08 потерян: %+v", lex.Moods)
	}

	cases := []struct {
		text   string
		lemmas []string
		want   string
	}{
		{"Это просто отлично!", []string{"это", "просто", "отлично"}, "радость"},
		{"мне очень нравится", []string{"я", "очень", "нравиться"}, "радость"},
		{"бесит!!!", []string{"бесить"}, "злость"},
		{"Грустно...", []string{"грустно"}, "уныние"},
		{"хнык", []string{"хнык"}, "уныние"},
		{"не боюсь", []string{"не", "бояться"}, ""},
	}
	for _, c := range cases {
		t.Run(c.text, func(t *testing.T) {
			if got := detectMood(c.text, c.lemmas, lex); got != c.want {
				t.Fatalf("detectMood(%q, %v) = %q, want %q", c.text, c.lemmas, got, c.want)
			}
		})
	}
}
