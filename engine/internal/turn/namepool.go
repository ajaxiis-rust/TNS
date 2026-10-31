package turn

import (
	"encoding/json"
	"fmt"
	"sort"

	"tns65/internal/db"
)

// poolTraits — traits_json карточки пула имён: для детерминированного порядка
// нужно только significance (TUN-5): major имена назначаются раньше прочих.
type poolTraits struct {
	Significance string `json:"significance"`
}

// poolName — строка пула имён из базы языка.
type poolName struct {
	id    int64
	name  string
	major bool
}

// PickPoolName — имя NPC из пула базы языка (TUN-5, шаг 1). Детерминированный
// порядок: significance='major' раньше прочих, внутри групп — по id; имя из
// taken пропускается; пул исчерпан → пустая строка без ошибки (имя остаётся
// назначаться позже).
func PickPoolName(basePath, lang string, taken map[string]bool) (string, error) {
	d, err := db.OpenReader(basePath)
	if err != nil {
		return "", fmt.Errorf("turn: открыть базу языка для пула имён: %w", err)
	}
	defer d.Close()

	rows, err := d.Query(`SELECT id, name, traits_json FROM characters WHERE lang = ? ORDER BY id`, lang)
	if err != nil {
		return "", fmt.Errorf("turn: читать пул имён: %w", err)
	}
	defer rows.Close()

	var pool []poolName
	for rows.Next() {
		var p poolName
		var raw string
		if err := rows.Scan(&p.id, &p.name, &raw); err != nil {
			return "", fmt.Errorf("turn: разбор имени пула: %w", err)
		}
		var tr poolTraits
		if raw != "" {
			_ = json.Unmarshal([]byte(raw), &tr)
		}
		p.major = tr.Significance == "major"
		pool = append(pool, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	// Стабильная сортировка по major сохраняет порядок id внутри групп:
	// строки читаются в порядке id, major выносятся вперёд без перестановки
	// внутри групп.
	sort.SliceStable(pool, func(i, j int) bool {
		return pool[i].major && !pool[j].major
	})

	for _, p := range pool {
		if p.name == "" || taken[p.name] {
			continue
		}
		return p.name, nil
	}
	return "", nil
}
