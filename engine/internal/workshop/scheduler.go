package workshop

import (
	"context"
	"database/sql"
	"strconv"
	"sync"

	"tns65/internal/db"
	"tns65/internal/journal"
)

// pauseStepEvent — событие журнала станка о смене ступени паузы (T-05-01,
// «Писатель»: переходы ступеней — в журнал станка).
const pauseStepEvent = "workshop.pause_step"

// orderFailedEvent — событие журнала станка о сбое заказа очереди (T-21-03):
// сбой одного заказа не останавливает прогон воркера.
const orderFailedEvent = "workshop.order_failed"

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
// её переходы пишутся в журнал станка. Жизненный цикл заказа (running →
// done/failed) пишет ядро (T-21-03): исполненный заказ в очередь не возвращается.
type Scheduler struct {
	db             *db.Handle
	timer          *PauseTimer
	slot           Slot
	logger         *journal.Logger
	sedimentRunner SedimentRunFunc

	mu            sync.Mutex
	chapterClosed bool
	lastStep      PauseStep
	runners       map[Kind]OrderRunner
	deps          RunnerDeps
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

// SetSedimentRunner подменяет исполнение заказов sediment (T-05-16); nil —
// заказ sediment проходит через слот, как прочие заказы.
func (s *Scheduler) SetSedimentRunner(run SedimentRunFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sedimentRunner = run
}

// SetOrderRunners задаёт реестр исполнителей по виду заказа и их зависимости
// (T-21-04): заказы видов реестра исполняет свой runner, остальные — слот.
func (s *Scheduler) SetOrderRunners(runners map[Kind]OrderRunner, deps RunnerDeps) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runners = runners
	s.deps = deps
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
		if !CanRun(o.Kind, o.Step, pause) {
			continue
		}
		// Оседание — тяжёлая работа (T-05-16): разбирается только на тяжёлой
		// ступени и в ночном окне.
		if o.Kind == KindSediment && pause != PauseHeavy && pause != PauseNight {
			continue
		}
		return &o, nil
	}
	return nil, rows.Err()
}

// Run — воркер мастерской (шаг 7): исполняет допустимые заказы по одному, без
// параллельных запросов к слоту. Заказ закрывается исходом (T-21-03): перед
// исполнением running, после — done с чекпойнтом ответа или failed. Сбой одного
// заказа не валит прогон: он пишется в журнал станка, воркер идёт дальше.
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
			s.logOrderFailed(o, err)
		}
	}
}

// runOne исполняет один заказ через исполнителя вида (T-21-04) или слот
// мастерской, подчиняясь планировщику ядра (T-03-17): слот ковки берётся только
// с разрешения ядра. Заказ sediment исполняется конвейером оседания (T-05-16),
// если подключён его исполнитель; без исполнителя заказ проходит через слот, как
// прочие заказы. Статус заказа ведёт ядро: running перед исполнением, done/failed
// после.
func (s *Scheduler) runOne(ctx context.Context, o *Order) error {
	run := s.executor(o)
	if run == nil {
		return nil
	}
	if err := s.markRunning(o); err != nil {
		return err
	}
	release := scheduler().AcquireWorkshop()
	defer release()
	out, err := run(ctx)
	if ferr := s.markFinished(o, out, err); ferr != nil {
		return ferr
	}
	return err
}

// executor — исполнение заказа: конвейер оседания (T-05-16), исполнитель вида из
// реестра (T-21-04) или слот мастерской; nil — исполнять нечем.
func (s *Scheduler) executor(o *Order) func(context.Context) (string, error) {
	if o.Kind == KindSediment {
		s.mu.Lock()
		run := s.sedimentRunner
		s.mu.Unlock()
		if run != nil {
			return func(ctx context.Context) (string, error) {
				_, err := run(ctx, s.db, o, s.logger)
				return "", err
			}
		}
	}
	s.mu.Lock()
	run := s.runners[o.Kind]
	deps := s.deps
	s.mu.Unlock()
	if run != nil {
		if deps.World == nil {
			deps.World = s.db
		}
		if deps.Slot == nil {
			deps.Slot = s.slot
		}
		return func(ctx context.Context) (string, error) {
			return run(ctx, deps, o)
		}
	}
	slot := s.slot
	if slot == nil {
		return nil
	}
	return func(ctx context.Context) (string, error) {
		return slot.Generate(ctx, string(o.Kind), o.Payload)
	}
}

// markRunning открывает исполнение заказа: running, время старта и счётчик
// попыток (ядро — единственный писатель workshop_queue).
func (s *Scheduler) markRunning(o *Order) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec("workshop_queue",
		`UPDATE workshop_queue SET status = ?, started_at = ?, attempts = attempts + 1 WHERE id = ?`,
		string(StatusRunning), nowUTC(), o.ID)
	return err
}

// markFinished закрывает заказ исходом: успех — done с ответом в
// checkpoint_json, ошибка — failed. Исполненный заказ в очередь не вернётся.
func (s *Scheduler) markFinished(o *Order, checkpoint string, runErr error) error {
	if s.db == nil {
		return nil
	}
	if runErr != nil {
		_, err := s.db.Exec("workshop_queue",
			`UPDATE workshop_queue SET status = ?, finished_at = ? WHERE id = ?`,
			string(StatusFailed), nowUTC(), o.ID)
		return err
	}
	if checkpoint == "" {
		_, err := s.db.Exec("workshop_queue",
			`UPDATE workshop_queue SET status = ?, finished_at = ? WHERE id = ?`,
			string(StatusDone), nowUTC(), o.ID)
		return err
	}
	_, err := s.db.Exec("workshop_queue",
		`UPDATE workshop_queue SET status = ?, finished_at = ?, checkpoint_json = ? WHERE id = ?`,
		string(StatusDone), nowUTC(), checkpoint, o.ID)
	return err
}

// logOrderFailed пишет сбой заказа в журнал станка; прогон продолжается.
func (s *Scheduler) logOrderFailed(o *Order, err error) {
	if s.logger == nil {
		return
	}
	_ = s.logger.Event("workshop", orderFailedEvent,
		journal.Field{Key: "order_id", Value: strconv.FormatInt(o.ID, 10)},
		journal.Field{Key: "kind", Value: string(o.Kind)},
		journal.Field{Key: "error", Value: err.Error()})
}
