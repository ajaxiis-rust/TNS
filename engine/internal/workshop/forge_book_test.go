package workshop_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// Потолки волны §8.1 (жёстко в тестовом коде, T-12-27): unit-тест не варит
// больше двух кусков; полная книга в тесте — ошибка, а не медленный успех.
const (
	maxTestChunks = 2
	maxSlotCalls  = 6
)

// castFixture — короткая фикстура fb2 (3 главы, ~4 КБ) с выдуманными именами и
// маркерами описаний; один герой меняет роль между главами (T-12-27).
const castFixture = "testdata/cast.fb2"

// capTestChunks режет куски до потолка maxTestChunks и пишет в лог, если чанкер
// вернул больше: полная книга в тесте — ошибка, а не медленный успех.
func capTestChunks(t *testing.T, chunks []workshop.Chunk) []workshop.Chunk {
	t.Helper()
	if len(chunks) > maxTestChunks {
		t.Logf("чанкер вернул %d кусков, режу до потолка %d", len(chunks), maxTestChunks)
		chunks = chunks[:maxTestChunks]
	}
	return chunks
}

// writeChunksTXT кладёт куски во временный txt: RunForgeBook разбирает файл сам,
// поэтому вход, обрезанный до потолка, записывается заново.
func writeChunksTXT(t *testing.T, chunks []workshop.Chunk) string {
	t.Helper()
	var b []byte
	for _, c := range chunks {
		b = append(b, c.Chapter...)
		b = append(b, '\n')
		b = append(b, c.Text...)
		b = append(b, '\n', '\n')
	}
	path := filepath.Join(t.TempDir(), "book.txt")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// TestForgeBookCastFixture — «Проверка» T-12-27: короткая фикстура fb2 варится с
// потолками — два куска, два вызова слота (≤ maxSlotCalls), фейковый эмбеддер
// даёт два вектора; повторный запуск без дублей.
func TestForgeBookCastFixture(t *testing.T) {
	parsed, err := workshop.ParseBookInput(castFixture)
	if err != nil {
		t.Fatalf("ParseBookInput(cast.fb2): %v", err)
	}
	if len(parsed.Chunks) != 3 {
		t.Fatalf("фикстура дала %d кусков, want 3 (три главы)", len(parsed.Chunks))
	}
	chunks := capTestChunks(t, parsed.Chunks)
	if len(chunks) != maxTestChunks {
		t.Fatalf("после потолка %d кусков, want %d", len(chunks), maxTestChunks)
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

	workshop.SetCensor(passCensor{})
	defer workshop.SetCensor(nil)
	slot := testkit.NewFakeSlot()
	slot.SetCallLimit(maxSlotCalls)
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)
	emb := testkit.NewFakeEmbed()
	workshop.SetEmbedder(emb)
	defer workshop.SetEmbedder(nil)

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
	if res.Chunks != maxTestChunks {
		t.Fatalf("чанков = %d, want %d", res.Chunks, maxTestChunks)
	}
	if res.Progress != 1.0 {
		t.Fatalf("прогресс = %v, want 1.0", res.Progress)
	}
	if lastDone != maxTestChunks || lastTotal != maxTestChunks {
		t.Fatalf("колбэк прогресса = %d/%d, want %d/%d", lastDone, lastTotal, maxTestChunks, maxTestChunks)
	}
	if got := slot.Calls(); got != maxTestChunks {
		t.Fatalf("вызовов слота = %d, want %d (один на кусок)", got, maxTestChunks)
	}
	if got := emb.Calls(); got != maxTestChunks {
		t.Fatalf("вызовов эмбеддера = %d, want %d (один вектор на кусок)", got, maxTestChunks)
	}
	if got := countBankPool(t, lit); got != maxTestChunks {
		t.Fatalf("записей bank_pool = %d, want %d", got, maxTestChunks)
	}
	if got := countBankVectors(t, world); got != maxTestChunks {
		t.Fatalf("векторов bank_pool = %d, want %d", got, maxTestChunks)
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
	if got := countBankPool(t, lit); got != maxTestChunks {
		t.Fatalf("записей bank_pool после повтора = %d, want %d", got, maxTestChunks)
	}
	if got := countBankVectors(t, world); got != maxTestChunks {
		t.Fatalf("векторов после повтора = %d, want %d", got, maxTestChunks)
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

// TestForgeBookDeepreadSeparateStores — регресс IMP-13: разбор книги идёт по
// RunPipeline(lit, world), поэтому базы на разных SQLite-файлах ловят свап
// аргументов: перепутанный порядок читает book_chunk в мире и падает
// «no such table: book_chunk».
func TestForgeBookDeepreadSeparateStores(t *testing.T) {
	if lemmatizerBinary(t) == "" {
		t.Skip("бинарник лемматизатора недоступен — разбор книги не проверить")
	}

	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world.db")
	litPath := filepath.Join(dir, "literary.db")
	newDB(t, worldPath, "world", "shared")
	newDB(t, litPath, "literary", "shared")

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

	input := writeChunksTXT(t, []workshop.Chunk{
		{Chapter: "Глава 1", Text: "Золотой дворец стоял на холме. Иван молчал от страха."},
		{Chapter: "Глава 2", Text: "Золотого дворца сияли окна. Иван ждал."},
	})

	slot := testkit.NewFakeSlot()
	slot.SetResponse(workshop.ComposeRole,
		`{"family_primary":"","pattern_secondary":"","technique":"","moral_stance":"","inversion":false}`)
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)
	workshop.SetEmbedder(testkit.NewFakeEmbed())
	defer workshop.SetEmbedder(nil)

	res, err := workshop.RunForgeBook(context.Background(), world, lit, input, workshop.ForgeBookOptions{
		Families: composeFamilies(),
		Deepread: true,
	})
	if err != nil {
		t.Fatalf("RunForgeBook(Deepread): %v", err)
	}
	if got := countForgeRows(t, lit, `SELECT COUNT(*) FROM book_chunk WHERE book_id = ?`, res.BookID); got != 2 {
		t.Fatalf("строк book_chunk = %d, want 2 (литературная база)", got)
	}
	if got := countForgeRows(t, lit, `SELECT COUNT(*) FROM book_chunk_tag WHERE chunk_id IN (SELECT id FROM book_chunk WHERE book_id = ?)`, res.BookID); got == 0 {
		t.Fatal("book_chunk_tag пуст: разбор не прочитал чанки литературной базы")
	}
	if got := countForgeRows(t, lit, `SELECT COUNT(*) FROM book_style WHERE book_id = ?`, res.BookID); got != 1 {
		t.Fatalf("строк book_style = %d, want 1", got)
	}
}

// TestForgeBookIgnoresCensor — «Проверка» IMP-13 (шаг 5): книга со стоп-словами
// 16+ куётся целиком — цензора в ковке книг нет, записи банка пишутся без
// отбраковки.
func TestForgeBookIgnoresCensor(t *testing.T) {
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

	input := writeChunksTXT(t, []workshop.Chunk{
		{Chapter: "Глава 1", Text: "Иван сказал вслух: он дебил и идиот."},
	})

	slot := testkit.NewFakeSlot()
	slot.SetResponse(workshop.ComposeRole,
		`{"family_primary":"","pattern_secondary":"","technique":"","moral_stance":"","inversion":false}`)
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)
	workshop.SetEmbedder(testkit.NewFakeEmbed())
	defer workshop.SetEmbedder(nil)

	res, err := workshop.RunForgeBook(context.Background(), world, lit, input, workshop.ForgeBookOptions{
		Families: composeFamilies(),
	})
	if err != nil {
		t.Fatalf("RunForgeBook: %v (книга со стоп-словами 16+ должна коваться целиком)", err)
	}
	if res.Chunks != 1 {
		t.Fatalf("чанков = %d, want 1", res.Chunks)
	}
	if got := countBankPool(t, lit); got != 1 {
		t.Fatalf("записей bank_pool = %d, want 1 (запись без отбраковки)", got)
	}
	var text string
	if err := lit.QueryRow(`SELECT text FROM bank_pool WHERE book_id = ?`, res.BookID).Scan(&text); err != nil {
		t.Fatalf("SELECT bank_pool text: %v", err)
	}
	if !strings.Contains(text, "дебил") {
		t.Fatalf("текст банка = %q, want стоп-слово сохранено", text)
	}
}

func countForgeRows(t *testing.T, h *db.Handle, query string, args ...any) int {
	t.Helper()
	var n int
	if err := h.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("COUNT (%s): %v", query, err)
	}
	return n
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
