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

// libraryListResponse — JSON-ответ полки: массив книг с их статусами.
type libraryListResponse struct {
	Books []library.Book `json:"books"`
}

// handleLibraryList — GET /api/library: список книг полки со статусами.
func handleLibraryList(w http.ResponseWriter, _ *http.Request) {
	books, err := library.Load(libraryDir())
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "library read failed")
		return
	}
	writeJSON(w, http.StatusOK, libraryListResponse{Books: books})
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
	writeJSON(w, http.StatusOK, libraryListResponse{Books: books})
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
	writeJSON(w, http.StatusOK, libraryListResponse{Books: books})
}
