package book

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"

	"tns65/internal/db"
	"tns65/internal/models"
)

// Публикация главы (T-07-07, CB-09): глава появляется в книге только после
// полной обработки — закрытие, Редактор, правки через Цензора, дайджест. До
// этого ни текста, ни дайджеста, ни заглушки; заглушек код не создаёт.

// ErrChapterNotProcessed — глава не прошла полную обработку и не публикуется.
var ErrChapterNotProcessed = errors.New("book: глава не прошла полную обработку")

const (
	chapterVectorTable = "book_chapters" // owner_table вектора главы (вход дрейфа T-07-09).
	chapterVectorModel = "bge-m3"        // эмбеддинг-модель текущей реализации (D-16.1).
	chapterVectorLang  = "ru"
)

// exists сообщает, вернул ли запрос хотя бы одну строку; ошибки чтения
// трактуются как отсутствие — предикат публикации не публикует при сбое.
func exists(h *db.Handle, query string, args ...any) bool {
	if h == nil {
		return false
	}
	var one int
	err := h.QueryRow(query, args...).Scan(&one)
	return err == nil
}

// CanPublish — единственный предикат публикации (шаг 1). Проверяет ровно четыре
// условия: глава закрыта; текст обработан Редактором; правки игрока прогнаны
// через Цензора; дайджест выпущен. Любой сбой чтения — ложь.
func CanPublish(h *db.Handle, chapterID int64) bool {
	if h == nil || chapterID <= 0 {
		return false
	}

	// Глава закрыта: статус уже не open (closed → processing → published).
	var status string
	if err := h.QueryRow(`SELECT status FROM book_chapters WHERE id = ?`, chapterID).Scan(&status); err != nil {
		return false
	}
	if status == string(StatusOpen) {
		return false
	}

	// Текст обработан Редактором (T-05-08): версия edited_by='editor'.
	if !exists(h, `SELECT 1 FROM book_text WHERE chapter_id = ? AND edited_by = 'editor'`, chapterID) {
		return false
	}

	// Правки прогнаны через Цензора (T-07-05): версия edited_by='player'.
	if !exists(h, `SELECT 1 FROM book_text WHERE chapter_id = ? AND edited_by = 'player'`, chapterID) {
		return false
	}

	// Дайджест выпущен (T-07-06).
	var digest sql.NullString
	if err := h.QueryRow(`SELECT digest FROM book_chapters WHERE id = ?`, chapterID).Scan(&digest); err != nil {
		return false
	}
	if !digest.Valid || strings.TrimSpace(digest.String) == "" {
		return false
	}

	return true
}

// Process — идемпотентный оркестратор публикации (шаг 2): доводит главу
// closed → processing → published, помечает действующую издательскую версию в
// book_text.published и пишет вектор смысла главы в vectors (owner_table=
// 'book_chapters', вход для дрейфа заявки T-07-09). Повторный вызов на
// опубликованной главе ничего не делает и дублей не создаёт.
func Process(ctx context.Context, h *db.Handle, embedder models.Embedder, chapterID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if h == nil {
		return errors.New("book: нет хэндла мира")
	}
	if chapterID <= 0 {
		return errors.New("book: id главы не задан")
	}
	if embedder == nil {
		return errors.New("book: эмбеддер не подключён")
	}

	var status string
	err := h.QueryRow(`SELECT status FROM book_chapters WHERE id = ?`, chapterID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("book: глава не найдена")
	}
	if err != nil {
		return err
	}

	if status == string(StatusPublished) {
		return nil
	}
	if status != string(StatusClosed) && status != string(StatusProcessing) {
		return ErrChapterNotProcessed
	}
	if !CanPublish(h, chapterID) {
		return ErrChapterNotProcessed
	}

	// Вектор смысла главы — по тексту последней (действующей) версии.
	var version int64
	var text string
	if err := h.QueryRow(`SELECT version, text FROM book_text WHERE chapter_id = ? ORDER BY version DESC LIMIT 1`, chapterID).Scan(&version, &text); err != nil {
		return err
	}

	vec, err := embedder.Embed(ctx, text)
	if err != nil {
		return err
	}
	blob, err := chapterVectorBlob(vec)
	if err != nil {
		return err
	}

	return h.Tx(db.WriterCore, func(tx *db.Tx) error {
		if status == string(StatusClosed) {
			res, err := tx.Exec("book_chapters",
				`UPDATE book_chapters SET status = 'processing' WHERE id = ? AND status = 'closed'`, chapterID)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil {
				return err
			} else if n == 0 {
				return ErrChapterNotProcessed
			}
		}

		// Вектор смысла главы — один на главу (идемпотентно).
		if _, err := tx.Exec("vectors",
			`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at)
			 SELECT ?, ?, ?, ?, ?, ?, ?, ?
			 WHERE NOT EXISTS (SELECT 1 FROM vectors WHERE owner_table = ? AND owner_id = ?)`,
			chapterVectorTable, chapterID, chapterVectorModel, models.EmbedDim, chapterVectorLang, text, blob, nowUTC(),
			chapterVectorTable, chapterID); err != nil {
			return err
		}

		// Действующая издательская версия — последняя версия текста.
		res, err := tx.Exec("book_text",
			`UPDATE book_text SET published = 1 WHERE chapter_id = ? AND version = ?`, chapterID, version)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return ErrNoVersion
		}
		if _, err := tx.Exec("book_text",
			`UPDATE book_text SET published = 0 WHERE chapter_id = ? AND version != ?`, chapterID, version); err != nil {
			return err
		}

		// processing → published.
		if _, err := tx.Exec("book_chapters",
			`UPDATE book_chapters SET status = 'published' WHERE id = ? AND status = 'processing'`, chapterID); err != nil {
			return err
		}
		return nil
	})
}

// chapterVectorBlob кодирует вектор смысла главы в BLOB Float32 little-endian
// (D-17.2), размерность строго models.EmbedDim.
func chapterVectorBlob(vec []float32) ([]byte, error) {
	if len(vec) != models.EmbedDim {
		return nil, fmt.Errorf("book: размерность вектора главы %d, want %d", len(vec), models.EmbedDim)
	}
	buf := make([]byte, len(vec)*4)
	for i, f := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf, nil
}
