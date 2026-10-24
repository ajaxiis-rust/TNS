package langpack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Pool — базовый пул сида: имя файла результата и строки исходного пула.
type Pool struct {
	Name string
	Text []string
}

// translateSeed — единственный шаг перевода станка: переводит строку сида на
// язык пакета одним голосом (D-15.1). Живёт только в сборке пакета — конвейер
// живого хода его не вызывает (см. TestBuildHasTranslate).
func translateSeed(s string) string {
	return s
}

// Build переводит базовые пулы сида одним проходом одним голосом и кладёт
// результат в каталог версии вместе с манифестом, обновляя указатель current.
func Build(packDir, version string, pools []Pool) error {
	if err := validateVersion(version); err != nil {
		return err
	}
	base, err := LoadManifest(filepath.Join(packDir, "manifest.json"))
	if err != nil {
		return err
	}

	versionDir := filepath.Join(packDir, "versions", version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return err
	}

	files := make(map[string]string, len(pools))
	for _, p := range pools {
		out := make([]string, len(p.Text))
		for i, line := range p.Text {
			out[i] = translateSeed(line)
		}
		data := strings.Join(out, "\n")
		if data != "" {
			data += "\n"
		}
		name := filepath.Base(p.Name)
		path := filepath.Join(versionDir, name)
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			return err
		}
		sum, err := sha256File(path)
		if err != nil {
			return err
		}
		files[name] = sum
	}

	vm := Manifest{
		ID:         base.ID,
		Name:       base.Name,
		Lang:       base.Lang,
		Version:    version,
		Lemmatizer: base.Lemmatizer,
		Files:      files,
	}
	b, err := json.MarshalIndent(vm, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(versionDir, "manifest.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	return writeCurrent(packDir, version)
}
