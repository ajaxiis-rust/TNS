package birth

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"tns65/internal/db"
)

// ErrNoWorld — переигровка isekai мира, которого нет (или который не готов).
var ErrNoWorld = errors.New("birth: мир для переигровки не найден")

// Константы переигровки isekai (T-02-12): вид события истории мира, режим
// новой сессии и первая глава новой книги.
const (
	pastHeroKind        = "past_hero"
	pastHeroReasonLeave = "departure"
	sourceIsekai        = "isekai"
	sessionModeIsekai   = "isekai"
	firstChapterTitle   = "Глава 1"
	chapterStatusOpen   = "open"
	chapterKindChapter  = "chapter"
)

// pastHeroPayload — содержимое события past_hero: уход прошлого героя из мира
// (смерть или уход, B-11.4).
type pastHeroPayload struct {
	Reason       string `json:"reason"`
	PreviousHero string `json:"previous_hero"`
}

// replayBirth — ветка повторного рождения (T-02-12): папка мира и обе базы не
// пересоздаются, пять замороженных ключей world_meta не трогаются, пачка не
// перегенерируется. В историю мира пишется событие past_hero, открываются
// новая сессия и первая глава новой книги.
func replayBirth(a *Ancketa, root string, cfg Config) (*Result, error) {
	dir := filepath.Join(root, "worlds", a.Slug)
	if st, err := readState(dir); err != nil || st != stateReady {
		return nil, fmt.Errorf("%w: %q", ErrNoWorld, a.Slug)
	}

	worldDB := filepath.Join(dir, a.Slug+".db")
	h, err := db.OpenWriterAs(worldDB, db.DBWorld, db.WriterCore, cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("birth: открыть мир для переигровки: %w", err)
	}
	defer h.Close()

	// Прошлый герой — значение переспрашиваемого поля hero_birth, оставшееся в
	// world_meta от прошлой жизни; оно же попадает в след past_hero.
	prevHero, err := readHeroBirth(h)
	if err != nil {
		return nil, fmt.Errorf("birth: прочитать прошлого героя: %w", err)
	}

	if err := h.Tx(db.WriterCore, func(tx *db.Tx) error {
		if err := recordPastHero(tx, prevHero); err != nil {
			return err
		}
		if err := openSession(tx); err != nil {
			return err
		}
		return openChapter(tx)
	}); err != nil {
		return nil, fmt.Errorf("birth: переигровка: %w", err)
	}

	return &Result{
		Slug:       a.Slug,
		Dir:        dir,
		WorldDB:    worldDB,
		LiteraryDB: filepath.Join(dir, "literary.db"),
	}, nil
}

// readHeroBirth читает прошлого героя из world_meta (JSON-значение).
func readHeroBirth(h *db.Handle) (string, error) {
	var raw string
	if err := h.QueryRow(`SELECT value FROM world_meta WHERE "key" = 'hero_birth'`).Scan(&raw); err != nil {
		return "", err
	}
	var hero string
	if err := json.Unmarshal([]byte(raw), &hero); err != nil {
		return "", fmt.Errorf("birth: разбор hero_birth: %w", err)
	}
	return hero, nil
}

// recordPastHero пишет событие past_hero в историю мира (events).
func recordPastHero(tx *db.Tx, prevHero string) error {
	payload, err := json.Marshal(pastHeroPayload{Reason: pastHeroReasonLeave, PreviousHero: prevHero})
	if err != nil {
		return err
	}
	_, err = tx.Exec("events",
		`INSERT INTO events (day, kind, payload_json, source) VALUES (?, ?, ?, ?)`,
		0, pastHeroKind, string(payload), sourceIsekai)
	return err
}

// openSession открывает новую сессию переигровки.
func openSession(tx *db.Tx) error {
	_, err := tx.Exec("sessions",
		`INSERT INTO sessions (started_at, mode) VALUES (?, ?)`,
		time.Now().UTC().Format(time.RFC3339), sessionModeIsekai)
	return err
}

// openChapter открывает первую главу новой книги.
func openChapter(tx *db.Tx) error {
	_, err := tx.Exec("book_chapters",
		`INSERT INTO book_chapters (day, title, status, kind) VALUES (?, ?, ?, ?)`,
		1, firstChapterTitle, chapterStatusOpen, chapterKindChapter)
	return err
}
