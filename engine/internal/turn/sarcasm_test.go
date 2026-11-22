package turn

import (
	"os"
	"path/filepath"
	"testing"
)

// sarcasmFixture — словарь T-18-02 с малыми весами: скор считается шагами, и
// тест видит вклад каждого коэффициента без наложения категорий.
const sarcasmFixture = `{
  "meta": {"language": "ru", "sarcasm_threshold": 1.0,
           "contrast_window_tokens": 6, "short_message_max_tokens": 10},
  "coefficients": {
    "position_start_first_3_tokens": 1.5,
    "position_end_last_3_tokens": 1.3,
    "short_message_multiplier": 1.25,
    "in_quotes_multiplier": 3.5,
    "demonstrative_prefix_multiplier": 2.5,
    "trailing_ellipsis_multiplier": 1.4,
    "negative_with_excess_parens_bonus": 0.45,
    "contrast_pair_base_bonus": 0.85
  },
  "patterns": {
    "demonstrative_prefixes": ["наш"],
    "explicit_markers": {"default_weight": 0.95, "items": {"/s": 1.0}},
    "pseudo_agreement": {"apply_start_multiplier": true, "default_weight": 0.6,
      "items": {"ну да": 0.4}},
    "false_surprise": {"apply_short_message_multiplier": true, "default_weight": 0.65,
      "items": {"да ладно": 0.2}},
    "dismissive": {"apply_end_multiplier": true, "default_weight": 0.7,
      "items": {"держи в курсе": 0.5}},
    "hyperbolic_positive": {"contrast_role": "positive", "standalone_weight": 0.15,
      "contrast_boost": 0.25, "items": ["гениально"]},
    "technical_everyday_negative": {"contrast_role": "negative", "standalone_weight": 0.0,
      "contrast_boost": 0.2, "items": ["сломалось"]},
    "ironic_labels": {"standalone_weight": 0.1, "apply_quote_or_prefix_multiplier": true,
      "items": ["эксперт"]}
  }
}`

// loadSarcasmFixture пишет словарь во временный файл и читает его загрузчиком.
func loadSarcasmFixture(t *testing.T) *SarcasmDictionary {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sarcasm.json")
	if err := os.WriteFile(path, []byte(sarcasmFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := loadSarcasmDictionary(path)
	if err != nil {
		t.Fatalf("loadSarcasmDictionary: %v", err)
	}
	if d == nil {
		t.Fatal("словарь сарказма не загружен")
	}
	return d
}

func approxEqual(got, want float64) bool {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-9
}

// TestSarcasmCoefficients проверяет пошаговый скор: позиции, кавычки,
// указательный префикс, контраст-пара, скобки и порог словаря.
func TestSarcasmCoefficients(t *testing.T) {
	lex := &Lexicon{Sarcasm: loadSarcasmFixture(t)}

	cases := []struct {
		name      string
		text      string
		wantScore float64
		want      bool
	}{
		{name: "начало реплики ×1.5", text: "ну да", wantScore: 0.4 * 1.5},
		{name: "короткая реплика ×1.25", text: "да ладно", wantScore: 0.2 * 1.25},
		{name: "конец реплики ×1.3", text: "держи в курсе", wantScore: 0.5 * 1.3},
		{name: "кавычки ×3.5", text: "«ну да»", wantScore: 0.4 * 1.5 * 3.5, want: true},
		{name: "указательный префикс ×2.5", text: "наш эксперт", wantScore: 0.1 * 2.5},
		{name: "ярлык в кавычках ×3.5", text: "«эксперт»", wantScore: 0.1 * 3.5},
		{
			name:      "контраст-пара в короткой реплике",
			text:      "гениально сломалось",
			wantScore: 0.85 + 0.25 + 0.2 + 0.15,
			want:      true,
		},
		{
			name:      "многоточие после похвалы ×1.4",
			text:      "гениально сломалось...",
			wantScore: 0.85 + 0.25 + 0.2 + 0.15*1.4,
			want:      true,
		},
		{name: "скобки после негатива +0.45", text: "сломалось)))", wantScore: 0.45},
		{name: "эксплицитный маркер ~1.0", text: "ну и ну /s", wantScore: 1.0, want: true},
		{name: "прямой негатив без издёвки", text: "ничего не сломалось."},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := lex.DetectSarcasm(c.text)
			if !approxEqual(got.Score, c.wantScore) {
				t.Fatalf("скор %q = %v, want %v", c.text, got.Score, c.wantScore)
			}
			if got.Sarcastic != c.want {
				t.Fatalf("сарказм %q = %v, want %v (скор %v)", c.text, got.Sarcastic, c.want, got.Score)
			}
		})
	}
}

