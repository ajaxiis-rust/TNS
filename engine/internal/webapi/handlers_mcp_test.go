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
	"unicode/utf8"

	textenc "golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	textunicode "golang.org/x/text/encoding/unicode"

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

// mcpJournalRecord — событие журнала станка с полями: проверки импорта книг
// (IMP-8) смотрят имя, формат, главы и исходную кодировку.
type mcpJournalRecord struct {
	Event  string          `json:"event"`
	Fields []journal.Field `json:"fields"`
}

// field возвращает значение поля события; нет поля — пустая строка.
func (r mcpJournalRecord) field(key string) string {
	for _, f := range r.Fields {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}

// seedMcpJournal поднимает журнал станка для импорта книг, возвращает путь файла.
func seedMcpJournal(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "journal.log")
	logger, err := journal.New(path)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	mcpLogger = logger
	return path
}

// readMcpJournal читает события журнала станка вместе с полями.
func readMcpJournal(t *testing.T, path string) []mcpJournalRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var out []mcpJournalRecord
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var rec mcpJournalRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("bad journal line %q: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

// encodeSource перекодирует фикстуру книги в кодировку источника.
func encodeSource(t *testing.T, enc textenc.Encoding, s string) string {
	t.Helper()
	out, err := enc.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return string(out)
}

// fb2Sample1251 — минимальная fb2-фикстура в windows-1251: две главы.
func fb2Sample1251(t *testing.T) string {
	t.Helper()
	const doc = `<?xml version="1.0" encoding="windows-1251"?>
<FictionBook>
<description><title-info><book-title>Сумрачный город</book-title>
<author><first-name>Иван</first-name><last-name>Петров</last-name></author>
</title-info></description>
<body>
<section><title><p>Глава 1. Встреча</p></title><p>Первый абзац первой главы.</p></section>
<section><title><p>Глава 2. Погоня</p></title><p>Первый абзац второй главы.</p></section>
</body>
</FictionBook>
`
	return encodeSource(t, charmap.Windows1251, doc)
}

// txtSample — русский текст txt-фикстур: две главы.
const txtSample = `Глава 1. Встреча
Она пришла в дом на краю города, когда стемнело, и долго стояла у окна.
Ветер гнал по улице пыль, но в комнате было тихо и тепло.

Глава 2. Погоня
Он не стал ждать: взял фонарь, вышел на двор и побежал к реке.
Там, за поворотом, кто-то шёл по берегу и оставлял следы на песке.
`

// TestBookImportCharset — «Проверка» IMP-8 (а): fb2 в windows-1251 при импорте
// перекодируется в utf-8, декларация переписана, карточка queued, главы
// посчитаны, исходная кодировка и метод уходят в журнал станка.
func TestBookImportCharset(t *testing.T) {
	seedMcpWorld(t)
	logPath := seedMcpJournal(t)

	rr := multipartImport(t, "alpha", "Флибустьер.fb2", fb2Sample1251(t))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST import fb2 1251: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out mcpSourcesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(out.Sources) != 1 {
		t.Fatalf("источников = %d, want 1", len(out.Sources))
	}
	card := out.Sources[0]
	if card.Status != sourceStatusQueued || card.Error != "" {
		t.Fatalf("карточка = %+v, want queued без ошибки", card)
	}
	if card.Chapters != 2 {
		t.Fatalf("глав в карточке = %d, want 2", card.Chapters)
	}

	raw, err := os.ReadFile(filepath.Join(mcpSourcesDir("alpha"), "Флибустьер.fb2"))
	if err != nil {
		t.Fatalf("ReadFile(source): %v", err)
	}
	if !utf8.Valid(raw) {
		t.Fatal("файл сырья после импорта не валидный utf-8")
	}
	if !strings.Contains(string(raw), `encoding="utf-8"`) || strings.Contains(string(raw), "windows-1251") {
		t.Fatalf("декларация fb2 не переписана на utf-8: %q", raw)
	}
	parsed, err := workshop.ParseBookInput(filepath.Join(mcpSourcesDir("alpha"), "Флибустьер.fb2"))
	if err != nil {
		t.Fatalf("ParseBookInput(imported fb2): %v", err)
	}
	if parsed.Meta.Title != "Сумрачный город" || len(parsed.Meta.Chapters) != 2 {
		t.Fatalf("разбор импортированного fb2: title=%q глав=%d, want Сумрачный город/2",
			parsed.Meta.Title, len(parsed.Meta.Chapters))
	}

	var found bool
	for _, rec := range readMcpJournal(t, logPath) {
		if rec.Event != journal.EventSourceImport || rec.field("name") != "Флибустьер.fb2" {
			continue
		}
		found = true
		if rec.field("format") != "fb2" || rec.field("chapters") != "2" {
			t.Fatalf("событие импорта: format=%q chapters=%q, want fb2/2",
				rec.field("format"), rec.field("chapters"))
		}
		if rec.field("encoding") != "windows-1251" || rec.field("method") != "декларация" {
			t.Fatalf("событие импорта: encoding=%q method=%q, want windows-1251/декларация",
				rec.field("encoding"), rec.field("method"))
		}
	}
	if !found {
		t.Fatal("в журнале нет события source.import про импортированный fb2")
	}
}

// TestBookImportBroken — «Проверка» IMP-8 (в): битый файл даёт карточку error с
// непустой причиной и событием source.error; файл неопределимой кодировки
// удаляется.
func TestBookImportBroken(t *testing.T) {
	seedMcpWorld(t)
	logPath := seedMcpJournal(t)

	rr := multipartImport(t, "alpha", "broken.fb2", `<?xml version="1.0" encoding="utf-8"?><FictionBook><body><section>`)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST import битого fb2: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out mcpSourcesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(out.Sources) != 1 || out.Sources[0].Status != sourceStatusError {
		t.Fatalf("карточка битого fb2 = %+v, want error", out.Sources)
	}
	if out.Sources[0].Error == "" {
		t.Fatal("у карточки error пустая причина")
	}

	// Кодировку txt выбрать нельзя — карточка error, файл удалён.
	rr = multipartImport(t, "alpha", "binary.txt", "\x80\x81\x82\x83")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST import binary.txt: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	var binary *sourceManifest
	for i := range out.Sources {
		if out.Sources[i].Name == "binary.txt" {
			binary = &out.Sources[i]
		}
	}
	if binary == nil || binary.Status != sourceStatusError || binary.Error == "" {
		t.Fatalf("карточка binary.txt = %+v, want error с причиной", binary)
	}
	if !strings.Contains(binary.Error, "CP866") {
		t.Fatalf("причина не перечисляет кандидатов кодировок: %q", binary.Error)
	}
	if _, err := os.Stat(filepath.Join(mcpSourcesDir("alpha"), "binary.txt")); !os.IsNotExist(err) {
		t.Fatalf("файл неопределимой кодировки не удалён: %v", err)
	}

	var importErrs, launchErrs int
	for _, rec := range readMcpJournal(t, logPath) {
		switch rec.Event {
		case journal.EventSourceError:
			if rec.field("name") == "" || rec.field("reason") == "" {
				t.Fatalf("событие source.error без имени или причины: %+v", rec.Fields)
			}
			importErrs++
		case journal.EventForgeLaunchFailed:
			launchErrs++
		}
	}
	if importErrs == 0 {
		t.Fatal("в журнале нет события source.error")
	}
	if launchErrs != 0 {
		t.Fatalf("импорт писал forge.launch_failed: %d", launchErrs)
	}
}

// TestBookImportTxtCharsets — «Проверка» IMP-8 (д): txt одного русского текста
// в windows-1251, KOI8-R, CP866, валидном utf-8 и utf-16LE с BOM импортируются
// в utf-8 с тем же текстом, исходная кодировка записана в журнал.
func TestBookImportTxtCharsets(t *testing.T) {
	seedMcpWorld(t)
	logPath := seedMcpJournal(t)

	txtCases := []struct {
		name     string
		content  string
		encoding string
	}{
		{"w1251.txt", encodeSource(t, charmap.Windows1251, txtSample), "windows-1251"},
		{"koi8.txt", encodeSource(t, charmap.KOI8R, txtSample), "KOI8-R"},
		{"cp866.txt", encodeSource(t, charmap.CodePage866, txtSample), "CP866"},
		{"utf8.txt", txtSample, "utf-8"},
		{"utf16.txt", encodeSource(t, textunicode.UTF16(textunicode.LittleEndian, textunicode.UseBOM), txtSample), "utf-16le"},
	}
	for _, tc := range txtCases {
		rr := multipartImport(t, "alpha", tc.name, tc.content)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200; body=%s", tc.name, rr.Code, rr.Body.String())
		}
		var out mcpSourcesResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: bad JSON: %v", tc.name, err)
		}
		var card *sourceManifest
		for i := range out.Sources {
			if out.Sources[i].Name == tc.name {
				card = &out.Sources[i]
			}
		}
		if card == nil {
			t.Fatalf("%s: карточки нет в ответе импорта", tc.name)
		}
		if card.Status != sourceStatusQueued || card.Error != "" {
			t.Fatalf("%s: карточка = %+v, want queued без ошибки", tc.name, card)
		}
		if card.Chapters != 2 {
			t.Fatalf("%s: глав = %d, want 2", tc.name, card.Chapters)
		}
		raw, err := os.ReadFile(filepath.Join(mcpSourcesDir("alpha"), tc.name))
		if err != nil {
			t.Fatalf("%s: ReadFile: %v", tc.name, err)
		}
		if !utf8.Valid(raw) {
			t.Fatalf("%s: файл после импорта не валидный utf-8", tc.name)
		}
		if string(raw) != txtSample {
			t.Fatalf("%s: текст после импорта = %q, want %q", tc.name, raw, txtSample)
		}
	}

	encodings := map[string]string{}
	for _, rec := range readMcpJournal(t, logPath) {
		if rec.Event == journal.EventSourceImport {
			encodings[rec.field("name")] = rec.field("encoding")
		}
	}
	for _, tc := range txtCases {
		if got := encodings[tc.name]; got != tc.encoding {
			t.Fatalf("%s: исходная кодировка в журнале = %q, want %q", tc.name, got, tc.encoding)
		}
	}
}

