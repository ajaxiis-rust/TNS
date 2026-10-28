package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/paganel"
)

// seedMapWorld создаёт базу мира alpha с геомоделью, картой игрока и графом и
// подменяет worldDBPath на реальную раскладку <root>/worlds/<slug>/<slug>.db.
// База живёт в t.TempDir(), как требует «Проверка» задач P6/P7.
func seedMapWorld(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	worldDir := filepath.Join(dir, "worlds", "alpha")
	if err := os.MkdirAll(worldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(worldDir, "alpha.db")

	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(raw, "world", "shared"); err != nil {
		raw.Close()
		t.Fatalf("Migrate: %v", err)
	}
	defer raw.Close()

	stmts := []string{
		`INSERT INTO places (id, name, kind, x, y) VALUES
			(1, 'Лес', 'место', 0, 1),
			(2, 'Город', 'место', 1, 0)`,
		`INSERT INTO paths (id, from_place_id, to_place_id, type, length_km, friction)
			VALUES (1, 1, 2, 'trakt', 100, 1)`,
		`INSERT INTO character_state (character_id, place_id, x, y) VALUES (0, 1, 0, 1)`,
		`INSERT INTO player_map (place_id, known_at, drawn) VALUES (1, '3', 0)`,
		`INSERT INTO rings (id, status, progress, budget_used, started_at, finished_at)
			VALUES (1, 'running', 0.4, 0, '2026-01-01T00:00:00Z', NULL)`,
		`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES
			(1, 'Альфа', 'INTJ', 'мещанин', '{}'),
			(2, 'Бета', 'ENFP', 'дворянин', '{}')`,
		`INSERT INTO social_graph (id, a_character_id, b_character_id, relation, weight, updated_at)
			VALUES (1, 1, 2, 'друг', 0.8, '2026-01-01T00:00:00Z')`,
	}
	for _, s := range stmts {
		if _, err := raw.Exec(s); err != nil {
			t.Fatalf("seed %s: %v", s, err)
		}
	}

	oldPath := worldDBPath
	worldDBPath = func(slug string) string {
		return filepath.Join(dir, "worlds", slug, slug+".db")
	}
	t.Cleanup(func() { worldDBPath = oldPath })
}

// mapRequest выполняет GET к эндпоинту карты/графа напрямую (маршрут не
// регистрируется в роутере — «Проверка» задачи проверяет хендлеры).
func mapRequest(t *testing.T, h http.HandlerFunc, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func TestMapViewport(t *testing.T) {
	seedMapWorld(t)

	rr := mapRequest(t, handleMapViewport, mapViewportPath+"?world=alpha")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", mapViewportPath, rr.Code, rr.Body.String())
	}

	var out mapViewportResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET %s: bad JSON: %v", mapViewportPath, err)
	}
	if out.World != "alpha" {
		t.Fatalf("world = %q, want alpha", out.World)
	}
	if len(out.Places) != 2 {
		t.Fatalf("places = %d, want 2", len(out.Places))
	}

	var forest, city *paganel.AtlasPlace
	for i := range out.Places {
		switch out.Places[i].Name {
		case "Лес":
			forest = &out.Places[i]
		case "Город":
			city = &out.Places[i]
		}
	}
	if forest == nil || city == nil {
		t.Fatalf("места «Лес»/«Город» отсутствуют: %+v", out.Places)
	}
	if !forest.Known || forest.KnownAt != "3" {
		t.Fatalf("«Лес» известность = (%v, %q), want (true, 3)", forest.Known, forest.KnownAt)
	}
	if city.Known {
		t.Fatal("«Город» помечен известным, want нет")
	}

	if len(out.Paths) != 1 {
		t.Fatalf("paths = %d, want 1", len(out.Paths))
	}
	if out.Paths[0].FromPlaceID != 1 || out.Paths[0].ToPlaceID != 2 || out.Paths[0].LengthKm != 100 {
		t.Fatalf("path = %+v, want (1→2, 100 км)", out.Paths[0])
	}
	if out.Hero.PlaceID != 1 {
		t.Fatalf("hero = %+v, want place_id 1", out.Hero)
	}
	if len(out.Rings) != 1 || out.Rings[0].Progress != 0.4 {
		t.Fatalf("rings = %+v, want одно кольцо с прогрессом 0.4", out.Rings)
	}

	// В ответе нет растровых и тайловых полей (W-20.3): только данные.
	body := rr.Body.String()
	for _, bad := range []string{"tile", "raster", "bitmap", "image", "pixel"} {
		if strings.Contains(strings.ToLower(body), bad) {
			t.Fatalf("в ответе есть растровое/тайловое поле %q: %s", bad, body)
		}
	}

	// Пустой слаг — 400; мира нет — 404.
	rr = mapRequest(t, handleMapViewport, mapViewportPath)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("GET без слага: status = %d, want 400", rr.Code)
	}
	rr = mapRequest(t, handleMapViewport, mapViewportPath+"?world=nope")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET несуществующего мира: status = %d, want 404", rr.Code)
	}
}

func TestRelationGraph(t *testing.T) {
	seedMapWorld(t)

	rr := mapRequest(t, handleRelationGraph, relationGraphPath+"?world=alpha")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", relationGraphPath, rr.Code, rr.Body.String())
	}

	var out relationGraphResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET %s: bad JSON: %v", relationGraphPath, err)
	}
	if out.World != "alpha" {
		t.Fatalf("world = %q, want alpha", out.World)
	}
	if len(out.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(out.Nodes))
	}
	if out.Nodes[0].Name != "Альфа" || out.Nodes[1].Name != "Бета" {
		t.Fatalf("nodes = %+v, want Альфа и Бета", out.Nodes)
	}
	if len(out.Edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(out.Edges))
	}
	e := out.Edges[0]
	if e.A != 1 || e.B != 2 || e.Relation != "друг" || e.Weight != 0.8 {
		t.Fatalf("edge = %+v, want (1→2, друг, 0.8)", e)
	}

	// Пустой слаг — 400; мира нет — 404.
	rr = mapRequest(t, handleRelationGraph, relationGraphPath)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("GET без слага: status = %d, want 400", rr.Code)
	}
	rr = mapRequest(t, handleRelationGraph, relationGraphPath+"?world=nope")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET несуществующего мира: status = %d, want 404", rr.Code)
	}
}
