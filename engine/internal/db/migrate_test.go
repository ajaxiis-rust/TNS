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

// TestEnsureMigratedPlayerProfile — T-17-04: мир, созданный до 0013, открывается
// и получает колонки психотипа и привычек без потери строк карты.
func TestEnsureMigratedPlayerProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "world(alpha).db")
	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(raw, "world", "shared"); err != nil {
		_ = raw.Close()
		t.Fatalf("Migrate(world, shared): %v", err)
	}

	// Состояние мира до 0013: карта в старом составе колонок, строка на месте,
	// учёт миграции снят.
	for _, q := range []string{
		`CREATE TABLE player_atlas_old (
		    id INTEGER PRIMARY KEY,
		    day INTEGER NOT NULL,
		    theme TEXT NOT NULL,
		    mood TEXT NOT NULL,
		    interest TEXT NOT NULL,
		    source TEXT NOT NULL
		)`,
		`INSERT INTO player_atlas_old (id, day, theme, mood, interest, source)
		    SELECT id, day, theme, mood, interest, source FROM player_atlas`,
		`DROP TABLE player_atlas`,
		`ALTER TABLE player_atlas_old RENAME TO player_atlas`,
		`INSERT INTO player_atlas (id, day, theme, mood, interest, source) VALUES (7, 2, 'habit', 'ровное', 'прогулки', 'pythia')`,
		`DELETE FROM schema_migrations WHERE version = 13 AND name = '0013_player_profile'`,
	} {
		if _, err := raw.Exec(q); err != nil {
			_ = raw.Close()
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer h.Close()

	if err := h.EnsureMigrated(); err != nil {
		t.Fatalf("EnsureMigrated: %v", err)
	}

	// Строка карты пережила миграцию, новые колонки пустые.
	var id, day int
	var theme, mood, interest, source, psychotype, habits string
	if err := h.QueryRow(`SELECT id, day, theme, mood, interest, source, psychotype, habits FROM player_atlas`).
		Scan(&id, &day, &theme, &mood, &interest, &source, &psychotype, &habits); err != nil {
		t.Fatalf("SELECT player_atlas после 0013: %v", err)
	}
	if id != 7 || day != 2 || theme != "habit" || mood != "ровное" || interest != "прогулки" || source != "pythia" {
		t.Fatalf("строка карты после 0013 = (%d, %d, %q, %q, %q, %q)", id, day, theme, mood, interest, source)
	}
	if psychotype != "" || habits != "" {
		t.Fatalf("новые колонки после 0013 = (%q, %q), want пустые", psychotype, habits)
	}

	// Повторное открытие мира изменений не вносит.
	if err := h.EnsureMigrated(); err != nil {
		t.Fatalf("EnsureMigrated (повтор): %v", err)
	}
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM player_atlas`).Scan(&n); err != nil {
		t.Fatalf("COUNT player_atlas: %v", err)
	}
	if n != 1 {
		t.Fatalf("строк карты после повторной миграции: %d, want 1", n)
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
