package turn

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"tns65/internal/models"
)

// confirmYAML — секция confirm с порогами из «Проверки» T-03-04.
const confirmYAML = `confirm:
  dict_threshold: 0.6
  vector_threshold: 0.75
  margin_dict: 0.7
  margin_vector: 0.85
`

// writeConfirmConfig пишет config.yaml с секцией confirm во временный каталог.
func writeConfirmConfig(t *testing.T) (string, ConfirmConfig) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(confirmYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfirmConfig(path)
	if err != nil {
		t.Fatalf("LoadConfirmConfig: %v", err)
	}
	return path, cfg
}

// fakeConfirmEmbed — эмбеддер гипотезы: единичный вектор по оси x, поэтому
// косинус вектора целой реплики к гипотезе равен первой координате реплики.
type fakeConfirmEmbed struct{}

func (fakeConfirmEmbed) Embed(_ context.Context, _ string) ([]float32, error) {
	v := make([]float32, models.EmbedDim)
	v[0] = 1
	return v, nil
}

// replicaVector — единичный вектор целой реплики dim=1024, косинус которого к
// гипотезе (оси x) равен cos.
func replicaVector(cos float32) []float32 {
	v := make([]float32, models.EmbedDim)
	v[0] = cos
	v[1] = float32(math.Sqrt(float64(1 - cos*cos)))
	return v
}

// runConfirm вычисляет подтверждение над гипотезой, леммами реплики и косинусом.
func runConfirm(t *testing.T, cfg ConfirmConfig, hypothesis, replica []string, cos float32) Confirmation {
	t.Helper()
	st := &TurnState{
		HypothesisLemmas: hypothesis,
		Embedder:         fakeConfirmEmbed{},
		Cut: Cut{
			Whole:    replicaVector(cos),
			Segments: []Segment{{Lemmas: replica}},
		},
	}
	return confirm(context.Background(), st, cfg)
}

func TestConfirm(t *testing.T) {
	_, cfg := writeConfirmConfig(t)

	// Пороги приходят из config.yaml (Проверка T-03-04).
	if cfg.DictThreshold != 0.6 || cfg.VectorThreshold != 0.75 ||
		cfg.MarginDict != 0.7 || cfg.MarginVector != 0.85 {
		t.Fatalf("пороги config.yaml = %+v, ожидались 0.6/0.75/0.7/0.85", cfg)
	}

	// Гипотеза намерения — леммы и глаголы из лексикона, с дублями.
	hyp := []string{"идти", "ехать", "плыть", "бежать", "идти", "ехать", "плыть", "бежать", "двигаться", "отправиться"}

	// Один совпавший глагол выбранного намерения подтверждает словарный голос:
	// доля по всему списку (1 из 10) отсекала естественную реплику.
	div := runConfirm(t, cfg, hyp, []string{"идти"}, 0.4)
	if div.Decision != DecisionConfirmed {
		t.Fatalf("одно совпадение: решение = %q, ожидалось %q", div.Decision, DecisionConfirmed)
	}
	if div.DictMatch != 1 {
		t.Fatalf("одно совпадение: dict_match = %f, ожидалось 1", div.DictMatch)
	}
	if !div.Disagreement {
		t.Fatalf("расхождение: Disagreement = false, ожидалось true")
	}
	if !div.Confirmed() {
		t.Fatalf("расхождение: по словарю смысл не подтверждён")
	}

	// Пересечения нет — словарного подтверждения нет, ход уходит на переспрос.
	reask := runConfirm(t, cfg, hyp, []string{"привет"}, 0.95)
	if reask.Decision != DecisionReask {
		t.Fatalf("нет совпадения: решение = %q, ожидалось %q", reask.Decision, DecisionReask)
	}
	if reask.Confirmed() {
		t.Fatalf("нет совпадения: смысл ошибочно подтверждён")
	}
	if empty := runConfirm(t, cfg, nil, []string{"идти"}, 0.95); empty.Decision != DecisionReask {
		t.Fatalf("пустая гипотеза: решение = %q, ожидалось %q", empty.Decision, DecisionReask)
	}

	// Дерзкий режим: dict_match ≥ 0.7 и cos ≥ 0.85 — подтверждение коммитится сразу.
	margin := runConfirm(t, cfg, hyp, []string{"идти", "ехать", "плыть"}, 0.866)
	if margin.Decision != DecisionMargin {
		t.Fatalf("запас: решение = %q, ожидалось %q", margin.Decision, DecisionMargin)
	}
	if !margin.Confirmed() {
		t.Fatalf("запас: смысл не подтверждён")
	}

	// Обычное согласие: dict_match ≥ 0.6 и cos ≥ 0.75 без запаса (cos < 0.85).
	plain := runConfirm(t, cfg, hyp, []string{"идти", "ехать"}, 0.8)
	if plain.Decision != DecisionConfirmed || plain.Disagreement {
		t.Fatalf("согласие: решение = %+v, ожидалось confirmed без расхождения", plain)
	}
}

// TestConfirmConfigLoaded проверяет, что пороги читаются из реального config.yaml
// движка (значения из «Проверки» T-03-04), а не зашиты в коде.
func TestConfirmConfigLoaded(t *testing.T) {
	cfg, err := LoadConfirmConfig(filepath.Join("..", "..", "config.yaml"))
	if err != nil {
		t.Fatalf("LoadConfirmConfig(engine/config.yaml): %v", err)
	}
	if cfg.DictThreshold != 0.6 || cfg.VectorThreshold != 0.75 ||
		cfg.MarginDict != 0.7 || cfg.MarginVector != 0.85 {
		t.Fatalf("engine/config.yaml пороги = %+v, ожидались 0.6/0.75/0.7/0.85", cfg)
	}
}
