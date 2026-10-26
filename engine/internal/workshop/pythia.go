package workshop

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"tns65/internal/db"
	"tns65/internal/version"
)

// PythiaRole — роль слота ночного прогона Пифии.
const PythiaRole = "pythia"

// pythiaSource — источник строк карты игрока, пополненных ночным прогоном.
const pythiaSource = "pythia"

// Фасеты карты игрока в player_atlas.theme: какая грань профиля пополнена.
const (
	pythiaThemeInterest = "interest"
	pythiaThemeHabit    = "habit"
	pythiaThemeAvoid    = "avoid"
)

// pythiaMaxTurns — сколько последних реплик игрока уходит в промпт Пифии.
const pythiaMaxTurns = 100

// PythiaOutput — строгий JSON ночного прогона Пифии (W-12, шаг 2). Поле
// psychotype принимается и отбрасывается: в базу оно не пишется (W-12.4 —
// модель не пересчитывает психотип).
type PythiaOutput struct {
	Mood       string   `json:"mood"`
	Interests  []string `json:"interests"`
	Habits     []string `json:"habits"`
	Avoid      []string `json:"avoid"`
	Psychotype string   `json:"psychotype"`
}

// RunPythia — ночной прогон Пифии (шаги 1–4): по стартовым данным героя
// (world_meta.hero_birth), психотипу из пачки и паттернам последних реплик
// модель пополняет карту игрока. Новые строки карты пишутся в player_atlas
// хэндлом core, сводка ночного прогона — в forge_log хэндлом forge. Повторный
// прогон того же дня не дублирует записи (идемпотентность по дню).
func RunPythia(world *db.Handle, literary *db.Handle, day int) error {
	var n int
	if err := world.QueryRow(`SELECT COUNT(*) FROM player_atlas WHERE day = ?`, day).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	birth, err := heroBirth(world)
	if err != nil {
		return err
	}
	psychotype, err := playerPsychotype(world)
	if err != nil {
		return err
	}
	turns, err := recentPlayerTurns(world)
	if err != nil {
		return err
	}

	slot := slotForProvider(DefaultProvider)
	if slot == nil {
		return errors.New("workshop: слот Пифии не подключён")
	}
	out, err := slot.Generate(context.Background(), PythiaRole, pythiaPrompt(birth, psychotype, turns))
	if err != nil {
		return err
	}

	var po PythiaOutput
	if err := json.Unmarshal([]byte(out), &po); err != nil {
		return fmt.Errorf("workshop: выход Пифии не JSON: %w", err)
	}
	if err := validatePythiaOutput(po); err != nil {
		return err
	}

	if err := writePlayerAtlas(world, day, po); err != nil {
		return err
	}
	return writePythiaLog(literary)
}

// heroBirth читает стартовые данные карты игрока из world_meta.hero_birth
// (W-12.3); значение world_meta хранится JSON-строкой (T-02-03, шаг 3).
func heroBirth(h *db.Handle) (string, error) {
	var raw string
	err := h.QueryRow(`SELECT value FROM world_meta WHERE "key" = 'hero_birth'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("workshop: world_meta.hero_birth не найден")
	}
	if err != nil {
		return "", err
	}
	var birth string
	if err := json.Unmarshal([]byte(raw), &birth); err != nil {
		return "", fmt.Errorf("workshop: разбор hero_birth: %w", err)
	}
	return birth, nil
}

// playerPsychotype читает психотип героя из пачки: карточка героя — строка
// characters с id 0 (пачка NPC занимает 1..96, герой — 0). Психотип не
// пересчитывается (W-12.4): модель может вернуть свой психотип, но в базу он
// не пишется.
func playerPsychotype(h *db.Handle) (string, error) {
	var psychotype string
	err := h.QueryRow(`SELECT psychotype FROM characters WHERE id = 0`).Scan(&psychotype)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("workshop: карточка героя (characters id 0) не найдена")
	}
	if err != nil {
		return "", err
	}
	return psychotype, nil
}

// recentPlayerTurns собирает паттерны прошедших сессий: последние реплики
// игрока в порядке хода (W-12.4).
func recentPlayerTurns(h *db.Handle) ([]string, error) {
	rows, err := h.Query(`SELECT player_text FROM turns ORDER BY id DESC LIMIT ?`, pythiaMaxTurns)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var turns []string
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		turns = append(turns, text)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// ORDER BY id DESC собрал реплики от новой к старой; в промпте нужен порядок хода.
	for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
		turns[i], turns[j] = turns[j], turns[i]
	}
	return turns, nil
}

// pythiaPrompt строит промпт ночного прогона: стартовые данные героя, психотип
// и паттерны последних реплик.
func pythiaPrompt(birth, psychotype string, turns []string) string {
	raw, _ := json.Marshal(map[string]any{
		"task":       PythiaRole,
		"hero_birth": birth,
		"psychotype": psychotype,
		"turns":      turns,
	})
	return string(raw)
}

// validatePythiaOutput проверяет строгий JSON Пифии: настроение и все грани
// карты на месте. psychotype не проверяется — поле принимается и отбрасывается
// (W-12.4).
func validatePythiaOutput(po PythiaOutput) error {
	if po.Mood == "" {
		return errors.New("workshop: выход Пифии без настроения")
	}
	if po.Interests == nil || po.Habits == nil || po.Avoid == nil {
		return errors.New("workshop: выход Пифии без граней карты")
	}
	return nil
}

// writePlayerAtlas пишет новые строки карты игрока в player_atlas хэндлом core
// (append-only: новая строка на прогон, старые не перезаписываются). Фасет грани
// идёт в theme, значение — в interest.
func writePlayerAtlas(world *db.Handle, day int, po PythiaOutput) error {
	return world.Tx(db.WriterCore, func(tx *db.Tx) error {
		for _, v := range po.Interests {
			if _, err := tx.Exec("player_atlas",
				`INSERT INTO player_atlas (day, theme, mood, interest, source) VALUES (?, ?, ?, ?, ?)`,
				day, pythiaThemeInterest, po.Mood, v, pythiaSource); err != nil {
				return err
			}
		}
		for _, v := range po.Habits {
			if _, err := tx.Exec("player_atlas",
				`INSERT INTO player_atlas (day, theme, mood, interest, source) VALUES (?, ?, ?, ?, ?)`,
				day, pythiaThemeHabit, po.Mood, v, pythiaSource); err != nil {
				return err
			}
		}
		for _, v := range po.Avoid {
			if _, err := tx.Exec("player_atlas",
				`INSERT INTO player_atlas (day, theme, mood, interest, source) VALUES (?, ?, ?, ?, ?)`,
				day, pythiaThemeAvoid, po.Mood, v, pythiaSource); err != nil {
				return err
			}
		}
		return nil
	})
}

// writePythiaLog пишет сводку ночного прогона в forge_log хэндлом forge.
func writePythiaLog(literary *db.Handle) error {
	now := nowUTC()
	return literary.Tx(db.WriterForge, func(tx *db.Tx) error {
		_, err := tx.Exec("forge_log",
			`INSERT INTO forge_log (kind, task, status, started_at, finished_at, log_path, package_version) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			PythiaRole, PythiaRole, "done", now, now, "", version.Version)
		return err
	})
}
