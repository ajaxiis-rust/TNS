package providerroute

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"tns65/internal/journal"
	"tns65/internal/netgate"
)

// allowLoopbackGate разрешает httptest-серверу пройти сетевой гейт: его хост —
// 127.0.0.1; после теста возвращается заводской белый список.
func allowLoopbackGate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := writeTemp(t, dir, "gate.yaml", "netgate:\n  allowed_hosts:\n    - 127.0.0.1\n")
	if err := netgate.Load(path); err != nil {
		t.Fatalf("netgate.Load: %v", err)
	}
	t.Cleanup(func() {
		if err := netgate.Load(filepath.Join("..", "..", "config.yaml")); err != nil {
			t.Errorf("восстановление белого списка netgate: %v", err)
		}
	})
}

// providersTOML — реестр из двух OpenAI-совместимых провайдеров на адресах
// подставных серверов.
func providersTOML(firstURL, secondURL string) string {
	return fmt.Sprintf(`
[[providers]]
name = "first"
protocol = "openai"
base_url = %q
default_model = "first-1"

[[providers]]
name = "second"
protocol = "openai"
base_url = %q
default_model = "second-1"
`, firstURL, secondURL)
}

// textServer — подставной провайдер: считает вызовы и отдаёт чанк text
// (status 0 — 200 с SSE-строкой, иначе HTTP-ошибка).
func textServer(calls *atomic.Int32, text string, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if status != 0 {
			http.Error(w, "refused", status)
			return
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", text)
	}))
}

