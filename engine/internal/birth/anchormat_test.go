package birth

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/models"
	"tns65/internal/testkit"
)

// anchorMaterialMeta — содержимое четырёх ключей world_meta сценарной рамки.
type anchorMaterialMeta struct {
	name     string
	summary  string
	verses   []string
	families []string
}

func TestAnchorMaterial(t *testing.T) {
	// Фикстура с 3 verses и 2 разными kind → четыре ключа с ожидаемым содержимым.
	dir := t.TempDir()
	verses := []string{"Первый кусок.", "Второй кусок.", "Третий кусок."}
	kinds := []string{"escape_liberation", "judgment_trial"}
	baseDB := writeAnchorMaterialBaseDB(t, dir, "ru", verses, kinds)

	res, err := Birth(validAncketa(), Config{WorldsRoot: t.TempDir(), BaseDBPath: baseDB, Embed: testkit.NewFakeEmbed()})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}
	if st, err := readState(res.Dir); err != nil || st != stateReady {
		t.Fatalf("состояние мира: %q (%v), want %q", st, err, stateReady)
	}

	meta := readAnchorMaterialMeta(t, res.WorldDB)
	if meta.name != "Наследие и возвращение" {
		t.Fatalf("anchor_name = %q, want «Наследие и возвращение»", meta.name)
	}
	if meta.summary != "Герой возвращается, чтобы изменить судьбу." {
		t.Fatalf("anchor_summary = %q", meta.summary)
	}
	if !reflect.DeepEqual(meta.verses, verses) {
		t.Fatalf("anchor_verses = %v, want %v", meta.verses, verses)
	}
	if !reflect.DeepEqual(meta.families, kinds) {
		t.Fatalf("anchor_families = %v, want %v", meta.families, kinds)
	}

	// Фикстура без patterns → anchor_families = [] (деградация без отказа).
	dir2 := t.TempDir()
	baseDB2 := writeAnchorMaterialBaseDB(t, dir2, "ru", []string{"Один кусок."}, nil)
	res2, err := Birth(validAncketa(), Config{WorldsRoot: t.TempDir(), BaseDBPath: baseDB2, Embed: testkit.NewFakeEmbed()})
	if err != nil {
		t.Fatalf("Birth без patterns: %v", err)
	}
	if st, err := readState(res2.Dir); err != nil || st != stateReady {
		t.Fatalf("состояние мира без patterns: %q (%v), want %q", st, err, stateReady)
	}
	if raw := metaValue(t, openReader(t, res2.WorldDB), anchorFamiliesKey); raw != "[]" {
		t.Fatalf("anchor_families без patterns = %q, want []", raw)
	}
}

// TestAnchorMaterialMissingNarrative — битая база: вектор есть, строки narratives
// нет — рождение обязано отказать с внятной ошибкой (T-10-06, шаг 4).
func TestAnchorMaterialMissingNarrative(t *testing.T) {
	dir := t.TempDir()
	baseDB := writeMissingNarrativeBaseDB(t, dir, "ru")

	worldsRoot := t.TempDir()
	a := validAncketa()
	_, err := Birth(a, Config{WorldsRoot: worldsRoot, BaseDBPath: baseDB, Embed: testkit.NewFakeEmbed()})
	if err == nil {
		t.Fatal("рождение с битой базой не вернуло ошибку")
	}
	if !strings.Contains(err.Error(), "якорный нарратив 1 не найден в базе языка") {
		t.Fatalf("причина: %q, want содержит «якорный нарратив 1 не найден в базе языка»", err.Error())
	}
	worldDir := filepath.Join(worldsRoot, "worlds", a.Slug)
	if _, statErr := os.Stat(worldDir); !os.IsNotExist(statErr) {
		t.Fatalf("каталог мира создан при битой базе: %v", statErr)
	}
}

// writeAnchorMaterialBaseDB создаёт base.<lang>.db с одним нарративом, его
// вектором dim=1024, verses и patterns — фикстура материализации якоря.
func writeAnchorMaterialBaseDB(t *testing.T, dir, lang string, verses, kinds []string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "base."+lang+".db")
	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(w, "base", "shared"); err != nil {
		_ = w.Close()
		t.Fatal(err)
	}
	vec := make([]float32, models.EmbedDim)
	vec[0] = 1
	res, err := w.Exec(`INSERT INTO narratives (name, arc_summary, lang, created_at) VALUES (?,?,?,?)`,
		"Наследие и возвращение", "Герой возвращается, чтобы изменить судьбу.", lang, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec(`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		"narratives", id, "bge-m3", models.EmbedDim, lang, "arc", float32Blob(vec), "2026-01-01T00:00:00Z"); err != nil {
		_ = w.Close()
		t.Fatal(err)
	}
	for i, text := range verses {
		if _, err := w.Exec(`INSERT INTO verses (narrative_id, text, lang, ord) VALUES (?,?,?,?)`,
			id, text, lang, i+1); err != nil {
			_ = w.Close()
			t.Fatal(err)
		}
	}
	for _, kind := range kinds {
		if _, err := w.Exec(`INSERT INTO patterns (narrative_id, kind, text, lang) VALUES (?,?,?,?)`,
			id, kind, "описание дуги", lang); err != nil {
			_ = w.Close()
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	writeFixtureSHA256(t, path)
	return path
}

// writeMissingNarrativeBaseDB создаёт base.<lang>.db с вектором, но без строки
// narratives — фикстура битой базы (T-10-06, шаг 4).
func writeMissingNarrativeBaseDB(t *testing.T, dir, lang string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "base."+lang+".db")
	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(w, "base", "shared"); err != nil {
		_ = w.Close()
		t.Fatal(err)
	}
	vec := make([]float32, models.EmbedDim)
	vec[0] = 1
	if _, err := w.Exec(`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		"narratives", 1, "bge-m3", models.EmbedDim, lang, "arc", float32Blob(vec), "2026-01-01T00:00:00Z"); err != nil {
		_ = w.Close()
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	writeFixtureSHA256(t, path)
	return path
}

// readAnchorMaterialMeta читает четыре ключа world_meta сценарной рамки якоря.
func readAnchorMaterialMeta(t *testing.T, worldDB string) anchorMaterialMeta {
	t.Helper()
	d := openReader(t, worldDB)

	var m anchorMaterialMeta
	if err := json.Unmarshal([]byte(metaValue(t, d, anchorNameKey)), &m.name); err != nil {
		t.Fatalf("разбор anchor_name: %v", err)
	}
	if err := json.Unmarshal([]byte(metaValue(t, d, anchorSummaryKey)), &m.summary); err != nil {
		t.Fatalf("разбор anchor_summary: %v", err)
	}
	if err := json.Unmarshal([]byte(metaValue(t, d, anchorVersesKey)), &m.verses); err != nil {
		t.Fatalf("разбор anchor_verses: %v", err)
	}
	if err := json.Unmarshal([]byte(metaValue(t, d, anchorFamiliesKey)), &m.families); err != nil {
		t.Fatalf("разбор anchor_families: %v", err)
	}
	return m
}

func metaValue(t *testing.T, d *sql.DB, key string) string {
	t.Helper()
	var raw string
	if err := d.QueryRow(`SELECT value FROM world_meta WHERE "key" = ?`, key).Scan(&raw); err != nil {
		t.Fatalf("world_meta %s: %v", key, err)
	}
	return raw
}
