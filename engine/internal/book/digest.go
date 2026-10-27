package book

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"tns65/internal/db"
	"tns65/internal/workshop"
)

// Границы дайджеста (шаг 1): сводка игроку из 5–6 предложений.
const (
	DigestMinSentences = 5
	DigestMaxSentences = 6
)

// ErrDigestSentences — дайджест не в границах 5–6 предложений.
var ErrDigestSentences = errors.New("book: дайджест — не 5–6 предложений")

// ErrTextNotProcessed — дайджест выпускается только после обработки текста главы.
var ErrTextNotProcessed = errors.New("book: текст главы не обработан Редактором")

// isDigestDelimiter — терминатор предложения дайджеста (шаг 2): `.`, `!`, `?`, `…`.
// `;` и `:` терминаторами не являются — в отличие от резки T-03-02.
func isDigestDelimiter(r rune) bool {
	switch r {
	case '.', '!', '?', '…':
		return true
	}
	return false
}

// SplitDigestSentences режет дайджест на предложения по `. ! ? …` и отбрасывает
// пустые куски: непрерывные терминаторы считаются одним концом предложения.
func SplitDigestSentences(text string) []string {
	parts := strings.FieldsFunc(text, isDigestDelimiter)
	out := parts[:0]
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

// CountDigestSentences — машиночитаемое число предложений дайджеста (шаг 2).
func CountDigestSentences(text string) int {
	return len(SplitDigestSentences(text))
}

// ValidateDigest проверяет, что дайджест содержит 5–6 предложений по сплиттеру
// `. ! ? …`.
func ValidateDigest(text string) error {
	n := CountDigestSentences(text)
	if n < DigestMinSentences || n > DigestMaxSentences {
		return fmt.Errorf("%w: %d предложений, нужно %d–%d", ErrDigestSentences, n, DigestMinSentences, DigestMaxSentences)
	}
	return nil
}

// DigestSlot — слот Редактора для дайджеста; счётчик вызовов инжектируется
// тестом (шаг 3), на одну главу — один вызов.
type DigestSlot interface {
	Generate(ctx context.Context, role, prompt string) (string, error)
}

// processedChapterText — текст обработанной Редактором главы (edited_by='editor',
// последняя версия). Если такой версии нет, текст не обработан (шаг 3).
func processedChapterText(h *db.Handle, chapterID int64) (string, error) {
	var text string
	err := h.QueryRow(
		`SELECT text FROM book_text WHERE chapter_id = ? AND edited_by = 'editor' ORDER BY version DESC LIMIT 1`,
		chapterID).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrTextNotProcessed
	}
	if err != nil {
		return "", err
	}
	return text, nil
}

// digestPrompt — промпт дайджеста: тот же запрос Редактора, что и краткое
// содержание (T-05-08 шаг 6).
func digestPrompt(text string) string {
	raw, _ := json.Marshal(map[string]any{"task": workshop.EditorDigestRole, "text": text})
	return string(raw)
}

// WriteDigest записывает дайджест обработанной главы в book_chapters.digest —
// единственное место хранения (шаг 3) — писателем core. Отказ, если текст главы
// ещё не обработан или дайджест не 5–6 предложений.
func WriteDigest(h *db.Handle, chapterID int64, digest string) error {
	if h == nil {
		return errors.New("book: нет хэндла мира")
	}
	if chapterID <= 0 {
		return errors.New("book: id главы не задан")
	}
	if err := ValidateDigest(digest); err != nil {
		return err
	}
	if _, err := processedChapterText(h, chapterID); err != nil {
		return err
	}
	return h.Tx(db.WriterCore, func(tx *db.Tx) error {
		_, err := tx.Exec("book_chapters",
			`UPDATE book_chapters SET digest = ? WHERE id = ?`, digest, chapterID)
		return err
	})
}

// BuildDigest выпускает дайджест обработанной главы: один вызов слота Редактора
// (EditorDigestRole) на главу, проверка 5–6 предложений и запись в
// book_chapters.digest писателем core. Текст дайджеста пишет Редактор, в базу он
// напрямую не пишет.
func BuildDigest(ctx context.Context, h *db.Handle, slot DigestSlot, chapterID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if h == nil {
		return errors.New("book: нет хэндла мира")
	}
	if chapterID <= 0 {
		return errors.New("book: id главы не задан")
	}
	if slot == nil {
		return errors.New("book: слот Редактора не подключён")
	}

	text, err := processedChapterText(h, chapterID)
	if err != nil {
		return err
	}
	digest, err := slot.Generate(ctx, workshop.EditorDigestRole, digestPrompt(text))
	if err != nil {
		return err
	}
	return WriteDigest(h, chapterID, digest)
}
