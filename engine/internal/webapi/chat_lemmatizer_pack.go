//go:build !e2e

package webapi

import (
	"tns65/internal/journal"
	"tns65/internal/lemmarpc"
	"tns65/internal/turn"
)

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
