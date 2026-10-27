package webapi

import (
	"tns65/internal/book"
	"tns65/internal/db"
)

// BookWindow — JSON-ответ окна «Моя книга» (T-07-10, шаг 6): данные четырёх
// вкладок и полоса дайджеста. WindowID — идентификатор модального окна; оба
// входа (кнопка BOOK в чате и вкладка «История» верхнего меню) ведут на одно
// окно (T-07-11). Контракт маршрута зафиксирован в ROUTES.md (T-08-02);
// рендер вкладок — P7.
type BookWindow struct {
	WindowID    string            `json:"window_id"`
	Tabs        []BookTab         `json:"tabs"`
	DigestState string            `json:"digest_state"`
	Digest      string            `json:"digest,omitempty"`
	Toc         []BookTocEntry    `json:"toc"`
	Chapter     *BookChapter      `json:"chapter"`
	CensorCuts  []BookCensorCut   `json:"censor_cuts"`
	RawLog      []BookRawLogEntry `json:"raw_log"`
}

// BookTab — вкладка окна: машиночитаемый id и видимое имя.
type BookTab struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// BookTocEntry — строка вкладки «Содержание».
type BookTocEntry struct {
	ChapterID int64  `json:"chapter_id"`
	Day       int64  `json:"day"`
	Title     string `json:"title"`
}

// BookChapter — вкладка «Глава»: последняя обработанная глава.
type BookChapter struct {
	ChapterID int64  `json:"chapter_id"`
	Day       int64  `json:"day"`
	Title     string `json:"title"`
	Text      string `json:"text"`
}

// BookCensorCut — строка вкладки «Правки Цензора».
type BookCensorCut struct {
	ChapterID int64  `json:"chapter_id"`
	Text      string `json:"text"`
	Reason    string `json:"reason"`
	Status    string `json:"status"`
}

// BookRawLogEntry — строка вкладки «Сырой лог».
type BookRawLogEntry struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// LoadBookWindow собирает окно «Моя книга» для мира h и точки входа entry.
func LoadBookWindow(h *db.Handle, entry book.WindowEntry) (BookWindow, error) {
	win, err := book.OpenWindow(h, entry)
	if err != nil {
		return BookWindow{}, err
	}
	return bookWindowDTO(win), nil
}

// bookWindowDTO — проекция доменной модели окна на JSON-контракт webapi.
func bookWindowDTO(w book.Window) BookWindow {
	tabs := make([]BookTab, 0, len(w.Tabs))
	for _, t := range w.Tabs {
		tabs = append(tabs, BookTab{ID: t.ID, Label: t.Label})
	}
	toc := make([]BookTocEntry, 0, len(w.Toc))
	for _, e := range w.Toc {
		toc = append(toc, BookTocEntry{ChapterID: e.ChapterID, Day: e.Day, Title: e.Title})
	}
	cuts := make([]BookCensorCut, 0, len(w.CensorCuts))
	for _, c := range w.CensorCuts {
		cuts = append(cuts, BookCensorCut{ChapterID: c.ChapterID, Text: c.Text, Reason: c.Reason, Status: c.Status})
	}
	raw := make([]BookRawLogEntry, 0, len(w.RawLog))
	for _, r := range w.RawLog {
		raw = append(raw, BookRawLogEntry{Kind: r.Kind, Text: r.Text})
	}

	out := BookWindow{
		WindowID:    book.BookWindowID,
		Tabs:        tabs,
		DigestState: string(w.DigestState),
		Digest:      w.Digest,
		Toc:         toc,
		CensorCuts:  cuts,
		RawLog:      raw,
	}
	if w.Chapter != nil {
		out.Chapter = &BookChapter{
			ChapterID: w.Chapter.ChapterID,
			Day:       w.Chapter.Day,
			Title:     w.Chapter.Title,
			Text:      w.Chapter.Text,
		}
	}
	return out
}
