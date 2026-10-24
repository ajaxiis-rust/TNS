package journal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestEventsUnique(t *testing.T) {
	events := []string{
		EventDBForeignWriter,
		EventNetgateDeny,
		EventPortgateDeny,
		EventChildDegraded,
		EventChildOutput,
		EventChildStopped,
		EventLemmaFail,
		EventSwarmVersionMismatch,
		EventWebAccess,
		EventEmbedFail,
		EventAnchorForced,
	}

	// Значения уникальны (нет дублей).
	seen := make(map[string]struct{}, len(events))
	for _, e := range events {
		if _, ok := seen[e]; ok {
			t.Fatalf("дубль значения события: %q", e)
		}
		seen[e] = struct{}{}
	}

	// Набор значений в точности равен ожидаемому.
	want := []string{
		"db.foreign_writer",
		"netgate.deny",
		"portgate.deny",
		"child.degraded",
		"child.output",
		"child.stopped",
		"lemma.fail",
		"swarm.version_mismatch",
		"web.access",
		"embed.fail",
		"birth.anchor_forced",
	}
	got := append([]string(nil), events...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("набор значений событий не совпадает:\n got=%v\nwant=%v", got, want)
	}
}

func TestRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "journal.log")

	l, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.maxBytes = 200
	l.maxFiles = 5

	const firstMarker = "the-very-first-event-marker"

	// Первое событие — с уникальным маркером.
	if err := l.Event("core", EventDBForeignWriter, Field{Key: "marker", Value: firstMarker}); err != nil {
		t.Fatalf("Event(first): %v", err)
	}

	// Событий достаточно для 7+ ротаций.
	const total = 300
	for i := 1; i < total; i++ {
		if err := l.Event("core", EventWebAccess, Field{Key: "i", Value: fmt.Sprintf("%d", i)}); err != nil {
			t.Fatalf("Event(%d): %v", i, err)
		}
	}

	archives := archivePaths(path, 5)

	// Архивов ровно 5.
	for _, a := range archives {
		if _, err := os.Stat(a); err != nil {
			t.Fatalf("архив %s отсутствует: %v", a, err)
		}
	}

	// Собираем все строки: текущий файл + архивы.
	files := append([]string{path}, archives...)
	var all []string
	for _, f := range files {
		lines, err := readLines(f)
		if err != nil {
			t.Fatalf("readLines(%s): %v", f, err)
		}
		all = append(all, lines...)
	}

	// Старейший маркер не встречается ни в текущем файле, ни в архивах.
	for _, line := range all {
		if strings.Contains(line, firstMarker) {
			t.Fatalf("старейший маркер не должен встречаться, найден в строке %q", line)
		}
	}

	// Текущий файл ≤ maxBytes.
	if info, err := os.Stat(path); err != nil {
		t.Fatalf("stat current: %v", err)
	} else if info.Size() > l.maxBytes {
		t.Fatalf("текущий файл %d > maxBytes %d", info.Size(), l.maxBytes)
	}

	// Каждая строка каждого файла — валидный JSON с ключами ts, level, source, event, fields.
	for _, line := range all {
		checkEventLine(t, line)
	}

	t.Run("concurrent", func(t *testing.T) {
		cpath := filepath.Join(t.TempDir(), "concurrent.log")
		cl, err := New(cpath)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		cl.maxBytes = 1 << 20 // большой — без ротаций
		cl.maxFiles = 5

		const N, M = 8, 25
		var wg sync.WaitGroup
		for g := 0; g < N; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				for j := 0; j < M; j++ {
					if err := cl.Event("core", EventChildStopped,
						Field{Key: "g", Value: fmt.Sprintf("%d", g)},
						Field{Key: "j", Value: fmt.Sprintf("%d", j)}); err != nil {
						t.Errorf("Event: %v", err)
						return
					}
				}
			}(g)
		}
		wg.Wait()

		var count int
		cfiles := append([]string{cpath}, archivePaths(cpath, 5)...)
		for _, f := range cfiles {
			if _, err := os.Stat(f); err != nil {
				continue
			}
			lines, err := readLines(f)
			if err != nil {
				t.Fatalf("readLines(%s): %v", f, err)
			}
			for _, line := range lines {
				checkEventLine(t, line)
				count++
			}
		}
		if count != N*M {
			t.Fatalf("строк = %d, ожидалось %d (потеряно или слиплось)", count, N*M)
		}
	})
}

