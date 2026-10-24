package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"tns65/internal/journal"
)

func TestWriterMap(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "engine.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	baseH := openMigrated(t, filepath.Join(dir, "base.db"), DBBase, []string{"base", "shared"}, WriterCore, logger)
	defer baseH.Close()
	litH := openMigrated(t, filepath.Join(dir, "literary.db"), DBLiterary, []string{"literary"}, WriterCore, logger)
	defer litH.Close()
	worldCore := openMigrated(t, filepath.Join(dir, "world-core.db"), DBWorld, []string{"world", "shared"}, WriterCore, logger)
	defer worldCore.Close()
	worldForge := openMigrated(t, filepath.Join(dir, "world-forge.db"), DBWorld, []string{"world", "shared"}, WriterForge, logger)
	defer worldForge.Close()

	// core пишет в turns (world).
	if _, err := worldCore.Exec("turns",
		`INSERT INTO turns (session_id, player_text, reply_text, saw_recommendation, trust_level, latency_ms, degraded) VALUES (?,?,?,?,?,?,?)`,
		1, "p", "r", 0, 0, 0, 0); err != nil {
		t.Errorf("core → turns: %v", err)
	}

	// core получает ErrForeignWriter на narratives (base), bank_pool (literary), places (world).
	if _, err := baseH.Exec("narratives",
		`INSERT INTO narratives (id, name, arc_summary, lang, created_at) VALUES (1,'n','s','ru','2026-01-01T00:00:00Z')`); !errors.Is(err, ErrForeignWriter) {
		t.Errorf("core → narratives: err = %v, want ErrForeignWriter", err)
	}
	if _, err := litH.Exec("bank_pool",
		`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at) VALUES ('fallback','t','ru','t',0,'o','2026-01-01T00:00:00Z')`); !errors.Is(err, ErrForeignWriter) {
		t.Errorf("core → bank_pool: err = %v, want ErrForeignWriter", err)
	}
	if _, err := worldCore.Exec("places",
		`INSERT INTO places (name, kind) VALUES ('p','city')`); !errors.Is(err, ErrForeignWriter) {
		t.Errorf("core → places: err = %v, want ErrForeignWriter", err)
	}

	// forge пишет в places и world_meta (world).
	if _, err := worldForge.Exec("places",
		`INSERT INTO places (name, kind) VALUES ('p','city')`); err != nil {
		t.Errorf("forge → places: %v", err)
	}
	if _, err := worldForge.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES ('k','v')`); err != nil {
		t.Errorf("forge → world_meta: %v", err)
	}

	// forge получает отказ на turns (world).
	if _, err := worldForge.Exec("turns",
		`INSERT INTO turns (session_id, player_text, reply_text, saw_recommendation, trust_level, latency_ms, degraded) VALUES (?,?,?,?,?,?,?)`,
		1, "p", "r", 0, 0, 0, 0); !errors.Is(err, ErrForeignWriter) {
		t.Errorf("forge → turns: err = %v, want ErrForeignWriter", err)
	}

	// В журнале есть строки db.foreign_writer с таблицей и писателем.
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile log: %v", err)
	}
	log := string(b)
	if !strings.Contains(log, `"event":"db.foreign_writer"`) {
		t.Errorf("журнал не содержит db.foreign_writer:\n%s", log)
	}
	if !strings.Contains(log, `"key":"table","value":"places"`) {
		t.Errorf("журнал не содержит table=places:\n%s", log)
	}
	if !strings.Contains(log, `"key":"writer","value":"core"`) {
		t.Errorf("журнал не содержит writer=core:\n%s", log)
	}
}

func TestTxRollback(t *testing.T) {
	dir := t.TempDir()
	logger, err := journal.New(filepath.Join(dir, "engine.log"))
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	h := openMigrated(t, filepath.Join(dir, "world(a).db"), DBWorld, []string{"world", "shared"}, WriterCore, logger)
	defer h.Close()

	err = h.Tx(WriterCore, func(tx *Tx) error {
		if _, err := tx.Exec("turns",
			`INSERT INTO turns (session_id, player_text, reply_text, saw_recommendation, trust_level, latency_ms, degraded) VALUES (?,?,?,?,?,?,?)`,
			1, "p", "r", 0, 0, 0, 0); err != nil {
			return err
		}
		if _, err := tx.Exec("places",
			`INSERT INTO places (name, kind) VALUES ('p','city')`); err != nil {
			return err
		}
		return nil
	})
	if !errors.Is(err, ErrForeignWriter) {
		t.Fatalf("Tx err = %v, want ErrForeignWriter", err)
	}

	// Откат: строк первой таблицы не осталось.
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM turns`).Scan(&n); err != nil {
		t.Fatalf("COUNT(turns): %v", err)
	}
	if n != 0 {
		t.Errorf("после отката строк в turns = %d, want 0", n)
	}
}

func TestWriterMapCoverage(t *testing.T) {
	dir := t.TempDir()

	type dbSpec struct {
		name  DB
		path  string
		dirs  []string
		tfile string
	}
	specs := []dbSpec{
		{DBBase, filepath.Join(dir, "base.db"), []string{"base", "shared"}, "schema/base/tables.txt"},
		{DBLiterary, filepath.Join(dir, "literary.db"), []string{"literary"}, "schema/literary/tables.txt"},
		{DBWorld, filepath.Join(dir, "world(a).db"), []string{"world", "shared"}, "schema/world/tables.txt"},
	}

	for _, s := range specs {
		w, err := OpenWriter(s.path)
		if err != nil {
			t.Fatalf("OpenWriter(%s): %v", s.name, err)
		}
		if err := Migrate(w, s.dirs...); err != nil {
			w.Close()
			t.Fatalf("Migrate(%s): %v", s.name, err)
		}

		actual := actualTables(t, w)
		w.Close()

		want := readTablesList(t, s.tfile)

		// Доменные таблицы = tables.txt без schema_migrations (её пишет мигратор).
		domain := make([]string, 0, len(want))
		for _, tbl := range want {
			if tbl != "schema_migrations" {
				domain = append(domain, tbl)
			}
		}

		got := mapTables(t, s.name)

		// Каждая доменная таблица покрыта картой ровно один раз (без пропусков и дублей).
		if !slices.Equal(got, domain) {
			t.Errorf("%s: карта = %v, want %v", s.name, got, domain)
		}

		// Каждая запись карты существует в DDL.
		for _, tbl := range got {
			if !slices.Contains(actual, tbl) {
				t.Errorf("%s: запись карты %q отсутствует в DDL (sqlite_master)", s.name, tbl)
			}
		}
	}
}

func openMigrated(t *testing.T, path string, dbName DB, dirs []string, writer Writer, logger *journal.Logger) *Handle {
	t.Helper()
	w, err := OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter(%s): %v", path, err)
	}
	if err := Migrate(w, dirs...); err != nil {
		w.Close()
		t.Fatalf("Migrate(%v): %v", dirs, err)
	}
	return &Handle{db: w, dbName: dbName, writer: writer, logger: logger}
}

func actualTables(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`)
	if err != nil {
		t.Fatalf("sqlite_master: %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("sqlite_master scan: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("sqlite_master rows: %v", err)
	}
	sort.Strings(names)
	return names
}

func readTablesList(t *testing.T, path string) []string {
	t.Helper()
	raw, err := schemaFS.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	var names []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	sort.Strings(names)
	return names
}

func mapTables(t *testing.T, dbName DB) []string {
	t.Helper()
	var names []string
	for k := range writerMap {
		if k.db == dbName {
			names = append(names, k.table)
		}
	}
	sort.Strings(names)
	return names
}
