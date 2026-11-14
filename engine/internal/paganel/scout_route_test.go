package paganel_test

import (
	"context"
	"errors"
	"fmt"
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

// allowScoutLoopback разрешает httptest-серверу пройти сетевой гейт: его хост —
// 127.0.0.1; после теста возвращается заводской белый список config.yaml.
func allowScoutLoopback(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gate.yaml")
	if err := os.WriteFile(path, []byte("netgate:\n  allowed_hosts:\n    - 127.0.0.1\n"), 0o644); err != nil {
		t.Fatalf("запись gate.yaml: %v", err)
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

// writeScoutFile пишет файл конфигурации экземпляра.
func writeScoutFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись %s: %v", path, err)
	}
}

// scoutAnswerServer — подставной OpenAI-совместимый провайдер: считает вызовы и
// отдаёт ответ одним чанком SSE.
func scoutAnswerServer(calls *atomic.Int32, text string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", text)
	}))
}

// scoutProvidersTOML — реестр из двух OpenAI-совместимых провайдеров на адресах
// подставных серверов.
func scoutProvidersTOML(realURL, fictionURL string) string {
	return fmt.Sprintf(`
[[providers]]
name = "scout-real"
protocol = "openai"
base_url = %q
default_model = "real-1"

[[providers]]
name = "scout-fiction"
protocol = "openai"
base_url = %q
default_model = "fiction-1"
`, realURL+"/v1", fictionURL+"/v1")
}

