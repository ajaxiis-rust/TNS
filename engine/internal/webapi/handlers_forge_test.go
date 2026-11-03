package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"tns65/internal/birth"
	"tns65/internal/db"
	"tns65/internal/library"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// passCensor — словарный фильтр, пропускающий любой текст (Цензор отключён для
// ковки в тесте; см. workshop/sediment_run_test.go).
type passCensor struct{}

func (passCensor) Check(string) (bool, []string) { return true, nil }

// setupForge подменяет корень миров, полку и пути баз на временные фикстуры.
func setupForge(t *testing.T) string {
	t.Helper()
	dir := setupAtlas(t)
	oldLib := libraryRoot
	libraryRoot = dir
	oldWorld := worldDBPath
	worldDBPath = func(slug string) string { return filepath.Join(dir, "worlds", slug, slug+".db") }
	oldLit := litDBPath
	litDBPath = func(slug string) string { return filepath.Join(dir, "worlds", slug, "literary.db") }
	t.Cleanup(func() {
		libraryRoot = oldLib
		worldDBPath = oldWorld
		litDBPath = oldLit
	})
	return dir
}

// importShelfBook кладёт книгу на полку и возвращает её id.
func importShelfBook(t *testing.T, dir, name, content string) string {
	t.Helper()
	book, err := library.Import(filepath.Join(dir, "library"), name, []byte(content))
	if err != nil {
		t.Fatalf("Import(%s): %v", name, err)
	}
	return book.ID
}

// seedFanficWorld рождает мир-фанфик с книгой-источником полки.
func seedFanficWorld(t *testing.T, dir, slug, shelfID string) {
	t.Helper()
	a := atlasValidAncketa(slug)
	a.Genres = []string{"фанфик"}
	a.SourceBook = shelfID
	if _, err := birth.Birth(a, birth.Config{
		WorldsRoot: dir,
		BaseDBPath: filepath.Join(dir, "base.db"),
		LibraryDir: filepath.Join(dir, "library"),
		Embed:      testkit.NewFakeEmbed(),
	}); err != nil {
		t.Fatalf("Birth(fanfic %s): %v", slug, err)
	}
}

// installFakeForge ставит фейковые слот, эмбеддер и цензор — «фейковая ковка»
// «Проверки» T-12-14 без живых моделей.
func installFakeForge() func() {
	slot := testkit.NewFakeSlot()
	slot.SetResponse(workshop.ComposeRole,
		`{"family_primary":"","pattern_secondary":"","technique":"","moral_stance":"","inversion":false}`)
	workshop.SetSlot(slot)
	workshop.SetEmbedder(testkit.NewFakeEmbed())
	workshop.SetCensor(passCensor{})
	return func() {
		workshop.SetSlot(nil)
		workshop.SetEmbedder(nil)
		workshop.SetCensor(nil)
	}
}

func forgeRequest(t *testing.T, h http.Handler, slug string) *httptest.ResponseRecorder {
	t.Helper()
	return atlasRequest(t, h, http.MethodPost, atlasForgePath, `{"world":"`+slug+`"}`)
}

func languageMarker(t *testing.T, dir, slug string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(dir, "worlds", slug, languageReadyFile))
	return err == nil
}

func bankPoolCount(t *testing.T, dir, slug string) int {
	t.Helper()
	d, err := db.OpenReader(filepath.Join(dir, "worlds", slug, "literary.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM bank_pool`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestAtlasForgeNoFanfic — не фанфик: метка language_ready ставится сразу,
// предупреждений нет.
func TestAtlasForgeNoFanfic(t *testing.T) {
	dir := setupForge(t)
	seedWorld(t, dir, "plain-world", "Простой мир")

	h := NewHandler(Config{})
	rr := forgeRequest(t, h, "plain-world")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST %s: status = %d, want 200; body=%s", atlasForgePath, rr.Code, rr.Body.String())
	}
	var out forgeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if !out.LanguageReady || out.Progress != 1 {
		t.Fatalf("forge = %+v, want language_ready + progress 1", out)
	}
	if out.Warning != "" {
		t.Fatalf("warning = %q, want empty", out.Warning)
	}
	if !languageMarker(t, dir, "plain-world") {
		t.Fatal("метка language_ready не поставлена")
	}
}

// TestAtlasForgeFanfic — фанфик: фейковая ковка пишет записи банка и ставит
// language_ready до первого хода.
func TestAtlasForgeFanfic(t *testing.T) {
	dir := setupForge(t)
	reset := installFakeForge()
	defer reset()

	shelfID := importShelfBook(t, dir, "hp.txt",
		"Глава 1. Встреча\nТекст первой главы.\nГлава 2. Погоня\nТекст второй главы.\nГлава 3. Развязка\nТекст третьей главы.\n")
	seedFanficWorld(t, dir, "hp-world", shelfID)

	h := NewHandler(Config{})
	rr := forgeRequest(t, h, "hp-world")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST %s: status = %d, want 200; body=%s", atlasForgePath, rr.Code, rr.Body.String())
	}
	var out forgeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if !out.LanguageReady || out.Progress != 1 {
		t.Fatalf("forge = %+v, want language_ready + progress 1", out)
	}
	if out.Warning != "" {
		t.Fatalf("warning = %q, want empty", out.Warning)
	}
	if !languageMarker(t, dir, "hp-world") {
		t.Fatal("метка language_ready не поставлена")
	}
	if n := bankPoolCount(t, dir, "hp-world"); n != 3 {
		t.Fatalf("записей bank_pool = %d, want 3", n)
	}
}

// TestAtlasForgeNoModel — отказ ковки (нет модели): предупреждение, но мир
// открывается на сиде — language_ready всё равно ставится.
func TestAtlasForgeNoModel(t *testing.T) {
	dir := setupForge(t)
	shelfID := importShelfBook(t, dir, "hp.txt",
		"Глава 1. Встреча\nТекст первой главы.\n")
	seedFanficWorld(t, dir, "hp-world", shelfID)

	oldForge := forgeBookRun
	forgeBookRun = func(world, lit *db.Handle, inputPath string, progress func(done, total int)) error {
		return errors.New("нет модели")
	}
	t.Cleanup(func() { forgeBookRun = oldForge })

	h := NewHandler(Config{})
	rr := forgeRequest(t, h, "hp-world")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST %s: status = %d, want 200; body=%s", atlasForgePath, rr.Code, rr.Body.String())
	}
	var out forgeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if !out.LanguageReady {
		t.Fatalf("language_ready = false, want true (продолжение на сиде)")
	}
	if out.Warning == "" {
		t.Fatal("warning пуст, want предупреждение об отсутствии модели")
	}
	if out.Progress != 0 {
		t.Fatalf("progress = %v, want 0", out.Progress)
	}
	if !languageMarker(t, dir, "hp-world") {
		t.Fatal("метка language_ready не поставлена")
	}
}
