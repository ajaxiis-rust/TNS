package supervisor

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// engineRoot возвращает корень модуля engine (каталог с go.mod).
func engineRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}").Output()
	if err != nil {
		t.Fatalf("go list -m: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// buildTns65 собирает бинарник cmd/tns65. Тест исполняет настоящий бинарник:
// проверяется путь кода 4 из main.go, а не мок Acquire.
func buildTns65(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "tns65")
	cmd := exec.Command("go", "build", "-o", bin, "tns65/cmd/tns65")
	cmd.Dir = engineRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build cmd/tns65: %v\n%s", err, out)
	}
	return bin
}

func TestSecondInstance(t *testing.T) {
	bin := buildTns65(t)

	// dir играет роль рабочего каталога ядра: main.go берёт замок по
	// относительному пути state/tns65.lock.
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "state", "tns65.lock")

	// Первый экземпляр (этот тестовый процесс) держит замок.
	first, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("первый Acquire: %v", err)
	}
	defer first.Release()

	// Первый экземпляр стартует надзирателя с одним ребёнком.
	logger, _ := newTestLogger(t)
	runner := &fakeRunner{}
	runner.makeProc = func(name string) *fakeProcess {
		return &fakeProcess{name: name, runner: runner, crashCh: make(chan error, 1), stopCh: make(chan struct{})}
	}
	s := New(logger, []Config{{Name: "res", Cmd: "fake"}},
		WithClock(newFakeClock()),
		WithRunner(runner),
		WithReady(func(Config) error { return nil }))
	go s.Run()
	waitFor(t, func() bool { return runner.startCount() >= 1 })

	// Второй экземпляр — отдельный ОС-процесс того же бинарника с тем же
	// рабочим каталогом: main.go сам захватывает state/tns65.lock и должен
	// выйти с кодом 4, не запуская детей.
	second := exec.Command(bin)
	second.Dir = dir
	out, err := second.CombinedOutput()
	if err == nil {
		t.Fatalf("второй запуск: код 0, want 4")
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("второй запуск: err = %v, want ExitError с кодом 4", err)
	}
	if code := ee.ExitCode(); code != 4 {
		t.Fatalf("второй запуск: код = %d, want 4\n%s", code, out)
	}

	// Второй запуск не удвоил детей: надзиратель первого экземпляра запустил
	// ровно одного ребёнка.
	if n := runner.startCount(); n != 1 {
		t.Fatalf("число запущенных детей = %d, want 1 (не удвоилось)", n)
	}

	s.Stop()

	// После штатной остановки замок свободен.
	if err := first.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	third, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("Acquire после штатной остановки: %v", err)
	}
	if err := third.Release(); err != nil {
		t.Fatalf("Release после штатной остановки: %v", err)
	}

	// После аварийного выхода ОС закрывает файл без Release — остаток файла
	// новому старту не мешает (блокировка сидит на открытом описании, а не в
	// имени файла).
	crash, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("Acquire для аварийного сценария: %v", err)
	}
	if err := crash.file.Close(); err != nil {
		t.Fatalf("close аварийного замка: %v", err)
	}
	again, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("Acquire после аварийного выхода: %v", err)
	}
	_ = again.Release()
}
