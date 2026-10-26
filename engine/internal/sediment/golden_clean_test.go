package sediment_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/models"
	"tns65/internal/sediment"
)

// goldenDir — золотой набор T-04-08: входная сцена, эталонная очищенная форма
// со слотами и эталонное решение дедупа. Прогон без модели и сети: векторы
// читаются из фикстур, а не из эмбеддера :5002.
const goldenDir = "testdata/golden"

// goldenInput — входная сцена набора: сцена оседания и привязки биндера.
type goldenInput struct {
	ID       int64              `json:"id"`
	Status   string             `json:"status"`
	Prose    string             `json:"prose"`
	Facts    []sediment.Fact    `json:"facts"`
	Bindings []sediment.Binding `json:"bindings"`
}

// goldenClean — ожидаемая очищенная форма со слот-токенами (W-03.2).
type goldenClean struct {
	CleanedText string   `json:"cleaned_text"`
	Slots       []string `json:"slots"`
}

// goldenDedupCheck — один ожидаемый результат дедупа: вектор существующей
// записи из фикстуры и решение pass/duplicate (W-03.4).
type goldenDedupCheck struct {
	Vec      []float32 `json:"vec"`
	Expected string    `json:"expected"`
}

// goldenDedup — ожидаемое решение дедупа: порог, вектор сцены и проверки.
type goldenDedup struct {
	Threshold float64            `json:"threshold"`
	Query     []float32          `json:"query"`
	Checks    []goldenDedupCheck `json:"checks"`
}

// TestGoldenCleanDedup — golden-тест очистки и дедупа (T-04-08): каждый файл
// набора даёт ровно один ожидаемый результат; сеть и модель не используются.
func TestGoldenCleanDedup(t *testing.T) {
	for _, pattern := range []string{"scene_in_*.json", "scene_clean_*.json", "scene_dedup_*.json"} {
		files, err := filepath.Glob(filepath.Join(goldenDir, pattern))
		if err != nil {
			t.Fatalf("glob %s: %v", filepath.Join(goldenDir, pattern), err)
		}
		if len(files) != 1 {
			t.Fatalf("%s: найдено %d файлов, want ровно 1: %v", pattern, len(files), files)
		}
	}

	ins, _ := filepath.Glob(filepath.Join(goldenDir, "scene_in_*.json"))
	for _, inPath := range ins {
		checkGoldenCase(t, inPath)
	}
}

// checkGoldenCase сверяет одну входную сцену набора с эталонами очистки и
// дедупа; расхождение падает с путём к файлу набора.
func checkGoldenCase(t *testing.T, inPath string) {
	t.Helper()

	in := loadJSON[goldenInput](t, inPath)
	scene := sediment.Scene{ID: in.ID, Status: in.Status, Prose: in.Prose, Facts: in.Facts}

	// Очистка — ровно одна ожидаемая форма со слотами.
	cleanPath := strings.Replace(inPath, "scene_in_", "scene_clean_", 1)
	wantClean := loadJSON[goldenClean](t, cleanPath)

	rec, err := sediment.Clean(scene, in.Bindings)
	if err != nil {
		t.Fatalf("Clean(%s): %v", inPath, err)
	}
	if rec.CleanedText != wantClean.CleanedText {
		t.Fatalf("cleaned_text не совпал с эталоном %s:\n got: %q\nwant: %q", cleanPath, rec.CleanedText, wantClean.CleanedText)
	}
	if !equalStrings(rec.Slots, wantClean.Slots) {
		t.Fatalf("slots не совпали с эталоном %s:\n got: %v\nwant: %v", cleanPath, rec.Slots, wantClean.Slots)
	}

	// Дедуп — ровно одно ожидаемое решение по векторам из фикстуры.
	dedupPath := strings.Replace(inPath, "scene_in_", "scene_dedup_", 1)
	got := loadJSON[goldenDedup](t, dedupPath)

	if len(got.Query) != models.EmbedDim {
		t.Fatalf("query-вектор из %s: dim=%d, want %d (векторы берутся из фикстур, сеть не используется)", dedupPath, len(got.Query), models.EmbedDim)
	}
	for i, check := range got.Checks {
		if len(check.Vec) != models.EmbedDim {
			t.Fatalf("check[%d] из %s: dim=%d, want %d", i, dedupPath, len(check.Vec), models.EmbedDim)
		}
		wantDup, ok := dedupExpect(check.Expected)
		if !ok {
			t.Fatalf("check[%d] из %s: неизвестное ожидание %q", i, dedupPath, check.Expected)
		}
		if dup := sediment.IsDuplicate(got.Query, [][]float32{check.Vec}, got.Threshold); dup != wantDup {
			t.Fatalf("дедуп check[%d] из %s: got %v, want %v (%s)", i, dedupPath, dup, wantDup, check.Expected)
		}
	}
}

// dedupExpect переводит ожидание фикстуры в булево решение IsDuplicate.
func dedupExpect(s string) (bool, bool) {
	switch s {
	case "pass":
		return false, true
	case "duplicate":
		return true, true
	default:
		return false, false
	}
}

// loadJSON читает и разбирает JSON-файл фикстуры; ошибка несёт путь к файлу.
func loadJSON[T any](t *testing.T, path string) T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("читать %s: %v", path, err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("разобрать %s: %v", path, err)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
