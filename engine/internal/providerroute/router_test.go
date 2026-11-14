package providerroute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"tns65/internal/journal"
	"tns65/internal/models"
	"tns65/internal/providercfg"
)

// testCatalog — реестр провайдеров для тестов роутера.
const testCatalog = `
[[providers]]
name = "alpha"
protocol = "openai"
base_url = "https://alpha.example/v1"
default_model = "alpha-1"

[[providers]]
name = "beta"
protocol = "openai"
base_url = "https://beta.example/v1"
default_model = "beta-1"
`

// fakeClient — подставной клиент провайдера: готовые чанки, ошибка отказа и
// счётчик вызовов.
type fakeClient struct {
	name   string
	chunks []string
	err    error
	limit  string

	mu    sync.Mutex
	calls int
}

func (f *fakeClient) Stream(_ context.Context, _ string) (<-chan string, error) {
	f.mu.Lock()
	f.calls++
	err := f.err
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	ch := make(chan string, len(f.chunks))
	for _, c := range f.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func (f *fakeClient) ProviderName() string   { return f.name }
func (f *fakeClient) LimitRemaining() string { return f.limit }

func (f *fakeClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// testRouter — роутер на подставных клиентах и журнале во временном каталоге.
func testRouter(t *testing.T, routes map[Task]Route, keys map[string]string, clients map[string]*fakeClient) (*Router, string) {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	cfg := Settings{Routes: routes, Keys: keys}
	r := NewRouter(cfg, providercfg.Parse(testCatalog), logger)
	r.factory = func(name string) (ProviderClient, error) {
		c, ok := clients[name]
		if !ok {
			return nil, fmt.Errorf("нет подставного клиента %q", name)
		}
		return c, nil
	}
	return r, logPath
}

// drain читает весь поток слота в одну строку.
func drain(t *testing.T, slot *RoutedProseSlot, prompt string) (string, error) {
	t.Helper()
	ch, err := slot.Stream(context.Background(), prompt)
	if err != nil {
		return "", err
	}
	var out string
	for chunk := range ch {
		out += chunk
	}
	return out, nil
}

// journalRecord — одна строка журнала станка.
type journalRecord struct {
	Source string `json:"source"`
	Event  string `json:"event"`
	Fields []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"fields"`
}

func (r journalRecord) field(key string) string {
	for _, f := range r.Fields {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}

func readJournal(t *testing.T, path string) []journalRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение журнала: %v", err)
	}
	var out []journalRecord
	for _, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}
		var rec journalRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("журнал: битая строка %q: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func eventsOf(records []journalRecord, event string) []journalRecord {
	var out []journalRecord
	for _, rec := range records {
		if rec.Event == event {
			out = append(out, rec)
		}
	}
	return out
}

func keyedKeys(names ...string) map[string]string {
	out := make(map[string]string, len(names))
	for _, name := range names {
		out[name] = "sk-" + name
	}
	return out
}

func TestRouterRotation(t *testing.T) {
	alpha := &fakeClient{name: "alpha", chunks: []string{"альфа"}}
	beta := &fakeClient{name: "beta", chunks: []string{"бета"}}
	r, logPath := testRouter(t,
		map[Task]Route{TaskChat: {"alpha", "beta"}},
		keyedKeys("alpha", "beta"),
		map[string]*fakeClient{"alpha": alpha, "beta": beta})

	slot := r.Slot(TaskChat)
	if got, err := drain(t, slot, "промпт"); err != nil || got != "альфа" {
		t.Fatalf("первый запрос = %q, %v; ожидалась альфа", got, err)
	}
	if got, err := drain(t, slot, "промпт"); err != nil || got != "бета" {
		t.Fatalf("второй запрос = %q, %v; ожидалась бета", got, err)
	}
	if slot.ProviderName() != "beta" {
		t.Fatalf("ProviderName = %q, ожидалось beta (поле provider в usage.jsonl)", slot.ProviderName())
	}

	rotates := eventsOf(readJournal(t, logPath), journal.EventProviderRotate)
	if len(rotates) != 2 {
		t.Fatalf("событий provider.rotate = %d, ожидалось 2", len(rotates))
	}
	if rotates[0].field("provider") != "alpha" || rotates[0].field("index") != "0" {
		t.Fatalf("первая ротация = %+v, ожидались alpha/0", rotates[0])
	}
	if rotates[1].field("provider") != "beta" || rotates[1].field("index") != "1" {
		t.Fatalf("вторая ротация = %+v, ожидались beta/1", rotates[1])
	}
	if rotates[0].field("task") != "chat" {
		t.Fatalf("задача в ротации = %q, ожидалась chat", rotates[0].field("task"))
	}
}

func TestRouterFailover(t *testing.T) {
	alpha := &fakeClient{name: "alpha", err: fmt.Errorf("%w: status 500", models.ErrProviderRefused)}
	beta := &fakeClient{name: "beta", chunks: []string{"второй"}}
	r, logPath := testRouter(t,
		map[Task]Route{TaskChat: {"alpha", "beta"}},
		keyedKeys("alpha", "beta"),
		map[string]*fakeClient{"alpha": alpha, "beta": beta})

	slot := r.Slot(TaskChat)
	got, err := drain(t, slot, "промпт")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got != "второй" {
		t.Fatalf("ответ = %q, ожидался ответ второго провайдера", got)
	}
	if alpha.callCount() != 1 || beta.callCount() != 1 {
		t.Fatalf("вызовы alpha/beta = %d/%d, ожидалось 1/1", alpha.callCount(), beta.callCount())
	}
	if slot.ProviderName() != "beta" {
		t.Fatalf("ProviderName = %q, ожидалось beta", slot.ProviderName())
	}

	fails := eventsOf(readJournal(t, logPath), journal.EventProviderFailover)
	if len(fails) != 1 {
		t.Fatalf("событий provider.failover = %d, ожидалось 1", len(fails))
	}
	if fails[0].field("task") != "chat" || fails[0].field("from") != "alpha" || fails[0].field("to") != "beta" {
		t.Fatalf("откат = %+v, ожидались chat: alpha → beta", fails[0])
	}
	if fails[0].field("error") == "" {
		t.Fatal("в событии provider.failover нет текста ошибки")
	}
}

func TestRouterQuarantine429(t *testing.T) {
	alpha := &fakeClient{name: "alpha", err: fmt.Errorf("%w: status 429", models.ErrProviderRefused)}
	beta := &fakeClient{name: "beta", chunks: []string{"бета"}}
	r, logPath := testRouter(t,
		map[Task]Route{TaskChat: {"alpha", "beta"}},
		keyedKeys("alpha", "beta"),
		map[string]*fakeClient{"alpha": alpha, "beta": beta})

	now := time.Now()
	r.now = func() time.Time { return now }
	slot := r.Slot(TaskChat)

	if _, err := drain(t, slot, "промпт"); err != nil {
		t.Fatalf("первый запрос: %v", err)
	}
	if alpha.callCount() != 1 {
		t.Fatalf("вызовов alpha = %d, ожидался 1", alpha.callCount())
	}
	// Следующий запрос не возвращается к лимитированному провайдеру.
	if _, err := drain(t, slot, "промпт"); err != nil {
		t.Fatalf("второй запрос: %v", err)
	}
	if alpha.callCount() != 1 {
		t.Fatalf("после 429 alpha вызван %d раз, ожидался карантин", alpha.callCount())
	}
	// Карантин истекает — провайдер снова в круге.
	now = now.Add(QuarantineWindow + time.Second)
	if _, err := drain(t, slot, "промпт"); err != nil {
		t.Fatalf("третий запрос: %v", err)
	}
	if alpha.callCount() != 2 {
		t.Fatalf("после карантина вызовов alpha = %d, ожидалось 2", alpha.callCount())
	}

	if len(eventsOf(readJournal(t, logPath), journal.EventProviderFailover)) != 2 {
		t.Fatal("отказы 429 не отмечены событиями provider.failover")
	}
}

// retryAfterFake — ошибка 429 с заголовком Retry-After.
type retryAfterFake struct {
	after time.Duration
}

func (e *retryAfterFake) Error() string             { return "provider: status 429" }
func (e *retryAfterFake) RetryAfter() time.Duration { return e.after }

func TestRouterRetryAfterQuarantine(t *testing.T) {
	alpha := &fakeClient{name: "alpha", err: &retryAfterFake{after: 2 * time.Minute}}
	beta := &fakeClient{name: "beta", chunks: []string{"бета"}}
	r, _ := testRouter(t,
		map[Task]Route{TaskChat: {"alpha", "beta"}},
		keyedKeys("alpha", "beta"),
		map[string]*fakeClient{"alpha": alpha, "beta": beta})

	now := time.Now()
	r.now = func() time.Time { return now }
	slot := r.Slot(TaskChat)

	if _, err := drain(t, slot, "промпт"); err != nil {
		t.Fatalf("первый запрос: %v", err)
	}
	// Второй запрос по кругу идёт к beta и курсор двигает.
	if _, err := drain(t, slot, "промпт"); err != nil {
		t.Fatalf("второй запрос: %v", err)
	}
	// Retry-After короче фиксированного окна карантина и главнее его: третий
	// запрос снова начинается с alpha.
	now = now.Add(2*time.Minute + time.Second)
	if _, err := drain(t, slot, "промпт"); err != nil {
		t.Fatalf("третий запрос: %v", err)
	}
	if alpha.callCount() != 2 {
		t.Fatalf("вызовов alpha = %d, ожидалось 2: Retry-After главнее окна карантина", alpha.callCount())
	}
}

func TestRouterKeyRejectedUntilRestart(t *testing.T) {
	alpha := &fakeClient{name: "alpha", err: fmt.Errorf("%w: status 401", models.ErrProviderKeyRejected)}
	beta := &fakeClient{name: "beta", chunks: []string{"бета"}}
	r, _ := testRouter(t,
		map[Task]Route{TaskChat: {"alpha", "beta"}},
		keyedKeys("alpha", "beta"),
		map[string]*fakeClient{"alpha": alpha, "beta": beta})

	slot := r.Slot(TaskChat)
	for i := 0; i < 3; i++ {
		if _, err := drain(t, slot, "промпт"); err != nil {
			t.Fatalf("запрос %d: %v", i+1, err)
		}
	}
	if alpha.callCount() != 1 {
		t.Fatalf("вызовов alpha после отказа ключа = %d, ожидался 1", alpha.callCount())
	}
}

func TestRouterAfterFirstChunkNoRepeat(t *testing.T) {
	alpha := &fakeClient{name: "alpha", chunks: []string{"кусок"}}
	beta := &fakeClient{name: "beta", chunks: []string{"второй"}}
	r, _ := testRouter(t,
		map[Task]Route{TaskChat: {"alpha", "beta"}},
		keyedKeys("alpha", "beta"),
		map[string]*fakeClient{"alpha": alpha, "beta": beta})

	slot := r.Slot(TaskChat)
	got, err := drain(t, slot, "промпт")
	if err != nil || got != "кусок" {
		t.Fatalf("ответ = %q, %v; ожидался кусок", got, err)
	}
	if beta.callCount() != 0 {
		t.Fatalf("после первого чанка beta вызван %d раз, ожидалось 0", beta.callCount())
	}
	if slot.ProviderName() != "alpha" {
		t.Fatalf("ProviderName = %q, ожидалось alpha", slot.ProviderName())
	}
}

func TestRouterAllFail(t *testing.T) {
	alpha := &fakeClient{name: "alpha", err: fmt.Errorf("%w: status 502", models.ErrProviderRefused)}
	beta := &fakeClient{name: "beta", err: fmt.Errorf("%w: status 503", models.ErrProviderRefused)}
	r, logPath := testRouter(t,
		map[Task]Route{TaskChat: {"alpha", "beta"}},
		keyedKeys("alpha", "beta"),
		map[string]*fakeClient{"alpha": alpha, "beta": beta})

	slot := r.Slot(TaskChat)
	if _, err := slot.Stream(context.Background(), "промпт"); err == nil {
		t.Fatal("полный отказ маршрута не вернул ошибку")
	} else if !errors.Is(err, ErrExhausted) {
		t.Fatalf("ошибка = %v, ожидался ErrExhausted", err)
	}

	exhausted := eventsOf(readJournal(t, logPath), journal.EventProviderExhausted)
	if len(exhausted) != 1 || exhausted[0].field("errors_count") != "2" {
		t.Fatalf("событие provider.exhausted = %+v, ожидалось errors_count=2", exhausted)
	}
	if got := len(eventsOf(readJournal(t, logPath), journal.EventProviderFailover)); got != 2 {
		t.Fatalf("событий provider.failover = %d, ожидалось 2", got)
	}
}

func TestRouterClientBuildFailure(t *testing.T) {
	beta := &fakeClient{name: "beta", chunks: []string{"бета"}}
	r, _ := testRouter(t,
		map[Task]Route{TaskChat: {"alpha", "beta"}},
		keyedKeys("alpha", "beta"),
		map[string]*fakeClient{"beta": beta})

	slot := r.Slot(TaskChat)
	if _, err := drain(t, slot, "промпт"); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if slot.ProviderName() != "beta" {
		t.Fatalf("провайдер = %q, ожидался beta: несобранный клиент пропущен", slot.ProviderName())
	}
}

func TestRouterNoProvider(t *testing.T) {
	alpha := &fakeClient{name: "alpha", chunks: []string{"альфа"}}
	beta := &fakeClient{name: "beta", chunks: []string{"бета"}}
	r, logPath := testRouter(t,
		map[Task]Route{TaskChat: {"alpha", "beta"}},
		map[string]string{},
		map[string]*fakeClient{"alpha": alpha, "beta": beta})

	slot := r.Slot(TaskChat)
	_, err := slot.Stream(context.Background(), "промпт")
	if err == nil || !errors.Is(err, ErrNoProvider) {
		t.Fatalf("ошибка = %v, ожидался ErrNoProvider", err)
	}
	if alpha.callCount() != 0 || beta.callCount() != 0 {
		t.Fatal("провайдер без ключа вызван")
	}
	exhausted := eventsOf(readJournal(t, logPath), journal.EventProviderExhausted)
	if len(exhausted) != 1 || exhausted[0].field("errors_count") != "0" {
		t.Fatalf("событие provider.exhausted = %+v, ожидалось errors_count=0", exhausted)
	}
}

func TestRouterNoFailoverOn400(t *testing.T) {
	alpha := &fakeClient{name: "alpha", err: fmt.Errorf("%w: status 400", models.ErrProviderRefused)}
	beta := &fakeClient{name: "beta", chunks: []string{"бета"}}
	r, logPath := testRouter(t,
		map[Task]Route{TaskChat: {"alpha", "beta"}},
		keyedKeys("alpha", "beta"),
		map[string]*fakeClient{"alpha": alpha, "beta": beta})

	slot := r.Slot(TaskChat)
	_, err := slot.Stream(context.Background(), "промпт")
	if err == nil || errors.Is(err, ErrExhausted) {
		t.Fatalf("ошибка = %v, ожидался отказ провайдера без отката", err)
	}
	if beta.callCount() != 0 {
		t.Fatalf("400 откатилось на beta: вызовов %d", beta.callCount())
	}
	if got := len(eventsOf(readJournal(t, logPath), journal.EventProviderFailover)); got != 0 {
		t.Fatalf("событий provider.failover = %d, ожидалось 0", got)
	}
}

func TestRouterTaskCursorIsolation(t *testing.T) {
	alpha := &fakeClient{name: "alpha", chunks: []string{"альфа"}}
	beta := &fakeClient{name: "beta", chunks: []string{"бета"}}
	r, _ := testRouter(t,
		map[Task]Route{TaskChat: {"alpha", "beta"}, TaskSources: {"alpha", "beta"}},
		keyedKeys("alpha", "beta"),
		map[string]*fakeClient{"alpha": alpha, "beta": beta})

	chat := r.Slot(TaskChat)
	for i := 0; i < 2; i++ {
		if _, err := drain(t, chat, "промпт"); err != nil {
			t.Fatalf("чат: %v", err)
		}
	}
	sources := r.Slot(TaskSources)
	if _, err := drain(t, sources, "промпт"); err != nil {
		t.Fatalf("источники: %v", err)
	}
	if sources.ProviderName() != "alpha" {
		t.Fatalf("источники начали с %q, ожидался alpha: курсоры задач независимы", sources.ProviderName())
	}
}

func TestRouterLocalLast(t *testing.T) {
	beta := &fakeClient{name: "beta", chunks: []string{"бета"}}
	local := &fakeClient{name: LocalProvider, chunks: []string{"локально"}}
	r, _ := testRouter(t,
		map[Task]Route{TaskChat: {LocalProvider, "beta"}},
		keyedKeys("beta"),
		map[string]*fakeClient{"beta": beta, LocalProvider: local})

	slot := r.Slot(TaskChat)
	if _, err := drain(t, slot, "промпт"); err != nil {
		t.Fatalf("первый запрос: %v", err)
	}
	if slot.ProviderName() != "beta" {
		t.Fatalf("первый провайдер = %q, ожидался beta: local идёт последним", slot.ProviderName())
	}
	if _, err := drain(t, slot, "промпт"); err != nil {
		t.Fatalf("второй запрос: %v", err)
	}
	if slot.ProviderName() != LocalProvider {
		t.Fatalf("второй провайдер = %q, ожидался local", slot.ProviderName())
	}
}

func TestRouterUnknownProviderDropped(t *testing.T) {
	alpha := &fakeClient{name: "alpha", chunks: []string{"альфа"}}
	r, _ := testRouter(t,
		map[Task]Route{TaskChat: {"нет-такого", "alpha"}},
		keyedKeys("alpha"),
		map[string]*fakeClient{"alpha": alpha})

	slot := r.Slot(TaskChat)
	if _, err := drain(t, slot, "промпт"); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if slot.ProviderName() != "alpha" {
		t.Fatalf("провайдер = %q, ожидался alpha: имя вне реестра отброшено", slot.ProviderName())
	}
}
