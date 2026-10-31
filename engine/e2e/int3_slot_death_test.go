//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/journal"
)

// TestINT3 — INT-3. Смерть внешнего слота (T-09-03): первый ход со слотом Ткача
// успешен; после обрыва провайдера второй ход доставлен запаской, путь fallback,
// деградация видна в журнале и мониторинге, запаска короче штатной и без маркера
// глубокой сцены.
func TestINT3(t *testing.T) {
	bin := buildBinary(t, []string{"e2e"})
	dir := worldDir(t)
	linkConfig(t, dir)
	linkPacks(t, dir)

	// Подставной внешний провайдер: /completion стримит штатную прозу с маркером
	// глубокой сцены. Закрытие сервера имитирует смерть внешнего слота.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/completion" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		for _, line := range deepSceneProse() {
			fmt.Fprintln(w, line)
		}
	}))

	slug := "smert-vneshnego-slota"
	ancketa := writeAncketa(t, t.TempDir(), slug,
		"Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.")
	stdout, stderr, code := runCmd(t, dir, bin, "world", "birth", "--ancketa", ancketa, "--fake-embed", "--fake-slot")
	if code != 0 {
		t.Fatalf("world birth exit=%d, want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	worldDB := worldDBPath(dir, slug)

	c := startCore(t, dir, bin, "--fake-slot", srv.URL)

	// Шаг 2: первый ход с внешним слотом — успех; затем оборвать провайдер и
	// подать вторую реплику.
	const replica = "Иду, еду, плыву, бегу."
	c.postTurn(t, slug, replica)
	srv.Close()
	c.postTurn(t, slug, replica)

	report := map[string]any{}

	// Шаг 3: ответ доставлен, ход не прерван, путь помечен деградированным, в
	// журнале slot.degraded (child.degraded и slot.degraded — разные имена).
	turns := lastTurns(t, worldDB, 2)
	if len(turns) < 2 {
		t.Fatalf("ходов в turns = %d, want ≥ 2", len(turns))
	}
	normalReply := turns[0].ReplyText
	fallbackReply := turns[1].ReplyText
	report["delivered"] = fallbackReply != ""
	report["path"] = turns[1].Path
	if !report["delivered"].(bool) {
		t.Fatal("delivered=false: ответ второго хода пуст")
	}
	if report["path"] != "fallback" {
		t.Fatalf("path=%q, want fallback", turns[1].Path)
	}

	journalText := readJournal(t, dir)
	if !strings.Contains(journalText, journal.EventSlotDegraded) {
		t.Fatalf("в журнале нет slot.degraded:\n%s", journalText)
	}
	report["degraded_events_distinct"] = journal.EventChildDegraded != journal.EventSlotDegraded
	if !report["degraded_events_distinct"].(bool) {
		t.Fatal("child.degraded и slot.degraded не различимы")
	}

	// Шаг 4: мониторинг — счётчик деградаций вырос, блок деградаций есть в ответе.
	degradations := monitorDegradations(t, c)
	report["degradation_counter"] = degradations
	if degradations != 1 {
		t.Fatalf("degradation_counter=%d, want 1", degradations)
	}

	// Шаг 5: честность деградации числами — запаска ≤ 0.6 штатной и без маркера
	// глубокой сцены.
	report["fallback_shorter"] = float64(len(fallbackReply)) <= 0.6*float64(len(normalReply))
	if !report["fallback_shorter"].(bool) {
		t.Fatalf("запаска не короче штатной: len(fallback)=%d, len(normal)=%d", len(fallbackReply), len(normalReply))
	}
	report["deep_scene"] = strings.Contains(fallbackReply, "scene_depth=deep")
	if report["deep_scene"].(bool) {
		t.Fatal("деградированный ответ содержит маркер глубокой сцены")
	}

	writeReport(t, "int3", report)
}

// deepSceneProse — штатная проза внешнего слота: длинная, с маркером глубины.
func deepSceneProse() []string {
	return []string{
		"Ты ступаешь по тропе, и мир вокруг оживает: ветер трогает траву, тени тянутся следом за тобой.",
		"Каждый шаг отдаётся эхом в груди, дорога помнит всех, кто проходил здесь до тебя, и хранит их следы.",
		"Впереди расстилается долина, над ней висит низкое небо цвета олова, и далёкие вершины зовут вперёд.",
		"scene_depth=deep",
		"Ты чувствуешь, как сцена набирает глубину: лица в толпе поворачиваются к тебе, но никто не произносит ни слова.",
	}
}

// turnRow — строка turns для сверки пути и ответа хода.
type turnRow struct {
	ReplyText string
	Path      string
}

// lastTurns возвращает последние n ходов в хронологическом порядке.
func lastTurns(t *testing.T, worldDB string, n int) []turnRow {
	t.Helper()
	h := openWorldDB(t, worldDB)
	defer h.Close()
	rows, err := h.Query(`SELECT reply_text, path FROM turns ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var rev []turnRow
	for rows.Next() {
		var tr turnRow
		if err := rows.Scan(&tr.ReplyText, &tr.Path); err != nil {
			t.Fatal(err)
		}
		rev = append(rev, tr)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	out := make([]turnRow, len(rev))
	for i := range rev {
		out[i] = rev[len(rev)-1-i]
	}
	return out
}

// readJournal читает журнал станка ядра.
func readJournal(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "state", "journal", "engine.log"))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	return string(data)
}

// monitorDegradations — счётчик деградаций из GET /api/monitor.
func monitorDegradations(t *testing.T, c *core) int64 {
	t.Helper()
	resp, err := c.client.Get(c.url + "/api/monitor")
	if err != nil {
		t.Fatalf("GET /api/monitor: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/monitor status %d, want 200", resp.StatusCode)
	}
	var out struct {
		Degradations int64    `json:"degradations"`
		JournalTail  []string `json:"journal_tail"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode monitor: %v", err)
	}
	return out.Degradations
}

// linkPacks кладёт языковой пакет в рабочий каталог ядра — лексикон предпути
// читается по пути packs/ru относительно CWD.
func linkPacks(t *testing.T, dir string) {
	t.Helper()
	root := engineRoot(t)
	if _, err := os.Stat(filepath.Join(root, "packs")); err != nil {
		t.Fatalf("packs отсутствует в %s", root)
	}
	if err := os.Symlink(filepath.Join(root, "packs"), filepath.Join(dir, "packs")); err != nil {
		t.Fatalf("symlink packs: %v", err)
	}
}
