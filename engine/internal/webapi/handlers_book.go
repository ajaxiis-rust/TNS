package webapi

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"tns65/internal/book"
	"tns65/internal/db"
)

// Окно «Моя книга» (T-08-07, UI-06): чтение глав, дайджеста, правок Цензора и
// сырого лога, а также выгрузка MD. Писатель: нет записи — хэндлы открываются
// только на чтение; экспорт пишет файл в папку мира, базы не трогает.

const (
	bookWindowPath   = "/api/book/window"
	bookDigestPath   = "/api/book/digest"
	bookCutsPath     = "/api/book/censor-cuts"
	bookExportPath   = "/api/book/export"
	exportScopeBook  = "book"
	exportScopeChapt = "chapter"
)

// bookRoot — корень миров для выгрузки; в тестах подменяется t.TempDir().
var bookRoot = db.WorldsRoot

// openBookReader открывает оперативную базу мира только на чтение.
func openBookReader(slug string) (*db.Handle, error) {
	return db.OpenReaderAs(worldDBPath(slug), db.DBWorld, db.WriterCore, nil)
}

// bookWindowResponse — JSON-контракт окна: данные четырёх вкладок и полосы
// дайджеста. Совпадает с bookWindowDTO, но censor_cuts несёт «было → стало».
type bookWindowResponse struct {
	WindowID    string            `json:"window_id"`
	Tabs        []BookTab         `json:"tabs"`
	DigestState string            `json:"digest_state"`
	Digest      string            `json:"digest,omitempty"`
	Toc         []BookTocEntry    `json:"toc"`
	Chapter     *BookChapter      `json:"chapter"`
	CensorCuts  []bookCutResponse `json:"censor_cuts"`
	RawLog      []BookRawLogEntry `json:"raw_log"`
}

