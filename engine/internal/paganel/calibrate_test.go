package paganel_test

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"tns65/internal/db"
	"tns65/internal/geo"
	"tns65/internal/paganel"
)

// TestCalibrate — калибровка трёх режимов походов (T-06-03, «Проверка»): в
// выдуманном режиме фраза «две недели» задаёт дистанцию, с которой Days сходится;
// в смешанном смещение реальной точки не двигает книжные точки (pinned); в
// реальном дни считаются от координат газетира.
func TestCalibrate(t *testing.T) {
	// Шаг 1: режим читается из world_meta явно, из жанров не выводится.
	for _, mode := range []paganel.TravelMode{paganel.TravelReal, paganel.TravelFiction, paganel.TravelMixed} {
		h, _ := newCalibrateWorld(t)
		setTravelMode(t, h, string(mode))
		if got, err := paganel.ReadTravelMode(h); err != nil || got != mode {
			t.Errorf("ReadTravelMode(%q) = (%v, %v)", mode, got, err)
		}
		h.Close()
	}

	// Значение вне списка отвергается.
	h, _ := newCalibrateWorld(t)
	setTravelMode(t, h, "воздушная")
	if _, err := paganel.ReadTravelMode(h); err == nil {
		t.Error("ReadTravelMode(воздушная) принят, want отказ")
	}
	h.Close()

	// Без ключа travel_mode — отказ; из жанров режим не выводится.
	h, _ = newCalibrateWorld(t)
	if _, err := paganel.ReadTravelMode(h); err == nil {
		t.Error("ReadTravelMode без travel_mode принят, want отказ")
	}
	if _, err := h.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES ('genres', '["фэнтези"]')`); err != nil {
		h.Close()
		t.Fatalf("INSERT genres: %v", err)
	}
	if _, err := paganel.ReadTravelMode(h); err == nil {
		t.Error("ReadTravelMode вывел режим из жанров, want отказ")
	}
	h.Close()

	// Шаг 2: выдуманный мир — «две недели пути» задаёт дистанцию до упора.
	dist, err := paganel.FictionDistance("две недели пути", 25)
	if err != nil {
		t.Fatalf("FictionDistance(две недели пути): %v", err)
	}
	if want := 14.0 * 25.0; math.Abs(dist-want) > 1e-12 {
		t.Fatalf("FictionDistance = %v, want %v", dist, want)
	}

	// Две точки на расстоянии dist вдоль меридиана: Days без пути сходится к 14.
	latDeg := dist / (geo.EarthRadiusKm * math.Pi / 180)
	a := paganel.Place{ID: 1, X: 0, Y: 0}
	b := paganel.Place{ID: 2, X: 0, Y: latDeg}
	gm := &paganel.Geomodel{Places: []paganel.Place{a, b}}
	c, err := paganel.NewDaysCalc(25, gm)
	if err != nil {
		t.Fatalf("NewDaysCalc: %v", err)
	}
	if days := c.Days(a, b); math.Abs(days-14) > 1e-9 {
		t.Fatalf("Days с дистанцией «две недели» = %v, want 14", days)
	}

	// Названное расстояние берётся как есть (W-07.2).
	if dist, err := paganel.FictionDistance("300 км пути", 25); err != nil || math.Abs(dist-300) > 1e-12 {
		t.Fatalf("FictionDistance(300 км пути) = (%v, %v), want (300, nil)", dist, err)
	}

	// Шаг 3: реальный мир — дни считаются от координат газетира.
	from := paganel.Place{X: 0, Y: 0}
	to := paganel.Place{X: 0, Y: 1}
	gotReal := paganel.RealDays(from, to, 25)
	wantReal := geo.Distance(geo.Place{X: 0, Y: 0}, geo.Place{X: 0, Y: 1}) / 25
	if math.Abs(gotReal-wantReal) > 1e-12 {
		t.Fatalf("RealDays = %v, want %v", gotReal, wantReal)
	}

	// Шаг 4: смешанный мир — смещение реальной опорной точки не двигает книжные
	// точки (pinned); обратное встраивание запрещено.
	places := []paganel.MixedPlace{
		{Place: paganel.Place{Name: "Реальная", X: 30, Y: 40}},
		{Place: paganel.Place{Name: "Книжная", X: 31, Y: 41}, Pinned: true},
	}
	shifted := paganel.ShiftMixedCanvas(places, 5, -3)
	if shifted[0].X != 35 || shifted[0].Y != 37 {
		t.Errorf("реальная точка после сдвига = (%v, %v), want (35, 37)", shifted[0].X, shifted[0].Y)
	}
	if shifted[1].X != 31 || shifted[1].Y != 41 {
		t.Errorf("книжная точка сдвинулась: (%v, %v), want (31, 41)", shifted[1].X, shifted[1].Y)
	}
	if places[0].X != 30 || places[0].Y != 40 {
		t.Errorf("исходный срез изменён: (%v, %v), want (30, 40)", places[0].X, places[0].Y)
	}
}

// newCalibrateWorld создаёт оперативную базу мира с миграциями world+shared и
// открывает её хэндлом писателя forge; возвращает хэндл и путь к базе.
func newCalibrateWorld(t *testing.T) (*db.Handle, string) {
	t.Helper()
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
	w.Close()

	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	return h, path
}

// setTravelMode пишет travel_mode в world_meta как JSON-строку (T-02-03, шаг 3).
func setTravelMode(t *testing.T, h *db.Handle, mode string) {
	t.Helper()
	if _, err := h.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES ('travel_mode', ?)`,
		strconv.Quote(mode)); err != nil {
		t.Fatalf("INSERT travel_mode: %v", err)
	}
}
