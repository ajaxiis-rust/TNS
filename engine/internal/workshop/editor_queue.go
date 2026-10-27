// Очередь редактуры (T-05-09): по закрытию главы в очередь мастерской ставится
// заказ edit на главу; очередь разбирает код и запускает Редактора пакетно
// (T-05-08), по одной главе на заказ. Заказ редактуры не вторгается в живой ход —
// его разбирает код в паузу/ночь, без участия модели до запуска Редактора.
package workshop

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"tns65/internal/db"
)

// EditorQueuePayload — payload заказа edit (шаг 1): id главы на редактуру.
type EditorQueuePayload struct {
	ChapterID int64 `json:"chapter_id"`
}

// EditorQueueTask — разобранная задача очереди редактуры.
type EditorQueueTask struct {
	ID        int64
	ChapterID int64
}

// EnqueueEditorTask ставит задачу edit на закрытую главу в очередь (шаг 1,
// писатель core). Заказ создаётся только queued; модель на постановке не
// вызывается.
func EnqueueEditorTask(world *db.Handle, chapterID int64) (int64, error) {
	if world == nil {
		return 0, errors.New("workshop: нет хэндла мира для очереди редактуры")
	}
	if chapterID <= 0 {
		return 0, errors.New("workshop: id главы для редактуры не задан")
	}
	raw, err := json.Marshal(EditorQueuePayload{ChapterID: chapterID})
	if err != nil {
		return 0, err
	}
	res, err := world.Exec("workshop_queue",
		`INSERT INTO workshop_queue (kind, payload_json, priority, status, step, resumable, attempts, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		string(KindEdit), string(raw), KindPriority(KindEdit), string(StatusQueued), string(StepHeavy), 0, 0, nowUTC())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DequeueEditorTask разбирает очередь (шаг 2): берёт следующий queued заказ edit
// и переводит его в running. Модель не участвует. Возвращает (nil, nil), когда
// стоящих задач редактуры нет.
func DequeueEditorTask(world *db.Handle) (*EditorQueueTask, error) {
	if world == nil {
		return nil, errors.New("workshop: нет хэндла мира для очереди редактуры")
	}
	var id int64
	var raw string
	err := world.QueryRow(
		`SELECT id, payload_json FROM workshop_queue WHERE kind = ? AND status = ? ORDER BY priority DESC, created_at ASC, id ASC LIMIT 1`,
		string(KindEdit), string(StatusQueued)).Scan(&id, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var p EditorQueuePayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("workshop: payload заказа edit: %w", err)
	}
	if p.ChapterID <= 0 {
		return nil, errors.New("workshop: заказ edit без id главы")
	}

	if _, err := world.Exec("workshop_queue",
		`UPDATE workshop_queue SET status = ?, started_at = ? WHERE id = ?`,
		string(StatusRunning), nowUTC(), id); err != nil {
		return nil, err
	}
	return &EditorQueueTask{ID: id, ChapterID: p.ChapterID}, nil
}

// RunEditorTask исполняет разобранную задачу редактуры (шаг 3): запускает
// Редактора пакетно (T-05-08) ровно один раз — одна глава на задачу — и
// переводит задачу в done. Статус очереди меняет только ядро (писатель core).
func RunEditorTask(ctx context.Context, world *db.Handle, slot Slot, task *EditorQueueTask) (EditorResult, error) {
	if task == nil {
		return EditorResult{}, errors.New("workshop: нет задачи редактуры для исполнения")
	}
	res, err := EditChapter(ctx, world, slot, task.ChapterID)
	if err != nil {
		return EditorResult{}, err
	}
	if _, err := world.Exec("workshop_queue",
		`UPDATE workshop_queue SET status = ?, finished_at = ? WHERE id = ?`,
		string(StatusDone), nowUTC(), task.ID); err != nil {
		return EditorResult{}, err
	}
	return res, nil
}
