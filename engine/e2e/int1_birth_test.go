//go:build e2e

package e2e

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
)

// birthHooksReport — машиночитаемый итог рождения, печатается ядром в stdout.
type birthHooksReport struct {
	LLMCalls     int  `json:"llm_calls"`
	AnchorForced bool `json:"anchor_forced"`
}

// TestINT1 — INT-1. Рождение мира целиком (T-09-01): 96 карточек, один якорь,
// ноль вызовов слота прозы, отказ на пустом синопсисе, принудительный якорь.
func TestINT1(t *testing.T) {
	bin := buildBinary(t, []string{"e2e"})
	dir := worldDir(t)

	report := map[string]any{}

	// Шаг 2: рождение с фейковым эмбеддером и счётчиком слота прозы.
	slug := "sumerechnyj-mir"
	ancketa := writeAncketa(t, t.TempDir(), slug,
		"Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.")
	stdout, stderr, code := runCmd(t, dir, bin, "world", "birth", "--ancketa", ancketa, "--fake-embed", "--fake-slot")
	if code != 0 {
		t.Fatalf("world birth exit=%d, want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	hooks := parseBirthReport(t, stdout)

	worldDB := worldDBPath(dir, slug)

	// Шаг 3: characters = 96, пары «психотип — сословие» уникальны.
	cards := countCharacters(t, worldDB)
	report["cards"] = cards
	if cards != 96 {
		t.Fatalf("characters=%d, want 96", cards)
	}
	if pairs := distinctPairs(t, worldDB); pairs != 96 {
		t.Fatalf("уникальных пар «психотип — сословие»=%d, want 96", pairs)
	}

	// Шаг 4: якорь ровно один, за время рождения слот прозы не вызывался.
	anchors := countAnchors(t, worldDB)
	report["anchors"] = anchors
	if anchors != 1 {
		t.Fatalf("anchors=%d, want 1", anchors)
	}
	report["llm_calls"] = hooks.LLMCalls
	if hooks.LLMCalls != 0 {
		t.Fatalf("llm_calls=%d, want 0", hooks.LLMCalls)
	}

	// Шаг 5: пустой синопсис — код 2, каталог мира не создан.
	empty := writeAncketa(t, t.TempDir(), "empty-synopsis", "")
	_, _, ec := runCmd(t, dir, bin, "world", "birth", "--ancketa", empty, "--fake-embed")
	report["empty_synopsis_exit"] = ec
	if ec != 2 {
		t.Fatalf("пустой синопсис: exit=%d, want 2", ec)
	}
	if _, err := os.Stat(filepath.Join(dir, "world_db", "worlds", "empty-synopsis")); !os.IsNotExist(err) {
		t.Fatal("каталог мира при пустом синопсисе создан, want отсутствует")
	}

	// Шаг 6: принудительный якорь — синопсис без близких нарративов, якорь всё
	// равно назначен с пометкой forced=true.
	forced := writeAncketa(t, t.TempDir(), "forced-anchor",
		"Странник ищет то, чего никогда не видел. Впереди только пустота. Никто не знает исхода.")
	fstdout, fstderr, fcode := runCmd(t, dir, bin, "world", "birth", "--ancketa", forced, "--fake-embed")
	if fcode != 0 {
		t.Fatalf("принудительный якорь: exit=%d, want 0\nstdout=%s\nstderr=%s", fcode, fstdout, fstderr)
	}
	fhooks := parseBirthReport(t, fstdout)
	report["forced_anchor"] = fhooks.AnchorForced
	if !fhooks.AnchorForced {
		t.Fatal("forced_anchor=false, want true")
	}

	writeReport(t, "int1", report)
}

// parseBirthReport находит JSON-строку итога рождения в stdout ядра.
func parseBirthReport(t *testing.T, stdout string) birthHooksReport {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var rep birthHooksReport
		if err := json.Unmarshal([]byte(line), &rep); err != nil {
			continue
		}
		return rep
	}
	t.Fatalf("в stdout нет JSON-итога рождения:\n%s", stdout)
	return birthHooksReport{}
}

func countCharacters(t *testing.T, worldDB string) int {
	t.Helper()
	d := openWorldDB(t, worldDB)
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM characters`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func distinctPairs(t *testing.T, worldDB string) int {
	t.Helper()
	d := openWorldDB(t, worldDB)
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM (SELECT DISTINCT psychotype, estate FROM characters)`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func countAnchors(t *testing.T, worldDB string) int {
	t.Helper()
	d := openWorldDB(t, worldDB)
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM world_meta WHERE "key" = 'anchor_narrative_id'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func openWorldDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	d, err := db.OpenReader(path)
	if err != nil {
		t.Fatalf("открыть базу мира %s: %v", path, err)
	}
	return d
}
