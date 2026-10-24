package birth

import (
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"tns65/internal/testkit"
)

// engineRoot возвращает корень engine/ — каталог с go.mod.
func engineRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("engine root (go.mod) not found")
		}
		dir = parent
	}
}

// realGazetteer — путь к реальному газетиру сида engine/seed/gazetteer.jsonl
// (T-01-16): «Проверка» T-02-16 обязана прогнать стартовый регион именно из
// этого файла, а не из фикстуры.
func realGazetteer(t *testing.T) string {
	t.Helper()
	return filepath.Join(engineRoot(t), "seed", "gazetteer.jsonl")
}

// gazetteerStartRegion — реальный/смешанный мир из газетира с радиусом 7 дней
// (175 км): в радиус Иерусалима попадает Тир (≈167 км), Александрия (≈506 км)
// и далее — за радиусом, поэтому мест ≥ 2 и путь ≥ 1.
func TestGeoSeedReal(t *testing.T) {
	dir := t.TempDir()
	a := validAncketa()
	a.TravelMode = travelReal

	res, err := Birth(a, Config{
		WorldsRoot:    dir,
		BaseDBPath:    testBaseDB(t, dir),
		Embed:         testkit.NewFakeEmbed(),
		GazetteerPath: realGazetteer(t),
		GeoRadiusDays: 7,
	})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}

	d := openReader(t, res.WorldDB)

	// Места ≥ 2 из газетира: стартовое и Тир в радиусе.
	var nPlaces int
	if err := d.QueryRow(`SELECT COUNT(*) FROM places`).Scan(&nPlaces); err != nil {
		t.Fatalf("places count: %v", err)
	}
	if nPlaces < 2 {
		t.Fatalf("places = %d, want ≥ 2", nPlaces)
	}

	names := placeNames(t, d)
	for _, want := range []string{"Иерусалим", "Тир"} {
		if !slices.Contains(names, want) {
			t.Errorf("в стартовом регионе нет места %q: %v", want, names)
		}
	}

	// Пути ≥ 1 между соседями кодом.
	var nPaths int
	if err := d.QueryRow(`SELECT COUNT(*) FROM paths`).Scan(&nPaths); err != nil {
		t.Fatalf("paths count: %v", err)
	}
	if nPaths < 1 {
		t.Fatalf("paths = %d, want ≥ 1", nPaths)
	}

	// У героя заполнен place_id стартового места (первое место газетира).
	assertHeroAtPlace(t, d, "Иерусалим")

	// В схеме нет поля дней.
	assertNoDaysColumn(t, d, "places")
	assertNoDaysColumn(t, d, "paths")
}

func TestGeoSeedFiction(t *testing.T) {
	dir := t.TempDir()
	a := validAncketa() // режим — вымышленная.
	a.Prologue = "Город у моря"

	res, err := Birth(a, Config{
		WorldsRoot: dir,
		BaseDBPath: testBaseDB(t, dir),
		Embed:      testkit.NewFakeEmbed(),
	})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}

	d := openReader(t, res.WorldDB)

	// Ровно одно место без путей.
	var nPlaces int
	if err := d.QueryRow(`SELECT COUNT(*) FROM places`).Scan(&nPlaces); err != nil {
		t.Fatalf("places count: %v", err)
	}
	if nPlaces != 1 {
		t.Fatalf("places = %d, want 1 (вымышленный мир)", nPlaces)
	}
	var nPaths int
	if err := d.QueryRow(`SELECT COUNT(*) FROM paths`).Scan(&nPaths); err != nil {
		t.Fatalf("paths count: %v", err)
	}
	if nPaths != 0 {
		t.Fatalf("paths = %d, want 0 (путей в вымышленном мире нет)", nPaths)
	}

	// Имя — из пролога.
	var name string
	if err := d.QueryRow(`SELECT name FROM places`).Scan(&name); err != nil {
		t.Fatalf("places name: %v", err)
	}
	if name != "Город у моря" {
		t.Fatalf("имя места = %q, want %q", name, "Город у моря")
	}

	assertHeroPlaceFilled(t, d)
	assertNoDaysColumn(t, d, "places")
	assertNoDaysColumn(t, d, "paths")
}

