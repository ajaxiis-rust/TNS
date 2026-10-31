package structural

import (
	"errors"
	"fmt"

	"tns65/internal/db"
)

// Области счётчиков дрейфа family_usage (SCR-§3.3, §6.2): chapter-счётчики
// сбрасываются на границе главы, book-счётчики живут всю книгу.
const (
	ScopeChapter = "chapter"
	ScopeBook    = "book"
)

// DefaultDriftForbidAfter — порог N правила «почти запрещает» (SCR-§3.3):
// при usage_chapter[family] ≥ N семья исключается из кандидатов, если есть
// кандидат другой семьи; иначе берётся наименее используемая (отказ хода
// запрещён). Рекомендуемое значение N = 3–4.
const DefaultDriftForbidAfter = 3

// DefaultForceEvery — период правила «периодически обязана» (SCR-§3.3):
// каждые force_every выборов композиция обязана взять кандидата с inversion
// или наименьший usage_book.
const DefaultForceEvery = 5

// validScope — допустимая область счётчика (CHECK в 0012_structural_drift.sql).
func validScope(scope string) bool {
	return scope == ScopeChapter || scope == ScopeBook
}

// BumpUsage увеличивает счётчик семьи в заданной области на единицу (апсерт).
// Запись идёт через хэндл `core`; чужой хэндл даёт db.ErrForeignWriter по карте
// писателей (family_usage → core).
func BumpUsage(h *db.Handle, scope, family string, now string) error {
	if h == nil {
		return errors.New("structural: нет хэндла мира")
	}
	if !validScope(scope) {
		return fmt.Errorf("structural: область счётчика %q вне ('chapter','book')", scope)
	}
	if family == "" {
		return errors.New("structural: семейство счётчика не задано")
	}
	_, err := h.Exec("family_usage",
		`INSERT INTO family_usage (scope, family, count, updated_at) VALUES (?, ?, 1, ?)
		 ON CONFLICT(scope, family) DO UPDATE SET count = family_usage.count + 1, updated_at = excluded.updated_at`,
		scope, family, now)
	return err
}

// Usage читает счётчики всех семей в области scope. Пустая область — пустая
// карта без ошибки.
func Usage(h *db.Handle, scope string) (map[string]int, error) {
	if h == nil {
		return nil, errors.New("structural: нет хэндла мира")
	}
	if !validScope(scope) {
		return nil, fmt.Errorf("structural: область счётчика %q вне ('chapter','book')", scope)
	}
	rows, err := h.Query(`SELECT family, count FROM family_usage WHERE scope = ?`, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	usage := make(map[string]int)
	for rows.Next() {
		var family string
		var count int
		if err := rows.Scan(&family, &count); err != nil {
			return nil, err
		}
		usage[family] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return usage, nil
}

// ResetScope удаляет все счётчики области scope (граница главы — SCR-§6.2).
// Запись идёт через хэндл `core`; чужой хэндл даёт db.ErrForeignWriter.
func ResetScope(h *db.Handle, scope, now string) error {
	if h == nil {
		return errors.New("structural: нет хэндла мира")
	}
	if !validScope(scope) {
		return fmt.Errorf("structural: область счётчика %q вне ('chapter','book')", scope)
	}
	_, err := h.Exec("family_usage",
		`DELETE FROM family_usage WHERE scope = ?`, scope)
	return err
}
