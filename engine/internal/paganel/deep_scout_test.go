package paganel_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tns65/internal/journal"
	"tns65/internal/models"
	"tns65/internal/netgate"
	"tns65/internal/paganel"
)

// liveSlot — слот разведки через живой HTTP: тело ответа приходит одним чанком,
// статус не 2xx — отказ (как у клиентов моделей).
type liveSlot struct {
	url    string
	client *http.Client
}

func (s *liveSlot) Stream(ctx context.Context, prompt string) (<-chan string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, strings.NewReader(prompt))
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("разведка вернула статус %d", resp.StatusCode)
	}
	ch := make(chan string, 1)
	ch <- string(body)
	close(ch)
	return ch, nil
}

func (s *liveSlot) ProviderName() string { return "paganel" }

func (s *liveSlot) LimitRemaining() string { return "99" }

// TestDeepScoutLiveCall — живой вызов разведки пишется в журнал вызовов и журнал
// станка: провайдер, размер промпта, токены, латентность, сводка результата.
func TestDeepScoutLiveCall(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		fmt.Fprintf(w, "справка %d: земля за холмом", n)
	}))
	defer srv.Close()

	dir := t.TempDir()
	usagePath := filepath.Join(dir, "usage.jsonl")
	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	slot := &liveSlot{url: srv.URL, client: netgate.Local(strings.TrimPrefix(srv.URL, "http://"))}
	scout := paganel.NewDeepScout(paganel.DeepScoutConfig{
		Provider:  "paganel",
		Endpoint:  srv.URL,
		Budget:    3,
		Timeout:   time.Second,
		UsagePath: usagePath,
		Logger:    logger,
	}, slot)

	text, err := scout.Run(context.Background(), []string{"география", "эпоха", "нравы"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(text, "справка 3") {
		t.Fatalf("ответ разведки = %q, want три ответа", text)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("вызовов сервера = %d, want 3", got)
	}
	if got := scout.Spent(); got != 3 {
		t.Fatalf("израсходовано вызовов = %d, want 3", got)
	}

	callsLog := models.ReadCalls(usagePath)
	if len(callsLog) != 3 {
		t.Fatalf("строк журнала вызовов = %d, want 3", len(callsLog))
	}
	for _, rec := range callsLog {
		if rec.Role != paganel.RoleScout {
			t.Fatalf("роль записи = %q, want %q", rec.Role, paganel.RoleScout)
		}
		if rec.Provider != "paganel" {
			t.Fatalf("провайдер записи = %q, want paganel", rec.Provider)
		}
		if rec.Calls != 1 || rec.Tokens == 0 || rec.PromptBytes == 0 {
			t.Fatalf("счётчики записи: calls=%d tokens=%d prompt_bytes=%d", rec.Calls, rec.Tokens, rec.PromptBytes)
		}
		if rec.Endpoint != srv.URL {
			t.Fatalf("endpoint записи = %q, want %q", rec.Endpoint, srv.URL)
		}
		if rec.Result == "" {
			t.Fatal("сводка результата в журнале вызовов пуста")
		}
		if rec.Error != "" {
			t.Fatalf("ошибка в удачной записи: %q", rec.Error)
		}
		if rec.LimitRemaining != "99" {
			t.Fatalf("остаток лимита = %q, want 99", rec.LimitRemaining)
		}
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("чтение журнала: %v", err)
	}
	if !strings.Contains(string(raw), journal.EventScoutCall) {
		t.Fatalf("в журнале станка нет события %s:\n%s", journal.EventScoutCall, raw)
	}
}

// TestDeepScoutErrorsLogged — отказ провайдера и тайм-аут пишутся в журнал
// вызовов: «обязательное» логирование включает отказы.
func TestDeepScoutErrorsLogged(t *testing.T) {
	t.Run("отказ провайдера", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		usagePath := filepath.Join(t.TempDir(), "usage.jsonl")
		scout := paganel.NewDeepScout(paganel.DeepScoutConfig{
			Provider: "paganel", Budget: 1, Timeout: time.Second, UsagePath: usagePath,
		}, &liveSlot{url: srv.URL, client: netgate.Local(strings.TrimPrefix(srv.URL, "http://"))})

		if _, err := scout.Run(context.Background(), []string{"география"}); err == nil {
			t.Fatal("Run при отказе провайдера не вернул ошибку")
		}
		records := models.ReadCalls(usagePath)
		if len(records) != 1 {
			t.Fatalf("строк журнала вызовов = %d, want 1", len(records))
		}
		if records[0].Error == "" {
			t.Fatal("запись отказа без ошибки: журнал не годится для отладки")
		}
	})

	t.Run("тайм-аут", func(t *testing.T) {
		stop := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done(): // сервер держит ответ до отмены запроса
			case <-stop:
			}
		}))
		defer srv.Close()
		defer close(stop)

		usagePath := filepath.Join(t.TempDir(), "usage.jsonl")
		scout := paganel.NewDeepScout(paganel.DeepScoutConfig{
			Provider: "paganel", Budget: 1, Timeout: 40 * time.Millisecond, UsagePath: usagePath,
		}, &liveSlot{url: srv.URL, client: netgate.Local(strings.TrimPrefix(srv.URL, "http://"))})

		if _, err := scout.Run(context.Background(), []string{"география"}); !errors.Is(err, paganel.ErrScoutTimeout) {
			t.Fatalf("Run при тайм-ауте = %v, want ErrScoutTimeout", err)
		}
		records := models.ReadCalls(usagePath)
		if len(records) != 1 {
			t.Fatalf("строк журнала вызовов = %d, want 1", len(records))
		}
		if !strings.Contains(records[0].Error, "тайм-аут") {
			t.Fatalf("ошибка записи = %q, want тайм-аут", records[0].Error)
		}
	})
}

