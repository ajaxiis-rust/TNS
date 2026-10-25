package turn

import (
	"encoding/json"

	"tns65/internal/db"
)

// Вектор судьбы дальнего круга (T-03-13, LT-12): в паузе мир продвигает NPC,
// которых живой ход не пересчитывает вблизи, кодом и без LLM. Коммиты ложатся в
// events с признаком offline (D-08.4); состояние NPC вне ближнего круга не
// фиксируется (LT-12.3).

// fateEventKind — имя события коммита вектора судьбы дальнего круга (D-08.4).
const fateEventKind = "fate_vector"

// fateSource — источник события вектора судьбы (писатель core, карта T-01-07).
const fateSource = "core"

// FateVector — структурная запись пути NPC (LT-12.4): домен жизни — положение,
// богатство, здоровье, связи; направление, сила, срок. Это траектория NPC,
// не проза и не эмбеддинг: с вектором смысла реплики не отождествляется (LT-12.6).
type FateVector struct {
	CharacterID int64   `json:"character_id"`
	Position    string  `json:"position"`
	Wealth      int64   `json:"wealth"`
	Health      int64   `json:"health"`
	Connections []int64 `json:"connections,omitempty"`
	Direction   string  `json:"direction"`
	Strength    float64 `json:"strength"`
	TermMin     int64   `json:"term_min"`
}

// fateCommit — коммит вектора судьбы дальнего круга: payload события events.
type fateCommit struct {
	Offline bool       `json:"offline"`
	Fate    FateVector `json:"fate"`
}

// Положения и направления домена жизни — код выбирает их детерминированно.
var fatePositions = []string{"в пути", "на месте", "в отлучке"}

var fateDirections = []string{"к орбите", "от орбиты", "по кругу"}

// orbitOrder — ранжирование NPC по близости к орбите игрока (LT-12.5): ближе тот,
// о ком в памяти мира (memory_topics) больше записей встреч; невстреченные дальше
// всех; при равенстве — меньший id. Детерминированно, без LLM.
func orbitOrder(h *db.Handle) ([]int64, error) {
	if h == nil {
		return nil, nil
	}
	rows, err := h.Query(`
		SELECT c.id
		FROM characters c
		LEFT JOIN memory_topics m ON m.character_id = c.id
		WHERE c.id != ?
		GROUP BY c.id
		ORDER BY COUNT(m.id) DESC, c.id`, heroCharacterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// advanceFates продвигает векторы судьбы дальнего круга за passes просветов
// (LT-12.5): верх ранга продвигается каждый просвет, остальные — ротацией. Каждый
// продвинутый вектор коммитится событием events с признаком offline.
func advanceFates(h *db.Handle, passes int, worldMin int64) (int, error) {
	if h == nil || passes <= 0 {
		return 0, nil
	}
	order, err := orbitOrder(h)
	if err != nil {
		return 0, err
	}
	if len(order) == 0 {
		return 0, nil
	}
	top := order[0]
	rest := order[1:]
	committed := 0
	for i := 0; i < passes; i++ {
		if err := commitFate(h, fateFor(top, worldMin+int64(i))); err != nil {
			return committed, err
		}
		committed++
		if len(rest) == 0 {
			continue
		}
		if err := commitFate(h, fateFor(rest[i%len(rest)], worldMin+int64(i))); err != nil {
			return committed, err
		}
		committed++
	}
	return committed, nil
}

// commitFate пишет вектор судьбы дальнего круга в events с признаком offline.
func commitFate(h *db.Handle, f FateVector) error {
	payload, err := json.Marshal(fateCommit{Offline: true, Fate: f})
	if err != nil {
		return err
	}
	_, err = h.Exec("events",
		`INSERT INTO events (day, kind, payload_json, source) VALUES (0, ?, ?, ?)`,
		fateEventKind, string(payload), fateSource)
	return err
}

// fateFor — детерминированный вектор судьбы NPC из id лица и мировой минуты:
// без LLM и без фиксации состояния вне ближнего круга (LT-12.3). Траектория
// меняется с мировой минутой, поэтому следующие просветы продвигают NPC.
func fateFor(id, worldMin int64) FateVector {
	h := uint64(id) ^ uint64(worldMin)*0x9E3779B97F4A7C15
	pos := fatePositions[int(h%uint64(len(fatePositions)))]
	dir := fateDirections[int((h>>8)%uint64(len(fateDirections)))]
	return FateVector{
		CharacterID: id,
		Position:    pos,
		Wealth:      int64(h % 10001),
		Health:      int64((h >> 4) % 101),
		Connections: []int64{id + 1 + int64(h%3)},
		Direction:   dir,
		Strength:    float64((h>>16)%1001) / 1000,
		TermMin:     MinutesPerDay + int64(h%uint64(MinutesPerDay)),
	}
}
