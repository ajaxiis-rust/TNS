package turn

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"tns65/internal/lemmarpc"
	"tns65/internal/models"
	"tns65/internal/testkit"
)

// fakeLemmatizer — подставной лемматизатор: лемма — текст в нижнем регистре.
type fakeLemmatizer struct{}

func (fakeLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error) {
	return []lemmarpc.Token{{Form: text, Lemma: strings.ToLower(text), Pos: "VERB"}}, nil
}

func TestCut(t *testing.T) {
	p, _ := newTestPipeline(t) // даёт writer core и схему world+shared (vectors).

	slot := testkit.NewFakeSlot()
	embed := testkit.NewFakeEmbed()

	stage := &cutStage{logger: p.Logger}
	st := &TurnState{
		Input:      TurnInput{SessionID: 1, PlayerText: "Он шёл по дороге. Ветер стих? Впереди город!"},
		Embedder:   embed,
		Lemmatizer: fakeLemmatizer{},
	}
	if err := stage.Run(context.Background(), st); err != nil {
		t.Fatalf("cut: %v", err)
	}

	// Реплика из трёх предложений режется на три сегмента (шаг 1).
	if len(st.Cut.Segments) != 3 {
		t.Fatalf("сегментов = %d, ожидалось 3", len(st.Cut.Segments))
	}
	for _, seg := range st.Cut.Segments {
		if seg.Text == "" {
			t.Fatalf("пустой сегмент не отброшен")
		}
	}

	// Векторов ровно 4: 3 сегмента + 1 целой реплики.
	var vectors [][]float32
	if st.Cut.Whole == nil {
		t.Fatalf("вектор целой реплики отсутствует")
	}
	vectors = append(vectors, st.Cut.Whole)
	for _, seg := range st.Cut.Segments {
		if seg.Vector == nil {
			t.Fatalf("вектор сегмента %q отсутствует", seg.Text)
		}
		vectors = append(vectors, seg.Vector)
	}
	if len(vectors) != 4 {
		t.Fatalf("векторов = %d, ожидалось 4", len(vectors))
	}
	for i, v := range vectors {
		if len(v) != models.EmbedDim {
			t.Fatalf("вектор %d: dim=%d, ожидалось %d", i, len(v), models.EmbedDim)
		}
		if n := vectorNorm(v); math.Abs(n-1) > 1e-4 {
			t.Fatalf("вектор %d: норма=%f, ожидалась 1", i, n)
		}
	}

	// Вызовов слота генерации нет (счётчик testkit).
	if slot.Calls() != 0 {
		t.Fatalf("вызовов слота генерации = %d, ожидалось 0", slot.Calls())
	}

	// Ранний вектор слова не пишет строку в vectors.
	if err := assertVectorsEmpty(p); err != nil {
		t.Fatal(err)
	}
}

func TestCutNoLLM(t *testing.T) {
	p, _ := newTestPipeline(t)

	slot := testkit.NewFakeSlot()
	embed := testkit.NewFakeEmbed()

	stage := &cutStage{logger: p.Logger}
	st := &TurnState{
		Input:      TurnInput{SessionID: 2, PlayerText: "Я иду домой, а ты сидишь."},
		Embedder:   embed,
		Lemmatizer: fakeLemmatizer{},
	}
	if err := stage.Run(context.Background(), st); err != nil {
		t.Fatalf("cut: %v", err)
	}

	// Ранние векторы по словам вычислены сразу, без слота генерации (шаг 3).
	if len(st.Cut.Words) == 0 {
		t.Fatalf("ранних векторов по словам нет")
	}
	for i, w := range st.Cut.Words {
		if len(w.Vector) != models.EmbedDim {
			t.Fatalf("вектор слова %d: dim=%d, ожидалось %d", i, len(w.Vector), models.EmbedDim)
		}
		if n := vectorNorm(w.Vector); math.Abs(n-1) > 1e-4 {
			t.Fatalf("вектор слова %d: норма=%f, ожидалась 1", i, n)
		}
	}

	// Без LLM: счётчик слота генерации 0, эмбеддер вызывался.
	if slot.Calls() != 0 {
		t.Fatalf("вызовов слота генерации = %d, ожидалось 0", slot.Calls())
	}
	if embed.Calls() == 0 {
		t.Fatalf("эмбеддер не вызывался")
	}

	// Промежуточные векторы не пишут строку в vectors.
	if err := assertVectorsEmpty(p); err != nil {
		t.Fatal(err)
	}
}

// assertVectorsEmpty проверяет, что таблица vectors пуста: резка и ранние векторы
// не пишут в базу (T-03-02, «Писатель: нет записи»).
func assertVectorsEmpty(p *Pipeline) error {
	var n int
	if err := p.DB.QueryRow(`SELECT COUNT(*) FROM vectors`).Scan(&n); err != nil {
		return fmt.Errorf("COUNT(vectors): %w", err)
	}
	if n != 0 {
		return fmt.Errorf("vectors непуста: %d строк, ожидалось 0", n)
	}
	return nil
}

func vectorNorm(v []float32) float64 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}
