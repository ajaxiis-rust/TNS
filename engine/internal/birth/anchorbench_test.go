package birth

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"tns65/internal/db"
	"tns65/internal/embedserver"
	"tns65/internal/models"
	"tns65/internal/testkit"
)

// anchorBenchSetPath — файл состава золотого набора относительно каталога пакета.
const anchorBenchSetPath = "testdata/anchorbench/set.json"

// anchorBenchSet — золотой набор «анкета → ожидаемый якорь» (T-17-03, B-08 п.2):
// пул нарративов и ожидания сняты с базы движка base.ru.db.
type anchorBenchSet struct {
	Version    int              `json:"version"`
	Lang       string           `json:"lang"`
	Source     string           `json:"source"`
	Note       string           `json:"note"`
	Narratives []benchNarrative `json:"narratives"`
	Cases      []benchCase      `json:"cases"`
}

// benchNarrative — нарратив пула: id — id нарратива в базе движка.
type benchNarrative struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	ArcSummary string `json:"arc_summary"`
}

// benchCase — анкета набора и её якоря: expect — семантическое ожидание для
// живого режима (отчёт), stub — воспроизводимый якорь заглушки (контракт).
type benchCase struct {
	Slug   string  `json:"slug"`
	Anketa Ancketa `json:"anketa"`
	Expect int64   `json:"expect"`
	Stub   int64   `json:"stub"`
	Note   string  `json:"note"`
}

// countingEmbedder — счётчик вызовов: подбор якоря обязан тратить один
// эмбеддинг на анкету, векторы нарративов читаются из базы (B-08 п.3).
type countingEmbedder struct {
	inner models.Embedder
	mu    sync.Mutex
	calls int
}

func (e *countingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	return e.inner.Embed(ctx, text)
}

func (e *countingEmbedder) Calls() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

// TestAnchorBench — бенч подбора якоря (T-17-03) в двух режимах: заглушка
// testkit проверяет контракт и воспроизводимость набора и входит в проверку;
// живой эмбеддер станции даёт отчёт с числами и провала не даёт.
func TestAnchorBench(t *testing.T) {
	set := loadAnchorBenchSet(t)
	checkAnchorBenchSet(t, set)

	t.Run("stub", func(t *testing.T) { runAnchorBenchStub(t, set) })
	t.Run("live", func(t *testing.T) { runAnchorBenchLive(t, set) })
}

// loadAnchorBenchSet читает состав набора из testdata пакета.
func loadAnchorBenchSet(t *testing.T) anchorBenchSet {
	t.Helper()
	raw, err := os.ReadFile(anchorBenchSetPath)
	if err != nil {
		t.Fatalf("набор %s: %v", anchorBenchSetPath, err)
	}
	var set anchorBenchSet
	if err := json.Unmarshal(raw, &set); err != nil {
		t.Fatalf("набор %s: разбор: %v", anchorBenchSetPath, err)
	}
	return set
}

// checkAnchorBenchSet сверяет состав набора: пул, анкеты и заявленные якоря.
func checkAnchorBenchSet(t *testing.T, set anchorBenchSet) {
	t.Helper()
	if set.Version != 1 {
		t.Fatalf("версия набора %d, want 1", set.Version)
	}
	if set.Lang == "" || set.Source == "" {
		t.Fatalf("набор не называет язык или источник: lang=%q source=%q", set.Lang, set.Source)
	}
	if len(set.Narratives) < 5 {
		t.Fatalf("нарративов в пуле: %d, want ≥ 5", len(set.Narratives))
	}
	if len(set.Cases) < 5 {
		t.Fatalf("анкет в наборе: %d, want ≥ 5", len(set.Cases))
	}

	pool := make(map[int64]bool, len(set.Narratives))
	for _, n := range set.Narratives {
		if n.ID <= 0 || n.Name == "" || strings.TrimSpace(n.ArcSummary) == "" {
			t.Fatalf("нарратив пула %+v: нужны id, name и arc_summary", n)
		}
		if pool[n.ID] {
			t.Fatalf("нарратив %d в пуле дважды", n.ID)
		}
		pool[n.ID] = true
	}

	slugs := make(map[string]bool, len(set.Cases))
	for _, c := range set.Cases {
		if c.Slug == "" || slugs[c.Slug] {
			t.Fatalf("слаг анкеты %q пуст или повторён", c.Slug)
		}
		slugs[c.Slug] = true
		if strings.TrimSpace(c.Anketa.Synopsis) == "" {
			t.Fatalf("анкета %s: пустой синопсис", c.Slug)
		}
		if c.Anketa.Lang != set.Lang {
			t.Fatalf("анкета %s: язык %q, want %q", c.Slug, c.Anketa.Lang, set.Lang)
		}
		for _, want := range []int64{c.Expect, c.Stub} {
			if !pool[want] {
				t.Fatalf("анкета %s: якорь %d вне пула набора", c.Slug, want)
			}
		}
	}
}

