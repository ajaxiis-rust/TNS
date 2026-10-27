package paganel

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"tns65/internal/db"
	"tns65/internal/geo"
)

// Вьюпорт Атласа (T-06-13, UI-05, W-20): данные «Прорисовки карты» для
// игрока. Рендер карты — на стороне браузера (F-11), вьюпорт отдаёт только
// данные из world(<slug>).db и ничего не пишет (W-20.4).

// AtlasPlace — место в данных вьюпорта: координаты как есть, известность
// игроку и готовые расстояние/дни от героя (считает код, W-20.1).
type AtlasPlace struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Kind       string  `json:"kind"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Known      bool    `json:"known"`
	KnownAt    string  `json:"known_at,omitempty"`
	DistanceKm float64 `json:"distance_km"`
	Days       float64 `json:"days"`
}

// AtlasPath — путь в данных вьюпорта: концы с координатами, тип, длина и дни.
// Дни — готовое число от кода (W-20.1).
type AtlasPath struct {
	ID          int64   `json:"id"`
	FromPlaceID int64   `json:"from_place_id"`
	ToPlaceID   int64   `json:"to_place_id"`
	FromX       float64 `json:"from_x"`
	FromY       float64 `json:"from_y"`
	ToX         float64 `json:"to_x"`
	ToY         float64 `json:"to_y"`
	Type        string  `json:"type"`
	LengthKm    float64 `json:"length_km"`
	Friction    float64 `json:"friction"`
	Days        float64 `json:"days"`
}

// AtlasHero — текущее положение героя (character_state, T-06-04).
type AtlasHero struct {
	PlaceID int64   `json:"place_id"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
}

// AtlasRing — кольцо Паганеля с прогрессом (rings, T-06-05).
type AtlasRing struct {
	ID       int64   `json:"id"`
	Status   string  `json:"status"`
	Progress float64 `json:"progress"`
}

// AtlasView — данные «Прорисовки карты» для UI-05 (T-06-13, шаг 1): места и
// пути с координатами, известность, кольца с прогрессом, положение героя.
// Растровых и тайловых полей в контракте нет — карту рисует браузер (W-20.3).
type AtlasView struct {
	Places []AtlasPlace `json:"places"`
	Paths  []AtlasPath  `json:"paths"`
	Hero   AtlasHero    `json:"hero"`
	Rings  []AtlasRing  `json:"rings"`
}

// BuildAtlasView собирает данные «Прорисовки карты» из world(<slug>).db
// (T-06-13, шаги 1 и 4): только чтение. Координаты передаются как есть,
// расстояния и дни — готовыми числами от кода (W-20.1).
func BuildAtlasView(h *db.Handle) (AtlasView, error) {
	var v AtlasView
	if h == nil {
		return v, fmt.Errorf("paganel: хэндл мира не задан")
	}

	hero, err := ReadHeroPosition(h)
	if err != nil {
		return v, err
	}
	v.Hero = AtlasHero{PlaceID: hero.PlaceID, X: hero.X, Y: hero.Y}

	norm, err := ReadDayNorm(h)
	if err != nil {
		return v, err
	}

	places, err := readAtlasPlaces(h, hero, norm)
	if err != nil {
		return v, err
	}
	v.Places = places

	paths, err := readAtlasPaths(h, places, norm)
	if err != nil {
		return v, err
	}
	v.Paths = paths

	rings, err := readAtlasRings(h)
	if err != nil {
		return v, err
	}
	v.Rings = rings
	return v, nil
}

