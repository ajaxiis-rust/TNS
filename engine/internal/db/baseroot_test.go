package db

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestBaseRootDefault(t *testing.T) {
	if BaseRoot != "base" {
		t.Fatalf("BaseRoot = %q, want %q", BaseRoot, "base")
	}
}

// TestBaseRootParametrized — BaseRoot подменяется для тестов так же, как
// WorldsRoot: рождение в t.TempDir() читает фикстурную базу из подменённого
// каталога поставки.
func TestBaseRootParametrized(t *testing.T) {
	dir := t.TempDir()
	old := BaseRoot
	BaseRoot = dir
	t.Cleanup(func() { BaseRoot = old })

	if BaseRoot != dir {
		t.Fatalf("BaseRoot = %q, want %q", BaseRoot, dir)
	}
}

// TestBaseWritersBakeOnly — базу языка пишет только bake (D-09.1, T-10-02 шаг 4):
// живой писатель (core) картой писателей отвергается, запись из игры невозможна.
func TestBaseWritersBakeOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "base.ru.db")

	w, err := OpenWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(w, "base", "shared"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	insert := `INSERT INTO narratives (name, arc_summary, lang, created_at) VALUES ('n', 'arc', 'ru', '2026-01-01T00:00:00Z')`

	core, err := OpenWriterAs(path, DBBase, WriterCore, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.Exec("narratives", insert); !errors.Is(err, ErrForeignWriter) {
		t.Fatalf("запись core в narratives: %v, want ErrForeignWriter", err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}

	bake, err := OpenWriterAs(path, DBBase, WriterBake, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bake.Exec("narratives", insert); err != nil {
		t.Fatalf("запись bake в narratives: %v", err)
	}
	if err := bake.Close(); err != nil {
		t.Fatal(err)
	}
}
