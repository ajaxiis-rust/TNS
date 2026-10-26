package workshop

import (
	"encoding/json"
	"time"

	"tns65/internal/db"
)

// DeferredStatus — статус отложенного события (T-05-10, шаг 1).
type DeferredStatus string

const (
	DeferredPending DeferredStatus = "pending"
	DeferredFired   DeferredStatus = "fired"
)

// DeferredKindCorridorStop — вид отложенного события остановки коридора
// (T-05-10, шаг 3): фиксируются только тип и статус; наполнение данными
// коридора — в P5 (T-06-09).
const DeferredKindCorridorStop = "corridor_stop"

// deferredFireKind — событие сработавшего отложенного события в events.
const deferredFireKind = "deferred_fired"

// deferredSource — источник события срабатывания (писатель core, карта T-01-07).
const deferredSource = "core"

// DeferredClock — источник времени таймера отложенных событий (T-05-10, шаг 4).
// testkit.Clock удовлетворяет контракту; в проде — системные часы.
type DeferredClock interface {
	Now() time.Time
}

// realDeferredClock — системные часы по умолчанию.
type realDeferredClock struct{}

func (realDeferredClock) Now() time.Time { return time.Now() }

// DeferredTimer — таймер движка: срабатывание отложенных событий по due_day
// независимо от присутствия игрока (T-05-10, шаг 2). Модель и человек событие
// не запускают — исполняет только код.
type DeferredTimer struct {
	h     *db.Handle
	clock DeferredClock
}

// NewDeferredTimer создаёт таймер мира: h — хэндл оперативной базы (писатель
// core), clock — источник времени; nil — системные часы.
func NewDeferredTimer(h *db.Handle, clock DeferredClock) *DeferredTimer {
	t := &DeferredTimer{h: h, clock: clock}
	if t.clock == nil {
		t.clock = realDeferredClock{}
	}
	return t
}

// CorridorStop записывает остановку коридора как отложенное событие (шаг 3):
// kind='corridor_stop', статус pending, пустой payload. dueDay — дедлайн
// срабатывания в мировых минутах.
func (t *DeferredTimer) CorridorStop(dueDay int64) error {
	if t.h == nil {
		return nil
	}
	_, err := t.h.Exec("deferred_events",
		`INSERT INTO deferred_events (kind, payload_json, due_day, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		DeferredKindCorridorStop, `{}`, dueDay, string(DeferredPending), t.createdAt())
	return err
}

// FireDue срабатывает отложенные события с дедлайном в прошлом (шаги 2, 4):
// due_day <= мировой минуты часов. Сработавшее событие фиксируется статусом
// fired и событием deferred_fired с временем срабатывания из часов. Возвращает
// число сработавших событий; модель не вызывается.
func (t *DeferredTimer) FireDue() (int, error) {
	if t.h == nil {
		return 0, nil
	}
	worldMin := t.clock.Now().Unix() / 60

	rows, err := t.h.Query(
		`SELECT id, kind FROM deferred_events WHERE status = ? AND due_day <= ?`,
		string(DeferredPending), worldMin)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type due struct {
		id   int64
		kind string
	}
	var dueEvents []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.kind); err != nil {
			return 0, err
		}
		dueEvents = append(dueEvents, d)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	fired := 0
	for _, d := range dueEvents {
		if _, err := t.h.Exec("deferred_events",
			`UPDATE deferred_events SET status = ? WHERE id = ?`,
			string(DeferredFired), d.id); err != nil {
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
		if _, err := t.h.Exec("events",
			`INSERT INTO events (day, kind, payload_json, source) VALUES (0, ?, ?, ?)`,
			deferredFireKind, string(payload), deferredSource); err != nil {
			return fired, err
		}
		fired++
	}
	return fired, nil
}

// createdAt — время записи отложенного события из часов таймера.
func (t *DeferredTimer) createdAt() string {
	return t.clock.Now().UTC().Format(time.RFC3339)
}
