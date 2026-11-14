package providerroute

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись %s: %v", name, err)
	}
	return path
}

func TestParseRoute(t *testing.T) {
	got := ParseRoute(` "alpha", beta ,, alpha, "gamma" `)
	want := Route{"alpha", "beta", "gamma"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseRoute = %v, ожидалось %v", got, want)
	}
	if empty := ParseRoute("  ,  "); len(empty) != 0 {
		t.Fatalf("пустая строка дала маршрут %v", empty)
	}
}

func TestLoadSettingsRoutes(t *testing.T) {
	dir := t.TempDir()
	configPath := writeTemp(t, dir, "config.yaml", `
slots:
  default: gemini-flash
routes:
  chat: "deepseek, mimo"
  sources: [qwen, "deepseek"]
  scout_real: ""
`)
	settingsPath := writeTemp(t, dir, "settings.toml", `
deepseek = "sk-deepseek"
deepseek_model = "deepseek-chat"
mimo = "sk-mimo"
route_chat = "mimo,deepseek"
route_scout_real = "gemini-flash"
`)

	cfg, err := LoadSettings(configPath, settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if got, want := cfg.Routes[TaskChat], (Route{"mimo", "deepseek"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("route_chat = %v, ожидалось %v (settings.toml главнее config.yaml)", got, want)
	}
	if got, want := cfg.Routes[TaskSources], (Route{"qwen", "deepseek"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("route_sources = %v, ожидалось %v", got, want)
	}
	if got, want := cfg.Routes[TaskScoutReal], (Route{"gemini-flash"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("route_scout_real = %v, ожидалось %v", got, want)
	}
	if cfg.Default != "gemini-flash" {
		t.Fatalf("slots.default = %q, ожидалось gemini-flash", cfg.Default)
	}
	if cfg.Keys["deepseek"] != "sk-deepseek" || cfg.Keys["mimo"] != "sk-mimo" {
		t.Fatalf("ключи провайдеров прочитаны неверно: %v", cfg.Keys)
	}
}

func TestLoadSettingsMissingSettings(t *testing.T) {
	dir := t.TempDir()
	configPath := writeTemp(t, dir, "config.yaml", "slots:\n  default: local\n")

	cfg, err := LoadSettings(configPath, filepath.Join(dir, "нет-settings.toml"))
	if err != nil {
		t.Fatalf("LoadSettings без settings.toml: %v", err)
	}
	if len(cfg.Keys) != 0 {
		t.Fatalf("ключи без settings.toml: %v", cfg.Keys)
	}
	if got := cfg.RouteFor(TaskChat); !reflect.DeepEqual(got, Route{"local"}) {
		t.Fatalf("route_chat без настроек = %v, ожидалось [local]", got)
	}
}

func TestLoadSettingsMissingConfig(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadSettings(filepath.Join(dir, "нет-config.yaml"), filepath.Join(dir, "settings.toml")); err == nil {
		t.Fatal("LoadSettings без config.yaml не вернул ошибку")
	}
}

func TestRouteForFallbacks(t *testing.T) {
	cfg := Settings{
		Routes: map[Task]Route{
			TaskChat:      {"local", "deepseek"},
			TaskSources:   {"qwen", "local"},
			TaskScoutReal: {"gemini-flash"},
		},
		Default: "gemini-flash",
	}
	cases := []struct {
		task Task
		want Route
	}{
		{TaskChat, Route{"deepseek", "local"}},   // local переставлен в конец.
		{TaskSources, Route{"qwen", "local"}},    // явный маршрут как задан.
		{TaskEditor, Route{"gemini-flash"}},      // пусто — slots.default.
		{TaskSourcesMap, Route{"qwen", "local"}}, // пусто — route_sources.
		{TaskScoutReal, Route{"gemini-flash"}},   // явный маршрут разведки.
		{TaskScoutFiction, nil},                  // разведка без маршрута выключена.
	}
	for _, c := range cases {
		if got := cfg.RouteFor(c.task); !reflect.DeepEqual(got, c.want) {
			t.Errorf("RouteFor(%s) = %v, ожидалось %v", c.task, got, c.want)
		}
	}

	empty := Settings{}
	if got := empty.RouteFor(TaskSourcesMap); len(got) != 0 {
		t.Fatalf("sources_map без маршрутов = %v, ожидалось пусто", got)
	}
}

func TestShippedConfigRoutes(t *testing.T) {
	path := filepath.Join("..", "..", "config.yaml")
	cfg, err := LoadSettings(path, filepath.Join(t.TempDir(), "settings.toml"))
	if err != nil {
		t.Fatalf("LoadSettings(engine/config.yaml): %v", err)
	}
	for _, task := range Tasks() {
		want := Route{"local"}
		switch task {
		case TaskScoutReal, TaskScoutFiction, TaskScoutMixed:
			want = nil // разведка без маршрута выключена.
		}
		if got := cfg.RouteFor(task); !reflect.DeepEqual(got, want) {
			t.Errorf("заводской RouteFor(%s) = %v, ожидалось %v", task, got, want)
		}
	}
}
