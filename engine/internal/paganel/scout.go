package paganel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Scout — разведка наружу (T-06-10, W-10): единственный целевой вызов внешней
// модели Паганеля в прологе, до Ткача. Дешёвая механическая проверка выполняется
// до вызова; «нет» — нулевая цена, «да» — один вызов. Пропуск разведки —
// ускоритель, а не единственный путь: исчерпание бюджета или тайм-аут не
// блокирует ход (W-10.3, W-10.4). Вызов идёт только через сетевой гейт.
type Scout struct {
	client *http.Client
	cfg    ScoutConfig
	budget *Budget
	clock  scoutClock
	url    string
}

// scoutClock — источник таймера разведки. В проде — системные часы, в тестах —
// подставные (testkit.Clock), чтобы тайм-аут не ждал по-настоящему (T-06-10,
// шаг 3).
type scoutClock interface {
	After(d time.Duration) <-chan time.Time
}

// realScoutClock — системные часы по умолчанию.
type realScoutClock struct{}

func (realScoutClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// ErrScoutDisabled — разведка выключена переключателем «походы наружу»
// (scout_enabled=false, F-08): внешних вызовов нет.
var ErrScoutDisabled = errors.New("paganel: разведка наружу выключена")

// ErrScoutBudgetExhausted — бюджет разведки на сессию исчерпан (W-10.4): вызов
// не выполняется, ход не останавливается и ковка не отменяется.
var ErrScoutBudgetExhausted = errors.New("paganel: бюджет разведки исчерпан")

// ErrScoutTimeout — тайм-аут целевого вызова разведки (W-10.3): пропуск не
// блокирует ход.
var ErrScoutTimeout = errors.New("paganel: тайм-аут разведки")

// NewScout создаёт разведку с настройками cfg и клиентом наружу. Бюджет
// создаётся из cfg.Budget; клиент наружу строится сетевым гейтом — loopback в
// тестах через netgate.Local, наружный хост — через netgate.Client (T-06-10,
// шаг 5).
func NewScout(cfg ScoutConfig, client *http.Client) *Scout {
	return &Scout{
		client: client,
		cfg:    cfg,
		budget: NewBudget(BudgetLimits{Scout: cfg.Budget}),
		clock:  realScoutClock{},
		url:    "http://paganel.example/research",
	}
}

// SetURL задаёт адрес целевого вызова (тестовая точка входа: httptest-сервер или
// хост вне белого списка). В контуре P5 адрес собирается из хоста внешней модели
// Паганеля в белом списке netgate; key_ref — ссылка на ключ в settings.toml.
func (s *Scout) SetURL(url string) {
	s.url = url
}

// SetClock подменяет часы разведки (тестовая точка входа); nil — системные.
func (s *Scout) SetClock(c scoutClock) {
	if c == nil {
		c = realScoutClock{}
	}
	s.clock = c
}

// SetBudget подменяет бюджет разведки (тестовая точка входа).
func (s *Scout) SetBudget(b *Budget) {
	s.budget = b
}

// Spent — израсходованные вызовы разведки (для счётчиков и тестов).
func (s *Scout) Spent() int {
	if s.budget == nil {
		return 0
	}
	return s.budget.ScoutSpent()
}

// Fetch — целевой вызов внешней модели Паганеля по теме. Разведка выключена —
// ErrScoutDisabled без вызова; бюджет исчерпан — ErrScoutBudgetExhausted;
// превышение тайм-аута — ErrScoutTimeout. Ход игрока отказом не затрагивается.
func (s *Scout) Fetch(ctx context.Context, topic string) (string, error) {
	if !s.cfg.Enabled {
		return "", ErrScoutDisabled
	}
	if s.budget != nil && !s.budget.ScoutSpend() {
		return "", ErrScoutBudgetExhausted
	}
	if s.client == nil {
		return "", errors.New("paganel: клиент разведки не подключён")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, s.url, nil)
	if err != nil {
		return "", fmt.Errorf("paganel: запрос разведки: %w", err)
	}

	type result struct {
		body string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := s.client.Do(req)
		if err != nil {
			done <- result{err: err}
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			done <- result{err: err}
			return
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			done <- result{err: fmt.Errorf("paganel: разведка вернула статус %d", resp.StatusCode)}
			return
		}
		done <- result{body: string(body)}
	}()

	timer := s.clock.After(s.cfg.Timeout)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timer:
		return "", ErrScoutTimeout
	case r := <-done:
		return r.body, r.err
	}
}
