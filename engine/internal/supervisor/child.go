package supervisor

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"

	"tns65/internal/journal"
)

// ErrNotReady — ребёнок не стал готов за ready_timeout (T-01-13, шаг 2).
var ErrNotReady = errors.New("supervisor: child not ready in time")

// restartPause — пауза перед перезапуском после падений: 5, 10, 20, 30, далее 60 с
// (шаг 3).
var restartPause = []time.Duration{
	5 * time.Second,
	10 * time.Second,
	20 * time.Second,
	30 * time.Second,
	60 * time.Second,
}

// child — один резидент под надзором: запуск, лечение и остановка.
type child struct {
	sup      *Supervisor
	spec     Config
	degraded int

	mu   sync.Mutex
	proc Process
	done chan struct{}
}

func newChild(sup *Supervisor, spec Config) *child {
	return &child{sup: sup, spec: spec, done: make(chan struct{})}
}

// run — цикл надзора за одним ребёнком: старт → готовность → ожидание выхода →
// лечение. Завершается по Stop (закрытие общего stopCh).
func (c *child) run() {
	defer close(c.done)
	for {
		if c.stopped() {
			return
		}

		stdout, err := journal.Pipe(c.spec.Name, c.sup.logger)
		if err != nil {
			c.degraded++
			c.logDegraded(err)
			if !c.backoff() {
				return
			}
			continue
		}
		stderr, err := journal.Pipe(c.spec.Name, c.sup.logger)
		if err != nil {
			_ = closeWriter(stdout)
			c.degraded++
			c.logDegraded(err)
			if !c.backoff() {
				return
			}
			continue
		}

		proc, err := c.sup.runner.Start(c.spec, stdout, stderr)
		if err != nil {
			_ = closeWriter(stdout)
			_ = closeWriter(stderr)
			c.degraded++
			c.logDegraded(err)
			if !c.backoff() {
				return
			}
			continue
		}
		c.setProc(proc)

		waitCh := make(chan error, 1)
		go func() { waitCh <- proc.Wait() }()

		ready, stopping := c.waitReady()

		var exitErr error
		if !ready && !stopping {
			// Не готов в срок — гасим и лечим как неудачу (шаг 2).
			_ = proc.Stop()
			<-waitCh
			exitErr = ErrNotReady
		} else {
			// Готов или остановка: ждём выхода процесса; при остановке его гасит
			// terminate() в обратном порядке — сам цикл процесс не останавливает.
			exitErr = <-waitCh
		}

		_ = closeWriter(stdout)
		_ = closeWriter(stderr)
		c.setProc(nil)

		if c.stopped() {
			c.logStopped()
			return
		}

		c.degraded++
		c.logDegraded(exitErr)
		if !c.backoff() {
			return
		}
	}
}

// waitReady опрашивает готовность каждые poll_interval до ready_timeout без
// фиксированного короткого бюджета: загрузка модели идёт минутами (шаг 2).
// Возвращает (готов, остановлен).
func (c *child) waitReady() (ready, stopping bool) {
	deadline := c.sup.clock.Now().Add(c.spec.ReadyTimeout)
	for {
		select {
		case <-c.sup.stopCh:
			return false, true
		default:
		}
		if err := c.sup.ready(c.spec); err == nil {
			return true, false
		}
		if !c.sup.clock.Now().Before(deadline) {
			return false, false
		}
		select {
		case <-c.sup.stopCh:
			return false, true
		case <-c.sup.clock.After(c.spec.PollInterval):
		}
	}
}

// backoff спит паузу перезапуска для текущего числа деградаций; false — надзиратель
// остановлен и цикл завершается.
func (c *child) backoff() bool {
	select {
	case <-c.sup.stopCh:
		return false
	case <-c.sup.clock.After(pauseFor(c.degraded)):
		return true
	}
}

// pauseFor выбирает паузу перезапуска по порядковому номеру неудачи.
func pauseFor(n int) time.Duration {
	if n <= 0 {
		n = 1
	}
	if n > len(restartPause) {
		n = len(restartPause)
	}
	return restartPause[n-1]
}

// terminate останавливает текущий процесс ребёнка и ждёт завершения его цикла.
func (c *child) terminate() {
	c.mu.Lock()
	proc := c.proc
	c.mu.Unlock()
	if proc != nil {
		_ = proc.Stop()
	}
	<-c.done
}

func (c *child) setProc(p Process) {
	c.mu.Lock()
	c.proc = p
	c.mu.Unlock()
}

func (c *child) stopped() bool {
	select {
	case <-c.sup.stopCh:
		return true
	default:
		return false
	}
}

// logDegraded пишет событие child.degraded (шаг 3).
func (c *child) logDegraded(err error) {
	if c.sup.logger == nil {
		return
	}
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	_ = c.sup.logger.Event("supervisor", journal.EventChildDegraded,
		journal.Field{Key: "child", Value: c.spec.Name},
		journal.Field{Key: "error", Value: reason})
}

// logStopped пишет событие child.stopped (шаг 4).
func (c *child) logStopped() {
	if c.sup.logger == nil {
		return
	}
	_ = c.sup.logger.Event("supervisor", journal.EventChildStopped,
		journal.Field{Key: "child", Value: c.spec.Name})
}

// closeWriter сбрасывает хвост writer'а, если он его поддерживает (journal.Pipe).
func closeWriter(w io.Writer) error {
	if c, ok := w.(interface{ Close() error }); ok {
		return c.Close()
	}
	return nil
}

// execRunner — реальная фабрика: os/exec с stdout/stderr через journal.Pipe,
// прямой os.Stdout ребёнка запрещён (шаг 2).
type execRunner struct{}

func (execRunner) Start(spec Config, stdout, stderr io.Writer) (Process, error) {
	cmd := exec.Command(spec.Cmd, spec.Args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProcess{cmd: cmd}, nil
}

// execProcess — запущенный процесс ОС.
type execProcess struct {
	cmd      *exec.Cmd
	stopOnce sync.Once
}

func (p *execProcess) Wait() error { return p.cmd.Wait() }

func (p *execProcess) Stop() error {
	p.stopOnce.Do(func() {
		if p.cmd.Process != nil {
			_ = signalProcess(p.cmd.Process)
		}
	})
	return nil
}

// signalProcess останавливает процесс: SIGTERM на unix, Kill на Windows
// (SIGTERM в syscall объявлен и на Windows, но os.Process.Signal его не шлёт).
func signalProcess(proc *os.Process) error {
	if runtime.GOOS == "windows" {
		return proc.Kill()
	}
	return proc.Signal(syscall.SIGTERM)
}
