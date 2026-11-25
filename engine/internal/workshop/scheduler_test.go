package workshop_test

import (
	"context"
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
