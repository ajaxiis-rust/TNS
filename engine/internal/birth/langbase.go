package birth

import (
	"os"
	"sort"
	"strings"

	"tns65/internal/db"
)

// Префикс и расширение имени файла базы языка base.<lang>.db в каталоге
// поставки db.BaseRoot (T-10-03).
const (
	langBasePrefix = "base."
	langBaseSuffix = ".db"
)

// AvailableLangs возвращает коды языков установленных баз base.<lang>.db в
// каталоге поставки db.BaseRoot, отсортированные по алфавиту. Легаси-слитая
// base.db (TUN-8) и служебные файлы (.sha256, -wal, -shm) в список не входят.
func AvailableLangs() ([]string, error) {
	entries, err := os.ReadDir(db.BaseRoot)
	if err != nil {
		return nil, err
	}
	var langs []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "base.db" {
			continue // легаси-слитая база — не база одного языка.
		}
		if !strings.HasPrefix(name, langBasePrefix) || !strings.HasSuffix(name, langBaseSuffix) {
			continue
		}
		lang := name[len(langBasePrefix) : len(name)-len(langBaseSuffix)]
		if lang == "" {
			continue
		}
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs, nil
}

// ValidateLang проверяет язык анкеты по набору установленных баз (TUN-2):
// пустой язык и язык без base.<lang>.db в db.BaseRoot отвергаются FieldError
// по полю lang. Валидация ничего не пишет — результат в память вызывающего.
func ValidateLang(a *Ancketa) *FieldError {
	if a.Lang == "" {
		return &FieldError{Field: "lang", Reason: "язык не указан"}
	}
	langs, err := AvailableLangs()
	if err != nil {
		langs = nil
	}
	for _, l := range langs {
		if l == a.Lang {
			return nil
		}
	}
	return &FieldError{Field: "lang", Reason: "язык не установлен: нет базы base." + a.Lang + ".db"}
}

// validateLangForConfig — проверка языка в Birth (T-10-03): при явном пути
// BaseDBPath (тесты/атлас) проверяется существование этого файла, иначе — набор
// установленных баз db.BaseRoot.
func validateLangForConfig(a *Ancketa, cfg Config) *FieldError {
	if a.Lang == "" {
		return &FieldError{Field: "lang", Reason: "язык не указан"}
	}
	if cfg.BaseDBPath != "" {
		if _, err := os.Stat(cfg.BaseDBPath); err != nil {
			return &FieldError{Field: "lang", Reason: "база языка не найдена: " + cfg.BaseDBPath}
		}
		return nil
	}
	return ValidateLang(a)
}
