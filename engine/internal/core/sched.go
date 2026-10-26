package core

import (
	"runtime"
	"sync"
)

// WorkshopSlots — верхняя граница одновременных задач мастерской (T-03-17, шаг 1):
// фоновых задач куётся не более трёх сразу.
const WorkshopSlots = 3

// Scheduler — планировщик ядер станка: раздаёт слоты фоновой мастерской и держит
// приоритет живого хода. Ход всегда приоритетнее ковки (LT-01), и это свойство
// механизма, а не норма поведения (T-03-17, шаг 2).
type Scheduler struct {
	mu      sync.Mutex
	cond    *sync.Cond
	busy    int // задач мастерской, держащих слот
	turns   int // активных ходов
	waiting int // задач мастерской, ожидающих слот
}

// NewScheduler заводит планировщик на четыре ядра.
func NewScheduler() *Scheduler {
	runtime.GOMAXPROCS(4)
	s := &Scheduler{}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// DefaultScheduler — планировщик всего станка.
var DefaultScheduler = NewScheduler()

// AcquireWorkshop занимает слот мастерской. Пока идёт ход, слоты не выдаются
// (семафор сжат до нуля); когда все три слота заняты, вызов ждёт освобождения.
// Возвращённую release вызывают ровно один раз, чтобы вернуть слот.
func (s *Scheduler) AcquireWorkshop() (release func()) {
	s.mu.Lock()
	s.waiting++
	for s.turns > 0 || s.busy >= WorkshopSlots {
		s.cond.Wait()
	}
	s.waiting--
	s.busy++
	s.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			s.busy--
			s.cond.Broadcast()
			s.mu.Unlock()
		})
	}
}

// BeginTurn захватывает планировщик на время хода и возвращает стража, чей End
// освобождает его. Ход не ждёт освобождения слотов мастерской (шаг 2).
func (s *Scheduler) BeginTurn() *TurnGuard {
	s.mu.Lock()
	s.turns++
	s.mu.Unlock()
	return &TurnGuard{s: s}
}

// TurnGuard — страж активного хода: End завершает ход для планировщика.
type TurnGuard struct {
	s    *Scheduler
	once sync.Once
}

// End завершает ход и будит задачи мастерской, ожидавшие его конца.
func (g *TurnGuard) End() {
	g.once.Do(func() {
		g.s.mu.Lock()
		g.s.turns--
		if g.s.turns == 0 {
			g.s.cond.Broadcast()
		}
		g.s.mu.Unlock()
	})
}

// WorkshopBusy — задач мастерской, держащих слот (наблюдаемость и тесты).
func (s *Scheduler) WorkshopBusy() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy
}

// WorkshopWaiting — задач мастерской, ожидающих слот (наблюдаемость и тесты).
func (s *Scheduler) WorkshopWaiting() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.waiting
}
