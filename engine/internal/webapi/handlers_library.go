package webapi

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"tns65/internal/db"
	"tns65/internal/library"
)

// Полка книг (P11, T-12-09): импорт txt/fb2, список со статусами и удаление.
// Полка глобальная — не привязана к миру; сырьё лежит в world_db/library/, в
// базы миров не пишет. webapi только гоняет файлы и манифест library.json.

const (
	libraryListPath   = "/api/library"
	libraryImportPath = "/api/library/import"
	libraryBooksPath  = "/api/library/books/"
)

// libraryRoot — корень рабочих данных (world_db/); в тестах подменяется
// t.TempDir(). Полка живёт в подкаталоге library/.
var libraryRoot = db.WorldsRoot

func libraryDir() string {
	return filepath.Join(libraryRoot, "library")
}

// libraryListResponse — JSON-ответ полки: массив книг с их статусами и картой
// героев (T-12-26).
type libraryListResponse struct {
	Books []libraryBookCard `json:"books"`
}

// libraryCastCharacter — герой карты книги: имя, описание (свободный текст или
// внешность) и канонические роли по отрезкам.
type libraryCastCharacter struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Roles       []string `json:"roles,omitempty"`
}

// libraryCast — карта книги для карточки полки (P12 §5.9, T-12-26). quick —
// имена и описания собраны, full — роли по отрезкам собраны.
type libraryCast struct {
	Quick      bool                   `json:"quick"`
	Full       bool                   `json:"full"`
	Characters []libraryCastCharacter `json:"characters"`
}

// libraryBookCard — книга полки с картой героев; поля library.Book развёрнуты
// в тот же JSON-объект.
type libraryBookCard struct {
	library.Book
	Cast libraryCast `json:"cast"`
}

// handleLibraryList — GET /api/library: список книг полки со статусами.
func handleLibraryList(w http.ResponseWriter, _ *http.Request) {
	books, err := library.Load(libraryDir())
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "library read failed")
		return
	}
	writeJSON(w, http.StatusOK, libraryListResponse{Books: libraryCards(books)})
}

// handleLibraryImport — POST /api/library/import (multipart/form-data, поле
// file): принимает файл книги txt/fb2 и кладёт его на полку. Возвращает
// обновлённый список.
func handleLibraryImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad multipart body")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "file field required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "read file failed")
		return
	}
	if _, err := library.Import(libraryDir(), filepath.Base(header.Filename), data); err != nil {
		if errors.Is(err, library.ErrUnsupportedFormat) {
			writeError(w, http.StatusBadRequest, codeBadRequest, "unsupported format: only txt and fb2 are accepted")
			return
		}
		writeError(w, http.StatusInternalServerError, codeInternal, "library import failed")
		return
	}
	books, err := library.Load(libraryDir())
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "library read failed")
		return
	}
	writeJSON(w, http.StatusOK, libraryListResponse{Books: libraryCards(books)})
}

// libraryCards раскладывает книги полки по карточкам, прикладывая карту героев
// из активного мира. Полка глобальная, карта живёт в литературной базе мира,
// поэтому без активного мира карточки идут без карты (T-12-26).
func libraryCards(books []library.Book) []libraryBookCard {
	casts := readLibraryCast()
	cards := make([]libraryBookCard, 0, len(books))
	for _, b := range books {
		cast := casts[b.ID]
		if cast.Characters == nil {
			cast.Characters = []libraryCastCharacter{}
		}
		cards = append(cards, libraryBookCard{Book: b, Cast: cast})
	}
	return cards
}

// readLibraryCast читает карты всех книг активного мира из его literary.db.
// Отказ чтения — пустая карта: полка должна отображаться, а не ломаться.
func readLibraryCast() map[string]libraryCast {
	slug := getActiveSlug()
	if slug == "" {
		return map[string]libraryCast{}
	}
	lit, err := db.OpenReaderAs(litDBPath(slug), db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		return map[string]libraryCast{}
	}
	defer lit.Close()
	return castByShelf(lit)
}

// castByShelf сопоставляет shelf_id реестра книг с картой героев.
func castByShelf(lit *db.Handle) map[string]libraryCast {
	out := map[string]libraryCast{}
	rows, err := lit.Query(`SELECT id, shelf_id FROM book WHERE shelf_id != ''`)
	if err != nil {
		return out
	}
	type shelfBook struct {
		id    int64
		shelf string
	}
	var refs []shelfBook
	for rows.Next() {
		var r shelfBook
		if err := rows.Scan(&r.id, &r.shelf); err != nil {
			continue
		}
		refs = append(refs, r)
	}
	_ = rows.Close()
	for _, r := range refs {
		out[r.shelf] = readBookCast(lit, r.id)
	}
	return out
}

// readBookCast читает героев книги и их роли из literary.db. Описание —
// свободный текст, при пустом — внешность (как в cast.DescriptionText).
func readBookCast(lit *db.Handle, bookID int64) libraryCast {
	c := libraryCast{Characters: []libraryCastCharacter{}}
	rows, err := lit.Query(
		`SELECT c.id, c.name, c.description, c.appearance, COALESCE(r.role, '')
		 FROM book_character c
		 LEFT JOIN book_character_role r ON r.character_id = c.id
		 WHERE c.book_id = ?
		 ORDER BY c.id, r.from_chunk, r.id`, bookID)
	if err != nil {
		return c
	}
	defer rows.Close()

	idx := map[int64]int{}
	for rows.Next() {
		var id int64
		var name, desc, appearance, role string
		if err := rows.Scan(&id, &name, &desc, &appearance, &role); err != nil {
			continue
		}
		i, ok := idx[id]
		if !ok {
			i = len(c.Characters)
			idx[id] = i
			d := desc
			if d == "" {
				d = appearance
			}
			c.Characters = append(c.Characters, libraryCastCharacter{
				Name:        name,
				Description: d,
				Roles:       []string{},
			})
		}
		if role != "" {
			c.Characters[i].Roles = append(c.Characters[i].Roles, role)
		}
	}
	if len(c.Characters) == 0 {
		return c
	}
	c.Quick = true
	for i := range c.Characters {
		c.Characters[i].Roles = dedupeStrings(c.Characters[i].Roles)
		if len(c.Characters[i].Roles) > 0 {
			c.Full = true
		}
	}
	return c
}

// dedupeStrings убирает повторы ролей, сохраняя порядок первого вхождения.
func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// handleLibraryDelete — DELETE /api/library/books/{id}: убирает книгу с полки.
func handleLibraryDelete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, libraryBooksPath))
	if id == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "book id required")
		return
	}
	if err := library.Delete(libraryDir(), id); err != nil {
		if errors.Is(err, library.ErrNotFound) {
			writeError(w, http.StatusNotFound, codeNotFound, "book not found")
			return
		}
		writeError(w, http.StatusInternalServerError, codeInternal, "library delete failed")
		return
	}
	books, err := library.Load(libraryDir())
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "library read failed")
		return
	}
	writeJSON(w, http.StatusOK, libraryListResponse{Books: libraryCards(books)})
}
