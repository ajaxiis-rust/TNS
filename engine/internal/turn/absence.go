package turn

import (
	"encoding/json"
	"time"

	"tns65/internal/db"
)

// Пауза без заморозки (T-03-13, LT-11): в отсутствие игрока мир живёт той же
// скоростью, что и при игре — заморозка запрещена. Часы мира — из T-03-12; обсчёт
// NPC — кодом, без LLM: векторы судьбы дальнего круга (fate.go) и срабатывание
// отложенных событий.

// fatePassMin — период просвета дальнего круга: раз в сутки мира продвигается
// верх ранга и по очереди остальные (LT-12.5).
const fatePassMin = MinutesPerDay

// Статусы отложенного события (полная механика таймера — T-05-10; здесь —
// минимальное срабатывание в паузе).
const (
	deferredPending = "pending"
	deferredFired   = "fired"
)

// deferredFireKind — событие сработавшего отложенного события в events.
const deferredFireKind = "deferred_fired"

// AbsenceReport — итог прогона паузы: сколько векторов судьбы продвинуто и сколько
// отложенных событий сработало.
type AbsenceReport struct {
	FatesAdvanced int
	DeferredFired int
}

// advanceAbsence прогоняет паузу с lastActive до текущего момента по часам clock.
// Мир продвигается той же скоростью, что и при игре; LLM не вызывается.
func advanceAbsence(h *db.Handle, clock ClockSource, lastActive time.Time) (AbsenceReport, error) {
	var r AbsenceReport
	now := time.Now()
	if clock != nil {
		now = clock.Now()
	}
	elapsed := now.Sub(lastActive)
	if elapsed <= 0 {
		return r, nil
	}

	// Мировая минута — целые минуты с начала часов (testkit.Clock в тестах стоит
	// на начале эпохи, поэтому Unix-минуты совпадают с прошедшими).
	worldMin := now.Unix() / 60
	passes := int(elapsed / (time.Duration(fatePassMin) * time.Minute))
	if passes < 1 {
		passes = 1 // короткое отсутствие — один просвет.
	}

	n, err := advanceFates(h, passes, worldMin)
	if err != nil {
		return r, err
	}
	r.FatesAdvanced = n

	fired, err := fireDeferred(h, worldMin)
	if err != nil {
		return r, err
	}
	r.DeferredFired = fired
	return r, nil
}

// fireDeferred — срабатывание отложенных событий (LT-12.2): события с дедлайном
// в прошлом фиксируются статусом fired и пишутся в events. Код, без LLM.
// due_day хранит целую мировую минуту дедлайна (единицы часов мира T-03-12).
func fireDeferred(h *db.Handle, worldMin int64) (int, error) {
	if h == nil {
		return 0, nil
	}
	rows, err := h.Query(
		`SELECT id, kind FROM deferred_events WHERE status = ? AND due_day <= ?`,
		deferredPending, worldMin)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type dueEvent struct {
		id   int64
		kind string
	}
	var due []dueEvent
	for rows.Next() {
		var d dueEvent
		if err := rows.Scan(&d.id, &d.kind); err != nil {
			return 0, err
		}
		due = append(due, d)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	fired := 0
	for _, d := range due {
		if _, err := h.Exec("deferred_events",
			`UPDATE deferred_events SET status = ? WHERE id = ?`, deferredFired, d.id); err != nil {
			return fired, err
		}
		payload, err := json.Marshal(map[string]any{
			"deferred_id": d.id,
			"kind":        d.kind,
			"fired_at":    worldMin,
		})
		if err != nil {
			return fired, err
		}
		if _, err := h.Exec("events",
			`INSERT INTO events (day, kind, payload_json, source) VALUES (0, ?, ?, ?)`,
			deferredFireKind, string(payload), fateSource); err != nil {
			return fired, err
		}
		fired++
	}
	return fired, nil
}
