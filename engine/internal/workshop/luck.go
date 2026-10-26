package workshop

import (
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand"
	"strconv"

	"tns65/internal/db"
)

// luckKey — ключ world_meta параметра globalLuck (W-18.1, UI-08).
const luckKey = "globalLuck"

// DefaultLuck — нейтральное значение globalLuck (T-05-11, шаг 1): 0.5.
const DefaultLuck = 0.5

// Weight — контракт веса удачи (T-05-11, шаг 2):
// Weight(p) = clamp(p·(0.5+L), 0, 1). LLM в расчёте не участвует (шаг 3).
func Weight(p, luck float64) float64 {
	w := p * (0.5 + luck)
	if w < 0 {
		return 0
	}
	if w > 1 {
		return 1
	}
	return w
}

// Luck — параметр globalLuck мира (T-05-11): чтение/запись world_meta и бросок
// детерминированным ГПСЧ, засеянным от слага (шаг 2).
type Luck struct {
	h   *db.Handle
	rng *rand.Rand
}

// NewLuck создаёт менеджер удачи мира. h — хэндл оперативной базы мира
// (world_meta пишет только forge по карте писателей T-01-07), slug — слаг мира,
// семя детерминированного ГПСЧ.
func NewLuck(h *db.Handle, slug string) *Luck {
	return &Luck{h: h, rng: rand.New(rand.NewSource(seedFromSlug(slug)))}
}

// Get читает L из world_meta; записи нет — DefaultLuck (шаг 1).
func (l *Luck) Get() (float64, error) {
	if l.h == nil {
		return DefaultLuck, nil
	}
	var raw string
	err := l.h.QueryRow(`SELECT value FROM world_meta WHERE "key" = ?`, luckKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultLuck, nil
	}
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("workshop: разбор globalLuck: %w", err)
	}
	return v, nil
}

// Set пишет L в world_meta (шаги 1, 4). Значение вне диапазона [0,1]
// отклоняется (шаг 5); смена применяется к будущим расчётам.
func (l *Luck) Set(v float64) error {
	if v < 0 || v > 1 {
		return fmt.Errorf("workshop: globalLuck вне диапазона [0,1]: %v", v)
	}
	if l.h == nil {
		return errors.New("workshop: нет хэндла для записи globalLuck")
	}
	_, err := l.h.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES (?, ?)
		 ON CONFLICT("key") DO UPDATE SET value = excluded.value`,
		luckKey, strconv.FormatFloat(v, 'g', -1, 64))
	return err
}

// Roll — исход одного броска удачи: R — значение броска из ГПСЧ, Fired — исход
// механики (r < Weight(p, L)).
type Roll struct {
	R     float64
	Fired bool
}

// Draw — детерминированный бросок ГПСЧ мира: значение в [0,1) от сида слага.
func (l *Luck) Draw() float64 {
	return l.rng.Float64()
}

// Roll — событийная механика с базовой вероятностью p (шаг 2): срабатывает при
// r < Weight(p, L). Исход вычисляется кодом один раз; смена L позже уже
// свершившийся исход не пересчитывает (шаг 4).
func (l *Luck) Roll(p float64) (Roll, error) {
	luck, err := l.Get()
	if err != nil {
		return Roll{}, err
	}
	r := l.Draw()
	return Roll{R: r, Fired: r < Weight(p, luck)}, nil
}

// seedFromSlug — детерминированное семя ГСЧ из слага мира (как B-05.3).
func seedFromSlug(slug string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(slug))
	return int64(h.Sum64())
}
