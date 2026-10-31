//go:build e2e

package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/book"
	"tns65/internal/db"
)

// TestINT8 — INT-8. Экспорт книги (T-09-08): выгрузка только MD; имена файлов
// строятся от слага мира из анкеты; в файле книги — заголовки всех обработанных
// глав; другой формат отвергается без создания файла; необработанная глава в
// экспорт не попадает и заглушки в файле нет.
func TestINT8(t *testing.T) {
	bin := buildBinary(t, []string{"e2e"})
	dir := worldDir(t)

	slug := "export-knigi"
	ancketa := writeAncketa(t, t.TempDir(), slug,
		"Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.")
	stdout, stderr, code := runCmd(t, dir, bin, "world", "birth", "--ancketa", ancketa, "--fake-embed", "--fake-slot")
	if code != 0 {
		t.Fatalf("world birth exit=%d, want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	worldDB := worldDBPath(dir, slug)
	h, err := db.OpenWriterAs(worldDB, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("открыть базу мира: %v", err)
	}
	defer h.Close()

	// Две обработанные главы и одна закрытая, но не прошедшая полную обработку.
	seedExportChapter(t, h, 1, "Глава 1", "Текст первой главы.", true)
	seedExportChapter(t, h, 2, "Глава 2", "Текст второй главы.", true)
	seedExportChapter(t, h, 3, "Глава 3", "Необработанная глава.", false)

	root := t.TempDir()
	report := map[string]any{}

	// Шаг 1: экспорт главы и книги — имена файлов от слага мира.
	chPath, err := book.ExportChapter(h, root, book.FormatMarkdown, 1)
	if err != nil {
		t.Fatalf("ExportChapter: %v", err)
	}
	bookPath, err := book.ExportBook(h, root, book.FormatMarkdown)
	if err != nil {
		t.Fatalf("ExportBook: %v", err)
	}

	// Шаг 2: расширение только .md, слаг из анкеты, в книге заголовки всех
	// обработанных глав.
	wantChPath := filepath.Join(root, "worlds", slug, "export", slug+"-chapter-1.md")
	wantBookPath := filepath.Join(root, "worlds", slug, "export", slug+"-book.md")
	if chPath != wantChPath {
		t.Fatalf("путь главы = %q, want %q", chPath, wantChPath)
	}
	if bookPath != wantBookPath {
		t.Fatalf("путь книги = %q, want %q", bookPath, wantBookPath)
	}
	report["files"] = []string{filepath.Base(chPath), filepath.Base(bookPath)}

	bookStr := readTextFile(t, bookPath)
	for _, heading := range []string{"# Глава 1", "# Глава 2"} {
		if !strings.Contains(bookStr, heading) {
			t.Fatalf("в книге нет заголовка %q: %q", heading, bookStr)
		}
	}

	// Шаг 3: другой формат (docx/epub/txt) отвергнут, файл не создан.
	foreignFormatRejected := true
	for _, f := range []book.Format{"docx", "epub", "txt"} {
		if _, err := book.ExportBook(h, root, f); !errors.Is(err, book.ErrExportFormat) {
			foreignFormatRejected = false
		}
	}
	report["foreign_format_rejected"] = foreignFormatRejected
	if !foreignFormatRejected {
		t.Fatal("foreign_format_rejected=false: чужой формат не отвергнут ErrExportFormat")
	}
	for _, f := range []book.Format{"docx", "epub", "txt"} {
		if _, err := book.ExportChapter(h, root, f, 1); !errors.Is(err, book.ErrExportFormat) {
			t.Fatalf("ExportChapter(%s) не отвергнут ErrExportFormat", f)
		}
	}

	// В каталоге экспорта только .md-файлы — чужие форматы файлов не создали.
	entries, err := os.ReadDir(filepath.Join(root, "worlds", slug, "export"))
	if err != nil {
		t.Fatalf("ReadDir export: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".md" {
			t.Fatalf("в каталоге экспорта файл чужого формата: %q", e.Name())
		}
	}

	// Шаг 4: необработанная глава в экспорт не попадает и заглушки в файле нет.
	unprocessedAbsent := !strings.Contains(bookStr, "# Глава 3")
	report["unprocessed_absent"] = unprocessedAbsent
	if !unprocessedAbsent {
		t.Fatal("unprocessed_absent=false: необработанная глава попала в книгу или осталась заглушка")
	}
	if _, err := book.ExportChapter(h, root, book.FormatMarkdown, 3); !errors.Is(err, book.ErrChapterNotPublished) {
		t.Fatalf("ExportChapter(необработанная) = %v, want ErrChapterNotPublished", err)
	}

	writeReport(t, "int8", report)
}

// seedExportChapter вставляет главу с действующей версией 1 в базу мира хэндлом
// core. Обработанная глава (published=true) помечается статусом published и
// версией published=1; необработанная — статусом closed и версией published=0.
func seedExportChapter(t *testing.T, h *db.Handle, day int64, title, text string, published bool) {
	t.Helper()
	status := "closed"
	if published {
		status = "published"
	}
	res, err := h.Exec("book_chapters",
		`INSERT INTO book_chapters (day, title, status, kind) VALUES (?, ?, ?, 'chapter')`, day, title, status)
	if err != nil {
		t.Fatalf("INSERT book_chapters (day %d): %v", day, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("book_chapters id: %v", err)
	}
	publishedFlag := 0
	if published {
		publishedFlag = 1
	}
	if _, err := h.Exec("book_text",
		`INSERT INTO book_text (chapter_id, version, text, edited_by, published) VALUES (?, 1, ?, NULL, ?)`,
		id, text, publishedFlag); err != nil {
		t.Fatalf("INSERT book_text: %v", err)
	}
}

// readTextFile читает файл целиком и возвращает содержимое строкой.
func readTextFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return string(raw)
}
