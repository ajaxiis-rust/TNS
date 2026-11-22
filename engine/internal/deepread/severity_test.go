package deepread_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/deepread"
)

// TestSeverityWeights — таблица весов книги по severity (T-18-03, шаг 2):
// severity 1–4 → 1/2/2/3, значения вне границ отвергаются.
func TestSeverityWeights(t *testing.T) {
	want := map[int]int{1: 1, 2: 2, 3: 2, 4: 3}
	for severity, weight := range want {
		got, ok := deepread.SeverityWeight(severity)
		if !ok || got != weight {
			t.Errorf("SeverityWeight(%d) = %d, %v; want %d, true", severity, got, ok, weight)
		}
	}
	for _, severity := range []int{-1, 0, 5} {
		if _, ok := deepread.SeverityWeight(severity); ok {
			t.Errorf("SeverityWeight(%d): severity вне 1–4 принята", severity)
		}
	}
}

// TestEroticSeverity — слой severity пакета (T-18-03): покрывает все леммы
// erotic.json, вес леммы равен таблице, разбор 18+ чанка даёт категорию erotic
// в полосе confident.
func TestEroticSeverity(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Fatal("корень движка не найден")
	}
	packDir := filepath.Join(root, "packs", "ru")

	dict, err := deepread.LoadDictionary(packDir, deepread.KeyErotic)
	if err != nil {
		t.Fatalf("LoadDictionary(erotic): %v", err)
	}
	if len(dict.Severity) != len(dict.Lemmas) {
		t.Fatalf("слой severity покрывает %d лемм из %d", len(dict.Severity), len(dict.Lemmas))
	}
	for lemma, weight := range dict.Lemmas {
		severity, ok := dict.Severity[lemma]
		if !ok {
			t.Errorf("лемма %q без severity", lemma)
			continue
		}
		want, ok := deepread.SeverityWeight(severity)
		if !ok {
			t.Errorf("лемма %q: severity %d вне 1–4", lemma, severity)
			continue
		}
		if weight != want {
			t.Errorf("лемма %q: вес %d, severity %d даёт вес %d", lemma, weight, severity, want)
		}
	}
	if severity := dict.Severity["эротика"]; severity != 4 {
		t.Errorf("severity(эротика) = %d, want 4", severity)
	}

	dicts, err := deepread.LoadDictionaries(packDir)
	if err != nil {
		t.Fatalf("LoadDictionaries: %v", err)
	}
	chunk := fixtureChunk(1, strings.Join([]string{
		"Она разделась и прильнула к нему.",
		"Он поцеловал её в губы.",
		"Это была чистая эротика и секс.",
	}, "\n"), mark{"разделась": "раздеться", "прильнула": "прильнуть", "поцеловал": "поцеловать"})

	tags, err := deepread.TagChunks([]deepread.SalienceChunk{chunk}, dicts, deepread.DefaultSalienceConfig())
	if err != nil {
		t.Fatalf("TagChunks: %v", err)
	}
	var erotic *deepread.ChunkTag
	for i := range tags[0].Tags {
		if tags[0].Tags[i].Key == deepread.KeyErotic {
			erotic = &tags[0].Tags[i]
		}
	}
	if erotic == nil {
		t.Fatal("нет тега категории erotic")
	}
	if erotic.Band != deepread.BandConfident {
		t.Errorf("18+ чанк: полоса erotic %q (скор %.3f), want confident", erotic.Band, erotic.Score)
	}
	if len(erotic.Anchors) == 0 {
		t.Error("18+ чанк: у erotic нет якорей")
	}
}

// writeErotic кладёт erotic.json фикстуры во временный каталог пака.
func writeErotic(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "erotic.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestEroticSeverityValidation — слой severity проверяется при загрузке:
// покрытие всех лемм, границы 1–4 и соответствие веса таблице; словарь без
// слоя грузится как раньше.
func TestEroticSeverityValidation(t *testing.T) {
	cases := []struct{ name, content, want string }{
		{"severity вне границ", `{"lemmas":{"секс":1},"severity":{"секс":5}}`, "вне 1..4"},
		{"вес не по severity", `{"lemmas":{"секс":3},"severity":{"секс":1}}`, "не соответствует severity"},
		{"лемма вне словаря", `{"lemmas":{"секс":3},"severity":{"секс":4,"порно":4}}`, "вне словаря"},
		{"неполное покрытие", `{"lemmas":{"секс":3,"порно":3},"severity":{"секс":4}}`, "покрывает"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := deepread.LoadDictionary(writeErotic(t, tc.content), deepread.KeyErotic)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("LoadDictionary: %v, ожидалась ошибка с %q", err, tc.want)
			}
		})
	}

	dir := writeErotic(t, `{"lemmas":{"секс":3}}`)
	dict, err := deepread.LoadDictionary(dir, deepread.KeyErotic)
	if err != nil {
		t.Fatalf("LoadDictionary без severity: %v", err)
	}
	if len(dict.Severity) != 0 {
		t.Errorf("словарь без severity: слой %v, want пусто", dict.Severity)
	}
}
