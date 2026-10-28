package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
)

// seedBookWorld создаёт базу мира alpha по раскладке <root>/worlds/<slug>/<slug>.db,
// кладёт в world_meta слаг и синопсис, подменяет worldDBPath и bookRoot.
func seedBookWorld(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	worldDir := filepath.Join(root, "worlds", "alpha")
	if err := os.MkdirAll(worldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(worldDir, "alpha.db")

	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(raw, "world", "shared"); err != nil {
		raw.Close()
		t.Fatalf("Migrate(world, shared): %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("Close(raw): %v", err)
	}

	forge, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	writeWorldMetaJSON := func(key, value string) {
		rawVal, err := json.Marshal(value)
		if err != nil {
			_ = forge.Close()
			t.Fatalf("json.Marshal(%s): %v", key, err)
		}
		if _, err := forge.Exec("world_meta",
			`INSERT INTO world_meta ("key", value) VALUES (?, ?)`, key, string(rawVal)); err != nil {
			_ = forge.Close()
			t.Fatalf("INSERT world_meta %s: %v", key, err)
		}
	}
	writeWorldMetaJSON("slug", "alpha")
	writeWorldMetaJSON("synopsis", "Герой ищет дорогу домой.")
	if err := forge.Close(); err != nil {
		t.Fatalf("Close(forge): %v", err)
	}

	core, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(core): %v", err)
	}
	t.Cleanup(func() { _ = core.Close() })

	oldPath := worldDBPath
	worldDBPath = func(slug string) string {
		return filepath.Join(root, "worlds", slug, slug+".db")
	}
	oldRoot := bookRoot
	bookRoot = root
	t.Cleanup(func() {
		worldDBPath = oldPath
		bookRoot = oldRoot
	})
	return path
}

// seedBookChapter вставляет обработанную (published) главу с действующим текстом
// и дайджестом. day — порядковый номер главы, n-ый среди обработанных.
func seedBookChapter(t *testing.T, path string, day int64, title, text, digest string) int64 {
	t.Helper()
	core, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(core): %v", err)
	}
	defer core.Close()

	res, err := core.Exec("book_chapters",
		`INSERT INTO book_chapters (day, title, status, kind, digest) VALUES (?, ?, 'published', 'chapter', ?)`,
		day, title, digest)
	if err != nil {
		t.Fatalf("INSERT book_chapters: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	if _, err := core.Exec("book_text",
		`INSERT INTO book_text (chapter_id, version, text, edited_by, published) VALUES (?, 3, ?, 'player', 1)`,
		id, text); err != nil {
		t.Fatalf("INSERT book_text: %v", err)
	}
	return id
}

// seedBookCut вставляет строку book_cuts (правка Цензора).
func seedBookCut(t *testing.T, path string, chapterID int64, cutText, reason, status string) {
	t.Helper()
	core, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(core): %v", err)
	}
	defer core.Close()
	if _, err := core.Exec("book_cuts",
		`INSERT INTO book_cuts (chapter_id, cut_text, reason, status) VALUES (?, ?, ?, ?)`,
		chapterID, cutText, reason, status); err != nil {
		t.Fatalf("INSERT book_cuts: %v", err)
	}
}

func bookRequest(t *testing.T, h http.HandlerFunc, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

// TestBookWindow — окно «Моя книга»: четыре вкладки, содержание и последняя
// обработанная глава (T-08-07, шаг 7).
func TestBookWindow(t *testing.T) {
	path := seedBookWorld(t)
	seedBookChapter(t, path, 1, "Глава 1", "Текст первой главы.", "Дайджест 1.")
	seedBookChapter(t, path, 2, "Глава 2", "Текст второй главы.", "Дайджест 2.")

	rr := bookRequest(t, handleBookWindow, bookWindowPath+"?world=alpha")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", bookWindowPath, rr.Code, rr.Body.String())
	}
	var out bookWindowResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.WindowID != "book" {
		t.Fatalf("window_id = %q, want book", out.WindowID)
	}
	if len(out.Tabs) != 4 {
		t.Fatalf("tabs = %d, want 4", len(out.Tabs))
	}
	if len(out.Toc) != 2 {
		t.Fatalf("toc = %d, want 2", len(out.Toc))
	}
	if out.Chapter == nil || out.Chapter.Title != "Глава 2" {
		t.Fatalf("chapter = %+v, want последнюю главу «Глава 2»", out.Chapter)
	}
	if out.DigestState != "digest" || out.Digest != "Дайджест 2." {
		t.Fatalf("digest = (%q, %q), want (digest, Дайджест 2.)", out.DigestState, out.Digest)
	}
}

// TestBookDigest — «Проверка» T-08-07: полоса дайджеста последней обработанной
// главы; без обработанных глав — no_digest и пустой текст.
func TestBookDigest(t *testing.T) {
	path := seedBookWorld(t)

	rr := bookRequest(t, handleBookDigest, bookDigestPath+"?world=alpha")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", bookDigestPath, rr.Code, rr.Body.String())
	}
	var empty bookDigestResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &empty); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if empty.World != "alpha" {
		t.Fatalf("world = %q, want alpha", empty.World)
	}
	if empty.DigestState != "no_digest" {
		t.Fatalf("digest_state = %q, want no_digest", empty.DigestState)
	}
	if empty.Digest != "" {
		t.Fatalf("digest = %q, want пусто", empty.Digest)
	}

	digest := "Первое предложение. Второе предложение. Третье предложение. Четвёртое предложение. Пятое предложение. Шестое предложение."
	seedBookChapter(t, path, 1, "Глава 1", "Текст первой главы.", digest)

	rr = bookRequest(t, handleBookDigest, bookDigestPath+"?world=alpha")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", bookDigestPath, rr.Code, rr.Body.String())
	}
	var got bookDigestResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if got.DigestState != "digest" {
		t.Fatalf("digest_state = %q, want digest", got.DigestState)
	}
	if got.Digest != digest {
		t.Fatalf("digest = %q, want %q", got.Digest, digest)
	}

	// Пустой слаг — 400; мира нет — 404.
	if rr := bookRequest(t, handleBookDigest, bookDigestPath); rr.Code != http.StatusBadRequest {
		t.Fatalf("GET без слага: status = %d, want 400", rr.Code)
	}
	if rr := bookRequest(t, handleBookDigest, bookDigestPath+"?world=nope"); rr.Code != http.StatusNotFound {
		t.Fatalf("GET несуществующего мира: status = %d, want 404", rr.Code)
	}
}

