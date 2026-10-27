package book

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"

	"tns65/internal/db"
	"tns65/internal/workshop"
)

// PlayerEdit — правка игрока «было → стало» (T-07-05, шаг 2). Правка касается
// только орфографии и пунктуации; содержание вручную не правится никогда.
type PlayerEdit struct {
	Before string
	After  string
}

// EditResult — итог правок игрока над главой: одобренные и применённые правки
// и отклонённые Цензором правок.
type EditResult struct {
	Applied  []PlayerEdit
	Rejected []PlayerEdit
}

// rejectedCut — отклонённая правка для записи в book_cuts.
type rejectedCut struct {
	edit             PlayerEdit
	reason           string
	lexiconCandidate string // слово-кандидат лексикона мира (CB-06.9)
}

// ApplyPlayerEdits применяет правки игрока к действующей издательской версии
// главы (T-07-05). Каждая правка проходит Цензор правок (workshop.Validate,
// T-05-15): одобренная создаёт новую версию book_text (edited_by='player',
// published=0), отклонённая пишется в book_cuts и в книгу не попадает. Слово,
// одинаково написанное трижды и более в обработанных главах, пишется в
// world_lexicon кандидатом; правка со словарным соседом применяется с пометкой
// «возможно намеренное» в дайджест.
func ApplyPlayerEdits(h *db.Handle, chapterID int64, edits []PlayerEdit) (EditResult, error) {
	if h == nil {
		return EditResult{}, errors.New("book: нет хэндла мира")
	}
	if chapterID <= 0 {
		return EditResult{}, errors.New("book: id главы не задан")
	}

	text, nextVersion, err := editSource(h, chapterID)
	if err != nil {
		return EditResult{}, err
	}

	lexicon, err := loadWorldLexicon(h)
	if err != nil {
		return EditResult{}, err
	}

	var res EditResult
	var marks []string
	var cuts []rejectedCut
	working := text

	for _, e := range edits {
		if strings.TrimSpace(e.Before) == "" || strings.TrimSpace(e.After) == "" {
			continue
		}
		if e.Before == e.After {
			continue
		}

		// Слово из world_lexicon не правится (шаг 3).
		if w, ok := editedLexiconWord(lexicon, e.Before, e.After); ok {
			cuts = append(cuts, rejectedCut{edit: e, reason: "слово из лексикона мира: " + w})
			res.Rejected = append(res.Rejected, e)
			continue
		}

		ok, reason := workshop.Validate(h, e.Before, e.After)
		if !ok {
			c := rejectedCut{edit: e, reason: reason}
			if w, found := strings.CutPrefix(reason, "кандидат в лексикон мира: "); found {
				c.lexiconCandidate = strings.TrimSpace(w)
			}
			cuts = append(cuts, c)
			res.Rejected = append(res.Rejected, e)
			continue
		}

		// Правка применяется, только если before найден ровно один раз.
		if strings.Count(working, e.Before) != 1 {
			continue
		}
		working = strings.Replace(working, e.Before, e.After, 1)
		res.Applied = append(res.Applied, e)

		if _, found := strings.CutPrefix(reason, "возможно намеренное: "); found {
			marks = append(marks, reason)
		}
	}

	var newDigest string
	if len(marks) > 0 {
		var digest sql.NullString
		if err := h.QueryRow(`SELECT digest FROM book_chapters WHERE id = ?`, chapterID).Scan(&digest); err != nil {
			return EditResult{}, err
		}
		markText := strings.Join(marks, "\n")
		if digest.Valid && digest.String != "" {
			newDigest = digest.String + "\n" + markText
		} else {
			newDigest = markText
		}
	}

	if err := writePlayerEdits(h, chapterID, nextVersion, working, newDigest, cuts, len(res.Applied) > 0); err != nil {
		return EditResult{}, err
	}
	return res, nil
}

