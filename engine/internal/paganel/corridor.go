package paganel

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"tns65/internal/db"
)

// BuildBudgetDefault — ориентир бюджета достройки (W-11.2): ~10 вызовов модели
// на кольцо. Совпадает со значением engine/config.yaml paganel.build_budget.
const BuildBudgetDefault = 10

// minutesPerDay — минут в сутках для перевода дней маршрута в мировые минуты
// дедлайна остановки (T-03-12: минута мира — целая минута часов).
const minutesPerDay = 1440

// ErrBuildBudgetExhausted — бюджет достройки исчерпан: вызов модели не сделан
// (T-06-09, шаг 3, W-11.1).
var ErrBuildBudgetExhausted = errors.New("paganel: бюджет достройки исчерпан")

// Статусы коридора: маршрут-обещание пишется queued, дальше ядро продвигает
// running → done (писатель core).
const (
	CorridorQueued  = "queued"
	CorridorRunning = "running"
	CorridorDone    = "done"
)

// corridorStopKind — вид отложенного события остановки коридора (W-17.3);
// совпадает с workshop.DeferredKindCorridorStop.
const corridorStopKind = "corridor_stop"

// corridorStopStatus — остановка записывается сразу отложенной (pending).
const corridorStopStatus = "pending"

// Waypoint — опорная точка коридора (T-06-09, шаг 1): имя и накопленный день
// пути от старта; разность дней соседних точек — отрезок с временем.
type Waypoint struct {
	Name string  `json:"name"`
	Day  float64 `json:"day"`
}

// Stop — остановка коридора: место и накопленный день пути. Каждая остановка
// сразу становится отложенным событием kind='corridor_stop' (W-17.3).
type Stop struct {
	Name string  `json:"name"`
	Day  float64 `json:"day"`
}

// CorridorPlan — маршрут-обещание коридора (шаги 1–2).
type CorridorPlan struct {
	RingID       int64
	FromPlaceID  int64
	ToPlaceID    int64
	Status       string
	Waypoints    []Waypoint
	Stops        []Stop
	PromisedDays float64
	StartMin     int64 // мировая минута старта маршрута (для due_day остановок).
}

// Corridor — записанный коридор.
type Corridor struct {
	ID           int64
	RingID       int64
	FromPlaceID  int64
	ToPlaceID    int64
	Status       string
	Waypoints    []Waypoint
	Stops        []Stop
	PromisedDays float64
}

