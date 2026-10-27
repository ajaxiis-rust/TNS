package book

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"tns65/internal/db"
	"tns65/internal/workshop"
)

// WakeStatus — статус пробуждения (CHECK в 0007_book_wakeup.sql).
type WakeStatus string

const (
	WakeForging WakeStatus = "forging"
	WakeReady   WakeStatus = "ready"
)

// Wakeup — пробуждение главы (T-07-02, шаг 1): выкованный заранее текст утра,
// который подаётся при открытии главы. chapter_id уникален — повторная ковка
// главы не создаёт вторую запись.
type Wakeup struct {
	ID          int64
	ChapterID   int64
	Text        string
	Status      WakeStatus
	ForgedAt    string
	DeliveredAt string
}

// ErrNoWakeup — пробуждение главы не выковано.
var ErrNoWakeup = errors.New("book: пробуждение не найдено")

// ErrWakeHooks — текст пробуждения содержит крючок (CB-01.5: пробуждение
// нейтрально).
var ErrWakeHooks = errors.New("book: пробуждение содержит крючок")

// hookMarkers — маркеры крючка в тексте пробуждения: вопрос игроку или обещание
// события/требование действия. Пробуждение нейтрально и крючков не несёт.
var hookMarkers = []string{
	"?",
	"вдруг",
	"неожиданно",
	"внезапно",
	"должно произойти",
	"скоро случится",
	"тебя ждёт",
	"готовься",
}

// HasHooks сообщает, содержит ли текст пробуждения крючок (CB-01.5).
func HasHooks(text string) bool {
	for _, m := range hookMarkers {
		if strings.Contains(text, m) {
			return true
		}
	}
	return false
}

// WakeConfig — настройки пробуждения (config.yaml, секция wakeup): таймер
// пассивного открытия главы (CB-01.3, по умолчанию 30 с).
type WakeConfig struct {
	PassiveTimeout time.Duration
}

// DefaultWakeConfig — значения по умолчанию: таймер пассивного открытия 30 с.
func DefaultWakeConfig() WakeConfig {
	return WakeConfig{PassiveTimeout: 30 * time.Second}
}

// LoadWakeConfig читает секцию wakeup из файла конфигурации; отсутствующая
// секция или поле оставляют дефолт.
func LoadWakeConfig(path string) (WakeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WakeConfig{}, err
	}
	return parseWakeSection(string(data))
}

// parseWakeSection разбирает топ-ключ `wakeup:` со скалярным полем
// passive_timeout (например, `passive_timeout: 30s`).
func parseWakeSection(data string) (WakeConfig, error) {
	cfg := DefaultWakeConfig()

	inWake := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inWake = trimmed == "wakeup:"
			continue
		}
		if !inWake {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		if strings.TrimSpace(key) == "passive_timeout" {
			if d, err := time.ParseDuration(value); err == nil && d > 0 {
				cfg.PassiveTimeout = d
			}
		}
	}
	if err := sc.Err(); err != nil {
		return WakeConfig{}, err
	}
	return cfg, nil
}

// Clock — минимальный контракт часов пробуждения (testkit.Clock): таймер
// пассивного открытия движется подставными часами, без реального ожидания.
type Clock interface {
	After(d time.Duration) <-chan time.Time
}

// Slot — контракт слота генерации живого запасного пути (testkit.FakeSlot).
type Slot interface {
	Generate(ctx context.Context, role, prompt string) (string, error)
}

// WakeForgePayload — payload заказа wakeup в очереди мастерской.
type WakeForgePayload struct {
	ChapterID int64 `json:"chapter_id"`
}

