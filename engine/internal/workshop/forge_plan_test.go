package workshop_test

import (
	"path/filepath"
	"testing"
	"time"

	"tns65/internal/db"
	"tns65/internal/workshop"
)

// TestForgePlanDayNight — план ковки по релевантности пути (T-21-12, W-02):
// днём окно одно — вечер, в ночном окне — утро и завтрашний день; дальняя
// заготовка допустима только при свободной очереди.
func TestForgePlanDayNight(t *testing.T) {
	day := workshop.PlanForge(workshop.PauseHeavy, 0, 0)
	if day.Night {
		t.Fatal("тяжёлая ступень днём помечена ночным окном")
	}
	if len(day.Targets) != 1 || day.Targets[0] != workshop.TargetEvening {
		t.Fatalf("дневные цели плана = %v, want [evening]", day.Targets)
	}
	if !day.Admit(workshop.TargetEvening) {
		t.Fatal("днём вечерняя заготовка не допущена")
	}
	if day.Admit(workshop.TargetMorning) || day.Admit(workshop.TargetTomorrow) {
		t.Fatal("днём допущена ночная заготовка")
	}

	night := workshop.PlanForge(workshop.PauseNight, workshop.DailyBlanksMin, 0)
	if !night.Night {
		t.Fatal("ночное окно не помечено ночью")
	}
	if got := night.Targets; len(got) != 2 || got[0] != workshop.TargetMorning || got[1] != workshop.TargetTomorrow {
		t.Fatalf("ночные цели плана = %v, want [morning tomorrow]", got)
	}
	if !night.Admit(workshop.TargetMorning) || !night.Admit(workshop.TargetTomorrow) {
		t.Fatal("ночью при свободной очереди заготовка не допущена")
	}
	if night.Admit(workshop.TargetEvening) {
		t.Fatal("ночью допущена вечерняя заготовка")
	}

	// Очередь занята — дальняя не куётся, ближняя ночная остаётся.
	busy := workshop.PlanForge(workshop.PauseNight, workshop.DailyBlanksMin, 1)
	if busy.Admit(workshop.TargetTomorrow) {
		t.Fatal("дальняя заготовка допущена при занятой очереди")
	}
	if !busy.Admit(workshop.TargetMorning) {
		t.Fatal("ближняя ночная заготовка отвергнута занятой очередью")
	}

	// Минимальный набор суток закрывается ближней целью раньше дальней.
	early := workshop.PlanForge(workshop.PauseNight, 0, 0)
	if early.Admit(workshop.TargetTomorrow) {
		t.Fatal("дальняя заготовка допущена до минимального набора суток")
	}
	if !early.Admit(workshop.TargetMorning) {
		t.Fatal("ближняя ночная заготовка отвергнута до минимального набора суток")
	}

	// Сверх суточной нормы не куётся ничего.
	full := workshop.PlanForge(workshop.PauseNight, workshop.DailyBlanksMax, 0)
	if full.Left != 0 {
		t.Fatalf("остаток нормы = %d, want 0", full.Left)
	}
	if full.Admit(workshop.TargetMorning) {
		t.Fatal("заготовка допущена сверх суточной нормы")
	}
}

// TestApplyForgePlan — воркер снимает заготовки вне окна ступени, сверх
// суточной нормы и дальние при занятой очереди (W-02.1, W-02.5).
func TestApplyForgePlan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "world(a).db")
	newDB(t, path, "world", "shared")

	world, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()

	noon := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	night := time.Date(2026, 3, 10, 23, 0, 0, 0, time.UTC)

	// День: остаётся вечер, ночная заготовка снимается.
	evening := insertForgeBlank(t, world, workshop.TargetEvening, "2026-03-10T11:00:00Z")
	morning := insertForgeBlank(t, world, workshop.TargetMorning, "2026-03-10T11:05:00Z")
	cancelled, err := workshop.ApplyForgePlan(world, workshop.PauseLight, noon)
	if err != nil {
		t.Fatalf("ApplyForgePlan(day): %v", err)
	}
	if cancelled != 1 {
		t.Fatalf("днём снято заготовок: %d, want 1", cancelled)
	}
	if got := orderStatus(t, world, evening); got != string(workshop.StatusQueued) {
		t.Fatalf("вечерняя заготовка днём = %q, want queued", got)
	}
	if got := orderStatus(t, world, morning); got != string(workshop.StatusCancelled) {
		t.Fatalf("ночная заготовка днём = %q, want cancelled", got)
	}

	// Ночь при занятой очереди: вечерние снимаются, дальняя ждёт свободной.
	tomorrowBusy := insertForgeBlank(t, world, workshop.TargetTomorrow, "2026-03-10T23:00:00Z")
	cancelled, err = workshop.ApplyForgePlan(world, workshop.PauseNight, night)
	if err != nil {
		t.Fatalf("ApplyForgePlan(night, busy): %v", err)
	}
	if cancelled != 2 {
		t.Fatalf("ночью при занятой очереди снято заготовок: %d, want 2", cancelled)
	}
	if got := orderStatus(t, world, evening); got != string(workshop.StatusCancelled) {
		t.Fatalf("вечерняя заготовка ночью = %q, want cancelled", got)
	}
	if got := orderStatus(t, world, tomorrowBusy); got != string(workshop.StatusCancelled) {
		t.Fatalf("дальняя заготовка при занятой очереди = %q, want cancelled", got)
	}

	// Ночь при свободной очереди: дальняя заготовка остаётся.
	tomorrowFree := insertForgeBlank(t, world, workshop.TargetTomorrow, "2026-03-10T23:10:00Z")
	cancelled, err = workshop.ApplyForgePlan(world, workshop.PauseNight, night)
	if err != nil {
		t.Fatalf("ApplyForgePlan(night, free): %v", err)
	}
	if cancelled != 0 {
		t.Fatalf("ночью при свободной очереди снято заготовок: %d, want 0", cancelled)
	}
	if got := orderStatus(t, world, tomorrowFree); got != string(workshop.StatusQueued) {
		t.Fatalf("дальняя заготовка при свободной очереди = %q, want queued", got)
	}

	// Норма суток исчерпана — снимается и ближняя заготовка.
	morningFull := insertForgeBlank(t, world, workshop.TargetMorning, "2026-03-10T23:20:00Z")
	cancelled, err = workshop.ApplyForgePlan(world, workshop.PauseNight, night)
	if err != nil {
		t.Fatalf("ApplyForgePlan(night, full): %v", err)
	}
	if cancelled != 2 {
		t.Fatalf("сверх нормы снято заготовок: %d, want 2", cancelled)
	}
	if got := orderStatus(t, world, morningFull); got != string(workshop.StatusCancelled) {
		t.Fatalf("ближняя заготовка сверх нормы = %q, want cancelled", got)
	}
}

