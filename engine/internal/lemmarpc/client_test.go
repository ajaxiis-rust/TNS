package lemmarpc

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"tns65/internal/journal"
)

// engineRoot возвращает корень engine/ — каталог с go.mod.
func engineRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("engine root (go.mod) not found")
		}
		dir = parent
	}
}

// buildFake собирает тестовый лемматизатор и возвращает путь бинарника.
func buildFake(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fake_lemmatizer")
	cmd := exec.Command("go", "build", "-o", bin, "tns65/internal/lemmarpc/testdata/fake_lemmatizer")
	cmd.Dir = engineRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake_lemmatizer: %v\n%s", err, out)
	}
	return bin
}

// TestProtocol — контракт stdio/JSON построчно: запрос/ответ с леммой, частью
// речи и метками; «не» — отдельный токен с леммой «не» (D-13.3).
func TestProtocol(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteRequest(&buf, 7, "не хочу"); err != nil {
		t.Fatalf("WriteRequest: %v", err)
	}
	if got, want := buf.String(), `{"id":7,"text":"не хочу"}`+"\n"; got != want {
		t.Fatalf("request line = %q, want %q", got, want)
	}

	req, err := ReadRequest(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("ReadRequest: %v", err)
	}
	if req.ID != 7 || req.Text != "не хочу" {
		t.Fatalf("request = %+v", req)
	}

	var out bytes.Buffer
	tokens := []Token{
		{Form: "не", Lemma: "не", Pos: "PART", Tags: []string{}},
		{Form: "хочу", Lemma: "хотеть", Pos: "VERB", Tags: []string{}},
	}
	if err := WriteResponse(&out, 7, tokens); err != nil {
		t.Fatalf("WriteResponse: %v", err)
	}
	resp, err := ReadResponse(bufio.NewReader(&out))
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}
	if resp.ID != 7 || len(resp.Tokens) != 2 {
		t.Fatalf("response = %+v", resp)
	}
	if resp.Tokens[0].Lemma != "не" || resp.Tokens[1].Lemma != "хотеть" {
		t.Fatalf("леммы = %q, %q, want «не», «хотеть»", resp.Tokens[0].Lemma, resp.Tokens[1].Lemma)
	}
	if resp.Tokens[1].Pos != "VERB" {
		t.Fatalf("pos = %q, want VERB", resp.Tokens[1].Pos)
	}
}

// TestClient — клиент запускает fake_lemmatizer: «не хочу» разбирается на
// токены «не» (лемма «не») и «хотеть»; тайм-аут 200 мс даёт ошибку, а не
// зависание; после трёх подряд тайм-аутов процесс перезапускается (шаг 2).
func TestClient(t *testing.T) {
	fake := buildFake(t)

	t.Run("не хочу", func(t *testing.T) {
		c := NewClient(fake, nil)
		defer c.Close()

		tokens, err := c.Lemmatize("не хочу")
		if err != nil {
			t.Fatalf("Lemmatize: %v", err)
		}
		if len(tokens) != 2 {
			t.Fatalf("токенов = %d, want 2: %+v", len(tokens), tokens)
		}
		if tokens[0].Form != "не" || tokens[0].Lemma != "не" || tokens[0].Pos != "PART" {
			t.Fatalf("токен 0 = %+v, want не/не/PART", tokens[0])
		}
		if tokens[1].Form != "хочу" || tokens[1].Lemma != "хотеть" || tokens[1].Pos != "VERB" {
			t.Fatalf("токен 1 = %+v, want хочу/хотеть/VERB", tokens[1])
		}
	})

	t.Run("timeout", func(t *testing.T) {
		t.Setenv("LEMMARPC_FAKE_DELAY", "300ms")
		logPath := filepath.Join(t.TempDir(), "journal.log")
		logger, err := journal.New(logPath)
		if err != nil {
			t.Fatal(err)
		}
		c := NewClient(fake, logger)
		defer c.Close()

		start := time.Now()
		_, err = c.Lemmatize("не хочу")
		if !errors.Is(err, ErrTimeout) {
			t.Fatalf("err = %v, want ErrTimeout", err)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("зависание: вернулся за %v, want < 1s", elapsed)
		}

		b, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), journal.EventLemmaFail) {
			t.Fatalf("в журнале нет события %s:\n%s", journal.EventLemmaFail, b)
		}
	})

	t.Run("restart after 3 timeouts", func(t *testing.T) {
		t.Setenv("LEMMARPC_FAKE_DELAY", "300ms")
		pidLog := filepath.Join(t.TempDir(), "pids.log")
		t.Setenv("LEMMARPC_FAKE_PID_LOG", pidLog)
		c := NewClient(fake, nil)
		defer c.Close()

		for i := 0; i < 4; i++ {
			if _, err := c.Lemmatize("не хочу"); !errors.Is(err, ErrTimeout) {
				t.Fatalf("вызов %d: err = %v, want ErrTimeout", i+1, err)
			}
		}

		b, err := os.ReadFile(pidLog)
		if err != nil {
			t.Fatal(err)
		}
		pids := distinctPIDs(string(b))
		if len(pids) != 2 {
			t.Fatalf("ожидалось 2 процесса (перезапуск), получено %d: %v", len(pids), pids)
		}
	})
}

// distinctPIDs возвращает уникальные PID из лога, по одному на строку.
func distinctPIDs(log string) []int {
	var out []int
	seen := map[int]bool{}
	for _, line := range strings.Fields(log) {
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		if !seen[pid] {
			seen[pid] = true
			out = append(out, pid)
		}
	}
	return out
}
