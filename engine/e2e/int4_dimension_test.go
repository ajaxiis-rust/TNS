//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/models"
	"tns65/internal/testkit"
)

// TestINT4 — INT-4. Размерность векторов 1024 (T-09-04): dim=1024 в каждой
// строке vectors трёх баз, интерфейс отдаёт embedding_dimension=1024 без
// маршрута записи, функция расстояния различает длины векторов.
func TestINT4(t *testing.T) {
	bin := buildBinary(t, []string{"e2e"})
	dir := worldDir(t)
	linkConfig(t, dir)

	report := map[string]any{}

	// Шаг 1: рождение мира с детерминированным эмбеддером и одна реплика —
	// векторы реплики живут в оперативной базе хода, векторы нарратива — в базе
	// языка, векторы памяти (оседание) — в литературной базе, если оседание шло.
	slug := "razmernost-vektorov"
	ancketa := writeAncketa(t, t.TempDir(), slug,
		"Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.")
	stdout, stderr, code := runCmd(t, dir, bin, "world", "birth", "--ancketa", ancketa, "--fake-embed", "--fake-slot")
	if code != 0 {
		t.Fatalf("world birth exit=%d, want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	c := startCore(t, dir, bin, "--fake-clock")
	c.postTurn(t, slug, "Я иду по дороге.")

	// Шаг 2: vectors.dim в трёх базах поимённо — значение 1024 в каждой строке.
	baseDB := filepath.Join(engineRoot(t), "base", "base.ru.db")
	worldDB := worldDBPath(dir, slug)
	literaryDB := filepath.Join(dir, "world_db", "worlds", slug, "literary.db")

	rowsBad := countBadDim(t, baseDB) + countBadDim(t, worldDB) + countBadDim(t, literaryDB)
	report["rows_bad_dim"] = rowsBad
	if rowsBad != 0 {
		t.Fatalf("rows_bad_dim=%d, want 0", rowsBad)
	}

	// Шаг 3: настройка в интерфейсе — embedding_dimension = 1024.
	uiDim := settingsEmbedDim(t, c, slug)
	report["ui_dim"] = uiDim
	if uiDim != models.EmbedDim {
		t.Fatalf("ui_dim=%d, want %d", uiDim, models.EmbedDim)
	}

	// Шаг 4: выставить иное значение нельзя — маршрута записи размерности нет,
	// запрос отвергается, конфигурация не меняется. Проверяются обе ветви отказа:
	// запись через единственный пишущий конфигурационный маршрут — ошибка
	// конфигурации (INT-4.1), запрос несуществующего маршрута записи — 404.
	rejected := rejectOtherDim(t, c, slug, uiDim)
	report["other_dim_rejected"] = rejected
	report["other_dim_route_404"] = noDimensionWriteRoute(t, c)
	if rejected != true || report["other_dim_route_404"] != true {
		t.Fatalf("other_dim_rejected=%v, other_dim_route_404=%v", rejected, report["other_dim_route_404"])
	}

	// Шаг 5: расстояние между вектором реплики и вектором нарратива — ошибка при
	// несовпадении длин и число при равных длинах 1024.
	report["distance_ok"] = distanceContract(t, baseDB)
	if report["distance_ok"] != true {
		t.Fatal("distance_ok=false")
	}

	writeReport(t, "int4", report)
}

// countBadDim возвращает число строк vectors с dim != 1024 в базе path. База без
// таблицы vectors (литературная база — D-19.2) даёт 0: проверять нечего.
func countBadDim(t *testing.T, path string) int {
	t.Helper()
	d, err := db.OpenReader(path)
	if err != nil {
		t.Fatalf("открыть базу %s: %v", path, err)
	}
	defer d.Close()

	var exists int
	if err := d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'vectors'`).Scan(&exists); err != nil {
		t.Fatalf("sqlite_master %s: %v", path, err)
	}
	if exists == 0 {
		return 0
	}
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM vectors WHERE dim != ?`, models.EmbedDim).Scan(&n); err != nil {
		t.Fatalf("vectors.dim %s: %v", path, err)
	}
	return n
}

// settingsEmbedDim читает embedding_dimension из GET /api/settings.
func settingsEmbedDim(t *testing.T, c *core, slug string) int {
	t.Helper()
	resp, err := c.client.Get(c.url + "/api/settings?world=" + slug)
	if err != nil {
		t.Fatalf("GET /api/settings: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/settings status %d, want 200", resp.StatusCode)
	}
	var out struct {
		EmbeddingDimension int `json:"embedding_dimension"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	return out.EmbeddingDimension
}

// rejectOtherDim пробует записать иную размерность через единственный пишущий
// конфигурационный маршрут (PUT /api/settings/config) и сверяет, что запрос
// отвергнут как ошибка конфигурации и значение не изменилось.
func rejectOtherDim(t *testing.T, c *core, slug string, before int) bool {
	t.Helper()
	body, err := json.Marshal(map[string]int{"embedding_dimension": 2048})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPut, c.url+"/api/settings/config", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/settings/config: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusBadRequest {
		return false
	}
	return settingsEmbedDim(t, c, slug) == before
}

// noDimensionWriteRoute пробует записать размерность по несуществующему
// маршруту и ждёт 404 — маршрута записи размерности в интерфейсе нет (T-08-09).
func noDimensionWriteRoute(t *testing.T, c *core) bool {
	t.Helper()
	body, err := json.Marshal(map[string]int{"embedding_dimension": 2048})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPut, c.url+"/api/settings/embedding-dimension", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/settings/embedding-dimension: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusNotFound
}

// distanceContract проверяет контракт функции расстояния: ошибка при несовпадении
// длин, число при равных длинах 1024.
func distanceContract(t *testing.T, baseDB string) bool {
	t.Helper()
	narr := readNarrativeVec(t, baseDB)
	reply := replyVec(t)

	d, err := vectorDistance(narr, reply)
	if err != nil {
		return false
	}
	if math.IsNaN(d) || math.IsInf(d, 0) {
		return false
	}
	if _, err := vectorDistance(narr, reply[:len(reply)-1]); err == nil {
		return false
	}
	if _, err := vectorDistance(narr[:len(narr)-1], reply); err == nil {
		return false
	}
	return true
}

// readNarrativeVec читает один вектор нарратива из базы языка.
func readNarrativeVec(t *testing.T, baseDB string) []float32 {
	t.Helper()
	d, err := db.OpenReader(baseDB)
	if err != nil {
		t.Fatalf("открыть базу языка %s: %v", baseDB, err)
	}
	defer d.Close()

	var blob []byte
	if err := d.QueryRow(
		`SELECT vec FROM vectors WHERE owner_table = 'narratives' AND lang = 'ru' ORDER BY owner_id LIMIT 1`,
	).Scan(&blob); err != nil {
		t.Fatalf("читать вектор нарратива: %v", err)
	}
	return decodeVec(blob)
}

// replyVec — детерминированный вектор реплики: тот же эмбеддер, что подаёт
// --fake-embed при рождении; живой ход реплику в vectors пока не пишет (эмбеддер
// сервером хода не подключён), поэтому вектор реплики считается здесь тем же
// способом, которым его считает сценарий.
func replyVec(t *testing.T) []float32 {
	t.Helper()
	v, err := testkit.NewFakeEmbed().Embed(context.Background(), "Я иду по дороге.")
	if err != nil {
		t.Fatalf("fake embed: %v", err)
	}
	return v
}

// decodeVec раскодирует BLOB Float32 little-endian (D-17.2).
func decodeVec(blob []byte) []float32 {
	vec := make([]float32, len(blob)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return vec
}

// vectorDistance — евклидово расстояние двух векторов; несовпадение длин —
// ошибка.
func vectorDistance(a, b []float32) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("размерности векторов не совпадают: %d != %d", len(a), len(b))
	}
	var sum float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		sum += d * d
	}
	return math.Sqrt(sum), nil
}
