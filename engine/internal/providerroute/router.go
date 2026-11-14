package providerroute

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"tns65/internal/journal"
	"tns65/internal/models"
	"tns65/internal/netgate"
	"tns65/internal/providercfg"
	"tns65/internal/turn"
)

// QuarantineWindow — окно карантина провайдера после 429: круг ротации не
// возвращается к лимитированному провайдеру следующим же запросом.
const QuarantineWindow = 10 * time.Minute

// ErrNoProvider — в маршруте задачи нет доступного провайдера: все без ключа
// или в карантине. Ход идёт существующей деградацией (slot.degraded).
var ErrNoProvider = errors.New("providerroute: нет доступного провайдера")

// ErrExhausted — все провайдеры маршрута отказали до первого чанка.
var ErrExhausted = errors.New("providerroute: все провайдеры отказали")

// ProviderClient — клиент провайдера с метаданными журнала вызовов (FIX-4):
// поток чанков, имя провайдера и остаток лимита из ответа. models.Weaver,
// models.ProviderClient и models.OpenAIClient удовлетворяют контракту.
type ProviderClient interface {
	Stream(ctx context.Context, prompt string) (<-chan string, error)
	ProviderName() string
	LimitRemaining() string
}

// ClientFactory строит клиента провайдера. Модель и ключ уже прочитаны на
// старте; клиент кэшируется роутером по имени.
type ClientFactory func(name string) (ProviderClient, error)

// Router — маршруты задач, курсоры ротации, карантин и кэш клиентов. Курсоры
// живут в памяти процесса и сбрасываются при перезапуске.
type Router struct {
	routes  map[Task]Route
	keys    map[string]string
	factory ClientFactory
	logger  *journal.Logger
	now     func() time.Time

	mu       sync.Mutex
	calls    map[Task]uint64
	down     map[string]time.Time // провайдер на карантине до момента.
	rejected map[string]bool      // ключ отвергнут (401/403) до перезапуска.
	clients  map[string]ProviderClient
}

// NewRouter собирает роутер по снимку настроек: маршруты задач с запасными
// путями и клиенты на startup-значениях ключей и моделей. Имена, которых нет
// в реестре providers.toml, отбрасываются.
func NewRouter(cfg Settings, cat providercfg.Config, logger *journal.Logger) *Router {
	r := &Router{
		routes:   make(map[Task]Route, len(Tasks())),
		keys:     cfg.Keys,
		factory:  slotClientFactory(cat, cfg.Keys),
		logger:   logger,
		now:      time.Now,
		calls:    make(map[Task]uint64),
		down:     make(map[string]time.Time),
		rejected: make(map[string]bool),
		clients:  make(map[string]ProviderClient),
	}
	for _, task := range Tasks() {
		r.routes[task] = knownRoute(cfg.RouteFor(task), cat)
	}
	return r
}

// Slot возвращает слот прозы задачи: на каждый Stream роутер выбирает
// провайдера по кругу, ошибка до первого чанка уходит следующему.
func (r *Router) Slot(task Task) *RoutedProseSlot {
	return &RoutedProseSlot{router: r, task: task}
}

// knownRoute отбрасывает имена, которых нет в реестре providers.toml: проверка
// имён при сохранении — задача API маршрутов, на старте такое имя не должно
// превращаться в клиента.
func knownRoute(route Route, cat providercfg.Config) Route {
	out := make(Route, 0, len(route))
	for _, name := range route {
		if name == LocalProvider {
			out = append(out, name)
			continue
		}
		if _, ok := cat.Provider(name); ok {
			out = append(out, name)
		}
	}
	return out
}

// slotClientFactory собирает клиента через turn.ProviderSlot: local — локальная
// запаска :5001, внешние — Gemini generateContent или OpenAI-совместимый вход.
// Модель — выбранная в settings.toml, иначе заводская из providers.toml.
func slotClientFactory(cat providercfg.Config, keys map[string]string) ClientFactory {
	return func(name string) (ProviderClient, error) {
		def := providercfg.Provider{Name: LocalProvider, Protocol: providercfg.ProtocolLocal}
		if name != LocalProvider {
			found, ok := cat.Provider(name)
			if !ok {
				return nil, fmt.Errorf("providerroute: провайдер %q не найден в providers.toml", name)
			}
			def = found
		}
		model := strings.TrimSpace(keys[models.ModelSettingsKey(models.Provider(name))])
		slot, err := turn.ProviderSlot(def, keys[name], model)
		if err != nil {
			return nil, err
		}
		client, ok := slot.(ProviderClient)
		if !ok {
			return nil, fmt.Errorf("providerroute: провайдер %q не отдаёт метаданные вызова", name)
		}
		return client, nil
	}
}

