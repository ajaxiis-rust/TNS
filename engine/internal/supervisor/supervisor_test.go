package supervisor

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"tns65/internal/journal"
)

// fakeClock — масштабируемые часы: After продвигает Now на длительность и
// срабатывает немедленно; реального ожидания в тестах нет.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(0, 0)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	c.mu.Unlock()
	ch := make(chan time.Time, 1)
	ch <- now
	return ch
}

// fakeProcess — тестовый ребёнок: Wait висит до Stop или до вброса ошибки.
type fakeProcess struct {
	name     string
	runner   *fakeRunner
	crashCh  chan error
	stopCh   chan struct{}
	stopOnce sync.Once
}

func (p *fakeProcess) Wait() error {
	select {
	case <-p.stopCh:
		return nil
	case err := <-p.crashCh:
		return err
	}
}

func (p *fakeProcess) Stop() error {
	p.stopOnce.Do(func() {
		close(p.stopCh)
		if p.runner != nil {
			p.runner.recordStop(p.name)
		}
	})
	return nil
}

func (p *fakeProcess) isStopped() bool {
	select {
	case <-p.stopCh:
		return true
	default:
		return false
	}
}

// fakeRunner — тестовая фабрика процессов, считает запуски и порядок остановок.
type fakeRunner struct {
	mu       sync.Mutex
	starts   []string
	stops    []string
	makeProc func(name string) *fakeProcess
}

func (r *fakeRunner) Start(spec Config, stdout, stderr io.Writer) (Process, error) {
	_ = stdout
	_ = stderr
	p := r.makeProc(spec.Name)
	r.mu.Lock()
	r.starts = append(r.starts, spec.Name)
	r.mu.Unlock()
	return p, nil
}

func (r *fakeRunner) recordStop(name string) {
	r.mu.Lock()
	r.stops = append(r.stops, name)
	r.mu.Unlock()
}

func (r *fakeRunner) startCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.starts)
}

func (r *fakeRunner) stopOrder() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.stops...)
}

// newTestLogger создаёт журнал во временном каталоге и возвращает путь к нему.
func newTestLogger(t *testing.T) (*journal.Logger, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "engine.log")
	l, err := journal.New(path)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	return l, path
}

// waitFor ждёт выполнения условия (потолок — защита от зависания, не ожидание
// тестируемого времени: подставные часы мгновенны).
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("условие не наступило за отведённое время")
}

func TestRestart(t *testing.T) {
	logger, logPath := newTestLogger(t)

	var mu sync.Mutex
	var procs []*fakeProcess
	runner := &fakeRunner{
		makeProc: func(name string) *fakeProcess {
			p := &fakeProcess{name: name, crashCh: make(chan error, 1), stopCh: make(chan struct{})}
			mu.Lock()
			procs = append(procs, p)
			mu.Unlock()
			return p
		},
	}

	s := New(logger, []Config{{Name: "res", Cmd: "fake"}},
		WithClock(newFakeClock()),
		WithRunner(runner),
		WithReady(func(Config) error { return nil }))

	go s.Run()

	waitFor(t, func() bool { return runner.startCount() >= 1 })

	mu.Lock()
	procs[0].crashCh <- errors.New("boom")
	mu.Unlock()

	// После аварийного выхода появляется перезапуск.
	waitFor(t, func() bool { return runner.startCount() >= 2 })

	s.Stop()

	if s.children[0].degraded < 1 {
		t.Fatalf("счётчик деградаций = %d, want >= 1", s.children[0].degraded)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if !strings.Contains(string(data), "child.degraded") {
		t.Fatalf("в журнале нет строки child.degraded:\n%s", data)
	}
}

func TestNoShortReadyBudget(t *testing.T) {
	logger, _ := newTestLogger(t)
	clock := newFakeClock()
	start := clock.Now()

	var mu sync.Mutex
	var readyFlag bool
	var procs []*fakeProcess
	runner := &fakeRunner{
		makeProc: func(name string) *fakeProcess {
			p := &fakeProcess{name: name, crashCh: make(chan error, 1), stopCh: make(chan struct{})}
			mu.Lock()
			procs = append(procs, p)
			mu.Unlock()
			return p
		},
	}

	s := New(logger, []Config{{
		Name:         "res",
		Cmd:          "fake",
		Port:         5001,
		HealthPath:   "/health",
		ReadyTimeout: 1000 * time.Second,
		PollInterval: time.Second,
	}},
		WithClock(clock),
		WithRunner(runner),
		WithReady(func(Config) error {
			mu.Lock()
			defer mu.Unlock()
			if clock.Now().Sub(start) >= 900*time.Second {
				readyFlag = true
				return nil
			}
			return errors.New("not ready")
		}))

	go s.Run()

	// Ребёнок признаётся готовым на 900-й «секунде», а не убитым коротким бюджетом.
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return readyFlag
	})

	mu.Lock()
	proc := procs[0]
	mu.Unlock()
	if proc.isStopped() {
		t.Fatal("ребёнок убит до готовности — короткий бюджет всё же есть")
	}

	s.Stop()
}

func TestStopOrder(t *testing.T) {
	logger, _ := newTestLogger(t)

	runner := &fakeRunner{}
	runner.makeProc = func(name string) *fakeProcess {
		return &fakeProcess{name: name, runner: runner, crashCh: make(chan error, 1), stopCh: make(chan struct{})}
	}

	s := New(logger, []Config{
		{Name: "a", Cmd: "fake"},
		{Name: "b", Cmd: "fake"},
		{Name: "c", Cmd: "fake"},
	},
		WithClock(newFakeClock()),
		WithRunner(runner),
		WithReady(func(Config) error { return nil }))

	go s.Run()

	waitFor(t, func() bool { return runner.startCount() >= 3 })

	s.Stop()

	got := runner.stopOrder()
	want := []string{"c", "b", "a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("порядок остановки = %v, want %v", got, want)
	}
}

func TestLoadChildren(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := `children:
  - name: llama
    cmd: bin/llama-server
    args:
      - -m
      - models/generation.gguf
    port: 5001
    health_path: /health
    ready_timeout: 600s
    poll_interval: 2s
  - name: embed
    cmd: bin/embed-server
    port: 5002
    health_path: /healthz
`
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	specs, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("children = %d, want 2", len(specs))
	}

	llama := specs[0]
	if llama.Name != "llama" || llama.Cmd != "bin/llama-server" {
		t.Fatalf("llama = %+v", llama)
	}
	if !reflect.DeepEqual(llama.Args, []string{"-m", "models/generation.gguf"}) {
		t.Fatalf("llama.Args = %v", llama.Args)
	}
	if llama.Port != 5001 || llama.HealthPath != "/health" {
		t.Fatalf("llama port/path = %d/%q", llama.Port, llama.HealthPath)
	}
	if llama.ReadyTimeout != 600*time.Second || llama.PollInterval != 2*time.Second {
		t.Fatalf("llama тайм-ауты = %v/%v", llama.ReadyTimeout, llama.PollInterval)
	}

	embed := specs[1]
	if embed.ReadyTimeout != DefaultReadyTimeout || embed.PollInterval != DefaultPollInterval {
		t.Fatalf("embed дефолты = %v/%v, want %v/%v",
			embed.ReadyTimeout, embed.PollInterval, DefaultReadyTimeout, DefaultPollInterval)
	}
}
