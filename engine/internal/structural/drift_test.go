package structural

import (
	"errors"
	"path/filepath"
	"testing"

	"tns65/internal/db"
)

// newDriftWorld открывает оперативную базу мира (миграции world+shared) и
// возвращает хэндл writer `core` — единственный писатель family_usage.
func newDriftWorld(t *testing.T) *db.Handle {
	t.Helper()
	path := filepath.Join(t.TempDir(), "world(a).db")

	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(w, "world", "shared"); err != nil {
		_ = w.Close()
		t.Fatalf("Migrate(world, shared): %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(core): %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// TestDriftCounters — «Проверка» T-10-10.
func TestDriftCounters(t *testing.T) {
	h := newDriftWorld(t)

	// user_version=12 после миграции 0012_structural_drift.
	var uv int
	if err := h.QueryRow(`PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	if uv != 12 {
		t.Fatalf("user_version = %d, want 12", uv)
	}

	// Свежая область пуста.
	if got, err := Usage(h, ScopeChapter); err != nil || len(got) != 0 {
		t.Fatalf("Usage(пустая chapter) = %v, %v; want пустая карта", got, err)
	}

	now := "2026-10-31T10:00:00Z"
	// Bump создаёт счётчик и апсертит его.
	if err := BumpUsage(h, ScopeChapter, "escape_liberation", now); err != nil {
		t.Fatalf("BumpUsage(1): %v", err)
	}
	if err := BumpUsage(h, ScopeChapter, "escape_liberation", now); err != nil {
		t.Fatalf("BumpUsage(2): %v", err)
	}
	if err := BumpUsage(h, ScopeChapter, "loyalty", now); err != nil {
		t.Fatalf("BumpUsage(loyalty): %v", err)
	}
	got, err := Usage(h, ScopeChapter)
	if err != nil {
		t.Fatalf("Usage(chapter): %v", err)
	}
	if got["escape_liberation"] != 2 || got["loyalty"] != 1 {
		t.Fatalf("Usage(chapter) = %v, want escape_liberation=2 loyalty=1", got)
	}

	// Book-счётчики независимы от chapter.
	if err := BumpUsage(h, ScopeBook, "escape_liberation", now); err != nil {
		t.Fatalf("BumpUsage(book): %v", err)
	}
	bookUsage, err := Usage(h, ScopeBook)
	if err != nil {
		t.Fatalf("Usage(book): %v", err)
	}
	if bookUsage["escape_liberation"] != 1 {
		t.Fatalf("Usage(book) = %v, want escape_liberation=1", bookUsage)
	}

	// Reset сбрасывает только chapter, book остаётся.
	if err := ResetScope(h, ScopeChapter, now); err != nil {
		t.Fatalf("ResetScope(chapter): %v", err)
	}
	if got, err := Usage(h, ScopeChapter); err != nil || len(got) != 0 {
		t.Fatalf("Usage(chapter после reset) = %v, %v; want пустая карта", got, err)
	}
	if bookUsage, err := Usage(h, ScopeBook); err != nil || bookUsage["escape_liberation"] != 1 {
		t.Fatalf("Usage(book после reset chapter) = %v, %v; want escape_liberation=1", bookUsage, err)
	}

	// Область вне списка — ошибка.
	if err := BumpUsage(h, "bogus", "loyalty", now); err == nil {
		t.Fatal("BumpUsage(scope=bogus) не отвергнут")
	}

	// Чужой писатель не может писать family_usage (писатель core по карте).
	forgePath := filepath.Join(t.TempDir(), "world-forge.db")
	fw, err := db.OpenWriter(forgePath)
	if err != nil {
		t.Fatalf("OpenWriter(forge): %v", err)
	}
	if err := db.Migrate(fw, "world", "shared"); err != nil {
		_ = fw.Close()
		t.Fatalf("Migrate(forge): %v", err)
	}
	_ = fw.Close()
	forgeH, err := db.OpenWriterAs(forgePath, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	defer forgeH.Close()

	if err := BumpUsage(forgeH, ScopeChapter, "loyalty", now); !errors.Is(err, db.ErrForeignWriter) {
		t.Fatalf("BumpUsage(forge) err = %v, want db.ErrForeignWriter", err)
	}
	if err := ResetScope(forgeH, ScopeChapter, now); !errors.Is(err, db.ErrForeignWriter) {
		t.Fatalf("ResetScope(forge) err = %v, want db.ErrForeignWriter", err)
	}
}
