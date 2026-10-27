package book

import (
	"errors"

	"tns65/internal/db"
)

// Эпилог и метки пиков (T-07-13, CB-13): эпилог — единственная глава чистого
// авторства игрока, движком не генерируется и не обрабатывается; book_peaks —
// авторская разметка важных/поворотных моментов (сцен и глав).

// PeakKind — вид метки пика (CHECK book_peaks.kind).
type PeakKind string

const (
	// PeakScene — метка сцены: сцена метится по scene_id.
	PeakScene PeakKind = "scene"
	// PeakChapter — метка главы: scene_id не задаётся.
	PeakChapter PeakKind = "chapter"
)

// Peak — авторская метка пика книги (CB-13.2).
type Peak struct {
	ID        int64
	ChapterID int64
	SceneID   int64
	Kind      PeakKind
	Note      string
	CreatedAt string
}

// Приоритет оседания сцены: базовый 50 (W-04, T-02-11), метка пика добавляет
// надбавку 30 — пик важнее лайка (+20 у сигналов T-04-06).
const (
	peakSedimentBase  = 50
	peakSedimentBoost = 30
)

// SaveEpilogue записывает эпилог — главу чистого авторства игрока (CB-13.1):
// строка book_chapters (kind='epilogue', status='published', day не назначается)
// и действующая издательская версия book_text (edited_by='player', published=1).
// Движок эпилог не обрабатывает: нет ни Редактора, ни Цензора, ни задачи
// редактуры в очереди.
func SaveEpilogue(h *db.Handle, title, text string) (Chapter, error) {
	if h == nil {
		return Chapter{}, errors.New("book: нет хэндла мира")
	}
	if title == "" {
		return Chapter{}, errors.New("book: заголовок эпилога не задан")
	}
	if text == "" {
		return Chapter{}, errors.New("book: текст эпилога не задан")
	}

	var ch Chapter
	err := h.Tx(db.WriterCore, func(tx *db.Tx) error {
		res, err := tx.Exec("book_chapters",
			`INSERT INTO book_chapters (day, title, status, kind) VALUES (0, ?, 'published', 'epilogue')`, title)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}

		if _, err := tx.Exec("book_text",
			`INSERT INTO book_text (chapter_id, version, text, edited_by, published) VALUES (?, 1, ?, 'player', 1)`,
			id, text); err != nil {
			return err
		}

		ch = Chapter{ID: id, Day: 0, Title: title, Status: StatusPublished, Kind: KindEpilogue}
		return nil
	})
	if err != nil {
		return Chapter{}, err
	}
	return ch, nil
}

// MarkPeak помечает важный/поворотный момент книги — авторская разметка пиков
// (CB-13.2). Сцена метится по scene_id (kind='scene'), глава — kind='chapter'
// (scene_id не задаётся). Запись пишет core.
func MarkPeak(h *db.Handle, chapterID, sceneID int64, kind PeakKind, note, now string) (Peak, error) {
	if h == nil {
		return Peak{}, errors.New("book: нет хэндла мира")
	}
	if chapterID <= 0 {
		return Peak{}, errors.New("book: id главы для метки не задан")
	}
	if note == "" {
		return Peak{}, errors.New("book: пометка пика не задана")
	}
	if now == "" {
		return Peak{}, errors.New("book: время метки не задано")
	}
	switch kind {
	case PeakScene:
		if sceneID <= 0 {
			return Peak{}, errors.New("book: сцена метится по scene_id")
		}
	case PeakChapter:
		sceneID = 0
	default:
		return Peak{}, errors.New("book: вид метки пика не задан")
	}

	var peak Peak
	err := h.Tx(db.WriterCore, func(tx *db.Tx) error {
		res, err := tx.Exec("book_peaks",
			`INSERT INTO book_peaks (chapter_id, scene_id, kind, note, created_at) VALUES (?, ?, ?, ?, ?)`,
			chapterID, sceneID, string(kind), note, now)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		peak = Peak{ID: id, ChapterID: chapterID, SceneID: sceneID, Kind: kind, Note: note, CreatedAt: now}
		return nil
	})
	if err != nil {
		return Peak{}, err
	}
	return peak, nil
}

// ScenePeaks читает метки пиков сцены из book_peaks по scene_id (CB-13.3,
// потребитель W-03). Метки глав сюда не попадают — у них scene_id не задан.
func ScenePeaks(h *db.Handle, sceneID int64) ([]Peak, error) {
	if h == nil {
		return nil, errors.New("book: нет хэндла мира")
	}
	if sceneID <= 0 {
		return nil, errors.New("book: id сцены не задан")
	}

	rows, err := h.Query(
		`SELECT id, chapter_id, scene_id, kind, note, created_at FROM book_peaks WHERE scene_id = ? ORDER BY id`,
		sceneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Peak
	for rows.Next() {
		var p Peak
		var sceneID int64
		var kind string
		if err := rows.Scan(&p.ID, &p.ChapterID, &sceneID, &kind, &p.Note, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.SceneID = sceneID
		p.Kind = PeakKind(kind)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// SedimentPriority — приоритет оседания сцены с учётом авторской метки пика
// (CB-13.3): базовый 50 плюс надбавка, если сцена помечена в book_peaks.
func SedimentPriority(h *db.Handle, sceneID int64) (int, error) {
	peaks, err := ScenePeaks(h, sceneID)
	if err != nil {
		return 0, err
	}
	priority := peakSedimentBase
	if len(peaks) > 0 {
		priority += peakSedimentBoost
	}
	return priority, nil
}
