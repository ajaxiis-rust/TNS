// Package cast — карта книги: имена, описания, характеры и роли действующих
// лиц (P12 §5.9). Извлечение идёт в литературной базе мира поверх миграции
// 0005_cast; сырьё остаётся файлом полки (B-03.7), в базе только выкованное.
package cast

import (
	"database/sql"
	"errors"
	"time"

	"tns65/internal/db"
)

// BookChunk — кусок книги в карте (P12 §5.9): номер, глава и хеш текста.
// Текст куска не дублируется — извлечение заново разбирает файл полки.
type BookChunk struct {
	ID       int64
	BookID   int64
	Ord      int64
	Chapter  string
	TextHash string
}

// BookCharacter — действующее лицо карты книги. aliases и traits — JSON-массивы
// строк; appearance и description — свободный текст.
type BookCharacter struct {
	ID          int64
	BookID      int64
	Name        string
	Aliases     string
	Appearance  string
	Description string
	Traits      string
	Confidence  float64
	FirstChunk  int64
	LastChunk   int64
	Mentions    int64
	Status      string
	CreatedAt   string
}

// BookMention — упоминание героя в куске (карта упоминаний, P12 §5.9 п.3).
type BookMention struct {
	CharacterID int64
	ChunkID     int64
	Count       int64
}

// BookRole — роль героя на отрезке кусков (P12 §5.9 п.5).
type BookRole struct {
	ID          int64
	CharacterID int64
	FromChunk   int64
	ToChunk     int64
	Role        string
	Confidence  float64
	Evidence    string
}

// StatusCandidate — начальный статус героя: кандидат, ещё не подтверждён.
const StatusCandidate = "candidate"

// ErrChunkNotFound — в карте нет куска с таким (book_id, ord).
var ErrChunkNotFound = errors.New("cast: кусок карты не найден")

// ErrCharacterNotFound — в карте нет героя с таким id.
var ErrCharacterNotFound = errors.New("cast: герой карты не найден")

// InsertChunk пишет кусок карты в literary.db хэндлом forge и возвращает его
// id. Уникальность (book_id, ord) держит DDL: повтор куска той же позиции —
// ошибка SQLite UNIQUE.
func InsertChunk(lit *db.Handle, c BookChunk) (int64, error) {
	if lit == nil {
		return 0, errors.New("cast: нет хэндла литературной базы")
	}
	if c.BookID <= 0 || c.TextHash == "" {
		return 0, errors.New("cast: кусок без книги или хеша")
	}
	res, err := lit.Exec("book_chunk",
		`INSERT INTO book_chunk (book_id, ord, chapter, text_hash) VALUES (?, ?, ?, ?)`,
		c.BookID, c.Ord, c.Chapter, c.TextHash)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ChunkByOrd читает кусок по (book_id, ord); куска нет — ErrChunkNotFound.
func ChunkByOrd(lit *db.Handle, bookID, ord int64) (BookChunk, error) {
	var c BookChunk
	err := lit.QueryRow(
		`SELECT id, book_id, ord, chapter, text_hash FROM book_chunk WHERE book_id = ? AND ord = ?`,
		bookID, ord).
		Scan(&c.ID, &c.BookID, &c.Ord, &c.Chapter, &c.TextHash)
	if errors.Is(err, sql.ErrNoRows) {
		return BookChunk{}, ErrChunkNotFound
	}
	if err != nil {
		return BookChunk{}, err
	}
	return c, nil
}

// InsertCharacter пишет героя карты и возвращает его id. Пустые статус и дата
// заполняются дефолтами статуса кандидата и текущего времени.
func InsertCharacter(lit *db.Handle, c BookCharacter) (int64, error) {
	if lit == nil {
		return 0, errors.New("cast: нет хэндла литературной базы")
	}
	if c.BookID <= 0 || c.Name == "" {
		return 0, errors.New("cast: герой без книги или имени")
	}
	if c.Status == "" {
		c.Status = StatusCandidate
	}
	if c.CreatedAt == "" {
		c.CreatedAt = nowUTC()
	}
	res, err := lit.Exec("book_character",
		`INSERT INTO book_character (book_id, name, aliases, appearance, description, traits, confidence, first_chunk, last_chunk, mentions, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.BookID, c.Name, c.Aliases, c.Appearance, c.Description, c.Traits,
		c.Confidence, c.FirstChunk, c.LastChunk, c.Mentions, c.Status, c.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CharacterByID читает героя по id; героя нет — ErrCharacterNotFound.
func CharacterByID(lit *db.Handle, id int64) (BookCharacter, error) {
	var c BookCharacter
	err := lit.QueryRow(
		`SELECT id, book_id, name, aliases, appearance, description, traits, confidence, first_chunk, last_chunk, mentions, status, created_at FROM book_character WHERE id = ?`, id).
		Scan(&c.ID, &c.BookID, &c.Name, &c.Aliases, &c.Appearance, &c.Description,
			&c.Traits, &c.Confidence, &c.FirstChunk, &c.LastChunk, &c.Mentions, &c.Status, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BookCharacter{}, ErrCharacterNotFound
	}
	if err != nil {
		return BookCharacter{}, err
	}
	return c, nil
}

// InsertMention пишет счётчик упоминаний героя в куске. Повтор по той же паре
// (character_id, chunk_id) — ошибка SQLite PRIMARY KEY.
func InsertMention(lit *db.Handle, m BookMention) error {
	if lit == nil {
		return errors.New("cast: нет хэндла литературной базы")
	}
	if m.CharacterID <= 0 || m.ChunkID <= 0 {
		return errors.New("cast: упоминание без героя или куска")
	}
	_, err := lit.Exec("book_character_mention",
		`INSERT INTO book_character_mention (character_id, chunk_id, count) VALUES (?, ?, ?)`,
		m.CharacterID, m.ChunkID, m.Count)
	return err
}

// InsertRole пишет роль героя на отрезке кусков и возвращает её id.
func InsertRole(lit *db.Handle, r BookRole) (int64, error) {
	if lit == nil {
		return 0, errors.New("cast: нет хэндла литературной базы")
	}
	if r.CharacterID <= 0 || r.Role == "" {
		return 0, errors.New("cast: роль без героя или названия")
	}
	res, err := lit.Exec("book_character_role",
		`INSERT INTO book_character_role (character_id, from_chunk, to_chunk, role, confidence, evidence) VALUES (?, ?, ?, ?, ?, ?)`,
		r.CharacterID, r.FromChunk, r.ToChunk, r.Role, r.Confidence, r.Evidence)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
