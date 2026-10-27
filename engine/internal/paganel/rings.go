package paganel

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"tns65/internal/db"
)

// Статусы колец (T-06-05, шаг 1): queued → running → done. Статусы меняет
// только ядро (писатель core, шаг 6).
const (
	RingQueued  = "queued"
	RingRunning = "running"
	RingDone    = "done"
)

// ErrForgeDisabled — ковка выключена переключателем paganel.enabled (T-06-05,
// шаг 3): кольцо в очередь не ставится.
var ErrForgeDisabled = errors.New("paganel: ковка колец выключена")

// Ring — строка таблицы rings: очередь колец, текущее кольцо, прогресс и
// счётчики ковки (T-06-05, шаг 2, W-09.1/W-09.3).
type Ring struct {
	ID          int64
	Status      string
	Progress    float64
	BudgetUsed  float64
	StartedAt   string
	FinishedAt  string
	PlacesCount int64
	PathsCount  int64
	PeopleCount int64
}

// RingCounters — счётчики ковки кольца: места и пути (включая неявные) и
// упомянутые люди (W-09.3).
type RingCounters struct {
	Places int64
	Paths  int64
	People int64
}

// EnqueueRing ставит кольцо в очередь rings статусом queued с progress=0
// (T-06-05, шаг 2). При выключенной ковке (cfg.Enabled=false) строка не
// создаётся — ErrForgeDisabled.
func EnqueueRing(h *db.Handle, cfg PaganelConfig) (Ring, error) {
	if !cfg.Enabled {
		return Ring{}, ErrForgeDisabled
	}
	if h == nil {
		return Ring{}, fmt.Errorf("paganel: хэндл мира не задан")
	}
	res, err := h.Exec("rings",
		`INSERT INTO rings (status, progress, budget_used, started_at, places_count, paths_count, people_count)
		 VALUES (?, 0, 0, '', 0, 0, 0)`,
		RingQueued)
	if err != nil {
		return Ring{}, fmt.Errorf("paganel: поставить кольцо: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Ring{}, fmt.Errorf("paganel: id кольца: %w", err)
	}
	return Ring{ID: id, Status: RingQueued, Progress: 0}, nil
}

// StartRing переводит кольцо queued → running и ставит started_at (T-06-05,
// шаг 2); кольцо строится между ходами, внутри хода не строится (шаг 4).
func StartRing(h *db.Handle, id int64) (Ring, error) {
	if h == nil {
		return Ring{}, fmt.Errorf("paganel: хэндл мира не задан")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := h.Exec("rings",
		`UPDATE rings SET status = ?, started_at = ? WHERE id = ? AND status = ?`,
		RingRunning, now, id, RingQueued)
	if err != nil {
		return Ring{}, fmt.Errorf("paganel: запустить кольцо: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Ring{}, fmt.Errorf("paganel: кольцо %d не в очереди (queued)", id)
	}
	return ReadRing(h, id)
}

// FinishRing переводит кольцо running → done, обновляет finished_at и progress
// (T-06-05, «Проверка»).
func FinishRing(h *db.Handle, id int64, progress float64) (Ring, error) {
	if h == nil {
		return Ring{}, fmt.Errorf("paganel: хэндл мира не задан")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := h.Exec("rings",
		`UPDATE rings SET status = ?, progress = ?, finished_at = ? WHERE id = ? AND status = ?`,
		RingDone, progress, now, id, RingRunning)
	if err != nil {
		return Ring{}, fmt.Errorf("paganel: завершить кольцо: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Ring{}, fmt.Errorf("paganel: кольцо %d не в работе (running)", id)
	}
	return ReadRing(h, id)
}

// UpdateRingCounters обновляет счётчики ковки кольца — места, пути (включая
// неявные) и упомянутые люди (T-06-05, шаг 5, W-09.3). Счётчики растут по мере
// ковки; статусы записей draft → baked продвигает ковка (T-06-08).
func UpdateRingCounters(h *db.Handle, id int64, c RingCounters) (Ring, error) {
	if h == nil {
		return Ring{}, fmt.Errorf("paganel: хэндл мира не задан")
	}
	if _, err := h.Exec("rings",
		`UPDATE rings SET places_count = ?, paths_count = ?, people_count = ? WHERE id = ?`,
		c.Places, c.Paths, c.People, id); err != nil {
		return Ring{}, fmt.Errorf("paganel: счётчики кольца: %w", err)
	}
	return ReadRing(h, id)
}

// ReadRing читает текущую строку кольца из rings.
func ReadRing(h *db.Handle, id int64) (Ring, error) {
	if h == nil {
		return Ring{}, fmt.Errorf("paganel: хэндл мира не задан")
	}
	r, err := scanRingRow(h.QueryRow(
		`SELECT id, status, progress, budget_used, started_at, finished_at,
		        places_count, paths_count, people_count
		 FROM rings WHERE id = ?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Ring{}, fmt.Errorf("paganel: кольцо %d не найдено", id)
		}
		return Ring{}, fmt.Errorf("paganel: читать кольцо: %w", err)
	}
	return r, nil
}

// CurrentRing — текущее кольцо в работе (running); работы нет — нулевой Ring.
func CurrentRing(h *db.Handle) (Ring, error) {
	if h == nil {
		return Ring{}, nil
	}
	r, err := scanRingRow(h.QueryRow(
		`SELECT id, status, progress, budget_used, started_at, finished_at,
		        places_count, paths_count, people_count
		 FROM rings WHERE status = ? ORDER BY id LIMIT 1`, RingRunning))
	if errors.Is(err, sql.ErrNoRows) {
		return Ring{}, nil
	}
	if err != nil {
		return Ring{}, fmt.Errorf("paganel: текущее кольцо: %w", err)
	}
	return r, nil
}

// scanRingRow разбирает одну строку rings.
func scanRingRow(row *sql.Row) (Ring, error) {
	var r Ring
	var finished sql.NullString
	err := row.Scan(&r.ID, &r.Status, &r.Progress, &r.BudgetUsed, &r.StartedAt, &finished,
		&r.PlacesCount, &r.PathsCount, &r.PeopleCount)
	if err != nil {
		return Ring{}, err
	}
	r.FinishedAt = finished.String
	return r, nil
}
