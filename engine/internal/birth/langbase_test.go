package birth

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"tns65/internal/db"
	"tns65/internal/models"
)

// writeLangBaseDB создаёт dir/base.<lang>.db с одним нарративом и вектором
// dim=1024 языка lang — фикстура поставки для тестов T-10-03.
func writeLangBaseDB(t *testing.T, dir, lang string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "base."+lang+".db")
	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := db.Migrate(w, "base", "shared"); err != nil {
		t.Fatal(err)
	}
	vec := make([]float32, models.EmbedDim)
	vec[0] = 1
	res, err := w.Exec(`INSERT INTO narratives (name, arc_summary, lang, created_at) VALUES (?,?,?,?)`,
		"n1", "arc 1", lang, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec(`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		"narratives", id, "bge-m3", models.EmbedDim, lang, "arc 1", float32Blob(vec), "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	return path
}

func withBaseRoot(t *testing.T, dir string) {
	t.Helper()
	old := db.BaseRoot
	db.BaseRoot = dir
	t.Cleanup(func() { db.BaseRoot = old })
}

func TestAvailableLangs(t *testing.T) {
	dir := t.TempDir()
	writeLangBaseDB(t, dir, "en")
	writeLangBaseDB(t, dir, "ru")
	// Легаси-слитая base.db и служебные файлы в список не входят.
	if err := os.WriteFile(filepath.Join(dir, "base.db"), []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "base.en.db.sha256"), []byte("deadbeef\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	withBaseRoot(t, dir)
	got, err := AvailableLangs()
	if err != nil {
		t.Fatalf("AvailableLangs: %v", err)
	}
	want := []string{"en", "ru"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AvailableLangs = %v, want %v", got, want)
	}
}

func TestValidateLangFields(t *testing.T) {
	dir := t.TempDir()
	writeLangBaseDB(t, dir, "en")
	withBaseRoot(t, dir)

	a := validAncketa()

	a.Lang = ""
	if fe := ValidateLang(a); fe == nil || fe.Field != "lang" || fe.Reason != "язык не указан" {
		t.Fatalf("пустой язык: %+v, want Field=lang Reason=«язык не указан»", fe)
	}

	a = validAncketa()
	a.Lang = "klingon"
	if fe := ValidateLang(a); fe == nil || fe.Field != "lang" || fe.Reason != "язык не установлен: нет базы base.klingon.db" {
		t.Fatalf("klingon: %+v, want Field=lang Reason=«язык не установлен: нет базы base.klingon.db»", fe)
	}

	a = validAncketa()
	a.Lang = "en"
	if fe := ValidateLang(a); fe != nil {
		t.Fatalf("en: %+v, want nil", fe)
	}
}

func TestValidateLangExplicitPath(t *testing.T) {
	a := validAncketa()
	a.Lang = "ru"
	cfg := Config{BaseDBPath: filepath.Join(t.TempDir(), "missing.db")}
	fe := validateLangForConfig(a, cfg)
	if fe == nil || fe.Field != "lang" || fe.Reason != "база языка не найдена: "+cfg.BaseDBPath {
		t.Fatalf("явный путь без файла: %+v", fe)
	}
}
