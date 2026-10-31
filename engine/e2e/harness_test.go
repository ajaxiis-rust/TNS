//go:build e2e

// Package e2e — сквозные приёмочные сценарии P8 (T-09). Каждый тест готовит
// временный каталог world_db/, собирает бинарник с тегом e2e, гоняет сценарий
// и пишет отчёт reports/<ID>.json (T-09-01, шаг 1).
package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"tns65/internal/netgate"
)

// engineRoot возвращает корень движка (каталог с go.mod).
func engineRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(file)) // engine/e2e -> engine
}

// buildBinary собирает ./cmd/tns65 с заданными тегами и возвращает путь к
// бинарнику. Тег e2e включает хуки --fake-* (T-09-01, «Правится»).
func buildBinary(t *testing.T, tags []string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "tns65")
	args := []string{"build", "-o", out}
	if len(tags) > 0 {
		args = append(args, "-tags")
		args = append(args, tags...)
	}
	args = append(args, "./cmd/tns65")
	cmd := exec.Command("go", args...)
	cmd.Dir = engineRoot(t)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if buf, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %v: %v\n%s", tags, err, buf)
	}
	return out
}

// worldDir готовит временный рабочий каталог: базы языков base/ и газетир seed/
// видны по прежним путям, а world_db/ рождается внутри временного каталога,
// не пачкая репозиторий.
func worldDir(t *testing.T) string {
	t.Helper()
	root := engineRoot(t)
	dir := t.TempDir()
	for _, name := range []string{"base", "seed"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			if err := os.Symlink(filepath.Join(root, name), filepath.Join(dir, name)); err != nil {
				t.Fatalf("symlink %s: %v", name, err)
			}
		}
	}
	return dir
}

// runCmd запускает бинарник в каталоге dir и возвращает stdout, stderr и код.
func runCmd(t *testing.T, dir, bin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %v: %v", append([]string{bin}, args...), err)
		}
		code = ee.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

// ancketa — валидная анкета рождения: ровно 14 полей (T-02-01).
type ancketa struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Synopsis    string   `json:"synopsis"`
	Prologue    string   `json:"prologue"`
	Lang        string   `json:"lang"`
	TravelMode  string   `json:"travel_mode"`
	Genres      []string `json:"genres"`
	WorldRules  []string `json:"world_rules"`
	SocialOrder string   `json:"social_order"`
	EconomyMods []string `json:"economy_mods"`
	Magic       string   `json:"magic"`
	HeroBirth   string   `json:"hero_birth"`
	Censorship  string   `json:"censorship"`
}

// writeAncketa пишет анкету в dir и возвращает путь к файлу.
func writeAncketa(t *testing.T, dir, slug, synopsis string) string {
	t.Helper()
	a := ancketa{
		Slug:        slug,
		Title:       "Тёмный мир",
		Description: "Мир, где свет угас навсегда.",
		Synopsis:    synopsis,
		Prologue:    "Всё началось с тишины.",
		Lang:        "ru",
		TravelMode:  "вымышленная",
		Genres:      []string{"фэнтези", "хоррор"},
		WorldRules:  []string{"магия: редкая и опасная"},
		SocialOrder: "феодализм",
		EconomyMods: []string{"бартер"},
		Magic:       "редкая",
		HeroBirth:   "простолюдин",
		Censorship:  "16+",
	}
	raw, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, slug+".json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeReport пишет отчёт сценария в engine/e2e/reports/<id>.json.
func writeReport(t *testing.T, id string, fields map[string]any) {
	t.Helper()
	dir := filepath.Join(engineRoot(t), "e2e", "reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir reports: %v", err)
	}
	raw, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(dir, id+".json"), raw, 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
}

// worldDBPath — путь оперативной базы мира по слагу в рабочем каталоге dir.
func worldDBPath(dir, slug string) string {
	return filepath.Join(dir, "world_db", "worlds", slug, slug+".db")
}

// core — запущенный процесс ядра.
type core struct {
	cmd    *exec.Cmd
	out    *bytes.Buffer
	err    *bytes.Buffer
	url    string
	client *http.Client
}

// startCore поднимает ядро как процесс в dir и ждёт /api/health.
func startCore(t *testing.T, dir, bin string, args ...string) *core {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	c := &core{
		cmd:    cmd,
		out:    &bytes.Buffer{},
		err:    &bytes.Buffer{},
		url:    "http://127.0.0.1:8720",
		client: netgate.Local("127.0.0.1:8720"),
	}
	cmd.Stdout = c.out
	cmd.Stderr = c.err
	if err := cmd.Start(); err != nil {
		t.Fatalf("start core: %v", err)
	}
	t.Cleanup(func() { c.stop() })
	waitHealth(t, c)
	return c
}

// waitHealth ждёт готовности /api/health ядра.
func waitHealth(t *testing.T, c *core) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := c.client.Get(c.url + "/api/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("core did not become healthy at %s", c.url)
}

// stop гасит ядро по SIGTERM и ждёт завершения процесса.
func (c *core) stop() {
	if c.cmd == nil || c.cmd.Process == nil {
		return
	}
	_ = c.cmd.Process.Signal(syscall.SIGTERM)
	_ = c.cmd.Wait()
}

// postTurn подаёт реплику игрока в ядро и возвращает id хода.
func (c *core) postTurn(t *testing.T, world, text string) int64 {
	t.Helper()
	body, err := json.Marshal(map[string]string{"world": world, "text": text})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.client.Post(c.url+"/api/turn", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post turn: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post turn status %d", resp.StatusCode)
	}
	var out struct {
		TurnID int64 `json:"turn_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode turn: %v", err)
	}
	return out.TurnID
}

// TestNoE2eHooksInRelease — обычная сборка без тега e2e не принимает хуки
// --fake-clock/--fake-embed/--fake-slot (T-09-01, шаг 1).
func TestNoE2eHooksInRelease(t *testing.T) {
	bin := buildBinary(t, nil)
	dir := worldDir(t)
	valid := writeAncketa(t, t.TempDir(), "hookless", "Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.")

	if _, _, code := runCmd(t, dir, bin, "world", "birth", "--ancketa", valid, "--fake-embed"); code == 0 {
		t.Fatal("релизная сборка приняла --fake-embed (exit 0), want отказ")
	}
	if _, _, code := runCmd(t, dir, bin, "world", "birth", "--ancketa", valid, "--fake-slot"); code == 0 {
		t.Fatal("релизная сборка приняла --fake-slot (exit 0), want отказ")
	}
	if _, _, code := runCmd(t, dir, bin, "--fake-clock"); code == 0 {
		t.Fatal("релизная сборка приняла --fake-clock (exit 0), want отказ")
	}
}
