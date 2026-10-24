package supervisor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"tns65/internal/journal"
)

// DefaultReadyTimeout и DefaultPollInterval — значения по умолчанию для ребёнка,
// чьи поля не заданы в конфигурации (T-01-13, шаг 1).
const (
	DefaultReadyTimeout = 600 * time.Second
	DefaultPollInterval = 2 * time.Second
)

// Config — один резидент-ребёнок из секции children (шаг 1). Интервал и тайм-аут
// инжектируются: тесты масштабируют время, подменяя часы.
type Config struct {
	Name         string
	Cmd          string
	Args         []string
	Port         int
	HealthPath   string
	ReadyTimeout time.Duration
	PollInterval time.Duration
}

// Clock — источник времени надзирателя. Тесты подменяют реальные часы
// масштабируемыми, чтобы паузы и тайм-ауты не ждали по-настоящему.
type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

// wallClock — реальные часы.
type wallClock struct{}

func (wallClock) Now() time.Time                         { return time.Now() }
func (wallClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// ProcessRunner запускает процесс ребёнка; stdout и stderr уже подключены к
// journal.Pipe. Тесты подменяют фабрику целиком.
type ProcessRunner interface {
	Start(spec Config, stdout, stderr io.Writer) (Process, error)
}

// Process — запущенный ребёнок надзирателя.
type Process interface {
	// Wait блокируется до выхода процесса и возвращает его ошибку.
	Wait() error
	// Stop останавливает процесс (SIGTERM на unix, Kill на Windows). Идемпотентен.
	Stop() error
}

// Option настраивает Supervisor; используется тестами для инжекции часов,
// фабрики процессов и проверки готовности.
type Option func(*Supervisor)

// WithClock подменяет часы (тесты масштабируют время).
func WithClock(c Clock) Option { return func(s *Supervisor) { s.clock = c } }

// WithRunner подменяет фабрику процессов (тестовый ребёнок).
func WithRunner(r ProcessRunner) Option { return func(s *Supervisor) { s.runner = r } }

// WithReady подменяет проверку готовности (тест без живого health-эндпоинта).
func WithReady(f ReadyFunc) Option { return func(s *Supervisor) { s.ready = f } }

// Supervisor — ядро-надзиратель: запускает детей, лечит падения и останавливает
// детей в обратном порядке (T-01-13).
type Supervisor struct {
	logger *journal.Logger
	clock  Clock
	runner ProcessRunner
	ready  ReadyFunc

	specs []Config

	mu       sync.Mutex
	children []*child
	stopCh   chan struct{}
	stopped  bool
	wg       sync.WaitGroup
}

// New собирает надзирателя из спецификаций детей и применяет значения по
// умолчанию к недозаданным тайм-аутам.
func New(logger *journal.Logger, specs []Config, opts ...Option) *Supervisor {
	normalized := make([]Config, len(specs))
	for i, spec := range specs {
		normalized[i] = normalize(spec)
	}
	s := &Supervisor{
		logger: logger,
		clock:  wallClock{},
		runner: execRunner{},
		ready:  defaultReady,
		specs:  normalized,
		stopCh: make(chan struct{}),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Run запускает по горутине на ребёнка и возвращается; дети живут до Stop.
// После Stop повторный вызов — бездействие.
func (s *Supervisor) Run() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	for _, spec := range s.specs {
		s.children = append(s.children, newChild(s, spec))
	}
	children := append([]*child(nil), s.children...)
	s.mu.Unlock()

	s.wg.Add(len(children))
	for _, c := range children {
		go func(c *child) {
			defer s.wg.Done()
			c.run()
		}(c)
	}
}

// Stop гасит детей в обратном порядке запуска, ожидает выхода каждого и
// дожидается завершения всех горутин надзирателя (шаг 4).
func (s *Supervisor) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	close(s.stopCh)
	children := append([]*child(nil), s.children...)
	s.mu.Unlock()

	for i := len(children) - 1; i >= 0; i-- {
		children[i].terminate()
	}
	s.wg.Wait()
}

// normalize заполняет нулевые ReadyTimeout и PollInterval значениями по умолчанию.
func normalize(spec Config) Config {
	if spec.ReadyTimeout <= 0 {
		spec.ReadyTimeout = DefaultReadyTimeout
	}
	if spec.PollInterval <= 0 {
		spec.PollInterval = DefaultPollInterval
	}
	return spec
}

// Load читает секцию children из файла конфигурации и возвращает спецификации
// детей с применёнными значениями по умолчанию.
func Load(path string) ([]Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	specs, err := parseChildren(string(data))
	if err != nil {
		return nil, err
	}
	out := make([]Config, len(specs))
	for i, spec := range specs {
		out[i] = normalize(spec)
	}
	return out, nil
}

// parseChildren разбирает топ-ключ `children:` — список детей с полями шага 1.
func parseChildren(data string) ([]Config, error) {
	var out []Config
	var cur *Config
	inChildren := false
	inArgs := false

	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))

		if indent == 0 {
			inChildren = trimmed == "children:" || trimmed == "children: []"
			cur = nil
			inArgs = false
			continue
		}
		if !inChildren {
			continue
		}

		if inArgs {
			if strings.HasPrefix(trimmed, "-") {
				if cur != nil {
					cur.Args = append(cur.Args, strings.TrimSpace(strings.TrimPrefix(trimmed, "-")))
				}
				continue
			}
			inArgs = false
		}

		if strings.HasPrefix(trimmed, "-") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			if item == "" {
				continue
			}
			out = append(out, Config{})
			cur = &out[len(out)-1]
			if err := setField(cur, item); err != nil {
				return nil, err
			}
			continue
		}

		if cur == nil {
			continue
		}
		key, val, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "args" {
			inArgs = true
			continue
		}
		inArgs = false
		if err := setField(cur, key+": "+strings.TrimSpace(val)); err != nil {
			return nil, err
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// setField раскладывает одну строку "key: value" по полям Config.
func setField(c *Config, line string) error {
	key, val, ok := strings.Cut(line, ":")
	if !ok {
		return nil
	}
	key = strings.TrimSpace(key)
	val = strings.TrimSpace(val)
	switch key {
	case "name":
		c.Name = val
	case "cmd":
		c.Cmd = val
	case "port":
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("supervisor: child %q: bad port %q", c.Name, val)
		}
		c.Port = n
	case "health_path":
		c.HealthPath = val
	case "ready_timeout":
		d, err := time.ParseDuration(val)
		if err != nil {
			return fmt.Errorf("supervisor: child %q: bad ready_timeout %q", c.Name, val)
		}
		c.ReadyTimeout = d
	case "poll_interval":
		d, err := time.ParseDuration(val)
		if err != nil {
			return fmt.Errorf("supervisor: child %q: bad poll_interval %q", c.Name, val)
		}
		c.PollInterval = d
	}
	return nil
}
