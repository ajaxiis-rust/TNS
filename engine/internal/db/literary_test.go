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

	// 3. Учёт миграций: user_version == 3 и schema_migrations ==
	// [{1, "0001_init"}, {2, "0002_sediment_idempotency"}, {3, "0003_composition"}].
	var uv int
	if err := w.QueryRow(`PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	if uv != 3 {
		t.Errorf("user_version = %d, want 3", uv)
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

	wantMigs := []migration{{1, "0001_init"}, {2, "0002_sediment_idempotency"}, {3, "0003_composition"}}
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

	// 6. Контракт композиции (T-10-08): шесть новых колонок bank_pool,
	// CHECK inversion_flag IN (0,1), чтение/запись всех колонок.
	wantCols := []string{
		"id", "kind", "text", "lang", "tags", "trust_level", "origin",
		"source_scene_id", "created_at",
		"family_primary", "pattern_secondary", "technique", "moral_stance",
		"inversion_flag", "usage_count",
	}
	if got := colsOf(t, w, "bank_pool"); !slices.Equal(got, wantCols) {
		t.Errorf("bank_pool columns = %v, want %v", got, wantCols)
	}

	insertComposed := func(inversion int) error {
		_, err := w.Exec(`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at, family_primary, pattern_secondary, technique, moral_stance, inversion_flag, usage_count) VALUES ('drama', 't', 'ru', 't', 2, 'comp', '2026-01-01T00:00:00Z', 'escape_liberation', 'rescue', 'slow_revelation', 'tragic', ?, 7)`, inversion)
		return err
	}
	if err := insertComposed(1); err != nil {
		t.Errorf("INSERT bank_pool с композицией inversion=1: %v", err)
	}
	if err := insertComposed(2); err == nil {
		t.Errorf("INSERT bank_pool inversion_flag=2 should fail CHECK (inversion_flag)")
	}

	var fp, ps, tech, ms string
	var inv, uc int
	if err := w.QueryRow(`SELECT family_primary, pattern_secondary, technique, moral_stance, inversion_flag, usage_count FROM bank_pool WHERE origin = 'comp'`).
		Scan(&fp, &ps, &tech, &ms, &inv, &uc); err != nil {
		t.Fatalf("SELECT composition columns: %v", err)
	}
	if fp != "escape_liberation" || ps != "rescue" || tech != "slow_revelation" || ms != "tragic" || inv != 1 || uc != 7 {
		t.Errorf("composition row = (%q, %q, %q, %q, %d, %d)", fp, ps, tech, ms, inv, uc)
	}

	// 7. Старая фикстура (схема до 0003) с записью банка: после миграции
	// деградационные дефолты family_primary='' и inversion_flag=0 (SCR-§4).
	oldPath := filepath.Join(t.TempDir(), "literary-old.db")
	ow, err := OpenWriter(oldPath)
	if err != nil {
		t.Fatalf("OpenWriter(old): %v", err)
	}
	defer ow.Close()

	for _, name := range []string{"0001_init.sql", "0002_sediment_idempotency.sql"} {
		content, err := schemaFS.ReadFile("schema/literary/" + name)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if _, err := ow.Exec(string(content)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	if _, err := ow.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	for _, m := range []migration{{1, "0001_init"}, {2, "0002_sediment_idempotency"}} {
		if _, err := ow.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`, m.version, m.name, "2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("insert schema_migrations: %v", err)
		}
	}
	if _, err := ow.Exec(`PRAGMA user_version = 2`); err != nil {
		t.Fatalf("PRAGMA user_version = 2: %v", err)
	}
	if _, err := ow.Exec(`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at) VALUES ('drama', 'старая', 'ru', 't', 2, 'old', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert old bank_pool row: %v", err)
	}
	if err := Migrate(ow, "literary"); err != nil {
		t.Fatalf("Migrate(literary) over old fixture: %v", err)
	}
	var oldFP string
	var oldInv int
	if err := ow.QueryRow(`SELECT family_primary, inversion_flag FROM bank_pool WHERE origin = 'old'`).Scan(&oldFP, &oldInv); err != nil {
		t.Fatalf("old bank_pool row scan: %v", err)
	}
	if oldFP != "" || oldInv != 0 {
		t.Errorf("old bank_pool row defaults = (%q, %d), want (\"\", 0)", oldFP, oldInv)
	}
}