// TestApplyForgePlanOrder — план ковки книги из заказа forge_plan сужает окно
// ступени: заготовка вне объявленных целей снимается, заказ без цели в payload
// планом не управляется (T-21-12).
func TestApplyForgePlanOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "world(a).db")
	newDB(t, path, "world", "shared")

	world, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()

	noon := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	evening := insertForgeBlank(t, world, workshop.TargetEvening, "2026-03-11T11:00:00Z")
	// Чужой заказ без цели (доукомплектация) планом не снимается.
	foreign := insertQueueOrder(t, world, workshop.KindComplement, workshop.StepLight, 30, "2026-03-11T11:05:00Z")

	cancelled, err := workshop.ApplyForgePlan(world, workshop.PauseLight, noon)
	if err != nil {
		t.Fatalf("ApplyForgePlan(без плана): %v", err)
	}
	if cancelled != 0 {
		t.Fatalf("без плана ковки снято заготовок: %d, want 0", cancelled)
	}
	if got := orderStatus(t, world, evening); got != string(workshop.StatusQueued) {
		t.Fatalf("вечерняя заготовка без плана = %q, want queued", got)
	}

	// План книги объявляет только утро — вечерняя заготовка вне окна.
	insertForgePlanOrder(t, world, "morning")
	cancelled, err = workshop.ApplyForgePlan(world, workshop.PauseLight, noon)
	if err != nil {
		t.Fatalf("ApplyForgePlan(план книги): %v", err)
	}
	if cancelled != 1 {
		t.Fatalf("по плану книги снято заготовок: %d, want 1", cancelled)
	}
	if got := orderStatus(t, world, evening); got != string(workshop.StatusCancelled) {
		t.Fatalf("вечерняя заготовка вне плана книги = %q, want cancelled", got)
	}
	if got := orderStatus(t, world, foreign); got != string(workshop.StatusQueued) {
		t.Fatalf("заказ без цели = %q, want queued", got)
	}
}

// insertForgeBlank ставит заказ-заготовку с целью ковки в payload.
func insertForgeBlank(t *testing.T, h *db.Handle, target workshop.ForgeTarget, created string) int64 {
	t.Helper()
	return insertForgeOrder(t, h, string(workshop.KindHoroscope), 60,
		`{"target":"`+string(target)+`","theme":"тема"}`, created)
}

// insertForgePlanOrder ставит заказ forge_plan с объявленными целями ковки.
func insertForgePlanOrder(t *testing.T, h *db.Handle, targets ...string) int64 {
	t.Helper()
	raw := `{"targets":[`
	for i, target := range targets {
		if i > 0 {
			raw += ","
		}
		raw += `{"target":"` + target + `","theme":"тема","peaks":["пик"]}`
	}
	raw += `]}`
	return insertForgeOrder(t, h, string(workshop.KindForgePlan), 50, raw, "2026-03-11T11:10:00Z")
}

// insertForgeOrder ставит ждущий заказ очереди с готовым payload.
func insertForgeOrder(t *testing.T, h *db.Handle, kind string, priority int, payload, created string) int64 {
	t.Helper()
	res, err := h.Exec("workshop_queue",
		`INSERT INTO workshop_queue (kind, payload_json, priority, status, step, resumable, attempts, created_at) VALUES (?, ?, ?, 'queued', 'light', 0, 0, ?)`,
		kind, payload, priority, created)
	if err != nil {
		t.Fatalf("INSERT workshop_queue: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("workshop_queue id: %v", err)
	}
	return id
}
