package structural

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFamilies(t *testing.T) {
	families, err := LoadFamilies(filepath.Join("testdata", "archetypes.json"))
	if err != nil {
		t.Fatalf("LoadFamilies: %v", err)
	}
	if len(families) != 14 {
		t.Fatalf("семей: %d, want 14", len(families))
	}
	// канонический порядок = порядок файла: загрузчик не сортирует.
	if families[0].Name != "escape_liberation" {
		t.Fatalf("первое семейство: %q, want escape_liberation", families[0].Name)
	}
	if families[13].Name != "teaching_discourse" {
		t.Fatalf("последнее семейство: %q, want teaching_discourse", families[13].Name)
	}
	for _, f := range families {
		if f.Name == "" || f.GlossEN == "" || len(f.Keywords) == 0 {
			t.Fatalf("неполная запись: %+v", f)
		}
	}
}

func TestLoadFamiliesRejectsNone(t *testing.T) {
	// ровно 14 записей, но одна — словарный резерв none.
	names := []string{
		"escape_liberation", "judgment_trial", "loyalty", "betrayal",
		"inheritance_return", "endurance_suffering", "rescue", "rise_fall_rise",
		"wisdom_counsel", "political_intrigue", "quest_journey", "temptation_fall",
		"prophetic_oracle", "none",
	}
	_, err := LoadFamilies(writeFamilies(t, names))
	if err == nil {
		t.Fatal("резерв none не отвергнут")
	}
	if !strings.Contains(err.Error(), "none") {
		t.Fatalf("ошибка без упоминания none: %v", err)
	}
}

func TestLoadFamiliesRejectsDuplicate(t *testing.T) {
	names := []string{
		"a", "b", "c", "d", "e", "f", "g",
		"h", "i", "j", "k", "l", "m", "a",
	}
	_, err := LoadFamilies(writeFamilies(t, names))
	if err == nil {
		t.Fatal("дубль имени не отвергнут")
	}
	if !strings.Contains(err.Error(), "дубль") {
		t.Fatalf("ошибка без упоминания дубля: %v", err)
	}
}

func TestLoadFamiliesRejectsWrongCount(t *testing.T) {
	_, err := LoadFamilies(writeFamilies(t, []string{"escape_liberation"}))
	if err == nil {
		t.Fatal("реестр не из 14 семей не отвергнут")
	}
	if !strings.Contains(err.Error(), "want 14") {
		t.Fatalf("ошибка без want 14: %v", err)
	}
}

func TestFamiliesPath(t *testing.T) {
	got := FamiliesPath("engine")
	want := filepath.Join("engine", "base", "archetypes.json")
	if got != want {
		t.Fatalf("FamiliesPath: %q, want %q", got, want)
	}
}

func writeFamilies(t *testing.T, names []string) string {
	t.Helper()
	fams := make([]Family, len(names))
	for i, n := range names {
		fams[i] = Family{Name: n, GlossEN: "Gloss", Keywords: []string{"kw"}}
	}
	b, err := json.Marshal(fams)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "archetypes.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}
