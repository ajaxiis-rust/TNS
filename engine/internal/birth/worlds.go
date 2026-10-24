package birth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"tns65/internal/db"
)

// World — мир в списке миров (T-02-10, шаг 1): слаг, заголовок, дата рождения
// и путь к папке.
type World struct {
	Slug      string
	Title     string
	BirthDate time.Time
	Dir       string
}

// List возвращает готовые миры из корня миров root (worlds/<slug>/), сортировка
// по слагу. Остатки прерванного рождения (состояние отлично от ready) в список
// не попадают. Пустой root означает db.WorldsRoot.
func List(root string) ([]World, error) {
	if root == "" {
		root = db.WorldsRoot
	}
	worldsDir := filepath.Join(root, "worlds")
	entries, err := os.ReadDir(worldsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("birth: читать каталог миров: %w", err)
	}

	var out []World
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		dir := filepath.Join(worldsDir, slug)

		// Миром считается только каталог с меткой состояния ready (шаг 2:
		// остаток прерванного рождения откатывается и не попадает в список).
		st, err := readState(dir)
		if err != nil || st != stateReady {
			continue
		}

		w := World{Slug: slug, Dir: dir}

		// Дата рождения — время финализации мира: метка состояния state
		// записывается последней в цепочке рождения и больше не трогается.
		if fi, err := os.Stat(filepath.Join(dir, stateFileName)); err == nil {
			w.BirthDate = fi.ModTime()
		}

		// Заголовок — имя мира в списке (B-02.1); слаг — часть имени файла
		// базы <slug>.db (B-01.1, шаг 2).
		w.Title, err = readMetaTitle(filepath.Join(dir, slug+".db"))
		if err != nil {
			return nil, err
		}

		out = append(out, w)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// readMetaTitle читает заголовок мира из world_meta оперативной базы. Значения
// world_meta хранятся JSON-строками (T-02-03, шаг 3).
func readMetaTitle(worldDB string) (string, error) {
	d, err := db.OpenReader(worldDB)
	if err != nil {
		return "", fmt.Errorf("birth: открыть базу мира %s: %w", worldDB, err)
	}
	defer d.Close()

	var raw string
	if err := d.QueryRow(`SELECT value FROM world_meta WHERE "key" = 'title'`).Scan(&raw); err != nil {
		return "", fmt.Errorf("birth: читать заголовок мира %s: %w", worldDB, err)
	}
	var title string
	if err := json.Unmarshal([]byte(raw), &title); err != nil {
		return "", fmt.Errorf("birth: разбор заголовка мира %s: %w", worldDB, err)
	}
	return title, nil
}
