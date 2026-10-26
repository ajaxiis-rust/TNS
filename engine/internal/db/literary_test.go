package db

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestLiterarySchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "literary.db")

	w, err := OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	defer w.Close()

	if err := Migrate(w, "literary"); err != nil {
		t.Fatalf("Migrate(literary): %v", err)
	}

	// 2. Список таблиц равен schema/literary/tables.txt.
	raw, err := schemaFS.ReadFile("schema/literary/tables.txt")
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

	// 6. Отдельный проход: в списке таблиц не должно быть имён с запрещёнными подстроками (D-14.2).
	for _, name := range got {
		for _, banned := range []string{"lexicon", "stoplist", "detector"} {
			if strings.Contains(name, banned) {
				t.Errorf("table %q contains banned substring %q (D-14.2)", name, banned)
			}
		}
	}

	// 3. Учёт миграций: user_version == 2 и schema_migrations ==
	// [{1, "0001_init"}, {2, "0002_sediment_idempotency"}].
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

	wantMigs := []migration{{1, "0001_init"}, {2, "0002_sediment_idempotency"}}
	if !slices.Equal(gotMigs, wantMigs) {
		t.Errorf("schema_migrations = %v, want %v", gotMigs, wantMigs)
	}

	// 3a. UNIQUE-индекс повторного оседания (T-04-10): по source_scene_id и уникален.
	if !indexHasCols(t, w, "sediment_scene_source_scene_id_idx", "source_scene_id") {
		t.Errorf("индекс sediment_scene_source_scene_id_idx (source_scene_id) отсутствует или не по той колонке")
	}
	irows, err := w.Query(`PRAGMA index_list(sediment_scene)`)
	if err != nil {
		t.Fatalf("PRAGMA index_list(sediment_scene): %v", err)
	}
	uniqueIdx := false
	for irows.Next() {
		var seq int
		var name string
		var unique int
		var origin, partial string
		if err := irows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("index_list scan: %v", err)
		}
		if name == "sediment_scene_source_scene_id_idx" {
			uniqueIdx = unique == 1
		}
	}
	irows.Close()
	if !uniqueIdx {
		t.Errorf("индекс sediment_scene_source_scene_id_idx не UNIQUE")
	}

	// 4. CHECK bank_pool.kind: 'fallback' проходит, 'poem' отклоняется.
	insertKind := func(kind string) error {
		_, err := w.Exec(`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			kind, "t", "ru", "t", 0, "o", "2026-01-01T00:00:00Z")
		return err
	}
	if err := insertKind("fallback"); err != nil {
		t.Errorf("INSERT bank_pool kind='fallback': %v", err)
	}
	if err := insertKind("poem"); err == nil {
		t.Errorf("INSERT bank_pool kind='poem' should fail CHECK (kind)")
	}

	// 5. CHECK trust_level: 0, 1, 2 проходят; 3 отклоняется.
	insertTrust := func(level int) error {
		_, err := w.Exec(`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"fallback", "t", "ru", "t", level, "o", "2026-01-01T00:00:00Z")
		return err
	}
	for _, level := range []int{0, 1, 2} {
		if err := insertTrust(level); err != nil {
			t.Errorf("INSERT bank_pool trust_level=%d: %v", level, err)
		}
	}
	if err := insertTrust(3); err == nil {
		t.Errorf("INSERT bank_pool trust_level=3 should fail CHECK (trust_level)")
	}
}
