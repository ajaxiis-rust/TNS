package senses_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"tns65/internal/senses"
	"tns65/internal/testkit"
)

// contextsFileJSON — senses_contexts.json пака как данные: тест сверяет слой
// векторов с файлом, а не только с загрузчиком.
type contextsFileJSON struct {
	Version  int                    `json:"version"`
	Contexts []senses.SenseContexts `json:"contexts"`
}

// readContextsFile читает senses_contexts.json пака.
func readContextsFile(t *testing.T) contextsFileJSON {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(packsDir(), "senses_contexts.json"))
	if err != nil {
		t.Fatalf("ReadFile(senses_contexts.json): %v", err)
	}
	var f contextsFileJSON
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("senses_contexts.json: %v", err)
	}
	if f.Version != 1 {
		t.Fatalf("версия схемы контекстов %d, ожидалась 1", f.Version)
	}
	return f
}

// contextsOf возвращает контексты смысла пака.
func contextsOf(t *testing.T, lemma, sense string) []string {
	t.Helper()
	for _, sc := range readContextsFile(t).Contexts {
		if sc.Lemma == lemma && sc.Sense == sense {
			return sc.Contexts
		}
	}
	t.Fatalf("в senses_contexts.json нет смысла %s/%s", lemma, sense)
	return nil
}

// loadContexts загружает контексты смыслов пака через загрузчик; он же сверяет
// покрытие словаря senses.json.
func loadContexts(t *testing.T, d *senses.Dictionary) *senses.Contexts {
	t.Helper()
	c, err := senses.LoadContexts(filepath.Join(packsDir(), "senses_contexts.json"), d)
	if err != nil {
		t.Fatalf("LoadContexts(senses_contexts.json): %v", err)
	}
	return c
}

// failEmbed — недоступный эмбеддер прогона: Embed всегда отдаёт ошибку.
type failEmbed struct {
	calls int
}

func (e *failEmbed) Embed(context.Context, string) ([]float32, error) {
	e.calls++
	return nil, errors.New("эмбеддер недоступен")
}

// TestSenseVectors — центроиды смыслов (шаг 5): при пустых триггерах смысл
// выбирается по близости контекста, центроиды считаются лениво и
// переиспользуются, контексты пака покрывают все смыслы senses.json.
func TestSenseVectors(t *testing.T) {
	d := loadSenses(t)
	contexts := loadContexts(t, d)
	file := readContextsFile(t)
	if len(file.Contexts) != len(readSensesFile(t)) {
		t.Fatalf("контекстов смыслов %d, смыслов словаря %d",
			len(file.Contexts), len(readSensesFile(t)))
	}

	embed := testkit.NewFakeEmbed()
	layer := senses.NewVectorLayer(d, contexts, embed, senses.DefaultEmbedWeight)
	if layer == nil {
		t.Fatal("слой векторов не собрался")
	}

	// Окно — первый контекст смысла «строение»; других токенов в абзаце нет,
	// поэтому триггерный счёт пуст и решает вектор.
	window := contextsOf(t, "замок", "строение")[0]
	tokens := []senses.Token{{Form: "замок", Lemma: "замок", POS: "NOUN", Line: 1, Index: 0}}
	got := d.ResolveWithVectors(tokens, "замок "+window, layer)
	if len(got) != 1 {
		t.Fatalf("решений %d, ожидалось 1", len(got))
	}
	if !got[0].Decided || got[0].Sense != "строение" || got[0].Category != "architecture" {
		t.Fatalf("смысл %q (category %q, decided %v), ожидался «строение»/architecture",
			got[0].Sense, got[0].Category, got[0].Decided)
	}
	if len(got[0].Evidence) != 0 {
		t.Fatalf("триггеры %v при пустом окне", got[0].Evidence)
	}
	if embed.Calls() == 0 {
		t.Fatal("эмбеддер не вызывался")
	}

	// Повторный чанк книги берёт готовые центроиды: новый вызов только окно.
	calls := embed.Calls()
	again := d.ResolveWithVectors(tokens, "замок "+window, layer)
	if grow := embed.Calls() - calls; grow != 1 {
		t.Fatalf("повторный разбор вызвал эмбеддер %d раз, ожидался 1 (окно)", grow)
	}
	if !reflect.DeepEqual(again, got) {
		t.Fatalf("повторный разбор дал %+v, первый %+v", again, got)
	}

	// Контекст второго смысла того же омонима переводит решение на него.
	rival := contextsOf(t, "замок", "запирающее устройство")[0]
	got = d.ResolveWithVectors(tokens, "замок "+rival, layer)
	if len(got) != 1 || !got[0].Decided || got[0].Sense != "запирающее устройство" {
		t.Fatalf("решение %+v, ожидался смысл «запирающее устройство»", got)
	}
}

// TestSenseVectorsFallback — недоступный эмбеддер и вес 0 (шаг 5): прогон не
// падает, счёт остаётся триггерным и совпадает с решением T-15-02.
func TestSenseVectorsFallback(t *testing.T) {
	d := loadSenses(t)
	contexts := loadContexts(t, d)

	tokens := sentenceTokens("замок", []string{"ров", "решётка", "рыцарь"})
	text := "замок ров решётка рыцарь"
	base := d.Resolve(tokens)
	if len(base) != 1 || !base[0].Decided || base[0].Sense != "строение" {
		t.Fatalf("триггерное решение %+v, ожидался смысл «строение»", base)
	}

	// Недоступный эмбеддер: решение то же, ошибка прогон не валит.
	bad := &failEmbed{}
	broken := senses.NewVectorLayer(d, contexts, bad, senses.DefaultEmbedWeight)
	got := d.ResolveWithVectors(tokens, text, broken)
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("недоступный эмбеддер дал %+v, триггерное решение %+v", got, base)
	}
	if bad.calls == 0 {
		t.Fatal("эмбеддер не вызывался")
	}
	calls := bad.calls
	got = d.ResolveWithVectors(tokens, text, broken)
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("повтор с недоступным эмбеддером дал %+v, ожидалось %+v", got, base)
	}
	if bad.calls != calls {
		t.Fatalf("отключённый слой снова вызвал эмбеддер: %d → %d", calls, bad.calls)
	}

	// Вес 0 — ровно поведение T-15-02, эмбеддер не вызывается.
	embed := testkit.NewFakeEmbed()
	off := senses.NewVectorLayer(d, contexts, embed, 0)
	got = d.ResolveWithVectors(tokens, text, off)
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("вес 0 дал %+v, ожидалось %+v", got, base)
	}
	if embed.Calls() != 0 {
		t.Fatalf("вес 0 вызвал эмбеддер %d раз", embed.Calls())
	}
}
