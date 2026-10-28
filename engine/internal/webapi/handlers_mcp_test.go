package webapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/paganel"
	"tns65/internal/swarm"
	"tns65/internal/workshop"
)

// seedMcpWorld готовит готовый мир alpha: оперативная база worlds/alpha/alpha.db
// и литературная worlds/alpha/literary.db в t.TempDir(), подменяет mcpRoot,
// worldDBPath и litDBPath на реальную раскладку.
func seedMcpWorld(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	worldDir := filepath.Join(root, "worlds", "alpha")
	if err := os.MkdirAll(worldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	worldPath := filepath.Join(worldDir, "alpha.db")
	litPath := filepath.Join(worldDir, "literary.db")

	raw, err := db.OpenWriter(worldPath)
	if err != nil {
		t.Fatalf("OpenWriter(world): %v", err)
	}
	if err := db.Migrate(raw, "world", "shared"); err != nil {
		_ = raw.Close()
		t.Fatalf("Migrate(world, shared): %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("Close(world raw): %v", err)
	}

	litRaw, err := db.OpenWriter(litPath)
	if err != nil {
		t.Fatalf("OpenWriter(literary): %v", err)
	}
	if err := db.Migrate(litRaw, "literary"); err != nil {
		_ = litRaw.Close()
		t.Fatalf("Migrate(literary): %v", err)
	}
	if err := litRaw.Close(); err != nil {
		t.Fatalf("Close(literary raw): %v", err)
	}

	oldRoot := mcpRoot
	mcpRoot = root
	oldWorld := worldDBPath
	worldDBPath = func(slug string) string {
		return filepath.Join(root, "worlds", slug, slug+".db")
	}
	oldLit := litDBPath
	litDBPath = func(slug string) string {
		return filepath.Join(root, "worlds", slug, "literary.db")
	}
	oldLaunch := forgeLaunch
	oldLogger := mcpLogger
	oldHub := mcpHub
	t.Cleanup(func() {
		mcpRoot = oldRoot
		worldDBPath = oldWorld
		litDBPath = oldLit
		forgeLaunch = oldLaunch
		mcpLogger = oldLogger
		mcpHub = oldHub
	})
}

func mcpRequest(t *testing.T, h http.HandlerFunc, method, path string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

// multipartImport строит multipart-запрос импорта книги.
func multipartImport(t *testing.T, slug, filename, content string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("world", slug); err != nil {
		t.Fatal(err)
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return mcpRequest(t, handleMcpImport, http.MethodPost, mcpImportPath, buf.Bytes(), mw.FormDataContentType())
}

// writeTestManifest пишет манифест сырья мира alpha поверх текущего.
func writeTestManifest(t *testing.T, items []sourceManifest) {
	t.Helper()
	if err := writeSourceManifest("alpha", items); err != nil {
		t.Fatalf("writeSourceManifest: %v", err)
	}
}

// seedFactMismatch вставляет реальную строку журнала ковки fact_check/mismatch
// (единственный producer записей forge-book в workshop/forge_pipeline.go).
func seedFactMismatch(t *testing.T) {
	t.Helper()
	lit, err := db.OpenWriterAs(litDBPath("alpha"), db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer lit.Close()
	if _, err := lit.Exec("forge_log",
		`INSERT INTO forge_log (kind, task, status, started_at, finished_at, package_version) VALUES ('forge-book', 'fact_check', 'mismatch', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 'dev')`); err != nil {
		t.Fatalf("INSERT forge_log fact_check: %v", err)
	}
}

// TestForgeDash — «Проверка» T-08-08: дашборд ковки — очередь в штуках, процент
// текущей работы и статистика из реальных источников (манифест сырья,
// lit_template, forge_log fact_check/mismatch).
func TestForgeDash(t *testing.T) {
	seedMcpWorld(t)

	// Очередь: одна книга ждёт ковки; одна выкована с тремя главами; одна с
	// ошибкой. Счётчики цензуры и делексификации живут в карточках сырья.
	writeTestManifest(t, []sourceManifest{
		{ID: "queued.txt", Name: "queued.txt", Format: "txt", Status: sourceStatusQueued},
		{ID: "done.txt", Name: "done.txt", Format: "txt", Status: sourceStatusDone, Progress: 1, Chapters: 3, CensorRejects: 1, DelexReplaces: 2},
		{ID: "bad.txt", Name: "bad.txt", Format: "txt", Status: sourceStatusError},
	})

	core, err := db.OpenWriterAs(worldDBPath("alpha"), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world core): %v", err)
	}
	// Текущая работа: кольцо running с прогрессом 0.4.
	if _, err := core.Exec("rings",
		`INSERT INTO rings (status, progress, budget_used, started_at, places_count, paths_count, people_count)
		 VALUES ('running', 0.4, 0, '2026-01-01T00:00:00Z', 0, 0, 0)`); err != nil {
		t.Fatalf("INSERT rings: %v", err)
	}
	if err := core.Close(); err != nil {
		t.Fatalf("Close(world core): %v", err)
	}

	lit, err := db.OpenWriterAs(litDBPath("alpha"), db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	if _, err := lit.Exec("lit_template",
		`INSERT INTO lit_template (kind, text, lang, tags) VALUES ('scene', 'шаблон', 'ru', '')`); err != nil {
		t.Fatalf("INSERT lit_template: %v", err)
	}
	if err := lit.Close(); err != nil {
		t.Fatalf("Close(literary): %v", err)
	}
	seedFactMismatch(t)

	rr := mcpRequest(t, handleMcpDash, http.MethodGet, mcpDashPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", mcpDashPath, rr.Code, rr.Body.String())
	}
	var out mcpDashResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.World != "alpha" {
		t.Fatalf("world = %q, want alpha", out.World)
	}
	if out.Queue != 1 {
		t.Fatalf("queue = %d, want 1", out.Queue)
	}
	if out.Current != 0.4 {
		t.Fatalf("current = %v, want 0.4", out.Current)
	}
	if out.Stats.BooksProcessed != 1 {
		t.Fatalf("books_processed = %d, want 1", out.Stats.BooksProcessed)
	}
	if out.Stats.ChaptersParsed != 3 {
		t.Fatalf("chapters_parsed = %d, want 3", out.Stats.ChaptersParsed)
	}
	if out.Stats.Templates != 1 {
		t.Fatalf("templates = %d, want 1", out.Stats.Templates)
	}
	if out.Stats.CensorRejects != 1 {
		t.Fatalf("censor_rejects = %d, want 1", out.Stats.CensorRejects)
	}
	if out.Stats.DelexReplaces != 2 {
		t.Fatalf("delex_replaces = %d, want 2", out.Stats.DelexReplaces)
	}
	if out.Stats.Errors != 2 {
		t.Fatalf("errors = %d, want 2 (карточка error + fact_check/mismatch)", out.Stats.Errors)
	}

	// Пустой слаг — 400; мира нет — 404.
	if rr := mcpRequest(t, handleMcpDash, http.MethodGet, mcpDashPath, nil, ""); rr.Code != http.StatusBadRequest {
		t.Fatalf("GET без слага: status = %d, want 400", rr.Code)
	}
	if rr := mcpRequest(t, handleMcpDash, http.MethodGet, mcpDashPath+"?world=nope", nil, ""); rr.Code != http.StatusNotFound {
		t.Fatalf("GET несуществующего мира: status = %d, want 404", rr.Code)
	}
}

// TestBookImport — «Проверка» T-08-08: импорт txt/md/fb2, повторный импорт того
// же файла — отказ, дублей нет; запуск ковки идёт однозадачным запуском через
// рой (ForgeBookSpec → forgeLaunch), а не заказом очереди мастерской.
func TestBookImport(t *testing.T) {
	seedMcpWorld(t)

	rr := multipartImport(t, "alpha", "book.txt", "Глава 1\nтекст")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST import txt: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out mcpSourcesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.World != "alpha" || len(out.Sources) != 1 {
		t.Fatalf("world/sources = %q/%d, want alpha/1", out.World, len(out.Sources))
	}
	s := out.Sources[0]
	if s.Name != "book.txt" || s.Format != "txt" || s.Status != sourceStatusQueued {
		t.Fatalf("source = %+v, want book.txt/txt/queued", s)
	}
	if s.Chapters != 1 {
		t.Fatalf("глав после импорта = %d, want 1", s.Chapters)
	}
	if _, err := os.Stat(filepath.Join(mcpSourcesDir("alpha"), "book.txt")); err != nil {
		t.Fatalf("файл сырья не сохранён: %v", err)
	}

	// Повторный импорт того же файла — 409, дублей нет.
	rr = multipartImport(t, "alpha", "book.txt", "другой текст")
	if rr.Code != http.StatusConflict {
		t.Fatalf("повторный импорт: status = %d, want 409; body=%s", rr.Code, rr.Body.String())
	}
	items, err := readSourceManifest("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("после дубля источников = %d, want 1", len(items))
	}

	// Неподдержанный формат — 400 с причиной.
	rr = multipartImport(t, "alpha", "book.epub", "текст")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("импорт epub: status = %d, want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "txt") || !strings.Contains(rr.Body.String(), "fb2") {
		t.Fatalf("причина отказа не называет txt/fb2: %s", rr.Body.String())
	}

	// Запуск ковки — реальный однозадачный запуск через рой, а не строка
	// workshop_queue. Фейковый лаунчер фиксирует Spec и возвращает успех.
	var gotSpec *swarm.Spec
	forgeLaunch = func(_ *journal.Logger, spec swarm.Spec, coreVersion string) (swarm.Result, error) {
		gotSpec = &spec
		return swarm.Result{PID: 1, Output: []byte("{}")}, nil
	}
	body, _ := json.Marshal(map[string]string{"world": "alpha"})
	rr = mcpRequest(t, handleMcpForge, http.MethodPost, mcpForgePath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST forge: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var forged mcpSourcesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &forged); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if forged.Sources[0].Status != sourceStatusDone {
		t.Fatalf("status после ковки = %q, want done", forged.Sources[0].Status)
	}
	if gotSpec == nil {
		t.Fatal("forgeLaunch не вызван")
	}
	if gotSpec.Name != workshop.ForgeBookRole {
		t.Fatalf("Spec.Name = %q, want %q", gotSpec.Name, workshop.ForgeBookRole)
	}
	if len(gotSpec.Args) != 3 || gotSpec.Args[0] != "swarm-task" || gotSpec.Args[1] != workshop.ForgeBookRole {
		t.Fatalf("Spec.Args = %v, want [swarm-task forge-book <payload>]", gotSpec.Args)
	}
	var payload workshop.ForgeBookPayload
	if err := json.Unmarshal([]byte(gotSpec.Args[2]), &payload); err != nil {
		t.Fatalf("payload не JSON: %v", err)
	}
	if payload.WorldSlug != "alpha" || payload.InputPath != filepath.Join(mcpSourcesDir("alpha"), "book.txt") {
		t.Fatalf("payload = %+v, want world alpha и путь сырья book.txt", payload)
	}
	// Очередь мастерской ковкой не трогается.
	world, err := db.OpenReaderAs(worldDBPath("alpha"), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenReaderAs(world): %v", err)
	}
	defer world.Close()
	var queued int
	if err := world.QueryRow(`SELECT COUNT(*) FROM workshop_queue`).Scan(&queued); err != nil {
		t.Fatalf("COUNT workshop_queue: %v", err)
	}
	if queued != 0 {
		t.Fatalf("строк workshop_queue = %d, want 0 (ковка через рой)", queued)
	}

	// Удаление сырья удаляет файл, выкованное в базе не трогает (базы нет записи
	// о сырье — проверяем только файл и манифест).
	rr = mcpRequest(t, handleMcpSourceDelete, http.MethodDelete, mcpSourcePath+"/book.txt?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE source: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(mcpSourcesDir("alpha"), "book.txt")); !os.IsNotExist(err) {
		t.Fatalf("файл сырья после удаления остался: %v", err)
	}
	items, err = readSourceManifest("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("после удаления источников = %d, want 0", len(items))
	}

	// Удаление несуществующего источника — 404.
	rr = mcpRequest(t, handleMcpSourceDelete, http.MethodDelete, mcpSourcePath+"/nope?world=alpha", nil, "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("DELETE несуществующего: status = %d, want 404", rr.Code)
	}
}

// TestRingsCard — «Проверка» T-08-08: журнал колец Паганеля — текущее кольцо,
// очередь, счётчики мест/путей/людей и статистика draft → baked.
func TestRingsCard(t *testing.T) {
	seedMcpWorld(t)

	core, err := db.OpenWriterAs(worldDBPath("alpha"), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(core): %v", err)
	}
	// Текущее кольцо running с прогрессом и счётчиками, очередь из двух колец.
	if _, err := core.Exec("rings",
		`INSERT INTO rings (status, progress, budget_used, started_at, places_count, paths_count, people_count)
		 VALUES ('running', 0.4, 0, '2026-01-01T00:00:00Z', 3, 2, 5),
		        ('queued', 0, 0, '', 0, 0, 0),
		        ('queued', 0, 0, '', 0, 0, 0)`); err != nil {
		t.Fatalf("INSERT rings: %v", err)
	}
	if err := core.Close(); err != nil {
		t.Fatalf("Close(core): %v", err)
	}

	forge, err := db.OpenWriterAs(worldDBPath("alpha"), db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	for _, s := range []struct{ name, status string }{
		{"лес", "draft"}, {"река", "draft"}, {"город", "baked"},
	} {
		if _, err := forge.Exec("places",
			`INSERT INTO places (name, kind, origin, status) VALUES (?, 'место', 'paganel', ?)`,
			s.name, s.status); err != nil {
			t.Fatalf("INSERT places: %v", err)
		}
	}
	for i, status := range []string{"draft", "baked", "baked"} {
		if _, err := forge.Exec("paths",
			`INSERT INTO paths (from_place_id, to_place_id, type, length_km, friction, origin, status)
			 VALUES (?, ?, 'trakt', 10, 1, 'paganel', ?)`,
			i+1, i+2, status); err != nil {
			t.Fatalf("INSERT paths: %v", err)
		}
	}
	if err := forge.Close(); err != nil {
		t.Fatalf("Close(forge): %v", err)
	}

	rr := mcpRequest(t, handleMcpRings, http.MethodGet, mcpRingsPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", mcpRingsPath, rr.Code, rr.Body.String())
	}
	var out struct {
		World   string              `json:"world"`
		Journal paganel.RingJournal `json:"journal"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.World != "alpha" {
		t.Fatalf("world = %q, want alpha", out.World)
	}
	j := out.Journal
	if j.Current.Status != paganel.RingRunning || j.Current.Progress <= 0 {
		t.Fatalf("current = %+v, want running с progress>0", j.Current)
	}
	if j.Current.PlacesCount != 3 || j.Current.PathsCount != 2 || j.Current.PeopleCount != 5 {
		t.Fatalf("счётчики current = (%d,%d,%d), want (3,2,5)",
			j.Current.PlacesCount, j.Current.PathsCount, j.Current.PeopleCount)
	}
	if len(j.Queue) != 2 {
		t.Fatalf("queue = %d, want 2", len(j.Queue))
	}
	if j.Forge.PlacesDraft != 2 || j.Forge.PlacesBaked != 1 {
		t.Fatalf("статистика мест = (%d draft, %d baked), want (2,1)",
			j.Forge.PlacesDraft, j.Forge.PlacesBaked)
	}
	if j.Forge.PathsDraft != 1 || j.Forge.PathsBaked != 2 {
		t.Fatalf("статистика путей = (%d draft, %d baked), want (1,2)",
			j.Forge.PathsDraft, j.Forge.PathsBaked)
	}
	// Полей переключателей в контракте нет.
	if strings.Contains(rr.Body.String(), "enabled") {
		t.Fatalf("в контракте колец есть поле переключателя: %s", rr.Body.String())
	}

	// Пустой слаг — 400; мира нет — 404.
	if rr := mcpRequest(t, handleMcpRings, http.MethodGet, mcpRingsPath, nil, ""); rr.Code != http.StatusBadRequest {
		t.Fatalf("GET без слага: status = %d, want 400", rr.Code)
	}
	if rr := mcpRequest(t, handleMcpRings, http.MethodGet, mcpRingsPath+"?world=nope", nil, ""); rr.Code != http.StatusNotFound {
		t.Fatalf("GET несуществующего мира: status = %d, want 404", rr.Code)
	}
}
