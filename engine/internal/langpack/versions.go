package langpack

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// currentFileName — имя текстового файла-указателя текущей версии.
const currentFileName = "current"

// validateVersion — версия вида MAJOR.MINOR.PATCH (шаг 2, проверка версии).
func validateVersion(v string) error {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return fmt.Errorf("langpack: invalid version %q", v)
	}
	for _, p := range parts {
		if p == "" {
			return fmt.Errorf("langpack: invalid version %q", v)
		}
		if _, err := strconv.Atoi(p); err != nil {
			return fmt.Errorf("langpack: invalid version %q", v)
		}
	}
	return nil
}

// compareVersion — -1, 0, 1 по MAJOR.MINOR.PATCH.
func compareVersion(a, b string) int {
	pa := parseVersion(a)
	pb := parseVersion(b)
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

func parseVersion(v string) [3]int {
	var out [3]int
	for i, p := range strings.Split(v, ".") {
		if i >= 3 {
			break
		}
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

// currentVersion читает указатель current — текстовый файл с именем версии.
func currentVersion(packDir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(packDir, currentFileName))
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if v == "" {
		return "", fmt.Errorf("langpack: empty current pointer")
	}
	return v, nil
}

// writeCurrent записывает указатель current. Обычный файл, не симлинк (шаг 5).
func writeCurrent(packDir, version string) error {
	return os.WriteFile(filepath.Join(packDir, currentFileName), []byte(version+"\n"), 0o644)
}

// listVersions возвращает имена версий каталога versions/ по возрастанию.
func listVersions(packDir string) []string {
	entries, err := os.ReadDir(filepath.Join(packDir, "versions"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Slice(out, func(i, j int) bool { return compareVersion(out[i], out[j]) < 0 })
	return out
}

// Rollback переключает указатель current на предыдущую версию и проверяет её
// суммы (шаг 3). Возвращает новую текущую версию.
func Rollback(packDir string) (string, error) {
	cur, err := currentVersion(packDir)
	if err != nil {
		return "", fmt.Errorf("langpack: rollback: %w", err)
	}

	previous := ""
	for _, v := range listVersions(packDir) {
		if compareVersion(v, cur) >= 0 {
			continue
		}
		previous = v
	}
	if previous == "" {
		return "", fmt.Errorf("langpack: rollback: no previous version")
	}

	targetDir := filepath.Join(packDir, "versions", previous)
	if _, err := Open(targetDir); err != nil {
		return "", fmt.Errorf("langpack: rollback: verify %s: %w", previous, err)
	}
	if err := writeCurrent(packDir, previous); err != nil {
		return "", err
	}
	return previous, nil
}
