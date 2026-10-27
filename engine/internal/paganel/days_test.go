package paganel_test

import (
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/geo"
	"tns65/internal/paganel"
)

// TestDays — дни на лету (T-06-02, «Проверка»): с путём — length_km/(норма·трение),
// без пути — геодезия/(норма·1.0); dayNormKm=10 отвергнут, 30 принят; изменение
// friction меняет дни без записи в базу; в places/paths нет колонки дней.
func TestDays(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "worlds", "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := paganel.WorldDBPath(root, "alpha")

	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(w, "world", "shared"); err != nil {
		w.Close()
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := w.Exec(`INSERT INTO places (name, kind, x, y, epoch_ref) VALUES ('Альфа', 'city', 30.0, 40.0, 'античность')`); err != nil {
		w.Close()
		t.Fatalf("INSERT places Альфа: %v", err)
	}
	if _, err := w.Exec(`INSERT INTO places (name, kind, x, y, epoch_ref) VALUES ('Бета', 'village', 31.0, 41.0, NULL)`); err != nil {
		w.Close()
		t.Fatalf("INSERT places Бета: %v", err)
	}
	if _, err := w.Exec(`INSERT INTO paths (from_place_id, to_place_id, type, length_km, friction) VALUES (1, 2, 'trakt', 100.0, NULL)`); err != nil {
		w.Close()
		t.Fatalf("INSERT paths: %v", err)
	}

	// Шаг 3 / W-06.2: в схемах places и paths нет поля с сохранёнными днями.
	for _, table := range []string{"places", "paths"} {
		for _, col := range tableColumns(t, w, table) {
			if strings.Contains(strings.ToLower(col), "day") {
				t.Errorf("%s содержит поле с днями: %q", table, col)
			}
		}
	}
	w.Close()

	gm, err := paganel.LoadGeomodel(root, "alpha")
	if err != nil {
		t.Fatalf("LoadGeomodel: %v", err)
	}
	if len(gm.Places) != 2 || len(gm.Paths) != 1 {
		t.Fatalf("мест/путей = %d/%d, want 2/1", len(gm.Places), len(gm.Paths))
	}

	// Шаг 2: dayNormKm=30 принят, 10 отвергнут, 20 (нижняя граница) принят.
	c, err := paganel.NewDaysCalc(30, gm)
	if err != nil {
		t.Fatalf("NewDaysCalc(30): %v", err)
	}
	if _, err := paganel.NewDaysCalc(10, gm); err == nil {
		t.Fatal("NewDaysCalc(10) принят, want отказ")
	}
	if _, err := paganel.NewDaysCalc(20, gm); err != nil {
		t.Fatalf("NewDaysCalc(20): %v", err)
	}

	from := gm.Places[0] // id=1, Альфа
	to := gm.Places[1]   // id=2, Бета

	// С известным путём: length_km/(норма·трение); trakt — константа трения 1.0.
	got := c.Days(from, to)
	want := 100.0 / (30.0 * paganel.FrictionTrakt)
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("Days с путём = %v, want %v", got, want)
	}

	// Без пути: геодезия/(норма·1.0).
	lone := paganel.Place{ID: 999, X: 0, Y: 0}
	gotNo := c.Days(from, lone)
	wantNo := geo.Distance(geo.Place{X: from.X, Y: from.Y}, geo.Place{X: lone.X, Y: lone.Y}) / 30.0
	if math.Abs(gotNo-wantNo) > 1e-12 {
		t.Fatalf("Days без пути = %v, want %v", gotNo, wantNo)
	}

	// Days только читает (W-06.5): вызовы не меняют число строк places/paths.
	placesBefore := countRows(t, path, "places")
	pathsBefore := countRows(t, path, "paths")
	_ = c.Days(from, to)
	_ = c.Days(from, lone)
	if placesAfter := countRows(t, path, "places"); placesAfter != placesBefore {
		t.Fatalf("Days записал строки в places: %d → %d", placesBefore, placesAfter)
	}
	if pathsAfter := countRows(t, path, "paths"); pathsAfter != pathsBefore {
		t.Fatalf("Days записал строки в paths: %d → %d", pathsBefore, pathsAfter)
	}

	// Изменение friction сразу меняет дни без миграции (W-06.5, шаг 3).
	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	if _, err := h.Exec("paths", `UPDATE paths SET friction = 3.0 WHERE id = 1`); err != nil {
		h.Close()
		t.Fatalf("UPDATE paths friction: %v", err)
	}
	h.Close()

	gm2, err := paganel.LoadGeomodel(root, "alpha")
	if err != nil {
		t.Fatalf("LoadGeomodel после правки friction: %v", err)
	}
	c2, err := paganel.NewDaysCalc(30, gm2)
	if err != nil {
		t.Fatalf("NewDaysCalc после правки friction: %v", err)
	}
	after := c2.Days(gm2.Places[0], gm2.Places[1])
	wantAfter := 100.0 / (30.0 * 3.0)
	if math.Abs(after-wantAfter) > 1e-12 {
		t.Fatalf("Days с трением 3.0 = %v, want %v", after, wantAfter)
	}
	if after == got {
		t.Fatal("изменение friction не меняет Days")
	}

	// Норма дня из world_meta (шаг 2): дефолт 25; 30 принят; 10 отвергнут.
	hm, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs (meta): %v", err)
	}
	defer hm.Close()
	if got, err := paganel.ReadDayNorm(hm); err != nil || got != 25 {
		t.Fatalf("ReadDayNorm() без записи = (%v, %v), want (25, nil)", got, err)
	}
	if _, err := hm.Exec("world_meta", `INSERT INTO world_meta ("key", value) VALUES ('dayNormKm', '30')`); err != nil {
		t.Fatalf("INSERT dayNormKm=30: %v", err)
	}
	if got, err := paganel.ReadDayNorm(hm); err != nil || got != 30 {
		t.Fatalf("ReadDayNorm() при 30 = (%v, %v), want (30, nil)", got, err)
	}
	if _, err := hm.Exec("world_meta", `UPDATE world_meta SET value = '10' WHERE "key" = 'dayNormKm'`); err != nil {
		t.Fatalf("UPDATE dayNormKm=10: %v", err)
	}
	if _, err := paganel.ReadDayNorm(hm); err == nil {
		t.Fatal("ReadDayNorm() при 10 принят, want отказ")
	}
}

// tableColumns возвращает имена колонок таблицы в порядке PRAGMA table_info.
func tableColumns(t *testing.T, w *sql.DB, table string) []string {
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

// countRows возвращает число строк таблицы в базе по пути.
func countRows(t *testing.T, path, table string) int {
	t.Helper()
	d, err := db.OpenReader(path)
	if err != nil {
		t.Fatalf("OpenReader(%s): %v", path, err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("COUNT(%s): %v", table, err)
	}
	return n
}
