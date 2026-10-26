package workshop_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// TestDeferred — отложенные события: таймер движка (T-05-10, «Проверка»):
// событие с due_day в прошлом срабатывает в отсутствие игрока и фиксируется в
// базе; слот не вызывается; подставные часы дают детерминированное время
// срабатывания.
func TestDeferred(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "world(a).db")

	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(w, "world", "shared"); err != nil {
		w.Close()
		t.Fatalf("Migrate: %v", err)
	}
	w.Close()

	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	defer h.Close()

	// Подставной слот — доказательство «без модели»: таймер его не вызывает.
	slot := testkit.NewFakeSlot()
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	clk := testkit.NewClock()
	timer := workshop.NewDeferredTimer(h, clk)

	// Остановки коридора сразу становятся отложенными событиями (шаг 3):
	// фиксируются только тип и статус.
	if err := timer.CorridorStop(1000); err != nil {
		t.Fatalf("CorridorStop(1000): %v", err)
	}
	if err := timer.CorridorStop(999999); err != nil {
		t.Fatalf("CorridorStop(999999): %v", err)
	}

	var kind, status, payload string
	if err := h.QueryRow(`SELECT kind, status, payload_json FROM deferred_events WHERE due_day = 1000`).
		Scan(&kind, &status, &payload); err != nil {
		t.Fatalf("corridor stop query: %v", err)
	}
	if kind != workshop.DeferredKindCorridorStop {
		t.Fatalf("kind = %q, want %q", kind, workshop.DeferredKindCorridorStop)
	}
	if status != string(workshop.DeferredPending) {
		t.Fatalf("статус при записи = %q, want %q", status, workshop.DeferredPending)
	}
	if payload != "{}" {
		t.Fatalf("payload при записи = %q, want %q", payload, "{}")
	}

	// До истечения дедлайна событие не срабатывает.
	clk.Advance(500 * time.Minute) // мировая минута 500 < 1000.
	if got, err := timer.FireDue(); err != nil || got != 0 {
		t.Fatalf("FireDue до дедлайна = (%d, %v), want (0, nil)", got, err)
	}

	// Дедлайн 1000 в прошлом: срабатывает в отсутствие игрока, детерминированно.
	clk.Advance(2 * 24 * time.Hour) // мировая минута 500+2880=3380 > 1000.
	fired, err := timer.FireDue()
	if err != nil {
		t.Fatalf("FireDue: %v", err)
	}
	if fired != 1 {
		t.Fatalf("сработало событий = %d, want 1", fired)
	}

	// Сработавшее событие зафиксировано статусом fired.
	var pastStatus string
	if err := h.QueryRow(`SELECT status FROM deferred_events WHERE due_day = 1000`).Scan(&pastStatus); err != nil {
		t.Fatalf("past status query: %v", err)
	}
	if pastStatus != string(workshop.DeferredFired) {
		t.Fatalf("статус сработавшего = %q, want %q", pastStatus, workshop.DeferredFired)
	}

	// Будущее событие не сработало.
	var futureStatus string
	if err := h.QueryRow(`SELECT status FROM deferred_events WHERE due_day = 999999`).Scan(&futureStatus); err != nil {
		t.Fatalf("future status query: %v", err)
	}
	if futureStatus != string(workshop.DeferredPending) {
		t.Fatalf("статус будущего = %q, want %q", futureStatus, workshop.DeferredPending)
	}

	// Время срабатывания — из подставных часов (мировая минута 3380).
	var raw string
	if err := h.QueryRow(`SELECT payload_json FROM events WHERE kind = 'deferred_fired'`).Scan(&raw); err != nil {
		t.Fatalf("deferred_fired query: %v", err)
	}
	var rec struct {
		DeferredID int64  `json:"deferred_id"`
		Kind       string `json:"kind"`
		FiredAt    int64  `json:"fired_at"`
	}
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		t.Fatalf("deferred_fired payload: %v", err)
	}
	if rec.FiredAt != 3380 {
		t.Fatalf("время срабатывания = %d, want 3380", rec.FiredAt)
	}
	if rec.Kind != workshop.DeferredKindCorridorStop {
		t.Fatalf("kind сработавшего = %q, want %q", rec.Kind, workshop.DeferredKindCorridorStop)
	}

	// Без модели: слот не вызывался.
	if slot.Calls() != 0 {
		t.Fatalf("вызовов слота = %d, want 0", slot.Calls())
	}

	// Повторный прогон не срабатывает уже fired событие.
	if got, err := timer.FireDue(); err != nil || got != 0 {
		t.Fatalf("повторный FireDue = (%d, %v), want (0, nil)", got, err)
	}
}
