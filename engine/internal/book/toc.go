package book

import (
	"database/sql"
	"errors"

	"tns65/internal/db"
)

// TocEntry — строка содержания (T-07-08, шаг 2): обработанная глава с заголовком.
type TocEntry struct {
	ChapterID int64
	Day       int64
	Title     string
}

// ErrChapterNotPublished — глава не обработана и отсутствует в содержании.
var ErrChapterNotPublished = errors.New("book: глава не в содержании")

// List — содержание книги (шаги 1–2): упорядоченный по day перечень
// обработанных (status='published') глав с заголовками. Необработанные главы в
// список не попадают; записей в базу не делает.
func List(h *db.Handle) ([]TocEntry, error) {
	if h == nil {
		return nil, errors.New("book: нет хэндла мира")
	}

	rows, err := h.Query(
		`SELECT id, day, title FROM book_chapters WHERE status = 'published' ORDER BY day, id`)
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

// OpenChapterText — переход к главе из содержания (шаг 3): возвращает текст
// обработанной главы — действующую издательскую версию (book_text.published=1)
// для вкладки «Глава» (T-07-10). Необработанная глава — ErrChapterNotPublished.
func OpenChapterText(h *db.Handle, chapterID int64) (string, error) {
	if h == nil {
		return "", errors.New("book: нет хэндла мира")
	}
	if chapterID <= 0 {
		return "", errors.New("book: id главы не задан")
	}

	var text string
	err := h.QueryRow(
		`SELECT b.text
		 FROM book_text b
		 JOIN book_chapters c ON c.id = b.chapter_id
		 WHERE b.chapter_id = ? AND b.published = 1 AND c.status = 'published'
		 ORDER BY b.version DESC LIMIT 1`,
		chapterID).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrChapterNotPublished
	}
	if err != nil {
		return "", err
	}
	return text, nil
}