// TestRoutedScoutTravelModeRoute — критерий §12.5/§6.4: разведка в игре идёт
// маршрутом режима походов, отвечает выбранный провайдер, бюджет и тайм-аут
// держатся из config.yaml, вызовы пишутся в журналы как в DEC-4.
func TestRoutedScoutTravelModeRoute(t *testing.T) {
	allowScoutLoopback(t)
	var realCalls, fictionCalls atomic.Int32
	realSrv := scoutAnswerServer(&realCalls, "справка реального мира")
	defer realSrv.Close()
	fictionSrv := scoutAnswerServer(&fictionCalls, "справка вымышленного мира")
	defer fictionSrv.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	providersPath := filepath.Join(dir, "providers.toml")
	settingsPath := filepath.Join(dir, "settings.toml")
	usagePath := filepath.Join(dir, "usage.jsonl")
	logPath := filepath.Join(dir, "journal.log")

	writeScoutFile(t, configPath, "routes:\n  scout_real: scout-real\n  scout_fiction: scout-fiction\n\n"+
		"paganel:\n  enabled: true\n  scout_enabled: true\n  scout_budget: 2\n  scout_timeout: 5s\n  key_ref: scout-real\n")
	writeScoutFile(t, providersPath, scoutProvidersTOML(realSrv.URL, fictionSrv.URL))
	writeScoutFile(t, settingsPath, "scout-real = \"sk-real\"\nscout-fiction = \"sk-fiction\"\n")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	scout := paganel.LoadRoutedScout(configPath, providersPath, settingsPath, usagePath, logger)
	if scout == nil {
		t.Fatal("LoadRoutedScout с маршрутами и ключами вернул nil")
	}
	if !scout.Routed(paganel.TravelReal) || !scout.Routed(paganel.TravelFiction) {
		t.Fatal("режимы с маршрутом и ключом не подключены")
	}

	// Реальный мир — маршрут scout_real: отвечает только его провайдер.
	text, err := scout.Run(context.Background(), paganel.TravelReal, []string{"география"})
	if err != nil {
		t.Fatalf("Run(реальная): %v", err)
	}
	if !strings.Contains(text, "реального мира") {
		t.Fatalf("ответ реального режима = %q", text)
	}
	if got := realCalls.Load(); got != 1 {
		t.Fatalf("вызовов провайдера scout_real = %d, want 1", got)
	}
	if got := fictionCalls.Load(); got != 0 {
		t.Fatalf("вызовов провайдера scout_fiction = %d, want 0", got)
	}

	// Режим берётся из world_meta активного мира: вымышленная походка идёт
	// маршрутом scout_fiction.
	h, _ := newCalibrateWorld(t)
	setTravelMode(t, h, string(paganel.TravelFiction))
	text, err = scout.Fetch(context.Background(), h, "эпоха")
	h.Close()
	if err != nil {
		t.Fatalf("Fetch(вымышленная): %v", err)
	}
	if !strings.Contains(text, "вымышленного мира") {
		t.Fatalf("ответ вымышленного режима = %q", text)
	}
	if got := fictionCalls.Load(); got != 1 {
		t.Fatalf("вызовов провайдера scout_fiction = %d, want 1", got)
	}

	// Пустой маршрут режима берёт key_ref по умолчанию (§6.4).
	if !scout.Routed(paganel.TravelMixed) {
		t.Fatal("смешанный режим без маршрута не ушёл на key_ref")
	}
	text, err = scout.Run(context.Background(), paganel.TravelMixed, []string{"путь"})
	if err != nil {
		t.Fatalf("Run(смешанная): %v", err)
	}
	if !strings.Contains(text, "реального мира") {
		t.Fatalf("ответ смешанного режима по key_ref = %q", text)
	}
	if got := realCalls.Load(); got != 2 {
		t.Fatalf("вызовов провайдера scout_real = %d, want 2", got)
	}

	// Бюджет режима — из config.yaml (scout_budget: 2): третья тема не уходит.
	if _, err := scout.Run(context.Background(), paganel.TravelReal, []string{"a", "b", "c"}); err != nil {
		t.Fatalf("Run(реальная, три темы): %v", err)
	}
	if got := realCalls.Load(); got != 3 {
		t.Fatalf("вызовов провайдера scout_real = %d, want 3 (бюджет 2)", got)
	}

	// Журнал вызовов — как в DEC-4: роль разведки, фактический провайдер,
	// адрес маршрута и сводка результата.
	records := models.ReadCalls(usagePath)
	if len(records) == 0 {
		t.Fatal("журнал вызовов пуст")
	}
	found := false
	for _, rec := range records {
		if rec.Role != paganel.RoleScout {
			t.Fatalf("роль записи = %q, want %q", rec.Role, paganel.RoleScout)
		}
		if rec.Provider == "scout-fiction" {
			found = true
			if rec.Endpoint != fictionSrv.URL+"/v1" {
				t.Fatalf("endpoint записи = %q, want %q", rec.Endpoint, fictionSrv.URL+"/v1")
			}
			if rec.Result == "" {
				t.Fatal("сводка результата пуста")
			}
		}
	}
	if !found {
		t.Fatalf("в журнале вызовов нет записи провайдера scout_fiction: %+v", records)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("чтение журнала: %v", err)
	}
	if !strings.Contains(string(raw), journal.EventScoutCall) {
		t.Fatalf("в журнале станка нет события %s", journal.EventScoutCall)
	}
}

