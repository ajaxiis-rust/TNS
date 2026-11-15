package webapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/journal"
	web "tns65/web"
)

// diskWebDefaults — содержимое поставки фронта на диске для тестов.
var diskWebDefaults = map[string]string{
	"index.html":     "<html>disk index</html>",
	"atlas.html":     "<html>disk atlas</html>",
	"history.html":   "<html>disk history</html>",
	"mcp.html":       "<html>disk mcp</html>",
	"settings.html":  "<html>disk settings</html>",
	"wizard.html":    "<html>disk wizard</html>",
	"static/app.js":  "// disk app",
	"static/app.css": "body {}",
}

// writeDiskWebFile кладёт файл фронта в каталог web/ по slash-пути.
func writeDiskWebFile(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// writeDiskWeb раскладывает полный каталог web/ на диске, перекрывая файлы из
// overrides (имя → содержимое).
func writeDiskWeb(t *testing.T, overrides map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{}
	for name, body := range diskWebDefaults {
		files[name] = body
	}
	for name, body := range overrides {
		files[name] = body
	}
	for name, body := range files {
		writeDiskWebFile(t, dir, name, body)
	}
	return dir
}

// pinDiskWebManifest пинит sha256 всех файлов каталога web/ (как build.sh);
// overrides подменяет отдельные пины.
func pinDiskWebManifest(t *testing.T, dir string, overrides map[string]string) {
	t.Helper()
	files := map[string]string{}
	err := fs.WalkDir(os.DirFS(dir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path == "manifest.json" {
			return err
		}
		raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		files[path] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("сверка каталога web/: %v", err)
	}
	for name, sha := range overrides {
		files[name] = sha
	}
	raw, err := json.Marshal(webManifest{ID: "web", Files: files})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	writeDiskWebFile(t, dir, "manifest.json", string(raw))
}

// staticJournalRecord — строка журнала с массивом fields.
type staticJournalRecord struct {
	Event  string          `json:"event"`
	Fields []journal.Field `json:"fields"`
}

func (r staticJournalRecord) field(key string) string {
	for _, f := range r.Fields {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}

// readStaticJournal читает все события журнала станка.
func readStaticJournal(t *testing.T, path string) []staticJournalRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var out []staticJournalRecord
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var rec staticJournalRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("bad journal line %q: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

// newStaticJournal заводит журнал во временном каталоге и возвращает его путь.
func newStaticJournal(t *testing.T) (string, *journal.Logger) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "engine.log")
	logger, err := journal.New(path)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	return path, logger
}

// staticSourceOf возвращает значение поля source события web.static.source.
func staticSourceOf(t *testing.T, recs []staticJournalRecord) string {
	t.Helper()
	for _, rec := range recs {
		if rec.Event == journal.EventWebStaticSource {
			return rec.field("source")
		}
	}
	return ""
}

// staticGet выполняет GET к обработчику webapi без сети.
func staticGet(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	return rr
}

// TestStaticDiskFirst — IMP-12, шаг 1/4: полный каталог web/ на диске
// перекрывает встроенный фронт — страницы и static/* отдаются с диска, в
// журнале источник disk.
func TestStaticDiskFirst(t *testing.T) {
	logPath, logger := newStaticJournal(t)
	dir := writeDiskWeb(t, nil)
	pinDiskWebManifest(t, dir, nil)

	h := NewHandler(Config{WebDir: dir, Logger: logger})
	for _, tc := range []struct{ path, want string }{
		{"/", diskWebDefaults["index.html"]},
		{"/static/app.css", diskWebDefaults["static/app.css"]},
		{"/settings.html", diskWebDefaults["settings.html"]},
		{"/history.html", diskWebDefaults["history.html"]},
	} {
		rr := staticGet(t, h, tc.path)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200; body=%s", tc.path, rr.Code, rr.Body.String())
		}
		if rr.Body.String() != tc.want {
			t.Fatalf("GET %s: отдан не файл диска: %q", tc.path, rr.Body.String())
		}
		if got := rr.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("GET %s: Cache-Control = %q, want no-cache", tc.path, got)
		}
	}

	recs := readStaticJournal(t, logPath)
	if got := staticSourceOf(t, recs); got != staticModeDisk {
		t.Fatalf("web.static.source = %q, want %q", got, staticModeDisk)
	}
	for _, rec := range recs {
		if rec.Event == journal.EventWebStaticDrift {
			t.Fatalf("манифест сходится, а в журнале дрейф файла %q", rec.field("file"))
		}
	}
}

