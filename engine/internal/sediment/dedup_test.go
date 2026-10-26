package sediment_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"tns65/internal/models"
	"tns65/internal/sediment"
	"tns65/internal/testkit"
)

// dedupYAML — секция sediment с порогом дедупа из «Проверки» T-04-04.
const dedupYAML = `sediment:
  dedup_threshold: 0.85
`

func TestDedup(t *testing.T) {
	ctx := context.Background()
	embed := testkit.NewFakeEmbed()

	t.Run("две сцены одного смысла — вторая отклонена", func(t *testing.T) {
		v1, err := sediment.Vectorize(ctx, embed, "Иван купил меч за тридцать монет")
		if err != nil {
			t.Fatalf("Vectorize: %v", err)
		}
		v2, err := sediment.Vectorize(ctx, embed, "Иван купил меч за тридцать монет")
		if err != nil {
			t.Fatalf("Vectorize: %v", err)
		}
		if !sediment.IsDuplicate(v2, [][]float32{v1}, 0.85) {
			t.Fatalf("сцена того же смысла должна быть отклонена")
		}
	})

	t.Run("сцены разного смысла — обе проходят", func(t *testing.T) {
		a := unitVector()
		b := negate(a)
		if sediment.IsDuplicate(a, [][]float32{b}, 0.85) {
			t.Fatalf("сцена разного смысла отклонена (a)")
		}
		if sediment.IsDuplicate(b, [][]float32{a}, 0.85) {
			t.Fatalf("сцена разного смысла отклонена (b)")
		}
	})

	t.Run("пограничные косинусы 0.84 и 0.86", func(t *testing.T) {
		base := unitVector()
		if sediment.IsDuplicate(along(0.84), [][]float32{base}, 0.85) {
			t.Fatalf("косинус 0.84 должен пройти")
		}
		if !sediment.IsDuplicate(along(0.86), [][]float32{base}, 0.85) {
			t.Fatalf("косинус 0.86 должен быть дублем")
		}
	})

	t.Run("первая запись без существующих векторов проходит", func(t *testing.T) {
		if sediment.IsDuplicate(unitVector(), nil, 0.85) {
			t.Fatalf("первая запись должна пройти")
		}
	})

	t.Run("размерность всех векторов 1024", func(t *testing.T) {
		v, err := sediment.Vectorize(ctx, embed, "очищенная сцена")
		if err != nil {
			t.Fatalf("Vectorize: %v", err)
		}
		if len(v) != models.EmbedDim {
			t.Fatalf("вектор dim=%d, want %d", len(v), models.EmbedDim)
		}
		for _, vec := range [][]float32{unitVector(), along(0.84), along(0.86), negate(unitVector())} {
			if len(vec) != models.EmbedDim {
				t.Fatalf("фикстура dim=%d, want %d", len(vec), models.EmbedDim)
			}
		}
	})

	t.Run("BLOB Float32 little-endian круговая проверка", func(t *testing.T) {
		v, err := sediment.Vectorize(ctx, embed, "ещё одна сцена")
		if err != nil {
			t.Fatalf("Vectorize: %v", err)
		}
		blob, err := sediment.VectorBlob(v)
		if err != nil {
			t.Fatalf("VectorBlob: %v", err)
		}
		if len(blob) != models.EmbedDim*4 {
			t.Fatalf("BLOB длина %d, want %d", len(blob), models.EmbedDim*4)
		}
		back, err := sediment.VectorFromBlob(blob)
		if err != nil {
			t.Fatalf("VectorFromBlob: %v", err)
		}
		if len(back) != models.EmbedDim {
			t.Fatalf("вектор из BLOB dim=%d, want %d", len(back), models.EmbedDim)
		}
		for i := range v {
			if back[i] != v[i] {
				t.Fatalf("вектор из BLOB не совпал на позиции %d: %v != %v", i, back[i], v[i])
			}
		}
	})

	t.Run("порог берётся из конфига", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte(dedupYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := sediment.LoadDedupConfig(path)
		if err != nil {
			t.Fatalf("LoadDedupConfig: %v", err)
		}
		if cfg.Threshold != 0.85 {
			t.Fatalf("порог из конфига = %v, want 0.85", cfg.Threshold)
		}
		base := unitVector()
		if !sediment.IsDuplicate(along(0.86), [][]float32{base}, cfg.Threshold) {
			t.Fatalf("0.86 при пороге 0.85 должен быть дублем")
		}

		// Иной порог в конфиге меняет решение: 0.86 при пороге 0.87 проходит.
		if err := os.WriteFile(path, []byte("sediment:\n  dedup_threshold: 0.87\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err = sediment.LoadDedupConfig(path)
		if err != nil {
			t.Fatalf("LoadDedupConfig: %v", err)
		}
		if cfg.Threshold != 0.87 {
			t.Fatalf("порог из конфига = %v, want 0.87", cfg.Threshold)
		}
		if sediment.IsDuplicate(along(0.86), [][]float32{base}, cfg.Threshold) {
			t.Fatalf("0.86 при пороге 0.87 должен пройти")
		}
	})

	t.Run("порог читается из реального config.yaml", func(t *testing.T) {
		cfg, err := sediment.LoadDedupConfig(filepath.Join("..", "..", "config.yaml"))
		if err != nil {
			t.Fatalf("LoadDedupConfig(engine/config.yaml): %v", err)
		}
		if cfg.Threshold != 0.85 {
			t.Fatalf("engine/config.yaml порог = %v, want 0.85", cfg.Threshold)
		}
	})
}

// unitVector — единичный вектор dim=1024 по оси x.
func unitVector() []float32 {
	v := make([]float32, models.EmbedDim)
	v[0] = 1
	return v
}

// negate — вектор противоположного смысла (косинус -1 к исходному).
func negate(v []float32) []float32 {
	out := make([]float32, len(v))
	for i, f := range v {
		out[i] = -f
	}
	return out
}

// along — единичный вектор dim=1024, косинус которого к оси x равен cos.
func along(cos float32) []float32 {
	v := make([]float32, models.EmbedDim)
	v[0] = cos
	v[1] = float32(math.Sqrt(float64(1 - cos*cos)))
	return v
}
