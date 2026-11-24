package db

import (
	"path/filepath"
	"testing"
)

// TestBankTagsBackfill — миграция 0008 (T-05-07): пустые теги записей ковки
// книги заполняются ключами разбора её чанка; брак не тег, чужие записи и
// записи без разбора не трогаются, повтор миграции изменений не даёт.
func TestBankTagsBackfill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "literary.db")
	raw, err := OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := Migrate(raw, "literary", "shared"); err != nil {
		_ = raw.Close()
		t.Fatalf("Migrate(literary, shared): %v", err)
	}

	// Состояние мира до 0008: данные на месте, строка учёта миграции снята.
	for _, q := range []string{
		`INSERT INTO book (id, name, lang, status, created_at) VALUES (1, 'книга', 'ru', 'ready', '2026-01-01T00:00:00Z')`,
		`INSERT INTO book_chunk (id, book_id, ord, chapter, text_hash) VALUES (10, 1, 0, 'гл. 1', 'h0')`,
		`INSERT INTO book_chunk (id, book_id, ord, chapter, text_hash) VALUES (11, 1, 1, 'гл. 1', 'h1')`,
		`INSERT INTO book_chunk (id, book_id, ord, chapter, text_hash) VALUES (12, 1, 2, 'гл. 2', 'h2')`,
		`INSERT INTO book_chunk_tag (chunk_id, key, score, band) VALUES (10, 'nature', 0.9, 'confident')`,
		`INSERT INTO book_chunk_tag (chunk_id, key, score, band) VALUES (10, 'architecture', 0.7, 'uncertain')`,
		`INSERT INTO book_chunk_tag (chunk_id, key, score, band) VALUES (10, 'violence', 0.1, 'reject')`,
		`INSERT INTO book_chunk_tag (chunk_id, key, score, band) VALUES (11, 'battle', 0.1, 'reject')`,
		`INSERT INTO bank_pool (id, kind, text, lang, tags, trust_level, origin, book_id, created_at)
		    VALUES (20, 'routine', 'текст чанка 0', 'ru', '', 2, 'forge-book:1:0', 1, '2026-01-01T00:00:00Z')`,
		`INSERT INTO bank_pool (id, kind, text, lang, tags, trust_level, origin, book_id, created_at)
		    VALUES (21, 'routine', 'текст чанка 1', 'ru', '', 2, 'forge-book:1:1', 1, '2026-01-01T00:00:00Z')`,
		`INSERT INTO bank_pool (id, kind, text, lang, tags, trust_level, origin, book_id, created_at)
		    VALUES (22, 'routine', 'текст чанка 2', 'ru', '', 2, 'forge-book:1:2', 1, '2026-01-01T00:00:00Z')`,
		`INSERT INTO bank_pool (id, kind, text, lang, tags, trust_level, origin, book_id, created_at)
		    VALUES (23, 'routine', 'сид', 'ru', 'seed', 2, 'сид', NULL, '2026-01-01T00:00:00Z')`,
		`DELETE FROM schema_migrations WHERE version = 8 AND name = '0008_bank_tags'`,
		`PRAGMA user_version = 7`,
	} {
		if _, err := raw.Exec(q); err != nil {
			_ = raw.Close()
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	h, err := OpenWriterAs(path, DBLiterary, WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer h.Close()
	if err := h.EnsureMigrated(); err != nil {
		t.Fatalf("EnsureMigrated: %v", err)
	}

	// Чанк 0: ключи разбора по алфавиту, брак violence отброшен.
	var tags string
	if err := h.QueryRow(`SELECT tags FROM bank_pool WHERE id = 20`).Scan(&tags); err != nil {
		t.Fatalf("tags записи 20: %v", err)
	}
	if tags != "architecture,nature" {
		t.Errorf("tags записи 20 = %q, want architecture,nature", tags)
	}
	// Чанк 1: только брак — тегов нет.
	if err := h.QueryRow(`SELECT tags FROM bank_pool WHERE id = 21`).Scan(&tags); err != nil {
		t.Fatalf("tags записи 21: %v", err)
	}
	if tags != "" {
		t.Errorf("tags записи 21 = %q, want пусто (только брак)", tags)
	}
	// Чанк 2: разбора нет — запись не трогается.
	if err := h.QueryRow(`SELECT tags FROM bank_pool WHERE id = 22`).Scan(&tags); err != nil {
		t.Fatalf("tags записи 22: %v", err)
	}
	if tags != "" {
		t.Errorf("tags записи 22 = %q, want пусто (разбора нет)", tags)
	}
	// Запись вне книг не трогается.
	if err := h.QueryRow(`SELECT tags FROM bank_pool WHERE id = 23`).Scan(&tags); err != nil {
		t.Fatalf("tags записи 23: %v", err)
	}
	if tags != "seed" {
		t.Errorf("tags записи 23 = %q, want seed (не книга)", tags)
	}

	// Повтор миграции изменений не даёт.
	if err := h.EnsureMigrated(); err != nil {
		t.Fatalf("EnsureMigrated (повтор): %v", err)
	}
	if err := h.QueryRow(`SELECT tags FROM bank_pool WHERE id = 20`).Scan(&tags); err != nil {
		t.Fatalf("tags записи 20 после повтора: %v", err)
	}
	if tags != "architecture,nature" {
		t.Errorf("tags записи 20 после повтора = %q, want architecture,nature", tags)
	}
}
