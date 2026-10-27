package geo_test

import (
	"database/sql"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/geo"
)

// TestDays — расчёт дней кодом (T-05-13, «Проверка»): с путём — length_km,
// без пути — геодезия с трением 1.0; dayNormKm=10 отвергнут, 30 принят;
// изменение friction меняет дни без миграции; норма дня читается из world_meta.
func TestDays(t *testing.T) {
	// dayNormKm=30 принят (верхняя граница), 10 — вне диапазона [20,30].
	g, err := geo.NewGeo(30)
	if err != nil {
		t.Fatalf("NewGeo(30): %v", err)
	}
	if _, err := geo.NewGeo(10); err == nil {
		t.Fatal("NewGeo(10) принят, want отказ")
	}
	if _, err := geo.NewGeo(20); err != nil {
		t.Fatalf("NewGeo(20): %v", err)
	}

	a := geo.Place{X: 0, Y: 0}
	b := geo.Place{X: 0, Y: 1}

	// С известным путём: length_km / (dayNormKm · friction).
	got := g.Days(a, b, &geo.Path{LengthKm: 60, Friction: 1.5})
	want := 60.0 / (30.0 * 1.5)
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("Days с путём = %v, want %v", got, want)
	}

	// Без пути: геодезическое расстояние с трением 1.0 (оценка).
	gotNo := g.Days(a, b, nil)
	wantNo := geo.Distance(a, b) / 30.0
	if math.Abs(gotNo-wantNo) > 1e-12 {
		t.Fatalf("Days без пути = %v, want %v", gotNo, wantNo)
	}

	// Изменение friction меняет дни — без миграции данных (W-06.5).
	fast := g.Days(a, b, &geo.Path{LengthKm: 60, Friction: 1.0})
	slow := g.Days(a, b, &geo.Path{LengthKm: 60, Friction: 3.0})
	if fast == slow {
		t.Fatal("изменение friction не меняет дни")
	}
	if fast != 60.0/30.0 || slow != 60.0/90.0 {
		t.Fatalf("Days по friction: fast=%v slow=%v", fast, slow)
	}

	// dayNormKm из world_meta: нет записи — дефолт 25; 30 принят; 10 отвергнут.
	dir := t.TempDir()
	path := filepath.Join(dir, "world(g).db")
	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(w, "world", "shared"); err != nil {
		w.Close()
		t.Fatalf("Migrate: %v", err)
	}
	w.Close()

	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	defer h.Close()

	if got, err := geo.ReadDayNorm(h); err != nil || got != geo.DefaultDayNormKm {
		t.Fatalf("ReadDayNorm() без записи = (%v, %v), want (%v, nil)", got, err, geo.DefaultDayNormKm)
	}

	if _, err := h.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES ('dayNormKm', '30')`); err != nil {
		t.Fatalf("INSERT dayNormKm=30: %v", err)
	}
	if got, err := geo.ReadDayNorm(h); err != nil || got != 30 {
		t.Fatalf("ReadDayNorm() при 30 = (%v, %v), want (30, nil)", got, err)
	}

	if _, err := h.Exec("world_meta",
		`UPDATE world_meta SET value = '10' WHERE "key" = 'dayNormKm'`); err != nil {
		t.Fatalf("UPDATE dayNormKm=10: %v", err)
	}
	if _, err := geo.ReadDayNorm(h); err == nil {
		t.Fatal("ReadDayNorm() при 10 принят, want отказ")
	}
}

// TestDistance — геодезическое расстояние по формуле гаверсинусов (T-05-13,
// шаг 1): один градус меридиана, симметрия и нулевое расстояние.
func TestDistance(t *testing.T) {
	a := geo.Place{X: 0, Y: 0}
	b := geo.Place{X: 0, Y: 1}

	// Один градус меридиана = R·(π/180) километров.
	want := geo.EarthRadiusKm * math.Pi / 180
	if got := geo.Distance(a, b); math.Abs(got-want) > 1e-9 {
		t.Fatalf("Distance(1°) = %v, want %v", got, want)
	}

	if got := geo.Distance(b, a); math.Abs(got-geo.Distance(a, b)) > 1e-12 {
		t.Fatalf("Distance несимметрична: %v vs %v", got, geo.Distance(a, b))
	}
	if got := geo.Distance(a, a); got != 0 {
		t.Fatalf("Distance(a, a) = %v, want 0", got)
	}
}

// TestNoDaysColumn — дни никогда не хранятся (T-05-13, шаг 3): в схемах places
// и paths нет поля с сохранёнными днями (по PRAGMA table_info).
func TestNoDaysColumn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "world(n).db")
	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	defer w.Close()

	if err := db.Migrate(w, "world", "shared"); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, table := range []string{"places", "paths"} {
		for _, col := range tableColumns(t, w, table) {
			if strings.Contains(strings.ToLower(col), "day") {
				t.Errorf("%s содержит поле с днями: %q", table, col)
			}
		}
	}
}

// TestPromptGate — промпт-гейт (T-05-13, шаг 5): сборка промпта не содержит
// запретных подстрок «посчитай расстояние» / «посчитай дни»; подстроки ловятся.
func TestPromptGate(t *testing.T) {
	a := geo.Place{X: 55.7558, Y: 37.6173}
	b := geo.Place{X: 59.9343, Y: 30.3351}
	d := geo.Distance(a, b)
	days := d / geo.DefaultDayNormKm

	prompt := geo.TravelPrompt(a, b, d, days)
	if err := geo.CheckPrompt(prompt); err != nil {
		t.Fatalf("промпт-гейт отклонил собственную сборку: %v", err)
	}
	if strings.Contains(prompt, "посчитай") {
		t.Fatalf("сборка промпта содержит «посчитай»: %s", prompt)
	}

	if err := geo.CheckPrompt("посчитай расстояние сам"); err == nil {
		t.Fatal("запретная подстрока «посчитай расстояние» принята")
	}
	if err := geo.CheckPrompt("посчитай дни сам"); err == nil {
		t.Fatal("запретная подстрока «посчитай дни» принята")
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
