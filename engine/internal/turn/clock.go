package turn

import "time"

// Время 1:1 (T-03-12, LT-10.1): время мира — целые минуты с начала мира,
// world_min = world_start_min + elapsed_real_min. Часы идут от инжектируемого
// источника (testkit.Clock) и не останавливаются ни в игре, ни в паузах —
// «мир не смещается относительно игрока» означает ровно эту формулу.

// ClockSource — источник реального времени для часов мира: текущий момент и
// таймер. testkit.Clock удовлетворяет контракту; в проде — системные часы.
type ClockSource interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// realClock — системные часы по умолчанию.
type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// WorldClock — часы мира темпом 1:1. origin — реальный момент, соответствующий
// startMin (минуте начала мира); каждая реальная минута — минута мира.
type WorldClock struct {
	startMin int64
	src      ClockSource
	origin   time.Time
}

// NewWorldClock создаёт часы мира: startMin — world_meta.world_start_min
// (пишется при рождении), src — источник времени; nil — системные часы.
// Точка отсчёта origin берётся в момент создания.
func NewWorldClock(startMin int64, src ClockSource) *WorldClock {
	origin := time.Now()
	if src != nil {
		origin = src.Now()
	}
	return &WorldClock{startMin: startMin, src: src, origin: origin}
}

// Now возвращает текущую минуту мира: startMin + целые прошедшие реальные минуты.
func (w *WorldClock) Now() int64 {
	return w.startMin + w.ElapsedMinutes()
}

// ElapsedMinutes — целые реальные минуты, прошедшие с начала мира.
func (w *WorldClock) ElapsedMinutes() int64 {
	now := time.Now()
	if w.src != nil {
		now = w.src.Now()
	}
	return int64(now.Sub(w.origin) / time.Minute)
}
