package supervisor

import (
	"errors"
	"fmt"
)

// ErrStopped — надзиратель остановлен: смена модели невозможна.
var ErrStopped = errors.New("supervisor: stopped")

// ChildStatus — снимок состояния одного ребёнка надзирателя для UI (T-12-19).
type ChildStatus struct {
	Name     string `json:"name"`
	Running  bool   `json:"running"`
	Healthy  bool   `json:"healthy"`
	Degraded int    `json:"degraded"`
	Port     int    `json:"port"`
}

// Status — снимок всех детей надзирателя. Не блокирует запуск: читает только
// зафиксированные поля.
func (s *Supervisor) Status() []ChildStatus {
	s.mu.Lock()
	children := append([]*child(nil), s.children...)
	s.mu.Unlock()
	out := make([]ChildStatus, 0, len(children))
	for _, c := range children {
		out = append(out, c.snapshot())
	}
	return out
}

// SwitchModel переключает аргументы модели ребёнка: стоп → старт → health.
// При провале health откатывает на прежние аргументы и возвращает ошибку
// (T-12-19, «смена модели с откатом»).
func (s *Supervisor) SwitchModel(name string, args []string) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return ErrStopped
	}
	idx := -1
	for i, c := range s.children {
		if c.spec.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.mu.Unlock()
		return fmt.Errorf("supervisor: child %q not found", name)
	}
	old := s.children[idx]
	oldSpec := old.spec
	newSpec := oldSpec
	newSpec.Args = append([]string(nil), args...)
	s.mu.Unlock()

	// Стоп прежнего ребёнка и ожидание выхода его цикла.
	old.terminate()

	// Старт с новыми аргументами.
	newc := newChild(s, newSpec)
	s.mu.Lock()
	s.children[idx] = newc
	s.mu.Unlock()
	s.wg.Add(1)
	go func() { defer s.wg.Done(); newc.run() }()

	if err := s.waitChildReady(newc); err != nil {
		// Откат: гасим нового, восстанавливаем прежние аргументы.
		newc.terminate()
		rollback := newChild(s, oldSpec)
		s.mu.Lock()
		s.children[idx] = rollback
		s.mu.Unlock()
		s.wg.Add(1)
		go func() { defer s.wg.Done(); rollback.run() }()
		_ = s.waitChildReady(rollback)
		return fmt.Errorf("supervisor: switch model %q: %w (rolled back)", name, err)
	}
	return nil
}

// waitChildReady ждёт готовности ребёнка (закрытие readyCh) или его выхода,
// но не дольше ready_timeout спецификации.
func (s *Supervisor) waitChildReady(c *child) error {
	deadline := s.clock.Now().Add(c.spec.ReadyTimeout)
	for {
		select {
		case <-c.readyCh:
			return nil
		case <-c.done:
			return ErrNotReady
		default:
		}
		if !s.clock.Now().Before(deadline) {
			return ErrNotReady
		}
		select {
		case <-c.readyCh:
			return nil
		case <-c.done:
			return ErrNotReady
		case <-s.clock.After(c.spec.PollInterval):
		}
	}
}
