package book

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tns65/internal/db"
	"tns65/internal/lemmarpc"
	"tns65/internal/prompts"
	"tns65/internal/workshop"
)

// ChapterStatus — статус главы (CHECK из T-01-06).
type ChapterStatus string

const (
	StatusOpen       ChapterStatus = "open"
	StatusClosed     ChapterStatus = "closed"
	StatusProcessing ChapterStatus = "processing"
	StatusPublished  ChapterStatus = "published"
)

// ChapterKind — вид главы (CHECK из T-01-06).
type ChapterKind string

const (
	KindChapter  ChapterKind = "chapter"
	KindEpilogue ChapterKind = "epilogue"
)

// Chapter — глава книги (T-07-01, шаг 1): игровой день как рамка от пробуждения
// до сна. Глава — журнал сессии (ходы, реплики, журнал) без правок, отдельно не
// генерируется. Привязка к сессии — SessionID: колонки в book_chapters нет, связь
// держит ядро, открывающее сессию и главу вместе.
type Chapter struct {
	ID        int64
	Day       int64
	Title     string
	Status    ChapterStatus
	Kind      ChapterKind
	SessionID int64
}

// Session — сессия, к которой привязана глава (sessions).
type Session struct {
	ID        int64
	StartedAt string
	EndedAt   string
	Mode      string
}

// IntentEntry — строка лексикона намерений: имя намерения, леммы и глаголы.
type IntentEntry struct {
	Intent string   `json:"intent"`
	Lemmas []string `json:"lemmas"`
	Verbs  []string `json:"verbs"`
}

// Lexicon — лексикон намерений языкового пакета (lexicon_intents.jsonl).
type Lexicon struct {
	Intents []IntentEntry
}

// SleepIntent — имя намерения «сон» в лексиконе намерений.
const SleepIntent = "сон"

// UnconsciousIntent — имя намерения «обморок» в лексиконе намерений: потеря
// сознания и «отключился» закрывают главу тем же путём, что и сон (CB-03.4).
const UnconsciousIntent = "обморок"

// CloseIntents — намерения, закрывающие главу явной репликой игрока.
var CloseIntents = []string{SleepIntent, UnconsciousIntent}

// LoadLexicon читает лексикон намерений из каталога языкового пакета dir.
func LoadLexicon(dir string) (*Lexicon, error) {
	b, err := os.ReadFile(filepath.Join(dir, "lexicon_intents.jsonl"))
	if err != nil {
		return nil, err
	}
	var lex Lexicon
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e IntentEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, err
		}
		lex.Intents = append(lex.Intents, e)
	}
	return &lex, nil
}

// IntentLemmas возвращает леммы намерения intent; пусто, если намерения нет.
func (l *Lexicon) IntentLemmas(intent string) []string {
	if l == nil {
		return nil
	}
	for _, e := range l.Intents {
		if e.Intent == intent {
			return e.Lemmas
		}
	}
	return nil
}

// SleepLemmas возвращает леммы намерения «сон»; пусто, если намерения нет.
func (l *Lexicon) SleepLemmas() []string {
	return l.IntentLemmas(SleepIntent)
}

// IsSleep распознаёт намерение «сон» по леммам реплики без LLM (шаг 3): хотя бы
// одна лемма реплики совпадает с леммой намерения «сон» из лексикона.
func IsSleep(lemmas []string, lex *Lexicon) bool {
	sleep := lex.SleepLemmas()
	for _, needle := range sleep {
		for _, l := range lemmas {
			if l == needle {
				return true
			}
		}
	}
	return false
}

// CloseIntentOf возвращает намерение закрытия главы, найденное в леммах реплики
// (CB-03.4): сон, потеря сознания и «отключился» ведут общим путём Close.
func CloseIntentOf(lemmas []string, lex *Lexicon) (string, bool) {
	for _, intent := range CloseIntents {
		for _, needle := range lex.IntentLemmas(intent) {
			for _, l := range lemmas {
				if l == needle {
					return intent, true
				}
			}
		}
	}
	return "", false
}

// Lemmatizer — минимальный контракт клиента лемматизации (T-01-09): разбор
// реплики в токены; в тестах подменяется.
type Lemmatizer interface {
	Lemmatize(text string) ([]lemmarpc.Token, error)
}

// DetectSleep лемматизирует реплику и распознаёт намерение «сон» кодом.
func DetectSleep(lem Lemmatizer, lex *Lexicon, text string) (bool, error) {
	if lem == nil {
		return false, errors.New("book: нет лемматизатора для распознавания сна")
	}
	toks, err := lem.Lemmatize(text)
	if err != nil {
		return false, err
	}
	lemmas := make([]string, 0, len(toks))
	for _, t := range toks {
		lemmas = append(lemmas, t.Lemma)
	}
	return IsSleep(lemmas, lex), nil
}

