package paganel_test

import (
	"os"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/paganel"
)

// TestGeomodel — геомодель: доменные типы мест и путей, трение по типу пути,
// чтение из world(<slug>).db по слагу и валидация типа (T-06-01, «Проверка»).
func TestGeomodel(t *testing.T) {
	// У каждого из трёх типов пути ненулевой коэффициент трения (шаг 3).
	if paganel.FrictionTrakt == 0 || paganel.FrictionRiver == 0 || paganel.FrictionSea == 0 {
		t.Fatalf("трения trakt/river/sea = %v/%v/%v, все должны быть ненулевыми",
			paganel.FrictionTrakt, paganel.FrictionRiver, paganel.FrictionSea)
	}

	// Типы trakt|river|sea принимаются, air отвергается (шаг 5).
	for _, typ := range []string{paganel.PathTypeTrakt, paganel.PathTypeRiver, paganel.PathTypeSea} {
		if err := paganel.ValidPathType(typ); err != nil {
			t.Fatalf("ValidPathType(%q) = %v, want nil", typ, err)
		}
	}
	if err := paganel.ValidPathType("air"); err == nil {
		t.Fatal("ValidPathType(air) = nil, want отказ")
	}

	// Чтение возвращает записи из places/paths по слагу мира (шаг 4).
	root := t.TempDir()
	dir := filepath.Join(root, "worlds", "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := paganel.WorldDBPath(root, "alpha")
	if path != filepath.Join(dir, "alpha.db") {
		t.Fatalf("WorldDBPath = %q, want %q", path, filepath.Join(dir, "alpha.db"))
	}

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
		t.Fatalf("INSERT places: %v", err)
	}
	if _, err := w.Exec(`INSERT INTO places (name, kind, x, y, epoch_ref) VALUES ('Бета', 'village', 31.0, 41.0, NULL)`); err != nil {
		w.Close()
		t.Fatalf("INSERT places: %v", err)
	}

	// trakt и sea без собственного трения (константа), river с собственным 1.4.
	if _, err := w.Exec(`INSERT INTO paths (from_place_id, to_place_id, type, length_km, friction) VALUES (1, 2, 'trakt', 100.0, NULL)`); err != nil {
		w.Close()
		t.Fatalf("INSERT paths trakt: %v", err)
	}
	if _, err := w.Exec(`INSERT INTO paths (from_place_id, to_place_id, type, length_km, friction) VALUES (2, 1, 'river', 50.0, 1.4)`); err != nil {
		w.Close()
		t.Fatalf("INSERT paths river: %v", err)
	}
	if _, err := w.Exec(`INSERT INTO paths (from_place_id, to_place_id, type, length_km, friction) VALUES (1, 2, 'sea', 200.0, NULL)`); err != nil {
		w.Close()
		t.Fatalf("INSERT paths sea: %v", err)
	}
	w.Close()

	g, err := paganel.LoadGeomodel(root, "alpha")
	if err != nil {
		t.Fatalf("LoadGeomodel: %v", err)
	}

	if len(g.Places) != 2 {
		t.Fatalf("мест = %d, want 2", len(g.Places))
	}
	if g.Places[0].Name != "Альфа" || g.Places[0].Kind != "city" {
		t.Errorf("место[0] = %+v", g.Places[0])
	}
	if g.Places[0].X != 30.0 || g.Places[0].Y != 40.0 || g.Places[0].EpochRef != "античность" {
		t.Errorf("место[0] координаты/эпоха = %+v", g.Places[0])
	}
	if g.Places[1].Name != "Бета" || g.Places[1].EpochRef != "" {
		t.Errorf("место[1] = %+v", g.Places[1])
	}

	if len(g.Paths) != 3 {
		t.Fatalf("путей = %d, want 3", len(g.Paths))
	}
	if g.Paths[0].Type != paganel.PathTypeTrakt || g.Paths[0].LengthKm != 100.0 {
		t.Errorf("путь[0] = %+v", g.Paths[0])
	}
	// Без собственного трения — константа типа.
	if g.Paths[0].FrictionValue() != paganel.FrictionTrakt {
		t.Errorf("путь trakt FrictionValue = %v, want %v", g.Paths[0].FrictionValue(), paganel.FrictionTrakt)
	}
	// С собственным трением — значение paths.friction, а не константа.
	if g.Paths[1].Type != paganel.PathTypeRiver || g.Paths[1].FrictionValue() != 1.4 {
		t.Errorf("путь river FrictionValue = %v, want 1.4 (собственный, не константа %v)", g.Paths[1].FrictionValue(), paganel.FrictionRiver)
	}
	if g.Paths[2].Type != paganel.PathTypeSea || g.Paths[2].FrictionValue() != paganel.FrictionSea {
		t.Errorf("путь sea FrictionValue = %v, want %v", g.Paths[2].FrictionValue(), paganel.FrictionSea)
	}
}
