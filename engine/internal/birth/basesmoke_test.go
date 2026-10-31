package birth

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
)

// shippedLangs — семь языков поставки баз (T-10-15): en, ru, de, es, fr, ja, zh.
var shippedLangs = []string{"en", "ru", "de", "es", "fr", "ja", "zh"}

// TestBirthOnShippedBases — приёмочная проводка: движок заводится на реальных
// базах поставки engine/base/ (D-01, D-03, TUN-1, TUN-9). Тест базы не копирует
// и не создаёт; без поставки — пропуск.
func TestBirthOnShippedBases(t *testing.T) {
	engineRoot := findEngineRoot()
	if engineRoot == "" {
		t.Skip("поставка баз языков не установлена")
	}

	// Базы берутся из каталога поставки рядом с бинарниками движка; внутри
	// Birth путь базы языка считается по умолчанию из T-10-02 (BaseDBPath не
	// задаётся).
	baseRoot := filepath.Join(engineRoot, "base")
	old := db.BaseRoot
	db.BaseRoot = baseRoot
	t.Cleanup(func() { db.BaseRoot = old })

	// Шаг 1: все семь языков обязаны быть на месте, иначе — пропуск.
	for _, lang := range shippedLangs {
		if _, err := os.Stat(filepath.Join(baseRoot, "base."+lang+".db")); err != nil {
			t.Skip("поставка баз языков не установлена")
		}
	}

	// Суммы поставки до прогона (шаг 4): движок читает базы, но не пишет (D-03).
	before := make(map[string]string, len(shippedLangs))
	for _, lang := range shippedLangs {
		before[lang] = fileSHA256(t, filepath.Join(baseRoot, "base."+lang+".db"))
	}

	// Шаг 2: рождение мира на каждой базе языка. Слаг анкеты совпадает, поэтому
	// каждый мир рождается в своём временном каталоге.
	for _, lang := range shippedLangs {
		a := validAncketa()
		a.Lang = lang
		worldsRoot := t.TempDir()

		res, err := Birth(a, Config{WorldsRoot: worldsRoot, Embed: testkit.NewFakeEmbed()})
		if err != nil {
			t.Fatalf("рождение мира на базе %s: %v", lang, err)
		}

		// Шаг 3: якорь назначен, каталог мира создан, base/ в WorldsRoot нет.
		if ids := anchorIDs(t, res.WorldDB); len(ids) != 1 || ids[0] == 0 {
			t.Fatalf("якорь языка %s: %v, want ровно один непустой", lang, ids)
		}
		if _, err := os.Stat(res.Dir); err != nil {
			t.Fatalf("каталог мира языка %s не создан: %v", lang, err)
		}
		if _, err := os.Stat(filepath.Join(worldsRoot, "base")); !os.IsNotExist(err) {
			t.Fatalf("каталог base/ создан под WorldsRoot языка %s: %v", lang, err)
		}
	}

	// Шаг 4: поставка не изменилась — суммы до и после совпадают.
	for _, lang := range shippedLangs {
		after := fileSHA256(t, filepath.Join(baseRoot, "base."+lang+".db"))
		if before[lang] != after {
			t.Fatalf("база %s изменилась при рождении: %s → %s", lang, before[lang], after)
		}
	}
}

// fileSHA256 — SHA-256 файла (hex), потоком, без загрузки базы в память.
func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}
