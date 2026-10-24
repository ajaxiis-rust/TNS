package swarm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"syscall"

	"tns65/internal/journal"
)

// Spec — описание однозадачного запуска роя: бинарник задачи, аргументы и
// дополнительные переменные окружения (EnvPackageVersion добавляет ядро).
type Spec struct {
	Name string
	Cmd  string
	Args []string
	Env  []string
}

// Result — исход штатно завершившейся задачи: её pid и результат — stdout после
// строки рукопожатия.
type Result struct {
	PID    int
	Output []byte
}

// Launch запускает задачу роя. Ядро передаёт свою версию через EnvPackageVersion;
// задача первым действием печатает рукопожатие первой строкой stdout, ядро сверяет
// версию. При расхождении процесс гасится, в журнал пишется swarm.version_mismatch,
// работа не начинается и возвращается ErrVersionMismatch.
func Launch(logger *journal.Logger, spec Spec, coreVersion string) (Result, error) {
	cmd := exec.Command(spec.Cmd, spec.Args...)
	cmd.Env = append(os.Environ(), spec.Env...)
	cmd.Env = append(cmd.Env, EnvPackageVersion+"="+coreVersion)
	cmd.Stdin = nil

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}

	stderr, err := journal.Pipe(spec.Name, logger)
	if err != nil {
		return Result{}, err
	}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		_ = closeWriter(stderr)
		return Result{}, err
	}

	br := bufio.NewReader(stdout)
	line, err := br.ReadBytes('\n')
	if err != nil && err != io.EOF {
		_ = killProcess(cmd.Process)
		_ = cmd.Wait()
		_ = closeWriter(stderr)
		return Result{}, fmt.Errorf("swarm: task %s: read handshake: %w", spec.Name, err)
	}

	var hs Handshake
	if err := json.Unmarshal(bytes.TrimSpace(line), &hs); err != nil || hs.Version != coreVersion {
		got := hs.Version
		if got == "" {
			got = string(bytes.TrimSpace(line))
		}
		_ = killProcess(cmd.Process)
		_, _ = io.Copy(io.Discard, br)
		_ = cmd.Wait()
		_ = closeWriter(stderr)
		logMismatch(logger, spec.Name, coreVersion, got)
		return Result{}, ErrVersionMismatch
	}

	out, err := io.ReadAll(br)
	if err != nil {
		waitErr := cmd.Wait()
		_ = closeWriter(stderr)
		if waitErr != nil {
			return Result{}, fmt.Errorf("swarm: task %s: %w", spec.Name, waitErr)
		}
		return Result{}, fmt.Errorf("swarm: task %s: read result: %w", spec.Name, err)
	}

	if err := cmd.Wait(); err != nil {
		_ = closeWriter(stderr)
		return Result{}, fmt.Errorf("swarm: task %s: %w", spec.Name, err)
	}
	_ = closeWriter(stderr)

	return Result{PID: cmd.Process.Pid, Output: out}, nil
}

// logMismatch пишет строку event=swarm.version_mismatch (шаг 2).
func logMismatch(logger *journal.Logger, name, expected, got string) {
	if logger == nil {
		return
	}
	_ = logger.Event("swarm", journal.EventSwarmVersionMismatch,
		journal.Field{Key: "task", Value: name},
		journal.Field{Key: "expected", Value: expected},
		journal.Field{Key: "got", Value: got})
}

// killProcess останавливает процесс задачи: SIGTERM на unix, Kill на Windows.
func killProcess(p *os.Process) error {
	if runtime.GOOS == "windows" {
		return p.Kill()
	}
	return p.Signal(syscall.SIGTERM)
}

// closeWriter сбрасывает хвост writer'а, если он его поддерживает (journal.Pipe).
func closeWriter(w io.Writer) error {
	if c, ok := w.(interface{ Close() error }); ok {
		return c.Close()
	}
	return nil
}
