package birth

import (
	"fmt"
	"math"
	"strings"

	"tns65/internal/db"
)

// Режимы походов из data/ancketa.json (B-01.6, W-07.1): реальная, вымышленная,
// смешанная. Из жанров режим не выводится (T-02-01, шаг 2).
const (
	travelReal    = "реальная"
	travelFiction = "вымышленная"
	travelMixed   = "смешанная"
)

// heroCharacterID — идентификатор героя в character_state. Пачка NPC занимает
// ids 1..96; герой — игрок, а не карточка пачки, поэтому ему отведён id 0.
const heroCharacterID int64 = 0

// fictionPlaceName — имя стартового места вымышленного мира при пустом прологе
// (T-02-16, шаг 3).
const fictionPlaceName = "Стартовое место"

// Газетир соединяет города сушей, поэтому пути стартового региона — тракт.
// Полная карта типов (тракт/река/море) с трением — контракт T-06-01.
const (
	pathTypeTrakt     = "trakt"
	pathFrictionTrakt = 1.0
)

// defaultGeoRadiusDays — радиус стартового региона в днях пути (T-02-16, шаг 2).
const defaultGeoRadiusDays = 3.0

// defaultDayNormKm — норма дня пути в км (20–30 км пешком, T-06-02); переводит
// радиус в днях в километры. Дни в базу не пишутся.
const defaultDayNormKm = 25.0

// gazetteerPlace — строка газетира engine/seed/gazetteer.jsonl (T-01-16):
// x — долгота, y — широта.
type gazetteerPlace struct {
	Name     string  `json:"name"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	EpochRef string  `json:"epoch_ref"`
	Kind     string  `json:"kind"`
}

// geoPath — ребро стартового региона: пара мест (индексы в places) и длина в км.
type geoPath struct {
	from     int
	to       int
	lengthKm float64
}

// geoSeed — шаг цепочки рождения: стартовая география (T-02-16). По режиму
// походов пишет места и пути хэндлом forge и place_id героя в character_state
// хэндлом core (карта писателей T-01-07).
func (b *builder) geoSeed() error {
	places, paths, start, err := b.computeStartRegion()
	if err != nil {
		return err
	}
	if len(places) == 0 {
		return fmt.Errorf("birth: стартовый регион пуст")
	}

	// Места и пути — forge (B-03.5: ровно три вещи пишет Forge — места, пути,
	// метаданные мира; метаданные уже записаны шагом world_meta).
	h, err := db.OpenWriterAs(b.worldDB, db.DBWorld, db.WriterForge, b.logger)
	if err != nil {
		return err
	}
	defer h.Close()

	ids := make([]int64, len(places))
	if err := h.Tx(db.WriterForge, func(tx *db.Tx) error {
		for i, p := range places {
			res, err := tx.Exec("places",
				`INSERT INTO places (name, kind, x, y, epoch_ref) VALUES (?, ?, ?, ?, ?)`,
				p.Name, p.Kind, p.X, p.Y, p.EpochRef)
			if err != nil {
				return err
			}
			id, err := res.LastInsertId()
			if err != nil {
				return err
			}
			ids[i] = id
		}
		for _, p := range paths {
			if _, err := tx.Exec("paths",
				`INSERT INTO paths (from_place_id, to_place_id, type, length_km, friction) VALUES (?, ?, ?, ?, ?)`,
				ids[p.from], ids[p.to], pathTypeTrakt, p.lengthKm, pathFrictionTrakt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// Положение героя — core: place_id стартового места (T-02-16, шаг 5).
	ch, err := db.OpenWriterAs(b.worldDB, db.DBWorld, db.WriterCore, b.logger)
	if err != nil {
		return err
	}
	defer ch.Close()
	_, err = ch.Exec("character_state",
		`INSERT INTO character_state (character_id, place_id, x, y) VALUES (?, ?, ?, ?)`,
		heroCharacterID, ids[start], places[start].X, places[start].Y)
	return err
}

// computeStartRegion возвращает места, пути и индекс стартового места по режиму
// походов (T-02-16, шаги 1–4). Вымышленный и пустой/неизвестный режим дают одно
// место без путей — самодостаточную землю без внешних данных (B-14).
func (b *builder) computeStartRegion() ([]gazetteerPlace, []geoPath, int, error) {
	switch b.a.TravelMode {
	case travelReal, travelMixed:
		return b.startRegionFromGazetteer()
	default:
		return b.fictionStartRegion(), nil, 0, nil
	}
}

// startRegionFromGazetteer загружает стартовый регион реального/смешанного мира
// из газетира: места в радиусе geoRadiusDays дней пути от первого места газетира
// (стартовое), пути между соседями в пределах радиуса кодом (T-02-16, шаги 2 и 4).
func (b *builder) startRegionFromGazetteer() ([]gazetteerPlace, []geoPath, int, error) {
	all, err := readJSONLines[gazetteerPlace](b.gazetteerPath)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("birth: читать газетир %s: %w", b.gazetteerPath, err)
	}
	if len(all) == 0 {
		return nil, nil, 0, fmt.Errorf("birth: газетир %s пуст", b.gazetteerPath)
	}

	start := all[0]
	radiusKm := b.geoRadiusDays * b.dayNormKm
	places := []gazetteerPlace{start}
	for _, p := range all[1:] {
		if haversineKm(start, p) <= radiusKm {
			places = append(places, p)
		}
	}

	var paths []geoPath
	for i := 0; i < len(places); i++ {
		for j := i + 1; j < len(places); j++ {
			d := haversineKm(places[i], places[j])
			if d <= radiusKm {
				paths = append(paths, geoPath{from: i, to: j, lengthKm: d})
			}
		}
	}
	return places, paths, 0, nil
}

// fictionStartRegion — вымышленный мир: одно стартовое место героя, координаты
// назначены при рождении (вилка B-5), имя из пролога или «Стартовое место»;
// путей нет — землю дальше строит Паганель (T-02-16, шаг 3).
func (b *builder) fictionStartRegion() []gazetteerPlace {
	name := strings.TrimSpace(b.a.Prologue)
	if name == "" {
		name = fictionPlaceName
	}
	return []gazetteerPlace{{Name: name, Kind: "city", X: 0, Y: 0}}
}

// haversineKm — расстояние между двумя местами по координатам в километрах.
// Дни вычисляются на лету из этой длины и нормы дня, в базу не пишутся (W-06.2).
func haversineKm(a, b gazetteerPlace) float64 {
	const earthRadiusKm = 6371.0
	lat1 := a.Y * math.Pi / 180
	lat2 := b.Y * math.Pi / 180
	dLat := (b.Y - a.Y) * math.Pi / 180
	dLon := (b.X - a.X) * math.Pi / 180
	s := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(s))
}
