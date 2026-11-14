package birth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"tns65/internal/db"
	"tns65/internal/journal"
)

// DeepScout — фоновая глубокая разведка при рождении (DEC-4, B-14.4): живой
// вызов реальной модели. Реализация — paganel.DeepScout; в тестах подменяется
// счётчиком.
type DeepScout interface {
	Run(ctx context.Context, topics []string) (string, error)
}

// scoutReportKey — ключ world_meta с результатом глубокой разведки (DEC-4).
// Значение — JSON-текст, как у остальных ключей world_meta.
const scoutReportKey = "scout_report"

// scoutReport — отчёт глубокой разведки в world_meta: темы, текст и время.
// Стартовую географию T-02-16 отчёт не заменяет — только дополняет фоном.
type scoutReport struct {
	Topics []string `json:"topics"`
	Text   string   `json:"text"`
	At     string   `json:"at"`
}

// deepScoutTopics — темы глубокой разведки при рождении: география, эпоха и
// культура, нравы и экономика. Три темы — бюджет ~3 вызова (как Э5).
func deepScoutTopics(a *Ancketa) []string {
	title := strings.TrimSpace(a.Title)
	synopsis := strings.TrimSpace(a.Synopsis)
	base := strings.TrimSpace(title + ". " + synopsis)
	return []string{
		"География и места мира: " + base,
		"Эпоха, культура и техника: " + strings.TrimSpace(a.SocialOrder) + "; магия: " + strings.TrimSpace(a.Magic),
		"Нравы, экономика и быт: " + strings.Join(a.EconomyMods, ", ") + "; " + base,
	}
}

// runDeepScout — фоновая задача после рождения (B-14.4): разведка по темам и
// запись отчёта в world_meta хэндлом forge. Ошибка и тайм-аут — пропуск:
// рождение и мир не затрагиваются. Повторный прогон видит готовый отчёт и
// разведку не дублирует.
func runDeepScout(worldDB string, logger *journal.Logger, scout DeepScout, topics []string) {
	if scout == nil || len(topics) == 0 {
		return
	}
	if hasScoutReport(worldDB) {
		logScoutReport(logger, "skip", "отчёт разведки уже есть")
		return
	}
	text, err := scout.Run(context.Background(), topics)
	if strings.TrimSpace(text) == "" {
		if err != nil {
			logScoutReport(logger, "error", err.Error())
		}
		return
	}
	if err := writeScoutReport(worldDB, logger, topics, text); err != nil {
		logScoutReport(logger, "error", "запись отчёта: "+err.Error())
		return
	}
	logScoutReport(logger, "ok", "")
}

// hasScoutReport проверяет, осел ли отчёт разведки в world_meta.
func hasScoutReport(worldDB string) bool {
	d, err := db.OpenReader(worldDB)
	if err != nil {
		return false
	}
	defer d.Close()
	var raw string
	err = d.QueryRow(`SELECT value FROM world_meta WHERE "key" = ?`, scoutReportKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	return err == nil && strings.TrimSpace(raw) != ""
}

// writeScoutReport пишет отчёт разведки в world_meta хэндлом forge (карта
// писателей T-01-07). Существующий ключ перезаписывается: отчёт один на мир.
func writeScoutReport(worldDB string, logger *journal.Logger, topics []string, text string) error {
	raw, err := json.Marshal(scoutReport{
		Topics: topics,
		Text:   text,
		At:     time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	h, err := db.OpenWriterAs(worldDB, db.DBWorld, db.WriterForge, logger)
	if err != nil {
		return err
	}
	defer h.Close()
	_, err = h.Exec("world_meta",
		`INSERT OR REPLACE INTO world_meta ("key", value) VALUES (?, ?)`,
		scoutReportKey, string(raw))
	return err
}

// logScoutReport пишет итог фоновой разведки в журнал станка.
func logScoutReport(logger *journal.Logger, status, detail string) {
	if logger == nil {
		return
	}
	fields := []journal.Field{{Key: "status", Value: status}}
	if detail != "" {
		fields = append(fields, journal.Field{Key: "error", Value: detail})
	}
	_ = logger.Event("birth", journal.EventScoutReport, fields...)
}
