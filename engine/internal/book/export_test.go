package book

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
)

// TestMain выносит корень миров пакета во временный каталог: публикация пишет
// ready/ рядом с базами мира (IMP-6) и не должна трогать рабочий каталог.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "tns65-book-worlds-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "tns65 book: временный корень миров: %v\n", err)
		os.Exit(1)
	}
	old := db.WorldsRoot
	db.WorldsRoot = root
	code := m.Run()
	db.WorldsRoot = old
	_ = os.RemoveAll(root)
	os.Exit(code)
}

// newExportWorld создаёт оперативную базу мира с ключами анкеты slug и synopsis
// в world_meta (пишет forge по карте писателей) и возвращает хэндл core.
func newExportWorld(t *testing.T, slug, synopsis string) *db.Handle {
	t.Helper()
	path := filepath.Join(t.TempDir(), slug+".db")

	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(w, "world", "shared"); err != nil {
		_ = w.Close()
		t.Fatalf("Migrate(world, shared): %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	fh, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	writeWorldMeta := func(key, value string) {
		raw, err := json.Marshal(value)
		if err != nil {
			_ = fh.Close()
			t.Fatalf("json.Marshal(%s): %v", key, err)
		}
		if _, err := fh.Exec("world_meta",
			`INSERT INTO world_meta ("key", value) VALUES (?, ?)`, key, string(raw)); err != nil {
			_ = fh.Close()
			t.Fatalf("INSERT world_meta %s: %v", key, err)
		}
	}
	writeWorldMeta("slug", slug)
	writeWorldMeta("synopsis", synopsis)
	if err := fh.Close(); err != nil {
		t.Fatalf("Close(forge): %v", err)
	}

	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// TestExport — «Проверка» T-07-12.
func TestExport(t *testing.T) {
	const slug = "alpha-world"
	const synopsis = "Герой ищет дорогу домой."
	const claimText = "Заявка книги. О чём она."

	t.Run("экспорт даёт chapter-N.md и book.md с заголовками и аннотацией", func(t *testing.T) {
		world := newExportWorld(t, slug, synopsis)
		root := t.TempDir()

		seedAndPublishChapter(t, world, insertOpenSession(t, world), 1, "Глава 1")
		seedAndPublishChapter(t, world, insertOpenSession(t, world), 2, "Глава 2")

		if _, err := SaveClaim(context.Background(), world, testkit.NewFakeEmbed(), claimText, ClaimApproved); err != nil {
			t.Fatalf("SaveClaim: %v", err)
		}

		chPath, err := ExportChapter(world, root, FormatMarkdown, 1)
		if err != nil {
			t.Fatalf("ExportChapter: %v", err)
		}
		wantChPath := filepath.Join(root, "worlds", slug, "export", slug+"-chapter-1.md")
		if chPath != wantChPath {
			t.Fatalf("путь главы = %q, want %q", chPath, wantChPath)
		}
		chContent, err := os.ReadFile(chPath)
		if err != nil {
			t.Fatalf("ReadFile главы: %v", err)
		}
		if !strings.Contains(string(chContent), "# Глава 1") {
			t.Fatalf("файл главы без заголовка «Глава 1»: %q", chContent)
		}

		bookPath, err := ExportBook(world, root, FormatMarkdown)
		if err != nil {
			t.Fatalf("ExportBook: %v", err)
		}
		wantBookPath := filepath.Join(root, "worlds", slug, "export", slug+"-book.md")
		if bookPath != wantBookPath {
			t.Fatalf("путь книги = %q, want %q", bookPath, wantBookPath)
		}
		book, err := os.ReadFile(bookPath)
		if err != nil {
			t.Fatalf("ReadFile книги: %v", err)
		}
		bookStr := string(book)
		for _, heading := range []string{"# Глава 1", "# Глава 2"} {
			if !strings.Contains(bookStr, heading) {
				t.Fatalf("в книге нет заголовка %q: %q", heading, bookStr)
			}
		}
		if !strings.Contains(bookStr, claimText) {
			t.Fatalf("в шапке книги нет аннотации %q: %q", claimText, bookStr)
		}
		// Аннотация в шапке — до первого заголовка главы.
		if strings.Index(bookStr, claimText) > strings.Index(bookStr, "# Глава 1") {
			t.Fatalf("аннотация не в шапке книги: %q", bookStr)
		}
	})

	t.Run("аннотация без заявки — синопсис анкеты", func(t *testing.T) {
		world := newExportWorld(t, slug, synopsis)
		root := t.TempDir()

		seedAndPublishChapter(t, world, insertOpenSession(t, world), 1, "Глава 1")

		bookPath, err := ExportBook(world, root, FormatMarkdown)
		if err != nil {
			t.Fatalf("ExportBook: %v", err)
		}
		book, err := os.ReadFile(bookPath)
		if err != nil {
			t.Fatalf("ReadFile книги: %v", err)
		}
		if !strings.Contains(string(book), synopsis) {
			t.Fatalf("в шапке книги нет синопсиса %q: %q", synopsis, book)
		}
	})

	t.Run("экспорт без обработанных глав — отказ, файл не создан", func(t *testing.T) {
		world := newExportWorld(t, slug, synopsis)
		root := t.TempDir()

		if _, err := ExportBook(world, root, FormatMarkdown); !errors.Is(err, ErrExportNoChapters) {
			t.Fatalf("ExportBook без глав = %v, want ErrExportNoChapters", err)
		}
		bookPath := filepath.Join(root, "worlds", slug, "export", slug+"-book.md")
		if _, err := os.Stat(bookPath); !os.IsNotExist(err) {
			t.Fatalf("файл книги создан без обработанных глав: %q", bookPath)
		}
	})

	t.Run("другой формат — отказ, файл не создан", func(t *testing.T) {
		world := newExportWorld(t, slug, synopsis)
		root := t.TempDir()

		seedAndPublishChapter(t, world, insertOpenSession(t, world), 1, "Глава 1")

		if _, err := ExportBook(world, root, Format("fb2")); !errors.Is(err, ErrExportFormat) {
			t.Fatalf("ExportBook(fb2) = %v, want ErrExportFormat", err)
		}
		if _, err := ExportChapter(world, root, Format("docx"), 1); !errors.Is(err, ErrExportFormat) {
			t.Fatalf("ExportChapter(docx) = %v, want ErrExportFormat", err)
		}
		exportDir := filepath.Join(root, "worlds", slug, "export")
		if _, err := os.Stat(exportDir); !os.IsNotExist(err) {
			t.Fatalf("каталог экспорта создан при отказе формата: %q", exportDir)
		}
	})
}

// splitFrontMatter отделяет YAML-шапку файла ready/ от текста главы.
func splitFrontMatter(content string) (head, text string, ok bool) {
	const fence = "---\n"
	if !strings.HasPrefix(content, fence) {
		return "", "", false
	}
	rest := content[len(fence):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return "", "", false
	}
	return rest[:end], rest[end+len("\n---\n"):], true
}

// readyPath — путь файла главы в ready/ мира slug.
func readyPath(slug, name string) string {
	return filepath.Join(db.WorldsRoot, "worlds", slug, "ready", name)
}

// TestExportReady — «Проверка» IMP-6: опубликованные главы уходят в ready/
// файлами Markdown с YAML front matter, неопубликованные — нет.
func TestExportReady(t *testing.T) {
	const synopsis = "Герой ищет дорогу домой."

	t.Run("публикация создаёт файл главы с front matter и текстом", func(t *testing.T) {
		const slug = "ready-publish"
		world := newExportWorld(t, slug, synopsis)
		ch := seedAndPublishChapter(t, world, insertOpenSession(t, world), 1, "Глава 1")

		want := readyPath(slug, fmt.Sprintf("chapter-1-%d.md", ch.ID))
		body, err := os.ReadFile(want)
		if err != nil {
			t.Fatalf("ReadFile ready: %v", err)
		}
		head, text, ok := splitFrontMatter(string(body))
		if !ok {
			t.Fatalf("нет YAML front matter: %q", body)
		}
		for _, field := range []string{
			fmt.Sprintf("world: %q", slug),
			fmt.Sprintf("chapter_id: %d", ch.ID),
			"day: 1",
			`title: "Глава 1"`,
			"kind: chapter",
			fmt.Sprintf("digest: %q", gateDigest),
		} {
			if !strings.Contains(head, field) {
				t.Fatalf("в front matter нет %q: %q", field, head)
			}
		}
		if strings.Contains(head, "date:") {
			t.Fatalf("дата в front matter без источника в журнале: %q", head)
		}
		if strings.TrimSpace(text) != gatePlayerText {
			t.Fatalf("текст главы = %q, want %q", text, gatePlayerText)
		}
	})

	t.Run("повторный экспорт перезаписывает файл без дублей", func(t *testing.T) {
		const slug = "ready-repeat"
		world := newExportWorld(t, slug, synopsis)
		ch := seedAndPublishChapter(t, world, insertOpenSession(t, world), 1, "Глава 1")

		if _, err := ExportReadyChapter(world, "", ch.ID); err != nil {
			t.Fatalf("ExportReadyChapter: %v", err)
		}
		path, err := ExportReadyChapter(world, "", ch.ID)
		if err != nil {
			t.Fatalf("ExportReadyChapter(повтор): %v", err)
		}
		if err := Process(context.Background(), world, testkit.NewFakeEmbed(), ch.ID); err != nil {
			t.Fatalf("Process(повтор): %v", err)
		}

		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil {
			t.Fatalf("ReadDir ready: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("в ready/ %d файлов, want 1", len(entries))
		}
		if entries[0].Name() != filepath.Base(path) {
			t.Fatalf("файл ready/ = %q, want %q", entries[0].Name(), filepath.Base(path))
		}
	})

	t.Run("неопубликованная глава не экспортируется", func(t *testing.T) {
		const slug = "ready-unpublished"
		world := newExportWorld(t, slug, synopsis)
		ch := insertChapterWithDay(t, world, insertOpenSession(t, world), 1, "Глава 1")

		if _, err := ExportReadyChapter(world, "", ch.ID); !errors.Is(err, ErrChapterNotPublished) {
			t.Fatalf("ExportReadyChapter(неопубликованная) = %v, want ErrChapterNotPublished", err)
		}
		dir := filepath.Join(db.WorldsRoot, "worlds", slug, "ready")
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("каталог ready/ создан для неопубликованной главы: %q", dir)
		}
	})

	t.Run("эпилог экспортируется с kind epilogue", func(t *testing.T) {
		const slug = "ready-epilogue"
		world := newExportWorld(t, slug, synopsis)
		ep, err := SaveEpilogue(world, "Эпилог", "Иван вернулся домой.")
		if err != nil {
			t.Fatalf("SaveEpilogue: %v", err)
		}

		path, err := ExportReadyChapter(world, "", ep.ID)
		if err != nil {
			t.Fatalf("ExportReadyChapter(эпилог): %v", err)
		}
		if want := readyPath(slug, fmt.Sprintf("epilogue-0-%d.md", ep.ID)); path != want {
			t.Fatalf("путь эпилога = %q, want %q", path, want)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile эпилога: %v", err)
		}
		head, text, ok := splitFrontMatter(string(body))
		if !ok {
			t.Fatalf("нет YAML front matter эпилога: %q", body)
		}
		if !strings.Contains(head, "kind: epilogue") {
			t.Fatalf("в front matter эпилога нет kind: epilogue: %q", head)
		}
		if strings.TrimSpace(text) != "Иван вернулся домой." {
			t.Fatalf("текст эпилога = %q", text)
		}
	})

	t.Run("backfill выгружает все опубликованные главы", func(t *testing.T) {
		const slug = "ready-backfill"
		world := newExportWorld(t, slug, synopsis)
		seedAndPublishChapter(t, world, insertOpenSession(t, world), 1, "Глава 1")
		seedAndPublishChapter(t, world, insertOpenSession(t, world), 2, "Глава 2")
		if _, err := SaveEpilogue(world, "Эпилог", "Иван вернулся домой."); err != nil {
			t.Fatalf("SaveEpilogue: %v", err)
		}

		paths, err := ExportReadyAll(world, "")
		if err != nil {
			t.Fatalf("ExportReadyAll: %v", err)
		}
		if len(paths) != 3 {
			t.Fatalf("ExportReadyAll вернул %d файлов, want 3: %v", len(paths), paths)
		}
		entries, err := os.ReadDir(filepath.Join(db.WorldsRoot, "worlds", slug, "ready"))
		if err != nil {
			t.Fatalf("ReadDir ready: %v", err)
		}
		if len(entries) != 3 {
			t.Fatalf("в ready/ %d файлов, want 3", len(entries))
		}
	})

	t.Run("сбой записи ready/ не откатывает публикацию", func(t *testing.T) {
		const slug = "ready-failure"
		broken := t.TempDir()
		old := db.WorldsRoot
		db.WorldsRoot = broken
		t.Cleanup(func() { db.WorldsRoot = old })

		world := newExportWorld(t, slug, synopsis)
		// ready — обычный файл: MkdirAll не создаёт каталог и экспорт падает.
		ready := filepath.Join(broken, "worlds", slug, "ready")
		if err := os.MkdirAll(filepath.Dir(ready), 0o755); err != nil {
			t.Fatalf("MkdirAll worlds: %v", err)
		}
		if err := os.WriteFile(ready, []byte("не каталог"), 0o644); err != nil {
			t.Fatalf("WriteFile ready: %v", err)
		}

		ch := seedReadyChapter(t, world)
		if err := Process(context.Background(), world, testkit.NewFakeEmbed(), ch.ID); !errors.Is(err, ErrReadyExport) {
			t.Fatalf("Process при сбое ready = %v, want ErrReadyExport", err)
		}

		var status string
		if err := world.QueryRow(`SELECT status FROM book_chapters WHERE id = ?`, ch.ID).Scan(&status); err != nil {
			t.Fatalf("SELECT book_chapters.status: %v", err)
		}
		if status != string(StatusPublished) {
			t.Fatalf("статус главы после сбоя ready = %q, want published", status)
		}
		if _, ok := chapterTabText(world, ch.ID); !ok {
			t.Fatal("действующая версия главы не помечена published после сбоя ready")
		}
	})
}
