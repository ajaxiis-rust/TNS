package testkit

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrCallLimit — превышен потолок вызовов фейкового слота (T-12-27): тест падает,
// когда ковка/карта делает больше maxSlotCalls обращений, а не тянет полную книгу.
var ErrCallLimit = errors.New("testkit: превышен потолок вызовов слота")

// FakeSlot — фейковый слот генерации: отвечает заготовленным текстом по роли,
// считает вызовы, умеет задержку, отказ и потолок вызовов. Сети не трогает.
type FakeSlot struct {
	mu        sync.Mutex
	responses map[string]string
	delay     time.Duration
	err       error
	clock     *Clock
	calls     int
	callLimit int
}

// NewFakeSlot — пустой слот: роль без заготовки отвечает пустой строкой.
func NewFakeSlot() *FakeSlot {
	return &FakeSlot{responses: map[string]string{}}
}

// SetResponse задаёт текст для роли; роль "" — ответ по умолчанию.
func (s *FakeSlot) SetResponse(role, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses[role] = text
}

// SetDelay задаёт задержку ответа; с подключёнными часами продвигается без
// реального ожидания, иначе спит реально.
func (s *FakeSlot) SetDelay(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delay = d
}

// SetClock подключает часы для отсчёта задержки.
func (s *FakeSlot) SetClock(c *Clock) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock = c
}

// FailWith — следующие вызовы возвращают err; nil снимает отказ.
func (s *FakeSlot) FailWith(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

// Calls — число вызовов Generate.
func (s *FakeSlot) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// SetCallLimit ставит потолок вызовов (T-12-27); n<=0 — потолка нет. Вызов сверх
// потолка возвращает ErrCallLimit, поэтому тест падает при превышении.
func (s *FakeSlot) SetCallLimit(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callLimit = n
}

// Generate отвечает заготовленным текстом роли, выждав задержку и вернув отказ,
// если он задан.
func (s *FakeSlot) Generate(ctx context.Context, role, _ string) (string, error) {
	s.mu.Lock()
	s.calls++
	if s.callLimit > 0 && s.calls > s.callLimit {
		s.mu.Unlock()
		return "", ErrCallLimit
	}
	delay := s.delay
	clock := s.clock
	err := s.err
	text, ok := s.responses[role]
	if !ok {
		text = s.responses[""]
	}
	s.mu.Unlock()

	if delay > 0 {
		var wait <-chan time.Time
		if clock != nil {
			wait = clock.After(delay)
		} else {
			wait = time.After(delay)
		}
		select {
		case <-wait:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if err != nil {
		return "", err
	}
	return text, nil
}
