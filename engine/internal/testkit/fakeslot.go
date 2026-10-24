package testkit

import (
	"context"
	"sync"
	"time"
)

// FakeSlot — фейковый слот генерации: отвечает заготовленным текстом по роли,
// считает вызовы, умеет задержку и отказ. Сети не трогает.
type FakeSlot struct {
	mu        sync.Mutex
	responses map[string]string
	delay     time.Duration
	err       error
	clock     *Clock
	calls     int
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

// Generate отвечает заготовленным текстом роли, выждав задержку и вернув отказ,
// если он задан.
func (s *FakeSlot) Generate(ctx context.Context, role, _ string) (string, error) {
	s.mu.Lock()
	s.calls++
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
