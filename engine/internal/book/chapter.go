package book

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"tns65/internal/db"
	"tns65/internal/lemmarpc"
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

// SleepLemmas возвращает леммы намерения «сон»; пусто, если намерения нет.
func (l *Lexicon) SleepLemmas() []string {
	if l == nil {
		return nil
	}
	for _, e := range l.Intents {
		if e.Intent == SleepIntent {
			return e.Lemmas
		}
	}
	return nil
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

// ErrChapterNotOpen — закрываемая глава не в статусе open.
var ErrChapterNotOpen = errors.New("book: глава не открыта")

// ErrSessionNotOpen — закрываемая сессия уже закрыта.
var ErrSessionNotOpen = errors.New("book: сессия не открыта")

// Close закрывает главу сном (шаги 2–4): переводит главу в closed, закрывает её
// сессию (ended_at=now) и ставит задачу редактуры edit на главу (T-05-09).
// Следующую сессию ядро не создаёт — она откроется только приходом игрока (шаг 5).
func (c Chapter) Close(h *db.Handle, now string) error {
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

	if err := h.Tx(db.WriterCore, func(tx *db.Tx) error {
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
	}); err != nil {
		return err
	}

	// Шаг 4: по закрытию главы — задача редактуры на главу (T-05-09).
	_, err := workshop.EnqueueEditorTask(h, c.ID)
	return err
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
