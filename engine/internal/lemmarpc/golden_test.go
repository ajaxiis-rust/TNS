package lemmarpc

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// Пороги «Проверки» T-01-10: доля верного первого разбора на золотом наборе
// lemmabench-golang-20261020 (гейт B-1) и бюджет памяти резидента (D-13.4).
const (
	goldenThreshold = 89.18
	memoryLimitMB   = 200
)

// goldenPair — пара «форма — эталонная лемма» из золотого набора (CONLLU).
type goldenPair struct {
	form  string
	lemma string
}

var ruLetter = regexp.MustCompile(`[А-ЯЁа-яё]`)

// buildLemmaru собирает резидента tns65-lemmaru и возвращает путь бинарника.
func buildLemmaru(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "tns65-lemmaru")
	cmd := exec.Command("go", "build", "-o", bin, "tns65/cmd/tns65-lemmaru")
	cmd.Dir = engineRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build tns65-lemmaru: %v\n%s", err, out)
	}
	return bin
}

// TestGoldenLemmaru — резидент tns65-lemmaru на золотом наборе (T-01-10).
// Проверяет протокол и «не» отдельной леммой, долю верного первого разбора
// на русской части золотого набора и память резидента.
func TestGoldenLemmaru(t *testing.T) {
	bin := buildLemmaru(t)

	t.Run("protocol", func(t *testing.T) {
		c := NewClient(bin, nil)
		defer c.Close()

		tokens, err := c.Lemmatize("не хочу")
		if err != nil {
			t.Fatalf("Lemmatize: %v", err)
		}
		if len(tokens) != 2 {
			t.Fatalf("токенов = %d, want 2: %+v", len(tokens), tokens)
		}
		if tokens[0].Form != "не" || tokens[0].Lemma != "не" {
			t.Fatalf("токен 0 = %+v, want форма «не», лемма «не»", tokens[0])
		}
		if tokens[1].Lemma != "хотеть" {
			t.Fatalf("токен 1 = %+v, want лемма «хотеть»", tokens[1])
		}
	})

	t.Run("golden", func(t *testing.T) {
		golden := findGolden(t)
		pairs := loadGolden(t, golden)
		rate, rssMB := runGolden(t, bin, pairs)

		t.Logf("первый разбор: %.2f %% (порог %.2f %%); rss резидента: %.1f МБ (лимит %d МБ)",
			rate, goldenThreshold, rssMB, memoryLimitMB)

		if rate < goldenThreshold {
			t.Fatalf("доля верного первого разбора %.2f %% ниже порога %.2f %%", rate, goldenThreshold)
		}
		if rssMB > 0 && rssMB > memoryLimitMB {
			t.Fatalf("память резидента %.1f МБ выше лимита %d МБ", rssMB, memoryLimitMB)
		}
	})
}

// findGolden возвращает путь к золотому набору: переменная LEMMABENCH_GOLDEN,
// затем эталонный набор из соседнего каталога lemmabench-20261018.
func findGolden(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("LEMMABENCH_GOLDEN"); p != "" {
		return p
	}
	root := engineRoot(t)
	candidates := []string{
		filepath.Join(root, "..", "..", "lemmabench-20261018", "golden", "ru_syntagrus-ud-test.conllu"),
		filepath.Join(root, "..", "..", "lemmabench-golang-20261020", "golden", "ru_syntagrus-ud-test.conllu"),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	t.Fatalf("золотой набор не найден: задайте LEMMABENCH_GOLDEN (пробовали: %v)", candidates)
	return ""
}

// loadGolden читает пары «форма — эталонная лемма» из CONLLU: пропускает
// комментарии, пустые строки и составные токены (id с '-' или '.').
func loadGolden(t *testing.T, path string) []goldenPair {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("открыть золотой набор %s: %v", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var pairs []goldenPair
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 3 {
			continue
		}
		if strings.ContainsAny(cols[0], "-.") {
			continue
		}
		pairs = append(pairs, goldenPair{form: cols[1], lemma: cols[2]})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("читать золотой набор: %v", err)
	}
	if len(pairs) == 0 {
		t.Fatalf("золотой набор %s пуст", path)
	}
	return pairs
}

// runGolden прогоняет пары через резидента по протоколу stdio/JSON и возвращает
// долю верного первого разбора на русской части набора и rss резидента в МБ.
func runGolden(t *testing.T, bin string, pairs []goldenPair) (rate, rssMB float64) {
	t.Helper()
	cmd := exec.Command(bin)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	br := bufio.NewReaderSize(stdout, 64*1024)
	ruTotal, ruMatch := 0, 0
	for i, p := range pairs {
		if err := WriteRequest(stdin, i, p.form); err != nil {
			t.Fatalf("WriteRequest(%q): %v", p.form, err)
		}
		resp, err := ReadResponse(br)
		if err != nil {
			t.Fatalf("ReadResponse(%q): %v", p.form, err)
		}
		first := p.form
		if len(resp.Tokens) > 0 {
			first = resp.Tokens[0].Lemma
		}
		if !ruLetter.MatchString(p.form) {
			continue
		}
		ruTotal++
		if first == p.lemma {
			ruMatch++
		}
	}

	rssMB = residentRSSMB(cmd.Process.Pid)
	_ = stdin.Close()
	return float64(ruMatch) / float64(ruTotal) * 100, rssMB
}

// residentRSSMB читает пиковую резидентную память процесса (VmHWM) из /proc
// (Linux); при отсутствии VmHWM — текущую VmRSS. 0 — не измерено.
func residentRSSMB(pid int) float64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return 0
	}
	var rss int
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "VmHWM:") || strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if kb, err := strconv.Atoi(fields[1]); err == nil {
					if strings.HasPrefix(line, "VmHWM:") {
						return float64(kb) / 1024.0
					}
					rss = kb
				}
			}
		}
	}
	if rss == 0 {
		return 0
	}
	return float64(rss) / 1024.0
}
