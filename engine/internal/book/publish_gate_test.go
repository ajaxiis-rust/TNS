package book

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
)

// Фикстуры главы для гейта публикации (T-07-07).
const (
	gateEditorText = "Иван пришёл.\nОн сел у огня.\nНочь опустилась на город."
	gatePlayerText = "Иван пришёл.\nОн сел у костра.\nНочь опустилась на город."
	gateDigest     = "Первое предложение. Второе предложение. Третье предложение. Четвёртое предложение. Пятое предложение. Шестое предложение."
)

// insertPlayerVersion помечает текст главы правкой игрока, прошедшей Цензора
// (edited_by='player', T-07-05).
func insertPlayerVersion(t *testing.T, h *db.Handle, chapterID int64, text string) {
	t.Helper()
	if _, err := h.Exec("book_text",
		`INSERT INTO book_text (chapter_id, version, text, edited_by, published) VALUES (?, 3, ?, 'player', 0)`,
		chapterID, text); err != nil {
		t.Fatalf("INSERT book_text (player): %v", err)
	}
}

// setChapterDigest записывает выпущенный дайджест главы (T-07-06).
func setChapterDigest(t *testing.T, h *db.Handle, chapterID int64, digest string) {
	t.Helper()
	if _, err := h.Exec("book_chapters",
		`UPDATE book_chapters SET digest = ? WHERE id = ?`, digest, chapterID); err != nil {
		t.Fatalf("UPDATE book_chapters.digest: %v", err)
	}
}

