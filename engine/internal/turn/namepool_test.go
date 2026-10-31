package turn

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"tns65/internal/db"
)

// writeNamePoolBaseDB создаёт базу языка с именами двух групп значимости
// (major и прочие) для пула имён (TUN-5).
func writeNamePoolBaseDB(t *testing.T, path string) {
	t.Helper()
	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(w, "base", "shared"); err != nil {
		_ = w.Close()
		t.Fatal(err)
	}
	if _, err := w.Exec(`INSERT INTO narratives (name, arc_summary, lang, created_at) VALUES (?,?,?,?)`,
		"n1", "arc", "ru", "2026-01-01T00:00:00Z"); err != nil {
		_ = w.Close()
		t.Fatal(err)
	}

	type row struct {
		id   int64
		name string
		sig  string
	}
	for _, r := range []row{
		{1, "Слуга1", "servant"},
		{2, "Боярин", "major"},
		{3, "Слуга2", "servant"},
		{4, "Князь", "major"},
	} {
		traits, err := json.Marshal(map[string]string{"significance": r.sig})
		if err != nil {
			_ = w.Close()
			t.Fatal(err)
		}
		if _, err := w.Exec(`INSERT INTO characters (id, narrative_id, name, traits_json, lang) VALUES (?,?,?,?,?)`,
			r.id, 1, r.name, string(traits), "ru"); err != nil {
			_ = w.Close()
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNamePool(t *testing.T) {
	t.Run("первым берётся major-имя", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "base.ru.db")
		writeNamePoolBaseDB(t, path)

		got, err := PickPoolName(path, "ru", nil)
		if err != nil {
			t.Fatalf("PickPoolName: %v", err)
		}
		if got != "Боярин" {
			t.Fatalf("первое имя = %q, want %q (major раньше прочих)", got, "Боярин")
		}
	})

	t.Run("занятые имена пропущены", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "base.ru.db")
		writeNamePoolBaseDB(t, path)

		got, err := PickPoolName(path, "ru", map[string]bool{"Боярин": true, "Князь": true})
		if err != nil {
			t.Fatalf("PickPoolName: %v", err)
		}
		if got != "Слуга1" {
			t.Fatalf("имя после пропуска занятых = %q, want %q", got, "Слуга1")
		}
	})

	t.Run("пул исчерпан — пустая строка без ошибки", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "base.ru.db")
		writeNamePoolBaseDB(t, path)

		got, err := PickPoolName(path, "ru", map[string]bool{
			"Боярин": true, "Князь": true, "Слуга1": true, "Слуга2": true,
		})
		if err != nil {
			t.Fatalf("PickPoolName: %v", err)
		}
		if got != "" {
			t.Fatalf("имя = %q, want пусто (пул исчерпан)", got)
		}
	})

	t.Run("два Spawn подряд дают разные имена", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		path := filepath.Join(t.TempDir(), "base.ru.db")
		writeNamePoolBaseDB(t, path)
		tr := NewTroupe(p.DB).WithNamePool(path, "ru")

		cardJSON := `{"archetype":"Trickster","voice":"скрипучий","appearance":"низкий, коренастый"}`
		if _, err := p.DB.Exec("characters",
			`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES (1, '', 'ENTJ', 'купец', ?)`,
			cardJSON); err != nil {
			t.Fatalf("insert characters 1: %v", err)
		}
		if _, err := p.DB.Exec("characters",
			`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES (2, '', 'INTJ', 'крестьянин', ?)`,
			cardJSON); err != nil {
			t.Fatalf("insert characters 2: %v", err)
		}

		n1, err := tr.Spawn(1)
		if err != nil {
			t.Fatalf("Spawn(1): %v", err)
		}
		n2, err := tr.Spawn(2)
		if err != nil {
			t.Fatalf("Spawn(2): %v", err)
		}
		if n1.Name == "" || n2.Name == "" {
			t.Fatalf("имена пусты: %q / %q", n1.Name, n2.Name)
		}
		if n1.Name == n2.Name {
			t.Fatalf("два Spawn дали одно имя %q", n1.Name)
		}
		if n1.Name != "Боярин" {
			t.Fatalf("первое имя = %q, want %q", n1.Name, "Боярин")
		}

		// Имя записано в characters хэндлом core.
		var stored string
		if err := p.DB.QueryRow(`SELECT name FROM characters WHERE id = 1`).Scan(&stored); err != nil {
			t.Fatalf("SELECT name: %v", err)
		}
		if stored != n1.Name {
			t.Fatalf("имя в characters = %q, want %q", stored, n1.Name)
		}
	})

	t.Run("без базы NPC появляется без имени, отказов нет", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		tr := NewTroupe(p.DB) // пустой пул — деградация без имени.

		cardJSON := `{"archetype":"Trickster","voice":"скрипучий","appearance":"низкий, коренастый"}`
		if _, err := p.DB.Exec("characters",
			`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES (3, '', 'ENTJ', 'купец', ?)`,
			cardJSON); err != nil {
			t.Fatalf("insert characters: %v", err)
		}

		npc, err := tr.Spawn(3)
		if err != nil {
			t.Fatalf("Spawn: %v", err)
		}
		if npc.Name != "" {
			t.Fatalf("имя без базы = %q, want пусто", npc.Name)
		}
	})
}