// RecordCorridor пишет коридор в corridors и его остановки в deferred_events
// хэндлом core (T-06-09, шаги 1–2). Заявленный срок — внешняя рамка: сумма
// дней маршрута сверяется с promised_days (маршрут длиннее обещания — отказ),
// но promised_days сохраняется как заявлен и вычислением не подменяется
// (W-06.7, W-07.7).
func RecordCorridor(core *db.Handle, plan CorridorPlan) (Corridor, error) {
	if core == nil {
		return Corridor{}, errors.New("paganel: хэндл ядра не задан")
	}
	if err := validateCorridorPlan(plan); err != nil {
		return Corridor{}, err
	}

	waypointsJSON, err := json.Marshal(plan.Waypoints)
	if err != nil {
		return Corridor{}, fmt.Errorf("paganel: опорные точки коридора: %w", err)
	}
	stopsJSON, err := json.Marshal(plan.Stops)
	if err != nil {
		return Corridor{}, fmt.Errorf("paganel: остановки коридора: %w", err)
	}

	status := plan.Status
	if status == "" {
		status = CorridorQueued
	}

	res, err := core.Exec("corridors",
		`INSERT INTO corridors (ring_id, from_place_id, to_place_id, status, waypoints_json, stops_json, promised_days)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		plan.RingID, plan.FromPlaceID, plan.ToPlaceID, status,
		string(waypointsJSON), string(stopsJSON), plan.PromisedDays)
	if err != nil {
		return Corridor{}, fmt.Errorf("paganel: записать коридор: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Corridor{}, fmt.Errorf("paganel: id коридора: %w", err)
	}

	corridor := Corridor{
		ID:           id,
		RingID:       plan.RingID,
		FromPlaceID:  plan.FromPlaceID,
		ToPlaceID:    plan.ToPlaceID,
		Status:       status,
		Waypoints:    plan.Waypoints,
		Stops:        plan.Stops,
		PromisedDays: plan.PromisedDays,
	}

	createdAt := time.Now().UTC().Format(time.RFC3339)
	for _, stop := range plan.Stops {
		dueDay := plan.StartMin + int64(math.Round(stop.Day*minutesPerDay))
		if _, err := core.Exec("deferred_events",
			`INSERT INTO deferred_events (kind, payload_json, due_day, status, created_at) VALUES (?, ?, ?, ?, ?)`,
			corridorStopKind, `{}`, dueDay, corridorStopStatus, createdAt); err != nil {
			return corridor, fmt.Errorf("paganel: остановка коридора %q: %w", stop.Name, err)
		}
	}
	return corridor, nil
}

// validateCorridorPlan проверяет маршрут-обещание: концы, опорные точки,
// неотрицательные дни и схождение суммы дней с обещанием (шаги 1–2).
func validateCorridorPlan(plan CorridorPlan) error {
	if plan.FromPlaceID == 0 || plan.ToPlaceID == 0 {
		return errors.New("paganel: коридор без концов маршрута")
	}
	if len(plan.Waypoints) == 0 {
		return errors.New("paganel: коридор без опорных точек")
	}
	for _, w := range plan.Waypoints {
		if strings.TrimSpace(w.Name) == "" || w.Day < 0 {
			return fmt.Errorf("paganel: опорная точка %q: день %v вне допустимого", w.Name, w.Day)
		}
	}
	for _, s := range plan.Stops {
		if strings.TrimSpace(s.Name) == "" || s.Day < 0 {
			return fmt.Errorf("paganel: остановка %q: день %v вне допустимого", s.Name, s.Day)
		}
	}
	if plan.PromisedDays < 0 {
		return fmt.Errorf("paganel: заявленный срок %v вне допустимого", plan.PromisedDays)
	}
	if computed := corridorComputedDays(plan); computed > plan.PromisedDays {
		return fmt.Errorf("paganel: сумма дней маршрута %.2f больше обещания %.2f", computed, plan.PromisedDays)
	}
	return nil
}

// corridorComputedDays — вычисленная длительность маршрута в днях: конец
// маршрута — наибольший накопленный день среди опорных точек и остановок.
func corridorComputedDays(plan CorridorPlan) float64 {
	var max float64
	for _, w := range plan.Waypoints {
		if w.Day > max {
			max = w.Day
		}
	}
	for _, s := range plan.Stops {
		if s.Day > max {
			max = s.Day
		}
	}
	return max
}

// BudgetLimits — независимые лимиты вызовов модели (W-11.1/W-11.3): разведки
// (лимит сессии) и достройки (лимит на кольцо/коридор).
type BudgetLimits struct {
	Scout int
	Build int
}

// Budget — счётчики бюджета по миру в памяти процесса (шаг 4). Разведка и
// достройка считаются независимо: исчерпание одного лимита не трогает другой.
type Budget struct {
	mu         sync.Mutex
	scoutLimit int
	buildLimit int
	scoutUsed  int
	buildUsed  int
}

// NewBudget создаёт бюджет с независимыми лимитами разведки и достройки;
// отрицательный лимит трактуется как 0 (вызовы запрещены).
func NewBudget(limits BudgetLimits) *Budget {
	b := &Budget{scoutLimit: limits.Scout, buildLimit: limits.Build}
	if b.scoutLimit < 0 {
		b.scoutLimit = 0
	}
	if b.buildLimit < 0 {
		b.buildLimit = 0
	}
	return b
}

// ScoutSpend — заявка на вызов разведки: true — вызов разрешён и учтён; false —
// лимит сессии исчерпан, вызов не делается.
func (b *Budget) ScoutSpend() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.scoutUsed >= b.scoutLimit {
		return false
	}
	b.scoutUsed++
	return true
}

// BuildSpend — заявка на вызов достройки: true — вызов разрешён и учтён; false —
// лимит достройки исчерпан.
func (b *Budget) BuildSpend() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.buildUsed >= b.buildLimit {
		return false
	}
	b.buildUsed++
	return true
}

// ScoutSpent — израсходованные вызовы разведки.
func (b *Budget) ScoutSpent() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.scoutUsed
}

// BuildSpent — израсходованные вызовы достройки.
func (b *Budget) BuildSpent() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buildUsed
}

// BuildRing — шаг достройки кольца (шаг 3, W-11.1): вызов ковки ForgeRing, если
// бюджет достройки не исчерпан. Лимит сессии разведки не делит и не тратит.
func BuildRing(forge *db.Handle, b *Budget) (ForgeOutput, error) {
	if b == nil {
		b = NewBudget(BudgetLimits{Build: BuildBudgetDefault})
	}
	if !b.BuildSpend() {
		return ForgeOutput{}, ErrBuildBudgetExhausted
	}
	return ForgeRing(forge)
}

// ReadBuildBudget читает paganel.build_budget из файла config.yaml; ключа нет —
// дефолт BuildBudgetDefault (W-11.2). Отрицательное значение — отказ.
func ReadBuildBudget(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return parseBuildBudget(string(data))
}

// parseBuildBudget разбирает ключ build_budget внутри секции paganel.
func parseBuildBudget(data string) (int, error) {
	inPaganel := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inPaganel = trimmed == "paganel:"
			continue
		}
		if !inPaganel {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok || strings.TrimSpace(key) != "build_budget" {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		n, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("paganel: build_budget %q не число", value)
		}
		if n < 0 {
			return 0, fmt.Errorf("paganel: build_budget %d вне допустимого", n)
		}
		return n, nil
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	return BuildBudgetDefault, nil
}
