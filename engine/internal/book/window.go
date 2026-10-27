package book

import (
	"database/sql"
	"errors"

	"tns65/internal/db"
)

// WindowEntry — точка входа в окно «Моя книга» (T-07-10, шаг 1): кнопка BOOK в
// чате и вкладка «История» верхнего меню — два ярлыка на одно и то же окно.
type WindowEntry string

const (
	EntryBookButton WindowEntry = "book"
	EntryHistoryTab WindowEntry = "history"
)

// WindowTab — вкладка окна «Моя книга» (CB-04.2).
type WindowTab struct {
	ID    string
	Label string
}

// WindowTabs — четыре вкладки окна в порядке CB-04.2: Содержание, Глава,
// Правки Цензора, Сырой лог. Порядок — контракт интерфейса (T-08-07).
func WindowTabs() []WindowTab {
	return []WindowTab{
		{ID: "toc", Label: "Содержание"},
		{ID: "chapter", Label: "Глава"},
		{ID: "censor", Label: "Правки Цензора"},
		{ID: "raw_log", Label: "Сырой лог"},
	}
}

// DigestState — состояние полосы дайджеста (CB-04.3).
type DigestState string

const (
	DigestNone  DigestState = "no_digest" // ни одной обработанной главы — полоса пуста
	DigestShown DigestState = "digest"    // дайджест последней обработанной главы
)

// ChapterView — вкладка «Глава»: последняя обработанная глава (CB-04.2).
type ChapterView struct {
	ChapterID int64
	Day       int64
	Title     string
	Text      string
}

// CensorCut — строка вкладки «Правки Цензора»: отклонённая правка из book_cuts.
type CensorCut struct {
	ChapterID int64
	Text      string
	Reason    string
	Status    string
}

// RawLogEntry — строка вкладки «Сырой лог»: реплика или сцена сессии без правок.
type RawLogEntry struct {
	Kind string // "player" | "reply" | "scene"
	Text string
}

// Window — данные окна «Моя книга»: четыре вкладки и полоса дайджеста.
type Window struct {
	Tabs        []WindowTab
	DigestState DigestState
	Digest      string
	Toc         []TocEntry
	Chapter     *ChapterView
	CensorCuts  []CensorCut
	RawLog      []RawLogEntry
}

// OpenWindow строит окно «Моя книга» (шаги 2–5). Состав не зависит от точки
// входа: обе точки входа — ярлык на одно окно (шаг 1).
func OpenWindow(h *db.Handle, entry WindowEntry) (Window, error) {
	if h == nil {
		return Window{}, errors.New("book: нет хэндла мира")
	}
	if entry != EntryBookButton && entry != EntryHistoryTab {
		return Window{}, errors.New("book: неизвестная точка входа окна")
	}

	toc, err := List(h)
	if err != nil {
		return Window{}, err
	}
	last, digest, err := lastPublished(h)
	if err != nil {
		return Window{}, err
	}
	cuts, err := censorCuts(h)
	if err != nil {
		return Window{}, err
	}
	raw, err := rawLog(h)
	if err != nil {
		return Window{}, err
	}

	win := Window{
		Tabs:       WindowTabs(),
		Toc:        toc,
		Chapter:    last,
		CensorCuts: cuts,
		RawLog:     raw,
	}
	if last == nil {
		win.DigestState = DigestNone
	} else {
		win.DigestState = DigestShown
		win.Digest = digest
	}
	return win, nil
}

// lastPublished — последняя обработанная глава (шаг 4): наибольший day среди
// опубликованных глав, текст — действующая издательская версия (published=1).
// Идущая (open) и неопубликованная главы сюда не попадают.
func lastPublished(h *db.Handle) (*ChapterView, string, error) {
	var id, day int64
	var title, text string
	var digest sql.NullString
	err := h.QueryRow(
		`SELECT c.id, c.day, c.title, c.digest, b.text
		 FROM book_chapters c
		 JOIN book_text b ON b.chapter_id = c.id
		 WHERE c.status = 'published' AND b.published = 1
		 ORDER BY c.day DESC, c.id DESC
		 LIMIT 1`).Scan(&id, &day, &title, &digest, &text)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return &ChapterView{ChapterID: id, Day: day, Title: title, Text: text}, digest.String, nil
}

// censorCuts — правки Цензора (шаг 2, вкладка «Правки Цензора»): отклонённые
// правки из book_cuts в порядке поступления.
func censorCuts(h *db.Handle) ([]CensorCut, error) {
	rows, err := h.Query(`SELECT chapter_id, cut_text, reason, status FROM book_cuts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CensorCut
	for rows.Next() {
		var c CensorCut
		if err := rows.Scan(&c.ChapterID, &c.Text, &c.Reason, &c.Status); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// rawLog — сырой лог сессии (шаг 5): реплики ходов и сцены без правок, к главам
// не относится. Порядок: ходы по id (реплика, ответ), затем сцены по id.
func rawLog(h *db.Handle) ([]RawLogEntry, error) {
	var out []RawLogEntry

	rows, err := h.Query(`SELECT player_text, reply_text FROM turns ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var player, reply string
		if err := rows.Scan(&player, &reply); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out,
			RawLogEntry{Kind: "player", Text: player},
			RawLogEntry{Kind: "reply", Text: reply},
		)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	rows, err = h.Query(`SELECT prose FROM scenes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var prose string
		if err := rows.Scan(&prose); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, RawLogEntry{Kind: "scene", Text: prose})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	return out, nil
}
