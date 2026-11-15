package providerroute

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"tns65/internal/journal"
)

// RoutedProseSlot — слот прозы задачи: на каждый Stream роутер выбирает
// провайдера по кругу, ошибка до первого чанка уходит следующему провайдеру
// маршрута. После первого чанка повтор невозможен: поток уже отдаётся ходу.
type RoutedProseSlot struct {
	router *Router
	task   Task

	mu       sync.Mutex
	provider string // провайдер последнего вызова — для usage.jsonl (FIX-4).
	limit    string // остаток лимита из ответа последнего вызова.
}

// Stream выбирает провайдера и стримит его поток чанков. Пустой маршрут или
// все провайдеры без ключа — ErrNoProvider; отказ всех до первого чанка —
// ErrExhausted. Оба случая ведут существующей деградацией (slot.degraded).
func (s *RoutedProseSlot) Stream(ctx context.Context, prompt string) (<-chan string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cands := s.router.candidates(s.task)
	if len(cands) == 0 {
		s.setMeta("", "")
		s.router.logTask(journal.EventProviderExhausted, s.task,
			journal.Field{Key: "errors_count", Value: "0"})
		return nil, fmt.Errorf("%w: задача %s", ErrNoProvider, s.task)
	}
	var lastErr error
	for i, cand := range cands {
		client, err := s.router.client(cand.name)
		if err == nil {
			var ch <-chan string
			ch, err = client.Stream(ctx, prompt)
			if err == nil {
				s.setMeta(cand.name, client.LimitRemaining())
				s.router.delivered(s.task)
				return ch, nil
			}
			s.setMeta(cand.name, client.LimitRemaining())
			if !failoverAllowed(err) {
				// 400 и отказ по содержанию: повтор другому провайдеру дал бы
				// второй платный запрос — ход идёт существующей деградацией.
				return nil, err
			}
		}
		// Несобранный клиент (адрес провайдера не разобрался) — тоже отказ
		// попытки: запрос уходит следующему провайдеру маршрута.
		lastErr = err
		s.router.markFailure(cand.name, err)
		to := ""
		if i+1 < len(cands) {
			to = cands[i+1].name
		}
		s.router.logTask(journal.EventProviderFailover, s.task,
			journal.Field{Key: "from", Value: cand.name},
			journal.Field{Key: "to", Value: to},
			journal.Field{Key: "error", Value: err.Error()})
	}
	s.router.logTask(journal.EventProviderExhausted, s.task,
		journal.Field{Key: "errors_count", Value: strconv.Itoa(len(cands))})
	return nil, fmt.Errorf("%w: задача %s: %w", ErrExhausted, s.task, lastErr)
}

// ProviderName — провайдер последнего вызова для журнала вызовов (FIX-4):
// usage.jsonl пишет поле provider, строки без него читаются как раньше.
func (s *RoutedProseSlot) ProviderName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.provider
}

// LimitRemaining — остаток лимита из ответа последнего вызова; пусто —
// провайдер лимита не отдал.
func (s *RoutedProseSlot) LimitRemaining() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.limit
}

func (s *RoutedProseSlot) setMeta(provider, limit string) {
	s.mu.Lock()
	s.provider = provider
	s.limit = limit
	s.mu.Unlock()
}
