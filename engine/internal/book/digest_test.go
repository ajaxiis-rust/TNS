package book

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// digestSlot — слот Редактора со счётчиком вызовов только по роли дайджеста.
type digestSlot struct {
	inner       *testkit.FakeSlot
	digestCalls int
}

func (s *digestSlot) Generate(ctx context.Context, role, prompt string) (string, error) {
	if role == workshop.EditorDigestRole {
		s.digestCalls++
	}
	return s.inner.Generate(ctx, role, prompt)
}

// insertEditorVersion помечает текст главы обработанным Редактором (edited_by='editor').
func insertEditorVersion(t *testing.T, h *db.Handle, chapterID int64, text string) {
	t.Helper()
	if _, err := h.Exec("book_text",
		`INSERT INTO book_text (chapter_id, version, text, edited_by, published) VALUES (?, 2, ?, 'editor', 0)`,
		chapterID, text); err != nil {
		t.Fatalf("INSERT book_text (editor): %v", err)
	}
}

// TestDigest — «Проверка» T-07-06.
func TestDigest(t *testing.T) {
	// Сплиттер предложений не режет по `;` и `:` (отличие от резки T-03-02).
	if n := CountDigestSentences("Раз; два: три."); n != 1 {
		t.Fatalf("сплиттер режет по `; :`: предложений = %d, want 1", n)
	}
	if err := ValidateDigest("Раз. Два. Три."); !errors.Is(err, ErrDigestSentences) {
		t.Fatalf("ValidateDigest(3 предложения) = %v, want ErrDigestSentences", err)
	}

	world := newBookWorld(t)
	sessionID := insertOpenSession(t, world)
	ch := insertOpenChapter(t, world, sessionID)
	insertJournal(t, world, sessionID)

	if err := ch.Close(world, "2026-10-27T22:00:00Z"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := AssembleChapter(world, ch); err != nil {
		t.Fatalf("AssembleChapter: %v", err)
	}

	// Дайджест не появляется раньше обработки текста главы.
	var digestBefore sql.NullString
	if err := world.QueryRow(`SELECT digest FROM book_chapters WHERE id = ?`, ch.ID).Scan(&digestBefore); err != nil {
		t.Fatalf("SELECT digest: %v", err)
	}
	if digestBefore.Valid {
		t.Fatalf("дайджест появился до обработки текста: %q", digestBefore.String)
	}

	slot := &digestSlot{inner: testkit.NewFakeSlot()}
	if err := BuildDigest(context.Background(), world, slot, ch.ID); !errors.Is(err, ErrTextNotProcessed) {
		t.Fatalf("BuildDigest до обработки = %v, want ErrTextNotProcessed", err)
	}
	if slot.digestCalls != 0 {
		t.Fatalf("вызовов слота до обработки = %d, want 0", slot.digestCalls)
	}

	// Текст главы обработан Редактором (T-05-08).
	processedText := "Мы легли спать.\nСпокойной ночи.\nНочь опустилась на город."
	insertEditorVersion(t, world, ch.ID, processedText)

	digest := "Первое предложение. Второе предложение. Третье предложение. Четвёртое предложение. Пятое предложение. Шестое предложение."
	slot.inner.SetResponse(workshop.EditorDigestRole, digest)

	if err := BuildDigest(context.Background(), world, slot, ch.ID); err != nil {
		t.Fatalf("BuildDigest: %v", err)
	}

	// На одну главу счётчик вызовов слота Редактора для дайджеста равен 1.
	if slot.digestCalls != 1 {
		t.Fatalf("вызовов слота для дайджеста = %d, want 1", slot.digestCalls)
	}

	// Дайджест обработанной главы содержит от 5 до 6 предложений по сплиттеру.
	var stored sql.NullString
	if err := world.QueryRow(`SELECT digest FROM book_chapters WHERE id = ?`, ch.ID).Scan(&stored); err != nil {
		t.Fatalf("SELECT digest после обработки: %v", err)
	}
	if !stored.Valid || stored.String != digest {
		t.Fatalf("digest = %q, want %q", stored.String, digest)
	}
	if err := ValidateDigest(stored.String); err != nil {
		t.Fatalf("ValidateDigest: %v", err)
	}
	if n := CountDigestSentences(stored.String); n < DigestMinSentences || n > DigestMaxSentences {
		t.Fatalf("предложений в дайджесте = %d, want %d–%d", n, DigestMinSentences, DigestMaxSentences)
	}

	// Дайджест хранится только в book_chapters.digest: текст главы не изменился.
	var editorText string
	if err := world.QueryRow(`SELECT text FROM book_text WHERE chapter_id = ? AND edited_by = 'editor'`, ch.ID).Scan(&editorText); err != nil {
		t.Fatalf("SELECT book_text (editor): %v", err)
	}
	if editorText != processedText {
		t.Fatalf("текст главы изменён дайджестом: %q, want %q", editorText, processedText)
	}
}
