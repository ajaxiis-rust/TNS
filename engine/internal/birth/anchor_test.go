package birth

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/models"
	"tns65/internal/testkit"
)

func TestAnchorNearest(t *testing.T) {
	dir := t.TempDir()
	embed := testkit.NewFakeEmbed()
	ctx := context.Background()
	synVec := synopsisVec(t, embed)

	// Два нарратива: близкий (косинус 1.0) и далёкий (косинус -1.0).
	baseDB := writeAnchorBaseDB(t, filepath.Join(dir, "base.db"), [][]float32{synVec, negate(synVec)})

	got, err := selectAnchor(ctx, embed, baseDB, validAncketa().Synopsis, 0)
	if err != nil {
		t.Fatalf("selectAnchor: %v", err)
	}
	if got.forced {
		t.Fatal("forced = true, want false (есть близкий нарратив)")
	}
	if got.narrativeID != 1 {
		t.Fatalf("якорь = %d, want 1 (ближайший нарратив)", got.narrativeID)
	}

	// Сквозной путь: якорь записан в world_meta ровно один раз.
	res, err := Birth(validAncketa(), Config{WorldsRoot: dir, BaseDBPath: baseDB, Embed: embed})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}
	ids := anchorIDs(t, res.WorldDB)
	if len(ids) != 1 {
		t.Fatalf("якорей в world_meta: %d, want 1", len(ids))
	}
	if ids[0] != 1 {
		t.Fatalf("якорь в world_meta = %d, want 1", ids[0])
	}
}

func TestAnchorForced(t *testing.T) {
	dir := t.TempDir()
	embed := testkit.NewFakeEmbed()
	ctx := context.Background()
	synVec := synopsisVec(t, embed)

	// Набор без близких: косинусы -1.0 и ≈0.3 — оба ниже порога 0.5.
	far1 := negate(synVec)
	far2 := combine(synVec, orthUnit(synVec), 0.3, 0.95)
	baseDB := writeAnchorBaseDB(t, filepath.Join(dir, "base.db"), [][]float32{far1, far2})

	got, err := selectAnchor(ctx, embed, baseDB, validAncketa().Synopsis, 0)
	if err != nil {
		t.Fatalf("selectAnchor: %v", err)
	}
	if !got.forced {
		t.Fatal("forced = false, want true (близких нет)")
	}
	if got.narrativeID != 2 {
		t.Fatalf("якорь = %d, want 2 (ближайший из далёких)", got.narrativeID)
	}

	// Пометка forced=true попадает в журнал рождения.
	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Birth(validAncketa(), Config{WorldsRoot: dir, BaseDBPath: baseDB, Embed: embed, Logger: logger})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}
	if ids := anchorIDs(t, res.WorldDB); len(ids) != 1 || ids[0] != 2 {
		t.Fatalf("якорь в world_meta = %v, want [2]", ids)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(raw)
	if !strings.Contains(log, journal.EventAnchorForced) {
		t.Fatalf("журнал не содержит пометку %q: %s", journal.EventAnchorForced, log)
	}
	if !strings.Contains(log, `"key":"forced","value":"true"`) {
		t.Fatalf("журнал не содержит forced=true: %s", log)
	}
}

func TestAnchorNoLLM(t *testing.T) {
	dir := t.TempDir()
	embed := testkit.NewFakeEmbed()
	slot := testkit.NewFakeSlot() // слот генерации: подбор якоря его не должен вызывать.
	baseDB := testBaseDB(t, dir)

	res, err := Birth(validAncketa(), Config{WorldsRoot: dir, BaseDBPath: baseDB, Embed: embed})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}
	if embed.Calls() != 1 {
		t.Fatalf("вызовов эмбеддера: %d, want 1 (только синопсис)", embed.Calls())
	}
	if slot.Calls() != 0 {
		t.Fatalf("вызовов слота генерации: %d, want 0", slot.Calls())
	}
	if ids := anchorIDs(t, res.WorldDB); len(ids) != 1 {
		t.Fatalf("якорей в world_meta: %d, want 1", len(ids))
	}
}

// testBaseDB — фикстурная base.db с одним нарративом и вектором dim=1024
// (T-02-07, шаг 2): юнит-тесты рождения не зависят от запечённого сида T-01-16.
func testBaseDB(t *testing.T, dir string) string {
	t.Helper()
	vec := make([]float32, models.EmbedDim)
	vec[0] = 1
	return writeAnchorBaseDB(t, filepath.Join(dir, "base.db"), [][]float32{vec})
}

// writeAnchorBaseDB создаёт base.db с нарративами и их векторами из vecs и
// возвращает путь. Векторы обязаны иметь dim=1024.
func writeAnchorBaseDB(t *testing.T, path string, vecs [][]float32) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := db.Migrate(w, "base", "shared"); err != nil {
		t.Fatal(err)
	}
	for i, v := range vecs {
		if len(v) != models.EmbedDim {
			t.Fatalf("вектор фикстуры %d dim=%d, want %d", i+1, len(v), models.EmbedDim)
		}
		res, err := w.Exec(`INSERT INTO narratives (name, arc_summary, lang, created_at) VALUES (?,?,?,?)`,
			fmt.Sprintf("n%d", i+1), fmt.Sprintf("arc %d", i+1), "ru", "2026-01-01T00:00:00Z")
		if err != nil {
			t.Fatal(err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Exec(`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?,?,?,?,?,?,?,?)`,
			"narratives", id, "bge-m3", models.EmbedDim, "ru", fmt.Sprintf("arc %d", i+1), float32Blob(v), "2026-01-01T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// anchorIDs читает значения якоря из world_meta.
func anchorIDs(t *testing.T, worldDB string) []int64 {
	t.Helper()
	d := openReader(t, worldDB)
	rows, err := d.Query(`SELECT value FROM world_meta WHERE "key" = ?`, anchorKey)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			t.Fatalf("значение якоря %q не число: %v", raw, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

// synopsisVec — вектор синопсиса анкеты через подставной эмбеддер; dim=1024.
func synopsisVec(t *testing.T, embed models.Embedder) []float32 {
	t.Helper()
	vec, err := embed.Embed(context.Background(), validAncketa().Synopsis)
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != models.EmbedDim {
		t.Fatalf("вектор синопсиса dim=%d, want %d", len(vec), models.EmbedDim)
	}
	return vec
}

func float32Blob(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func negate(v []float32) []float32 {
	out := make([]float32, len(v))
	for i, f := range v {
		out[i] = -f
	}
	return out
}

// orthUnit — единичный вектор, ортогональный v.
func orthUnit(v []float32) []float32 {
	n := len(v)
	out := make([]float32, n)
	i := 0
	for i < n && v[i] == 0 {
		i++
	}
	j := i + 1
	if j >= n {
		j = 0
	}
	out[i] = v[j]
	out[j] = -v[i]
	return normalize(out)
}

// combine — нормированная сумма wa*a + wb*b.
func combine(a, b []float32, wa, wb float32) []float32 {
	out := make([]float32, len(a))
	for i := range a {
		out[i] = wa*a[i] + wb*b[i]
	}
	return normalize(out)
}

func normalize(v []float32) []float32 {
	var sumSq float64
	for _, f := range v {
		sumSq += float64(f) * float64(f)
	}
	norm := float32(math.Sqrt(sumSq))
	if norm == 0 {
		v[0] = 1
		return v
	}
	for i := range v {
		v[i] /= norm
	}
	return v
}