// TestSarcasmMarkers — сработавшие маркеры попадают в вердикт для цензора.
func TestSarcasmMarkers(t *testing.T) {
	lex := &Lexicon{Sarcasm: loadSarcasmFixture(t)}
	v := lex.DetectSarcasm("«ну да»")
	if len(v.Markers) != 1 || v.Markers[0] != "ну да" {
		t.Fatalf("маркеры = %v, want [ну да]", v.Markers)
	}
}

// TestSarcasmRealPack — словарь владельца из packs/ru: порог 1.0, коэффициенты
// и решение на живых репликах чата.
func TestSarcasmRealPack(t *testing.T) {
	lex, err := LoadLexicon(filepath.Join("..", "..", "packs", "ru"))
	if err != nil {
		t.Fatalf("LoadLexicon(packs/ru): %v", err)
	}
	if lex.Sarcasm == nil {
		t.Fatal("sarcasm.json не загружен")
	}
	if lex.Sarcasm.Meta.SarcasmThreshold != 1.0 {
		t.Fatalf("порог = %v, want 1.0", lex.Sarcasm.Meta.SarcasmThreshold)
	}
	coef := lex.Sarcasm.Coefficients
	if coef.PositionStartFirst3 != 1.5 || coef.PositionEndLast3 != 1.3 ||
		coef.ShortMessageMultiplier != 1.25 || coef.InQuotesMultiplier != 3.5 ||
		coef.ContrastPairBaseBonus != 0.85 {
		t.Fatalf("коэффициенты словаря: %+v", coef)
	}

	cases := []struct {
		text string
		want bool
	}{
		{"Ну да, конечно, гениально.", true},
		{"Гениально, опять всё сломалось.", true},
		{"Какая неожиданность!", true},
		{"Очередной отчёт сдан, держи в курсе.", true},
		{"Отличная работа /s", true},
		{"Ну да.", false},
		{"«Эксперт» нашёлся.", false},
		{"Спасибо, всё работает.", false},
		{"Я люблю этот город и его улицы.", false},
		{"Мне грустно, дождь и ветер.", false},
	}
	for _, c := range cases {
		if got := lex.DetectSarcasm(c.text).Sarcastic; got != c.want {
			t.Errorf("сарказм %q = %v, want %v", c.text, got, c.want)
		}
	}
}

// TestSarcasmOptional — пакет без sarcasm.json грузится, детектор не взводится.
func TestSarcasmOptional(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lexicon_intents.jsonl"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "detectors_mood.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	lex, err := LoadLexicon(dir)
	if err != nil {
		t.Fatalf("LoadLexicon без sarcasm.json: %v", err)
	}
	if lex.Sarcasm != nil {
		t.Fatal("словарь сарказма не должен загружаться")
	}
	if v := lex.DetectSarcasm("ну да, конечно"); v.Sarcastic || v.Score != 0 {
		t.Fatalf("вердикт без словаря: %+v", v)
	}
}

// TestSarcasmBroken — битый sarcasm.json — ошибка загрузки пакета.
func TestSarcasmBroken(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lexicon_intents.jsonl"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "detectors_mood.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sarcasm.json"), []byte("{\"patterns\":"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLexicon(dir); err == nil {
		t.Fatal("битый sarcasm.json не дал ошибки загрузки")
	}
}
