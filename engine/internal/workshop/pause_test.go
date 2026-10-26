package workshop_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

func TestPauseBoundaries(t *testing.T) {
	cases := []struct {
		name    string
		silence time.Duration
		closed  bool
		want    workshop.PauseStep
	}{
		{"9:59 нет работы", 9*time.Minute + 59*time.Second, true, workshop.PauseNone},
		{"10:00 light", 10 * time.Minute, true, workshop.PauseLight},
		{"60:00 light", time.Hour, true, workshop.PauseLight},
		{"60:01 средняя", time.Hour + time.Second, true, workshop.PauseMedium},
		{"3:00:00 средняя", 3 * time.Hour, true, workshop.PauseMedium},
		{"3:00:01 тяжёлая", 3*time.Hour + time.Second, true, workshop.PauseHeavy},
		{"8:00:00 тяжёлая", 8 * time.Hour, true, workshop.PauseHeavy},
		{"8:00:01 закрытая → ночь", 8*time.Hour + time.Second, true, workshop.PauseNight},
		{"8:00:01 открытая → тяжёлая", 8*time.Hour + time.Second, false, workshop.PauseHeavy},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := workshop.PauseStepFor(c.silence, c.closed); got != c.want {
				t.Fatalf("PauseStepFor(%v, closed=%v) = %q, want %q", c.silence, c.closed, got, c.want)
			}
		})
	}
}

func TestPauseSteps(t *testing.T) {
	clk := testkit.NewClock()
	tm := workshop.NewPauseTimer(clk)

	if got := tm.Step(false); got != workshop.PauseNone {
		t.Fatalf("0 мин = %q, want none", got)
	}

	clk.Advance(10 * time.Minute)
	if got := tm.Step(false); got != workshop.PauseLight {
		t.Fatalf("10 мин = %q, want light", got)
	}

	// На лёгкой ступени проза запрещена, подготовка без прозы — разрешена (шаг 4).
	if workshop.CanRun(workshop.KindHoroscope, workshop.StepLight, workshop.PauseLight) {
		t.Fatal("проза на light должна быть отклонена")
	}
	if !workshop.CanRun(workshop.KindForgePlan, workshop.StepLight, workshop.PauseLight) {
		t.Fatal("подготовка без прозы на light должна быть разрешена")
	}

	// Возврат игрока обнуляет молчание.
	tm.MarkActive()
	if got := tm.Step(false); got != workshop.PauseNone {
		t.Fatalf("после MarkActive = %q, want none", got)
	}
}

// recordSlot — слот, записывающий порядок ролей и максимум одновременных вызовов.
type recordSlot struct {
	mu        sync.Mutex
	roles     []string
	inFlight  int
	maxFlight int
}

func (s *recordSlot) Generate(_ context.Context, role, _ string) (string, error) {
	s.mu.Lock()
	s.inFlight++
	if s.inFlight > s.maxFlight {
		s.maxFlight = s.inFlight
	}
	s.roles = append(s.roles, role)
	s.mu.Unlock()

	// Короткая пауза расширяет окно пересечения для детектора одновременности;
	// к таймеру паузы (testkit.Clock) это не относится.
	time.Sleep(2 * time.Millisecond)

	s.mu.Lock()
	s.inFlight--
	s.mu.Unlock()
	return "ok", nil
}

func TestSchedulerOrder(t *testing.T) {
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

	insert := func(kind workshop.Kind, priority int, created string) {
		t.Helper()
		if _, err := h.Exec("workshop_queue",
			`INSERT INTO workshop_queue (kind, payload_json, priority, status, step, resumable, attempts, created_at) VALUES (?, '{}', ?, 'queued', 'heavy', 0, 0, ?)`,
			string(kind), priority, created); err != nil {
			t.Fatalf("insert %s: %v", kind, err)
		}
	}

	insert(workshop.KindStyle, 10, "2026-01-01T00:00:00Z")
	insert(workshop.KindSediment, 50, "2026-01-01T01:00:00Z")
	insert(workshop.KindComplement, 30, "2026-01-01T00:30:00Z")
	insert(workshop.KindHoroscope, 60, "2026-01-01T00:00:00Z")
	// Равный приоритет с sediment выше, но более ранний created_at.
	insert(workshop.KindSediment, 50, "2026-01-01T00:00:00Z")

	clk := testkit.NewClock()

	slot := &recordSlot{}
	s := workshop.NewScheduler(h, clk, slot, nil)
	s.SetChapterClosed(false)
	clk.Advance(4 * time.Hour) // тяжёлая ступень

	// Высший приоритет выбирается первым.
	o, err := s.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if o == nil || o.Kind != workshop.KindHoroscope {
		t.Fatalf("Next = %+v, want horoscope", o)
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{"horoscope", "sediment", "sediment", "complement", "style"}
	if len(slot.roles) != len(want) {
		t.Fatalf("исполнено заказов: %d (%v), want %d", len(slot.roles), slot.roles, len(want))
	}
	for i := range want {
		if slot.roles[i] != want[i] {
			t.Fatalf("порядок исполнения = %v, want %v", slot.roles, want)
		}
	}
	if slot.maxFlight != 1 {
		t.Fatalf("одновременных запросов к слоту: %d, want 1", slot.maxFlight)
	}
}