// runAnchorBenchStub — режим заглушки: контракт подбора и воспроизводимость набора.
func runAnchorBenchStub(t *testing.T, set anchorBenchSet) {
	t.Helper()
	ctx := context.Background()
	stub := &countingEmbedder{inner: testkit.NewFakeEmbed()}
	baseDB := writeAnchorBenchBaseDB(t, ctx, filepath.Join(t.TempDir(), "base."+set.Lang+".db"), set, stub)

	// Пул читается тем же путём, что и в живом рождении: только на чтение.
	vecs, err := readNarrativeVectors(baseDB, set.Lang)
	if err != nil {
		t.Fatalf("векторы пула: %v", err)
	}
	if len(vecs) != len(set.Narratives) {
		t.Fatalf("векторов пула: %d, want %d", len(vecs), len(set.Narratives))
	}

	forced := 0
	first := make(map[string]anchorResult, len(set.Cases))
	for _, c := range set.Cases {
		before := stub.Calls()
		got, err := selectAnchor(ctx, stub, baseDB, c.Anketa.Lang, c.Anketa.Synopsis, 0)
		if err != nil {
			t.Fatalf("анкета %s: selectAnchor: %v", c.Slug, err)
		}
		if calls := stub.Calls() - before; calls != 1 {
			t.Fatalf("анкета %s: вызовов эмбеддера %d, want 1", c.Slug, calls)
		}
		if got.narrativeID != c.Stub {
			t.Fatalf("анкета %s: якорь заглушки %d, в наборе зафиксирован %d — обновите поле stub", c.Slug, got.narrativeID, c.Stub)
		}
		if got.forced {
			forced++
		}

		// Контракт B-08 п.2: якорь — ближайший нарратив пула по косинусу.
		syn, err := testkit.NewFakeEmbed().Embed(ctx, c.Anketa.Synopsis)
		if err != nil {
			t.Fatalf("анкета %s: вектор синопсиса: %v", c.Slug, err)
		}
		wantID, wantSim := nearestNarrative(vecs, syn)
		if got.narrativeID != wantID {
			t.Fatalf("анкета %s: якорь %d, ближайший по косинусу %d", c.Slug, got.narrativeID, wantID)
		}
		if got.forced != (wantSim < defaultMatchThreshold) {
			t.Fatalf("анкета %s: forced=%v при косинусе %.3f и пороге %.2f", c.Slug, got.forced, wantSim, defaultMatchThreshold)
		}
		first[c.Slug] = got
	}

	// Воспроизводимость набора: второй прогон по той же базе даёт те же якоря.
	for _, c := range set.Cases {
		got, err := selectAnchor(ctx, stub, baseDB, c.Anketa.Lang, c.Anketa.Synopsis, 0)
		if err != nil {
			t.Fatalf("анкета %s, повтор: selectAnchor: %v", c.Slug, err)
		}
		if got != first[c.Slug] {
			t.Fatalf("анкета %s: повтор дал %+v, первый прогон %+v", c.Slug, got, first[c.Slug])
		}
	}

	t.Logf("заглушка: анкет %d, пул %d, якорей совпало %d, forced %d, порог %.2f",
		len(set.Cases), len(set.Narratives), len(first), forced, defaultMatchThreshold)
}

