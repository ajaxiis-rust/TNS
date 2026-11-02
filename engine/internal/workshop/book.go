package workshop

import (
	"database/sql"
	"errors"

	"tns65/internal/db"
)

// Book — строка реестра книг литературной базы мира (P11 §5.3, миграция
// 0004_book). Реестр опознаёт книгу по id, а не по строке origin, и даёт
// выпадающий список, привязку и фильтр «свои записи» без разбора строк.
type Book struct {
	ID        int64
	ShelfID   string
	Name      string
	Lang      string
	Genres    string
	Rating    string
	Chapters  int64
	Status    string
	CreatedAt string
}

// BookStatusPlayer — статус книги игрока (P11 §5.4): строка book со
// status='player' создаётся при рождении мира, имя — заголовок мира.
const BookStatusPlayer = "player"

// ErrBookNotFound — в реестре нет книги с таким id.
var ErrBookNotFound = errors.New("workshop: книга реестра не найдена")

// InsertBook пишет строку реестра в literary.db хэндлом forge и возвращает её
// id. Имя, язык и статус — минимальные поля канона §5.3; их отсутствие — ошибка.
func InsertBook(lit *db.Handle, b Book) (int64, error) {
	if lit == nil {
		return 0, errors.New("workshop: нет хэндла литературной базы")
	}
	if b.Name == "" || b.Lang == "" || b.Status == "" {
		return 0, errors.New("workshop: книга без имени, языка или статуса")
	}
	if b.CreatedAt == "" {
		b.CreatedAt = nowUTC()
	}
	res, err := lit.Exec("book",
		`INSERT INTO book (shelf_id, name, lang, genres, rating, chapters, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ShelfID, b.Name, b.Lang, b.Genres, b.Rating, b.Chapters, b.Status, b.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// BookByID читает строку реестра по id; книги нет — ErrBookNotFound.
func BookByID(lit *db.Handle, id int64) (Book, error) {
	var b Book
	err := lit.QueryRow(
		`SELECT id, shelf_id, name, lang, genres, rating, chapters, status, created_at FROM book WHERE id = ?`, id).
		Scan(&b.ID, &b.ShelfID, &b.Name, &b.Lang, &b.Genres, &b.Rating, &b.Chapters, &b.Status, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Book{}, ErrBookNotFound
	}
	if err != nil {
		return Book{}, err
	}
	return b, nil
}

// EnsurePlayerBook создаёт строку книги игрока (P11 §5.4), если её ещё нет, и
// возвращает id; повторный вызов с тем же именем дубля не создаёт. name — заголовок
// мира, lang — язык мира.
func EnsurePlayerBook(lit *db.Handle, name, lang string) (int64, error) {
	var id int64
	err := lit.QueryRow(
		`SELECT id FROM book WHERE status = ? AND name = ? LIMIT 1`, BookStatusPlayer, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return InsertBook(lit, Book{Name: name, Lang: lang, Status: BookStatusPlayer})
}
