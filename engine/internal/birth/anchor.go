package birth

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/models"
)

// anchorKey — ключ world_meta со ссылкой на нарратив-якорь (T-02-07, шаг 4).
const anchorKey = "anchor_narrative_id"

// defaultMatchThreshold — порог косинусной близости якоря по умолчанию (T-02-07, шаг 3).
const defaultMatchThreshold = 0.5

// anchorResult — итог подбора якоря.
type anchorResult struct {
	narrativeID int64
	forced      bool
}

// anchor — шаг цепочки рождения: подбор нарратива-якоря по векторам и запись
// ссылки в world_meta хэндлом forge (карта писателей T-01-07).
func (b *builder) anchor() error {
	res, err := selectAnchor(context.Background(), b.embedder, b.baseDBPath, b.a.Synopsis, b.matchThreshold)
	if err != nil {
		return err
	}
	if res.forced {
		b.logAnchorForced(res.narrativeID)
	}

	h, err := db.OpenWriterAs(b.worldDB, db.DBWorld, db.WriterForge, b.logger)
	if err != nil {
		return err
	}
	defer h.Close()
	if _, err := h.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES (?, ?)`,
		anchorKey, strconv.FormatInt(res.narrativeID, 10)); err != nil {
		return err
	}
	return nil
}

// logAnchorForced пишет пометку forced=true в журнал рождения (B-10, шаг 3).
func (b *builder) logAnchorForced(id int64) {
	if b.logger == nil {
		return
	}
	_ = b.logger.Event("birth", journal.EventAnchorForced,
		journal.Field{Key: "narrative_id", Value: strconv.FormatInt(id, 10)},
		journal.Field{Key: "forced", Value: "true"})
}

// selectAnchor выбирает нарратив-якорь по косинусной близости векторов смысла
// синопсиса и нарративов base.db. Близкий — косинус ≥ threshold; при отсутствии
// близкого берётся принудительно ближайший с пометкой forced (B-10).
func selectAnchor(ctx context.Context, embedder models.Embedder, baseDBPath, synopsis string, threshold float64) (anchorResult, error) {
	if threshold <= 0 {
		threshold = defaultMatchThreshold
	}
	synVec, err := embedder.Embed(ctx, synopsis)
	if err != nil {
		return anchorResult{}, fmt.Errorf("birth: эмбеддинг синопсиса: %w", err)
	}

	vecs, err := readNarrativeVectors(baseDBPath)
	if err != nil {
		return anchorResult{}, err
	}
	if len(vecs) == 0 {
		// Пустой base.db — дефект запекания T-01-16; отдельной ветки нет (шаг 3).
		return anchorResult{}, fmt.Errorf("birth: в base.db нет векторов нарративов")
	}

	best := int64(0)
	bestSim := math.Inf(-1)
	for id, vec := range vecs {
		if len(vec) != len(synVec) {
			return anchorResult{}, fmt.Errorf("birth: размерность вектора нарратива %d: %d, want %d", id, len(vec), len(synVec))
		}
		if sim := cosine(synVec, vec); sim > bestSim {
			bestSim = sim
			best = id
		}
	}
	return anchorResult{narrativeID: best, forced: bestSim < threshold}, nil
}

// readNarrativeVectors читает векторы нарративов из base.db только на чтение.
func readNarrativeVectors(baseDBPath string) (map[int64][]float32, error) {
	d, err := db.OpenReader(baseDBPath)
	if err != nil {
		return nil, fmt.Errorf("birth: открыть base.db для чтения: %w", err)
	}
	defer d.Close()

	rows, err := d.Query(`SELECT owner_id, vec FROM vectors WHERE owner_table = 'narratives'`)
	if err != nil {
		return nil, fmt.Errorf("birth: читать векторы нарративов: %w", err)
	}
	defer rows.Close()

	out := make(map[int64][]float32)
	for rows.Next() {
		var id int64
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, fmt.Errorf("birth: разбор вектора нарратива: %w", err)
		}
		if len(blob)%4 != 0 {
			return nil, fmt.Errorf("birth: вектор нарратива %d: длина %d не кратна 4", id, len(blob))
		}
		vec := make([]float32, len(blob)/4)
		for i := range vec {
			vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
		}
		out[id] = vec
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// cosine — косинусная близость двух векторов одинаковой размерности.
func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		na += ai * ai
		nb += bi * bi
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
