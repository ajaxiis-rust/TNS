package book

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"tns65/internal/db"
	"tns65/internal/lemmarpc"
	"tns65/internal/workshop"
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
	// Слаг мира — имя каталога ready/ при экспорте публикации (IMP-6).
	if _, err := w.Exec(`INSERT INTO world_meta ("key", value) VALUES ('slug', '"a"')`); err != nil {
		t.Fatalf("INSERT world_meta slug: %v", err)
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

// notesSlot — подставной слот заметок о жизни мира.
type notesSlot struct {
	text string
	err  error
	role string
}

func (s *notesSlot) Generate(_ context.Context, role, _ string) (string, error) {
	s.role = role
	if s.err != nil {
		return "", s.err
	}
	return s.text, nil
}

// fakePauseClock — подставные часы таймера паузы мастерской.
type fakePauseClock struct{ now time.Time }

func (c *fakePauseClock) Now() time.Time { return c.now }

// TestCloseIntents — CB-03.4: главу закрывает не только сон, но и «потерял
// сознание»/«отключился»; путь закрытия общий.
func TestCloseIntents(t *testing.T) {
	lex, err := LoadLexicon(filepath.Join(engineRoot(t), "packs", "ru"))
	if err != nil {
		t.Fatalf("LoadLexicon(packs/ru): %v", err)
	}
	if got := lex.IntentLemmas(UnconsciousIntent); !slices.Equal(got, []string{"сознание", "отключиться"}) {
		t.Fatalf("леммы намерения «обморок» = %v, want [сознание отключиться]", got)
	}

	lem := sleepLemmatizer{lemmas: map[string]string{
		"потерял":    "потерять",
		"теряю":      "терять",
		"сознание":   "сознание",
		"отключился": "отключиться",
		"отключаюсь": "отключиться",
		"легли":      "лечь",
		"спать":      "спать",
		"идём":       "идти",
	}}

	cases := []struct {
		text   string
		intent string
		ok     bool
	}{
		{"Я потерял сознание", UnconsciousIntent, true},
		{"Я теряю сознание", UnconsciousIntent, true},
		{"Я отключился", UnconsciousIntent, true},
		{"Я отключаюсь", UnconsciousIntent, true},
		{"Мы легли спать", SleepIntent, true},
		{"Мы идём в город", "", false},
	}
	for _, c := range cases {
		intent, ok, err := DetectClose(lem, lex, c.text)
		if err != nil {
			t.Fatalf("DetectClose(%q): %v", c.text, err)
		}
		if intent != c.intent || ok != c.ok {
			t.Fatalf("DetectClose(%q) = (%q, %v), want (%q, %v)", c.text, intent, ok, c.intent, c.ok)
		}
	}

	// Распознанный финал закрывает главу тем же путём Close.
	world := newBookWorld(t)
	sessionID := insertOpenSession(t, world)
	ch := insertOpenChapter(t, world, sessionID)
	if _, ok, err := DetectClose(lem, lex, "Я отключился"); err != nil || !ok {
		t.Fatalf("DetectClose(отключился) = (_, %v), want ok", err)
	}
	if err := ch.Close(world, "2026-10-27T22:00:00Z"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	var status string
	if err := world.QueryRow(`SELECT status FROM book_chapters WHERE id = ?`, ch.ID).Scan(&status); err != nil {
		t.Fatalf("SELECT book_chapters.status: %v", err)
	}
	if status != string(StatusClosed) {
		t.Fatalf("status главы = %q, want closed", status)
	}
}

// seedWorldLife кладёт жизнь мира за простой: отложенную весть, вектор судьбы
// дальнего круга и вести возврата.
func seedWorldLife(t *testing.T, h *db.Handle, since string) {
	t.Helper()
	if _, err := h.Exec("characters",
		`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES (7, 'Купец', 'торговец', 'купец', '{}')`); err != nil {
		t.Fatalf("INSERT characters: %v", err)
	}
	if _, err := h.Exec("deferred_events",
		`INSERT INTO deferred_events (kind, payload_json, due_day, status, created_at) VALUES ('corridor_stop', '{}', 900, 'fired', ?)`,
		since); err != nil {
		t.Fatalf("INSERT deferred_events: %v", err)
	}
	fate, err := json.Marshal(map[string]any{
		"offline": true,
		"fate": map[string]any{
			"character_id": 7,
			"position":     "в пути",
			"wealth":       42,
			"health":       88,
			"connections":  []int64{8, 9},
		},
	})
	if err != nil {
		t.Fatalf("marshal fate: %v", err)
	}
	if _, err := h.Exec("events",
		`INSERT INTO events (day, kind, payload_json, source) VALUES (1, 'fate_vector', ?, 'core')`, string(fate)); err != nil {
		t.Fatalf("INSERT events fate: %v", err)
	}
	news, err := json.Marshal(map[string][]string{"texts": {"Весть: дальний круг мира продвинулся (2)."}})
	if err != nil {
		t.Fatalf("marshal news: %v", err)
	}
	if _, err := h.Exec("events",
		`INSERT INTO events (day, kind, payload_json, source) VALUES (1, 'news', ?, 'core')`, string(news)); err != nil {
		t.Fatalf("INSERT events news: %v", err)
	}
}

// TestAutoClose — CB-03.5–6: глава, открытая более 20 часов, закрывается
// автозакрытием с пометкой и заметками о жизни мира; после автозакрытия ночная
// ступень паузы наступает.
func TestAutoClose(t *testing.T) {
	world := newBookWorld(t)
	sessionID := insertOpenSession(t, world)
	ch := insertOpenChapter(t, world, sessionID)
	seedWorldLife(t, world, "2026-10-27T08:00:00Z")

	now := "2026-10-28T05:00:00Z" // 21 ч от started_at — больше лимита 20 ч.
	got, closed, err := AutoClose(world, now, nil)
	if err != nil {
		t.Fatalf("AutoClose: %v", err)
	}
	if !closed || got.ID != ch.ID {
		t.Fatalf("AutoClose = (%d, %v), want (%d, true)", got.ID, closed, ch.ID)
	}

	// Глава закрыта, сессия закрыта.
	var status string
	if err := world.QueryRow(`SELECT status FROM book_chapters WHERE id = ?`, ch.ID).Scan(&status); err != nil {
		t.Fatalf("SELECT book_chapters.status: %v", err)
	}
	if status != string(StatusClosed) {
		t.Fatalf("status главы = %q, want closed", status)
	}
	var ended sql.NullString
	if err := world.QueryRow(`SELECT ended_at FROM sessions WHERE id = ?`, sessionID).Scan(&ended); err != nil {
		t.Fatalf("SELECT sessions.ended_at: %v", err)
	}
	if !ended.Valid || ended.String != now {
		t.Fatalf("ended_at = %v, want %q", ended, now)
	}

	// Заметки о жизни мира вписаны в главу и видны в её тексте.
	var notes string
	if err := world.QueryRow(`SELECT prose FROM scenes WHERE session_id = ? ORDER BY id DESC LIMIT 1`, sessionID).Scan(&notes); err != nil {
		t.Fatalf("SELECT scenes.prose: %v", err)
	}
	for _, want := range []string{
		"Заметки о жизни мира.",
		"Отложенная весть «corridor_stop» дошла",
		"Судьба дальнего круга: Купец — в пути, достаток 42, здоровье 88, связей 2.",
		"Вести возврата: Весть: дальний круг мира продвинулся (2).",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("заметки не содержат %q:\n%s", want, notes)
		}
	}
	text, err := got.Text(world)
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	if !strings.Contains(text, "Заметки о жизни мира.") {
		t.Fatalf("текст главы без заметок о жизни мира: %q", text)
	}

	// Пометка автозакрытия в событиях мира.
	var rawPayload string
	if err := world.QueryRow(`SELECT payload_json FROM events WHERE kind = ?`, AutoCloseEventKind).Scan(&rawPayload); err != nil {
		t.Fatalf("SELECT events.%s: %v", AutoCloseEventKind, err)
	}
	var mark struct {
		ChapterID int64  `json:"chapter_id"`
		Reason    string `json:"reason"`
		HoursOpen int64  `json:"hours_open"`
	}
	if err := json.Unmarshal([]byte(rawPayload), &mark); err != nil {
		t.Fatalf("payload пометки: %v", err)
	}
	if mark.ChapterID != ch.ID || mark.Reason != string(CloseByLimit) || mark.HoursOpen != 21 {
		t.Fatalf("пометка автозакрытия = %+v, want chapter=%d reason=%s hours=21", mark, ch.ID, CloseByLimit)
	}

	// Ночная ступень паузы после автозакрытия наступает (W-01.5): молчание
	// больше 8 часов при закрытой главе — ночь.
	clock := &fakePauseClock{now: time.Date(2026, 10, 28, 6, 0, 0, 0, time.UTC)}
	sched := workshop.NewScheduler(world, clock, nil, nil)
	sched.MarkActive()
	clock.now = clock.now.Add(9 * time.Hour)
	sched.SetChapterClosed(true)
	if step := sched.Step(); step != workshop.PauseNight {
		t.Fatalf("ступень паузы после автозакрытия = %q, want night", step)
	}

	// Повторное автозакрытие — не ошибка и не второй проход.
	if _, second, err := AutoClose(world, "2026-10-29T05:00:00Z", nil); err != nil || second {
		t.Fatalf("повторный AutoClose = (_, %v, %v), want (false, nil)", second, err)
	}
	var marks int
	if err := world.QueryRow(`SELECT COUNT(*) FROM events WHERE kind = ?`, AutoCloseEventKind).Scan(&marks); err != nil {
		t.Fatalf("SELECT COUNT(events): %v", err)
	}
	if marks != 1 {
		t.Fatalf("пометок автозакрытия = %d, want 1", marks)
	}
}

// TestAutoCloseBeforeLimit — молчание само главу не закрывает: до 20 часов
// глава остаётся открытой.
func TestAutoCloseBeforeLimit(t *testing.T) {
	world := newBookWorld(t)
	sessionID := insertOpenSession(t, world)
	ch := insertOpenChapter(t, world, sessionID)

	got, closed, err := AutoClose(world, "2026-10-28T03:00:00Z", nil) // 19 ч
	if err != nil {
		t.Fatalf("AutoClose: %v", err)
	}
	if closed {
		t.Fatalf("AutoClose(19 ч) закрыл главу %d, want открытой", got.ID)
	}
	var status string
	if err := world.QueryRow(`SELECT status FROM book_chapters WHERE id = ?`, ch.ID).Scan(&status); err != nil {
		t.Fatalf("SELECT book_chapters.status: %v", err)
	}
	if status != string(StatusOpen) {
		t.Fatalf("status главы = %q, want open", status)
	}
	var notes int
	if err := world.QueryRow(`SELECT COUNT(*) FROM scenes`).Scan(&notes); err != nil {
		t.Fatalf("SELECT COUNT(scenes): %v", err)
	}
	if notes != 0 {
		t.Fatalf("сцен до лимита = %d, want 0", notes)
	}
}

// TestAutoCloseWorldNotesSlot — заметки пишет существующий слот Редактора; при
// его недоступности — детерминированная сборка из событий.
func TestAutoCloseWorldNotesSlot(t *testing.T) {
	world := newBookWorld(t)
	sessionID := insertOpenSession(t, world)
	insertOpenChapter(t, world, sessionID)

	slot := &notesSlot{text: "Город был взят без боя: жители сами распахнули ворота."}
	if _, closed, err := AutoClose(world, "2026-10-28T05:00:00Z", slot); err != nil || !closed {
		t.Fatalf("AutoClose(слот) = (_, %v, %v), want closed", closed, err)
	}
	if slot.role != WorldNotesRole {
		t.Fatalf("роль слота заметок = %q, want %q", slot.role, WorldNotesRole)
	}
	var notes string
	if err := world.QueryRow(`SELECT prose FROM scenes WHERE session_id = ? ORDER BY id DESC LIMIT 1`, sessionID).Scan(&notes); err != nil {
		t.Fatalf("SELECT scenes.prose: %v", err)
	}
	if notes != slot.text {
		t.Fatalf("заметки слота = %q, want %q", notes, slot.text)
	}

	// Отказ слота не мешает автозакрытию: заметки собирает код.
	failWorld := newBookWorld(t)
	failSession := insertOpenSession(t, failWorld)
	insertOpenChapter(t, failWorld, failSession)
	seedWorldLife(t, failWorld, "2026-10-27T08:00:00Z")
	failing := &notesSlot{err: errors.New("слот недоступен")}
	if _, closed, err := AutoClose(failWorld, "2026-10-28T05:00:00Z", failing); err != nil || !closed {
		t.Fatalf("AutoClose(отказ слота) = (_, %v, %v), want closed", closed, err)
	}
	if err := failWorld.QueryRow(`SELECT prose FROM scenes WHERE session_id = ? ORDER BY id DESC LIMIT 1`, failSession).Scan(&notes); err != nil {
		t.Fatalf("SELECT scenes.prose: %v", err)
	}
	if !strings.Contains(notes, "Заметки о жизни мира.") || !strings.Contains(notes, "Купец") {
		t.Fatalf("заметки при отказе слота = %q, want сборку из событий", notes)
	}
}
