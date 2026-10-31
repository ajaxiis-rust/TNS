package models

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/journal"
	"tns65/internal/netgate"
)

// fakeChat — подставной провайдер: стримит заготовленные чанки, умеет отказ.
// Сети не трогает — заменяет живой API в тестах контракта провайдера.
type fakeChat struct {
	chunks []string
	err    error
}

func (f *fakeChat) Stream(ctx context.Context, _ string) (<-chan string, error) {
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan string)
	go func() {
		defer close(ch)
		for _, c := range f.chunks {
			select {
			case ch <- c:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// allowLoopback вносит loopback в белый список гейта на время теста: подставной
// провайдер живёт на httptest-сервере 127.0.0.1 и проходит только через гейт.
func allowLoopback(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte("netgate:\n  allowed_hosts:\n    - 127.0.0.1/32\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := netgate.Load(cfg); err != nil {
		t.Fatalf("netgate.Load: %v", err)
	}
}

// withFallbackWeaver подменяет локальную запаску :5001 на подставной httptest-слот,
// стримящий chunks построчно; возвращает отмену подмены (восстанавливает фабрику
// и закрывает сервер). Порт :5001 в тестах не занимается.
func withFallbackWeaver(t *testing.T, chunks ...string) func() {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/completion" {
			http.NotFound(w, r)
			return
		}
		fl, _ := w.(http.Flusher)
		for _, c := range chunks {
			_, _ = w.Write([]byte(c + "\n"))
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	old := fallbackChat
	fallbackChat = func() (Chat, error) { return NewWeaver(srv.URL) }
	return func() {
		fallbackChat = old
		srv.Close()
	}
}

func TestProviderContract(t *testing.T) {
	t.Run("стриминг и счётчик вызовов", func(t *testing.T) {
		var u Usage
		text, path := CompleteChat(context.Background(), &fakeChat{chunks: []string{"Смотрит ", "на ", "тебя."}},
			"промпт", string(ProviderGeminiFlash), &u, nil, "")
		if text != "Смотрит на тебя." {
			t.Fatalf("текст = %q, ожидался %q", text, "Смотрит на тебя.")
		}
		if path != string(ProviderGeminiFlash) {
			t.Fatalf("путь = %q, ожидался %q", path, string(ProviderGeminiFlash))
		}
		if u.Calls != 1 {
			t.Fatalf("счётчик вызовов = %d, ожидался 1", u.Calls)
		}
	})

	t.Run("отказ провайдера — ответ запаски :5001 и ход доставлен", func(t *testing.T) {
		restore := withFallbackWeaver(t, "Ответ запаски :5001")
		defer restore()

		var u Usage
		text, path := CompleteChat(context.Background(), &fakeChat{err: ErrProviderRefused},
			"промпт", string(ProviderGeminiFlash), &u, nil, "")
		if text != "Ответ запаски :5001" {
			t.Fatalf("ход не доставлен ответом запаски: текст = %q", text)
		}
		if path != PathFallback {
			t.Fatalf("путь = %q, ожидался %q", path, PathFallback)
		}
		if u.Calls != 1 {
			t.Fatalf("счётчик вызовов = %d, ожидался 1", u.Calls)
		}
	})

	t.Run("отказ сети — ответ запаски", func(t *testing.T) {
		restore := withFallbackWeaver(t, "Ответ запаски :5001")
		defer restore()

		text, path := CompleteChat(context.Background(), &fakeChat{err: ErrNetwork},
			"промпт", string(ProviderGeminiFlash), nil, nil, "")
		if text != "Ответ запаски :5001" || path != PathFallback {
			t.Fatalf("путь = %q, текст = %q; ожидались fallback и ответ запаски", path, text)
		}
	})

	t.Run("пустой ответ — ответ запаски", func(t *testing.T) {
		restore := withFallbackWeaver(t, "Ответ запаски :5001")
		defer restore()

		text, path := CompleteChat(context.Background(), &fakeChat{}, "промпт", string(ProviderGeminiFlash), nil, nil, "")
		if text != "Ответ запаски :5001" || path != PathFallback {
			t.Fatalf("путь = %q, текст = %q; ожидались fallback и ответ запаски", path, text)
		}
	})

	t.Run("деградация — событие slot.degraded", func(t *testing.T) {
		restore := withFallbackWeaver(t, "Ответ запаски :5001")
		defer restore()

		logPath := filepath.Join(t.TempDir(), "journal.log")
		logger, err := journal.New(logPath)
		if err != nil {
			t.Fatalf("journal.New: %v", err)
		}

		text, path := CompleteChat(context.Background(), &fakeChat{err: ErrNetwork},
			"промпт", string(ProviderGeminiFlash), nil, logger, "")
		if text != "Ответ запаски :5001" || path != PathFallback {
			t.Fatalf("текст = %q, путь = %q", text, path)
		}

		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read journal: %v", err)
		}
		if !strings.Contains(string(data), journal.EventSlotDegraded) {
			t.Fatalf("в журнале нет события slot.degraded:\n%s", data)
		}
		if !strings.Contains(string(data), string(ProviderGeminiFlash)) {
			t.Fatalf("в событии slot.degraded нет роли слота:\n%s", data)
		}
	})

	t.Run("usage.jsonl — счётчики каждого слота", func(t *testing.T) {
		restore := withFallbackWeaver(t, "Ответ запаски :5001")
		defer restore()

		usagePath := filepath.Join(t.TempDir(), "usage.jsonl")

		// Успешный вызов внешнего слота — запись с ролью gemini-flash.
		var u Usage
		text, path := CompleteChat(context.Background(), &fakeChat{chunks: []string{"Штатный ответ."}},
			"промпт", string(ProviderGeminiFlash), &u, nil, usagePath)
		if text != "Штатный ответ." || path != string(ProviderGeminiFlash) {
			t.Fatalf("текст = %q, путь = %q", text, path)
		}

		// Отказ — ответ запаски, плюс запись локального слота local.
		var u2 Usage
		text, path = CompleteChat(context.Background(), &fakeChat{err: ErrProviderRefused},
			"промпт", string(ProviderGeminiFlash), &u2, nil, usagePath)
		if text != "Ответ запаски :5001" || path != PathFallback {
			t.Fatalf("текст = %q, путь = %q", text, path)
		}

		data, err := os.ReadFile(usagePath)
		if err != nil {
			t.Fatalf("read usage.jsonl: %v", err)
		}
		if !strings.Contains(string(data), `"role":"gemini-flash"`) {
			t.Fatalf("в usage.jsonl нет записи внешнего слота:\n%s", data)
		}
		if !strings.Contains(string(data), `"role":"local"`) {
			t.Fatalf("в usage.jsonl нет записи запаски local:\n%s", data)
		}
		if !strings.Contains(string(data), `"calls":1`) {
			t.Fatalf("в usage.jsonl нет счётчика calls=1:\n%s", data)
		}
	})

	t.Run("разделённые ошибки", func(t *testing.T) {
		if !errors.Is(classifyDoErr(netgate.ErrDeniedHost), netgate.ErrDeniedHost) {
			t.Fatalf("ошибка гейта не сохранена: %v", classifyDoErr(netgate.ErrDeniedHost))
		}
		if !errors.Is(classifyDoErr(errors.New("conn refused")), ErrNetwork) {
			t.Fatalf("ошибка сети не обёрнута в ErrNetwork")
		}

		allowLoopback(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		c, err := NewProviderClient(srv.URL, "", "")
		if err != nil {
			t.Fatalf("NewProviderClient: %v", err)
		}
		if _, err := c.Stream(context.Background(), "промпт"); !errors.Is(err, ErrProviderRefused) {
			t.Fatalf("статус 500 не классифицирован как отказ провайдера: %v", err)
		}
	})

	t.Run("отмена по контексту", func(t *testing.T) {
		allowLoopback(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fl, _ := w.(http.Flusher)
			_, _ = w.Write([]byte("первый\n"))
			if fl != nil {
				fl.Flush()
			}
			<-r.Context().Done()
		}))
		defer srv.Close()

		c, err := NewProviderClient(srv.URL, "", "")
		if err != nil {
			t.Fatalf("NewProviderClient: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		ch, err := c.Stream(ctx, "промпт")
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		if got := <-ch; got != "первый" {
			t.Fatalf("первый чанк = %q, ожидался %q", got, "первый")
		}
		cancel()
		for range ch {
		}
	})
}

func TestPresetDefault(t *testing.T) {
	if DefaultProvider != ProviderLocal {
		t.Fatalf("DefaultProvider = %q, ожидался %q", DefaultProvider, ProviderLocal)
	}
	if got := ResetProvider(); got != ProviderLocal {
		t.Fatalf("ResetProvider = %q, ожидался %q", got, ProviderLocal)
	}
	if DefaultWeaverURL != "http://127.0.0.1:5001" {
		t.Fatalf("DefaultWeaverURL = %q, ожидался http://127.0.0.1:5001", DefaultWeaverURL)
	}
	want := []Provider{ProviderLocal, ProviderGeminiFlash, ProviderGeminiPro}
	got := ProviderPresets()
	if len(got) != len(want) {
		t.Fatalf("реестр пресетов = %v, ожидался %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("реестр пресетов = %v, ожидался %v", got, want)
		}
	}
	if p := ProviderByName("неизвестный"); p != ProviderLocal {
		t.Fatalf("неизвестный пресет = %q, ожидался local", p)
	}

	cfg, err := LoadSlotsConfig(filepath.Join("..", "..", "config.yaml"))
	if err != nil {
		t.Fatalf("LoadSlotsConfig(engine/config.yaml): %v", err)
	}
	if cfg.Default != ProviderLocal {
		t.Fatalf("slots.default = %q, ожидался local", cfg.Default)
	}
	if cfg.GeminiHost == "" {
		t.Fatalf("slots.gemini_host пуст")
	}
}

func TestProviderDeniedHost(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte("netgate:\n  allowed_hosts:\n    - generativelanguage.googleapis.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := netgate.Load(cfg); err != nil {
		t.Fatalf("netgate.Load: %v", err)
	}
	netgate.SetLogger(logger)

	c, err := NewProviderClient("https://evil.example", "", "")
	if err != nil {
		t.Fatalf("NewProviderClient: %v", err)
	}
	if _, err := c.Stream(context.Background(), "промпт"); !errors.Is(err, netgate.ErrDeniedHost) {
		t.Fatalf("err = %v, ожидался netgate.ErrDeniedHost", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if !strings.Contains(string(data), "netgate.deny") || !strings.Contains(string(data), "evil.example") {
		t.Fatalf("в журнале нет записи netgate.deny с хостом evil.example:\n%s", data)
	}
}
