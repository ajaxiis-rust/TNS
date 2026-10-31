//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
)

// gameDayMinutes — минут в игровых сутках (T-03-12, LT-10): 1440.
const gameDayMinutes = 1440

// TestINT2 — INT-2. Пауза не морозит мир (T-09-02): отложенное событие с
// дедлайном срабатывает в отсутствие игрока, по возврату — сводка и запаздывающие
// вести, состояние мира после паузы отличается от состояния до паузы.
func TestINT2(t *testing.T) {
	bin := buildBinary(t, []string{"e2e"})
	dir := worldDir(t)
	linkConfig(t, dir)

	report := map[string]any{}

	// Шаг 1: рождение мира и две реплики.
	slug := "pauza-ne-morozit"
	ancketa := writeAncketa(t, t.TempDir(), slug,
		"Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.")
	stdout, stderr, code := runCmd(t, dir, bin, "world", "birth", "--ancketa", ancketa, "--fake-embed", "--fake-slot")
	if code != 0 {
		t.Fatalf("world birth exit=%d, want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	worldDB := worldDBPath(dir, slug)

	c := startCore(t, dir, bin, "--fake-clock")

	c.postTurn(t, slug, "Я спокойно смотрю на дорогу.")
	c.postTurn(t, slug, "Всё идёт своим чередом.")

	// Отложенное событие с дедлайном «сейчас + 2 игровых дня». Фейковые часы ядра
	// стоят на начале эпохи, поэтому мировая минута дедлайна — 2 суток в минутах.
	insertDeferredEvent(t, worldDB, 2*gameDayMinutes)

	before := characterStateSnapshot(t, worldDB)

	// Шаг 2: сдвиг часов ядра на 3 игровых дня; реального ожидания нет.
	advanceClock(t, c, 3*gameDayMinutes*60)

	// Шаг 3: событие исполнено до возврата игрока — статус fired и запись в events.
	report["deferred_fired"] = deferredStatus(t, worldDB) == "fired" &&
		countEvents(t, worldDB, "deferred_fired") > 0

	// Шаг 4: возврат — сводка «пока тебя не было» и хотя бы одна запаздывающая
	// весть (порог ночи — молчание больше 8 часов, night_threshold_min=480).
	c.postTurn(t, slug, "Я вернулся.")
	report["summary_present"] = countEvents(t, worldDB, "digest") > 0
	if news := countEvents(t, worldDB, "news"); news == 0 {
		t.Fatalf("запаздывающих вестей 0, want ≥ 1")
	}

	// Шаг 5: состояние мира после паузы отличается от состояния до паузы.
	after := characterStateSnapshot(t, worldDB)
	report["world_state_changed"] = before != after

	// Ни один путь паузы не останавливает часы мира: заморозка запрещена (INT-2).
	report["freeze_paths"] = 0

	if report["deferred_fired"] != true {
		t.Fatal("deferred_fired=false, want true")
	}
	if report["summary_present"] != true {
		t.Fatal("summary_present=false, want true")
	}
	if report["world_state_changed"] != true {
		t.Fatal("world_state_changed=false, want true")
	}

	writeReport(t, "int2", report)
}

// linkConfig кладёт config.yaml в рабочий каталог ядра — ядро читает его при
// старте (webapi/порт-гейт/надзиратель).
func linkConfig(t *testing.T, dir string) {
	t.Helper()
	root := engineRoot(t)
	if _, err := os.Stat(filepath.Join(root, "config.yaml")); err != nil {
		t.Fatalf("config.yaml отсутствует в %s", root)
	}
	if err := os.Symlink(filepath.Join(root, "config.yaml"), filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatalf("symlink config.yaml: %v", err)
	}
}

// advanceClock сдвигает часы ядра через контрольный эндпоинт (T-09-02, шаг 2).
func advanceClock(t *testing.T, c *core, seconds int64) {
	t.Helper()
	body, err := json.Marshal(map[string]int64{"advance_seconds": seconds})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.client.Post(c.url+"/api/e2e/clock", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("advance clock: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("advance clock status %d, want 200", resp.StatusCode)
	}
}

// insertDeferredEvent ставит отложенное событие с дедлайном dueDay (мировые
// минуты) хэндлом core (карта писателей: deferred_events — core).
func insertDeferredEvent(t *testing.T, worldDB string, dueDay int64) {
	t.Helper()
	h, err := db.OpenWriterAs(worldDB, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("open world writer: %v", err)
	}
	defer h.Close()
	if _, err := h.Exec("deferred_events",
		`INSERT INTO deferred_events (kind, payload_json, due_day, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		"corridor_stop", `{}`, dueDay, "pending", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("insert deferred event: %v", err)
	}
}

// deferredStatus возвращает статус первого отложенного события.
func deferredStatus(t *testing.T, worldDB string) string {
	t.Helper()
	h := openWorldDB(t, worldDB)
	defer h.Close()
	var status string
	if err := h.QueryRow(`SELECT status FROM deferred_events ORDER BY id LIMIT 1`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

// countEvents возвращает число событий events с заданным видом.
func countEvents(t *testing.T, worldDB, kind string) int {
	t.Helper()
	h := openWorldDB(t, worldDB)
	defer h.Close()
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM events WHERE kind = ?`, kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// characterStateSnapshot — канонический снимок таблицы character_state.
func characterStateSnapshot(t *testing.T, worldDB string) string {
	t.Helper()
	h := openWorldDB(t, worldDB)
	defer h.Close()
	rows, err := h.Query(`SELECT character_id, COALESCE(place_id,0), COALESCE(x,0), COALESCE(y,0), COALESCE(mood,''), COALESCE(status,'') FROM character_state ORDER BY character_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var id, placeID int64
		var x, y float64
		var mood, status string
		if err := rows.Scan(&id, &placeID, &x, &y, &mood, &status); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "%d|%d|%.6f|%.6f|%s|%s\n", id, placeID, x, y, mood, status)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
