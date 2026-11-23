package workshop

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand"
	"strconv"

	"tns65/internal/db"
)

// luckKey — ключ world_meta параметра globalLuck (W-18.1, UI-08).
const luckKey = "globalLuck"

// prngKey — ключ world_meta снапшота ГСЧ мира (T-19-05).
const prngKey = "prng"

// rollEventKind — вид события журнала мира: бросок удачи (T-19-05).
const rollEventKind = "luck_roll"

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
	h     *db.Handle
	seed  int64
	draws uint64
	rng   *rand.Rand
}

// NewLuck создаёт менеджер удачи мира. h — хэндл оперативной базы мира
// (world_meta пишет только forge по карте писателей T-01-07), slug — слаг мира,
// семя детерминированного ГПСЧ.
func NewLuck(h *db.Handle, slug string) *Luck {
	return newLuck(h, seedFromSlug(slug))
}

// newLuck — поток ГСЧ от готового семени (снапшот, чекпоинт мира).
func newLuck(h *db.Handle, seed int64) *Luck {
	return &Luck{h: h, seed: seed, rng: rand.New(rand.NewSource(seed))}
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
// Счётчик бросков входит в снапшот (T-19-05).
func (l *Luck) Draw() float64 {
	l.draws++
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

// Checkpoint — снапшот состояния ГСЧ мира (T-19-05): семя и число сделанных
// с этого семени бросков.
type Checkpoint struct {
	Seed  int64  `json:"seed"`
	Draws uint64 `json:"draws"`
}

// Snapshot — текущее состояние ГСЧ мира.
func (l *Luck) Snapshot() Checkpoint {
	return Checkpoint{Seed: l.seed, Draws: l.draws}
}

// Restore — восстановление потока ГСЧ из снапшота: повторный посев и
// отбрасывание уже сделанных бросков дают точное продолжение последовательности.
func (l *Luck) Restore(c Checkpoint) {
	l.seed = c.Seed
	l.draws = c.Draws
	l.rng = rand.New(rand.NewSource(c.Seed))
	for i := uint64(0); i < c.Draws; i++ {
		l.rng.Float64()
	}
}

// SaveCheckpoint — снапшот ГСЧ в чекпоинт мира (world_meta рядом с журналом
// событий).
func (l *Luck) SaveCheckpoint() error {
	if l.h == nil {
		return errors.New("workshop: нет хэндла для записи снапшота ГСЧ")
	}
	raw, err := json.Marshal(l.Snapshot())
	if err != nil {
		return fmt.Errorf("workshop: сериализация снапшота ГСЧ: %w", err)
	}
	_, err = l.h.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES (?, ?)
		 ON CONFLICT("key") DO UPDATE SET value = excluded.value`,
		prngKey, string(raw))
	return err
}

// LoadCheckpoint — снапшот ГСЧ из чекпоинта мира; записи нет — false.
func (l *Luck) LoadCheckpoint() (Checkpoint, bool, error) {
	if l.h == nil {
		return Checkpoint{}, false, nil
	}
	var raw string
	err := l.h.QueryRow(`SELECT value FROM world_meta WHERE "key" = ?`, prngKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Checkpoint{}, false, nil
	}
	if err != nil {
		return Checkpoint{}, false, err
	}
	var c Checkpoint
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return Checkpoint{}, false, fmt.Errorf("workshop: разбор снапшота ГСЧ: %w", err)
	}
	return c, true, nil
}

// RollEvent — запись броска удачи в журнале событий мира (T-19-05).
type RollEvent struct {
	Day   int     `json:"day"`
	P     float64 `json:"p"`
	R     float64 `json:"r"`
	Fired bool    `json:"fired"`
}

// RecordRoll — запись броска в журнал событий (kind='luck_roll', писатель core):
// материал реплея от снапшота.
func RecordRoll(h *db.Handle, day int, p float64, r Roll) error {
	if h == nil {
		return errors.New("workshop: нет хэндла журнала событий")
	}
	raw, err := json.Marshal(RollEvent{Day: day, P: p, R: r.R, Fired: r.Fired})
	if err != nil {
		return fmt.Errorf("workshop: сериализация броска: %w", err)
	}
	_, err = h.Exec("events",
		`INSERT INTO events (day, kind, payload_json, source) VALUES (?, ?, ?, ?)`,
		day, rollEventKind, string(raw), string(db.WriterCore))
	return err
}

// LoadRollEvents — броски удачи из журнала событий в порядке записи.
func LoadRollEvents(h *db.Handle) ([]RollEvent, error) {
	rows, err := h.Query(`SELECT payload_json FROM events WHERE kind = ? ORDER BY id`, rollEventKind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []RollEvent
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var e RollEvent
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			return nil, fmt.Errorf("workshop: разбор события броска: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// ReplayRolls — реплей журнальных бросков от снапшота (T-19-05): ГСЧ
// восстанавливается из снапшота и повторяет броски; возвращает число совпавших R.
func ReplayRolls(c Checkpoint, events []RollEvent) int {
	l := newLuck(nil, c.Seed)
	l.Restore(c)
	matched := 0
	for _, e := range events {
		if l.Draw() == e.R {
			matched++
		}
	}
	return matched
}

// seedFromSlug — детерминированное семя ГСЧ из слага мира (как B-05.3).
func seedFromSlug(slug string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(slug))
	return int64(h.Sum64())
}
