package book

import (
	"reflect"
	"testing"
)

// TestTrigger — «Проверка» T-07-11: оба входа (кнопка BOOK в чате и вкладка
// «История» верхнего меню) приводят к одному модальному окну «Моя книга»
// с тем же составом вкладок.
func TestTrigger(t *testing.T) {
	bookTrigger := Trigger{Entry: EntryBookButton}
	historyTrigger := Trigger{Entry: EntryHistoryTab}

	t.Run("оба входа — один идентификатор окна", func(t *testing.T) {
		if bookTrigger.WindowID() != historyTrigger.WindowID() {
			t.Fatalf("идентификатор окна зависит от входа: book=%q history=%q",
				bookTrigger.WindowID(), historyTrigger.WindowID())
		}
		if bookTrigger.WindowID() != BookWindowID {
			t.Fatalf("идентификатор окна = %q, want %q", bookTrigger.WindowID(), BookWindowID)
		}
	})

	t.Run("состав вкладок одинаков для обоих входов", func(t *testing.T) {
		fromBook := bookTrigger.Tabs()
		fromHistory := historyTrigger.Tabs()
		if !reflect.DeepEqual(fromBook, fromHistory) {
			t.Fatalf("состав вкладок зависит от входа:\nbook=%+v\nhistory=%+v", fromBook, fromHistory)
		}
		if len(fromBook) != 4 {
			t.Fatalf("вкладок = %d, want 4", len(fromBook))
		}
	})

	t.Run("оба входа открывают одно окно с тем же составом вкладок", func(t *testing.T) {
		world := newBookWorld(t)

		fromBook, err := OpenWindow(world, EntryBookButton)
		if err != nil {
			t.Fatalf("OpenWindow(book): %v", err)
		}
		fromHistory, err := OpenWindow(world, EntryHistoryTab)
		if err != nil {
			t.Fatalf("OpenWindow(history): %v", err)
		}

		if !reflect.DeepEqual(fromBook.Tabs, fromHistory.Tabs) {
			t.Fatalf("состав вкладок окна зависит от входа:\nbook=%+v\nhistory=%+v",
				fromBook.Tabs, fromHistory.Tabs)
		}
		if !reflect.DeepEqual(fromBook.Tabs, bookTrigger.Tabs()) {
			t.Fatalf("вкладки окна не совпадают с вкладками триггера:\nокно=%+v\nтриггер=%+v",
				fromBook.Tabs, bookTrigger.Tabs())
		}
	})
}