// TestGeoSeedMixed — смешанный мир: стартовый регион как в реальном — из
// реального газетира, радиус 7 дней.
func TestGeoSeedMixed(t *testing.T) {
	dir := t.TempDir()
	a := validAncketa()
	a.TravelMode = travelMixed

	res, err := Birth(a, Config{
		WorldsRoot:    dir,
		BaseDBPath:    testBaseDB(t, dir),
		Embed:         testkit.NewFakeEmbed(),
		GazetteerPath: realGazetteer(t),
		GeoRadiusDays: 7,
	})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}

	d := openReader(t, res.WorldDB)

	// Стартовый регион — как в реальном: места ≥ 2 из газетира, пути между ними.
	var nPlaces int
	if err := d.QueryRow(`SELECT COUNT(*) FROM places`).Scan(&nPlaces); err != nil {
		t.Fatalf("places count: %v", err)
	}
	if nPlaces < 2 {
		t.Fatalf("places = %d, want ≥ 2 (стартовый регион как в реальном)", nPlaces)
	}
	names := placeNames(t, d)
	for _, want := range []string{"Иерусалим", "Тир"} {
		if !slices.Contains(names, want) {
			t.Errorf("в стартовом регионе нет места %q: %v", want, names)
		}
	}
	var nPaths int
	if err := d.QueryRow(`SELECT COUNT(*) FROM paths`).Scan(&nPaths); err != nil {
		t.Fatalf("paths count: %v", err)
	}
	if nPaths < 1 {
		t.Fatalf("paths = %d, want ≥ 1", nPaths)
	}

	assertHeroAtPlace(t, d, "Иерусалим")
	assertNoDaysColumn(t, d, "places")
	assertNoDaysColumn(t, d, "paths")
}

// placeNames возвращает имена мест в порядке чтения.
func placeNames(t *testing.T, d *sql.DB) []string {
	t.Helper()
	rows, err := d.Query(`SELECT name FROM places`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

// assertHeroAtPlace проверяет, что герой (character_id 0) стоит в месте name.
func assertHeroAtPlace(t *testing.T, d *sql.DB, name string) {
	t.Helper()
	var placeID int64
	if err := d.QueryRow(`SELECT id FROM places WHERE name = ?`, name).Scan(&placeID); err != nil {
		t.Fatalf("id места %q: %v", name, err)
	}
	var got sql.NullInt64
	if err := d.QueryRow(`SELECT place_id FROM character_state WHERE character_id = ?`, heroCharacterID).Scan(&got); err != nil {
		t.Fatalf("character_state героя: %v", err)
	}
	if !got.Valid || got.Int64 != placeID {
		t.Errorf("place_id героя = %v, want %d", got, placeID)
	}
}

// assertHeroPlaceFilled проверяет, что place_id героя заполнен.
func assertHeroPlaceFilled(t *testing.T, d *sql.DB) {
	t.Helper()
	var got sql.NullInt64
	if err := d.QueryRow(`SELECT place_id FROM character_state WHERE character_id = ?`, heroCharacterID).Scan(&got); err != nil {
		t.Fatalf("character_state героя: %v", err)
	}
	if !got.Valid {
		t.Error("place_id героя не заполнен")
	}
}

// assertNoDaysColumn проверяет, что в таблице нет колонки, хранящей дни (W-06.2).
func assertNoDaysColumn(t *testing.T, d *sql.DB, table string) {
	t.Helper()
	rows, err := d.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("table_info(%s): %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("table_info(%s) scan: %v", table, err)
		}
		if strings.Contains(strings.ToLower(name), "day") {
			t.Errorf("%s: в схеме есть поле дней %q", table, name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table_info(%s) rows: %v", table, err)
	}
}
