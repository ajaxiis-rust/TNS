package workshop_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// TestForgeBook — «Проверка» T-12-11: фейковые слот и эмбеддер; 3 чанка → 3
// записи bank_pool, 3 вектора, прогресс 1.0; повторный запуск без дублей.
func TestForgeBook(t *testing.T) {
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

	workshop.SetCensor(passCensor{})
	defer workshop.SetCensor(nil)
	slot := testkit.NewFakeSlot()
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)
	workshop.SetEmbedder(testkit.NewFakeEmbed())
	defer workshop.SetEmbedder(nil)

	input := filepath.Join(dir, "book.txt")
	if err := os.WriteFile(input, []byte(
		"Глава 1. Встреча\nТекст первой главы.\nГлава 2. Погоня\nТекст второй главы.\nГлава 3. Развязка\nТекст третьей главы.\n",
	), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	slot.SetResponse(workshop.ComposeRole,
		`{"family_primary":"","pattern_secondary":"","technique":"","moral_stance":"","inversion":false}`)

	var lastDone, lastTotal int
	res, err := workshop.RunForgeBook(context.Background(), world, lit, input, workshop.ForgeBookOptions{
		Families: composeFamilies(),
		Progress: func(done, total int) { lastDone, lastTotal = done, total },
	})
	if err != nil {
		t.Fatalf("RunForgeBook: %v", err)
	}
	if res.Chunks != 3 {
		t.Fatalf("чанков = %d, want 3", res.Chunks)
	}
	if res.Progress != 1.0 {
		t.Fatalf("прогресс = %v, want 1.0", res.Progress)
	}
	if lastDone != 3 || lastTotal != 3 {
		t.Fatalf("колбэк прогресса = %d/%d, want 3/3", lastDone, lastTotal)
	}
	if got := countBankPool(t, lit); got != 3 {
		t.Fatalf("записей bank_pool = %d, want 3", got)
	}
	if got := countBankVectors(t, world); got != 3 {
		t.Fatalf("векторов bank_pool = %d, want 3", got)
	}

	// Повторный запуск той же книги: тот же book_id, дублей банка и векторов нет.
	res2, err := workshop.RunForgeBook(context.Background(), world, lit, input, workshop.ForgeBookOptions{
		Families: composeFamilies(),
	})
	if err != nil {
		t.Fatalf("RunForgeBook (повтор): %v", err)
	}
	if res2.BookID != res.BookID {
		t.Fatalf("book_id повтора = %d, want %d", res2.BookID, res.BookID)
	}
	if got := countBankPool(t, lit); got != 3 {
		t.Fatalf("записей bank_pool после повтора = %d, want 3", got)
	}
	if got := countBankVectors(t, world); got != 3 {
		t.Fatalf("векторов после повтора = %d, want 3", got)
	}
}

// TestForgeBookChunkLimit — лимит чанка (T-12-11): глава длиннее MaxChunkRunes
// режется на куски, короткие главы не трогаются.
func TestForgeBookChunkLimit(t *testing.T) {
	long := make([]rune, workshop.MaxChunkRunes+10)
	for i := range long {
		long[i] = 'а'
	}
	body := "Глава 1\n" + string(long) + "\nГлава 2\nкороткий текст\n"

	dir := t.TempDir()
	input := filepath.Join(dir, "book.txt")
	if err := os.WriteFile(input, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	parsed, err := workshop.ParseBookInput(input)
	if err != nil {
		t.Fatalf("ParseBookInput: %v", err)
	}
	if len(parsed.Chunks) != 3 {
		t.Fatalf("чанков = %d, want 3 (длинная глава на два куска + короткая)", len(parsed.Chunks))
	}
	if got := len([]rune(parsed.Chunks[0].Text)); got > workshop.MaxChunkRunes {
		t.Fatalf("кусок 0 длиной %d рун, want ≤ %d", got, workshop.MaxChunkRunes)
	}
	if got := len([]rune(parsed.Chunks[1].Text)); got > workshop.MaxChunkRunes {
		t.Fatalf("кусок 1 длиной %d рун, want ≤ %d", got, workshop.MaxChunkRunes)
	}
	if parsed.Chunks[0].Chapter != "Глава 1" || parsed.Chunks[1].Chapter != "Глава 1" {
		t.Fatalf("заголовки под-кусков = %q/%q, want Глава 1/Глава 1", parsed.Chunks[0].Chapter, parsed.Chunks[1].Chapter)
	}
	if parsed.Chunks[2].Chapter != "Глава 2" {
		t.Fatalf("заголовок куска 2 = %q, want Глава 2", parsed.Chunks[2].Chapter)
	}
}

func countBankPool(t *testing.T, lit *db.Handle) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM bank_pool`).Scan(&n); err != nil {
		t.Fatalf("COUNT bank_pool: %v", err)
	}
	return n
}

func countBankVectors(t *testing.T, world *db.Handle) int {
	t.Helper()
	var n int
	if err := world.QueryRow(`SELECT COUNT(*) FROM vectors WHERE owner_table = 'bank_pool'`).Scan(&n); err != nil {
		t.Fatalf("COUNT vectors: %v", err)
	}
	return n
}
