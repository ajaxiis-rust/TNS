//go:build local

package workshop_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tns65/internal/db"
	"tns65/internal/models"
	"tns65/internal/supervisor"
	"tns65/internal/workshop"
)

// localDeadline — дедлайн локального теста (§8.1): по истечении t.Fatal, не
// зависание.
const localDeadline = 5 * time.Minute

// localSkipModel — сообщение пропуска без локальной модели (§8.1).
const localSkipModel = "локальная модель не поднята, см. scripts/test-local.sh"

const (
	localGenURL   = "http://127.0.0.1:5001"
	localEmbedURL = "http://127.0.0.1:5002"
)

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

// localSlot — реальный слот генерации :5001 поверх models.Weaver: workshop.Slot
// ждёт готовую строку, Weaver стримит чанки /completion — адаптер их склеивает.
type localSlot struct{ weaver *models.Weaver }

func (s localSlot) Generate(ctx context.Context, _ string, prompt string) (string, error) {
	ch, err := s.weaver.Stream(ctx, prompt)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for line := range ch {
		b.WriteString(line)
	}
	return b.String(), nil
}

// TestLocalForgeBook — «Проверка» T-12-28 (2 куска): самая маленькая книга
// варится реальной локальной моделью, не больше двух кусков; без модели — SKIP.
func TestLocalForgeBook(t *testing.T) {
	requireLocalModel(t)
	book := smallestFB2(t, localBookDir())

	parsed, err := workshop.ParseBookInput(book)
	if err != nil {
		t.Fatalf("ParseBookInput(%s): %v", book, err)
	}
	if len(parsed.Chunks) == 0 {
		t.Fatalf("книга %s без кусков", book)
	}
	chunks := capTestChunks(t, parsed.Chunks)
	if len(chunks) == 0 || len(chunks) > maxTestChunks {
		t.Fatalf("кусков после потолка = %d, want 1..%d", len(chunks), maxTestChunks)
	}
	input := writeChunksTXT(t, chunks)

	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world.db")
	litPath := filepath.Join(dir, "literary.db")
	newDB(t, worldPath, "world", "shared")
	newDB(t, litPath, "literary")

	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer lit.Close()

	weaver, err := models.NewWeaver(localGenURL)
	if err != nil {
		t.Fatalf("NewWeaver: %v", err)
	}
	embedder, err := models.NewClient(localEmbedURL, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	workshop.SetCensor(passCensor{})
	defer workshop.SetCensor(nil)
	workshop.SetSlot(localSlot{weaver: weaver})
	defer workshop.SetSlot(nil)
	workshop.SetEmbedder(embedder)
	defer workshop.SetEmbedder(nil)

	ctx, cancel := context.WithTimeout(context.Background(), localDeadline)
	defer cancel()

	res, err := workshop.RunForgeBook(ctx, world, lit, input, workshop.ForgeBookOptions{
		Families: composeFamilies(),
	})
	if ctx.Err() != nil {
		t.Fatalf("дедлайн локального теста истёк: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("RunForgeBook: %v", err)
	}
	if res.Chunks != len(chunks) {
		t.Fatalf("чанков = %d, want %d", res.Chunks, len(chunks))
	}
	if res.Progress != 1.0 {
		t.Fatalf("прогресс = %v, want 1.0", res.Progress)
	}
	if got := countBankPool(t, lit); got != len(chunks) {
		t.Fatalf("записей bank_pool = %d, want %d", got, len(chunks))
	}
	if got := countBankVectors(t, world); got != len(chunks) {
		t.Fatalf("векторов bank_pool = %d, want %d", got, len(chunks))
	}
	t.Logf("выковано %s: %d кусков, %d записей банка, %d векторов",
		filepath.Base(book), res.Chunks, countBankPool(t, lit), countBankVectors(t, world))
}
