package paganel

import (
	"fmt"

	"tns65/internal/db"
)

// RingJournalEntry — карточка кольца для вкладки MCP (T-06-07, шаг 3): только
// статус, прогресс и счётчики ковки. Органов управления переключателями в
// карточке нет — переключатели остаются в настройках (T-06-05, settings.go).
type RingJournalEntry struct {
	ID          int64   `json:"id"`
	Status      string  `json:"status"`
	Progress    float64 `json:"progress"`
	PlacesCount int64   `json:"places_count"`
	PathsCount  int64   `json:"paths_count"`
	PeopleCount int64   `json:"people_count"`
}

// ForgeStats — статистика ковки: сколько мест и путей в draft и baked
// (T-06-07, шаг 1, «статусы draft → baked», W-05.9/W-05.10).
type ForgeStats struct {
	PlacesDraft int64 `json:"places_draft"`
	PlacesBaked int64 `json:"places_baked"`
	PathsDraft  int64 `json:"paths_draft"`
	PathsBaked  int64 `json:"paths_baked"`
}

// RingJournal — контракт данных вкладки MCP (T-06-07, шаг 3): текущее кольцо,
// очередь, готовые кольца и статистика ковки. Полей переключателей в контракте
// нет (шаг 2).
type RingJournal struct {
	Current RingJournalEntry   `json:"current"`
	Queue   []RingJournalEntry `json:"queue"`
	Done    []RingJournalEntry `json:"done"`
	Forge   ForgeStats         `json:"forge"`
}

// CollectRingJournal собирает журнал колец для вкладки MCP (T-06-07, шаг 1):
// очередь, текущее кольцо, процент готовности, счётчики мест/путей/упомянутых
// людей и статистику статусов draft → baked. Журнал — только чтение (шаг 2),
// ничего не пишет.
func CollectRingJournal(h *db.Handle) (RingJournal, error) {
	if h == nil {
		return RingJournal{}, fmt.Errorf("paganel: хэндл мира не задан")
	}
	rows, err := h.Query(
		`SELECT id, status, progress, places_count, paths_count, people_count
		 FROM rings ORDER BY id`)
	if err != nil {
		return RingJournal{}, fmt.Errorf("paganel: журнал колец: %w", err)
	}
	defer rows.Close()

	j := RingJournal{}
	for rows.Next() {
		var e RingJournalEntry
		if err := rows.Scan(&e.ID, &e.Status, &e.Progress,
			&e.PlacesCount, &e.PathsCount, &e.PeopleCount); err != nil {
			return RingJournal{}, fmt.Errorf("paganel: строка журнала колец: %w", err)
		}
		switch e.Status {
		case RingRunning:
			if j.Current.ID == 0 {
				j.Current = e
			}
		case RingQueued:
			j.Queue = append(j.Queue, e)
		case RingDone:
			j.Done = append(j.Done, e)
		}
	}
	if err := rows.Err(); err != nil {
		return RingJournal{}, fmt.Errorf("paganel: журнал колец: %w", err)
	}

	forge, err := readForgeStats(h)
	if err != nil {
		return RingJournal{}, err
	}
	j.Forge = forge
	return j, nil
}

// readForgeStats считает места и пути по статусу draft/baked — статистика
// ковки для журнала (T-06-07, шаг 1). Пустой таблице соответствует ноль.
func readForgeStats(h *db.Handle) (ForgeStats, error) {
	var s ForgeStats
	err := h.QueryRow(
		`SELECT
		    COALESCE(SUM(CASE WHEN status = 'draft' THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN status = 'baked' THEN 1 ELSE 0 END), 0)
		 FROM places`).Scan(&s.PlacesDraft, &s.PlacesBaked)
	if err != nil {
		return ForgeStats{}, fmt.Errorf("paganel: статистика мест: %w", err)
	}
	err = h.QueryRow(
		`SELECT
		    COALESCE(SUM(CASE WHEN status = 'draft' THEN 1 ELSE 0 END), 0),
		    COALESCE(SUM(CASE WHEN status = 'baked' THEN 1 ELSE 0 END), 0)
		 FROM paths`).Scan(&s.PathsDraft, &s.PathsBaked)
	if err != nil {
		return ForgeStats{}, fmt.Errorf("paganel: статистика путей: %w", err)
	}
	return s, nil
}