func archivePaths(path string, n int) []string {
	out := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, fmt.Sprintf("%s.%d", path, i))
	}
	return out
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

func checkEventLine(t *testing.T, line string) {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("строка не валидный JSON: %v\n%s", err, line)
	}
	for _, k := range []string{"ts", "level", "source", "event", "fields"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("нет ключа %q в строке: %s", k, line)
		}
	}
}

func TestChildOutput(t *testing.T) {
	// 1. nil-писатель → ErrNoWriter, писатель nil.
	pw, err := Pipe("x", nil)
	if !errors.Is(err, ErrNoWriter) {
		t.Fatalf("Pipe(nil) err = %v, want ErrNoWriter", err)
	}
	if pw != nil {
		t.Fatalf("Pipe(nil) writer = %v, want nil", pw)
	}

	// 2. построчная разметка: Write("hello\nworld\n") → ровно две строки-события.
	var buf bytes.Buffer
	pw, err = Pipe("lemmaru", &buf)
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	if _, err := pw.Write([]byte("hello\nworld\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("строк = %d, ожидалось 2: %q", len(lines), buf.String())
	}
	for i, want := range []string{"hello", "world"} {
		ev := parseChildEvent(t, lines[i])
		if ev.Source != "child:lemmaru" {
			t.Fatalf("строка %d source = %q, want child:lemmaru", i, ev.Source)
		}
		if ev.Event != EventChildOutput {
			t.Fatalf("строка %d event = %q, want %q", i, ev.Event, EventChildOutput)
		}
		if len(ev.Fields) != 1 || ev.Fields[0].Key != "text" || ev.Fields[0].Value != want {
			t.Fatalf("строка %d fields = %v, want text=%q", i, ev.Fields, want)
		}
	}

	// 3. Write("partial") + Close() → третья строка с хвостом, не потеряна.
	if _, err := pw.Write([]byte("partial")); err != nil {
		t.Fatalf("Write(partial): %v", err)
	}
	closePipe(t, pw)
	lines = strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("после Close строк = %d, ожидалось 3: %q", len(lines), buf.String())
	}
	if ev := parseChildEvent(t, lines[2]); len(ev.Fields) != 1 || ev.Fields[0].Value != "partial" {
		t.Fatalf("хвост не сохранён: %v", ev.Fields)
	}

	// 4. невалидные UTF-8 байты кодируются литералами \xNN, а не отбрасываются/заменяются.
	var buf2 bytes.Buffer
	pw2, err := Pipe("lemmaru", &buf2)
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	if _, err := pw2.Write([]byte{'a', 0xff, 0xfe, 'b', '\n'}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	closePipe(t, pw2)
	out := buf2.String()
	if !strings.Contains(out, `\xff`) {
		t.Fatalf("байт 0xff не закодирован как \\xff: %q", out)
	}
	if !strings.Contains(out, `\xfe`) {
		t.Fatalf("байт 0xfe не закодирован как \\xfe: %q", out)
	}
	if strings.Contains(out, "\uFFFD") {
		t.Fatalf("присутствует replacement-символ: %q", out)
	}

	// 5. сквозной подтест: Pipe(name, logger) на файле с малым maxBytes.
	dir := t.TempDir()
	path := filepath.Join(dir, "child.log")
	logger, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.maxBytes = 200
	logger.maxFiles = 5
	pw3, err := Pipe("forge", logger)
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	for i := 0; i < 200; i++ {
		if _, err := pw3.Write([]byte(fmt.Sprintf("line-%d\n", i))); err != nil {
			t.Fatalf("Write(%d): %v", i, err)
		}
	}
	closePipe(t, pw3)

	found := false
	for _, f := range append([]string{path}, archivePaths(path, 5)...) {
		flines, err := readLines(f)
		if err != nil {
			t.Fatalf("readLines(%s): %v", f, err)
		}
		for _, line := range flines {
			ev := parseChildEvent(t, line)
			if ev.Source == "child:forge" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("нет строк с source=child:forge в журнале")
	}
}

func parseChildEvent(t *testing.T, line string) eventRecord {
	t.Helper()
	var rec eventRecord
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("строка не валидный JSON: %v\n%s", err, line)
	}
	return rec
}

func closePipe(t *testing.T, pw io.Writer) {
	t.Helper()
	c, ok := pw.(io.Closer)
	if !ok {
		t.Fatalf("писатель Pipe не реализует io.Closer")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
