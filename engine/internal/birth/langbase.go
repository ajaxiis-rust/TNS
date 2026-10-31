package birth

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tns65/internal/db"
	"tns65/internal/journal"
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
// установленных баз db.BaseRoot. После существования базы сверяется её
// контрольная сумма (TUN-9, T-10-04).
func validateLangForConfig(a *Ancketa, cfg Config) *FieldError {
	if a.Lang == "" {
		return &FieldError{Field: "lang", Reason: "язык не указан"}
	}

	basePath := langBasePath(a, cfg)
	if cfg.BaseDBPath != "" {
		if _, err := os.Stat(basePath); err != nil {
			return &FieldError{Field: "lang", Reason: "база языка не найдена: " + basePath}
		}
	} else if fe := ValidateLang(a); fe != nil {
		return fe
	}

	if err := VerifyBaseChecksum(basePath); err != nil {
		if errors.Is(err, errBaseChecksumMismatch) {
			logBaseCorrupt(cfg.Logger, basePath)
		}
		return &FieldError{Field: "lang", Reason: err.Error()}
	}
	return nil
}

// langBasePath — путь базы языка анкеты: явный cfg.BaseDBPath или каталог
// поставки db.BaseRoot/base.<lang>.db (T-10-02).
func langBasePath(a *Ancketa, cfg Config) string {
	if cfg.BaseDBPath != "" {
		return cfg.BaseDBPath
	}
	return filepath.Join(db.BaseRoot, "base."+a.Lang+".db")
}

// logBaseCorrupt пишет событие birth.base_corrupt в журнал рождения при
// несовпадении контрольной суммы базы языка (T-10-04, шаг 3).
func logBaseCorrupt(logger *journal.Logger, path string) {
	if logger == nil {
		return
	}
	_ = logger.Event("birth", journal.EventBaseCorrupt,
		journal.Field{Key: "path", Value: path})
}
