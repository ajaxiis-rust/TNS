package book

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/lemmarpc"
)

// engineRoot возвращает корень engine/ — каталог с go.mod.
func engineRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("engine root (go.mod) not found")
		}
		dir = parent
	}
}

// sleepLemmatizer — подставной лемматизатор: лемма из карты форм, иначе — слово.
type sleepLemmatizer struct {
	lemmas map[string]string
}

func (s sleepLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error) {
	var toks []lemmarpc.Token
	for _, w := range strings.Fields(text) {
		lemma, ok := s.lemmas[w]
		if !ok {
			lemma = w
		}
		toks = append(toks, lemmarpc.Token{Form: w, Lemma: lemma, Pos: "VERB"})
	}
	return toks, nil
}

func newBookWorld(t *testing.T) *db.Handle {
	t.Helper()
	path := filepath.Join(t.TempDir(), "world(a).db")
	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(w, "world", "shared"); err != nil {
		t.Fatalf("Migrate(world, shared): %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func insertOpenSession(t *testing.T, h *db.Handle) int64 {
	t.Helper()
	res, err := h.Exec("sessions",
		`INSERT INTO sessions (started_at, mode) VALUES ('2026-10-27T08:00:00Z', 'normal')`)
	if err != nil {
		t.Fatalf("INSERT sessions: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("sessions id: %v", err)
	}
	return id
}

func insertOpenChapter(t *testing.T, h *db.Handle, sessionID int64) Chapter {
	t.Helper()
	res, err := h.Exec("book_chapters",
		`INSERT INTO book_chapters (day, title, status, kind) VALUES (1, 'Глава 1', 'open', 'chapter')`)
	if err != nil {
		t.Fatalf("INSERT book_chapters: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("book_chapters id: %v", err)
	}
	return Chapter{ID: id, Day: 1, Title: "Глава 1", Status: StatusOpen, Kind: KindChapter, SessionID: sessionID}
}

func insertJournal(t *testing.T, h *db.Handle, sessionID int64) {
	t.Helper()
	if _, err := h.Exec("turns",
		`INSERT INTO turns (session_id, player_text, reply_text, saw_recommendation, trust_level, latency_ms, degraded) VALUES (?, ?, ?, 0, 1, 10, 0)`,
		sessionID, "Мы легли спать.", "Доброй ночи."); err != nil {
		t.Fatalf("INSERT turns: %v", err)
	}
	if _, err := h.Exec("scenes",
		`INSERT INTO scenes (session_id, day, status, prose) VALUES (?, 1, 'closed', 'Ночь опустилась на город.')`,
		sessionID); err != nil {
		t.Fatalf("INSERT scenes: %v", err)
	}
}

// TestChapterDay — «Проверка» T-07-01: реплика сна (по леммам лексикона)
// закрывает главу и сессию; без действия игрока новая сессия не открывается;
// текст главы совпадает с журналом сессии без правок; после закрытия в
// workshop_queue стоит задача редактуры kind='edit' на главу.
func TestChapterDay(t *testing.T) {
	lex, err := LoadLexicon(filepath.Join(engineRoot(t), "packs", "ru"))
	if err != nil {
		t.Fatalf("LoadLexicon(packs/ru): %v", err)
	}
	if got := lex.SleepLemmas(); !slices.Equal(got, []string{"спать", "лечь", "постель", "отходить"}) {
		t.Fatalf("леммы намерения «сон» = %v, want [спать лечь постель отходить]", got)
	}

	lem := sleepLemmatizer{lemmas: map[string]string{
		"легли": "лечь",
		"спать": "спать",
		"идём":  "идти",
	}}

	// Не-соновая реплика намерение «сон» не даёт.
	if ok, err := DetectSleep(lem, lex, "Мы идём в город"); err != nil || ok {
		t.Fatalf("DetectSleep(движение) = (%v, %v), want (false, nil)", ok, err)
	}

	world := newBookWorld(t)
	sessionID := insertOpenSession(t, world)
	ch := insertOpenChapter(t, world, sessionID)
	insertJournal(t, world, sessionID)

	// Реплика сна распознаётся по леммам лексикона.
	if ok, err := DetectSleep(lem, lex, "Мы легли спать"); err != nil || !ok {
		t.Fatalf("DetectSleep(сон) = (%v, %v), want (true, nil)", ok, err)
	}

	now := "2026-10-27T22:00:00Z"
	if err := ch.Close(world, now); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Глава закрыта.
	var status string
	if err := world.QueryRow(`SELECT status FROM book_chapters WHERE id = ?`, ch.ID).Scan(&status); err != nil {
		t.Fatalf("SELECT book_chapters.status: %v", err)
	}
	if status != string(StatusClosed) {
		t.Fatalf("status главы = %q, want closed", status)
	}

	// Сессия закрыта.
	var ended sql.NullString
	if err := world.QueryRow(`SELECT ended_at FROM sessions WHERE id = ?`, sessionID).Scan(&ended); err != nil {
		t.Fatalf("SELECT sessions.ended_at: %v", err)
	}
	if !ended.Valid || ended.String != now {
		t.Fatalf("ended_at = %v, want %q", ended, now)
	}

	// Без действия игрока новая сессия не открывается.
	var sessions int
	if err := world.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatalf("SELECT COUNT(sessions): %v", err)
	}
	if sessions != 1 {
		t.Fatalf("сессий после закрытия = %d, want 1 (новая не создана)", sessions)
	}

	// Текст главы — журнал сессии без правок.
	text, err := ch.Text(world)
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	wantText := "Мы легли спать.\nДоброй ночи.\nНочь опустилась на город."
	if text != wantText {
		t.Fatalf("текст главы = %q, want %q", text, wantText)
	}

	// После закрытия в очереди стоит задача редактуры kind='edit' на главу.
	var kind, rawPayload string
	if err := world.QueryRow(`SELECT kind, payload_json FROM workshop_queue`).Scan(&kind, &rawPayload); err != nil {
		t.Fatalf("SELECT workshop_queue: %v", err)
	}
	if kind != "edit" {
		t.Fatalf("kind заказа = %q, want edit", kind)
	}
	var payload struct {
		ChapterID int64 `json:"chapter_id"`
	}
	if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
		t.Fatalf("payload заказа: %v", err)
	}
	if payload.ChapterID != ch.ID {
		t.Fatalf("payload.ChapterID = %d, want %d", payload.ChapterID, ch.ID)
	}
}
