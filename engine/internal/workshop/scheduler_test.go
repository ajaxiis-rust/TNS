package workshop_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// TestSchedulerWorker — воркер мастерской на фейковых часах (T-20-04): заказ
// sediment доходит до исполнителя только на тяжёлой ступени, на лёгкой
// остаётся queued.
func TestSchedulerWorker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "world(a).db")
	newDB(t, path, "world", "shared")

	world, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()

	order := insertSedimentOrder(t, world, 1, 50, "2026-01-01T00:00:00Z")

	clk := testkit.NewClock()
	s := workshop.NewScheduler(world, clk, nil, nil)

	var runs int32
	s.SetSedimentRunner(func(_ context.Context, h *db.Handle, o *workshop.Order, _ *journal.Logger) (workshop.SedimentOutcome, error) {
		atomic.AddInt32(&runs, 1)
		_, err := h.Exec("workshop_queue",
			`UPDATE workshop_queue SET status = ? WHERE id = ?`, string(workshop.StatusDone), o.ID)
		return workshop.SedimentWrite, err
	})

	// Лёгкая ступень: заказ sediment не запускается, очередь не тронута.
	clk.Advance(10 * time.Minute)
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run(light): %v", err)
	}
	if got := atomic.LoadInt32(&runs); got != 0 {
		t.Fatalf("на лёгкой ступени исполнитель вызван %d раз, want 0", got)
	}
	if got := orderStatus(t, world, order); got != string(workshop.StatusQueued) {
		t.Fatalf("на лёгкой ступени статус заказа = %q, want queued", got)
	}

	// Тяжёлая ступень: исполнитель получил заказ, статус закрыт исходом.
	clk.Advance(4 * time.Hour)
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run(heavy): %v", err)
	}
	if got := atomic.LoadInt32(&runs); got != 1 {
		t.Fatalf("на тяжёлой ступени исполнитель вызван %d раз, want 1", got)
	}
	if got := orderStatus(t, world, order); got != string(workshop.StatusDone) {
		t.Fatalf("на тяжёлой ступени статус заказа = %q, want done", got)
	}

	// Повторный тик заказ не исполняет: исполненные в прогоне не возвращаются.
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run(повтор): %v", err)
	}
	if got := atomic.LoadInt32(&runs); got != 1 {
		t.Fatalf("повторный тик вызвал исполнителя %d раз, want 1", got)
	}
}

// TestSchedulerWorkerNoWorld — тик воркера без активного мира не падает:
// хэндла нет, Run возвращает пустую очередь на любой ступени.
func TestSchedulerWorkerNoWorld(t *testing.T) {
	clk := testkit.NewClock()
	s := workshop.NewScheduler(nil, clk, nil, nil)

	for _, d := range []time.Duration{0, 10 * time.Minute, 4 * time.Hour} {
		clk.Advance(d)
		if err := s.Run(context.Background()); err != nil {
			t.Fatalf("Run без активного мира (молчание %v): %v", d, err)
		}
	}

	// Закрытая глава открывает ночное окно — без мира тик так же пуст.
	s.SetChapterClosed(true)
	clk.Advance(5 * time.Hour)
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run без активного мира (ночь): %v", err)
	}
}

// probeSlot — слот, снимающий статус заказа в момент исполнения.
type probeSlot struct {
	h    *db.Handle
	id   int64
	out  string
	err  error
	seen []string
}

func (p *probeSlot) Generate(_ context.Context, _, _ string) (string, error) {
	var status string
	if err := p.h.QueryRow(`SELECT status FROM workshop_queue WHERE id = ?`, p.id).Scan(&status); err != nil {
		return "", err
	}
	p.seen = append(p.seen, status)
	return p.out, p.err
}