// bookCutResponse — одна правка Цензора «было → стало» (применённая или
// отклонённая).
type bookCutResponse struct {
	ChapterID int64  `json:"chapter_id"`
	Before    string `json:"before,omitempty"`
	After     string `json:"after,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Status    string `json:"status"`
}

// bookDigestResponse — полоса дайджеста окна.
type bookDigestResponse struct {
	World       string `json:"world"`
	DigestState string `json:"digest_state"`
	Digest      string `json:"digest,omitempty"`
}

// bookCutsResponse — вкладка «Правки Цензора».
type bookCutsResponse struct {
	World string            `json:"world"`
	Cuts  []bookCutResponse `json:"cuts"`
}

// bookExportResponse — результат выгрузки MD.
type bookExportResponse struct {
	World  string `json:"world"`
	Scope  string `json:"scope"`
	Format string `json:"format"`
	Path   string `json:"path"`
}

// handleBookWindow — GET /api/book/window?world=<slug>: данные окна «Моя книга».
func handleBookWindow(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("world"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}

	h, err := openBookReader(slug)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	defer h.Close()

	win, err := book.OpenWindow(h, book.EntryHistoryTab)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "book window failed")
		return
	}

	cuts, err := readBookCuts(h)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "censor cuts failed")
		return
	}

	out := bookWindowResponse{
		WindowID:    book.BookWindowID,
		Tabs:        tabsDTO(win.Tabs),
		DigestState: string(win.DigestState),
		Digest:      win.Digest,
		Toc:         tocDTO(win.Toc),
		CensorCuts:  cuts,
		RawLog:      rawLogDTO(win.RawLog),
	}
	if win.Chapter != nil {
		out.Chapter = &BookChapter{
			ChapterID: win.Chapter.ChapterID,
			Day:       win.Chapter.Day,
			Title:     win.Chapter.Title,
			Text:      win.Chapter.Text,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleBookDigest — GET /api/book/digest?world=<slug>: полоса дайджеста окна.
func handleBookDigest(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("world"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}

	h, err := openBookReader(slug)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	defer h.Close()

	win, err := book.OpenWindow(h, book.EntryHistoryTab)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "book window failed")
		return
	}
	writeJSON(w, http.StatusOK, bookDigestResponse{
		World:       slug,
		DigestState: string(win.DigestState),
		Digest:      win.Digest,
	})
}

// handleBookCuts — GET /api/book/censor-cuts?world=<slug>: правки Цензора.
func handleBookCuts(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("world"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}

	h, err := openBookReader(slug)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	defer h.Close()

	cuts, err := readBookCuts(h)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "censor cuts failed")
		return
	}
	writeJSON(w, http.StatusOK, bookCutsResponse{World: slug, Cuts: cuts})
}

// handleBookExport — GET /api/book/export?world=<slug>&format=md&scope=book|chapter
// [&chapter=N]: выгрузка главы или всей книги в MD файлами <slug>-book.md и
// <slug>-chapter-<N>.md в <root>/worlds/<slug>/export/.
func handleBookExport(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("world"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	format := book.Format(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = book.FormatMarkdown
	}
	if format != book.FormatMarkdown {
		writeError(w, http.StatusBadRequest, codeBadRequest, "format not supported")
		return
	}
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	if scope == "" {
		scope = exportScopeBook
	}
	if scope != exportScopeBook && scope != exportScopeChapt {
		writeError(w, http.StatusBadRequest, codeBadRequest, "scope must be book or chapter")
		return
	}

	h, err := openBookReader(slug)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	defer h.Close()

	var filePath string
	if scope == exportScopeChapt {
		n, convErr := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("chapter")))
		if convErr != nil || n < 1 {
			writeError(w, http.StatusBadRequest, codeBadRequest, "chapter number required")
			return
		}
		filePath, err = book.ExportChapter(h, bookRoot, format, n)
	} else {
		filePath, err = book.ExportBook(h, bookRoot, format)
	}
	if err != nil {
		switch err {
		case book.ErrExportFormat, book.ErrChapterNotPublished:
			writeError(w, http.StatusBadRequest, codeBadRequest, err.Error())
		case book.ErrExportNoChapters:
			writeError(w, http.StatusBadRequest, codeBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, codeInternal, "export failed")
		}
		return
	}

	writeJSON(w, http.StatusOK, bookExportResponse{
		World:  slug,
		Scope:  scope,
		Format: string(format),
		Path:   filepath.Base(filePath),
	})
}

// readBookCuts читает правки Цензора из book_cuts в порядке поступления. Текст
// «было → стало» разбивается по стрелке; если стрелки нет, правка целиком идёт
// в After (статус rejected хранит только результат отклонённой правки).
func readBookCuts(h *db.Handle) ([]bookCutResponse, error) {
	rows, err := h.Query(`SELECT chapter_id, cut_text, reason, status FROM book_cuts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]bookCutResponse, 0)
	for rows.Next() {
		var c bookCutResponse
		var text string
		if err := rows.Scan(&c.ChapterID, &text, &c.Reason, &c.Status); err != nil {
			return nil, err
		}
		before, after, ok := strings.Cut(text, "→")
		if ok {
			c.Before = strings.TrimSpace(before)
			c.After = strings.TrimSpace(after)
		} else {
			c.After = text
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func tabsDTO(tabs []book.WindowTab) []BookTab {
	out := make([]BookTab, 0, len(tabs))
	for _, t := range tabs {
		out = append(out, BookTab{ID: t.ID, Label: t.Label})
	}
	return out
}

func tocDTO(toc []book.TocEntry) []BookTocEntry {
	out := make([]BookTocEntry, 0, len(toc))
	for _, e := range toc {
		out = append(out, BookTocEntry{ChapterID: e.ChapterID, Day: e.Day, Title: e.Title})
	}
	return out
}

func rawLogDTO(raw []book.RawLogEntry) []BookRawLogEntry {
	out := make([]BookRawLogEntry, 0, len(raw))
	for _, r := range raw {
		out = append(out, BookRawLogEntry{Kind: r.Kind, Text: r.Text})
	}
	return out
}