// editSource возвращает текст действующей издательской версии главы и следующий
// номер версии (максимальный + 1 — версии растут монотонно, T-07-04 шаг 4).
func editSource(h *db.Handle, chapterID int64) (text string, nextVersion int64, err error) {
	var version int64
	err = h.QueryRow(
		`SELECT text, version FROM book_text WHERE chapter_id = ? AND published = 1 ORDER BY version DESC LIMIT 1`,
		chapterID).Scan(&text, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrNoPublished
	}
	if err != nil {
		return "", 0, err
	}

	var maxVersion int64
	if err := h.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM book_text WHERE chapter_id = ?`, chapterID).Scan(&maxVersion); err != nil {
		return "", 0, err
	}
	return text, maxVersion + 1, nil
}

// loadWorldLexicon читает нормализованные слова world_lexicon в множество.
func loadWorldLexicon(h *db.Handle) (map[string]bool, error) {
	lexicon := map[string]bool{}
	rows, err := h.Query(`SELECT word FROM world_lexicon`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			return nil, err
		}
		lexicon[foldWord(w)] = true
	}
	return lexicon, rows.Err()
}

// editedLexiconWord сообщает, правит ли правка слово из world_lexicon: слово
// лексикона есть в before, и в after его стало меньше (его меняют).
func editedLexiconWord(lexicon map[string]bool, before, after string) (string, bool) {
	if len(lexicon) == 0 {
		return "", false
	}
	b := editWordCounts(before)
	a := editWordCounts(after)
	for w, n := range b {
		if lexicon[w] && n > a[w] {
			return w, true
		}
	}
	return "", false
}

func editWordCounts(s string) map[string]int {
	counts := map[string]int{}
	for _, w := range editWords(s) {
		counts[foldWord(w)]++
	}
	return counts
}

// editWords разбирает текст на слова (буквы/цифры, дефис внутри слова) — как
// Цензор правок, но без токенов разметки.
func editWords(s string) []string {
	runes := []rune(s)
	var words []string
	i := 0
	for i < len(runes) {
		if !isEditWordRune(runes[i]) {
			i++
			continue
		}
		j := i
		for j < len(runes) {
			c := runes[j]
			if isEditWordRune(c) {
				j++
				continue
			}
			if c == '-' && j > i && j+1 < len(runes) &&
				isEditWordRune(runes[j-1]) && isEditWordRune(runes[j+1]) {
				j++
				continue
			}
			break
		}
		words = append(words, string(runes[i:j]))
		i = j
	}
	return words
}

func isEditWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// foldWord — нормализация слова для сравнения (как в Цензоре правок): нижний
// регистр, без дефисов и комбинируемых знаков ударения.
func foldWord(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if r == '-' {
			continue
		}
		if r >= 0x0300 && r <= 0x036f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// writePlayerEdits пишет итог правок писателем core: отклонённые в book_cuts,
// кандидатов в world_lexicon, одобренную новую версию в book_text и пометку
// «возможно намеренное» в book_chapters.digest.
func writePlayerEdits(h *db.Handle, chapterID, nextVersion int64, text, newDigest string, cuts []rejectedCut, applied bool) error {
	return h.Tx(db.WriterCore, func(tx *db.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		for _, c := range cuts {
			if _, err := tx.Exec("book_cuts",
				`INSERT INTO book_cuts (chapter_id, cut_text, reason, status) VALUES (?, ?, ?, 'rejected')`,
				chapterID, c.edit.After, c.reason); err != nil {
				return err
			}
			if c.lexiconCandidate != "" {
				word := foldWord(c.lexiconCandidate)
				if _, err := tx.Exec("world_lexicon",
					`INSERT INTO world_lexicon (word, status, created_at)
					 SELECT ?, 'candidate', ?
					 WHERE NOT EXISTS (SELECT 1 FROM world_lexicon WHERE word = ?)`,
					word, now, word); err != nil {
					return err
				}
			}
		}

		if !applied {
			return nil
		}

		if _, err := tx.Exec("book_text",
			`INSERT INTO book_text (chapter_id, version, text, edited_by, published) VALUES (?, ?, ?, 'player', 0)`,
			chapterID, nextVersion, text); err != nil {
			return err
		}
		if newDigest != "" {
			if _, err := tx.Exec("book_chapters",
				`UPDATE book_chapters SET digest = ? WHERE id = ?`,
				newDigest, chapterID); err != nil {
				return err
			}
		}
		return nil
	})
}
