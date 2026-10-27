package paganel_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tns65/internal/journal"
	"tns65/internal/netgate"
	"tns65/internal/paganel"
)

// fireClock — подставные часы: таймер срабатывает только когда тест закроет
// канал fire. Разведка не ждёт реального тайм-аута (T-06-10, шаг 3).
type fireClock struct {
	fire chan time.Time
}

func (c *fireClock) After(time.Duration) <-chan time.Time { return c.fire }

// TestScout — разведка наружу (T-06-10, «Проверка»): вызов через loopback
// (netgate.Local) проходит; при scout_enabled=false внешних вызовов ноль;
// превышение тайм-аута на подставных часах не задерживает ответ; четвёртый вызов
// за сессию не выполняется; в журнале строка netgate.deny для хоста вне списка.
func TestScout(t *testing.T) {
	t.Run("вызов через loopback netgate.Local проходит", func(t *testing.T) {
		var calls int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(&calls, 1)
			_, _ = w.Write([]byte("разведка ок"))
		}))
		defer srv.Close()

		scout := paganel.NewScout(paganel.ScoutConfig{Enabled: true, Budget: 3, Timeout: time.Second},
			netgate.Local(strings.TrimPrefix(srv.URL, "http://")))
		scout.SetURL(srv.URL)

		got, err := scout.Fetch(context.Background(), "город за холмом")
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if got != "разведка ок" {
			t.Fatalf("ответ разведки = %q, want «разведка ок»", got)
		}
		if atomic.LoadInt32(&calls) != 1 {
			t.Fatalf("вызовов сервера = %d, want 1", calls)
		}
	})

	t.Run("при scout_enabled=false внешних вызовов ноль", func(t *testing.T) {
		var calls int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(&calls, 1)
			_, _ = w.Write([]byte("наружу не выходим"))
		}))
		defer srv.Close()

		scout := paganel.NewScout(paganel.ScoutConfig{Enabled: false, Budget: 3, Timeout: time.Second},
			netgate.Local(strings.TrimPrefix(srv.URL, "http://")))
		scout.SetURL(srv.URL)

		if _, err := scout.Fetch(context.Background(), "город за холмом"); !errors.Is(err, paganel.ErrScoutDisabled) {
			t.Fatalf("Fetch(disabled) = %v, want ErrScoutDisabled", err)
		}
		if atomic.LoadInt32(&calls) != 0 {
			t.Fatalf("вызовов сервера при выключенной разведке = %d, want 0", calls)
		}
		if scout.Spent() != 0 {
			t.Fatalf("бюджет потрачен при выключенной разведке: %d, want 0", scout.Spent())
		}
	})

	t.Run("превышение тайм-аута на подставных часах не задерживает ответ", func(t *testing.T) {
		started := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			close(started)
			<-r.Context().Done() // сервер держит ответ до отмены запроса
		}))
		defer srv.Close()

		clock := &fireClock{fire: make(chan time.Time)}
		scout := paganel.NewScout(paganel.ScoutConfig{Enabled: true, Budget: 3, Timeout: 30 * time.Second},
			netgate.Local(strings.TrimPrefix(srv.URL, "http://")))
		scout.SetURL(srv.URL)
		scout.SetClock(clock)

		done := make(chan error, 1)
		go func() {
			_, err := scout.Fetch(context.Background(), "город за холмом")
			done <- err
		}()

		<-started
		close(clock.fire) // масштабированные часы: тайм-аут наступил сразу

		select {
		case err := <-done:
			if !errors.Is(err, paganel.ErrScoutTimeout) {
				t.Fatalf("Fetch при тайм-ауте = %v, want ErrScoutTimeout", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("разведка задержала ответ после тайм-аута")
		}
	})

	t.Run("четвёртый вызов за сессию не выполняется", func(t *testing.T) {
		var calls int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(&calls, 1)
			_, _ = w.Write([]byte("ок"))
		}))
		defer srv.Close()

		scout := paganel.NewScout(paganel.ScoutConfig{Enabled: true, Budget: 3, Timeout: time.Second},
			netgate.Local(strings.TrimPrefix(srv.URL, "http://")))
		scout.SetURL(srv.URL)

		for i := 0; i < 3; i++ {
			if _, err := scout.Fetch(context.Background(), "тема"); err != nil {
				t.Fatalf("вызов %d: %v", i+1, err)
			}
		}
		if _, err := scout.Fetch(context.Background(), "тема"); !errors.Is(err, paganel.ErrScoutBudgetExhausted) {
			t.Fatalf("четвёртый вызов = %v, want ErrScoutBudgetExhausted", err)
		}
		if atomic.LoadInt32(&calls) != 3 {
			t.Fatalf("вызовов сервера = %d, want 3", calls)
		}
		if scout.Spent() != 3 {
			t.Fatalf("израсходованный бюджет = %d, want 3", scout.Spent())
		}
	})

	t.Run("поля разведки читаются из config.yaml", func(t *testing.T) {
		cfg, err := paganel.LoadPaganelConfig(filepath.Join("..", "..", "config.yaml"))
		if err != nil {
			t.Fatalf("LoadPaganelConfig(engine/config.yaml): %v", err)
		}
		if cfg.Scout.Enabled {
			t.Fatal("engine/config.yaml scout_enabled = true, want false (дефолт — выключено)")
		}
		if cfg.Scout.Budget != paganel.ScoutBudgetDefault {
			t.Fatalf("scout_budget = %d, want %d", cfg.Scout.Budget, paganel.ScoutBudgetDefault)
		}
		if cfg.Scout.Timeout != paganel.ScoutTimeoutDefault {
			t.Fatalf("scout_timeout = %v, want %v", cfg.Scout.Timeout, paganel.ScoutTimeoutDefault)
		}
		if cfg.Scout.KeyRef == "" {
			t.Fatal("key_ref разведки пуст, want ссылку на ключ в settings.toml")
		}
	})

	t.Run("в журнале строка netgate.deny для хоста вне списка", func(t *testing.T) {
		dir := t.TempDir()
		logPath := filepath.Join(dir, "journal.log")
		logger, err := journal.New(logPath)
		if err != nil {
			t.Fatalf("journal.New: %v", err)
		}
		netgate.SetLogger(logger)
		defer netgate.SetLogger(nil)

		scout := paganel.NewScout(paganel.ScoutConfig{Enabled: true, Budget: 1, Timeout: time.Second},
			netgate.Client("outside.example"))
		scout.SetURL("http://outside.example/research")

		if _, err := scout.Fetch(context.Background(), "город за холмом"); !errors.Is(err, netgate.ErrDeniedHost) {
			t.Fatalf("Fetch вне списка = %v, want netgate.ErrDeniedHost", err)
		}

		raw, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("чтение журнала: %v", err)
		}
		if !strings.Contains(string(raw), journal.EventNetgateDeny) ||
			!strings.Contains(string(raw), "outside.example") {
			t.Fatalf("в журнале нет строки netgate.deny с хостом outside.example:\n%s", raw)
		}
	})
}
