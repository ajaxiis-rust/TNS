package sediment

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"tns65/internal/models"
)

// Vectorize вычисляет вектор смысла очищенной записи через клиент эмбеддингов
// (в тестах — testkit.FakeEmbed). Вектор — ключ дедупа; его BLOB-форма и ссылка
// vector_id пишутся строкой таблицы vectors (T-04-04, шаг 1).
func Vectorize(ctx context.Context, embedder models.Embedder, cleanedText string) ([]float32, error) {
	if embedder == nil {
		return nil, fmt.Errorf("sediment: эмбеддер не задан")
	}
	vec, err := embedder.Embed(ctx, cleanedText)
	if err != nil {
		return nil, fmt.Errorf("sediment: эмбеддинг очищенной записи: %w", err)
	}
	if len(vec) != models.EmbedDim {
		return nil, fmt.Errorf("sediment: размерность вектора %d, want %d", len(vec), models.EmbedDim)
	}
	return vec, nil
}

// VectorBlob кодирует вектор смысла в BLOB Float32 little-endian (D-17.2).
func VectorBlob(vec []float32) ([]byte, error) {
	if len(vec) != models.EmbedDim {
		return nil, fmt.Errorf("sediment: размерность вектора %d, want %d", len(vec), models.EmbedDim)
	}
	buf := make([]byte, len(vec)*4)
	for i, f := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf, nil
}

// VectorFromBlob разбирает BLOB Float32 little-endian в вектор смысла (D-17.2).
func VectorFromBlob(blob []byte) ([]float32, error) {
	if len(blob)%4 != 0 {
		return nil, fmt.Errorf("sediment: длина BLOB %d не кратна 4", len(blob))
	}
	vec := make([]float32, len(blob)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return vec, nil
}
