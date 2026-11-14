package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"tns65/internal/providerroute"
	"tns65/internal/textlimit"
)

// routesGet — чтение маршрутов через хэндл GET /api/providers/routes.
func routesGet(t *testing.T) providerRoutesResponse {
	t.Helper()
	rr := mcpRequest(t, handleProviderRoutesGet, http.MethodGet, settingsRoutesPath, nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", settingsRoutesPath, rr.Code)
	}
	var resp providerRoutesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal routes: %v", err)
	}
	return resp
}

// routesPut — запись маршрута через хэндл PUT /api/providers/routes.
func routesPut(t *testing.T, task string, providers []string) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"task": task, "providers": providers})
	if err != nil {
		t.Fatal(err)
	}
	rr := mcpRequest(t, handleProviderRoutesPut, http.MethodPut, settingsRoutesPath, raw, "application/json")
	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	return rr.Code, body
}

func routeNames(route providerroute.Route) string {
	return strings.Join(route, ",")
}

// TestProviderRoutesAPI — «Проверка» Э2: списки на каждую задачу читаются,
// сохраняются в settings.toml и переживают повторную загрузку конфига
// (перезапуск ядра), негодные значения отклоняются с 400.
func TestProviderRoutesAPI(t *testing.T) {
	seedSettingsWorld(t)

	resp := routesGet(t)
	if len(resp.Routes) != len(providerroute.Tasks()) {
		t.Fatalf("задач в ответе = %d, want %d", len(resp.Routes), len(providerroute.Tasks()))
	}
	for _, task := range providerroute.Tasks() {
		if providers, ok := resp.Routes[string(task)]; !ok || len(providers) != 0 {
			t.Fatalf("routes[%s] = %v, want пустой список", task, providers)
		}
	}
	if len(resp.Warnings) != 0 {
		t.Fatalf("Warnings = %v, want пусто", resp.Warnings)
	}

	code, body := routesPut(t, string(providerroute.TaskChat), []string{"gemini-flash", "gemini-pro", "local"})
	if code != http.StatusOK {
		t.Fatalf("PUT chat: status = %d, body = %v", code, body)
	}
	if body["restart_required"] != true {
		t.Fatalf("PUT chat: restart_required = %v, want true", body["restart_required"])
	}

	keys := readSettingsKeys(settingsTOMLPath)
	if got, want := keys["route_chat"], "gemini-flash, gemini-pro, local"; got != want {
		t.Fatalf("route_chat = %q, want %q", got, want)
	}

	// Повторная загрузка конфига — то, что делает ядро при перезапуске.
	cfg, err := providerroute.LoadSettings(settingsConfigPath, settingsTOMLPath)
	if err != nil {
		t.Fatalf("перезагрузка конфига: %v", err)
	}
	if got := routeNames(cfg.Routes[providerroute.TaskChat]); got != "gemini-flash,gemini-pro,local" {
		t.Fatalf("маршрут чата после перезагрузки = %q", got)
	}

	if code, body = routesPut(t, string(providerroute.TaskSources), []string{"gemini-pro"}); code != http.StatusOK {
		t.Fatalf("PUT sources: status = %d, body = %v", code, body)
	}

	resp = routesGet(t)
	if got := routeNames(resp.Routes[string(providerroute.TaskChat)]); got != "gemini-flash,gemini-pro,local" {
		t.Fatalf("GET chat = %q", got)
	}
	if got := routeNames(resp.Routes[string(providerroute.TaskSources)]); got != "gemini-pro" {
		t.Fatalf("GET sources = %q", got)
	}

	cases := []struct {
		name      string
		task      string
		providers []string
	}{
		{"unknown provider", string(providerroute.TaskEditor), []string{"ghost-provider"}},
		{"duplicate", string(providerroute.TaskEditor), []string{"gemini-flash", "gemini-flash"}},
		{"local in middle", string(providerroute.TaskEditor), []string{"gemini-flash", "local", "gemini-pro"}},
		{"unknown task", "unknown", []string{"gemini-flash"}},
	}
	for _, tc := range cases {
		if code, body = routesPut(t, tc.task, tc.providers); code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, body = %v, want 400", tc.name, code, body)
		}
	}
	if keys = readSettingsKeys(settingsTOMLPath); keys["route_editor"] != "" {
		t.Fatalf("route_editor записан после ошибки: %q", keys["route_editor"])
	}

	// Пустой список — «не задан»: задача уходит на запасной путь.
	if code, body = routesPut(t, string(providerroute.TaskSources), nil); code != http.StatusOK {
		t.Fatalf("PUT sources empty: status = %d, body = %v", code, body)
	}
	resp = routesGet(t)
	if got := resp.Routes[string(providerroute.TaskSources)]; len(got) != 0 {
		t.Fatalf("GET sources после очистки = %v, want пусто", got)
	}
}