// TestRoutedScoutTimeout — тайм-аут целевого вызова из config.yaml (W-10.3):
// провайдер молчит — разведка пропускает вызов, не дожидаясь его.
func TestRoutedScoutTimeout(t *testing.T) {
	allowScoutLoopback(t)
	// Молчащий провайдер: обработчик держится до закрытия сервера, заголовки не
	// отправляются, поэтому клиент ждёт ответа до тайм-аута разведки.
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-release
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	providersPath := filepath.Join(dir, "providers.toml")
	settingsPath := filepath.Join(dir, "settings.toml")
	writeScoutFile(t, configPath, "routes:\n  scout_real: scout-real\n\n"+
		"paganel:\n  scout_enabled: true\n  scout_budget: 3\n  scout_timeout: 100ms\n  key_ref: scout-real\n")
	writeScoutFile(t, providersPath, scoutProvidersTOML(srv.URL, srv.URL))
	writeScoutFile(t, settingsPath, "scout-real = \"sk-real\"\n")

	scout := paganel.LoadRoutedScout(configPath, providersPath, settingsPath, "", nil)
	if scout == nil {
		t.Fatal("LoadRoutedScout вернул nil")
	}
	started := time.Now()
	_, err := scout.Run(context.Background(), paganel.TravelReal, []string{"география"})
	if !errors.Is(err, paganel.ErrScoutTimeout) {
		t.Fatalf("Run при молчащем провайдере = %v, want ErrScoutTimeout", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("тайм-аут разведки не соблюдён: %v", elapsed)
	}
}

// TestRoutedScoutWithoutKeys — без ключей разведка пропускается: маршрут режима
// не подключён, загрузчик возвращает nil, ход не блокируется (W-10.4).
func TestRoutedScoutWithoutKeys(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	providersPath := filepath.Join(dir, "providers.toml")
	settingsPath := filepath.Join(dir, "settings.toml")

	writeScoutFile(t, providersPath, "[[providers]]\nname = \"scout-real\"\nprotocol = \"openai\"\nbase_url = \"https://api.example.com/v1\"\n")

	// Маршрут задан, ключа нет — режим не подключён, разведки нет.
	writeScoutFile(t, configPath, "routes:\n  scout_real: scout-real\n\n"+
		"paganel:\n  scout_enabled: true\n  key_ref: scout-real\n")
	writeScoutFile(t, settingsPath, "")
	if scout := paganel.LoadRoutedScout(configPath, providersPath, settingsPath, "", nil); scout != nil {
		t.Fatal("LoadRoutedScout без ключей вернул разведку")
	}

	// Ключ есть у одного режима: режим без ключа пропускается, остальные идут.
	writeScoutFile(t, configPath, "routes:\n  scout_real: scout-real\n  scout_fiction: scout-fiction\n\n"+
		"paganel:\n  scout_enabled: true\n  key_ref: scout-real\n")
	writeScoutFile(t, providersPath, "[[providers]]\nname = \"scout-real\"\nprotocol = \"openai\"\nbase_url = \"https://api.example.com/v1\"\n\n"+
		"[[providers]]\nname = \"scout-fiction\"\nprotocol = \"openai\"\nbase_url = \"https://api.example.org/v1\"\n")
	writeScoutFile(t, settingsPath, "scout-real = \"sk-real\"\n")
	scout := paganel.LoadRoutedScout(configPath, providersPath, settingsPath, "", nil)
	if scout == nil {
		t.Fatal("LoadRoutedScout с ключом одного режима вернул nil")
	}
	if !scout.Routed(paganel.TravelReal) {
		t.Fatal("режим scout_real с ключом не подключён")
	}
	if scout.Routed(paganel.TravelFiction) {
		t.Fatal("режим scout_fiction подключён без ключа")
	}
	if _, err := scout.Run(context.Background(), paganel.TravelFiction, []string{"эпоха"}); !errors.Is(err, paganel.ErrScoutNotRouted) {
		t.Fatalf("Run без маршрута = %v, want ErrScoutNotRouted", err)
	}

	// Переключатель «походы наружу» выключен — разведки нет.
	writeScoutFile(t, configPath, "routes:\n  scout_real: scout-real\n\npaganel:\n  scout_enabled: false\n")
	writeScoutFile(t, settingsPath, "scout-real = \"sk-real\"\n")
	if scout := paganel.LoadRoutedScout(configPath, providersPath, settingsPath, "", nil); scout != nil {
		t.Fatal("LoadRoutedScout при scout_enabled=false вернул разведку")
	}

	// Не подключённая разведка пропускает вызов, а не падает.
	var nilScout *paganel.RoutedScout
	if _, err := nilScout.Run(context.Background(), paganel.TravelReal, []string{"тема"}); !errors.Is(err, paganel.ErrScoutNotRouted) {
		t.Fatalf("Run у пустой разведки = %v, want ErrScoutNotRouted", err)
	}
}