// readAtlasPlaces читает места с известностью из player_map (T-06-12) и считает
// расстояние/дни от героя (готовые числа, W-20.1).
func readAtlasPlaces(h *db.Handle, hero HeroPosition, norm float64) ([]AtlasPlace, error) {
	known := make(map[int64]string)
	pm, err := ReadPlayerMap(h)
	if err != nil {
		return nil, err
	}
	for _, p := range pm {
		known[p.PlaceID] = p.KnownAt
	}

	rows, err := h.Query(`SELECT id, name, kind, COALESCE(x, 0), COALESCE(y, 0) FROM places ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("paganel: читать places: %w", err)
	}
	defer rows.Close()

	var out []AtlasPlace
	for rows.Next() {
		var p AtlasPlace
		if err := rows.Scan(&p.ID, &p.Name, &p.Kind, &p.X, &p.Y); err != nil {
			return nil, fmt.Errorf("paganel: разбор места: %w", err)
		}
		if knownAt, ok := known[p.ID]; ok {
			p.Known = true
			p.KnownAt = knownAt
		}
		p.DistanceKm = geo.Distance(geo.Place{X: hero.X, Y: hero.Y}, geo.Place{X: p.X, Y: p.Y})
		p.Days = p.DistanceKm / norm
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("paganel: читать places: %w", err)
	}
	return out, nil
}

// readAtlasPaths читает пути, подставляет координаты концов из мест и считает
// дни кодом (длина / (норма дня · трение), W-06.3).
func readAtlasPaths(h *db.Handle, places []AtlasPlace, norm float64) ([]AtlasPath, error) {
	coords := make(map[int64]AtlasPlace, len(places))
	for _, p := range places {
		coords[p.ID] = p
	}

	rows, err := h.Query(
		`SELECT id, from_place_id, to_place_id, type, COALESCE(length_km, 0), friction
		 FROM paths ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("paganel: читать paths: %w", err)
	}
	defer rows.Close()

	var out []AtlasPath
	for rows.Next() {
		var p AtlasPath
		var friction sql.NullFloat64
		if err := rows.Scan(&p.ID, &p.FromPlaceID, &p.ToPlaceID, &p.Type, &p.LengthKm, &friction); err != nil {
			return nil, fmt.Errorf("paganel: разбор пути: %w", err)
		}
		p.Friction = TypeFriction(p.Type)
		if friction.Valid {
			p.Friction = friction.Float64
		}
		if from, ok := coords[p.FromPlaceID]; ok {
			p.FromX, p.FromY = from.X, from.Y
		}
		if to, ok := coords[p.ToPlaceID]; ok {
			p.ToX, p.ToY = to.X, to.Y
		}
		p.Days = p.LengthKm / (norm * p.Friction)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("paganel: читать paths: %w", err)
	}
	return out, nil
}

// readAtlasRings читает кольца Паганеля с прогрессом (только чтение).
func readAtlasRings(h *db.Handle) ([]AtlasRing, error) {
	rows, err := h.Query(`SELECT id, status, progress FROM rings ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("paganel: читать rings: %w", err)
	}
	defer rows.Close()

	var out []AtlasRing
	for rows.Next() {
		var r AtlasRing
		if err := rows.Scan(&r.ID, &r.Status, &r.Progress); err != nil {
			return nil, fmt.Errorf("paganel: разбор кольца: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("paganel: читать rings: %w", err)
	}
	return out, nil
}

// AtlasViewPrompt — промпт вьюпорта Атласа (T-06-13, шаг 2, W-20.2): координаты
// передаются как есть, расстояния и дни — готовыми числами от кода. Задач на
// вычисление расстояний/дней в промпте нет.
func AtlasViewPrompt(v AtlasView) string {
	var b strings.Builder
	b.WriteString("Карта мира игрока.")
	if len(v.Places) == 0 {
		b.WriteString(" Известных мест пока нет.")
	} else {
		b.WriteString(" Места: ")
		for i, p := range v.Places {
			if i > 0 {
				b.WriteString("; ")
			}
			fmt.Fprintf(&b, "%s (долгота %s, широта %s), от героя %s км (%s дней)",
				p.Name, atlasNum(p.X), atlasNum(p.Y), atlasNum(p.DistanceKm), atlasNum(p.Days))
			if p.Known {
				b.WriteString(", известно с дня " + p.KnownAt)
			}
		}
		b.WriteString(".")
	}
	if len(v.Paths) > 0 {
		b.WriteString(" Пути: ")
		for i, p := range v.Paths {
			if i > 0 {
				b.WriteString("; ")
			}
			fmt.Fprintf(&b, "%d→%d, длина %s км (%s дней)",
				p.FromPlaceID, p.ToPlaceID, atlasNum(p.LengthKm), atlasNum(p.Days))
		}
		b.WriteString(".")
	}
	fmt.Fprintf(&b, " Герой: долгота %s, широта %s.", atlasNum(v.Hero.X), atlasNum(v.Hero.Y))
	if len(v.Rings) > 0 {
		b.WriteString(" Кольца: ")
		for i, r := range v.Rings {
			if i > 0 {
				b.WriteString("; ")
			}
			fmt.Fprintf(&b, "%s — готово %s", r.Status, atlasNum(r.Progress))
		}
		b.WriteString(".")
	}
	return b.String()
}

// atlasNum — число для промпта без лишней точности и экспоненты.
func atlasNum(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
