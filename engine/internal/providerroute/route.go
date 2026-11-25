// Package providerroute — маршруты провайдеров генерации по задачам:
// упорядоченный список имён на задачу, выбор провайдера на каждый Stream,
// откат на следующего при ошибке до первого чанка. Маршруты, ключи и модели
// читаются один раз на старте ядра; изменение применяется после перезапуска.
package providerroute

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"

	"tns65/internal/llmparams"
	"tns65/internal/models"
)

// Task — задача маршрутизации: у каждой задачи свой список провайдеров.
type Task string

const (
	TaskChat         Task = "chat"          // Ткач, живой ход.
	TaskSources      Task = "sources"       // Разбор и варка книги-источника.
	TaskSourcesMap   Task = "sources_map"   // Карта книги-источника отдельным проходом.
	TaskEditor       Task = "editor"        // Литературная обработка готовой главы.
	TaskScoutReal    Task = "scout_real"    // Разведка: реальный мир.
	TaskScoutFiction Task = "scout_fiction" // Разведка: вымышленный мир.
	TaskScoutMixed   Task = "scout_mixed"   // Разведка: смешанный мир.
)

// LocalProvider — локальная запаска :5001: ключа не требует, в цепочке идёт
// последней (решение владельца, план PROVIDER-ROUTING §13.1).
const LocalProvider = "local"

// Tasks — задачи в порядке интерфейса.
func Tasks() []Task {
	return []Task{TaskChat, TaskSources, TaskSourcesMap, TaskEditor, TaskScoutReal, TaskScoutFiction, TaskScoutMixed}
}

// Route — упорядоченный список имён провайдеров; порядок задаёт ротацию.
type Route []string

// ParseRoute разбирает список имён через запятую: пробелы и кавычки
// отбрасываются, пустые элементы пропускаются, повтор остаётся один раз.
func ParseRoute(s string) Route {
	var out Route
	seen := make(map[string]struct{})
	for _, item := range strings.Split(s, ",") {
		name := strings.TrimSpace(strings.Trim(strings.TrimSpace(item), `"'`))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// Settings — снимок маршрутизации на старте ядра: маршруты задач, ключи и
// модели провайдеров из settings.toml и запасной провайдер slots.default.
type Settings struct {
	Routes     map[Task]Route
	Keys       map[string]string
	Default    string
	GenTimeout time.Duration // llm.gen_timeout: тайм-аут HTTP-клиента слота
}

// LoadSettings читает маршруты и ключи. Маршруты берутся из секции `routes:`
// config.yaml и перекрываются плоскими ключами `route_<задача>` settings.toml
// (их пишет интерфейс маршрутов). Ключи провайдеров и выбранные модели —
// `<имя>` и `<имя>_model` settings.toml. Ошибка чтения config.yaml — ошибка:
// без неё не собрать ни маршруты, ни запасной slots.default.
func LoadSettings(configPath, settingsPath string) (Settings, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return Settings{}, err
	}
	slots, err := models.LoadSlotsConfig(configPath)
	if err != nil {
		return Settings{}, err
	}
	keys := readSettingsKeys(settingsPath)
	routes := parseRoutesSection(string(data))
	for _, task := range Tasks() {
		if v := strings.TrimSpace(keys[routeKey(task)]); v != "" {
			routes[task] = ParseRoute(v)
		}
	}
	return Settings{
		Routes:     routes,
		Keys:       keys,
		Default:    string(slots.Default),
		GenTimeout: llmparams.LoadSettings(configPath).Timeout,
	}, nil
}

// routeKey — ключ settings.toml с маршрутом задачи.
func routeKey(task Task) string { return "route_" + string(task) }

// RouteFor возвращает маршрут задачи с запасными путями: чат, источники и
// редактор падают на slots.default, карта книги-источника — на route_sources,
// разведка без маршрута выключена. local переставляется в конец цепочки.
func (s Settings) RouteFor(task Task) Route {
	if route := normalizeRoute(s.Routes[task]); len(route) > 0 {
		return route
	}
	switch task {
	case TaskSourcesMap:
		return s.RouteFor(TaskSources)
	case TaskScoutReal, TaskScoutFiction, TaskScoutMixed:
		return nil
	}
	if s.Default == "" {
		return nil
	}
	return normalizeRoute(Route{s.Default})
}

// normalizeRoute переставляет local в конец цепочки и отбрасывает пустые имена.
func normalizeRoute(route Route) Route {
	out := make(Route, 0, len(route))
	local := false
	for _, name := range route {
		name = strings.TrimSpace(name)
		switch {
		case name == "":
			continue
		case name == LocalProvider:
			local = true
		default:
			out = append(out, name)
		}
	}
	if local {
		out = append(out, LocalProvider)
	}
	return out
}

// parseRoutesSection разбирает топ-ключ `routes:` config.yaml: задача — строка
// через запятую или список в квадратных скобках; комментарий после `#`
// отбрасывается. Неизвестные задачи пропускаются.
func parseRoutesSection(data string) map[Task]Route {
	out := make(map[Task]Route)
	inRoutes := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inRoutes = trimmed == "routes:"
			continue
		}
		if !inRoutes {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		task := Task(strings.TrimSpace(key))
		if !validTask(task) {
			continue
		}
		out[task] = parseRouteValue(value)
	}
	return out
}

// parseRouteValue разбирает значение строки маршрута: `a,b` или `[a, b]`.
func parseRouteValue(v string) Route {
	v = strings.TrimSpace(strings.SplitN(v, "#", 2)[0])
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		v = strings.TrimSuffix(strings.TrimPrefix(v, "["), "]")
	}
	return ParseRoute(v)
}

func validTask(task Task) bool {
	for _, t := range Tasks() {
		if t == task {
			return true
		}
	}
	return false
}

// readSettingsKeys читает settings.toml как top-level строки key = "value";
// файла нет — пустая карта (станок стартует, маршрут деградирует).
func readSettingsKeys(path string) map[string]string {
	out := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if unquoted, err := strconv.Unquote(val); err == nil {
			val = unquoted
		} else {
			val = strings.Trim(val, `"'`)
		}
		if key != "" {
			out[key] = val
		}
	}
	return out
}
