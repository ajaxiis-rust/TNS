package workshop_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// openForgeBookStores собирает пару баз и хэндлы ковки книги: мир (core) и
// литературную базу (forge).
func openForgeBookStores(t *testing.T) (*db.Handle, *db.Handle) {
	t.Helper()
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world.db")
	litPath := filepath.Join(dir, "literary.db")
	newDB(t, worldPath, "world", "shared")
	newDB(t, litPath, "literary")

	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	t.Cleanup(func() { _ = world.Close() })
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	t.Cleanup(func() { _ = lit.Close() })
	return world, lit
}

// seedBookChunk заводит строку book_chunk книги; возвращает её id.
func seedBookChunk(t *testing.T, lit *db.Handle, bookID int64, ord int, chapter string) int64 {
	t.Helper()
	res, err := lit.Exec("book_chunk",
		`INSERT INTO book_chunk (book_id, ord, chapter, text_hash) VALUES (?, ?, ?, ?)`,
		bookID, ord, chapter, fmt.Sprintf("hash-%d", ord))
	if err != nil {
		t.Fatalf("INSERT book_chunk ord=%d: %v", ord, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("book_chunk id: %v", err)
	}
	return id
}

// seedChunkTag заводит тег чанка с полосой.
func seedChunkTag(t *testing.T, lit *db.Handle, chunkID int64, key string, score float64, band string) {
	t.Helper()
	if _, err := lit.Exec("book_chunk_tag",
		`INSERT INTO book_chunk_tag (chunk_id, key, score, band) VALUES (?, ?, ?, ?)`,
		chunkID, key, score, band); err != nil {
		t.Fatalf("INSERT book_chunk_tag %q: %v", key, err)
	}
}

// TestForgeBookWritesChunkTags — ковка книги пишет настоящие теги чанка:
// ключи разбора без полосы брака и теги покрывающей сцены.
func TestForgeBookWritesChunkTags(t *testing.T) {
	world, lit := openForgeBookStores(t)
	input := writeChunksTXT(t, []workshop.Chunk{
		{Chapter: "Глава 1", Text: "Лес стоял у стен дворца."},
		{Chapter: "Глава 2", Text: "Схватка у ворот."},
	})

	bookID, err := workshop.EnsureBook(lit, "book.txt")
	if err != nil {
		t.Fatalf("EnsureBook: %v", err)
	}
	first := seedBookChunk(t, lit, bookID, 0, "Глава 1")
	seedChunkTag(t, lit, first, "nature", 0.9, "confident")
	seedChunkTag(t, lit, first, "architecture", 0.7, "uncertain")
	seedChunkTag(t, lit, first, "violence", 0.1, "reject")
	second := seedBookChunk(t, lit, bookID, 1, "Глава 2")
	seedChunkTag(t, lit, second, "social", 0.8, "confident")
	if _, err := lit.Exec("book_scene",
		`INSERT INTO book_scene (book_id, ord, from_chunk, to_chunk, tags) VALUES (?, 0, 1, 1, '["battle"]')`,
		bookID); err != nil {
		t.Fatalf("INSERT book_scene: %v", err)
	}

	slot := testkit.NewFakeSlot()
	slot.SetResponse(workshop.ComposeRole,
		`{"family_primary":"","pattern_secondary":"","technique":"","moral_stance":"","inversion":false}`)
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)
	workshop.SetEmbedder(testkit.NewFakeEmbed())
	defer workshop.SetEmbedder(nil)

	if _, err := workshop.RunForgeBook(context.Background(), world, lit, input, workshop.ForgeBookOptions{
		Families: composeFamilies(),
	}); err != nil {
		t.Fatalf("RunForgeBook: %v", err)
	}

	if got := bankTagsAt(t, lit, bookID, 0); got != "architecture,nature" {
		t.Errorf("теги записи чанка 0 = %q, want architecture,nature", got)
	}
	if got := bankTagsAt(t, lit, bookID, 1); got != "battle,social" {
		t.Errorf("теги записи чанка 1 = %q, want battle,social", got)
	}
}

// TestSyncBankTags — записи, выкованные до разбора, получают теги чанков
// синхронизацией после разбора; повтор синхронизации их не меняет.
func TestSyncBankTags(t *testing.T) {
	_, lit := openForgeBookStores(t)
	bookID, err := workshop.EnsureBook(lit, "синхро")
	if err != nil {
		t.Fatalf("EnsureBook: %v", err)
	}
	chunkID := seedBookChunk(t, lit, bookID, 0, "")
	seedChunkTag(t, lit, chunkID, "nature", 0.9, "confident")
	seedChunkTag(t, lit, chunkID, "architecture", 0.7, "uncertain")
	if _, err := lit.Exec("book_scene",
		`INSERT INTO book_scene (book_id, ord, from_chunk, to_chunk, tags) VALUES (?, 0, 0, 0, '["time"]')`,
		bookID); err != nil {
		t.Fatalf("INSERT book_scene: %v", err)
	}
	if _, err := lit.Exec("bank_pool",
		`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, book_id, created_at) VALUES ('routine', 'форма', 'ru', '', 2, ?, ?, '2026-01-01T00:00:00Z')`,
		fmt.Sprintf("forge-book:%d:0", bookID), bookID); err != nil {
		t.Fatalf("INSERT bank_pool: %v", err)
	}

	updated, err := workshop.SyncBankTags(lit, bookID)
	if err != nil {
		t.Fatalf("SyncBankTags: %v", err)
	}
	if updated != 1 {
		t.Fatalf("SyncBankTags заполнил %d записей, want 1", updated)
	}
	if got := bankTagsAt(t, lit, bookID, 0); got != "architecture,nature,time" {
		t.Errorf("теги записи = %q, want architecture,nature,time", got)
	}
	if updated, err := workshop.SyncBankTags(lit, bookID); err != nil || updated != 0 {
		t.Fatalf("повтор SyncBankTags = (%d, %v), want (0, nil)", updated, err)
	}
}

// bankTagsAt читает теги записи банка чанка по origin.
func bankTagsAt(t *testing.T, lit *db.Handle, bookID int64, ord int) string {
	t.Helper()
	var tags string
	if err := lit.QueryRow(
		`SELECT tags FROM bank_pool WHERE origin = ?`, fmt.Sprintf("forge-book:%d:%d", bookID, ord)).
		Scan(&tags); err != nil {
		t.Fatalf("SELECT tags записи чанка %d: %v", ord, err)
	}
	return tags
}
