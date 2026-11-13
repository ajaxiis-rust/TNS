//go:build e2e

package webapi

import (
	"tns65/internal/journal"
	"tns65/internal/turn"
)

// DefaultPackLemmatizer в e2e-сборке отдаёт подставной лемматизатор (INT-3):
// рабочий каталог харнесса — временный, живого packs/ru рядом нет, а без лемм
// гипотеза намерения не строится и ход не коммитится.
func DefaultPackLemmatizer(_ *journal.Logger) turn.Lemmatizer {
	return e2eLemmatizer{}
}
