package turn

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"tns65/internal/db"
)

// Сдвиг положения героя событием (T-03-15, LT-17, W-08): положение героя в живом
// ходе приблизительное — достаточно для срабатываний; событие (буря и т.п.)
// сдвигает его кодом. Новые координаты пишутся одной транзакцией db.Tx(core, …)
// с ответом хода; places со стороны хода не пишется (карта писателей: places —
// forge).

// PositionShift — сдвиг положения героя по событию хода: дельты координат и,
// если задан, новый place_id (0 — место не меняется).
type PositionShift struct {
	DX      float64
	DY      float64
	PlaceID int64
}

// HeroPosition — приблизительное положение героя в памяти хода.
type HeroPosition struct {
	PlaceID int64
	X       float64
	Y       float64
	Shifted bool // координаты сдвинуты событием и пишутся коммитом с ответом.
}

// PlaceCoord — координаты известного места для триггера близости к краю.
type PlaceCoord struct {
	X float64
	Y float64
}

// Триггер близости к краю (T-06-06, шаг 2): вокруг героя 8 секторов по 45°.
const (
	edgeSectors   = 8
	edgeSectorDeg = 360.0 / edgeSectors
)

// positionStage — стадия сдвига положения (T-03-15, «Правится» stages.go): читает
// текущее положение героя и применяет сдвиг события хода. Сама в базу не пишет —
// координаты фиксирует коммит одной транзакцией с ответом.
type positionStage struct {
	db *db.Handle
}

func (*positionStage) Name() string { return "position" }

func (s *positionStage) Run(_ context.Context, st *TurnState) error {
	if st.PositionShift == nil {
		return nil
	}
	cur, err := readHeroPosition(s.db)
	if err != nil {
		return err
	}
	st.Position = applyPositionShift(cur, *st.PositionShift)
	return nil
}

// applyPositionShift возвращает новое положение героя после сдвига события.
func applyPositionShift(cur HeroPosition, shift PositionShift) HeroPosition {
	out := HeroPosition{
		PlaceID: cur.PlaceID,
		X:       cur.X + shift.DX,
		Y:       cur.Y + shift.DY,
		Shifted: true,
	}
	if shift.PlaceID != 0 {
		out.PlaceID = shift.PlaceID
	}
	return out
}

// readHeroPosition читает положение героя из character_state — единственный
// источник истины (T-06-04). Отсутствие строки даёт нулевое положение.
func readHeroPosition(h *db.Handle) (HeroPosition, error) {
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
		return HeroPosition{}, err
	}
	return p, nil
}

// writeHeroPosition обновляет состояние героя одной строкой character_state:
// место из рамки сцены, настроение реплики и — при сдвиге события — новые
// x/y/place_id. Строку героя заводит, если её ещё нет.
func writeHeroPosition(tx *db.Tx, st *TurnState) error {
	if st.Position.Shifted {
		_, err := tx.Exec("character_state",
			`INSERT INTO character_state (character_id, place_id, x, y, mood) VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(character_id) DO UPDATE SET place_id = excluded.place_id,
				x = excluded.x, y = excluded.y, mood = excluded.mood`,
			heroCharacterID, st.Position.PlaceID, st.Position.X, st.Position.Y, st.Prepath.Mood)
		return err
	}
	_, err := tx.Exec("character_state",
		`INSERT INTO character_state (character_id, place_id, mood) VALUES (?, ?, ?)
		 ON CONFLICT(character_id) DO UPDATE SET place_id = excluded.place_id, mood = excluded.mood`,
		heroCharacterID, st.Prepath.Frame.HeroPlaceID, st.Prepath.Mood)
	return err
}

// commitTurnPosition — коммит хода с положением героя (T-03-15): та же единая
// транзакция, что и commitTurn (T-03-05), но состояние героя пишет
// writeHeroPosition — при сдвиге события новые x/y ложатся в character_state
// вместе с ответом хода.
func commitTurnPosition(st *TurnState, h *db.Handle, failTable string) error {
	if h == nil || !shouldCommit(st) {
		return nil
	}
	return h.Tx(db.WriterCore, func(tx *db.Tx) error {
		turnID, err := writeTurn(tx, st)
		if err != nil {
			return err
		}
		if failTable == "turns" {
			return fmt.Errorf("turn: инъекция отказа на таблице turns")
		}

		sceneID, err := writeScene(tx, st, turnID)
		if err != nil {
			return err
		}
		if failTable == "scenes" {
			return fmt.Errorf("turn: инъекция отказа на таблице scenes")
		}

		if err := writeTurnEvent(tx, turnID); err != nil {
			return err
		}
		if failTable == "events" {
			return fmt.Errorf("turn: инъекция отказа на таблице events")
		}

		if err := writeVectors(tx, st, turnID); err != nil {
			return err
		}
		if failTable == "vectors" {
			return fmt.Errorf("turn: инъекция отказа на таблице vectors")
		}

		if err := writeBindings(tx, st, sceneID); err != nil {
			return err
		}
		if failTable == "token_bindings" {
			return fmt.Errorf("turn: инъекция отказа на таблице token_bindings")
		}

		if err := writeHeroPosition(tx, st); err != nil {
			return err
		}
		if failTable == "character_state" {
			return fmt.Errorf("turn: инъекция отказа на таблице character_state")
		}
		return nil
	})
}

// nearEdge сообщает, близко ли положение героя к краю известной земли: вокруг
// героя 8 секторов по 45°; сектор без известного места в пределах thresholdKm —
// край в этом направлении. Читает те же координаты character_state, что и сдвиг
// (T-06-06, шаг 2); точность не требуется (W-08.4).
func nearEdge(h *db.Handle, thresholdKm float64) (bool, error) {
	hero, err := readHeroPosition(h)
	if err != nil {
		return false, err
	}
	places, err := readPlaceCoords(h)
	if err != nil {
		return false, err
	}
	return edgeNear(hero.X, hero.Y, places, thresholdKm), nil
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

// readPlaceCoords читает координаты известных мест из places (только чтение).
func readPlaceCoords(h *db.Handle) ([]PlaceCoord, error) {
	if h == nil {
		return nil, nil
	}
	rows, err := h.Query(`SELECT x, y FROM places`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlaceCoord
	for rows.Next() {
		var p PlaceCoord
		if err := rows.Scan(&p.X, &p.Y); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
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
