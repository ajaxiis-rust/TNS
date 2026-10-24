package birth

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"tns65/internal/testkit"
)

func TestWorldList(t *testing.T) {
	dir := t.TempDir()

	first := validAncketa()
	first.Slug = "alpha-world"
	first.Title = "Альфа-мир"
	if _, err := Birth(first, Config{WorldsRoot: dir, BaseDBPath: testBaseDB(t, dir), Embed: testkit.NewFakeEmbed()}); err != nil {
		t.Fatalf("Birth(alpha): %v", err)
	}

	second := validAncketa()
	second.Slug = "beta-world"
	second.Title = "Бета-мир"
	if _, err := Birth(second, Config{WorldsRoot: dir, BaseDBPath: testBaseDB(t, dir), Embed: testkit.NewFakeEmbed()}); err != nil {
		t.Fatalf("Birth(beta): %v", err)
	}

	// Остаток прерванного рождения (состояние creating) в список не попадает.
	interrupted := filepath.Join(dir, "worlds", "interrupted")
	if err := os.MkdirAll(interrupted, 0o755); err != nil {
		t.Fatalf("MkdirAll interrupted: %v", err)
	}
	if err := writeState(interrupted, stateCreating); err != nil {
		t.Fatalf("writeState interrupted: %v", err)
	}

	worlds, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(worlds) != 2 {
		t.Fatalf("миров в списке: %d, want 2", len(worlds))
	}

	// Сортировка по слагу; слаги латиницей (B-01.1, кириллица отсутствует).
	if worlds[0].Slug != "alpha-world" || worlds[1].Slug != "beta-world" {
		t.Fatalf("порядок миров: %q, %q, want alpha-world, beta-world", worlds[0].Slug, worlds[1].Slug)
	}

	// Заголовок — имя мира в списке (B-02.1).
	if worlds[0].Title != "Альфа-мир" || worlds[1].Title != "Бета-мир" {
		t.Fatalf("заголовки миров: %q, %q", worlds[0].Title, worlds[1].Title)
	}

	now := time.Now()
	for i, w := range worlds {
		if w.Dir != filepath.Join(dir, "worlds", w.Slug) {
			t.Fatalf("путь мира %d: %q, want worlds/%s", i, w.Dir, w.Slug)
		}
		if w.BirthDate.IsZero() {
			t.Fatalf("мир %d без даты рождения", i)
		}
		if w.BirthDate.After(now) || w.BirthDate.Before(now.Add(-time.Minute)) {
			t.Fatalf("дата рождения мира %d вне окна: %v", i, w.BirthDate)
		}
	}
}