// TestCensorCuts — «Проверка» T-08-07: правки Цензора «было → стало»,
// применённые и отклонённые, в порядке поступления.
func TestCensorCuts(t *testing.T) {
	path := seedBookWorld(t)
	ch := seedBookChapter(t, path, 1, "Глава 1", "Текст.", "Дайджест.")
	seedBookCut(t, path, ch, "ёлку. → ёлку…", "пунктуация", "applied")
	seedBookCut(t, path, ch, "красный быстрый автомобиль", "замена или перестановка слова", "rejected")

	rr := bookRequest(t, handleBookCuts, bookCutsPath+"?world=alpha")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", bookCutsPath, rr.Code, rr.Body.String())
	}
	var out bookCutsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.World != "alpha" {
		t.Fatalf("world = %q, want alpha", out.World)
	}
	if len(out.Cuts) != 2 {
		t.Fatalf("cuts = %d, want 2 (%+v)", len(out.Cuts), out.Cuts)
	}

	if out.Cuts[0].Status != "applied" || out.Cuts[0].Before != "ёлку." || out.Cuts[0].After != "ёлку…" {
		t.Fatalf("cut[0] = %+v, want applied «ёлку. → ёлку…»", out.Cuts[0])
	}
	if out.Cuts[1].Status != "rejected" || out.Cuts[1].Before != "" || out.Cuts[1].After != "красный быстрый автомобиль" {
		t.Fatalf("cut[1] = %+v, want rejected с текстом результата", out.Cuts[1])
	}

	// Пустой слаг — 400; мира нет — 404.
	if rr := bookRequest(t, handleBookCuts, bookCutsPath); rr.Code != http.StatusBadRequest {
		t.Fatalf("GET без слага: status = %d, want 400", rr.Code)
	}
	if rr := bookRequest(t, handleBookCuts, bookCutsPath+"?world=nope"); rr.Code != http.StatusNotFound {
		t.Fatalf("GET несуществующего мира: status = %d, want 404", rr.Code)
	}
}

// TestBookExport — «Проверка» T-08-07: выгрузка MD главы и книги с именами
// <slug>-chapter-<N>.md и <slug>-book.md.
func TestBookExport(t *testing.T) {
	path := seedBookWorld(t)
	seedBookChapter(t, path, 1, "Глава 1", "Текст первой главы.", "Дайджест 1.")
	seedBookChapter(t, path, 2, "Глава 2", "Текст второй главы.", "Дайджест 2.")

	rr := bookRequest(t, handleBookExport, bookExportPath+"?world=alpha&format=md&scope=chapter&chapter=1")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s (chapter): status = %d, want 200; body=%s", bookExportPath, rr.Code, rr.Body.String())
	}
	var chRes bookExportResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &chRes); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if chRes.Path != "alpha-chapter-1.md" {
		t.Fatalf("path = %q, want alpha-chapter-1.md", chRes.Path)
	}
	chFile := filepath.Join(bookRoot, "worlds", "alpha", "export", "alpha-chapter-1.md")
	data, err := os.ReadFile(chFile)
	if err != nil {
		t.Fatalf("ReadFile главы: %v", err)
	}
	if !strings.Contains(string(data), "# Глава 1") {
		t.Fatalf("файл главы без заголовка: %q", data)
	}

	rr = bookRequest(t, handleBookExport, bookExportPath+"?world=alpha&format=md&scope=book")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s (book): status = %d, want 200; body=%s", bookExportPath, rr.Code, rr.Body.String())
	}
	var bookRes bookExportResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &bookRes); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if bookRes.Path != "alpha-book.md" {
		t.Fatalf("path = %q, want alpha-book.md", bookRes.Path)
	}
	bookFile := filepath.Join(bookRoot, "worlds", "alpha", "export", "alpha-book.md")
	data, err = os.ReadFile(bookFile)
	if err != nil {
		t.Fatalf("ReadFile книги: %v", err)
	}
	for _, heading := range []string{"# Глава 1", "# Глава 2"} {
		if !strings.Contains(string(data), heading) {
			t.Fatalf("в книге нет заголовка %q: %q", heading, data)
		}
	}

	// Другой формат и неверная гранулярность — 400.
	if rr := bookRequest(t, handleBookExport, bookExportPath+"?world=alpha&format=docx&scope=book"); rr.Code != http.StatusBadRequest {
		t.Fatalf("GET docx: status = %d, want 400", rr.Code)
	}
	if rr := bookRequest(t, handleBookExport, bookExportPath+"?world=alpha&format=md&scope=nope"); rr.Code != http.StatusBadRequest {
		t.Fatalf("GET scope=nope: status = %d, want 400", rr.Code)
	}
	if rr := bookRequest(t, handleBookExport, bookExportPath+"?world=alpha&format=md&scope=chapter"); rr.Code != http.StatusBadRequest {
		t.Fatalf("GET chapter без номера: status = %d, want 400", rr.Code)
	}
}
