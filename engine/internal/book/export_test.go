package book

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
)

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
