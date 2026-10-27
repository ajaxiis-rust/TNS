// Редактор (T-05-08): пакетная правка орфографии и пунктуации главы моделью.
// Каждая правка — пара «было → стало», непрошедшее Цензора правок (T-05-15)
// бракуется автоматически. Книга обновляется точечно: применяются только
// конкретные правки, целый текст заново не публикуется. Краткое содержание
// главы пишет тот же Редактор после обработки текста (контур дайджеста T-07-06).
package workshop

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"tns65/internal/db"
)

// Роли слота Редактора: одна модель, два запроса — правки и краткое содержание.
const (
	EditorRole       = "editor"
	EditorDigestRole = "editor_digest"
)

// maxEditorRetries — мусорный ответ или тайм-аут повторяются не более двух раз,
// затем модельный шаг считается проваленным (00-INDEX «Сквозные гейты»).
const maxEditorRetries = 2

// maxEditorSceneTokens — верхняя граница сцены редактуры (W-15.3: 2–4 тыс. токенов).
const maxEditorSceneTokens = 4000

// EditorEdit — одна правка «было → стало» (W-15.5).
type EditorEdit struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// EditorResult — итог редактуры главы: применённые правки и краткое содержание.
type EditorResult struct {
	Edits   []EditorEdit
	Summary string
}

// editorReply — строгий JSON-контракт ответа Редактора на правки (шаг 4).
type editorReply struct {
	Edits []EditorEdit `json:"edits"`
}

// editorSummaryReply — строгий JSON-контракт краткого содержания (шаг 6).
type editorSummaryReply struct {
	Summary string `json:"summary"`
}

// EditChapter правит главу (W-15). Вход — собранная кодом версия 1
// (edited_by IS NULL); текст режется на сцены, каждая сцена идёт к Редактору,
// правки применяются только после Цензора правок; затем тот же Редактор пишет
// краткое содержание. Результат — новая версия book_text (edited_by='editor',
// published=0) и строка book_chapters.digest, писатель core.
func EditChapter(ctx context.Context, world *db.Handle, slot Slot, chapterID int64) (EditorResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if world == nil {
		return EditorResult{}, errors.New("workshop: нет хэндла мира для редактуры")
	}
	if slot == nil {
		return EditorResult{}, errors.New("workshop: слот Редактора не подключён")
	}

	text, version, err := readEditorSource(world, chapterID)
	if err != nil {
		return EditorResult{}, err
	}

	scenes := splitEditorScenes(text)
	edited := make([]string, len(scenes))
	var applied []EditorEdit
	for i, scene := range scenes {
		if strings.TrimSpace(scene) == "" {
			edited[i] = scene
			continue
		}
		edits, err := askEditorEdits(ctx, slot, scene)
		if err != nil {
			return EditorResult{}, err
		}
		edited[i] = applyEdits(world, scene, edits, &applied)
	}
	editedText := strings.Join(edited, "\n\n")

	summary, err := askEditorSummary(ctx, slot, editedText)
	if err != nil {
		return EditorResult{}, err
	}

	if err := writeEditorResult(world, chapterID, version, editedText, summary); err != nil {
		return EditorResult{}, err
	}
	return EditorResult{Edits: applied, Summary: summary}, nil
}

// readEditorSource читает вход редактуры — собранную кодом версию главы
// (edited_by IS NULL, T-07-04). Правленая версия входом не является, поэтому
// повторный запуск не создаёт вторую версию 2.
func readEditorSource(world *db.Handle, chapterID int64) (string, int, error) {
	var text string
	var version int
	err := world.QueryRow(
		`SELECT text, version FROM book_text WHERE chapter_id = ? AND edited_by IS NULL ORDER BY version DESC LIMIT 1`,
		chapterID).Scan(&text, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, errors.New("workshop: нет собранной версии главы для редактуры")
	}
	if err != nil {
		return "", 0, err
	}
	return text, version, nil
}

// writeEditorResult пишет новую версию book_text (edited_by='editor', published=0 —
// редактура не публикует) и краткое содержание в book_chapters.digest одной
// транзакцией писателя core.
func writeEditorResult(world *db.Handle, chapterID int64, version int, text, summary string) error {
	return world.Tx(db.WriterCore, func(tx *db.Tx) error {
		if _, err := tx.Exec("book_text",
			`INSERT INTO book_text (chapter_id, version, text, edited_by, published) VALUES (?, ?, ?, 'editor', 0)`,
			chapterID, version+1, text); err != nil {
			return err
		}
		if _, err := tx.Exec("book_chapters",
			`UPDATE book_chapters SET digest = ? WHERE id = ?`, summary, chapterID); err != nil {
			return err
		}
		return nil
	})
}

