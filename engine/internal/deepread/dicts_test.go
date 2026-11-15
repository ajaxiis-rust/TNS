package deepread_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/censor"
	"tns65/internal/deepread"
	"tns65/internal/lemmarpc"
)

// TestDictGates — гейт словарей разбора (W-22.4, приёмка T-14-01): каждая
// лемма семи файлов лемматизируется резидентом пакета в саму себя, вес в 1–3,
// пересечение со стоп-листом цензора пусто, ключи категорий уникальны, ярлыки
// стиля — только из канона styles.json. Без бинарника лемматизатора проверка
// лемм пропускается (образец — TestStoplistGolden в internal/censor).
func TestDictGates(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Skip("корень движка не найден — гейт пакета недоступен")
	}
	packDir := filepath.Join(root, "packs", "ru")

	dicts, err := deepread.LoadDictionaries(packDir)
	if err != nil {
		t.Fatalf("LoadDictionaries: %v", err)
	}
	if len(dicts) != len(deepread.DictionaryKeys) {
		t.Fatalf("загружено словарей %d, ожидалось %d", len(dicts), len(deepread.DictionaryKeys))
	}

	stoplist, err := censor.LoadStoplist(packDir)
	if err != nil {
		t.Fatalf("LoadStoplist: %v", err)
	}
	stop := make(map[string]struct{}, len(stoplist))
	for _, line := range stoplist {
		stop[strings.ToLower(strings.TrimSpace(line))] = struct{}{}
	}

	t.Run("реестр ключей", func(t *testing.T) {
		seen := make(map[string]struct{}, len(deepread.CategoryKeys))
		for _, key := range deepread.CategoryKeys {
			if _, dup := seen[key]; dup {
				t.Errorf("ключ %q повторяется в реестре", key)
			}
			seen[key] = struct{}{}
		}
		if len(deepread.CategoryKeys) != 12 {
			t.Errorf("ключей в реестре %d, ожидалось 12", len(deepread.CategoryKeys))
		}
		for _, key := range deepread.DictionaryKeys {
			if deepread.CategoryID(key) < 0 {
				t.Errorf("ключ словаря %q отсутствует в реестре", key)
			}
		}
	})

	t.Run("файлы словарей", func(t *testing.T) {
		for _, key := range deepread.DictionaryKeys {
			d, ok := dicts[key]
			if !ok {
				t.Errorf("словарь %q не загружен", key)
				continue
			}
			// Верхняя граница 80 — размер словарей v1; IMP-9 добавил золотые
			// якоря, social дошёл до 142 лемм.
			if n := len(d.Lemmas); n < 40 || n > 160 {
				t.Errorf("%s: лемм %d, ожидалось 40–160", key, n)
			}
			for lemma, weight := range d.Lemmas {
				if weight < 1 || weight > 3 {
					t.Errorf("%s: лемма %q: вес %d вне 1–3", key, lemma, weight)
				}
				if lemma != strings.ToLower(lemma) || strings.ContainsAny(lemma, " \t") {
					t.Errorf("%s: лемма %q не является одной словарной формой в нижнем регистре", key, lemma)
				}
				if _, bad := stop[lemma]; bad {
					t.Errorf("%s: лемма %q пересекается со стоп-листом цензора", key, lemma)
				}
			}
		}
	})

	t.Run("канон стиля", func(t *testing.T) {
		styles, err := deepread.LoadStyles(packDir)
		if err != nil {
			t.Fatalf("LoadStyles: %v", err)
		}
		got := make(map[string]struct{}, len(styles.Labels))
		for _, label := range styles.Labels {
			got[label.Name] = struct{}{}
		}
		for _, want := range deepread.StyleLabels {
			if _, ok := got[want]; !ok {
				t.Errorf("в styles.json нет ярлыка канона %q", want)
			}
		}
		if len(got) != len(deepread.StyleLabels) {
			t.Errorf("ярлыков в styles.json %d, канон — %d", len(got), len(deepread.StyleLabels))
		}
	})

	t.Run("леммы резидентом", func(t *testing.T) {
		binary := packLemmatizerBinary(packDir)
		if binary == "" {
			t.Skip("манифест пакета не задаёт бинарник лемматизатора")
		}
		if _, err := os.Stat(binary); err != nil {
			t.Skipf("бинарник лемматизатора недоступен: %v", err)
		}
		client := lemmarpc.NewClient(binary, nil)
		defer client.Close()

		for _, key := range deepread.DictionaryKeys {
			for lemma := range dicts[key].Lemmas {
				tokens, err := client.Lemmatize(lemma)
				if err != nil {
					t.Errorf("%s: лемма %q: лемматизатор отказал: %v", key, lemma, err)
					continue
				}
				if len(tokens) == 0 {
					t.Errorf("%s: лемма %q: лемматизатор вернул 0 токенов", key, lemma)
					continue
				}
				if got := strings.ToLower(tokens[0].Lemma); got != lemma {
					t.Errorf("%s: лемма %q: лемматизатор вернул %q — словарь и резидент разошлись",
						key, lemma, got)
				}
			}
		}
	})
}

// packLemmatizerBinary возвращает путь к резиденту из манифеста пака.
func packLemmatizerBinary(packDir string) string {
	raw, err := os.ReadFile(filepath.Join(packDir, "manifest.json"))
	if err != nil {
		return ""
	}
	var m struct {
		Lemmatizer struct {
			Binary string `json:"binary"`
		} `json:"lemmatizer"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || m.Lemmatizer.Binary == "" {
		return ""
	}
	name := m.Lemmatizer.Binary
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(packDir, filepath.FromSlash(name))
}

// findEngineRoot поднимается от рабочего каталога до каталога с go.mod.
func findEngineRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