// TestSourceReforge — «Проверка» IMP-8 (г): «Перековать» переводит карточки
// error с файлом сырья на месте в queued и чистит причину; карточка без файла
// остаётся с ошибкой.
func TestSourceReforge(t *testing.T) {
	seedMcpWorld(t)
	if err := os.MkdirAll(mcpSourcesDir("alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mcpSourcesDir("alpha"), "fixed.txt"), []byte("Глава 1\nтекст"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestManifest(t, []sourceManifest{
		{ID: "fixed.txt", Name: "fixed.txt", Format: "txt", Status: sourceStatusError, Error: "битый файл"},
		{ID: "gone.txt", Name: "gone.txt", Format: "txt", Status: sourceStatusError, Error: "файла нет"},
		{ID: "done.txt", Name: "done.txt", Format: "txt", Status: sourceStatusDone, Progress: 1},
	})

	rr := mcpRequest(t, handleMcpReforge, http.MethodPost, mcpReforgePath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST reforge: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out mcpSourcesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	byName := map[string]sourceManifest{}
	for _, s := range out.Sources {
		byName[s.Name] = s
	}
	fixed := byName["fixed.txt"]
	if fixed.Status != sourceStatusQueued || fixed.Error != "" || fixed.Progress != 0 {
		t.Fatalf("карточка с файлом после reforge = %+v, want queued без причины", fixed)
	}
	if gone := byName["gone.txt"]; gone.Status != sourceStatusError || gone.Error != "файла нет" {
		t.Fatalf("карточка без файла после reforge = %+v, want error с прежней причиной", gone)
	}
	if done := byName["done.txt"]; done.Status != sourceStatusDone {
		t.Fatalf("готовая карточка изменилась: %+v", done)
	}
	items, err := readSourceManifest("alpha")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range items {
		if s.Name == "fixed.txt" && s.Status != sourceStatusQueued {
			t.Fatalf("манифест не сохранён: %+v", s)
		}
	}

	if rr := mcpRequest(t, handleMcpReforge, http.MethodPost, mcpReforgePath, nil, ""); rr.Code != http.StatusBadRequest {
		t.Fatalf("POST reforge без слага: status = %d, want 400", rr.Code)
	}
	if rr := mcpRequest(t, handleMcpReforge, http.MethodPost, mcpReforgePath+"?world=nope", nil, ""); rr.Code != http.StatusNotFound {
		t.Fatalf("POST reforge несуществующего мира: status = %d, want 404", rr.Code)
	}
}