// splitEditorScenes режет главу на сцены по границам абзацев, укладываясь в
// верхнюю границу токенов на сцену (W-15.3). Разбиение без потерь: склейка сцен
// по "\n\n" возвращает исходный текст.
func splitEditorScenes(text string) []string {
	paragraphs := strings.Split(text, "\n\n")
	if len(paragraphs) <= 1 {
		return paragraphs
	}

	var scenes []string
	var cur []string
	tokens := 0
	for _, p := range paragraphs {
		t := countTokens(p)
		if tokens > 0 && tokens+t > maxEditorSceneTokens {
			scenes = append(scenes, strings.Join(cur, "\n\n"))
			cur = nil
			tokens = 0
		}
		cur = append(cur, p)
		tokens += t
	}
	if len(cur) > 0 {
		scenes = append(scenes, strings.Join(cur, "\n\n"))
	}
	return scenes
}

// askEditorEdits вызывает Редактора за списком правок сцены; мусорный ответ или
// отказ слота повторяются не более maxEditorRetries раз.
func askEditorEdits(ctx context.Context, slot Slot, scene string) ([]EditorEdit, error) {
	var lastErr error
	for attempt := 0; attempt <= maxEditorRetries; attempt++ {
		raw, err := slot.Generate(ctx, EditorRole, editorEditPrompt(scene))
		if err != nil {
			lastErr = err
			continue
		}
		edits, err := parseEditorEdits(raw)
		if err != nil {
			lastErr = err
			continue
		}
		return edits, nil
	}
	if lastErr == nil {
		lastErr = errors.New("workshop: Редактор не вернул правки")
	}
	return nil, lastErr
}

// parseEditorEdits разбирает строгий JSON-ответ Редактора: неизвестные поля и
// лишний хвост отвергаются, у каждой правки обе стороны непустые и различны.
func parseEditorEdits(raw string) ([]EditorEdit, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()

	var out editorReply
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("workshop: лишние данные после JSON-правок")
	}
	for _, e := range out.Edits {
		if strings.TrimSpace(e.Before) == "" || strings.TrimSpace(e.After) == "" {
			return nil, errors.New("workshop: пустая сторона правки")
		}
		if e.Before == e.After {
			return nil, errors.New("workshop: правка «было == стало»")
		}
	}
	return out.Edits, nil
}

// applyEdits применяет правки к сцене: правка берётся, только если before найден
// ровно один раз и правка прошла Цензор правок; непрошедшая в текст не попадает.
func applyEdits(world *db.Handle, scene string, edits []EditorEdit, applied *[]EditorEdit) string {
	for _, e := range edits {
		if strings.Count(scene, e.Before) != 1 {
			continue
		}
		if ok, _ := Validate(world, e.Before, e.After); !ok {
			continue
		}
		scene = strings.Replace(scene, e.Before, e.After, 1)
		*applied = append(*applied, e)
	}
	return scene
}

// askEditorSummary вызывает Редактора за кратким содержанием обработанной главы;
// политика отказов та же, что у списка правок.
func askEditorSummary(ctx context.Context, slot Slot, text string) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= maxEditorRetries; attempt++ {
		raw, err := slot.Generate(ctx, EditorDigestRole, editorSummaryPrompt(text))
		if err != nil {
			lastErr = err
			continue
		}
		summary, err := parseEditorSummary(raw)
		if err != nil {
			lastErr = err
			continue
		}
		return summary, nil
	}
	if lastErr == nil {
		lastErr = errors.New("workshop: Редактор не вернул краткое содержание")
	}
	return "", lastErr
}

// parseEditorSummary разбирает строгий JSON-ответ краткого содержания.
func parseEditorSummary(raw string) (string, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()

	var out editorSummaryReply
	if err := dec.Decode(&out); err != nil {
		return "", err
	}
	if _, err := dec.Token(); err != io.EOF {
		return "", errors.New("workshop: лишние данные после JSON-содержания")
	}
	if strings.TrimSpace(out.Summary) == "" {
		return "", errors.New("workshop: пустое краткое содержание")
	}
	return out.Summary, nil
}

// editorEditPrompt строит промпт правки сцены с фиксированным пунктуационным
// листом (шаг 3): тире, «ёлочки», многоточие.
func editorEditPrompt(scene string) string {
	raw, _ := json.Marshal(map[string]any{
		"task":  EditorRole,
		"scene": scene,
		"punct": []string{"тире", "ёлочки", "многоточие"},
	})
	return string(raw)
}

// editorSummaryPrompt строит промпт краткого содержания обработанной главы.
func editorSummaryPrompt(text string) string {
	raw, _ := json.Marshal(map[string]any{
		"task": EditorDigestRole,
		"text": text,
	})
	return string(raw)
}
