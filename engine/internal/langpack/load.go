package langpack

import (
	"os"
	"path/filepath"
	"sort"
)

// Pack — загруженный языковой пакет.
type Pack struct {
	Dir      string
	Manifest *Manifest
}

// Open загружает манифест пакета из каталога dir и проверяет наличие файлов,
// суммы и версию (T-01-08, шаг 2). Расхождение — *BrokenFilesError.
func Open(dir string) (*Pack, error) {
	m, err := LoadManifest(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	if err := validateVersion(m.Version); err != nil {
		return nil, err
	}
	if err := m.verifyFiles(dir); err != nil {
		return nil, err
	}
	return &Pack{Dir: dir, Manifest: m}, nil
}

// Listing — строка списка пакетов: пакет, версия и флаг текущей.
type Listing struct {
	ID      string
	Version string
	Current bool
}

// List сканирует каталог пакетов и возвращает пакеты с версиями (шаг 5).
func List(packsDir string) ([]Listing, error) {
	entries, err := os.ReadDir(packsDir)
	if err != nil {
		return nil, err
	}

	var out []Listing
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(packsDir, e.Name())
		m, err := LoadManifest(filepath.Join(dir, "manifest.json"))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		current := m.Version
		if c, err := currentVersion(dir); err == nil {
			current = c
		}
		versions := listVersions(dir)
		if len(versions) == 0 {
			versions = []string{m.Version}
		}
		for _, v := range versions {
			out = append(out, Listing{ID: m.ID, Version: v, Current: v == current})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return compareVersion(out[i].Version, out[j].Version) < 0
	})
	return out, nil
}
