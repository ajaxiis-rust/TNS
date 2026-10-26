package workshop

import (
	"context"
	"database/sql"
	"sync"

	"tns65/internal/db"
	"tns65/internal/journal"
)

// pauseStepEvent — событие журнала станка о смене ступени паузы (T-05-01,
// «Писатель»: переходы ступеней — в журнал станка).
const pauseStepEvent = "workshop.pause_step"

// Order — заказ очереди, выбранный планировщиком (T-05-01, шаг 5).
type Order struct {
	ID         int64
	Kind       Kind
	Step       Step
	Priority   int
	Resumable  bool
	Payload    string
	Checkpoint string
	CreatedAt  string
}

// RequeueAllowed — гейт постановки незавершённого шага (шаг 6): работа с
// resumable=0 в очередь не ставится.
func (o *Order) RequeueAllowed() bool { return o.Resumable }

// Scheduler — планировщик мастерской (шаг 5): один воркер разбирает
// workshop_queue по приоритету и (created_at, id), допускает только работы
// текущей ступени и исполняет их по одной (шаг 7). Ступень держится в памяти,
// её переходы пишутся в журнал станка; базу очереди эта задача не пишет
// (Писатель: нет записи).
type Scheduler struct {
	db     *db.Handle
	timer  *PauseTimer
	slot   Slot
	logger *journal.Logger

	mu            sync.Mutex
	chapterClosed bool
	lastStep      PauseStep
}

// NewScheduler создаёт планировщик мира: h — хэндл оперативной базы (чтение),
// clock — источник времени паузы, slot — единственный слот мастерской (:5001),
// logger — журнал станка (nil допустим).
func NewScheduler(h *db.Handle, clock PauseClock, slot Slot, logger *journal.Logger) *Scheduler {
	return &Scheduler{
		db:     h,
		timer:  NewPauseTimer(clock),
		slot:   slot,
		logger: logger,
	}
}

// MarkActive фиксирует активность игрока (возврат с паузы).
func (s *Scheduler) MarkActive() {
	s.timer.MarkActive()
}

// SetChapterClosed задаёт состояние главы для ночного окна (шаг 2): при открытой
// главе молчание дольше 8 часов остаётся тяжёлой ступенью.
func (s *Scheduler) SetChapterClosed(closed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chapterClosed = closed
}

// Step — текущая ступень паузы; смена ступени пишется в журнал станка.
func (s *Scheduler) Step() PauseStep {
	s.mu.Lock()
	defer s.mu.Unlock()
	step := s.timer.Step(s.chapterClosed)
	if step != s.lastStep {
		s.lastStep = step
		if s.logger != nil {
			_ = s.logger.Event("workshop", pauseStepEvent,
				journal.Field{Key: "step", Value: string(step)})
		}
	}
	return step
}

// Next — следующий допустимый заказ очереди: высший приоритет, при равенстве —
// ранний (created_at, id); только работы, допустимые текущей ступенью.
// Возвращает nil, когда допустимых заказов нет.
func (s *Scheduler) Next() (*Order, error) {
	return s.next(nil)
}

// next — выбор следующего допустимого заказа; skip — уже исполненные в этом
// прогоне заказы, повторно не возвращаются.
func (s *Scheduler) next(skip map[int64]bool) (*Order, error) {
	if s.db == nil {
		return nil, nil
	}
	pause := s.Step()
	rows, err := s.db.Query(
		`SELECT id, kind, step, priority, payload_json, resumable, checkpoint_json, created_at
		 FROM workshop_queue
		 WHERE status = 'queued'
		 ORDER BY priority DESC, created_at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var o Order
		var kind, step string
		var resumable int
		var checkpoint sql.NullString
		if err := rows.Scan(&o.ID, &kind, &step, &o.Priority, &o.Payload, &resumable, &checkpoint, &o.CreatedAt); err != nil {
			return nil, err
		}
		o.Kind = Kind(kind)
		o.Step = Step(step)
		o.Resumable = resumable == 1
		if checkpoint.Valid {
			o.Checkpoint = checkpoint.String
		}
		if skip != nil && skip[o.ID] {
			continue
		}
		if CanRun(o.Kind, o.Step, pause) {
			return &o, nil
		}
	}
	return nil, rows.Err()
}

// Run — воркер мастерской (шаг 7): исполняет допустимые заказы по одному, без
// параллельных запросов к слоту. За один прогон заказ исполняется не более
// одного раза; статусы очереди в этой задаче не меняются.
func (s *Scheduler) Run(ctx context.Context) error {
	seen := make(map[int64]bool)
	for {
		o, err := s.next(seen)
		if err != nil {
			return err
		}
		if o == nil {
			return nil
		}
		seen[o.ID] = true
		if err := s.runOne(ctx, o); err != nil {
			return err
		}
	}
}

// runOne исполняет один заказ через слот мастерской, подчиняясь планировщику
// ядра (T-03-17): слот ковки берётся только с разрешения ядра.
func (s *Scheduler) runOne(ctx context.Context, o *Order) error {
	if s.slot == nil {
		return nil
	}
	release := scheduler().AcquireWorkshop()
	defer release()
	_, err := s.slot.Generate(ctx, string(o.Kind), o.Payload)
	return err
}