// streamSlot читает весь поток слота в одну строку.
func streamSlot(t *testing.T, slot *RoutedProseSlot, prompt string) (string, error) {
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

// TestChatWeaverFailoverTwoProviders — критерий §12.1: две модели с ключами,
// второй провайдер отвечает после ошибки первого, откат виден в журнале.
func TestChatWeaverFailoverTwoProviders(t *testing.T) {
	allowLoopbackGate(t)
	dir := t.TempDir()

	var firstCalls, secondCalls atomic.Int32
	first := textServer(&firstCalls, "", http.StatusInternalServerError)
	defer first.Close()
	second := textServer(&secondCalls, "второй", 0)
	defer second.Close()

	providersPath := writeTemp(t, dir, "providers.toml", providersTOML(first.URL+"/v1", second.URL+"/v1"))
	settingsPath := writeTemp(t, dir, "settings.toml", `
first = "sk-first"
second = "sk-second"
route_chat = "first,second"
`)
	configPath := writeTemp(t, dir, "config.yaml", "slots:\n  default: local\n")
	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	weaver := ChatWeaver(configPath, settingsPath, providersPath, logger)
	if weaver == nil {
		t.Fatal("ChatWeaver вернул nil при заданном route_chat")
	}

	got, err := streamSlot(t, weaver, "промпт")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got != "второй" {
		t.Fatalf("ответ = %q, ожидался ответ второго провайдера", got)
	}
	if firstCalls.Load() != 1 || secondCalls.Load() != 1 {
		t.Fatalf("вызовы first/second = %d/%d, ожидалось 1/1", firstCalls.Load(), secondCalls.Load())
	}
	if weaver.ProviderName() != "second" {
		t.Fatalf("ProviderName = %q, ожидалось second (поле provider в usage.jsonl)", weaver.ProviderName())
	}

	records := readJournal(t, logPath)
	if rotates := eventsOf(records, journal.EventProviderRotate); len(rotates) != 0 {
		t.Fatalf("события provider.rotate = %+v, ожидалось пусто: выбор провайдера идёт в usage.jsonl (DOC-6)", rotates)
	}
	fails := eventsOf(records, journal.EventProviderFailover)
	if len(fails) != 1 || fails[0].field("from") != "first" || fails[0].field("to") != "second" {
		t.Fatalf("события provider.failover = %+v, ожидался откат first → second", fails)
	}

	// Курсор сдвинут доставленным запросом: следующий идёт к second.
	if _, err := streamSlot(t, weaver, "промпт"); err != nil {
		t.Fatalf("второй Stream: %v", err)
	}
	if firstCalls.Load() != 1 || secondCalls.Load() != 2 {
		t.Fatalf("после второго запроса first/second = %d/%d, ожидалось 1/2: ротация по кругу",
			firstCalls.Load(), secondCalls.Load())
	}
}

// TestChatWeaverRotation — два рабочих провайдера: запросы идут по кругу,
// провайдер последнего вызова виден в ProviderName (поле provider журнала
// вызовов), события provider.rotate в журнале станка нет (DOC-6).
func TestChatWeaverRotation(t *testing.T) {
	allowLoopbackGate(t)
	dir := t.TempDir()

	var firstCalls, secondCalls atomic.Int32
	first := textServer(&firstCalls, "первый", 0)
	defer first.Close()
	second := textServer(&secondCalls, "второй", 0)
	defer second.Close()

	providersPath := writeTemp(t, dir, "providers.toml", providersTOML(first.URL+"/v1", second.URL+"/v1"))
	settingsPath := writeTemp(t, dir, "settings.toml", `
first = "sk-first"
second = "sk-second"
route_chat = "first,second"
`)
	configPath := writeTemp(t, dir, "config.yaml", "slots:\n  default: local\n")
	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	weaver := ChatWeaver(configPath, settingsPath, providersPath, logger)
	if weaver == nil {
		t.Fatal("ChatWeaver вернул nil")
	}
	if got, err := streamSlot(t, weaver, "промпт"); err != nil || got != "первый" {
		t.Fatalf("первый запрос = %q, %v; ожидался первый", got, err)
	}
	if got, err := streamSlot(t, weaver, "промпт"); err != nil || got != "второй" {
		t.Fatalf("второй запрос = %q, %v; ожидался второй", got, err)
	}
	if weaver.ProviderName() != "second" {
		t.Fatalf("ProviderName = %q, ожидалось second (поле provider в usage.jsonl)", weaver.ProviderName())
	}
	if rotates := eventsOf(readJournal(t, logPath), journal.EventProviderRotate); len(rotates) != 0 {
		t.Fatalf("события provider.rotate = %+v, ожидалось пусто: выбор провайдера идёт в usage.jsonl (DOC-6)", rotates)
	}
}

// TestChatWeaverNoKeys — маршрут задан, ключей нет: слот собран, но запрос
// сразу деградирует (ErrNoProvider), событие provider.exhausted.
func TestChatWeaverNoKeys(t *testing.T) {
	allowLoopbackGate(t)
	dir := t.TempDir()

	var firstCalls, secondCalls atomic.Int32
	first := textServer(&firstCalls, "первый", 0)
	defer first.Close()
	second := textServer(&secondCalls, "второй", 0)
	defer second.Close()

	providersPath := writeTemp(t, dir, "providers.toml", providersTOML(first.URL+"/v1", second.URL+"/v1"))
	settingsPath := writeTemp(t, dir, "settings.toml", `route_chat = "first,second"`+"\n")
	configPath := writeTemp(t, dir, "config.yaml", "slots:\n  default: local\n")
	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	weaver := ChatWeaver(configPath, settingsPath, providersPath, logger)
	if weaver == nil {
		t.Fatal("ChatWeaver вернул nil")
	}
	if _, err := weaver.Stream(context.Background(), "промпт"); !errors.Is(err, ErrNoProvider) {
		t.Fatalf("ошибка = %v, ожидался ErrNoProvider", err)
	}
	if firstCalls.Load() != 0 || secondCalls.Load() != 0 {
		t.Fatal("провайдер без ключа вызван")
	}
	if got := len(eventsOf(readJournal(t, logPath), journal.EventProviderExhausted)); got != 1 {
		t.Fatalf("событий provider.exhausted = %d, ожидалось 1", got)
	}
}

// TestChatWeaverWiringFail — нет config.yaml: слот не собран, в журнале
// slot.wiring_fail, ход уходит заготовкой.
func TestChatWeaverWiringFail(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	if weaver := ChatWeaver(filepath.Join(dir, "нет-config.yaml"), filepath.Join(dir, "settings.toml"), filepath.Join(dir, "providers.toml"), logger); weaver != nil {
		t.Fatal("ChatWeaver без config.yaml вернул слот")
	}
	if got := len(eventsOf(readJournal(t, logPath), journal.EventSlotWiring)); got != 1 {
		t.Fatalf("событий slot.wiring_fail = %d, ожидалось 1", got)
	}
}
