// Оседание в очереди мастерской (T-05-16): заказ kind='sediment', поставленный
// при закрытии сцены (T-03-10) и поднятый лайком (T-04-06), исполняется
// планировщиком на тяжёлой ступени и в ночном окне. Один заказ — одна сцена;
// исполнение — прогон конвейера оседания (T-04-05) на сцену-кандидата.
package workshop

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/sediment"
)

// SedimentOrderPayload — payload заказа sediment (T-03-10, шаг 3): ссылка на
// сцену-кандидата, ветку, не вошедшую в ход. До оседания кандидат существует
// только строкой workshop_queue.
type SedimentOrderPayload struct {
	TurnID    int64  `json:"turn_id"`
	SessionID int64  `json:"session_id"`
	Kind      string `json:"kind"`
	Text      string `json:"text,omitempty"`
}

// SedimentOutcome — исход прогона конвейера оседания (T-04-05): пишется в
// статус заказа и в журнал станка.
type SedimentOutcome string

// Исходы оседания; имена совпадают с событиями журнала оседания без префикса
// «sediment.» (T-04-09).
const (
	SedimentWrite        SedimentOutcome = "write"
	SedimentDupSkip      SedimentOutcome = "dup_skip"
	SedimentCensorReject SedimentOutcome = "censor_reject"
	SedimentPrefiltered  SedimentOutcome = "prefiltered"
)

// sedimentRunEvent — событие журнала станка об исходе заказа оседания.
const sedimentRunEvent = "workshop.sediment"

// SedimentRunFunc — исполнение заказа оседания: прогон конвейера на сцену
// кандидата, перевод заказа в done/failed и событие журнала с исходом.
type SedimentRunFunc func(ctx context.Context, world *db.Handle, o *Order, logger *journal.Logger) (SedimentOutcome, error)

// DefaultSedimentRun — реализация по умолчанию; в проде собирается с
// литературной базой мира и конвейером оседания, в тестах подменяется.
var DefaultSedimentRun SedimentRunFunc

// NewSedimentRunner собирает исполнитель заказов sediment (T-05-16): конвейер
// оседания (T-04-05) на литературной базе мира.
func NewSedimentRunner(lit *db.Handle, pipeline *sediment.Pipeline) SedimentRunFunc {
	return func(ctx context.Context, world *db.Handle, o *Order, logger *journal.Logger) (SedimentOutcome, error) {
		return RunSedimentOrder(ctx, world, lit, pipeline, o, logger)
	}
}

// RunSedimentOrder исполняет заказ sediment (T-05-16, шаг 2): грузит сцену
// кандидата по turn_id из payload, прогоняет конвейер оседания и закрывает заказ
// исходом. Статус очереди меняет только ядро (писатель core); сам конвейер пишет
// литературную базу через sediment (T-04-05).
func RunSedimentOrder(ctx context.Context, world *db.Handle, lit *db.Handle, pipeline *sediment.Pipeline, o *Order, logger *journal.Logger) (SedimentOutcome, error) {
	if world == nil {
		return "", errors.New("workshop: нет хэндла мира для оседания")
	}
	if o == nil {
		return "", errors.New("workshop: нет заказа оседания")
	}

	if _, err := world.Exec("workshop_queue",
		`UPDATE workshop_queue SET status = ?, started_at = ? WHERE id = ?`,
		string(StatusRunning), nowUTC(), o.ID); err != nil {
		return "", err
	}

	scene, bindings, err := loadSceneForSediment(world, o.Payload)
	if err != nil {
		return "", err
	}

	_, runErr := pipeline.Run(ctx, lit, scene, bindings)
	outcome := outcomeFor(runErr)
	status := string(StatusDone)
	if outcome != SedimentWrite && outcome != SedimentDupSkip {
		status = string(StatusFailed)
	}

	if _, err := world.Exec("workshop_queue",
		`UPDATE workshop_queue SET status = ?, finished_at = ? WHERE id = ?`,
		status, nowUTC(), o.ID); err != nil {
		return "", err
	}

	if logger != nil {
		fields := []journal.Field{{Key: "order_id", Value: strconv.FormatInt(o.ID, 10)},
			{Key: "outcome", Value: string(outcome)}}
		_ = logger.Event("workshop", sedimentRunEvent, fields...)
	}
	if runErr != nil {
		return outcome, runErr
	}
	return outcome, nil
}

// outcomeFor сопоставляет причину отказа конвейера исходу оседания; успех — write.
func outcomeFor(err error) SedimentOutcome {
	switch {
	case err == nil:
		return SedimentWrite
	case errors.Is(err, sediment.ErrDuplicate):
		return SedimentDupSkip
	case errors.Is(err, sediment.ErrCensorReject):
		return SedimentCensorReject
	case errors.Is(err, sediment.ErrPrefiltered):
		return SedimentPrefiltered
	default:
		return ""
	}
}

// loadSceneForSediment читает сцену кандидата из оперативной базы по payload
// заказа: turn_id ссылается на сцену, чья ветка не вошла в ход.
func loadSceneForSediment(world *db.Handle, payload string) (sediment.Scene, []sediment.Binding, error) {
	var p SedimentOrderPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return sediment.Scene{}, nil, fmt.Errorf("workshop: payload заказа sediment: %w", err)
	}
	if p.TurnID == 0 {
		return sediment.Scene{}, nil, errors.New("workshop: заказ sediment без turn_id")
	}

	var scene sediment.Scene
	err := world.QueryRow(
		`SELECT id, status, prose FROM scenes WHERE turn_id = ? ORDER BY id DESC LIMIT 1`,
		p.TurnID).Scan(&scene.ID, &scene.Status, &scene.Prose)
	if errors.Is(err, sql.ErrNoRows) {
		return sediment.Scene{}, nil, fmt.Errorf("workshop: сцена заказа sediment (turn_id=%d) не найдена", p.TurnID)
	}
	if err != nil {
		return sediment.Scene{}, nil, err
	}

	bindings, err := loadSceneBindings(world, scene.ID)
	if err != nil {
		return sediment.Scene{}, nil, err
	}
	return scene, bindings, nil
}

// loadSceneBindings читает привязки биндера сцены из token_bindings и
// подставляет значение из characters (имя) или places (топоним). Значение вне
// этих источников заменой не считается — его просто нет в привязках.
func loadSceneBindings(world *db.Handle, sceneID int64) ([]sediment.Binding, error) {
	rows, err := world.Query(
		`SELECT token, character_id FROM token_bindings WHERE scene_id = ?`, sceneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rawBinding struct {
		token       string
		characterID int64
	}
	var raws []rawBinding
	for rows.Next() {
		var b rawBinding
		if err := rows.Scan(&b.token, &b.characterID); err != nil {
			return nil, err
		}
		raws = append(raws, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []sediment.Binding
	for _, b := range raws {
		if b.characterID == 0 {
			continue
		}
		var name string
		if err := world.QueryRow(`SELECT name FROM characters WHERE id = ?`, b.characterID).Scan(&name); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}
		out = append(out, sediment.Binding{Token: b.token, Value: name})
	}
	return out, nil
}