// DetectClose лемматизирует реплику и распознаёт намерение закрытия главы
// (CB-03.4): сон, потеря сознания и «отключился» ведут общим путём Close.
func DetectClose(lem Lemmatizer, lex *Lexicon, text string) (string, bool, error) {
	if lem == nil {
		return "", false, errors.New("book: нет лемматизатора для распознавания финала главы")
	}
	toks, err := lem.Lemmatize(text)
	if err != nil {
		return "", false, err
	}
	lemmas := make([]string, 0, len(toks))
	for _, t := range toks {
		lemmas = append(lemmas, t.Lemma)
	}
	intent, ok := CloseIntentOf(lemmas, lex)
	return intent, ok, nil
}

// ErrChapterNotOpen — закрываемая глава не в статусе open.
var ErrChapterNotOpen = errors.New("book: глава не открыта")

// ErrSessionNotOpen — закрываемая сессия уже закрыта.
var ErrSessionNotOpen = errors.New("book: сессия не открыта")

// Close закрывает главу сном (шаги 2–4): переводит главу в closed, закрывает её
// сессию (ended_at=now) и ставит задачу редактуры edit на главу (T-05-09).
// Следующую сессию ядро не создаёт — она откроется только приходом игрока (шаг 5).
func (c Chapter) Close(h *db.Handle, now string) error {
	if err := closeChapter(h, c, now); err != nil {
		return err
	}

	// Шаг 4: по закрытию главы — задача редактуры на главу (T-05-09).
	_, err := workshop.EnqueueEditorTask(h, c.ID)
	return err
}

// closeChapter переводит главу в closed и закрывает её сессию одной транзакцией
// писателя core (шаги 2–3); задачу редактуры ставит вызывающий.
func closeChapter(h *db.Handle, c Chapter, now string) error {
	if h == nil {
		return errors.New("book: нет хэндла мира")
	}
	if c.ID <= 0 {
		return errors.New("book: id главы не задан")
	}
	if c.SessionID <= 0 {
		return errors.New("book: id сессии главы не задан")
	}
	if now == "" {
		return errors.New("book: время закрытия не задано")
	}

	return h.Tx(db.WriterCore, func(tx *db.Tx) error {
		res, err := tx.Exec("book_chapters",
			`UPDATE book_chapters SET status = ? WHERE id = ? AND status = ?`,
			string(StatusClosed), c.ID, string(StatusOpen))
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return ErrChapterNotOpen
		}

		res, err = tx.Exec("sessions",
			`UPDATE sessions SET ended_at = ? WHERE id = ? AND ended_at IS NULL`,
			now, c.SessionID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return ErrSessionNotOpen
		}
		return nil
	})
}

// CloseReason — причина закрытия главы (CB-03): явная реплика игрока или
// автозакрытие по лимиту открытой главы.
type CloseReason string

const (
	CloseByPlayer CloseReason = "player"
	CloseByLimit  CloseReason = "autoclose"
)

// AutoCloseAfter — предел открытой главы (CB-03.5): более 20 часов от started_at
// сессии — глава закрывается автоматически.
const AutoCloseAfter = 20 * time.Hour

// AutoCloseEventKind — событие-пометка автозакрытия главы в events.
const AutoCloseEventKind = "chapter_autoclose"

// WorldNotesRole — роль заметок о жизни мира: слот Редактора, пишущий прозу
// книги (существующая роль мастерской).
const WorldNotesRole = workshop.EditorRole

// AutoClose закрывает открытую главу, если с начала её сессии прошло больше
// 20 часов (CB-03.5): молчание главу не закрывает, лимит открытия закрывает.
// Сессия главы — последняя открытая (колонки сессии в book_chapters нет).
// Заметки о жизни мира вписываются сценой главы (CB-03.6): слот необязателен,
// при его недоступности заметки собираются кодом из событий простоя. Ошибки
// заметок автозакрытие не отменяют — ход не валится. Возвращает главу, признак
// закрытия и ошибку; отсутствие открытой главы — (Chapter{}, false, nil).
func AutoClose(h *db.Handle, now string, slot Slot) (Chapter, bool, error) {
	if h == nil {
		return Chapter{}, false, errors.New("book: нет хэндла мира")
	}
	closedAt, err := time.Parse(time.RFC3339, now)
	if err != nil {
		return Chapter{}, false, fmt.Errorf("book: время автозакрытия не разобрано: %w", err)
	}

	ch, startedAt, ok, err := openChapterForClose(h)
	if err != nil {
		return Chapter{}, false, err
	}
	if !ok {
		return Chapter{}, false, nil
	}

	openedAt, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return Chapter{}, false, nil
	}
	elapsed := closedAt.Sub(openedAt)
	if elapsed <= AutoCloseAfter {
		return ch, false, nil
	}

	notes := worldNotes(h, ch, startedAt, now, slot)
	if err := closeChapter(h, ch, now); err != nil {
		if errors.Is(err, ErrChapterNotOpen) || errors.Is(err, ErrSessionNotOpen) {
			return Chapter{}, false, nil
		}
		return Chapter{}, false, err
	}
	if notes != "" {
		_ = writeWorldNotesScene(h, ch, notes)
	}
	_ = markAutoClose(h, ch, startedAt, now, elapsed)
	_, _ = workshop.EnqueueEditorTask(h, ch.ID)
	return ch, true, nil
}