// insertQueueOrder ставит заказ вида kind со ступенью step в очередь мира.
func insertQueueOrder(t *testing.T, h *db.Handle, kind workshop.Kind, step workshop.Step, priority int, created string) int64 {
	t.Helper()
	res, err := h.Exec("workshop_queue",
		`INSERT INTO workshop_queue (kind, payload_json, priority, status, step, resumable, attempts, created_at) VALUES (?, '{}', ?, 'queued', ?, 0, 0, ?)`,
		string(kind), priority, string(step), created)
	if err != nil {
		t.Fatalf("INSERT workshop_queue: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("workshop_queue id: %v", err)
	}
	return id
}

// orderLifecycle читает статус, число попыток и чекпойнт заказа.
func orderLifecycle(t *testing.T, h *db.Handle, id int64) (string, int, string) {
	t.Helper()
	var status, checkpoint string
	var attempts int
	if err := h.QueryRow(
		`SELECT status, attempts, COALESCE(checkpoint_json, '') FROM workshop_queue WHERE id = ?`, id).
		Scan(&status, &attempts, &checkpoint); err != nil {
		t.Fatalf("SELECT заказа %d: %v", id, err)
	}
	return status, attempts, checkpoint
}

// TestSchedulerOrderLifecycle — заказ проходит running → done с чекпойнтом
// ответа (T-21-03): в момент исполнения заказ в running, повторный тик его не
// берёт.
func TestSchedulerOrderLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "world(a).db")
	newDB(t, path, "world", "shared")

	world, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()

	order := insertQueueOrder(t, world, workshop.KindForgePlan, workshop.StepLight, 10, "2026-01-01T00:00:00Z")

	slot := &probeSlot{h: world, id: order, out: "план ковки"}
	clk := testkit.NewClock()
	s := workshop.NewScheduler(world, clk, slot, nil)
	clk.Advance(10 * time.Minute)

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(slot.seen) != 1 || slot.seen[0] != string(workshop.StatusRunning) {
		t.Fatalf("заказ пришёл в слот со статусами %v, want [running]", slot.seen)
	}
	status, attempts, checkpoint := orderLifecycle(t, world, order)
	if status != string(workshop.StatusDone) {
		t.Fatalf("статус заказа = %q, want done", status)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if checkpoint != "план ковки" {
		t.Fatalf("checkpoint = %q, want ответ слота", checkpoint)
	}

	// Повторный тик done не исполняет.
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run(повтор): %v", err)
	}
	if len(slot.seen) != 1 {
		t.Fatalf("повторный тик вызвал слот %d раз, want 1", len(slot.seen))
	}
}

// TestSchedulerOrderFailure — сбой заказа закрывает его failed с попыткой и не
// останавливает прогон (T-21-03): второй заказ тоже исполняется, повторный тик
// failed не берёт.
func TestSchedulerOrderFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "world(a).db")
	newDB(t, path, "world", "shared")

	world, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()

	first := insertQueueOrder(t, world, workshop.KindForgePlan, workshop.StepLight, 20, "2026-01-01T00:00:00Z")
	second := insertQueueOrder(t, world, workshop.KindForgePlan, workshop.StepLight, 10, "2026-01-01T00:00:01Z")

	slot := testkit.NewFakeSlot()
	slot.FailWith(errors.New("слот отказал"))
	clk := testkit.NewClock()
	s := workshop.NewScheduler(world, clk, slot, nil)
	clk.Advance(10 * time.Minute)

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run на сбое заказа вернул ошибку: %v", err)
	}
	if got := slot.Calls(); got != 2 {
		t.Fatalf("слот вызван %d раз, want 2 (прогон не встал на первом сбое)", got)
	}
	for _, id := range []int64{first, second} {
		status, attempts, _ := orderLifecycle(t, world, id)
		if status != string(workshop.StatusFailed) {
			t.Fatalf("статус заказа %d = %q, want failed", id, status)
		}
		if attempts != 1 {
			t.Fatalf("attempts заказа %d = %d, want 1", id, attempts)
		}
	}

	// Повторный тик failed не исполняет.
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run(повтор): %v", err)
	}
	if got := slot.Calls(); got != 2 {
		t.Fatalf("повторный тик вызвал слот %d раз, want 2", got)
	}
}
