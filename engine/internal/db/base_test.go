package db

import (
	"database/sql"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestBaseSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "base.db")

	w, err := OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	defer w.Close()

	if err := Migrate(w, "base", "shared"); err != nil {
		t.Fatalf("Migrate(base, shared): %v", err)
	}

	// 2. Список таблиц равен schema/base/tables.txt.
	raw, err := schemaFS.ReadFile("schema/base/tables.txt")
	if err != nil {
		t.Fatalf("ReadFile tables.txt: %v", err)
	}
	var want []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			want = append(want, line)
		}
	}
	sort.Strings(want)

	rows, err := w.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`)
	if err != nil {
		t.Fatalf("sqlite_master query: %v", err)
	}
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("sqlite_master scan: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("sqlite_master rows: %v", err)
	}
	rows.Close()

	if !slices.Equal(got, want) {
		t.Errorf("tables = %v, want %v", got, want)
	}

	// 3. user_version == 2 и две записи в schema_migrations.
	var uv int
	if err := w.QueryRow(`PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	if uv != 2 {
		t.Errorf("user_version = %d, want 2", uv)
	}

	type migration struct {
		version int
		name    string
	}
	var gotMigs []migration
	mrows, err := w.Query(`SELECT version, name FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("schema_migrations query: %v", err)
	}
	for mrows.Next() {
		var m migration
		if err := mrows.Scan(&m.version, &m.name); err != nil {
			t.Fatalf("schema_migrations scan: %v", err)
		}
		gotMigs = append(gotMigs, m)
	}
	mrows.Close()

	wantMigs := []migration{{1, "0001_init"}, {2, "0002_vectors"}}
	if !slices.Equal(gotMigs, wantMigs) {
		t.Errorf("schema_migrations = %v, want %v", gotMigs, wantMigs)
	}

	// 4. INSERT vectors с dim=1024 успешен.
	_, err = w.Exec(`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"narratives", 1, "bge-m3", 1024, "ru", "text", make([]byte, 4096), "2026-01-01T00:00:00Z")
	if err != nil {
		t.Errorf("INSERT vectors dim=1024: %v", err)
	}

	// 5. Тот же INSERT с dim=384 отклоняется CHECK-ом.
	_, err = w.Exec(`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"narratives", 1, "bge-m3", 384, "ru", "text", make([]byte, 4096), "2026-01-01T00:00:00Z")
	if err == nil {
		t.Errorf("INSERT vectors dim=384 should fail CHECK (dim = 1024)")
	}

	// 6. Запись через read-only хэндл даёт ошибку "readonly".
	r, err := OpenReader(path)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer r.Close()
	_, err = r.Exec(`INSERT INTO narratives (id, name, arc_summary, lang, created_at) VALUES (1, 'n', 's', 'ru', '2026-01-01T00:00:00Z')`)
	if err == nil {
		t.Errorf("INSERT narratives on read-only handle should fail")
	} else if !strings.Contains(err.Error(), "readonly") {
		t.Errorf("INSERT narratives error = %q, want containing %q", err.Error(), "readonly")
	}

	// 7. Колонки verses/patterns/characters.
	type colCheck struct {
		table string
		want  []string
	}
	checks := []colCheck{
		{"verses", []string{"id", "narrative_id", "text", "lang", "ord"}},
		{"patterns", []string{"id", "narrative_id", "kind", "text", "lang"}},
		{"characters", []string{"id", "narrative_id", "name", "traits_json"}},
	}
	for _, c := range checks {
		irows, err := w.Query(`PRAGMA table_info(` + c.table + `)`)
		if err != nil {
			t.Errorf("table_info(%s): %v", c.table, err)
			continue
		}
		var cols []string
		for irows.Next() {
			var cid int
			var name, typ string
			var notnull int
			var dflt sql.NullString
			var pk int
			if err := irows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				t.Errorf("table_info(%s) scan: %v", c.table, err)
			}
			cols = append(cols, name)
		}
		if err := irows.Err(); err != nil {
			t.Errorf("table_info(%s) rows: %v", c.table, err)
		}
		irows.Close()
		if !slices.Equal(cols, c.want) {
			t.Errorf("table %s columns = %v, want %v", c.table, cols, c.want)
		}
	}
}
