package structural

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// Family — одно семейство структурной композиции (SCR-§1.2): канонический
// slug, англ. глосса и ключевые слова для сопоставления. Канонический порядок
// — порядок файла archetypes.json.
type Family struct {
	Name     string   `json:"name"`
	GlossEN  string   `json:"gloss_en"`
	Keywords []string `json:"keywords"`
}

// FamiliesPath — единая точка пути к реестру archetypes.json относительно
// корня движка. engineRoot — тот же корень, от которого берётся db.BaseRoot
// (T-10-02).
func FamiliesPath(engineRoot string) string {
	return filepath.Join(engineRoot, "base", "archetypes.json")
}

// nameRe — каноническая форма имени семейства (slug из сквозного канона TUN-6).
var nameRe = regexp.MustCompile(`^[a-z_]+$`)

// LoadFamilies читает и валидирует реестр семей из JSON-файла. Загрузчик не
// сортирует: канонический порядок равен порядку записей в файле.
func LoadFamilies(path string) ([]Family, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("structural: read %s: %w", path, err)
	}
	var families []Family
	if err := json.Unmarshal(data, &families); err != nil {
		return nil, fmt.Errorf("structural: %s: %w", path, err)
	}
	if len(families) != 14 {
		return nil, fmt.Errorf("structural: семей %d, want 14", len(families))
	}
	seen := make(map[string]struct{}, len(families))
	for i, f := range families {
		if f.Name == "" {
			return nil, fmt.Errorf("structural: запись %d: пустое имя", i)
		}
		if !nameRe.MatchString(f.Name) {
			return nil, fmt.Errorf("structural: запись %d: имя %q не соответствует ^[a-z_]+$", i, f.Name)
		}
		if f.Name == "none" {
			return nil, fmt.Errorf("structural: запись %d: резерв %q не является семейством", i, f.Name)
		}
		if _, dup := seen[f.Name]; dup {
			return nil, fmt.Errorf("structural: запись %d: дубль имени %q", i, f.Name)
		}
		seen[f.Name] = struct{}{}
		if len(f.Keywords) == 0 {
			return nil, fmt.Errorf("structural: семейство %q без ключевых слов", f.Name)
		}
	}
	return families, nil
}
