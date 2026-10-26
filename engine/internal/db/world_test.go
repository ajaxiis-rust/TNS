package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestWorldSchema(t *testing.T) {
	dir := t.TempDir()
	slug := "alpha"
	path := filepath.Join(dir, "world("+slug+").db")

	w, err := OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	defer w.Close()

	if err := Migrate(w, "world", "shared"); err != nil {
		t.Fatalf("Migrate(world, shared): %v", err)
	}

	// 1. По слагу создаётся файл world(<slug>).db.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("world db file %q not created: %v", path, err)
	}

	// 2. Список таблиц равен schema/world/tables.txt.
	raw, err := schemaFS.ReadFile("schema/world/tables.txt")
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

	// 3. Учёт миграций: общий vectors подключён отдельным шагом, очередь
	// мастерской — третьей миграцией, Пила и арка сцены — четвёртой, сигналы
	// игрока — пятой (user_version == 5).
	var uv int
	if err := w.QueryRow(`PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	if uv != 5 {
		t.Errorf("user_version = %d, want 5", uv)
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

	wantMigs := []migration{{1, "0001_init"}, {2, "0002_vectors"}, {3, "0003_workshop_queue"}, {4, "0004_saw"}, {5, "0005_scene_signals"}}
	if !slices.Equal(gotMigs, wantMigs) {
		t.Errorf("schema_migrations = %v, want %v", gotMigs, wantMigs)
	}

	// 4. CHECK paths.type: только trakt/river/sea.
	insertPathType := func(typ string) error {
		_, err := w.Exec(`INSERT INTO paths (from_place_id, to_place_id, type) VALUES (1, 2, ?)`, typ)
		return err
	}
	if err := insertPathType("trakt"); err != nil {
		t.Errorf("INSERT paths type='trakt': %v", err)
	}
	if err := insertPathType("road"); err == nil {
		t.Errorf("INSERT paths type='road' should fail CHECK (type)")
	}

	// 5. CHECK book_chapters.status: 'published' проходит, 'draft' отклоняется.
	insertChapterStatus := func(status string) error {
		_, err := w.Exec(`INSERT INTO book_chapters (day, title, status, kind) VALUES (1, 't', ?, 'chapter')`, status)
		return err
	}
	if err := insertChapterStatus("published"); err != nil {
		t.Errorf("INSERT book_chapters status='published': %v", err)
	}
	if err := insertChapterStatus("draft"); err == nil {
		t.Errorf("INSERT book_chapters status='draft' should fail CHECK (status)")
	}

	// 6. В character_state есть колонки x и y.
	irows, err := w.Query(`PRAGMA table_info(character_state)`)
	if err != nil {
		t.Fatalf("table_info(character_state): %v", err)
	}
	var cols []string
	for irows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := irows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("table_info(character_state) scan: %v", err)
		}
		cols = append(cols, name)
	}
	if err := irows.Err(); err != nil {
		t.Fatalf("table_info(character_state) rows: %v", err)
	}
	irows.Close()

	wantCols := []string{"character_id", "place_id", "x", "y", "mood", "status"}
	if !slices.Equal(cols, wantCols) {
		t.Errorf("character_state columns = %v, want %v", cols, wantCols)
	}

	// 7. Контракт DDL Пилы и арки сцены (T-03-06): колонки обеих таблиц и
	// индекс saw_state по (character_id, thread_id).
	if got := colsOf(t, w, "saw_state"); !slices.Equal(got, []string{"character_id", "thread_id", "value", "updated_at"}) {
		t.Errorf("saw_state columns = %v", got)
	}
	if got := colsOf(t, w, "scene_arc"); !slices.Equal(got, []string{"scene_id", "value", "updated_at"}) {
		t.Errorf("scene_arc columns = %v", got)
	}
	if !indexHasCols(t, w, "saw_state_character_thread_idx", "character_id", "thread_id") {
		t.Errorf("saw_state index on (character_id, thread_id) missing")
	}

	// 8. Контракт DDL сигналов игрока (T-04-06): колонки scene_signals и
	// CHECK kind IN ('like','dislike').
	if got := colsOf(t, w, "scene_signals"); !slices.Equal(got, []string{"scene_id", "kind", "created_at"}) {
		t.Errorf("scene_signals columns = %v", got)
	}
	insertSignal := func(kind string) error {
		_, err := w.Exec(`INSERT INTO scene_signals (scene_id, kind, created_at) VALUES (1, ?, '2026-01-01T00:00:00Z')`, kind)
		return err
	}
	if err := insertSignal("like"); err != nil {
		t.Errorf("INSERT scene_signals kind='like': %v", err)
	}
	if err := insertSignal("dislike"); err != nil {
		t.Errorf("INSERT scene_signals kind='dislike': %v", err)
	}
	if err := insertSignal("neutral"); err == nil {
		t.Errorf("INSERT scene_signals kind='neutral' should fail CHECK (kind)")
	}
}

// colsOf возвращает имена колонок таблицы в порядке PRAGMA table_info.
func colsOf(t *testing.T, w *sql.DB, table string) []string {
	t.Helper()
	rows, err := w.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("table_info(%s): %v", table, err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("table_info(%s) scan: %v", table, err)
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table_info(%s) rows: %v", table, err)
	}
	return cols
}

// indexHasCols проверяет, что индекс indexName покрывает ровно заданные колонки
// в заданном порядке (по PRAGMA index_info).
func indexHasCols(t *testing.T, w *sql.DB, indexName string, want ...string) bool {
	t.Helper()
	rows, err := w.Query(`PRAGMA index_info(` + indexName + `)`)
	if err != nil {
		t.Fatalf("index_info(%s): %v", indexName, err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var seqno, cid int
		var name sql.NullString
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			t.Fatalf("index_info(%s) scan: %v", indexName, err)
		}
		got = append(got, name.String)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("index_info(%s) rows: %v", indexName, err)
	}
	return slices.Equal(got, want)
}
