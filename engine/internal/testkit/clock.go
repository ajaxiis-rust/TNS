// Package testkit — тестовый набор: инжектируемые часы, фейковый слот генерации,
// фейковый эмбеддер и счётчик вызовов слота (T-02-15).
package testkit

import (
	"sync"
	"time"
)

// Clock — инжектируемые часы: время движется только вызовом Advance, тесты не
// ждут реально. Все таймеры движка в тестах берут время отсюда.
type Clock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*clockTimer
}

type clockTimer struct {
	deadline time.Time
	ch       chan time.Time
	fired    bool
}

// NewClock возвращает часы, стоящие на начале эпохи (UTC).
func NewClock() *Clock {
	return &Clock{now: time.Unix(0, 0).UTC()}
}

// Now — текущее время часов.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance сдвигает часы вперёд на d и срабатывает подписанные таймеры.
func (c *Clock) Advance(d time.Duration) {
	if d <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for _, t := range c.timers {
		if !t.fired && !t.deadline.After(c.now) {
			t.fired = true
			select {
			case t.ch <- c.now:
			default:
			}
		}
	}
}

// After возвращает канал, получающий время при пересечении дедлайна d по часам.
func (c *Clock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	c.timers = append(c.timers, &clockTimer{deadline: c.now.Add(d), ch: ch})
	return ch
}
