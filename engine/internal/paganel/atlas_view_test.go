package paganel_test

import (
	"encoding/json"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/geo"
	"tns65/internal/paganel"
)

// seedAtlasWorld готовит базу мира: два места (одно известное игроку), путь,
// положение героя у места и кольцо в работе с прогрессом.
func seedAtlasWorld(t *testing.T) string {
	t.Helper()
	path := newRingWorld(t)
	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	defer raw.Close()

	stmts := []string{
		`INSERT INTO places (id, name, kind, x, y, origin, status) VALUES
			(1, 'Лес', 'место', 0, 1, 'paganel', 'baked'),
			(2, 'Город', 'место', 1, 0, 'paganel', 'baked')`,
		`INSERT INTO paths (id, from_place_id, to_place_id, type, length_km, friction, origin, status)
			VALUES (1, 1, 2, 'trakt', 100, 1, 'paganel', 'baked')`,
		`INSERT INTO character_state (character_id, place_id, x, y) VALUES (0, 1, 0, 1)`,
		`INSERT INTO player_map (place_id, known_at, drawn) VALUES (1, '3', 0)`,
		`INSERT INTO rings (id, status, progress, budget_used, started_at, finished_at)
			VALUES (1, 'running', 0.4, 0, '2026-01-01T00:00:00Z', NULL)`,
	}
	for _, s := range stmts {
		if _, err := raw.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return path
}

// openAtlasReader открывает базу мира только на чтение для вьюпорта (T-06-13,
// шаг 4: вьюпорт read-only).
func openAtlasReader(t *testing.T, path string) *db.Handle {
	t.Helper()
	h, err := db.OpenReaderAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenReaderAs: %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

// atlasPlace ищет место вьюпорта по имени; false — места нет.
func atlasPlace(t *testing.T, v paganel.AtlasView, name string) (paganel.AtlasPlace, bool) {
	t.Helper()
	for _, p := range v.Places {
		if p.Name == name {
			return p, true
		}
	}
	return paganel.AtlasPlace{}, false
}

// TestAtlasView — вьюпорт Атласа (T-06-13, «Проверка»): возвращает места/пути/
// положение и прогресс колец; в ответе нет растровых и тайловых полей; в
// промпте нет задач на вычисление расстояний/дней.
func TestAtlasView(t *testing.T) {
	t.Run("вьюпорт возвращает места, пути, положение и прогресс колец", func(t *testing.T) {
		path := seedAtlasWorld(t)
		h := openAtlasReader(t, path)

		v, err := paganel.BuildAtlasView(h)
		if err != nil {
			t.Fatalf("BuildAtlasView: %v", err)
		}

		if len(v.Places) != 2 {
			t.Fatalf("мест = %d, want 2", len(v.Places))
		}
		forest, ok := atlasPlace(t, v, "Лес")
		if !ok {
			t.Fatal("место «Лес» отсутствует во вьюпорте")
		}
		if !forest.Known || forest.KnownAt != "3" {
			t.Fatalf("«Лес» известность = (%v, %q), want (true, 3)", forest.Known, forest.KnownAt)
		}
		if forest.DistanceKm != 0 || forest.Days != 0 {
			t.Fatalf("«Лес» от героя = (%v км, %v дней), want (0, 0)", forest.DistanceKm, forest.Days)
		}
		if city, ok := atlasPlace(t, v, "Город"); !ok {
			t.Fatal("место «Город» отсутствует во вьюпорте")
		} else if city.Known {
			t.Fatal("место «Город» помечено известным, want нет")
		} else if city.DistanceKm <= 0 || city.Days <= 0 {
			t.Fatalf("«Город» от героя = (%v км, %v дней), want > 0", city.DistanceKm, city.Days)
		}

		if len(v.Paths) != 1 {
			t.Fatalf("путей = %d, want 1", len(v.Paths))
		}
		p := v.Paths[0]
		if p.FromPlaceID != 1 || p.ToPlaceID != 2 {
			t.Fatalf("концы пути = (%d, %d), want (1, 2)", p.FromPlaceID, p.ToPlaceID)
		}
		if p.FromX != 0 || p.FromY != 1 || p.ToX != 1 || p.ToY != 0 {
			t.Fatalf("координаты концов пути = (%v,%v → %v,%v), want (0,1 → 1,0)",
				p.FromX, p.FromY, p.ToX, p.ToY)
		}
		if p.LengthKm != 100 {
			t.Fatalf("длина пути = %v, want 100", p.LengthKm)
		}
		if p.Days != 4 {
			t.Fatalf("дни пути = %v, want 4 (100 км / (25 км/день · 1))", p.Days)
		}

		if v.Hero.PlaceID != 1 || v.Hero.X != 0 || v.Hero.Y != 1 {
			t.Fatalf("положение героя = (%d, %v, %v), want (1, 0, 1)", v.Hero.PlaceID, v.Hero.X, v.Hero.Y)
		}

		if len(v.Rings) != 1 {
			t.Fatalf("колец = %d, want 1", len(v.Rings))
		}
		if v.Rings[0].Status != paganel.RingRunning || v.Rings[0].Progress != 0.4 {
			t.Fatalf("кольцо = (%q, %v), want (running, 0.4)", v.Rings[0].Status, v.Rings[0].Progress)
		}
	})

	t.Run("в ответе нет растровых и тайловых полей", func(t *testing.T) {
		path := seedAtlasWorld(t)
		h := openAtlasReader(t, path)

		v, err := paganel.BuildAtlasView(h)
		if err != nil {
			t.Fatalf("BuildAtlasView: %v", err)
		}
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		s := string(b)

		for _, want := range []string{`"places"`, `"paths"`, `"hero"`, `"rings"`} {
			if !strings.Contains(s, want) {
				t.Fatalf("в ответе нет поля %s: %s", want, s)
			}
		}
		for _, bad := range []string{"tile", "raster", "bitmap", "image", "pixel"} {
			if strings.Contains(strings.ToLower(s), bad) {
				t.Fatalf("в ответе есть растровое/тайловое поле %q: %s", bad, s)
			}
		}
	})

	t.Run("в промпте нет задач на вычисление расстояний/дней", func(t *testing.T) {
		path := seedAtlasWorld(t)
		h := openAtlasReader(t, path)

		v, err := paganel.BuildAtlasView(h)
		if err != nil {
			t.Fatalf("BuildAtlasView: %v", err)
		}
		prompt := paganel.AtlasViewPrompt(v)

		if err := geo.CheckPrompt(prompt); err != nil {
			t.Fatalf("промпт-гейт отклонил собственную сборку: %v", err)
		}
		for _, word := range []string{"посчитай", "вычисли", "рассчитай", "определи расстояние", "определи дни"} {
			if strings.Contains(strings.ToLower(prompt), word) {
				t.Fatalf("промпт содержит задачу на вычисление %q: %s", word, prompt)
			}
		}
		if !strings.Contains(prompt, "100") || !strings.Contains(prompt, "4") {
			t.Fatalf("промпт не несёт готовые числа длины/дней: %s", prompt)
		}
	})
}
