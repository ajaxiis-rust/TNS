// Package library — глобальная полка книг (P11, T-12-09): сырьё txt/fb2
// лежит файлами в world_db/library/books/<id>/, а манифест library.json
// описывает книги и их статусы. Полка не привязана к миру — birth.List
// сканирует только worlds/*, поэтому полка миром не считается.
package library

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ManifestName — имя файла манифеста полки.
const ManifestName = "library.json"

// Статусы книги в манифесте (P11, раздел 5.2): файл лёг (imported), идёт
// разбор/ковка (parsing/queued/forging), готов (ready), сбой (error) и книга
// игрока (player, только в literary.db).
const (
	StatusImported = "imported"
	StatusParsing  = "parsing"
	StatusQueued   = "queued"
	StatusForging  = "forging"
	StatusReady    = "ready"
	StatusError    = "error"
	StatusPlayer   = "player"
)

var (
	// ErrUnsupportedFormat — на полку принимаются только txt и fb2.
	ErrUnsupportedFormat = errors.New("library: принимаются только txt и fb2")
	// ErrNotFound — книги с таким id на полке нет.
	ErrNotFound = errors.New("library: книга не найдена")
)

// Book — запись манифеста полки. Поля, которых нет у txt (автор, язык, жанры,
// рейтинг), заполняются позже, на этапе ковки, поэтому помечены omitempty.
type Book struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Author    string   `json:"author,omitempty"`
	Format    string   `json:"format"`
	Lang      string   `json:"lang,omitempty"`
	Genres    []string `json:"genres,omitempty"`
	Rating    string   `json:"rating,omitempty"`
	Chapters  int64    `json:"chapters"`
	Bytes     int64    `json:"bytes"`
	SHA256    string   `json:"sha256"`
	Status    string   `json:"status"`
	Progress  float64  `json:"progress"`
	CreatedAt string   `json:"created_at"`
}

// ManifestPath возвращает путь манифеста полки с корнем root.
func ManifestPath(root string) string {
	return filepath.Join(root, ManifestName)
}

// Load читает манифест полки; файла нет — пустой список.
func Load(root string) ([]Book, error) {
	data, err := os.ReadFile(ManifestPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return []Book{}, nil
	}
	if err != nil {
		return nil, err
	}
	var books []Book
	if err := json.Unmarshal(data, &books); err != nil {
		return nil, err
	}
	if books == nil {
		books = []Book{}
	}
	return books, nil
}

// Save записывает манифест полки атомарно: временный файл и rename, чтобы
// падение не оставило половину файла.
func Save(root string, books []Book) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(books, "", "  ")
	if err != nil {
		return err
	}
	tmp := ManifestPath(root) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, ManifestPath(root))
}

// Import кладёт файл книги на полку: сырьё в books/<id>/<имя>, запись в
// манифест. Формат берётся из расширения (только txt/fb2), id — уникальный
// слаг имени. Возвращает созданную запись.
func Import(root, name string, data []byte) (Book, error) {
	base := filepath.Base(name)
	format, ok := formatFor(base)
	if !ok {
		return Book{}, ErrUnsupportedFormat
	}
	books, err := Load(root)
	if err != nil {
		return Book{}, err
	}
	id := uniqueID(base, books)
	dir := filepath.Join(root, "books", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Book{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, base), data, 0o644); err != nil {
		return Book{}, err
	}

	book := Book{
		ID:        id,
		Name:      strings.TrimSuffix(base, filepath.Ext(base)),
		Format:    format,
		Bytes:     int64(len(data)),
		SHA256:    sha256hex(data),
		Status:    StatusImported,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	books = append(books, book)
	if err := Save(root, books); err != nil {
		_ = os.RemoveAll(dir)
		return Book{}, err
	}
	return book, nil
}

// Delete убирает книгу с полки по id: файл книги и запись манифеста.
func Delete(root, id string) error {
	books, err := Load(root)
	if err != nil {
		return err
	}
	idx := -1
	for i := range books {
		if books[i].ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrNotFound
	}
	if err := os.RemoveAll(filepath.Join(root, "books", id)); err != nil {
		return err
	}
	books = append(books[:idx], books[idx+1:]...)
	return Save(root, books)
}

// formatFor возвращает формат книги по расширению (только txt/fb2).
func formatFor(name string) (string, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt":
		return "txt", true
	case ".fb2":
		return "fb2", true
	default:
		return "", false
	}
}

// uniqueID строит уникальный id: слаг имени файла, при совпадении — суффикс.
func uniqueID(base string, books []Book) string {
	slug := slugify(strings.TrimSuffix(base, filepath.Ext(base)))
	if slug == "" {
		slug = "book"
	}
	id := slug
	for i := 2; idExists(id, books); i++ {
		id = slug + "-" + strconv.Itoa(i)
	}
	return id
}

func idExists(id string, books []Book) bool {
	for _, b := range books {
		if b.ID == id {
			return true
		}
	}
	return false
}

// slugify оставляет латинские буквы и цифры, прочее сворачивает в дефис.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	lastSep := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastSep = false
			continue
		}
		if b.Len() > 0 && !lastSep {
			b.WriteByte('-')
			lastSep = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func sha256hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
