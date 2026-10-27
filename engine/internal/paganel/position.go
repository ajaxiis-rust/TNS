package paganel

import (
	"database/sql"
	"errors"
	"fmt"
	"math"

	"tns65/internal/db"
)

// Положение героя в живом ходе (T-06-04, W-08): единственный источник истины —
// character_state (place_id плюс приблизительные x, y); событие (буря и тому
// подобное) сдвигает положение кодом (T-03-15), точность не требуется (W-08.4).
// Контур положения пишет только character_state хэндлом core; places не
// трогается.

// heroCharacterID — идентификатор героя в character_state (0 — как в рождении,
// T-02-16; тот же, что в internal/turn).
const heroCharacterID = 0

// HeroPosition — приблизительное положение героя: место и координаты.
type HeroPosition struct {
	PlaceID int64
	X       float64
	Y       float64
}

// ReadHeroPosition читает положение героя из character_state — единственный
// источник истины (T-06-04, шаг 1). Строки героя нет — нулевое положение.
func ReadHeroPosition(h *db.Handle) (HeroPosition, error) {
	var p HeroPosition
	if h == nil {
		return p, nil
	}
	err := h.QueryRow(
		`SELECT COALESCE(place_id, 0), COALESCE(x, 0), COALESCE(y, 0)
		 FROM character_state WHERE character_id = ?`, heroCharacterID).
		Scan(&p.PlaceID, &p.X, &p.Y)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return HeroPosition{}, nil
		}
		return HeroPosition{}, fmt.Errorf("paganel: читать character_state: %w", err)
	}
	return p, nil
}

// SetHeroPlace — смена места героя в ходе (T-06-04, шаг 1): новый place_id и
// координаты места из places ложатся в character_state хэндлом core. places со
// стороны контура положения не пишется (W-08.2).
func SetHeroPlace(h *db.Handle, placeID int64) (HeroPosition, error) {
	x, y, err := readPlaceCoords(h, placeID)
	if err != nil {
		return HeroPosition{}, err
	}
	return writeHeroPosition(h, placeID, x, y)
}

// ShiftHeroPosition — движение вне мест (T-06-04, шаг 1): сдвиг приблизительных
// координат x/y без смены place_id; применяется кодом.
func ShiftHeroPosition(h *db.Handle, dx, dy float64) (HeroPosition, error) {
	cur, err := ReadHeroPosition(h)
	if err != nil {
		return HeroPosition{}, err
	}
	return writeHeroPosition(h, cur.PlaceID, cur.X+dx, cur.Y+dy)
}

// writeHeroPosition пишет положение героя одной строкой character_state хэндлом
// core (карта писателей); строку героя заводит, если её ещё нет.
func writeHeroPosition(h *db.Handle, placeID int64, x, y float64) (HeroPosition, error) {
	if h == nil {
		return HeroPosition{}, fmt.Errorf("paganel: хэндл мира не задан")
	}
	_, err := h.Exec("character_state",
		`INSERT INTO character_state (character_id, place_id, x, y) VALUES (?, ?, ?, ?)
		 ON CONFLICT(character_id) DO UPDATE SET place_id = excluded.place_id,
			x = excluded.x, y = excluded.y`,
		heroCharacterID, placeID, x, y)
	if err != nil {
		return HeroPosition{}, fmt.Errorf("paganel: записать character_state: %w", err)
	}
	return HeroPosition{PlaceID: placeID, X: x, Y: y}, nil
}

// readPlaceCoords читает координаты места из places (только чтение).
func readPlaceCoords(h *db.Handle, placeID int64) (float64, float64, error) {
	if h == nil {
		return 0, 0, fmt.Errorf("paganel: хэндл мира не задан")
	}
	var x, y float64
	err := h.QueryRow(`SELECT COALESCE(x, 0), COALESCE(y, 0) FROM places WHERE id = ?`, placeID).
		Scan(&x, &y)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, 0, fmt.Errorf("paganel: место %d не найдено в places", placeID)
		}
		return 0, 0, fmt.Errorf("paganel: читать places: %w", err)
	}
	return x, y, nil
}

// Сектора края карты (T-06-06, шаг 2): вокруг героя 8 секторов по 45°.
const (
	edgeSectors   = 8
	edgeSectorDeg = 360.0 / edgeSectors
)

// PlaceCoord — координаты известного места для триггера близости к краю.
type PlaceCoord struct {
	X float64
	Y float64
}

// NearEdge сообщает, близко ли положение героя к краю известной земли (T-06-06,
// шаг 2): сектор без известного места в пределах thresholdKm — край в этом
// направлении. Положение героя читает из character_state (T-06-04, шаг 2);
// точность не требуется (W-08.4).
func NearEdge(h *db.Handle, thresholdKm float64) (bool, error) {
	hero, err := ReadHeroPosition(h)
	if err != nil {
		return false, err
	}
	places, err := readKnownPlaceCoords(h)
	if err != nil {
		return false, err
	}
	return edgeNear(hero.X, hero.Y, places, thresholdKm), nil
}

// readKnownPlaceCoords читает координаты известных мест из places (только чтение).
func readKnownPlaceCoords(h *db.Handle) ([]PlaceCoord, error) {
	if h == nil {
		return nil, nil
	}
	rows, err := h.Query(`SELECT COALESCE(x, 0), COALESCE(y, 0) FROM places`)
	if err != nil {
		return nil, fmt.Errorf("paganel: читать places: %w", err)
	}
	defer rows.Close()
	var out []PlaceCoord
	for rows.Next() {
		var p PlaceCoord
		if err := rows.Scan(&p.X, &p.Y); err != nil {
			return nil, fmt.Errorf("paganel: разбор places: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// edgeNear — чистая проверка близости к краю по координатам героя и известным
// местам. Пустая земля — край в любом направлении.
func edgeNear(x, y float64, places []PlaceCoord, thresholdKm float64) bool {
	if len(places) == 0 {
		return true
	}
	var covered [edgeSectors]bool
	for _, p := range places {
		if haversineKm(x, y, p.X, p.Y) > thresholdKm {
			continue
		}
		covered[sectorIndex(x, y, p.X, p.Y)] = true
	}
	for _, c := range covered {
		if !c {
			return true
		}
	}
	return false
}

// sectorIndex — сектор (0..7) направления от героя (x,y) к месту (px,py);
// 0 — север, далее по часовой. Точность не требуется: долгота приводится к
// плоскости масштабом косинуса широты.
func sectorIndex(x, y, px, py float64) int {
	latRad := y * math.Pi / 180
	dx := (px - x) * math.Cos(latRad)
	dy := py - y
	bearing := math.Atan2(dx, dy) * 180 / math.Pi
	if bearing < 0 {
		bearing += 360
	}
	return int(math.Floor(bearing / edgeSectorDeg))
}

// haversineKm — расстояние между двумя точками по координатам в километрах.
// Дни в базу не пишутся (W-06.2).
func haversineKm(x1, y1, x2, y2 float64) float64 {
	const earthRadiusKm = 6371.0
	lat1 := y1 * math.Pi / 180
	lat2 := y2 * math.Pi / 180
	dLat := (y2 - y1) * math.Pi / 180
	dLon := (x2 - x1) * math.Pi / 180
	s := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(s))
}
