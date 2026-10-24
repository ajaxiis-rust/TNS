package swarm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"tns65/internal/journal"
)

// Имена переменных окружения, под которые подстраивается подставная задача
// (re-exec тестового бинарника).
const (
	envTaskHelper  = "TNS65_SWARM_TEST_TASK"
	envTaskVersion = "TNS65_SWARM_TEST_VERSION"
	envTaskWork    = "TNS65_SWARM_TEST_WORK"
	envTaskDelay   = "TNS65_SWARM_TEST_DELAY"
)

// TestMain: когда тестовый бинарник запущен с envTaskHelper, он играет роль
// подставной задачи роя, а не набора тестов.
func TestMain(m *testing.M) {
	if os.Getenv(envTaskHelper) != "" {
		fakeTaskMain()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeTaskMain — подставная задача: первое действие — рукопожатие в stdout,
// затем (после заданной задержки) работа и результат.
func fakeTaskMain() {
	hs, _ := json.Marshal(Handshake{Version: os.Getenv(envTaskVersion)})
	fmt.Println(string(hs))

	if s := os.Getenv(envTaskDelay); s != "" {
		if ms, err := strconv.Atoi(s); err == nil && ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
	}

	if work := os.Getenv(envTaskWork); work != "" {
		_ = os.WriteFile(work, []byte(os.Getenv(EnvPackageVersion)), 0o644)
	}

	fmt.Println("done")
}

func TestVersionMismatch(t *testing.T) {
	logger, logPath := newTestLogger(t)

	work := filepath.Join(t.TempDir(), "work.marker")
	// Работа начнётся только через 5 с; ядро должно погасить задачу раньше.
	spec := taskSpec(t, "0.9.0", work, 5000)

	start := time.Now()
	_, err := Launch(logger, spec, "1.0.0")
	if !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("Launch err = %v, want ErrVersionMismatch", err)
	}
	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Fatalf("Launch ждал %v — задача не погашена до работы", elapsed)
	}

	if _, err := os.Stat(work); !os.IsNotExist(err) {
		t.Fatalf("маркер работы создан (работа началась): stat err = %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if !strings.Contains(string(data), journal.EventSwarmVersionMismatch) {
		t.Fatalf("в журнале нет event=%s:\n%s", journal.EventSwarmVersionMismatch, data)
	}
}

func TestTaskLifecycle(t *testing.T) {
	logger, _ := newTestLogger(t)

	stateDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stateDir, "sentinel.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	before := listDir(t, stateDir)

	work := filepath.Join(t.TempDir(), "work.marker")
	spec := taskSpec(t, "1.0.0", work, 0)

	res, err := Launch(logger, spec, "1.0.0")
	if err != nil {
		t.Fatalf("Launch err = %v, want nil", err)
	}

	if res.PID == 0 {
		t.Fatal("res.PID == 0 — pid не сохранён")
	}
	processGone(t, res.PID)

	// Работа выполнена и получила версию ядра из TNS65_PACKAGE_VERSION.
	got, err := os.ReadFile(work)
	if err != nil {
		t.Fatalf("маркер работы не создан (работа не выполнена): %v", err)
	}
	if string(got) != "1.0.0" {
		t.Fatalf("содержимое маркера = %q, want 1.0.0", got)
	}
	if !strings.Contains(string(res.Output), "done") {
		t.Fatalf("результат задачи не содержит done: %q", res.Output)
	}

	after := listDir(t, stateDir)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("каталог состояния изменился:\n before=%v\n after=%v", before, after)
	}
}

func newTestLogger(t *testing.T) (*journal.Logger, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "swarm.log")
	l, err := journal.New(path)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	return l, path
}

func taskSpec(t *testing.T, version, work string, delayMs int) Spec {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return Spec{
		Name: "task",
		Cmd:  exe,
		Env: []string{
			envTaskHelper + "=1",
			envTaskVersion + "=" + version,
			envTaskWork + "=" + work,
			envTaskDelay + "=" + strconv.Itoa(delayMs),
		},
	}
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func processGone(t *testing.T, pid int) {
	t.Helper()
	if runtime.GOOS != "linux" {
		return
	}
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	if err == nil {
		t.Fatalf("процесс pid=%d ещё жив (/proc/%d существует)", pid, pid)
	}
	if !os.IsNotExist(err) {
		t.Fatalf("stat /proc/%d: %v", pid, err)
	}
}
