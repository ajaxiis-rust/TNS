package webapi

import (
	"encoding/json"
	"os"
	"path/filepath"

	"tns65/internal/journal"
	"tns65/internal/lemmarpc"
	"tns65/internal/turn"
)

// SetChatLemmatizer подключает лемматизатор пакета к чатовому конвейеру.
// Прод-путь (cmd/tns65) вызывает её с клиентом языкового пакета по умолчанию
// packs/ru — тот же пакет, что читает loadDefaultLexicon. Без вызова
// chatLemmatizer остаётся nil: лемм нет, гипотеза намерения не считается и
// каждый ход уходит в переспрос.
func SetChatLemmatizer(l turn.Lemmatizer) {
	chatLemmatizer = l
}

// DefaultPackLemmatizer собирает лемматизатор языкового пакета по умолчанию
// (packs/ru) из корня работающего экземпляра. Бинарник берётся из manifest.json
// пакета — тот же формат, что читает цензор. Пакета или записи нет — nil, чат
// деградирует до прежнего поведения без лемм.
func DefaultPackLemmatizer(logger *journal.Logger) turn.Lemmatizer {
	bin := chatPackLemmatizerBinary(instanceRoot(), "ru")
	if bin == "" {
		return nil
	}
	return lemmarpc.NewClient(bin, logger)
}

// instanceRoot повторяет резолв цензора: каталог с go.mod, а в развёрнутой папке
// без go.mod — рабочий каталог, рядом с которым лежат base/, packs/, state/.
func instanceRoot() string {
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

// chatPackLemmatizerBinary читает manifest.json пакета lang и возвращает путь
// бинарника лемматизатора; пусто — пакета или записи нет.
func chatPackLemmatizerBinary(root, lang string) string {
	packDir := filepath.Join(root, "packs", lang)
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