// EnsureSession возвращает открытую сессию прихода игрока; повторный вход не
// создаёт вторую сессию (CB-01.1): если сессия ещё не закрыта, возвращается она.
func EnsureSession(h *db.Handle, now string) (int64, error) {
	if h == nil {
		return 0, errors.New("book: нет хэндла мира")
	}
	if now == "" {
		return 0, errors.New("book: время прихода не задано")
	}

	var id int64
	err := h.QueryRow(`SELECT id FROM sessions WHERE ended_at IS NULL ORDER BY id DESC LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	res, err := h.Exec("sessions",
		`INSERT INTO sessions (started_at, mode) VALUES (?, 'normal')`, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// OpenChapter открывает главу игрового дня приходом игрока (CB-01.1, CB-01.3):
// сессия — через EnsureSession, глава — привязана к ней. Если глава уже открыта,
// возвращается она — повторный вход не создаёт ни вторую сессию, ни вторую главу.
func OpenChapter(h *db.Handle, day int64, now string) (Chapter, error) {
	if h == nil {
		return Chapter{}, errors.New("book: нет хэндла мира")
	}
	if day <= 0 {
		return Chapter{}, errors.New("book: день главы не задан")
	}

	sessionID, err := EnsureSession(h, now)
	if err != nil {
		return Chapter{}, err
	}

	var id, gotDay int64
	var title, status, kind string
	err = h.QueryRow(`SELECT id, day, title, status, kind FROM book_chapters WHERE status = 'open' ORDER BY day DESC LIMIT 1`).
		Scan(&id, &gotDay, &title, &status, &kind)
	if err == nil {
		return Chapter{ID: id, Day: gotDay, Title: title, Status: ChapterStatus(status), Kind: ChapterKind(kind), SessionID: sessionID}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Chapter{}, err
	}

	title = fmt.Sprintf("Глава %d", day)
	res, err := h.Exec("book_chapters",
		`INSERT INTO book_chapters (day, title, status, kind) VALUES (?, ?, 'open', 'chapter')`, day, title)
	if err != nil {
		return Chapter{}, err
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return Chapter{}, err
	}
	return Chapter{ID: newID, Day: day, Title: title, Status: StatusOpen, Kind: KindChapter, SessionID: sessionID}, nil
}

// PrologueText читает поле «Пролог» из world_meta (первая глава книги
// открывается текстом Пролога, B-02.4; значения world_meta — JSON-строки).
func PrologueText(h *db.Handle) (string, error) {
	if h == nil {
		return "", errors.New("book: нет хэндла мира")
	}
	var raw string
	if err := h.QueryRow(`SELECT value FROM world_meta WHERE "key" = 'prologue'`).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("book: world_meta.prologue не найден")
		}
		return "", err
	}
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return "", fmt.Errorf("book: разбор world_meta.prologue: %w", err)
	}
	return s, nil
}

// EnqueueWakeForge ставит ковку пробуждения главы в очередь мастерской (рой,
// CB-01.2) и заводит запись wakeups со статусом forging. Повторная постановка не
// создаёт вторую запись — chapter_id UNIQUE, запись переводится в forging.
func EnqueueWakeForge(h *db.Handle, chapterID int64) error {
	if h == nil {
		return errors.New("book: нет хэндла мира")
	}
	if chapterID <= 0 {
		return errors.New("book: id главы для пробуждения не задан")
	}

	if _, err := h.Exec("wakeups",
		`INSERT INTO wakeups (chapter_id, text, status, forged_at, delivered_at) VALUES (?, '', 'forging', NULL, NULL)
		 ON CONFLICT(chapter_id) DO UPDATE SET text = excluded.text, status = 'forging', forged_at = NULL, delivered_at = NULL`,
		chapterID); err != nil {
		return err
	}

	err := workshop.Enqueue(h, workshop.KindWakeup, workshop.StepHeavy, workshop.DefaultProvider, WakeForgePayload{ChapterID: chapterID})
	return err
}

// ForgeWakeup принимает текст пробуждения от задачи роя и переводит запись
// forging → ready (CB-01.2). Текст с крючком отклоняется (CB-01.5); повторная
// ковка главы не создаёт вторую запись (chapter_id UNIQUE).
func ForgeWakeup(h *db.Handle, chapterID int64, text, now string) error {
	if h == nil {
		return errors.New("book: нет хэндла мира")
	}
	if chapterID <= 0 {
		return errors.New("book: id главы для пробуждения не задан")
	}
	if now == "" {
		return errors.New("book: время ковки не задано")
	}
	if HasHooks(text) {
		return ErrWakeHooks
	}

	_, err := h.Exec("wakeups",
		`INSERT INTO wakeups (chapter_id, text, status, forged_at, delivered_at) VALUES (?, ?, 'ready', ?, NULL)
		 ON CONFLICT(chapter_id) DO UPDATE SET text = excluded.text, status = 'ready', forged_at = excluded.forged_at, delivered_at = NULL`,
		chapterID, text, now)
	return err
}

// LoadWakeup читает запись пробуждения главы.
func LoadWakeup(h *db.Handle, chapterID int64) (Wakeup, error) {
	if h == nil {
		return Wakeup{}, errors.New("book: нет хэндла мира")
	}
	if chapterID <= 0 {
		return Wakeup{}, errors.New("book: id главы не задан")
	}

	var w Wakeup
	var status string
	var forgedAt, deliveredAt sql.NullString
	err := h.QueryRow(`SELECT id, chapter_id, text, status, forged_at, delivered_at FROM wakeups WHERE chapter_id = ?`, chapterID).
		Scan(&w.ID, &w.ChapterID, &w.Text, &status, &forgedAt, &deliveredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Wakeup{}, ErrNoWakeup
	}
	if err != nil {
		return Wakeup{}, err
	}
	w.Status = WakeStatus(status)
	w.ForgedAt = forgedAt.String
	w.DeliveredAt = deliveredAt.String
	return w, nil
}

// DeliverWakeup подаёт пробуждение при открытии главы (CB-01.3). Игрок пишет
// сразу — пробуждение подаётся мгновенно; игрок пассивен — открытие ждёт таймер
// cfg.PassiveTimeout на часах clock. Готовое пробуждение идёт из базы; непрогретая
// ковка (forging) запускает живой запасной путь через слот slot (CB-01.2).
func DeliverWakeup(h *db.Handle, ch Chapter, cfg WakeConfig, clock Clock, slot Slot, playerReplica string) (string, error) {
	if h == nil {
		return "", errors.New("book: нет хэндла мира")
	}
	if ch.ID <= 0 {
		return "", errors.New("book: id главы не задан")
	}
	if cfg.PassiveTimeout <= 0 {
		cfg.PassiveTimeout = 30 * time.Second
	}

	if playerReplica == "" {
		if clock == nil {
			return "", errors.New("book: нужны часы для пассивного открытия")
		}
		<-clock.After(cfg.PassiveTimeout)
	}

	return serveWakeup(h, ch, slot)
}

// serveWakeup выдаёт текст пробуждения: готовое — из базы, непрогретое — живым
// запасным путём. Подача помечает delivered_at.
func serveWakeup(h *db.Handle, ch Chapter, slot Slot) (string, error) {
	w, err := LoadWakeup(h, ch.ID)

	var text string
	switch {
	case err == nil && w.Status == WakeReady:
		text = w.Text
	case errors.Is(err, ErrNoWakeup) || w.Status == WakeForging:
		if slot == nil {
			return "", errors.New("book: ковка не готова и нет слота запасного пути")
		}
		text, err = slot.Generate(context.Background(), "wakeup", wakePrompt(ch))
		if err != nil {
			return "", err
		}
		if err := ForgeWakeup(h, ch.ID, text, nowUTC()); err != nil {
			return "", err
		}
	default:
		return "", err
	}

	if _, err := h.Exec("wakeups",
		`UPDATE wakeups SET delivered_at = ? WHERE chapter_id = ?`, nowUTC(), ch.ID); err != nil {
		return "", err
	}
	return text, nil
}

// wakePrompt — промпт живого запасного пути: нейтральное пробуждение без крючков.
func wakePrompt(ch Chapter) string {
	return fmt.Sprintf("Нейтральное пробуждение главы %d (день %d): без крючков и вопросов, цель — спровоцировать ответ игрока.", ch.ID, ch.Day)
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