// TestDeepScoutBudget — четвёртая тема за прогон не выполняется: бюджет 3 вызова
// (как у разведки в игре, W-10.4).
func TestDeepScoutBudget(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte("ответ"))
	}))
	defer srv.Close()

	scout := paganel.NewDeepScout(paganel.DeepScoutConfig{
		Provider: "paganel", Budget: 3, Timeout: time.Second,
		UsagePath: filepath.Join(t.TempDir(), "usage.jsonl"),
	}, &liveSlot{url: srv.URL, client: netgate.Local(strings.TrimPrefix(srv.URL, "http://"))})

	if _, err := scout.Run(context.Background(), []string{"a", "b", "c", "d"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("вызовов сервера = %d, want 3", got)
	}
	if got := scout.Spent(); got != 3 {
		t.Fatalf("израсходовано вызовов = %d, want 3", got)
	}
}

// TestLoadDeepScout — сборка разведки из конфигурации экземпляра по конвенциям
// B-4: key_ref — имя провайдера в providers.toml и ключ в settings.toml.
func TestLoadDeepScout(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	providersPath := filepath.Join(dir, "providers.toml")
	settingsPath := filepath.Join(dir, "settings.toml")

	writeFile := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("запись %s: %v", path, err)
		}
	}
	writeFile(configPath, "paganel:\n  enabled: true\n  scout_enabled: true\n  scout_budget: 2\n  scout_timeout: 5s\n  key_ref: paganel\n")
	writeFile(providersPath, "[[providers]]\nname = \"paganel\"\nprotocol = \"openai\"\nbase_url = \"https://api.example.com/v1\"\ndefault_model = \"m1\"\n")
	writeFile(settingsPath, "paganel = \"sk-test\"\n")

	scout := paganel.LoadDeepScout(configPath, providersPath, settingsPath, "", nil)
	if scout == nil {
		t.Fatal("LoadDeepScout с провайдером и ключом вернул nil")
	}
	if scout.Provider() != "paganel" {
		t.Fatalf("провайдер = %q, want paganel", scout.Provider())
	}

	// Нет ключа — разведка выключена, а не падает: рождение не зависит от API.
	writeFile(settingsPath, "")
	if scout := paganel.LoadDeepScout(configPath, providersPath, settingsPath, "", nil); scout != nil {
		t.Fatal("LoadDeepScout без ключа вернул разведку")
	}

	// Переключатель выключен — разведки нет.
	writeFile(settingsPath, "paganel = \"sk-test\"\n")
	writeFile(configPath, "paganel:\n  scout_enabled: false\n  key_ref: paganel\n")
	if scout := paganel.LoadDeepScout(configPath, providersPath, settingsPath, "", nil); scout != nil {
		t.Fatal("LoadDeepScout при scout_enabled=false вернул разведку")
	}

	// Провайдера key_ref нет в реестре — разведки нет.
	writeFile(configPath, "paganel:\n  scout_enabled: true\n  key_ref: paganel\n")
	writeFile(providersPath, "[[providers]]\nname = \"other\"\nprotocol = \"openai\"\nbase_url = \"https://api.example.com/v1\"\n")
	if scout := paganel.LoadDeepScout(configPath, providersPath, settingsPath, "", nil); scout != nil {
		t.Fatal("LoadDeepScout без провайдера key_ref вернул разведку")
	}
}
