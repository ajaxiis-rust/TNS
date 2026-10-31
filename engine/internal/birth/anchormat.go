package birth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"tns65/internal/db"
)

// anchorVersesLimit — потолок битов якоря, материализуемых в world_meta
// (T-10-06, шаг 2).
const anchorVersesLimit = 12

// Ключи world_meta сценарной рамки якоря (T-10-06, шаг 2).
const (
	anchorNameKey     = "anchor_name"
	anchorSummaryKey  = "anchor_summary"
	anchorVersesKey   = "anchor_verses"
	anchorFamiliesKey = "anchor_families"
)

// materializeAnchor — шаг цепочки рождения после якоря: превращает
// нарратив-якорь в сценарный материал (имя, дуга, биты, семьи) и пишет четыре
// ключа world_meta хэндлом forge (TUN-4, TUN-6; SCR-§4).
func (b *builder) materializeAnchor() error {
	id, err := readAnchorNarrativeID(b.worldDB)
	if err != nil {
		return err
	}

	base, err := db.OpenReader(b.baseDBPath)
	if err != nil {
		return fmt.Errorf("birth: открыть базу языка для чтения: %w", err)
	}
	defer base.Close()

	var name, summary string
	if err := base.QueryRow(`SELECT name, arc_summary FROM narratives WHERE id = ?`, id).Scan(&name, &summary); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("якорный нарратив %d не найден в базе языка", id)
		}
		return fmt.Errorf("birth: читать нарратив-якорь: %w", err)
	}

	verses, err := anchorVerses(base, id)
	if err != nil {
		return err
	}
	families, err := anchorFamilies(base, id)
	if err != nil {
		return err
	}

	h, err := db.OpenWriterAs(b.worldDB, db.DBWorld, db.WriterForge, b.logger)
	if err != nil {
		return err
	}
	defer h.Close()

	return writeAnchorMeta(h, name, summary, verses, families)
}

// readAnchorNarrativeID читает ссылку на нарратив-якорь из world_meta, куда её
// положил шаг anchor.
func readAnchorNarrativeID(worldDB string) (int64, error) {
	d, err := db.OpenReader(worldDB)
	if err != nil {
		return 0, fmt.Errorf("birth: открыть мир для чтения якоря: %w", err)
	}
	defer d.Close()

	var raw string
	if err := d.QueryRow(`SELECT value FROM world_meta WHERE "key" = ?`, anchorKey).Scan(&raw); err != nil {
		return 0, fmt.Errorf("birth: читать якорь из world_meta: %w", err)
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("birth: значение якоря %q не число: %w", raw, err)
	}
	return id, nil
}

// anchorVerses читает биты якоря из базы языка в порядке ord, но не более
// anchorVersesLimit. Пустой набор даёт [] (деградация без отказа, B-10).
func anchorVerses(base *sql.DB, id int64) ([]string, error) {
	rows, err := base.Query(`SELECT text FROM verses WHERE narrative_id = ? ORDER BY ord LIMIT ?`, id, anchorVersesLimit)
	if err != nil {
		return nil, fmt.Errorf("birth: читать verses якоря: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0, anchorVersesLimit)
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, fmt.Errorf("birth: разбор verse якоря: %w", err)
		}
		out = append(out, text)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// anchorFamilies читает слаги семей якоря из patterns.kind (канон TUN-6).
// Пустой набор даёт [] (деградация без отказа, B-10).
func anchorFamilies(base *sql.DB, id int64) ([]string, error) {
	rows, err := base.Query(`SELECT DISTINCT kind FROM patterns WHERE narrative_id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("birth: читать patterns якоря: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			return nil, fmt.Errorf("birth: разбор pattern якоря: %w", err)
		}
		out = append(out, kind)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// writeAnchorMeta пишет четыре ключа world_meta одним хэндлом forge. Значения —
// JSON-текст, как у остальных ключей world_meta (T-02-03, шаг 3).
func writeAnchorMeta(h *db.Handle, name, summary string, verses, families []string) error {
	nameJSON, err := json.Marshal(name)
	if err != nil {
		return fmt.Errorf("birth: маршалинг anchor_name: %w", err)
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("birth: маршалинг anchor_summary: %w", err)
	}
	versesJSON, err := json.Marshal(verses)
	if err != nil {
		return fmt.Errorf("birth: маршалинг anchor_verses: %w", err)
	}
	familiesJSON, err := json.Marshal(families)
	if err != nil {
		return fmt.Errorf("birth: маршалинг anchor_families: %w", err)
	}

	for _, kv := range [][2]string{
		{anchorNameKey, string(nameJSON)},
		{anchorSummaryKey, string(summaryJSON)},
		{anchorVersesKey, string(versesJSON)},
		{anchorFamiliesKey, string(familiesJSON)},
	} {
		if _, err := h.Exec("world_meta",
			`INSERT INTO world_meta ("key", value) VALUES (?, ?)`, kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}
