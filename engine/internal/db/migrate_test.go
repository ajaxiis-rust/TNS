package db_test

import (
	"path/filepath"
	"testing"

	"tns65/internal/db"
)

// TestEnsureMigratedOnOpen — IMP-13: открытие базы доносит недостающие миграции —
// литературная база старого мира получает таблицы 0006 без перерождения;
// повторный вызов идемпотентен.
func TestEnsureMigratedOnOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "literary.db")
	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(raw, "literary", "shared"); err != nil {
		_ = raw.Close()
		t.Fatalf("Migrate(literary, shared): %v", err)
	}

	// Состояние мира до 0006: таблиц волны нет, строка учёта снята.
	for _, q := range []string{
		`DROP TABLE book_chunk_tag`,
		`DROP TABLE book_place`,
		`DROP TABLE book_scene`,
		`DROP TABLE book_style`,
		`DROP TABLE dict_staging`,
		`DELETE FROM schema_migrations WHERE version = 6 AND name = '0006_book_deepread'`,
	} {
		if _, err := raw.Exec(q); err != nil {
			_ = raw.Close()
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	h, err := db.OpenWriterAs(path, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer h.Close()

	if err := h.EnsureMigrated(); err != nil {
		t.Fatalf("EnsureMigrated: %v", err)
	}
	for _, table := range []string{"book_chunk_tag", "book_place", "book_scene", "book_style", "dict_staging"} {
		var n int
		if err := h.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatalf("таблица %s после EnsureMigrated: %v", table, err)
		}
	}

	if err := h.EnsureMigrated(); err != nil {
		t.Fatalf("EnsureMigrated (повтор): %v", err)
	}
}

// TestEnsureMigratedUnknownDB — миграции определены только для баз мира и
// литературы: base собирается запеканием.
func TestEnsureMigratedUnknownDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "base.db")
	h, err := db.OpenWriterAs(path, db.DBBase, db.WriterBake, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(base): %v", err)
	}
	defer h.Close()

	if err := h.EnsureMigrated(); err == nil {
		t.Fatal("EnsureMigrated(base) прошёл, want ошибку")
	}
}