// candidate — попытка вызова: имя провайдера и его место в маршруте.
type candidate struct {
	name  string
	index int
}

// candidates возвращает порядок попыток на один запрос: первый провайдер по
// кругу (calls(task) % len(alive)), затем следующие по списку. Пусто — ни
// одного доступного провайдера.
func (r *Router) candidates(task Task) []candidate {
	r.mu.Lock()
	defer r.mu.Unlock()
	var alive []candidate
	for i, name := range r.routes[task] {
		if r.aliveLocked(name) {
			alive = append(alive, candidate{name: name, index: i})
		}
	}
	if len(alive) == 0 {
		return nil
	}
	start := int(r.calls[task] % uint64(len(alive)))
	out := make([]candidate, 0, len(alive))
	for i := range alive {
		out = append(out, alive[(start+i)%len(alive)])
	}
	return out
}

// aliveLocked — провайдер участвует в круге: есть ключ (local — всегда),
// не в карантине и не отверг ключ.
func (r *Router) aliveLocked(name string) bool {
	if r.rejected[name] {
		return false
	}
	if until, ok := r.down[name]; ok && r.now().Before(until) {
		return false
	}
	if name == LocalProvider {
		return true
	}
	return strings.TrimSpace(r.keys[name]) != ""
}

// delivered двигает курсор задачи: счётчик растёт только по доставленным
// запросам, неудачная попытка курсор не двигает.
func (r *Router) delivered(task Task) {
	r.mu.Lock()
	r.calls[task]++
	r.mu.Unlock()
}

// markFailure заносит отказ в состояние роутера: 429 — карантин на окно
// отката (Retry-After провайдера главнее окна), отказ ключа — выбывание до
// перезапуска. Сеть и тайм-аут карантина не дают.
func (r *Router) markFailure(name string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case errors.Is(err, models.ErrProviderKeyRejected):
		r.rejected[name] = true
	case httpStatus(err) == http.StatusTooManyRequests:
		window := QuarantineWindow
		if ra := retryAfter(err); ra > 0 {
			window = ra
		}
		r.down[name] = r.now().Add(window)
	}
}

// client отдаёт клиента из кэша или строит нового.
func (r *Router) client(name string) (ProviderClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.clients[name]; ok {
		return c, nil
	}
	c, err := r.factory(name)
	if err != nil {
		return nil, err
	}
	r.clients[name] = c
	return c, nil
}

// log пишет событие маршрутизации; журнал не подключён — события не пишутся.
func (r *Router) log(event string, fields ...journal.Field) {
	if r.logger == nil {
		return
	}
	_ = r.logger.Event("providerroute", event, fields...)
}

// logTask пишет событие с полем задачи.
func (r *Router) logTask(event string, task Task, fields ...journal.Field) {
	r.log(event, append([]journal.Field{{Key: "task", Value: string(task)}}, fields...)...)
}

// statusPattern — клиенты models оформляют HTTP-отказ текстом `...: status 429`;
// отдельного типа ошибки со статусом нет, поэтому статус читается из текста.
var statusPattern = regexp.MustCompile(`status (\d{3})`)

// httpStatus достаёт HTTP-статус из ошибки провайдера; 0 — статуса нет.
func httpStatus(err error) int {
	if err == nil {
		return 0
	}
	m := statusPattern.FindStringSubmatch(err.Error())
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// retryAfterError — ошибка провайдера с заголовком Retry-After.
type retryAfterError interface {
	RetryAfter() time.Duration
}

// retryAfter — окно из заголовка Retry-After, если ошибка его несёт.
func retryAfter(err error) time.Duration {
	var ra retryAfterError
	if errors.As(err, &ra) {
		return ra.RetryAfter()
	}
	return 0
}

// failoverAllowed решает, уходит ли запрос следующему провайдеру: сеть,
// тайм-аут, 429, 5xx, отказ ключа и запрет хоста. 400 и прочие 4xx не
// откатываются — повтор другому провайдеру дал бы второй платный запрос.
func failoverAllowed(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, netgate.ErrDeniedHost) ||
		errors.Is(err, models.ErrNetwork) ||
		errors.Is(err, models.ErrProviderKeyRejected) {
		return true
	}
	status := httpStatus(err)
	return status == http.StatusTooManyRequests || status >= 500
}
