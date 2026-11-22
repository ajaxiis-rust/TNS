package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// TestLiteraryGefestRename — T-17-06 (DEC-6): открытие литературной базы мира
// доносит миграцию 0007 и заменяет историческую запись «Пополняется Forge» на
// «пополняется Гефестом»; повторный прогон SQL-файла меняет ноль строк, а
// `Forge` как код-идентификатор в чужих текстах не трогается.
func TestLiteraryGefestRename(t *testing.T) {
	const (
		oldPhrase = "Пополняется Forge"
		newPhrase = "пополняется Гефестом"
	)

	t.Run("открытие мира", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "literary.db")
		raw, err := OpenWriter(path)
		if err != nil {
			t.Fatalf("OpenWriter: %v", err)
		}
		if err := Migrate(raw, "literary", "shared"); err != nil {
			_ = raw.Close()
			t.Fatalf("Migrate(literary, shared): %v", err)
		}

		// Мир до 0007: старые записи в свободных текстовых колонках, строка учёта
		// миграции снята. Вторая запись банка — контроль: `Forge` без фразы.
		for _, q := range []string{
			"INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at)" +
				" VALUES ('routine', 'Пополняется Forge', 'ru', '[]', 1, 'Пополняется `Forge`', '2026-01-01T00:00:00Z')",
			`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at)
			 VALUES ('routine', 'Forge — код-идентификатор TNS35', 'ru', '[]', 1, '', '2026-01-01T00:00:00Z')`,
			`INSERT INTO lit_template (kind, text, lang, tags) VALUES ('note', 'Пополняется Forge', 'ru', '[]')`,
			`INSERT INTO book (name, lang, status, created_at) VALUES ('Пополняется Forge', 'ru', 'ready', '2026-01-01T00:00:00Z')`,
			`INSERT INTO forge_log (kind, task, status, started_at) VALUES ('forge', 'Пополняется Forge', 'done', '2026-01-01T00:00:00Z')`,
			`DELETE FROM schema_migrations WHERE version = 7 AND name = '0007_gefest_rename'`,
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

		var bankText, bankOrigin, controlText string
		rows, err := h.Query(`SELECT text, origin FROM bank_pool ORDER BY id`)
		if err != nil {
			t.Fatalf("bank_pool: %v", err)
		}
		var bank []string
		for rows.Next() {
			var text, origin string
			if err := rows.Scan(&text, &origin); err != nil {
				rows.Close()
				t.Fatalf("bank_pool scan: %v", err)
			}
			bank = append(bank, text+"\x00"+origin)
		}
		rows.Close()
		if len(bank) != 2 {
			t.Fatalf("bank_pool: строк %d, ожидалось 2", len(bank))
		}
		bankText, bankOrigin, _ = strings.Cut(bank[0], "\x00")
		controlText, _, _ = strings.Cut(bank[1], "\x00")
		if bankText != newPhrase || bankOrigin != newPhrase {
			t.Errorf("bank_pool после миграции: text=%q origin=%q, ожидалось %q", bankText, bankOrigin, newPhrase)
		}
		if controlText != "Forge — код-идентификатор TNS35" {
			t.Errorf("bank_pool контрольная запись изменена: %q", controlText)
		}

		for _, tc := range []struct{ query, want string }{
			{`SELECT text FROM lit_template`, newPhrase},
			{`SELECT name FROM book`, newPhrase},
			{`SELECT task FROM forge_log`, newPhrase},
		} {
			var got string
			if err := h.QueryRow(tc.query).Scan(&got); err != nil {
				t.Fatalf("%s: %v", tc.query, err)
			}
			if got != tc.want {
				t.Errorf("%s: %q, ожидалось %q", tc.query, got, tc.want)
			}
		}

		for _, tc := range []struct{ query string }{
			{`SELECT COUNT(*) FROM bank_pool WHERE instr(text, 'Пополняется') > 0 OR instr(origin, 'Пополняется') > 0`},
			{`SELECT COUNT(*) FROM lit_template WHERE instr(text, 'Пополняется') > 0`},
			{`SELECT COUNT(*) FROM book WHERE instr(name, 'Пополняется') > 0`},
			{`SELECT COUNT(*) FROM forge_log WHERE instr(task, 'Пополняется') > 0`},
		} {
			var n int
			if err := h.QueryRow(tc.query).Scan(&n); err != nil {
				t.Fatalf("%s: %v", tc.query, err)
			}
			if n != 0 {
				t.Errorf("%s: %d старых записей, ожидалось 0", tc.query, n)
			}
		}
	})

	t.Run("счётчик повтора", func(t *testing.T) {
		sqlText, err := schemaFS.ReadFile("schema/literary/0007_gefest_rename.sql")
		if err != nil {
			t.Fatalf("ReadFile 0007: %v", err)
		}

		path := filepath.Join(t.TempDir(), "literary.db")
		raw, err := OpenWriter(path)
		if err != nil {
			t.Fatalf("OpenWriter: %v", err)
		}
		defer raw.Close()
		if err := Migrate(raw, "literary", "shared"); err != nil {
			t.Fatalf("Migrate(literary, shared): %v", err)
		}
		if _, err := raw.Exec(`INSERT INTO lit_template (kind, text, lang, tags) VALUES ('note', 'Пополняется Forge', 'ru', '[]')`); err != nil {
			t.Fatalf("INSERT lit_template: %v", err)
		}

		before := totalChanges(t, raw)
		if _, err := raw.Exec(string(sqlText)); err != nil {
			t.Fatalf("первый прогон 0007: %v", err)
		}
		first := totalChanges(t, raw) - before
		if first != 1 {
			t.Errorf("первый прогон изменил %d строк, ожидалось 1", first)
		}
		if _, err := raw.Exec(string(sqlText)); err != nil {
			t.Fatalf("повторный прогон 0007: %v", err)
		}
		second := totalChanges(t, raw) - before - first
		if second != 0 {
			t.Errorf("повторный прогон изменил %d строк, ожидалось 0", second)
		}
	})
}

func totalChanges(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(`SELECT total_changes()`).Scan(&n); err != nil {
		t.Fatalf("total_changes: %v", err)
	}
	return n
}
