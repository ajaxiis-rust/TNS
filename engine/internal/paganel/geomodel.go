package paganel

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"tns65/internal/db"
)

// Типы пути (W-06.1): тракт, река, море. Список закрыт — иные типы отвергаются
// CHECK из T-01-06 и ValidPathType.
const (
	PathTypeTrakt = "trakt"
	PathTypeRiver = "river"
	PathTypeSea   = "sea"
)

// Коэффициенты трения по типу пути (T-06-01, шаг 3): меньше трение — быстрее
// путь. Константа — дефолт для нового пути; значение paths.friction конкретного
// пути имеет приоритет.
const (
	FrictionTrakt = 1.0
	FrictionRiver = 0.7
	FrictionSea   = 0.5
)

// pathTypeFriction — дефолтное трение каждого типа пути.
var pathTypeFriction = map[string]float64{
	PathTypeTrakt: FrictionTrakt,
	PathTypeRiver: FrictionRiver,
	PathTypeSea:   FrictionSea,
}

// ValidPathType — тип пути обязан быть одним из trakt/river/sea (T-06-01, шаг 5;
// согласовано с CHECK paths.type из T-01-06).
func ValidPathType(pathType string) error {
	if _, ok := pathTypeFriction[pathType]; !ok {
		return fmt.Errorf("paganel: тип пути %q вне списка trakt|river|sea", pathType)
	}
	return nil
}

// TypeFriction — дефолтное трение типа пути; неизвестный тип — 0.
func TypeFriction(pathType string) float64 {
	return pathTypeFriction[pathType]
}

// Place — место из places (T-06-01, шаг 2): id, имя, род, реальные координаты
// (x — долгота, y — широта) и ссылка на эпоху.
type Place struct {
	ID       int64
	Name     string
	Kind     string
	X        float64
	Y        float64
	EpochRef string
}

// Path — путь из paths (T-06-01, шаг 2): id, концы, тип, длина в км и трение.
// Friction — значение paths.friction конкретного пути (0, когда не задано);
// эффективное трение возвращает FrictionValue.
type Path struct {
	ID          int64
	FromPlaceID int64
	ToPlaceID   int64
	Type        string
	LengthKm    float64
	Friction    float64
	frictionSet bool
}

// FrictionValue — трение пути: приоритет — значение paths.friction конкретного
// пути, иначе константа типа (T-06-01, шаг 3).
func (p Path) FrictionValue() float64 {
	if p.frictionSet {
		return p.Friction
	}
	return TypeFriction(p.Type)
}

// Geomodel — геомодель мира: места и пути, считанные из оперативной базы.
type Geomodel struct {
	Places []Place
	Paths  []Path
}

// WorldDBPath — путь к оперативной базе мира по корню миров и слагу:
// <root>/worlds/<slug>/<slug>.db (B-01.1, T-02-10). Пустой root означает
// db.WorldsRoot.
func WorldDBPath(root, slug string) string {
	if root == "" {
		root = db.WorldsRoot
	}
	return filepath.Join(root, "worlds", slug, slug+".db")
}

// LoadGeomodel читает места и пути из оперативной базы мира по слагу
// (T-06-01, шаг 4).
func LoadGeomodel(root, slug string) (*Geomodel, error) {
	return loadGeomodelFromPath(WorldDBPath(root, slug))
}

// loadGeomodelFromPath читает места и пути из файла оперативной базы мира
// (только чтение, db.OpenReader).
func loadGeomodelFromPath(path string) (*Geomodel, error) {
	d, err := db.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("paganel: открыть базу мира %s: %w", path, err)
	}
	defer d.Close()

	g := &Geomodel{}
	if g.Places, err = readPlaces(d); err != nil {
		return nil, err
	}
	if g.Paths, err = readPaths(d); err != nil {
		return nil, err
	}
	return g, nil
}

// readPlaces читает таблицу places целиком.
func readPlaces(d *sql.DB) ([]Place, error) {
	rows, err := d.Query(`SELECT id, name, kind, x, y, epoch_ref FROM places ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("paganel: читать places: %w", err)
	}
	defer rows.Close()

	var places []Place
	for rows.Next() {
		var p Place
		var x, y sql.NullFloat64
		var epoch sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.Kind, &x, &y, &epoch); err != nil {
			return nil, fmt.Errorf("paganel: разбор места: %w", err)
		}
		p.X, p.Y = x.Float64, y.Float64
		p.EpochRef = epoch.String
		places = append(places, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("paganel: читать places: %w", err)
	}
	return places, nil
}

// readPaths читает таблицу paths целиком и проверяет тип каждого пути (шаг 5).
func readPaths(d *sql.DB) ([]Path, error) {
	rows, err := d.Query(`SELECT id, from_place_id, to_place_id, type, length_km, friction FROM paths ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("paganel: читать paths: %w", err)
	}
	defer rows.Close()

	var paths []Path
	for rows.Next() {
		var p Path
		var length, friction sql.NullFloat64
		if err := rows.Scan(&p.ID, &p.FromPlaceID, &p.ToPlaceID, &p.Type, &length, &friction); err != nil {
			return nil, fmt.Errorf("paganel: разбор пути: %w", err)
		}
		if err := ValidPathType(p.Type); err != nil {
			return nil, fmt.Errorf("paganel: путь %d: %w", p.ID, err)
		}
		p.LengthKm = length.Float64
		p.Friction = friction.Float64
		p.frictionSet = friction.Valid
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("paganel: читать paths: %w", err)
	}
	return paths, nil
}
