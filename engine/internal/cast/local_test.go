//go:build local

package cast_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tns65/internal/cast"
	"tns65/internal/supervisor"
	"tns65/internal/workshop"
)

// localMaxChunks — потолок локального теста (§8.1): карта строится не больше
// чем по двум кускам самой маленькой книги.
const localMaxChunks = 2

// localDeadline — дедлайн локального теста (§8.1): по истечении t.Fatal, не
// зависание.
const localDeadline = 5 * time.Minute

// localSkipModel — сообщение пропуска без локальной модели (§8.1).
const localSkipModel = "локальная модель не поднята, см. scripts/test-local.sh"

// localBookDir — каталог книг из TNS65_FB2_DIR; пусто — заводской.
func localBookDir() string {
	if d := os.Getenv("TNS65_FB2_DIR"); d != "" {
		return d
	}
	return "/media/ajaxiis/ext3/fb2"
}

// smallestFB2 находит самую маленькую книгу .fb2 каталога; TNS65_TEST_BOOK
// задаёт конкретное имя. Нет книг — пропуск с понятным сообщением.
func smallestFB2(t *testing.T, dir string) string {
	t.Helper()
	if name := os.Getenv("TNS65_TEST_BOOK"); name != "" {
		p := name
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, name)
		}
		if _, err := os.Stat(p); err != nil {
			t.Skipf("книга %s не найдена в %s", name, dir)
		}
		return p
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("каталог книг %s не найден (задайте TNS65_FB2_DIR): %v", dir, err)
	}
	var best string
	var bestSize int64 = -1
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".fb2" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if bestSize < 0 || info.Size() < bestSize {
			bestSize = info.Size()
			best = filepath.Join(dir, e.Name())
		}
	}
	if best == "" {
		t.Skipf("в каталоге %s нет книг .fb2", dir)
	}
	return best
}

// requireLocalModel пропускает тест, если локальные слоты :5001/:5002 не
// отвечают на /health (§8.1).
func requireLocalModel(t *testing.T) {
	t.Helper()
	for _, port := range []int{5001, 5002} {
		if err := supervisor.CheckHealth(supervisor.Config{Port: port, HealthPath: "/health"}); err != nil {
			t.Skip(localSkipModel)
		}
	}
}

// TestLocalCastMap — «Проверка» T-12-28 (карта, 3 героя): самая маленькая книга,
// не больше двух кусков, карта строится по трём героям; без модели — SKIP.
func TestLocalCastMap(t *testing.T) {
	requireLocalModel(t)
	book := smallestFB2(t, localBookDir())

	ctx, cancel := context.WithTimeout(context.Background(), localDeadline)
	defer cancel()

	parsed, err := workshop.ParseBookInput(book)
	if err != nil {
		t.Fatalf("ParseBookInput(%s): %v", book, err)
	}
	if len(parsed.Chunks) == 0 {
		t.Fatalf("книга %s без кусков", book)
	}
	if len(parsed.Chunks) > localMaxChunks {
		t.Logf("чанкер вернул %d кусков, режу до потолка %d", len(parsed.Chunks), localMaxChunks)
		parsed.Chunks = parsed.Chunks[:localMaxChunks]
	}

	chunks := make([]cast.Chunk, 0, len(parsed.Chunks))
	for i, c := range parsed.Chunks {
		chunks = append(chunks, cast.Chunk{Ord: int64(i + 1), Chapter: c.Chapter, Text: c.Text})
	}

	cands, err := cast.Scan(chunks, cast.ScanOptions{Profile: ruProfile(t)})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(cands) == 0 {
		t.Fatalf("скан %s не нашёл героев", book)
	}
	cands = capTestCharacters(t, cands)
	if len(cands) > maxTestCharacters {
		t.Fatalf("героев после потолка = %d, want ≤ %d", len(cands), maxTestCharacters)
	}

	described, err := cast.Describe(chunks, cands, loadRuDict(t), cast.DescribeOptions{})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if len(described) != len(cands) {
		t.Fatalf("описано %d героев, want %d", len(described), len(cands))
	}
	for _, c := range cands {
		if _, err := cast.ChunkRoles(chunks, c, cast.DefaultRoleDict(), cast.RoleOptions{}); err != nil {
			t.Fatalf("ChunkRoles(%s): %v", c.Name, err)
		}
	}

	if err := ctx.Err(); err != nil {
		t.Fatalf("дедлайн локального теста истёк: %v", err)
	}

	names := make([]string, 0, len(cands))
	for _, c := range cands {
		names = append(names, c.Name)
	}
	t.Logf("карта %s: %d героев (%v) из %d кусков", filepath.Base(book), len(cands), names, len(chunks))
}
