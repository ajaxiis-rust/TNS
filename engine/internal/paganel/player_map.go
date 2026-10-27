package paganel

import (
	"errors"
	"fmt"
	"strconv"

	"tns65/internal/db"
)

// Карта мира игрока (T-06-12, W-05.1): Паганель отвечает за известность
// земли — места, пути, известность. player_map держит только известные игроку
// места (place_id, known_at, drawn); пишет ядро хэндлом core, игровой
// интерфейс карту только читает (прорисовка — в браузере, P7/W-20.3).

// PlayerMapPlace — известное игроку место карты мира.
type PlayerMapPlace struct {
	PlaceID int64
	KnownAt string // день, когда место стало известно (TEXT в схеме).
	Drawn   bool   // признак прорисованного места; прорисовка — в браузере.
}

// MarkPlaceKnown помечает место известным игроку (T-06-12, шаги 1–2): пишет в
// player_map place_id, known_at (день) и drawn хэндлом core. Повторная пометка
// обновляет known_at и drawn (посещение после ковки). Место за краем, ещё не
// выкованное, сюда не пишется — известность вносит ядро по факту ковки или
// фактического посещения.
func MarkPlaceKnown(core *db.Handle, placeID int64, day int, drawn bool) error {
	if core == nil {
		return errors.New("paganel: хэндл ядра не задан")
	}
	if placeID == 0 {
		return errors.New("paganel: place_id не задан")
	}
	drawnInt := 0
	if drawn {
		drawnInt = 1
	}
	_, err := core.Exec("player_map",
		`INSERT INTO player_map (place_id, known_at, drawn) VALUES (?, ?, ?)
		 ON CONFLICT(place_id) DO UPDATE SET known_at = excluded.known_at,
			drawn = excluded.drawn`,
		placeID, strconv.Itoa(day), drawnInt)
	if err != nil {
		return fmt.Errorf("paganel: записать player_map: %w", err)
	}
	return nil
}

// MarkHeroPlaceKnown помечает текущее место героя известным (T-06-12, шаг 1):
// положение героя читается из character_state (T-06-04), известность пишется в
// player_map днём визита.
func MarkHeroPlaceKnown(core *db.Handle, day int, drawn bool) (int64, error) {
	hero, err := ReadHeroPosition(core)
	if err != nil {
		return 0, err
	}
	if hero.PlaceID == 0 {
		return 0, errors.New("paganel: герой не у места")
	}
	if err := MarkPlaceKnown(core, hero.PlaceID, day, drawn); err != nil {
		return 0, err
	}
	return hero.PlaceID, nil
}

// ReadPlayerMap читает известные игроку места из player_map (только чтение).
func ReadPlayerMap(h *db.Handle) ([]PlayerMapPlace, error) {
	if h == nil {
		return nil, nil
	}
	rows, err := h.Query(`SELECT place_id, known_at, drawn FROM player_map ORDER BY place_id`)
	if err != nil {
		return nil, fmt.Errorf("paganel: читать player_map: %w", err)
	}
	defer rows.Close()

	var out []PlayerMapPlace
	for rows.Next() {
		var p PlayerMapPlace
		var drawn int
		if err := rows.Scan(&p.PlaceID, &p.KnownAt, &drawn); err != nil {
			return nil, fmt.Errorf("paganel: разбор player_map: %w", err)
		}
		p.Drawn = drawn != 0
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("paganel: читать player_map: %w", err)
	}
	return out, nil
}
