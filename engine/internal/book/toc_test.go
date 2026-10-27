package book

import (
	"context"
	"errors"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
)

// insertChapterWithDay заводит открытую главу с заданным днём и заголовком.
func insertChapterWithDay(t *testing.T, h *db.Handle, sessionID int64, day int64, title string) Chapter {
	t.Helper()
	res, err := h.Exec("book_chapters",
		`INSERT INTO book_chapters (day, title, status, kind) VALUES (?, ?, 'open', 'chapter')`, day, title)
	if err != nil {
		t.Fatalf("INSERT book_chapters (day %d): %v", day, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("book_chapters id: %v", err)
	}
	return Chapter{ID: id, Day: day, Title: title, Status: StatusOpen, Kind: KindChapter, SessionID: sessionID}
}

// seedAndPublishChapter доводит главу через полную обработку и публикацию.
func seedAndPublishChapter(t *testing.T, h *db.Handle, sessionID int64, day int64, title string) Chapter {
	t.Helper()
	ch := insertChapterWithDay(t, h, sessionID, day, title)
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
	if err := Process(context.Background(), h, testkit.NewFakeEmbed(), ch.ID); err != nil {
		t.Fatalf("Process: %v", err)
	}
	return ch
}

// TestToc — «Проверка» T-07-08: список содержания пополняется только после
// обработки глав; выбор главы в списке возвращает её текст; необработанная глава
// в списке отсутствует.
func TestToc(t *testing.T) {
	t.Run("список пополняется только после обработки", func(t *testing.T) {
		world := newBookWorld(t)
		ch := seedReadyChapter(t, world)

		// До публикации содержание пусто.
		got, err := List(world)
		if err != nil {
			t.Fatalf("List до обработки: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("содержание до обработки = %v, want пусто", got)
		}

		// Выбор необработанной главы текста не даёт.
		if _, err := OpenChapterText(world, ch.ID); !errors.Is(err, ErrChapterNotPublished) {
			t.Fatalf("OpenChapterText(необработанная) = %v, want ErrChapterNotPublished", err)
		}

		if err := Process(context.Background(), world, testkit.NewFakeEmbed(), ch.ID); err != nil {
			t.Fatalf("Process: %v", err)
		}

		got, err = List(world)
		if err != nil {
			t.Fatalf("List после обработки: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("строк содержания = %d, want 1", len(got))
		}
		if got[0].ChapterID != ch.ID || got[0].Day != ch.Day || got[0].Title != ch.Title {
			t.Fatalf("строка содержания = %+v, want {ChapterID:%d Day:%d Title:%q}", got[0], ch.ID, ch.Day, ch.Title)
		}

		// Выбор главы в списке возвращает её текст — действующую издательскую версию.
		text, err := OpenChapterText(world, ch.ID)
		if err != nil {
			t.Fatalf("OpenChapterText: %v", err)
		}
		if text != gatePlayerText {
			t.Fatalf("текст выбранной главы = %q, want %q", text, gatePlayerText)
		}
	})

	t.Run("необработанная глава отсутствует", func(t *testing.T) {
		world := newBookWorld(t)

		done := seedReadyChapter(t, world)
		if err := Process(context.Background(), world, testkit.NewFakeEmbed(), done.ID); err != nil {
			t.Fatalf("Process: %v", err)
		}

		// Вторая глава только собрана из журнала, но не обработана.
		sessionID := insertOpenSession(t, world)
		raw := insertChapterWithDay(t, world, sessionID, 2, "Глава 2")
		insertJournal(t, world, sessionID)
		if err := raw.Close(world, "2026-10-27T23:00:00Z"); err != nil {
			t.Fatalf("Close(глава 2): %v", err)
		}
		if err := AssembleChapter(world, raw); err != nil {
			t.Fatalf("AssembleChapter(глава 2): %v", err)
		}

		got, err := List(world)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("строк содержания = %d, want 1 (только обработанная)", len(got))
		}
		if got[0].ChapterID != done.ID {
			t.Fatalf("в содержании глава %d, want %d", got[0].ChapterID, done.ID)
		}

		if _, err := OpenChapterText(world, raw.ID); !errors.Is(err, ErrChapterNotPublished) {
			t.Fatalf("OpenChapterText(необработанная) = %v, want ErrChapterNotPublished", err)
		}
	})

	t.Run("содержание упорядочено по day", func(t *testing.T) {
		world := newBookWorld(t)

		second := seedAndPublishChapter(t, world, insertOpenSession(t, world), 2, "Глава 2")
		first := seedAndPublishChapter(t, world, insertOpenSession(t, world), 1, "Глава 1")

		got, err := List(world)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("строк содержания = %d, want 2", len(got))
		}
		if got[0].ChapterID != first.ID || got[1].ChapterID != second.ID {
			t.Fatalf("порядок содержания = [%d %d], want [%d %d] по day",
				got[0].ChapterID, got[1].ChapterID, first.ID, second.ID)
		}
	})
}
