package birth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"tns65/internal/testkit"
)

func TestSeedFill(t *testing.T) {
	packRoot := t.TempDir()
	writeSeedPack(t, packRoot)

	worldsRoot := t.TempDir()
	res, err := Birth(validAncketa(), Config{WorldsRoot: worldsRoot, PacksRoot: packRoot, BaseDBPath: testBaseDB(t, worldsRoot), Embed: testkit.NewFakeEmbed()})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}

	lit := openReader(t, res.LiteraryDB)

	// bank_pool непуст; все три вида записей присутствуют; trust_level=2.
	var total int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM bank_pool`).Scan(&total); err != nil {
		t.Fatalf("bank_pool count: %v", err)
	}
	if total == 0 {
		t.Fatal("bank_pool пуст")
	}

	kinds := map[string]int{}
	rows, err := lit.Query(`SELECT kind, trust_level FROM bank_pool`)
	if err != nil {
		t.Fatalf("bank_pool query: %v", err)
	}
	for rows.Next() {
		var kind string
		var level int
		if err := rows.Scan(&kind, &level); err != nil {
			t.Fatalf("bank_pool scan: %v", err)
		}
		if level != 2 {
			t.Errorf("trust_level = %d, want 2", level)
		}
		kinds[kind]++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("bank_pool rows: %v", err)
	}

	for _, kind := range []string{"routine", "drama", "fallback"} {
		if kinds[kind] == 0 {
			t.Errorf("вид %q отсутствует в bank_pool", kind)
		}
	}

	// Шаг 2: словарные шаблоны наполняют lit_template.
	var templates int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM lit_template`).Scan(&templates); err != nil {
		t.Fatalf("lit_template count: %v", err)
	}
	if templates == 0 {
		t.Fatal("lit_template пуст")
	}

	// Шаг 3: в forge_log одна запись kind='seed' с версией пакета.
	var seeds int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM forge_log WHERE kind = 'seed'`).Scan(&seeds); err != nil {
		t.Fatalf("forge_log seed count: %v", err)
	}
	if seeds != 1 {
		t.Fatalf("записей seed в forge_log: %d, want 1", seeds)
	}
	var version string
	if err := lit.QueryRow(`SELECT package_version FROM forge_log WHERE kind = 'seed'`).Scan(&version); err != nil {
		t.Fatalf("forge_log package_version: %v", err)
	}
	if version != "1.0.0" {
		t.Errorf("package_version = %q, want 1.0.0", version)
	}

	// Шаг 4: повторное наполнение того же мира не добавляет записей.
	if err := seedLiteraryBase(res.LiteraryDB, filepath.Join(packRoot, "ru"), nil); err != nil {
		t.Fatalf("повторное наполнение: %v", err)
	}
	var afterTotal int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM bank_pool`).Scan(&afterTotal); err != nil {
		t.Fatalf("bank_pool count after repeat: %v", err)
	}
	if afterTotal != total {
		t.Errorf("bank_pool после повтора: %d, want %d", afterTotal, total)
	}
	var afterSeeds int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM forge_log WHERE kind = 'seed'`).Scan(&afterSeeds); err != nil {
		t.Fatalf("forge_log seed count after repeat: %v", err)
	}
	if afterSeeds != seeds {
		t.Errorf("записей seed после повтора: %d, want %d", afterSeeds, seeds)
	}
}

// writeSeedPack создаёт фикстуру языкового пакета ru: сид-банки трёх видов в
// versions/1.0.0/, словарные шаблоны lit_templates.jsonl, манифест и current.
func writeSeedPack(t *testing.T, packRoot string) {
	t.Helper()
	ru := filepath.Join(packRoot, "ru")
	versionDir := filepath.Join(ru, "versions", "1.0.0")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJSONL := func(name string, lines []map[string]string) {
		var b []byte
		for _, l := range lines {
			raw, err := json.Marshal(l)
			if err != nil {
				t.Fatal(err)
			}
			b = append(b, raw...)
			b = append(b, '\n')
		}
		if err := os.WriteFile(filepath.Join(versionDir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeJSONL("bank_routine.jsonl", []map[string]string{
		{"kind": "routine", "text": "— Добро пожаловать, путник.", "tags": "приветствие", "lang": "ru"},
		{"kind": "routine", "text": "Торговец молча взвешивает товар.", "tags": "торг", "lang": "ru"},
	})
	writeJSONL("bank_drama.jsonl", []map[string]string{
		{"kind": "drama", "text": "Тень легла на порог, и все замолчали.", "tags": "нагнетание", "lang": "ru"},
	})
	writeJSONL("bank_fallback.jsonl", []map[string]string{
		{"kind": "fallback", "text": "Мир замер в ожидании.", "tags": "нейтрально", "lang": "ru"},
	})

	var templates []byte
	for _, l := range []map[string]string{
		{"kind": "phrase", "text": "Ветер донёс до него звук.", "tags": "сенсорика", "lang": "ru"},
	} {
		raw, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		templates = append(templates, raw...)
		templates = append(templates, '\n')
	}
	if err := os.WriteFile(filepath.Join(ru, "lit_templates.jsonl"), templates, 0o644); err != nil {
		t.Fatal(err)
	}

	manifest := map[string]any{
		"id":      "ru",
		"name":    "Русский",
		"lang":    "ru",
		"version": "1.0.0",
		"files":   map[string]string{},
	}
	mb, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ru, "manifest.json"), mb, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ru, "current"), []byte("1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
