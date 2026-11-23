package deepread_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/deepread"
)

// TestSenseSalience — гейт смыслового слоя salience (T-15-02): омоним,
// решённый в чужую категорию, ложного тега не даёт; нерешённый смысл считается
// как в T-14; порог deepread.senses_margin снимает решение; без senses.json
// разбор работает как раньше.
func TestSenseSalience(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Fatal("корень движка не найден")
	}
	packDir := filepath.Join(root, "packs", "ru")

	cfg, err := deepread.LoadSalienceConfig(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatalf("LoadSalienceConfig(engine/config.yaml): %v", err)
	}
	if cfg.SensesMargin != 0.5 {
		t.Fatalf("deepread.senses_margin = %g, ожидалось 0.5", cfg.SensesMargin)
	}
	layer, err := deepread.LoadSenseLayer(packDir, cfg)
	if err != nil {
		t.Fatalf("LoadSenseLayer: %v", err)
	}
	if layer == nil || layer.Dict == nil {
		t.Fatal("LoadSenseLayer: словарь смыслов пака не загружен")
	}

	// architecture с ловушкой «замок», nature с «ключ»-родником.
	dicts := map[string]*deepread.Dictionary{
		deepread.KeyArchitecture: {
			Key:    deepread.KeyArchitecture,
			Lemmas: map[string]int{"замок": 2},
		},
		deepread.KeyNature: {
			Key:    deepread.KeyNature,
			Lemmas: map[string]int{"ключ": 1},
		},
	}

	// lockChunk — запирающий замок: триггеры смысла «запирающее устройство».
	lockChunk := senseChunk(1, "замок", "замок", "скважина", "защёлка", "засов")
	// springChunk — родник: триггеры смысла «родник».
	springChunk := senseChunk(2, "ключ", "ключ", "журчать", "напиться", "студёный")
	// bareChunk — омоним без единого триггера: смысл не решён.
	bareChunk := senseChunk(3, "замок")

	t.Run("ловушка омонима", func(t *testing.T) {
		chunks := []deepread.SalienceChunk{lockChunk, springChunk}
		plain, err := deepread.TagChunks(chunks, dicts, cfg)
		if err != nil {
			t.Fatalf("TagChunks: %v", err)
		}
		withSenses, err := deepread.TagChunksWithSenses(chunks, dicts, layer, cfg)
		if err != nil {
			t.Fatalf("TagChunksWithSenses: %v", err)
		}

		plainArch := senseTag(t, plain[0], deepread.KeyArchitecture)
		if plainArch.Band != deepread.BandConfident {
			t.Fatalf("без смыслового слоя замок-ловушка: полоса %q, ожидалась confident", plainArch.Band)
		}
		senseArch := senseTag(t, withSenses[0], deepread.KeyArchitecture)
		if senseArch.Score != 0 || senseArch.Band != deepread.BandReject {
			t.Errorf("замок-ловушка: скор %g, полоса %q, ожидалось 0 и reject",
				senseArch.Score, senseArch.Band)
		}
		if len(senseArch.Anchors) != 0 {
			t.Errorf("замок-ловушка: якорей %d, ожидалось 0", len(senseArch.Anchors))
		}
		// Смысл «родник» в своей категории вклад сохраняет.
		senseNature := senseTag(t, withSenses[1], deepread.KeyNature)
		if senseNature.Score != 1 || senseNature.Band != deepread.BandConfident {
			t.Errorf("ключ-родник: скор %g, полоса %q, ожидалось 1 и confident",
				senseNature.Score, senseNature.Band)
		}
	})

	t.Run("нерешённый смысл консервативен", func(t *testing.T) {
		chunks := []deepread.SalienceChunk{bareChunk}
		plain, err := deepread.TagChunks(chunks, dicts, cfg)
		if err != nil {
			t.Fatalf("TagChunks: %v", err)
		}
		withSenses, err := deepread.TagChunksWithSenses(chunks, dicts, layer, cfg)
		if err != nil {
			t.Fatalf("TagChunksWithSenses: %v", err)
		}
		got, want := senseTag(t, withSenses[0], deepread.KeyArchitecture), senseTag(t, plain[0], deepread.KeyArchitecture)
		if !sameTag(got, want) {
			t.Errorf("нерешённый смысл: тег %+v, ожидался как без слоя %+v", got, want)
		}
	})

	t.Run("порог senses_margin", func(t *testing.T) {
		chunks := []deepread.SalienceChunk{lockChunk}
		plain, err := deepread.TagChunks(chunks, dicts, cfg)
		if err != nil {
			t.Fatalf("TagChunks: %v", err)
		}

		// Отрыв смысла «запирающее устройство» — 3.1; порог 3.5 снимает решение.
		strict := &deepread.SenseLayer{Dict: layer.Dict, Margin: 3.5}
		withStrict, err := deepread.TagChunksWithSenses(chunks, dicts, strict, cfg)
		if err != nil {
			t.Fatalf("TagChunksWithSenses(порог 3.5): %v", err)
		}
		got, want := senseTag(t, withStrict[0], deepread.KeyArchitecture), senseTag(t, plain[0], deepread.KeyArchitecture)
		if !sameTag(got, want) {
			t.Errorf("порог 3.5: тег %+v, ожидался консервативный как без слоя %+v", got, want)
		}

		// Порог 0 — константа T-15-01: решение действует.
		withConstant, err := deepread.TagChunksWithSenses(chunks, dicts, &deepread.SenseLayer{Dict: layer.Dict}, cfg)
		if err != nil {
			t.Fatalf("TagChunksWithSenses(порог 0): %v", err)
		}
		if tag := senseTag(t, withConstant[0], deepread.KeyArchitecture); tag.Band != deepread.BandReject {
			t.Errorf("порог 0: полоса %q, ожидалась reject", tag.Band)
		}
	})

	t.Run("нет senses.json", func(t *testing.T) {
		dir := t.TempDir()
		empty, err := deepread.LoadSenseLayer(dir, cfg)
		if err != nil {
			t.Fatalf("LoadSenseLayer(без senses.json): %v", err)
		}
		if empty != nil {
			t.Fatalf("LoadSenseLayer(без senses.json) = %+v, ожидался nil", empty)
		}
		chunks := []deepread.SalienceChunk{lockChunk}
		plain, err := deepread.TagChunks(chunks, dicts, cfg)
		if err != nil {
			t.Fatalf("TagChunks: %v", err)
		}
		withSenses, err := deepread.TagChunksWithSenses(chunks, dicts, empty, cfg)
		if err != nil {
			t.Fatalf("TagChunksWithSenses(nil): %v", err)
		}
		got, want := senseTag(t, withSenses[0], deepread.KeyArchitecture), senseTag(t, plain[0], deepread.KeyArchitecture)
		if !sameTag(got, want) {
			t.Errorf("без senses.json: тег %+v, ожидался как без слоя %+v", got, want)
		}
	})

	t.Run("порог в конфиге", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		body := "deepread:\n  thresholds:\n    confident: 0.6\n    reject: 0.15\n  senses_margin: 0\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("config.yaml: %v", err)
		}
		zero, err := deepread.LoadSalienceConfig(path)
		if err != nil {
			t.Fatalf("LoadSalienceConfig(senses_margin 0): %v", err)
		}
		if zero.SensesMargin != 0 || zero.Confident != 0.6 || zero.Reject != 0.15 {
			t.Errorf("конфиг %+v, ожидалось senses_margin 0 и пороги 0.6/0.15", zero)
		}
		bad := "deepread:\n  senses_margin: 1.5\n"
		if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
			t.Fatalf("config.yaml: %v", err)
		}
		if _, err := deepread.LoadSalienceConfig(path); err == nil {
			t.Error("senses_margin 1.5: ошибки нет")
		}
	})
}

// senseChunk собирает чанк из одного токена-омонима и токенов-триггеров: формы
// и леммы совпадают, часть речи — NOUN, строка одна.
func senseChunk(id int64, forms ...string) deepread.SalienceChunk {
	lemmas := make([]deepread.Lemma, len(forms))
	for i, form := range forms {
		lemmas[i] = deepread.Lemma{Form: form, Lemma: form, POS: "NOUN", Line: 1, Index: i}
	}
	return deepread.SalienceChunk{ID: id, Text: "контекст омонима", Lemmas: lemmas}
}

// sameTag сравнивает теги по скору, полосе и якорям.
func sameTag(a, b deepread.ChunkTag) bool {
	return a.Key == b.Key && a.Score == b.Score && a.Band == b.Band &&
		strings.Join(a.Anchors, "|") == strings.Join(b.Anchors, "|")
}

// senseTag возвращает тег ключа чанка.
func senseTag(t *testing.T, chunk deepread.ChunkTags, key string) deepread.ChunkTag {
	t.Helper()
	for _, tag := range chunk.Tags {
		if tag.Key == key {
			return tag
		}
	}
	t.Fatalf("чанк %d: нет тега ключа %q", chunk.ChunkID, key)
	return deepread.ChunkTag{}
}