// TestStaticEmbedFallback — IMP-12, шаг 1/4: каталога web/ нет — фронт отдаётся
// из бинарника как раньше, в журнале источник embed.
func TestStaticEmbedFallback(t *testing.T) {
	logPath, logger := newStaticJournal(t)
	missing := filepath.Join(t.TempDir(), "web")

	h := NewHandler(Config{WebDir: missing, Logger: logger})

	wantIndex, err := web.FS.ReadFile("index.html")
	if err != nil {
		t.Fatalf("embed index: %v", err)
	}
	wantJS, err := web.FS.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("embed app.js: %v", err)
	}
	for _, tc := range []struct {
		path string
		want []byte
	}{
		{"/", wantIndex},
		{"/static/app.js", wantJS},
	} {
		rr := staticGet(t, h, tc.path)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200", tc.path, rr.Code)
		}
		if rr.Body.String() != string(tc.want) {
			t.Fatalf("GET %s: отдан не встроенный файл", tc.path)
		}
		if got := rr.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("GET %s: Cache-Control = %q, want no-cache", tc.path, got)
		}
	}

	if got := staticSourceOf(t, readStaticJournal(t, logPath)); got != staticModeEmbed {
		t.Fatalf("web.static.source = %q, want %q", got, staticModeEmbed)
	}
}

// TestStaticIncompleteDirFallsBack — IMP-12: неполный каталог web/ (оборванный
// rsync) источником не считается: фронт из embed, в журнале первый недостающий
// файл.
func TestStaticIncompleteDirFallsBack(t *testing.T) {
	logPath, logger := newStaticJournal(t)
	dir := t.TempDir()
	writeDiskWebFile(t, dir, "index.html", "<html>only index</html>")

	h := NewHandler(Config{WebDir: dir, Logger: logger})

	wantIndex, err := web.FS.ReadFile("index.html")
	if err != nil {
		t.Fatalf("embed index: %v", err)
	}
	rr := staticGet(t, h, "/")
	if rr.Body.String() != string(wantIndex) {
		t.Fatalf("неполный каталог отдал свой index.html: %q", rr.Body.String())
	}

	var source, missing string
	for _, rec := range readStaticJournal(t, logPath) {
		if rec.Event == journal.EventWebStaticSource {
			source, missing = rec.field("source"), rec.field("missing")
		}
	}
	if source != staticModeEmbed {
		t.Fatalf("web.static.source = %q, want %q", source, staticModeEmbed)
	}
	if missing != "atlas.html" {
		t.Fatalf("web.static.source missing = %q, want atlas.html", missing)
	}
}

// TestStaticManifestDrift — IMP-12, шаг 2: файл, правленный после сборки, всё
// равно раздаётся, но в журнале есть web.static.drift с файлом и хэшами.
func TestStaticManifestDrift(t *testing.T) {
	logPath, logger := newStaticJournal(t)
	dir := writeDiskWeb(t, nil)
	pinDiskWebManifest(t, dir, nil)

	// Фронт на станции новее бинаря: правка после манифеста.
	pinned := sha256.Sum256([]byte(diskWebDefaults["static/app.css"]))
	edited := "body { color: red; }"
	writeDiskWebFile(t, dir, "static/app.css", edited)

	h := NewHandler(Config{WebDir: dir, Logger: logger})
	rr := staticGet(t, h, "/static/app.css")
	if rr.Code != http.StatusOK || rr.Body.String() != edited {
		t.Fatalf("GET /static/app.css: status=%d body=%q, want правленый файл", rr.Code, rr.Body.String())
	}

	recs := readStaticJournal(t, logPath)
	var drift *staticJournalRecord
	for i := range recs {
		if recs[i].Event == journal.EventWebStaticDrift && recs[i].field("file") == "static/app.css" {
			drift = &recs[i]
			break
		}
	}
	if drift == nil {
		t.Fatal("в журнале нет события web.static.drift про static/app.css")
	}
	if drift.field("want") != hex.EncodeToString(pinned[:]) {
		t.Fatalf("web.static.drift want = %q, want пин манифеста", drift.field("want"))
	}
	sum := sha256.Sum256([]byte(edited))
	if drift.field("got") != hex.EncodeToString(sum[:]) {
		t.Fatalf("web.static.drift got = %q, want хэш правленого файла", drift.field("got"))
	}
}

// TestStaticDefaultDirIsBinaryDir — IMP-12: каталог не задан — web/ ищется
// рядом с бинарником, и источник совпадает с явно указанным таким каталогом.
func TestStaticDefaultDirIsBinaryDir(t *testing.T) {
	_, logger := newStaticJournal(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	want := resolveStatic(filepath.Join(filepath.Dir(exe), "web"), logger).mode
	if got := resolveStatic("", logger).mode; got != want {
		t.Fatalf("resolveStatic(\"\").mode = %q, want %q (web/ рядом с бинарником)", got, want)
	}
}
