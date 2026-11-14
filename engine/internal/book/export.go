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

// exportChapters — обработанные главы книги (kind='chapter') по порядку day.
// Эпилог сюда не входит — он добавляется последним при сборке книги (T-07-13).
func exportChapters(h *db.Handle) ([]TocEntry, error) {
	return queryBookChapters(h, `WHERE kind = 'chapter' AND status = 'published' ORDER BY day, id`)
}

// exportEpilogues — эпилоги книги (kind='epilogue') в порядке записи (id).
func exportEpilogues(h *db.Handle) ([]TocEntry, error) {
	return queryBookChapters(h, `WHERE kind = 'epilogue' ORDER BY id`)
}

func queryBookChapters(h *db.Handle, clause string) ([]TocEntry, error) {
	if h == nil {
		return nil, errors.New("book: нет хэндла мира")
	}
	rows, err := h.Query(`SELECT id, day, title FROM book_chapters ` + clause)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TocEntry
	for rows.Next() {
		var e TocEntry
		if err := rows.Scan(&e.ChapterID, &e.Day, &e.Title); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
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

	chapters, err := exportChapters(h)
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
// книги — аннотация, дальше каждая глава заголовком, эпилог — последней главой
// (T-07-13). Без обработанных глав и эпилога — отказ, файл не создаётся.
func ExportBook(h *db.Handle, root string, format Format) (string, error) {
	if err := checkFormat(format); err != nil {
		return "", err
	}
	if h == nil {
		return "", errors.New("book: нет хэндла мира")
	}

	chapters, err := exportChapters(h)
	if err != nil {
		return "", err
	}
	epilogues, err := exportEpilogues(h)
	if err != nil {
		return "", err
	}
	if len(chapters) == 0 && len(epilogues) == 0 {
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
	for _, ep := range epilogues {
		text, err := OpenChapterText(h, ep.ChapterID)
		if err != nil {
			return "", err
		}
		b.WriteString("\n\n")
		b.WriteString(chapterMarkdown(ep.Title, text))
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

// Экспорт опубликованных глав в ready/ (IMP-6): один файл Markdown на главу с
// YAML front matter и текстом действующей версии. Каталог живёт в state-каталоге
// мира рядом с базами: <root>/worlds/<slug>/ready; пустой root — db.WorldsRoot.

// ErrReadyExport — глава опубликована, но файл ready/ не записан: публикация в
// базе мира остаётся, ошибка возвращается наружу.
var ErrReadyExport = errors.New("book: глава опубликована, но экспорт в ready/ не удался")

// ReadyDir — каталог готовых к публикации глав мира slug (IMP-6, шаг 1).
func ReadyDir(root, slug string) string {
	if root == "" {
		root = db.WorldsRoot
	}
	return filepath.Join(root, "worlds", slug, "ready")
}

// readyFileName — детерминированное имя файла главы (IMP-6, шаг 2): глава —
// chapter-<day>-<id>.md, эпилог — epilogue-<day>-<id>.md. Пере-экспорт
// перезаписывает тот же файл, дублей не появляется.
func readyFileName(kind ChapterKind, day, id int64) string {
	prefix := "chapter"
	if kind == KindEpilogue {
		prefix = "epilogue"
	}
	return fmt.Sprintf("%s-%d-%d.md", prefix, day, id)
}

// readyChapter — глава файла ready/: строки book_chapters, действующая версия и
// дата из журнала мира (пусто — дата опускается).
type readyChapter struct {
	ID     int64
	Day    int64
	Title  string
	Kind   ChapterKind
	Digest string
	Date   string
	Text   string
}

// readyFrontMatter — YAML-шапка файла главы (IMP-6, шаг 2): мир, глава, день,
// заголовок, вид, дайджест и дата, если у главы есть источник даты. Строки в
// двойных кавычках: заголовок и дайджест содержат двоеточия и переводы строк.
func readyFrontMatter(slug string, ch readyChapter) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "world: %q\n", slug)
	fmt.Fprintf(&b, "chapter_id: %d\n", ch.ID)
	fmt.Fprintf(&b, "day: %d\n", ch.Day)
	fmt.Fprintf(&b, "title: %q\n", ch.Title)
	fmt.Fprintf(&b, "kind: %s\n", ch.Kind)
	fmt.Fprintf(&b, "digest: %q\n", ch.Digest)
	if ch.Date != "" {
		fmt.Fprintf(&b, "date: %q\n", ch.Date)
	}
	b.WriteString("---\n")
	return b.String()
}

// readyDate — дата главы из журнала мира: closed_at события автозакрытия главы.
// Прямой связи главы с сессией в базе мира нет (book_chapters.session_id не
// хранится), поэтому sessions.ended_at к главе не привязать; без события дата
// опускается.
func readyDate(h *db.Handle, chapterID int64) string {
	rows, err := h.Query(`SELECT payload_json FROM events WHERE kind = ? ORDER BY id`, AutoCloseEventKind)
	if err != nil {
		return ""
	}
	defer rows.Close()

	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			continue
		}
		var payload struct {
			ChapterID int64  `json:"chapter_id"`
			ClosedAt  string `json:"closed_at"`
		}
		if json.Unmarshal([]byte(raw), &payload) != nil {
			continue
		}
		if payload.ChapterID == chapterID && payload.ClosedAt != "" {
			return payload.ClosedAt
		}
	}
	return ""
}

// ExportReadyChapter выгружает опубликованную главу в
// <root>/worlds/<slug>/ready/<kind>-<day>-<id>.md (IMP-6) и возвращает путь.
// Неопубликованная глава не выгружается — ErrChapterNotPublished, файла нет.
func ExportReadyChapter(h *db.Handle, root string, chapterID int64) (string, error) {
	if h == nil {
		return "", errors.New("book: нет хэндла мира")
	}
	if chapterID <= 0 {
		return "", errors.New("book: id главы не задан")
	}

	ch := readyChapter{ID: chapterID}
	var status, kind string
	err := h.QueryRow(`SELECT day, title, status, kind FROM book_chapters WHERE id = ?`, chapterID).
		Scan(&ch.Day, &ch.Title, &status, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("book: глава не найдена")
	}
	if err != nil {
		return "", err
	}
	if status != string(StatusPublished) {
		return "", ErrChapterNotPublished
	}
	ch.Kind = ChapterKind(kind)

	pub, err := LoadPublished(h, chapterID)
	if err != nil {
		return "", err
	}
	ch.Text = pub.Text
	ch.Digest = pub.Digest

	slug, err := worldSlug(h)
	if err != nil {
		return "", err
	}
	ch.Date = readyDate(h, chapterID)

	dir := ReadyDir(root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, readyFileName(ch.Kind, ch.Day, ch.ID))
	body := readyFrontMatter(slug, ch) + ch.Text + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ExportReadyAll пере-экспортирует все опубликованные главы мира (backfill,
// IMP-6, шаг 3): порядок day, id, эпилоги идут за главами. Возвращает пути
// записанных файлов; при сбое записи останавливается и отдаёт ошибку.
func ExportReadyAll(h *db.Handle, root string) ([]string, error) {
	if h == nil {
		return nil, errors.New("book: нет хэндла мира")
	}

	rows, err := h.Query(`SELECT id FROM book_chapters WHERE status = 'published' ORDER BY day, id`)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	paths := make([]string, 0, len(ids))
	for _, id := range ids {
		path, err := ExportReadyChapter(h, root, id)
		if err != nil {
			return paths, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}
