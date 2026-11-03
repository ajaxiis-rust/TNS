package cast_test

import (
	"errors"
	"path/filepath"
	"testing"

	"tns65/internal/cast"
	"tns65/internal/db"
)

func newLit(t *testing.T) *db.Handle {
	t.Helper()
	path := filepath.Join(t.TempDir(), "literary.db")
	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(w, "literary"); err != nil {
		w.Close()
		t.Fatalf("Migrate(literary): %v", err)
	}
	w.Close()
	lit, err := db.OpenWriterAs(path, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	return lit
}

func insertBook(t *testing.T, lit *db.Handle, name string) int64 {
	t.Helper()
	res, err := lit.Exec("book",
		`INSERT INTO book (shelf_id, name, lang, genres, rating, chapters, status, created_at) VALUES ('hp-1997', ?, 'ru', 'фэнтези', '16+', 17, 'ready', '2026-01-01T00:00:00Z')`, name)
	if err != nil {
		t.Fatalf("INSERT book: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("book LastInsertId: %v", err)
	}
	return id
}

func TestCastStore(t *testing.T) {
	lit := newLit(t)
	defer lit.Close()
	bookID := insertBook(t, lit, "Гарри Поттер и философский камень")

	// 1. Куски: запись, чтение, уникальность (book_id, ord) (P12 §5.9, T-12-21).
	id1, err := cast.InsertChunk(lit, cast.BookChunk{BookID: bookID, Ord: 1, Chapter: "1", TextHash: "h1"})
	if err != nil {
		t.Fatalf("InsertChunk: %v", err)
	}
	if id1 <= 0 {
		t.Fatalf("InsertChunk id = %d, want > 0", id1)
	}
	if _, err := cast.InsertChunk(lit, cast.BookChunk{BookID: bookID, Ord: 1, Chapter: "1", TextHash: "h1"}); err == nil {
		t.Errorf("повторный кусок (book_id, ord) должен упасть по UNIQUE")
	}

	got, err := cast.ChunkByOrd(lit, bookID, 1)
	if err != nil {
		t.Fatalf("ChunkByOrd: %v", err)
	}
	if got.ID != id1 || got.Chapter != "1" || got.TextHash != "h1" {
		t.Errorf("ChunkByOrd = %+v", got)
	}
	if _, err := cast.ChunkByOrd(lit, bookID, 999); !errors.Is(err, cast.ErrChunkNotFound) {
		t.Errorf("ChunkByOrd(999) = %v, want ErrChunkNotFound", err)
	}

	// 2. Герой: запись с дефолтами (status='candidate', created_at) и чтение.
	cid, err := cast.InsertCharacter(lit, cast.BookCharacter{
		BookID:      bookID,
		Name:        "Северус Снейп",
		Aliases:     `["Снейп","профессор Снейп"]`,
		Appearance:  "чёрные волосы",
		Description: "мастер зелий",
		Traits:      `["суровый"]`,
		Confidence:  0.9,
		FirstChunk:  1,
		LastChunk:   3,
		Mentions:    12,
	})
	if err != nil {
		t.Fatalf("InsertCharacter: %v", err)
	}
	if cid <= 0 {
		t.Fatalf("InsertCharacter id = %d, want > 0", cid)
	}

	ch, err := cast.CharacterByID(lit, cid)
	if err != nil {
		t.Fatalf("CharacterByID: %v", err)
	}
	if ch.Name != "Северус Снейп" || ch.Aliases != `["Снейп","профессор Снейп"]` ||
		ch.Appearance != "чёрные волосы" || ch.Status != cast.StatusCandidate ||
		ch.Mentions != 12 || ch.CreatedAt == "" {
		t.Errorf("CharacterByID = %+v", ch)
	}
	if _, err := cast.CharacterByID(lit, 999999); !errors.Is(err, cast.ErrCharacterNotFound) {
		t.Errorf("CharacterByID(999999) = %v, want ErrCharacterNotFound", err)
	}

	// 3. Упоминание: запись и повтор по PK (character_id, chunk_id) падает.
	if err := cast.InsertMention(lit, cast.BookMention{CharacterID: cid, ChunkID: id1, Count: 3}); err != nil {
		t.Fatalf("InsertMention: %v", err)
	}
	if err := cast.InsertMention(lit, cast.BookMention{CharacterID: cid, ChunkID: id1, Count: 4}); err == nil {
		t.Errorf("повторное упоминание (character_id, chunk_id) должно упасть по PK")
	}

	// 4. Роль: запись и чтение обратно.
	rid, err := cast.InsertRole(lit, cast.BookRole{CharacterID: cid, FromChunk: 1, ToChunk: 3, Role: "антагонист", Confidence: 0.8, Evidence: "угрожал"})
	if err != nil {
		t.Fatalf("InsertRole: %v", err)
	}
	if rid <= 0 {
		t.Fatalf("InsertRole id = %d, want > 0", rid)
	}
	var role string
	var fromChunk, toChunk int64
	if err := lit.QueryRow(`SELECT role, from_chunk, to_chunk FROM book_character_role WHERE id = ?`, rid).Scan(&role, &fromChunk, &toChunk); err != nil {
		t.Fatalf("SELECT book_character_role: %v", err)
	}
	if role != "антагонист" || fromChunk != 1 || toChunk != 3 {
		t.Errorf("role row = (%q, %d, %d)", role, fromChunk, toChunk)
	}

	// 5. Минимальные поля обязательны.
	if _, err := cast.InsertChunk(lit, cast.BookChunk{BookID: bookID, Ord: 2}); err == nil {
		t.Errorf("InsertChunk без хеша должен вернуть ошибку")
	}
	if _, err := cast.InsertCharacter(lit, cast.BookCharacter{BookID: bookID}); err == nil {
		t.Errorf("InsertCharacter без имени должен вернуть ошибку")
	}
	if err := cast.InsertMention(lit, cast.BookMention{CharacterID: cid}); err == nil {
		t.Errorf("InsertMention без куска должен вернуть ошибку")
	}
	if _, err := cast.InsertRole(lit, cast.BookRole{CharacterID: cid}); err == nil {
		t.Errorf("InsertRole без роли должен вернуть ошибку")
	}
}
