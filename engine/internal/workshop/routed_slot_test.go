package workshop_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"tns65/internal/db"
	"tns65/internal/netgate"
	"tns65/internal/providercfg"
	"tns65/internal/providerroute"
	"tns65/internal/workshop"
)

// composeOK — валидный выход роли compose: композицию задаёт код, слот
// возвращает пустые поля (T-10-12, шаг 4).
const composeOK = `{"family_primary":"","pattern_secondary":"","technique":"","moral_stance":"","inversion":false}`

// fakeRouteSlot — маршрутный слот подставного провайдера: помнит промпты и
// отдаёт заданные чанки одним потоком.
type fakeRouteSlot struct {
	prompts []string
	chunks  []string
}

func (s *fakeRouteSlot) Stream(_ context.Context, prompt string) (<-chan string, error) {
	s.prompts = append(s.prompts, prompt)
	ch := make(chan string, len(s.chunks))
	for _, c := range s.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

// TestRoutedWorkshopSlotOneRequestOneModel — кусок уходит одному провайдеру
// целиком: поток одной модели собирается в строку, промпт роли не меняется,
// роль в маршрут не уходит.
func TestRoutedWorkshopSlotOneRequestOneModel(t *testing.T) {
	routes := &fakeRouteSlot{chunks: []string{"{", `"edits"`, ":", "[]", "}"}}
	slot := workshop.NewRoutedWorkshopSlot(routes)

	prompt := `{"task":"compose","chunk":"текст куска"}`
	got, err := slot.Generate(context.Background(), workshop.ComposeRole, prompt)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != `{"edits":[]}` {
		t.Fatalf("ответ = %q, ожидалась склейка всех чанков одного провайдера", got)
	}
	if len(routes.prompts) != 1 {
		t.Fatalf("запросов к провайдеру = %d, ожидался один на кусок", len(routes.prompts))
	}
	if routes.prompts[0] != prompt {
		t.Fatalf("промпт маршрута = %q, ожидался промпт роли без изменений %q", routes.prompts[0], prompt)
	}
}

// allowLoopbackGate разрешает httptest-серверам пройти сетевой гейт: их хост —
// 127.0.0.1; после теста возвращается заводской белый список.
func allowLoopbackGate(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gate.yaml")
	if err := os.WriteFile(path, []byte("netgate:\n  allowed_hosts:\n    - 127.0.0.1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(gate.yaml): %v", err)
	}
	if err := netgate.Load(path); err != nil {
		t.Fatalf("netgate.Load: %v", err)
	}
	t.Cleanup(func() {
		if err := netgate.Load(filepath.Join("..", "..", "config.yaml")); err != nil {
			t.Errorf("восстановление белого списка netgate: %v", err)
		}
	})
}

// providerServer — подставной провайдер: считает вызовы и отдаёт text одним
// SSE-чанком (status 0 — 200 с чанком, иначе HTTP-ошибка).
func providerServer(calls *atomic.Int32, text string, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if status != 0 {
			http.Error(w, "refused", status)
			return
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", text)
	}))
}

