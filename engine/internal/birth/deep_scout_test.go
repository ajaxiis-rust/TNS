package birth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tns65/internal/models"
	"tns65/internal/netgate"
	"tns65/internal/paganel"
	"tns65/internal/testkit"
)

// liveScoutSlot — слот глубокой разведки через живой HTTP (httptest): ответ
// приходит одним чанком, статус не 2xx — отказ.
type liveScoutSlot struct {
	url    string
	client *http.Client
}

func (s *liveScoutSlot) Stream(ctx context.Context, prompt string) (<-chan string, error) {
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

func (s *liveScoutSlot) ProviderName() string { return "paganel" }

func (s *liveScoutSlot) LimitRemaining() string { return "" }

// TestDeepScoutLiveAtBirth — живой вызов разведки при рождении: фоновая задача
// завершается после готовности мира, отчёт ложится в world_meta, каждый вызов
// виден в журнале вызовов (DEC-4, B-14.4).
func TestDeepScoutLiveAtBirth(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		fmt.Fprintf(w, "разведка %d: холм, река, тракт", n)
	}))
	defer srv.Close()

	dir := t.TempDir()
	usagePath := dir + "/usage.jsonl"
	scout := paganel.NewDeepScout(paganel.DeepScoutConfig{
		Provider:  "paganel",
		Endpoint:  srv.URL,
		Budget:    3,
		Timeout:   time.Second,
		UsagePath: usagePath,
	}, &liveScoutSlot{url: srv.URL, client: netgate.Local(strings.TrimPrefix(srv.URL, "http://"))})

	res, err := Birth(validAncketa(), Config{
		WorldsRoot: dir,
		BaseDBPath: testBaseDB(t, dir),
		Embed:      testkit.NewFakeEmbed(),
		Scout:      scout,
	})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}
	waitScout(t, res)

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("вызовов разведки = %d, want 3", got)
	}
	worldDB := openReader(t, res.WorldDB)
	var raw string
	if err := worldDB.QueryRow(`SELECT value FROM world_meta WHERE "key" = ?`, scoutReportKey).Scan(&raw); err != nil {
		t.Fatalf("world_meta %s: %v", scoutReportKey, err)
	}
	if !strings.Contains(raw, "разведка 3") {
		t.Fatalf("отчёт разведки = %q, want ответы живого вызова", raw)
	}
	records := models.ReadCalls(usagePath)
	if len(records) != 3 {
		t.Fatalf("строк журнала вызовов = %d, want 3", len(records))
	}
	for _, rec := range records {
		if rec.Role != paganel.RoleScout || rec.Result == "" {
			t.Fatalf("запись разведки: role=%q result=%q", rec.Role, rec.Result)
		}
	}
}

// TestDeepScoutUnavailableAtBirth — недоступный провайдер не валит рождение и не
// блокирует его: отчёт не пишется, отказ остаётся в журнале вызовов (B-14.3).
func TestDeepScoutUnavailableAtBirth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("недоступно"))
	}))
	url := srv.URL
	srv.Close() // соединение отвергается: внешнего вызова нет

	dir := t.TempDir()
	usagePath := dir + "/usage.jsonl"
	scout := paganel.NewDeepScout(paganel.DeepScoutConfig{
		Provider:  "paganel",
		Endpoint:  url,
		Budget:    2,
		Timeout:   200 * time.Millisecond,
		UsagePath: usagePath,
	}, &liveScoutSlot{url: url, client: netgate.Local(strings.TrimPrefix(url, "http://"))})

	res, err := Birth(validAncketa(), Config{
		WorldsRoot: dir,
		BaseDBPath: testBaseDB(t, dir),
		Embed:      testkit.NewFakeEmbed(),
		Scout:      scout,
	})
	if err != nil {
		t.Fatalf("Birth при недоступном провайдере разведки: %v", err)
	}
	waitScout(t, res)

	worldDB := openReader(t, res.WorldDB)
	var raw string
	err = worldDB.QueryRow(`SELECT value FROM world_meta WHERE "key" = ?`, scoutReportKey).Scan(&raw)
	if err == nil {
		t.Fatalf("отчёт разведки записан при недоступном провайдере: %q", raw)
	}
	records := models.ReadCalls(usagePath)
	if len(records) == 0 {
		t.Fatal("отказы разведки не попали в журнал вызовов")
	}
	for _, rec := range records {
		if rec.Error == "" {
			t.Fatalf("запись разведки без ошибки при недоступном провайдере: %+v", rec)
		}
	}
}

// countingScout — подставная разведка для проверки идемпотентности.
type countingScout struct {
	calls int32
}

func (c *countingScout) Run(context.Context, []string) (string, error) {
	atomic.AddInt32(&c.calls, 1)
	return "отчёт", nil
}

// TestDeepScoutNoDuplicate — повторный прогон не дублирует разведку: готовый
// отчёт в world_meta останавливает задачу до вызовов модели.
func TestDeepScoutNoDuplicate(t *testing.T) {
	dir := t.TempDir()
	res, err := Birth(validAncketa(), Config{
		WorldsRoot: dir,
		BaseDBPath: testBaseDB(t, dir),
		Embed:      testkit.NewFakeEmbed(),
	})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}
	if res.ScoutDone != nil {
		t.Fatal("без настроенной разведки ScoutDone должен быть nil")
	}

	scout := &countingScout{}
	topics := []string{"география", "эпоха"}
	runDeepScout(res.WorldDB, nil, scout, topics)
	runDeepScout(res.WorldDB, nil, scout, topics)

	if got := atomic.LoadInt32(&scout.calls); got != 1 {
		t.Fatalf("прогонов разведки = %d, want 1", got)
	}
	worldDB := openReader(t, res.WorldDB)
	var n int
	if err := worldDB.QueryRow(`SELECT COUNT(*) FROM world_meta WHERE "key" = ?`, scoutReportKey).Scan(&n); err != nil {
		t.Fatalf("world_meta count: %v", err)
	}
	if n != 1 {
		t.Fatalf("записей отчёта разведки = %d, want 1", n)
	}
}

// waitScout ждёт завершения фоновой разведки после рождения.
func waitScout(t *testing.T, res *Result) {
	t.Helper()
	if res.ScoutDone == nil {
		t.Fatal("фоновая разведка не запущена: ScoutDone = nil")
	}
	select {
	case <-res.ScoutDone:
	case <-time.After(10 * time.Second):
		t.Fatal("фоновая разведка не завершилась")
	}
}
