package paganel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/models"
	"tns65/internal/providercfg"
	"tns65/internal/providerroute"
)

// ScoutTask — задача маршрута по режиму походов (Э5, §6.4, §13.3): реальный мир
// — scout_real (Летописец), вымышленный — scout_fiction (Картограф вымысла),
// смешанный — scout_mixed (Смешанный/Штурман).
func ScoutTask(mode TravelMode) (providerroute.Task, error) {
	switch mode {
	case TravelReal:
		return providerroute.TaskScoutReal, nil
	case TravelFiction:
		return providerroute.TaskScoutFiction, nil
	case TravelMixed:
		return providerroute.TaskScoutMixed, nil
	}
	return "", fmt.Errorf("paganel: режим походов %q вне списка реальная|вымышленная|смешанная", string(mode))
}

// ErrScoutNotRouted — маршрут разведки режима не подключён: список пуст или в
// нём нет провайдера с ключом. Вызов пропускается, ход не блокируется
// (W-10.3, W-10.4).
var ErrScoutNotRouted = errors.New("paganel: маршрут разведки режима не подключён")

// RoutedScout — разведка пути в игре по маршруту режима походов (Э5, §6.4):
// режим выбирает задачу маршрута, внутри задачи работает обычный роутер
// провайдеров — ротация, откат до первого чанка, карантин 429. Каждый режим
// получает свой DeepScout: бюджет и тайм-аут общие из config.yaml, журналы
// вызовов и станка — как в DEC-4 (B-14.4).
type RoutedScout struct {
	scouts map[TravelMode]*DeepScout
}

// Routed — режим подключён: маршрут задачи собран и есть провайдер с ключом.
func (r *RoutedScout) Routed(mode TravelMode) bool {
	if r == nil {
		return false
	}
	return r.scouts[mode] != nil
}

// Run выполняет разведку по темам в рамках режима походов: бюджет и тайм-аут
// берутся из настроек разведки, отказ одного вызова темы не прерывает.
// Режим не подключён — ErrScoutNotRouted: пропуск, ход не блокируется.
func (r *RoutedScout) Run(ctx context.Context, mode TravelMode, topics []string) (string, error) {
	if r == nil || r.scouts[mode] == nil {
		return "", ErrScoutNotRouted
	}
	return r.scouts[mode].Run(ctx, topics)
}

// Fetch — прицельный вызов разведки в игре по режиму походов мира h (замена
// заглушки Scout.Fetch, §6.4): режим читается из world_meta, маршрут
// выбирается по нему. Отказ чтения режима или пропуск маршрута возвращается
// вызывающему, ход от него не зависит (W-10.3).
func (r *RoutedScout) Fetch(ctx context.Context, h *db.Handle, topic string) (string, error) {
	mode, err := ReadTravelMode(h)
	if err != nil {
		return "", err
	}
	return r.Run(ctx, mode, []string{topic})
}

// LoadRoutedScout собирает разведку в игре по маршрутам scout_real /
// scout_fiction / scout_mixed (Э5, §6.4). Маршруты, ключи и модели читаются
// здесь один раз, на старте процесса; пустой маршрут режима заменяется
// ссылкой key_ref (ключ по умолчанию из config.yaml). Выключенный переключатель
// «походы наружу», нечитаемый конфиг или отсутствие ключей во всех режимах —
// nil: пропуск разведки не блокирует ход (W-10.4).
func LoadRoutedScout(configPath, providersPath, settingsPath, usagePath string, logger *journal.Logger) *RoutedScout {
	cfg, err := LoadPaganelConfig(configPath)
	if err != nil || !cfg.Scout.Enabled {
		return nil
	}
	settings, err := providerroute.LoadSettings(configPath, settingsPath)
	if err != nil {
		return nil
	}
	cat := providercfg.Load(providersPath)

	routes := make(map[TravelMode]providerroute.Route, 3)
	for _, mode := range []TravelMode{TravelReal, TravelFiction, TravelMixed} {
		task, err := ScoutTask(mode)
		if err != nil {
			continue
		}
		route := scoutRoute(settings, task, cfg.Scout.KeyRef, cat)
		if len(route) == 0 || !routeHasKey(route, settings.Keys) {
			continue
		}
		// Запасной путь key_ref идёт через тот же роутер: маршрут задачи
		// остаётся единственным источником провайдеров.
		settings.Routes[task] = route
		routes[mode] = route
	}
	if len(routes) == 0 {
		return nil
	}

	router := providerroute.NewRouter(settings, cat, logger)
	out := &RoutedScout{scouts: make(map[TravelMode]*DeepScout, len(routes))}
	for mode, route := range routes {
		task, _ := ScoutTask(mode)
		out.scouts[mode] = NewDeepScout(DeepScoutConfig{
			Provider:  route[0],
			Endpoint:  routeEndpoint(cat, route),
			Budget:    cfg.Scout.Budget,
			Timeout:   cfg.Scout.Timeout,
			UsagePath: usagePath,
			Logger:    logger,
		}, router.Slot(task))
	}
	return out
}

// scoutRoute — маршрут задачи с отбрасыванием имён вне реестра providers.toml и
// запасным путём key_ref, когда список пуст (§6.4: key_ref — ключ по умолчанию).
func scoutRoute(settings providerroute.Settings, task providerroute.Task, keyRef string, cat providercfg.Config) providerroute.Route {
	route := settings.RouteFor(task)
	if len(route) == 0 {
		if name := strings.TrimSpace(keyRef); name != "" {
			route = providerroute.Route{name}
		}
	}
	return knownScoutRoute(route, cat)
}

// knownScoutRoute оставляет имена реестра providers.toml и local; прочие
// отбрасываются — на старте такое имя не должно превращаться в клиента.
func knownScoutRoute(route providerroute.Route, cat providercfg.Config) providerroute.Route {
	out := make(providerroute.Route, 0, len(route))
	for _, name := range route {
		if name == providerroute.LocalProvider {
			out = append(out, name)
			continue
		}
		if _, ok := cat.Provider(name); ok {
			out = append(out, name)
		}
	}
	return out
}

// routeHasKey — в маршруте есть провайдер с ключом (local ключа не требует).
func routeHasKey(route providerroute.Route, keys map[string]string) bool {
	for _, name := range route {
		if name == providerroute.LocalProvider || strings.TrimSpace(keys[name]) != "" {
			return true
		}
	}
	return false
}

// routeEndpoint — адрес первого провайдера маршрута для журнала вызовов:
// фактического провайдера вызова отдаёт слот роутера (ProviderName).
func routeEndpoint(cat providercfg.Config, route providerroute.Route) string {
	if len(route) == 0 {
		return ""
	}
	if route[0] == providerroute.LocalProvider {
		return models.DefaultWeaverURL
	}
	if def, ok := cat.Provider(route[0]); ok {
		return def.BaseURL
	}
	return ""
}
