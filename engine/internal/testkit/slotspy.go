package testkit

import (
	"context"
	"errors"
	"sync"
)

// Slot — слот генерации прозы: по роли и промпту возвращает текст ответа.
type Slot interface {
	Generate(ctx context.Context, role, prompt string) (string, error)
}

// ErrTimeout — отказ слота по тайм-ауту; SlotSpy считает его тайм-аутом.
var ErrTimeout = errors.New("testkit: slot timeout")

// SlotSpy — обёртка над любым слотом со счётчиком вызовов и тайм-аутов.
type SlotSpy struct {
	inner Slot

	mu       sync.Mutex
	calls    int
	timeouts int
}

// NewSlotSpy оборачивает inner.
func NewSlotSpy(inner Slot) *SlotSpy {
	return &SlotSpy{inner: inner}
}

// Generate передаёт вызов внутреннему слоту и считает вызовы и тайм-ауты.
func (s *SlotSpy) Generate(ctx context.Context, role, prompt string) (string, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()

	out, err := s.inner.Generate(ctx, role, prompt)
	if errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		s.mu.Lock()
		s.timeouts++
		s.mu.Unlock()
	}
	return out, err
}

// Calls — число вызовов Generate.
func (s *SlotSpy) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// Timeouts — число вызовов, завершившихся тайм-аутом.
func (s *SlotSpy) Timeouts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.timeouts
}
