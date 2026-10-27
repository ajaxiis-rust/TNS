package book

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tns65/internal/db"
)

// Экспорт книги (T-07-12, CB-08): выгрузка главы и всей книги в Markdown.
// Имена файлов строятся из слага мира, разметка каноническая и выгружается без
// потерь; экспорт только читает обработанные главы и в базы не пишет.

// Format — формат выгрузки (CB-08.1): поддерживается только Markdown.
type Format string

// FormatMarkdown — единственный поддерживаемый формат выгрузки.
const FormatMarkdown Format = "md"

var (
	// ErrExportFormat — запрошенный формат выгрузки не поддерживается.
	ErrExportFormat = errors.New("book: поддерживается только выгрузка в Markdown")
	// ErrExportNoChapters — экспорт книги без обработанных глав.
	ErrExportNoChapters = errors.New("book: нет обработанных глав для экспорта")
)

// checkFormat пропускает только Markdown (CB-08.1).
func checkFormat(f Format) error {
	if f != FormatMarkdown {
		return fmt.Errorf("%w: %q", ErrExportFormat, f)
	}
	return nil
}

// worldMetaString читает значение анкеты мира из world_meta по ключу; значения
// world_meta хранятся JSON-строками (T-02-03).
func worldMetaString(h *db.Handle, key string) (string, error) {
	var raw string
	err := h.QueryRow(`SELECT value FROM world_meta WHERE "key" = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("book: world_meta.%s не найден", key)
	}
	if err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return "", fmt.Errorf("book: разбор world_meta.%s: %w", key, err)
	}
	return s, nil
}

// worldSlug — слаг мира из анкеты (T-02-10): часть имени файла выгрузки.
func worldSlug(h *db.Handle) (string, error) {
	return worldMetaString(h, "slug")
}

// annotation — аннотация книги в шапке (T-07-12, шаг 5): действующая заявка
// игрока; без заявки — синопсис анкеты.
func annotation(h *db.Handle) (string, error) {
	claim, err := CurrentClaim(h)
	if err == nil {
		return claim.Text, nil
	}
	if !errors.Is(err, ErrNoClaim) {
		return "", err
	}
	return claimSynopsis(h)
}

// exportDir — каталог выгрузки мира: <root>/worlds/<slug>/export (шаг 3).
func exportDir(root, slug string) string {
	if root == "" {
		root = db.WorldsRoot
	}
	return filepath.Join(root, "worlds", slug, "export")
}

// chapterMarkdown — каноническая разметка главы (CB-08.4): заголовок H1 и текст
// главы без потерь.
func chapterMarkdown(title, text string) string {
	return "# " + title + "\n\n" + text
}

// ExportChapter выгружает одну обработанную главу по порядковому номеру n
// (1-based по day среди обработанных) в
// <root>/worlds/<slug>/export/<slug>-chapter-<n>.md и возвращает путь к файлу.
func ExportChapter(h *db.Handle, root string, format Format, n int) (string, error) {
	if err := checkFormat(format); err != nil {
		return "", err
	}
	if h == nil {
		return "", errors.New("book: нет хэндла мира")
	}
	if n < 1 {
		return "", errors.New("book: номер главы не задан")
	}

	chapters, err := List(h)
	if err != nil {
		return "", err
	}
	if n > len(chapters) {
		return "", ErrChapterNotPublished
	}
	ch := chapters[n-1]

	text, err := OpenChapterText(h, ch.ChapterID)
	if err != nil {
		return "", err
	}
	slug, err := worldSlug(h)
	if err != nil {
		return "", err
	}

	dir := exportDir(root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-chapter-%d.md", slug, n))
	if err := os.WriteFile(path, []byte(chapterMarkdown(ch.Title, text)+"\n"), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ExportBook выгружает всю книгу обработанных глав по порядку day в
// <root>/worlds/<slug>/export/<slug>-book.md и возвращает путь к файлу. В шапке
// книги — аннотация, дальше каждая глава заголовком. Без обработанных глав —
// отказ, файл не создаётся.
func ExportBook(h *db.Handle, root string, format Format) (string, error) {
	if err := checkFormat(format); err != nil {
		return "", err
	}
	if h == nil {
		return "", errors.New("book: нет хэндла мира")
	}

	chapters, err := List(h)
	if err != nil {
		return "", err
	}
	if len(chapters) == 0 {
		return "", ErrExportNoChapters
	}
	slug, err := worldSlug(h)
	if err != nil {
		return "", err
	}
	ann, err := annotation(h)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString(ann)
	for _, ch := range chapters {
		text, err := OpenChapterText(h, ch.ChapterID)
		if err != nil {
			return "", err
		}
		b.WriteString("\n\n")
		b.WriteString(chapterMarkdown(ch.Title, text))
	}
	b.WriteString("\n")

	dir := exportDir(root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, slug+"-book.md")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
