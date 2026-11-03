// Промоушен героев книги в мир (P12 §5.9 п.7, T-12-25): топ подтверждённых
// попадает в characters мира-фанфика, протагониста выделяет частотность
// упоминаний. Описание героя подставляется в пустую карточку по имени (LT-06.2).
package cast

import (
	"errors"
	"strings"

	"tns65/internal/db"
)

// StatusConfirmed — герой подтверждён (P12 §5.9 п.7): прошёл порог упоминаний
// или пришёл готовым списком имён; кандидат по умолчанию — StatusCandidate.
const StatusConfirmed = "confirmed"

// defaultPromoteLimit — потолок промоушена: топ-16 подтверждённых в мир-фанфик
// (P12 §5.9 п.7). Пачка остаётся 96 карточек, именные герои — отдельный слой.
const defaultPromoteLimit = 16

// PromoteTopConfirmed возвращает топ-n подтверждённых героев книги по частоте
// упоминаний; ничьи решаются confidence и id — результат детерминирован. n <= 0
// — дефолт defaultPromoteLimit.
func PromoteTopConfirmed(lit *db.Handle, bookID int64, n int) ([]BookCharacter, error) {
	if lit == nil {
		return nil, errors.New("cast: нет хэндла литературной базы")
	}
	if bookID <= 0 {
		return nil, errors.New("cast: промоушен без книги")
	}
	if n <= 0 {
		n = defaultPromoteLimit
	}

	rows, err := lit.Query(
		`SELECT id, book_id, name, aliases, appearance, description, traits, confidence, first_chunk, last_chunk, mentions, status, created_at
		 FROM book_character WHERE book_id = ? AND status = ? ORDER BY mentions DESC, confidence DESC, id ASC LIMIT ?`,
		bookID, StatusConfirmed, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BookCharacter
	for rows.Next() {
		var c BookCharacter
		if err := rows.Scan(&c.ID, &c.BookID, &c.Name, &c.Aliases, &c.Appearance, &c.Description,
			&c.Traits, &c.Confidence, &c.FirstChunk, &c.LastChunk, &c.Mentions, &c.Status, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DescriptionByName возвращает описание героя по имени в литературной базе
// (LT-06.2, T-12-25): описание, при пустом — внешность. Сверка регистронезависима
// и в Go (SQLite lower() не знает кириллицы). Героя нет или имя пустое —
// ("", false); подстановка декоративная, ошибка чтения гасится в false.
func DescriptionByName(lit *db.Handle, name string) (string, bool) {
	if lit == nil || name == "" {
		return "", false
	}
	want := strings.ToLower(strings.TrimSpace(name))
	rows, err := lit.Query(`SELECT name, description, appearance FROM book_character ORDER BY id`)
	if err != nil {
		return "", false
	}
	defer rows.Close()
	for rows.Next() {
		var n, desc, appearance string
		if err := rows.Scan(&n, &desc, &appearance); err != nil {
			return "", false
		}
		if strings.ToLower(strings.TrimSpace(n)) != want {
			continue
		}
		if desc != "" {
			return desc, true
		}
		return appearance, appearance != ""
	}
	return "", false
}

// DescriptionText возвращает текстовое описание героя для карточки: свободное
// описание, при пустом — внешность.
func (c BookCharacter) DescriptionText() string {
	if c.Description != "" {
		return c.Description
	}
	return c.Appearance
}