// seedReadyChapter готовит главу, прошедшую полную обработку (закрытие,
// Редактор, правки через Цензора, дайджест), но ещё не опубликованную.
func seedReadyChapter(t *testing.T, h *db.Handle) Chapter {
	t.Helper()
	sessionID := insertOpenSession(t, h)
	ch := insertOpenChapter(t, h, sessionID)
	insertJournal(t, h, sessionID)

	if err := ch.Close(h, "2026-10-27T22:00:00Z"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := AssembleChapter(h, ch); err != nil {
		t.Fatalf("AssembleChapter: %v", err)
	}
	insertEditorVersion(t, h, ch.ID, gateEditorText)
	insertPlayerVersion(t, h, ch.ID, gatePlayerText)
	setChapterDigest(t, h, ch.ID, gateDigest)
	return ch
}

// Три представления книги (T-07-08, T-07-10): в них попадают только
// опубликованные главы. Здесь они проверяются сырыми запросами — сами окна
// реализуются следующими задачами волны.

// tocHas — глава в содержании (список только обработанных глав).
func tocHas(h *db.Handle, chapterID int64) bool {
	return exists(h, `SELECT 1 FROM book_chapters WHERE id = ? AND status = 'published'`, chapterID)
}

// chapterTabText — текст вкладки «Глава»: действующая издательская версия
// опубликованной главы.
func chapterTabText(h *db.Handle, chapterID int64) (string, bool) {
	var text string
	err := h.QueryRow(
		`SELECT b.text FROM book_text b JOIN book_chapters c ON c.id = b.chapter_id
		 WHERE b.chapter_id = ? AND b.published = 1 AND c.status = 'published'`,
		chapterID).Scan(&text)
	return text, err == nil
}

// digestStrip — полоса дайджеста: дайджест последней обработанной главы.
func digestStrip(h *db.Handle, chapterID int64) (string, bool) {
	var digest sql.NullString
	err := h.QueryRow(`SELECT digest FROM book_chapters WHERE id = ? AND status = 'published'`, chapterID).Scan(&digest)
	if err != nil || !digest.Valid || digest.String == "" {
		return "", false
	}
	return digest.String, true
}

func decodeVec(blob []byte) []float32 {
	vec := make([]float32, len(blob)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return vec
}

// TestPublishGate — «Проверка» T-07-07.
func TestPublishGate(t *testing.T) {
	t.Run("ложен без закрытия", func(t *testing.T) {
		world := newBookWorld(t)
		sessionID := insertOpenSession(t, world)
		ch := insertOpenChapter(t, world, sessionID)
		insertEditorVersion(t, world, ch.ID, gateEditorText)
		insertPlayerVersion(t, world, ch.ID, gatePlayerText)
		setChapterDigest(t, world, ch.ID, gateDigest)

		if CanPublish(world, ch.ID) {
			t.Fatal("CanPublish = true при открытой главе, want false")
		}
		if err := Process(context.Background(), world, testkit.NewFakeEmbed(), ch.ID); !errors.Is(err, ErrChapterNotProcessed) {
			t.Fatalf("Process(открытая) = %v, want ErrChapterNotProcessed", err)
		}
	})

	t.Run("ложен без Редактора", func(t *testing.T) {
		world := newBookWorld(t)
		sessionID := insertOpenSession(t, world)
		ch := insertOpenChapter(t, world, sessionID)
		if err := ch.Close(world, "2026-10-27T22:00:00Z"); err != nil {
			t.Fatalf("Close: %v", err)
		}
		insertPlayerVersion(t, world, ch.ID, gatePlayerText)
		setChapterDigest(t, world, ch.ID, gateDigest)

		if CanPublish(world, ch.ID) {
			t.Fatal("CanPublish = true без обработки Редактором, want false")
		}
		if err := Process(context.Background(), world, testkit.NewFakeEmbed(), ch.ID); !errors.Is(err, ErrChapterNotProcessed) {
			t.Fatalf("Process(без Редактора) = %v, want ErrChapterNotProcessed", err)
		}
	})

	t.Run("ложен без Цензора", func(t *testing.T) {
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
		insertEditorVersion(t, world, ch.ID, gateEditorText)
		setChapterDigest(t, world, ch.ID, gateDigest)

		if CanPublish(world, ch.ID) {
			t.Fatal("CanPublish = true без правок через Цензора, want false")
		}
		if err := Process(context.Background(), world, testkit.NewFakeEmbed(), ch.ID); !errors.Is(err, ErrChapterNotProcessed) {
			t.Fatalf("Process(без Цензора) = %v, want ErrChapterNotProcessed", err)
		}
	})

	t.Run("ложен без дайджеста", func(t *testing.T) {
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
		insertEditorVersion(t, world, ch.ID, gateEditorText)
		insertPlayerVersion(t, world, ch.ID, gatePlayerText)

		if CanPublish(world, ch.ID) {
			t.Fatal("CanPublish = true без дайджеста, want false")
		}
		if err := Process(context.Background(), world, testkit.NewFakeEmbed(), ch.ID); !errors.Is(err, ErrChapterNotProcessed) {
			t.Fatalf("Process(без дайджеста) = %v, want ErrChapterNotProcessed", err)
		}
	})

	t.Run("истинен при всех условиях", func(t *testing.T) {
		world := newBookWorld(t)
		ch := seedReadyChapter(t, world)
		if !CanPublish(world, ch.ID) {
			t.Fatal("CanPublish = false при полной обработке, want true")
		}
	})

	t.Run("до обработки нет в книге, после — есть; повтор без дублей; вектор 1024", func(t *testing.T) {
		world := newBookWorld(t)
		ch := seedReadyChapter(t, world)

		// До публикации главы в книге нет нигде.
		if tocHas(world, ch.ID) {
			t.Fatal("глава в содержании до обработки")
		}
		if text, ok := chapterTabText(world, ch.ID); ok {
			t.Fatalf("глава во вкладке «Глава» до обработки: %q", text)
		}
		if d, ok := digestStrip(world, ch.ID); ok {
			t.Fatalf("дайджест в полосе до обработки: %q", d)
		}

		embed := testkit.NewFakeEmbed()
		if err := Process(context.Background(), world, embed, ch.ID); err != nil {
			t.Fatalf("Process: %v", err)
		}

		// После публикации глава видна во всех трёх представлениях.
		if !tocHas(world, ch.ID) {
			t.Fatal("глава отсутствует в содержании после обработки")
		}
		if text, ok := chapterTabText(world, ch.ID); !ok || text != gatePlayerText {
			t.Fatalf("вкладка «Глава» = (%q, %v), want (%q, true)", text, ok, gatePlayerText)
		}
		if d, ok := digestStrip(world, ch.ID); !ok || d != gateDigest {
			t.Fatalf("полоса дайджеста = (%q, %v), want (%q, true)", d, ok, gateDigest)
		}

		var status string
		if err := world.QueryRow(`SELECT status FROM book_chapters WHERE id = ?`, ch.ID).Scan(&status); err != nil {
			t.Fatalf("SELECT status: %v", err)
		}
		if status != string(StatusPublished) {
			t.Fatalf("status = %q, want published", status)
		}

		// Вектор смысла главы: owner_table='book_chapters', dim=1024, вектор —
		// эмбеддинг последней (опубликованной) версии текста.
		var ownerTable, sourceText string
		var dim int
		var blob []byte
		if err := world.QueryRow(
			`SELECT owner_table, dim, source_text, vec FROM vectors WHERE owner_table = 'book_chapters' AND owner_id = ?`,
			ch.ID).Scan(&ownerTable, &dim, &sourceText, &blob); err != nil {
			t.Fatalf("SELECT vectors: %v", err)
		}
		if ownerTable != "book_chapters" {
			t.Fatalf("owner_table = %q, want book_chapters", ownerTable)
		}
		if dim != 1024 {
			t.Fatalf("dim = %d, want 1024", dim)
		}
		if sourceText != gatePlayerText {
			t.Fatalf("source_text вектора = %q, want %q", sourceText, gatePlayerText)
		}
		wantVec, err := embed.Embed(context.Background(), gatePlayerText)
		if err != nil {
			t.Fatalf("Embed: %v", err)
		}
		gotVec := decodeVec(blob)
		if len(gotVec) != 1024 {
			t.Fatalf("длина вектора = %d, want 1024", len(gotVec))
		}
		for i := range gotVec {
			if gotVec[i] != wantVec[i] {
				t.Fatalf("vec[%d] = %v, want %v", i, gotVec[i], wantVec[i])
			}
		}

		// Повторный Process ничего не делает и дублей не создаёт.
		var textsBefore, vectorsBefore int
		if err := world.QueryRow(`SELECT COUNT(*) FROM book_text WHERE chapter_id = ?`, ch.ID).Scan(&textsBefore); err != nil {
			t.Fatalf("COUNT book_text: %v", err)
		}
		if err := world.QueryRow(`SELECT COUNT(*) FROM vectors WHERE owner_table = 'book_chapters' AND owner_id = ?`, ch.ID).Scan(&vectorsBefore); err != nil {
			t.Fatalf("COUNT vectors: %v", err)
		}

		if err := Process(context.Background(), world, embed, ch.ID); err != nil {
			t.Fatalf("повторный Process: %v", err)
		}

		var textsAfter, vectorsAfter int
		if err := world.QueryRow(`SELECT COUNT(*) FROM book_text WHERE chapter_id = ?`, ch.ID).Scan(&textsAfter); err != nil {
			t.Fatalf("COUNT book_text: %v", err)
		}
		if err := world.QueryRow(`SELECT COUNT(*) FROM vectors WHERE owner_table = 'book_chapters' AND owner_id = ?`, ch.ID).Scan(&vectorsAfter); err != nil {
			t.Fatalf("COUNT vectors: %v", err)
		}
		if textsAfter != textsBefore {
			t.Fatalf("строк book_text после повтора = %d, want %d", textsAfter, textsBefore)
		}
		if vectorsAfter != vectorsBefore {
			t.Fatalf("векторов после повтора = %d, want %d", vectorsAfter, vectorsBefore)
		}
		if d, ok := digestStrip(world, ch.ID); !ok || d != gateDigest {
			t.Fatalf("дайджест после повтора = (%q, %v), want (%q, true)", d, ok, gateDigest)
		}
	})
}