// sourcesSlot собирает слот обработки источников по маршруту first,second;
// keys — строки ключей settings.toml (пусто — провайдеры без ключей).
func sourcesSlot(t *testing.T, firstURL, secondURL, keys string) *workshop.RoutedWorkshopSlot {
	t.Helper()
	dir := t.TempDir()
	providersPath := filepath.Join(dir, "providers.toml")
	providers := fmt.Sprintf(`
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
	if err := os.WriteFile(providersPath, []byte(providers), 0o644); err != nil {
		t.Fatalf("WriteFile(providers.toml): %v", err)
	}
	settingsPath := filepath.Join(dir, "settings.toml")
	settings := keys + "\nroute_sources = \"first,second\"\n"
	if err := os.WriteFile(settingsPath, []byte(settings), 0o644); err != nil {
		t.Fatalf("WriteFile(settings.toml): %v", err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("slots:\n  default: local\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(config.yaml): %v", err)
	}

	cfg, err := providerroute.LoadSettings(configPath, settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	router := providerroute.NewRouter(cfg, providercfg.Load(providersPath), nil)
	return workshop.NewRoutedWorkshopSlot(router.Slot(providerroute.TaskSources))
}

// TestRoutedWorkshopSlotFailover — критерий Э3: отказ провайдера до первого
// чанка уводит запрос следующему, ротация идёт между запросами.
func TestRoutedWorkshopSlotFailover(t *testing.T) {
	allowLoopbackGate(t)
	var firstCalls, secondCalls atomic.Int32
	first := providerServer(&firstCalls, "", http.StatusInternalServerError)
	defer first.Close()
	second := providerServer(&secondCalls, composeOK, 0)
	defer second.Close()

	slot := sourcesSlot(t, first.URL+"/v1", second.URL+"/v1",
		"first = \"sk-first\"\nsecond = \"sk-second\"")
	got, err := slot.Generate(context.Background(), workshop.ComposeRole, "промпт куска")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != composeOK {
		t.Fatalf("ответ = %q, ожидался ответ второго провайдера", got)
	}
	if firstCalls.Load() != 1 || secondCalls.Load() != 1 {
		t.Fatalf("вызовы first/second = %d/%d, ожидалось 1/1: отказ first → ответ second",
			firstCalls.Load(), secondCalls.Load())
	}

	// Следующий запрос (следующий кусок) идёт следующему провайдеру по кругу.
	if _, err := slot.Generate(context.Background(), workshop.ComposeRole, "промпт куска"); err != nil {
		t.Fatalf("второй Generate: %v", err)
	}
	if firstCalls.Load() != 1 || secondCalls.Load() != 2 {
		t.Fatalf("после второго запроса first/second = %d/%d, ожидалось 1/2: ротация по кругу",
			firstCalls.Load(), secondCalls.Load())
	}
}

// TestRoutedWorkshopSlotNoKeys — ключей нет: слот подключён, но запрос сразу
// деградирует, провайдеры не вызываются.
func TestRoutedWorkshopSlotNoKeys(t *testing.T) {
	var firstCalls, secondCalls atomic.Int32
	first := providerServer(&firstCalls, composeOK, 0)
	defer first.Close()
	second := providerServer(&secondCalls, composeOK, 0)
	defer second.Close()

	slot := sourcesSlot(t, first.URL+"/v1", second.URL+"/v1", "")
	if _, err := slot.Generate(context.Background(), workshop.ComposeRole, "промпт"); !errors.Is(err, providerroute.ErrNoProvider) {
		t.Fatalf("ошибка = %v, ожидался ErrNoProvider", err)
	}
	if firstCalls.Load() != 0 || secondCalls.Load() != 0 {
		t.Fatal("провайдер без ключа вызван")
	}
}

// TestForgeBookThroughSourcesRoute — критерий Э3: варка книги идёт по маршруту
// sources; отказ первого провайдера уводит запрос следующему, каждый кусок —
// один запрос одной модели.
func TestForgeBookThroughSourcesRoute(t *testing.T) {
	allowLoopbackGate(t)
	var firstCalls, secondCalls atomic.Int32
	first := providerServer(&firstCalls, "", http.StatusInternalServerError)
	defer first.Close()
	second := providerServer(&secondCalls, composeOK, 0)
	defer second.Close()

	parsed, err := workshop.ParseBookInput(castFixture)
	if err != nil {
		t.Fatalf("ParseBookInput(cast.fb2): %v", err)
	}
	chunks := capTestChunks(t, parsed.Chunks)
	input := writeChunksTXT(t, chunks)

	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world.db")
	litPath := filepath.Join(dir, "literary.db")
	newDB(t, worldPath, "world", "shared")
	newDB(t, litPath, "literary")
	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer lit.Close()

	workshop.SetCensor(passCensor{})
	defer workshop.SetCensor(nil)
	workshop.SetSlot(sourcesSlot(t, first.URL+"/v1", second.URL+"/v1",
		"first = \"sk-first\"\nsecond = \"sk-second\""))
	defer workshop.SetSlot(nil)

	res, err := workshop.RunForgeBook(context.Background(), world, lit, input, workshop.ForgeBookOptions{
		Families: composeFamilies(),
	})
	if err != nil {
		t.Fatalf("RunForgeBook: %v", err)
	}
	if res.Chunks != len(chunks) {
		t.Fatalf("чанков = %d, want %d", res.Chunks, len(chunks))
	}
	if got := countBankPool(t, lit); got != len(chunks) {
		t.Fatalf("записей bank_pool = %d, want %d", got, len(chunks))
	}
	if firstCalls.Load() < 1 {
		t.Fatal("отказавший первый провайдер не вызывался: откат не проверен")
	}
	if got := secondCalls.Load(); got != int32(len(chunks)) {
		t.Fatalf("запросов к ответившему провайдеру = %d, want %d (один на кусок)",
			got, len(chunks))
	}
}

// TestForgeBookWithoutKeys — ключей нет: ковка не падает, композиция берётся
// кодом (family-only), записи банка на месте — деградация как без модели.
func TestForgeBookWithoutKeys(t *testing.T) {
	var firstCalls, secondCalls atomic.Int32
	first := providerServer(&firstCalls, composeOK, 0)
	defer first.Close()
	second := providerServer(&secondCalls, composeOK, 0)
	defer second.Close()

	parsed, err := workshop.ParseBookInput(castFixture)
	if err != nil {
		t.Fatalf("ParseBookInput(cast.fb2): %v", err)
	}
	chunks := capTestChunks(t, parsed.Chunks)
	input := writeChunksTXT(t, chunks)

	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world.db")
	litPath := filepath.Join(dir, "literary.db")
	newDB(t, worldPath, "world", "shared")
	newDB(t, litPath, "literary")
	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer lit.Close()

	workshop.SetCensor(passCensor{})
	defer workshop.SetCensor(nil)
	workshop.SetSlot(sourcesSlot(t, first.URL+"/v1", second.URL+"/v1", ""))
	defer workshop.SetSlot(nil)

	res, err := workshop.RunForgeBook(context.Background(), world, lit, input, workshop.ForgeBookOptions{
		Families: composeFamilies(),
	})
	if err != nil {
		t.Fatalf("RunForgeBook без ключей: %v", err)
	}
	if res.Chunks != len(chunks) {
		t.Fatalf("чанков = %d, want %d", res.Chunks, len(chunks))
	}
	if got := countBankPool(t, lit); got != len(chunks) {
		t.Fatalf("записей bank_pool = %d, want %d", got, len(chunks))
	}
	if firstCalls.Load() != 0 || secondCalls.Load() != 0 {
		t.Fatal("провайдер без ключа вызван при варке книги")
	}
}