// runAnchorBenchLive — живой режим: отчёт по эмбеддеру станции; недоступность
// не провал, а помета в реестр и отложенная калибровка.
func runAnchorBenchLive(t *testing.T, set anchorBenchSet) {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d", embedserver.DefaultPort)

	probeCtx, cancelProbe := context.WithTimeout(context.Background(), 3*time.Second)
	status := embedserver.StatusOf(probeCtx, url, nil)
	cancelProbe()
	if !status.Healthy || !status.PoolingOK {
		reason := status.Error
		if reason == "" {
			reason = "нет ответа"
		}
		t.Logf("живой режим: эмбеддер станции %s недоступен (%s) — калибровка отложена, помета в реестр", url, reason)
		return
	}

	client, err := models.NewClient(url, nil)
	if err != nil {
		t.Logf("живой режим: клиент эмбеддера %s: %v — пропуск", url, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	baseDB := writeAnchorBenchBaseDB(t, ctx, filepath.Join(t.TempDir(), "base."+set.Lang+".db"), set, client)
	vecs, err := readNarrativeVectors(baseDB, set.Lang)
	if err != nil {
		t.Logf("живой режим: векторы пула: %v — пропуск", err)
		return
	}

	matches := 0
	for _, c := range set.Cases {
		got, err := selectAnchor(ctx, client, baseDB, c.Anketa.Lang, c.Anketa.Synopsis, 0)
		if err != nil {
			t.Logf("живой режим: анкета %s: %v", c.Slug, err)
			continue
		}
		syn, err := client.Embed(ctx, c.Anketa.Synopsis)
		if err != nil {
			t.Logf("живой режим: анкета %s: вектор синопсиса: %v", c.Slug, err)
			continue
		}
		_, sim := nearestNarrative(vecs, syn)
		hit := got.narrativeID == c.Expect
		if hit {
			matches++
		}
		t.Logf("живой режим: анкета %s, якорь %d (ожидался %d), косинус ближайшего %.3f, forced=%v, попадание=%v",
			c.Slug, got.narrativeID, c.Expect, sim, got.forced, hit)
	}
	t.Logf("живой режим: попаданий %d из %d, порог %.2f", matches, len(set.Cases), defaultMatchThreshold)
}

// writeAnchorBenchBaseDB собирает базу пула набора с id нарративов базы
// движка: векторы считает переданный эмбеддер, как при запекании (B-08 п.3).
func writeAnchorBenchBaseDB(t *testing.T, ctx context.Context, path string, set anchorBenchSet, embed models.Embedder) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(w, "base", "shared"); err != nil {
		_ = w.Close()
		t.Fatal(err)
	}
	const created = "2026-11-22T00:00:00Z"
	for _, n := range set.Narratives {
		vec, err := embed.Embed(ctx, n.ArcSummary)
		if err != nil {
			_ = w.Close()
			t.Fatalf("нарратив %d: вектор: %v", n.ID, err)
		}
		if len(vec) != models.EmbedDim {
			_ = w.Close()
			t.Fatalf("нарратив %d: dim=%d, want %d", n.ID, len(vec), models.EmbedDim)
		}
		if _, err := w.Exec(`INSERT INTO narratives (id, name, arc_summary, lang, created_at) VALUES (?,?,?,?,?)`,
			n.ID, n.Name, n.ArcSummary, set.Lang, created); err != nil {
			_ = w.Close()
			t.Fatalf("нарратив %d: %v", n.ID, err)
		}
		if _, err := w.Exec(`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?,?,?,?,?,?,?,?)`,
			"narratives", n.ID, models.DefaultEmbedModel, models.EmbedDim, set.Lang, n.ArcSummary, float32Blob(vec), created); err != nil {
			_ = w.Close()
			t.Fatalf("вектор нарратива %d: %v", n.ID, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// nearestNarrative возвращает ближайший к syn нарратив пула и его косинус.
func nearestNarrative(vecs map[int64][]float32, syn []float32) (int64, float64) {
	best := int64(0)
	bestSim := math.Inf(-1)
	for id, vec := range vecs {
		if sim := cosine(syn, vec); sim > bestSim {
			best, bestSim = id, sim
		}
	}
	return best, bestSim
}
