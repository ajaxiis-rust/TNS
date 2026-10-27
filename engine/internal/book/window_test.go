package book

import (
	"reflect"
	"testing"
)

// TestWindow — «Проверка» T-07-10: кнопка BOOK и вкладка «История» возвращают
// один и тот же состав окна; вкладок ровно четыре; идущая глава в данных окна
// отсутствует; без обработанных глав окно отдаёт состояние no_digest.
func TestWindow(t *testing.T) {
	t.Run("кнопка BOOK и вкладка История возвращают один состав", func(t *testing.T) {
		world := newBookWorld(t)
		seedAndPublishChapter(t, world, insertOpenSession(t, world), 1, "Глава 1")

		fromBook, err := OpenWindow(world, EntryBookButton)
		if err != nil {
			t.Fatalf("OpenWindow(book): %v", err)
		}
		fromHistory, err := OpenWindow(world, EntryHistoryTab)
		if err != nil {
			t.Fatalf("OpenWindow(history): %v", err)
		}

		if !reflect.DeepEqual(fromBook, fromHistory) {
			t.Fatalf("состав окна зависит от точки входа:\nbook=%+v\nhistory=%+v", fromBook, fromHistory)
		}
	})

	t.Run("вкладок ровно четыре", func(t *testing.T) {
		tabs := WindowTabs()
		if len(tabs) != 4 {
			t.Fatalf("вкладок = %d, want 4", len(tabs))
		}
		want := []string{"Содержание", "Глава", "Правки Цензора", "Сырой лог"}
		for i, label := range want {
			if tabs[i].Label != label {
				t.Fatalf("вкладка %d = %q, want %q", i, tabs[i].Label, label)
			}
		}
	})

	t.Run("идущая глава в данных окна отсутствует", func(t *testing.T) {
		world := newBookWorld(t)

		// Идущая (открытая) глава — живой ход виден в чате, не в книге.
		current := insertOpenChapter(t, world, insertOpenSession(t, world))

		// Обработанная и опубликованная глава.
		done := seedAndPublishChapter(t, world, insertOpenSession(t, world), 2, "Глава 2")

		win, err := OpenWindow(world, EntryBookButton)
		if err != nil {
			t.Fatalf("OpenWindow: %v", err)
		}

		for _, e := range win.Toc {
			if e.ChapterID == current.ID {
				t.Fatalf("идущая глава %d попала в содержание", current.ID)
			}
		}
		if win.Chapter == nil || win.Chapter.ChapterID != done.ID {
			t.Fatalf("вкладка «Глава» = %+v, want главу %d", win.Chapter, done.ID)
		}
	})

	t.Run("без обработанных глав — no_digest", func(t *testing.T) {
		world := newBookWorld(t)
		_ = insertOpenChapter(t, world, insertOpenSession(t, world))

		win, err := OpenWindow(world, EntryBookButton)
		if err != nil {
			t.Fatalf("OpenWindow: %v", err)
		}
		if win.DigestState != DigestNone {
			t.Fatalf("DigestState = %q, want %q", win.DigestState, DigestNone)
		}
		if win.Digest != "" {
			t.Fatalf("Digest = %q, want пусто", win.Digest)
		}
		if win.Chapter != nil {
			t.Fatalf("Chapter = %+v, want nil", win.Chapter)
		}
	})

	t.Run("с обработанной главой полоса несёт её дайджест", func(t *testing.T) {
		world := newBookWorld(t)
		seedAndPublishChapter(t, world, insertOpenSession(t, world), 1, "Глава 1")

		win, err := OpenWindow(world, EntryBookButton)
		if err != nil {
			t.Fatalf("OpenWindow: %v", err)
		}
		if win.DigestState != DigestShown {
			t.Fatalf("DigestState = %q, want %q", win.DigestState, DigestShown)
		}
		if win.Digest != gateDigest {
			t.Fatalf("Digest = %q, want %q", win.Digest, gateDigest)
		}
	})
}
