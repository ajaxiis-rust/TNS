package webapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// seedLibraryShelf подменяет libraryRoot на t.TempDir(), чтобы тесты не писали
// в настоящую world_db/library/.
func seedLibraryShelf(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	old := libraryRoot
	libraryRoot = root
	t.Cleanup(func() { libraryRoot = old })
	return root
}

func libraryMultipart(t *testing.T, filename, content string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return mcpRequest(t, handleLibraryImport, http.MethodPost, libraryImportPath, buf.Bytes(), mw.FormDataContentType())
}

func libraryList(t *testing.T) libraryListResponse {
	t.Helper()
	rr := mcpRequest(t, handleLibraryList, http.MethodGet, libraryListPath, nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/library = %d, want 200", rr.Code)
	}
	var out libraryListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	return out
}

func TestLibraryImportAndList(t *testing.T) {
	root := seedLibraryShelf(t)
	rr := libraryMultipart(t, "hp.txt", "Глава 1\nПролог.\n")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST import = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	out := libraryList(t)
	if len(out.Books) != 1 {
		t.Fatalf("полка = %d книг, want 1", len(out.Books))
	}
	book := out.Books[0]
	if book.Format != "txt" || book.Status != "imported" {
		t.Fatalf("книга = %+v, want txt/imported", book)
	}
	// Книга лежит в <root>/library/books/<id>/<имя> — мир не требуется.
	path := filepath.Join(root, "library", "books", book.ID, "hp.txt")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("файл книги: %v", err)
	}
}

func TestLibraryImportUnsupported(t *testing.T) {
	seedLibraryShelf(t)
	rr := libraryMultipart(t, "book.pdf", "x")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("POST import(pdf) = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestLibraryDelete(t *testing.T) {
	root := seedLibraryShelf(t)
	if rr := libraryMultipart(t, "hp.txt", "текст"); rr.Code != http.StatusOK {
		t.Fatalf("import = %d", rr.Code)
	}
	out := libraryList(t)
	id := out.Books[0].ID

	rr := mcpRequest(t, handleLibraryDelete, http.MethodDelete, libraryBooksPath+id, nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	if got := libraryList(t); len(got.Books) != 0 {
		t.Fatalf("полка после удаления = %d книг, want 0", len(got.Books))
	}
	if _, err := os.Stat(filepath.Join(root, "library", "books", id)); !os.IsNotExist(err) {
		t.Fatalf("каталог книги не удалён: %v", err)
	}
}

func TestLibraryDeleteNotFound(t *testing.T) {
	seedLibraryShelf(t)
	rr := mcpRequest(t, handleLibraryDelete, http.MethodDelete, libraryBooksPath+"no-such-id", nil, "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("DELETE unknown = %d, want 404: %s", rr.Code, rr.Body.String())
	}
}
