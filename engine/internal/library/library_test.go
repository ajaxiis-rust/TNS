package library

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newShelf(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func TestLoadMissingManifest(t *testing.T) {
	books, err := Load(newShelf(t))
	if err != nil {
		t.Fatalf("Load(empty): %v", err)
	}
	if len(books) != 0 {
		t.Fatalf("Load(empty) = %d книг, want 0", len(books))
	}
}

func TestImportLaysFile(t *testing.T) {
	root := newShelf(t)
	content := []byte("Глава 1\nПролог.\n")
	book, err := Import(root, "hp.txt", content)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if book.ID == "" || book.Format != "txt" || book.Status != StatusImported {
		t.Fatalf("Import = %+v, want id/format/status", book)
	}
	if book.Bytes != int64(len(content)) {
		t.Fatalf("bytes = %d, want %d", book.Bytes, len(content))
	}
	wantSHA := sha256.Sum256(content)
	if book.SHA256 != hex.EncodeToString(wantSHA[:]) {
		t.Fatalf("sha256 = %s, want %s", book.SHA256, hex.EncodeToString(wantSHA[:]))
	}

	// Книга лежит в <root>/books/<id>/<имя>, мир не требуется.
	path := filepath.Join(root, "books", book.ID, "hp.txt")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(got) != string(content) {
		t.Fatalf("файл книги = %q, want %q", got, content)
	}

	books, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(books) != 1 || books[0].ID != book.ID {
		t.Fatalf("манифест = %+v, want одна книга", books)
	}
}

func TestImportFB2(t *testing.T) {
	root := newShelf(t)
	book, err := Import(root, "war.fb2", []byte("<FictionBook/>"))
	if err != nil {
		t.Fatalf("Import(fb2): %v", err)
	}
	if book.Format != "fb2" {
		t.Fatalf("format = %q, want fb2", book.Format)
	}
}

func TestImportUnsupportedFormat(t *testing.T) {
	_, err := Import(newShelf(t), "book.pdf", []byte("x"))
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("Import(pdf) = %v, want ErrUnsupportedFormat", err)
	}
}

func TestImportUniqueID(t *testing.T) {
	root := newShelf(t)
	first, err := Import(root, "hp.txt", []byte("a"))
	if err != nil {
		t.Fatalf("Import(1): %v", err)
	}
	second, err := Import(root, "hp.txt", []byte("b"))
	if err != nil {
		t.Fatalf("Import(2): %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("id повторился: %q", first.ID)
	}
	if _, err := os.Stat(filepath.Join(root, "books", second.ID, "hp.txt")); err != nil {
		t.Fatalf("файл второго импорта: %v", err)
	}
}

func TestDelete(t *testing.T) {
	root := newShelf(t)
	book, err := Import(root, "hp.txt", []byte("текст"))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if err := Delete(root, book.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "books", book.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("каталог книги не удалён: %v", err)
	}
	books, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(books) != 0 {
		t.Fatalf("манифест после удаления = %+v, want пусто", books)
	}
}

func TestDeleteNotFound(t *testing.T) {
	if err := Delete(newShelf(t), "no-such-id"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(unknown) = %v, want ErrNotFound", err)
	}
}
