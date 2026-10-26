package sediment

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/models"
)

// ErrDuplicate — ступень дедупа: сцена повторяет смысл существующей записи
// (W-03.4), запись не создаётся.
var ErrDuplicate = errors.New("sediment: дубль по смыслу")

// Pipeline — конвейер оседания (T-04-05, шаг 1). Ступени в фиксированном порядке:
// префильтр → проверка полноты → очистка → авто-критик → дедуп → Цензор → запись.
type Pipeline struct {
	Prefilter Prefilter
	Embedder  models.Embedder
	Slot      Slot
	Censor    Censor
	Logger    *journal.Logger
	Dedup     DedupConfig
}

// NewPipeline собирает конвейер с порогом дедупа по умолчанию (config.yaml) и
// дефолтным префильтром pass-through (без Паганеля конвейер работает, T-04-07).
func NewPipeline(embedder models.Embedder, slot Slot, censor Censor, logger *journal.Logger) *Pipeline {
	return &Pipeline{
		Prefilter: passPrefilter{},
		Embedder:  embedder,
		Slot:      slot,
		Censor:    censor,
		Logger:    logger,
		Dedup:     DefaultDedupConfig(),
	}
}

// WrittenScene — строка sediment_scene, созданная конвейером.
type WrittenScene struct {
	ID            int64
	SourceSceneID int64
	VectorID      int64
	Verdict       string
	TrustLevel    int
}

// Run прогоняет полную сцену через ступени и записывает строку литературной
// базы хэндлом писателя sediment (h). Отказ любой ступени останавливает конвейер:
// строка не создаётся, возвращается причина ступени. События журнала следуют
// контракту исходов (T-04-09, шаг 2): успех — in → clean → autocritic.verdict →
// dedup → write; дубль — dedup → dup_skip; отказ Цензора — censor_reject;
// отсев префильтром — in → prefiltered.
func (p *Pipeline) Run(ctx context.Context, h *db.Handle, scene Scene, bindings []Binding) (WrittenScene, error) {
	p.log(EventIn, journal.Field{Key: "scene_id", Value: strconv.FormatInt(scene.ID, 10)})

	// Префильтр — первая ступень (W-04.7): отсев останавливает конвейер до очистки.
	if p.Prefilter != nil && !p.Prefilter.Keep(scene) {
		p.log(EventPrefiltered, journal.Field{Key: "scene_id", Value: strconv.FormatInt(scene.ID, 10)})
		return WrittenScene{}, ErrPrefiltered
	}

	// Полнота — отдельная причина отказа.
	if scene.Status != SceneStatusClosed {
		return WrittenScene{}, fmt.Errorf("%w: status=%q", ErrNotFullScene, scene.Status)
	}

	rec, err := Clean(scene, bindings)
	if err != nil {
		return WrittenScene{}, fmt.Errorf("sediment: очистка: %w", err)
	}
	p.log(EventClean)

	rec = Autocritic(ctx, p.Slot, p.Logger, rec)

	vec, err := Vectorize(ctx, p.Embedder, rec.CleanedText)
	if err != nil {
		return WrittenScene{}, fmt.Errorf("sediment: дедуп: %w", err)
	}
	existing, err := loadExistingVectors(h)
	if err != nil {
		return WrittenScene{}, fmt.Errorf("sediment: дедуп: %w", err)
	}
	p.log(EventDedup)
	if IsDuplicate(vec, existing, p.Dedup.Threshold) {
		p.log(EventDupSkip)
		return WrittenScene{}, ErrDuplicate
	}

	// Цензор стоит перед записью внутри Write (T-04-05, шаг 2).
	id, err := Write(h, p.Censor, rec, 0, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		if errors.Is(err, ErrCensorReject) {
			p.log(EventCensorReject, journal.Field{Key: "err", Value: err.Error()})
		}
		return WrittenScene{}, err
	}
	p.log(EventWrite, journal.Field{Key: "trust_level", Value: strconv.Itoa(TrustLevelUnchecked)})
	return WrittenScene{
		ID:            id,
		SourceSceneID: rec.SourceSceneID,
		Verdict:       rec.Verdict,
		TrustLevel:    TrustLevelUnchecked,
	}, nil
}

// log пишет событие оседания в журнал станка; nil-журнал — пропуск события.
func (p *Pipeline) log(event string, fields ...journal.Field) {
	if p.Logger == nil {
		return
	}
	_ = p.Logger.Event(Source, event, fields...)
}

// loadExistingVectors возвращает векторы существующих записей sediment_scene для
// дедупа. Литературная база пока без таблицы vectors (D-19.2 — shared-векторы не
// подключены): сравнивать не с чем, первая запись проходит всегда.
func loadExistingVectors(h *db.Handle) ([][]float32, error) {
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'vectors'`).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}

	rows, err := h.Query(`SELECT vec FROM vectors WHERE owner_table = 'sediment_scene'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out [][]float32
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, err
		}
		vec, err := VectorFromBlob(blob)
		if err != nil {
			return nil, err
		}
		out = append(out, vec)
	}
	return out, rows.Err()
}