// TestProviderRoutesWarnings — сохранённый в config.yaml маршрут с неизвестным
// провайдером и local не последним читается как записан, но с предупреждениями.
func TestProviderRoutesWarnings(t *testing.T) {
	seedSettingsWorld(t)
	body := "routes:\n  chat: \"gemini-flash, local, ghost\"\n  sources: \"ghost, gemini-pro\"\n"
	if err := os.WriteFile(settingsConfigPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	resp := routesGet(t)
	if got := routeNames(resp.Routes[string(providerroute.TaskChat)]); got != "gemini-flash,local,ghost" {
		t.Fatalf("GET chat = %q, want сохранённый порядок", got)
	}
	if len(resp.Warnings) != 3 {
		t.Fatalf("Warnings = %v, want 3", resp.Warnings)
	}
	joined := strings.Join(resp.Warnings, "\n")
	for _, want := range []string{"ghost", "local должен быть последним"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("Warnings = %v, нет %q", resp.Warnings, want)
		}
	}
}

// TestRouteTablePaths — таблица ROUTES.md не расходится с роутером: у каждой
// строки находится маршрут по методу и пути, устаревших путей нет. Шаблон
// `{id}` проходит по префиксу-каталогу — так его матчит pathMatch.
func TestRouteTablePaths(t *testing.T) {
	data, err := os.ReadFile("ROUTES.md")
	if err != nil {
		t.Fatal(err)
	}
	routes := routeLines(NewHub())
	rows := 0
	for _, line := range strings.Split(string(data), "\n") {
		method, path, ok := routeTableRow(line)
		if !ok {
			continue
		}
		rows++
		if !routeDocumented(method, path, routes) {
			t.Errorf("ROUTES.md: маршрута %s %s нет в роутере", method, path)
		}
	}
	if rows == 0 {
		t.Fatal("ROUTES.md: не найдено ни одной строки таблицы маршрутов")
	}
}

// routeTableRow разбирает строку таблицы маршрутов: метод и путь. Заголовок
// таблицы, разделитель и прочие строки дают ok=false.
func routeTableRow(line string) (method, path string, ok bool) {
	cells := strings.Split(line, "|")
	if len(cells) < 3 {
		return "", "", false
	}
	method = strings.TrimSpace(cells[1])
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete:
	default:
		return "", "", false
	}
	path = strings.Trim(strings.TrimSpace(cells[2]), "`")
	if !strings.HasPrefix(path, "/") {
		return "", "", false
	}
	return method, path, true
}

// routeDocumented — маршрут роутера подходит под строку таблицы: тот же метод и
// дословный путь либо путь-каталог с «/» на конце (шаблон `{id}` в документе).
func routeDocumented(method, path string, routes []string) bool {
	for _, r := range routes {
		m, p, ok := strings.Cut(r, " ")
		if !ok || m != method {
			continue
		}
		if p == path || (strings.HasSuffix(p, "/") && strings.HasPrefix(path, p)) {
			return true
		}
	}
	return false
}

// TestRouteTableLimits — числовое зеркало IMP-7: потолок вводимых текстов в
// ROUTES.md берётся из textlimit.DefaultLimit, отдельной цифры в документе нет.
func TestRouteTableLimits(t *testing.T) {
	data, err := os.ReadFile("ROUTES.md")
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%d токенов", textlimit.DefaultLimit)
	rows := []string{
		"| POST | `/api/turn` |",
		"| POST | `/api/atlas/worlds/birth` |",
		"| POST | `/api/atlas/worlds/replay` |",
	}
	for _, prefix := range rows {
		line := ""
		for _, l := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(l, prefix) {
				line = l
				break
			}
		}
		if line == "" {
			t.Fatalf("ROUTES.md: нет строки %s", prefix)
		}
		if !strings.Contains(line, want) {
			t.Errorf("ROUTES.md: строка %q без потолка «%s»", prefix, want)
		}
	}
}