// openChapterForClose читает открытую главу и started_at сессии, к которой она
// привязана: открытую главу держит последняя открытая сессия — её закрывает тот
// же closeChapter.
func openChapterForClose(h *db.Handle) (Chapter, string, bool, error) {
	var ch Chapter
	var status, kind, startedAt string
	err := h.QueryRow(
		`SELECT c.id, c.day, c.title, c.status, c.kind, s.id, s.started_at
		 FROM book_chapters c, sessions s
		 WHERE c.status = ? AND s.ended_at IS NULL
		 ORDER BY c.id DESC, s.id DESC LIMIT 1`,
		string(StatusOpen)).Scan(&ch.ID, &ch.Day, &ch.Title, &status, &kind, &ch.SessionID, &startedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Chapter{}, "", false, nil
	}
	if err != nil {
		return Chapter{}, "", false, err
	}
	ch.Status = ChapterStatus(status)
	ch.Kind = ChapterKind(kind)
	return ch, startedAt, true, nil
}

// markAutoClose пишет событие-пометку автозакрытия с причиной и временем
// открытия главы.
func markAutoClose(h *db.Handle, ch Chapter, startedAt, closedAt string, elapsed time.Duration) error {
	payload, err := json.Marshal(map[string]any{
		"chapter_id": ch.ID,
		"session_id": ch.SessionID,
		"started_at": startedAt,
		"closed_at":  closedAt,
		"hours_open": int64(elapsed / time.Hour),
		"reason":     string(CloseByLimit),
	})
	if err != nil {
		return err
	}
	_, err = h.Exec("events",
		`INSERT INTO events (day, kind, payload_json, source) VALUES (?, ?, ?, ?)`,
		ch.Day, AutoCloseEventKind, string(payload), "book")
	return err
}

// writeWorldNotesScene вписывает заметки о жизни мира сценой сессии главы: текст
// главы собирается из журнала сессии, поэтому заметки видны в книге.
func writeWorldNotesScene(h *db.Handle, ch Chapter, notes string) error {
	_, err := h.Exec("scenes",
		`INSERT INTO scenes (session_id, day, status, prose) VALUES (?, ?, ?, ?)`,
		ch.SessionID, ch.Day, "closed", notes)
	return err
}

// worldNotes собирает заметки о жизни мира для автозакрытой главы (CB-03.6):
// сначала слот Редактора по фактам простоя, при пустом или неудачном ответе —
// детерминированная сборка из событий. Пробуждение T-07-02 не переиспользуется:
// это заранее выкованный текст утра, а не заметки о прошедшем времени. Факты
// простоя всегда собираются кодом: слот получает их готовыми и не считает сам.
func worldNotes(h *db.Handle, ch Chapter, startedAt, now string, slot Slot) string {
	facts := worldFacts(h, startedAt)
	var b strings.Builder
	b.WriteString("Заметки о жизни мира.\n")
	b.WriteString("Пока игрока не было, мир жил своей жизнью.\n")
	for _, f := range facts {
		b.WriteString(f)
		b.WriteString("\n")
	}
	fallback := strings.TrimSpace(b.String())

	if slot == nil {
		return fallback
	}
	raw, err := slot.Generate(context.Background(), WorldNotesRole, worldNotesPrompt(ch, startedAt, now, facts))
	if err != nil {
		return fallback
	}
	if text := strings.TrimSpace(raw); text != "" {
		return text
	}
	return fallback
}

