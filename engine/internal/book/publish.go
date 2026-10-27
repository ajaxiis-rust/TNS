package book

import (
	"database/sql"
	"errors"

	"tns65/internal/db"
)

// AssembleVersion — номер версии, собранной кодом из журнала сессии (T-07-04,
// шаг 2): версия 1 — вход Редактора (T-05-08) и правок игрока (T-07-05).
const AssembleVersion = 1

// Publication — действующая издательская версия главы (CB-11.1): обработанный
// текст из book_text и дайджест из book_chapters; оба читаются из
// world(<slug>).db (T-07-04, шаг 5).
type Publication struct {
	Text   string
	Digest string
}

// ErrNoVersion — версия текста главы не найдена.
var ErrNoVersion = errors.New("book: версия текста главы не найдена")

// ErrNoPublished — действующая издательская версия главы не найдена.
var ErrNoPublished = errors.New("book: издательская версия главы не найдена")

// AssembleChapter собирает издательскую версию 1 главы (T-07-04, шаг 2): текст
// из журнала сессии (turns, scenes) дословно и без правок в book_text с
// version=1, edited_by=NULL, published=0. Журнал сессии не правится. Сборка
// идемпотентна: повторная сборка той же главы не создаёт вторую версию 1.
func AssembleChapter(h *db.Handle, ch Chapter) error {
	if h == nil {
		return errors.New("book: нет хэндла мира")
	}
	if ch.ID <= 0 {
		return errors.New("book: id главы не задан")
	}
	if ch.SessionID <= 0 {
		return errors.New("book: id сессии главы не задан")
	}

	// Идемпотентность: версия 1 уже собрана кодом.
	var one int
	err := h.QueryRow(
		`SELECT 1 FROM book_text WHERE chapter_id = ? AND version = ? AND edited_by IS NULL LIMIT 1`,
		ch.ID, AssembleVersion).Scan(&one)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	text, err := ch.Text(h)
	if err != nil {
		return err
	}

	_, err = h.Exec("book_text",
		`INSERT INTO book_text (chapter_id, version, text, edited_by, published) VALUES (?, ?, ?, NULL, 0)`,
		ch.ID, AssembleVersion, text)
	return err
}

// Publish помечает версию version главы действующей издательской версией
// (T-07-04, шаг 4): published=1 у выбранной версии, published=0 у остальных.
// Повторная пометка идемпотентна.
func Publish(h *db.Handle, chapterID, version int64) error {
	if h == nil {
		return errors.New("book: нет хэндла мира")
	}
	if chapterID <= 0 {
		return errors.New("book: id главы не задан")
	}
	if version <= 0 {
		return errors.New("book: версия текста не задана")
	}

	return h.Tx(db.WriterCore, func(tx *db.Tx) error {
		res, err := tx.Exec("book_text",
			`UPDATE book_text SET published = 1 WHERE chapter_id = ? AND version = ?`,
			chapterID, version)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return ErrNoVersion
		}

		if _, err := tx.Exec("book_text",
			`UPDATE book_text SET published = 0 WHERE chapter_id = ? AND version != ?`,
			chapterID, version); err != nil {
			return err
		}
		return nil
	})
}

// LoadPublished читает действующую издательскую версию главы: текст из book_text
// (published=1) и дайджест из book_chapters — оба из world(<slug>).db
// (T-07-04, шаг 5).
func LoadPublished(h *db.Handle, chapterID int64) (Publication, error) {
	if h == nil {
		return Publication{}, errors.New("book: нет хэндла мира")
	}
	if chapterID <= 0 {
		return Publication{}, errors.New("book: id главы не задан")
	}

	var out Publication
	var digest sql.NullString
	err := h.QueryRow(
		`SELECT b.text, c.digest
		 FROM book_text b
		 JOIN book_chapters c ON c.id = b.chapter_id
		 WHERE b.chapter_id = ? AND b.published = 1
		 ORDER BY b.version DESC LIMIT 1`,
		chapterID).Scan(&out.Text, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return Publication{}, ErrNoPublished
	}
	if err != nil {
		return Publication{}, err
	}
	out.Digest = digest.String
	return out, nil
}