// worldFacts собирает факты простоя из существующих источников: сработавшие
// отложенные вести, векторы судьбы дальнего круга и вести возврата.
func worldFacts(h *db.Handle, since string) []string {
	var facts []string

	rows, err := h.Query(
		`SELECT kind, created_at FROM deferred_events WHERE status = ? AND created_at >= ? ORDER BY created_at, id`,
		"fired", since)
	if err == nil {
		for rows.Next() {
			var kind, createdAt string
			if rows.Scan(&kind, &createdAt) == nil {
				facts = append(facts, fmt.Sprintf("Отложенная весть «%s» дошла (%s).", kind, createdAt))
			}
		}
		rows.Close()
	}

	rows, err = h.Query(`SELECT payload_json FROM events WHERE kind = ? ORDER BY id DESC LIMIT 5`, "fate_vector")
	if err == nil {
		var raws []string
		for rows.Next() {
			var raw string
			if rows.Scan(&raw) == nil {
				raws = append(raws, raw)
			}
		}
		rows.Close()
		// Имена лиц читаются после закрытия курсора: в базе мира одно соединение.
		for _, raw := range raws {
			if line := fateFact(h, raw); line != "" {
				facts = append(facts, line)
			}
		}
	}

	var raw string
	if err := h.QueryRow(`SELECT payload_json FROM events WHERE kind = ? ORDER BY id DESC LIMIT 1`, "news").Scan(&raw); err == nil {
		var payload struct {
			Texts []string `json:"texts"`
			Text  string   `json:"text"`
		}
		if json.Unmarshal([]byte(raw), &payload) == nil {
			texts := payload.Texts
			if len(texts) == 0 && payload.Text != "" {
				texts = []string{payload.Text}
			}
			if len(texts) > 0 {
				facts = append(facts, "Вести возврата: "+strings.Join(texts, " "))
			}
		}
	}

	return facts
}

// fateFact переводит коммит вектора судьбы в строку заметок: имя лица из пачки
// мира, положение и домены жизни (достаток, здоровье, связи).
func fateFact(h *db.Handle, raw string) string {
	var commit struct {
		Fate struct {
			CharacterID int64   `json:"character_id"`
			Position    string  `json:"position"`
			Wealth      int64   `json:"wealth"`
			Health      int64   `json:"health"`
			Connections []int64 `json:"connections"`
		} `json:"fate"`
	}
	if err := json.Unmarshal([]byte(raw), &commit); err != nil {
		return ""
	}
	if commit.Fate.CharacterID <= 0 {
		return ""
	}
	name := fmt.Sprintf("лицо #%d", commit.Fate.CharacterID)
	var got string
	if err := h.QueryRow(`SELECT name FROM characters WHERE id = ?`, commit.Fate.CharacterID).Scan(&got); err == nil && strings.TrimSpace(got) != "" {
		name = got
	}
	position := commit.Fate.Position
	if position == "" {
		position = "на месте"
	}
	return fmt.Sprintf("Судьба дальнего круга: %s — %s, достаток %d, здоровье %d, связей %d.",
		name, position, commit.Fate.Wealth, commit.Fate.Health, len(commit.Fate.Connections))
}

// defaultWorldNotesPrompt — дефолт промпта заметок о жизни мира (без файла):
// только факты простоя из журнала мира, без вопросов игроку и обещаний
// будущих событий.
const defaultWorldNotesPrompt = "Глава {day} закрыта автоматически: открыта {started_at}, закрыта {now}. Напиши 3–5 предложений заметок о жизни мира за это время: что случилось без участия игрока, как сложились судьбы NPC. Без вопросов игроку и без обещаний будущих событий. Факты журнала мира:\n{facts}"

// worldNotesPrompt — запрос заметок к слоту: только факты простоя из журнала
// мира, без вопросов игроку и обещаний будущих событий.
func worldNotesPrompt(ch Chapter, startedAt, now string, facts []string) string {
	data := "фактов простоя нет"
	if len(facts) > 0 {
		data = strings.Join(facts, "\n")
	}
	return prompts.Render(WorldNotesRole, "book.world_notes", defaultWorldNotesPrompt, map[string]any{
		"day":        ch.Day,
		"started_at": startedAt,
		"now":        now,
		"facts":      data,
	})
}

// Text собирает текст главы из журнала сессии без правок (шаг 1): ходы
// (player_text, reply_text) и сцены (prose) сессии в порядке id, части разделены
// переводом строки.
func (c Chapter) Text(h *db.Handle) (string, error) {
	if h == nil {
		return "", errors.New("book: нет хэндла мира")
	}
	if c.SessionID <= 0 {
		return "", errors.New("book: id сессии главы не задан")
	}

	var parts []string

	rows, err := h.Query(`SELECT player_text, reply_text FROM turns WHERE session_id = ? ORDER BY id`, c.SessionID)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var player, reply string
		if err := rows.Scan(&player, &reply); err != nil {
			rows.Close()
			return "", err
		}
		parts = append(parts, player, reply)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	rows, err = h.Query(`SELECT prose FROM scenes WHERE session_id = ? ORDER BY id`, c.SessionID)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var prose string
		if err := rows.Scan(&prose); err != nil {
			rows.Close()
			return "", err
		}
		parts = append(parts, prose)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	return strings.Join(parts, "\n"), nil
}
