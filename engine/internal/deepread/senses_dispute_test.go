package deepread_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/deepread"
	"tns65/internal/testkit"
)

// TestSenseDispute — гейт спорных смыслов (T-15-04, шаги 1–3): омоним с
// нерешённым смыслом уходит в очередь с цитатой, номером токена и смыслами с
// триггерами; ответ слота меняет тег чанка и пишется в forge_log; отклонённая
// цитата тег не меняет — он остаётся консервативным.
func TestSenseDispute(t *testing.T) {
	dir := sensePack(t)
	cfg := deepread.DefaultSalienceConfig()
	layer, err := deepread.LoadSenseLayer(dir, cfg)
	if err != nil {
		t.Fatalf("LoadSenseLayer: %v", err)
	}
	if layer == nil || layer.Dict == nil {
		t.Fatal("LoadSenseLayer: словарь смыслов не загружен")
	}
	catalog, err := deepread.LoadSenseCatalog(dir)
	if err != nil {
		t.Fatalf("LoadSenseCatalog: %v", err)
	}
	dicts := senseDicts()
	chunk := senseTextChunk(11, "Замок у реки.", "Замок", "река")
	questions := senseQuestions(t, chunk, layer, catalog)

	t.Run("очередь с цитатой", func(t *testing.T) {
		questions := senseQuestions(t, chunk, layer, catalog)
		if len(questions) != 1 {
			t.Fatalf("вопросов %d, want 1", len(questions))
		}
		q := questions[0]
		if q.Lemma != "замок" || q.POS != "NOUN" {
			t.Errorf("омоним %q/%q, want замок/NOUN", q.Lemma, q.POS)
		}
		if q.Token != 1 || q.ChunkID != chunk.ID {
			t.Errorf("токен/чанк = %d/%d, want 1/%d", q.Token, q.ChunkID, chunk.ID)
		}
		if !strings.Contains(chunk.Text, q.Quote) || !strings.Contains(q.Quote, "Замок") {
			t.Errorf("цитата %q не подстрока чанка %q с омонимом", q.Quote, chunk.Text)
		}
		if len(q.Options) != 2 || !hasString(q.Options, "строение") ||
			!hasString(q.Options, "запирающее устройство") {
			t.Errorf("канон смыслов = %v, want строение и запирающее устройство", q.Options)
		}
		for _, option := range q.Senses {
			if len(option.Triggers) == 0 {
				t.Errorf("смысл %q без триггеров", option.Sense)
			}
		}

		// Решённый смысл вопросом не становится (шаг 1).
		decided := senseTextChunk(12, "Замок ров решётка рыцарь.", "Замок", "ров", "решётка", "рыцарь")
		if got := senseQuestions(t, decided, layer, catalog); len(got) != 0 {
			t.Fatalf("решённый смысл ушёл в очередь: %v", got)
		}
	})

	t.Run("ответ меняет тег", func(t *testing.T) {
		lit, _ := newTestLiterary(t)
		key := senseKey("answered")
		baseline := senseTags(t, chunk, dicts, layer, cfg)
		if tag := senseTag(t, baseline, deepread.KeyArchitecture); tag.Band != deepread.BandConfident {
			t.Fatalf("до ответа: полоса %q, want confident", tag.Band)
		}

		slot := testkit.NewFakeSlot()
		slot.SetResponse(deepread.DisputeSlotRole,
			`[{"value":"запирающее устройство","quote":"Замок у реки","confidence":0.9}]`)
		logger, counter := newDisputeJournal(t)

		outcomes, err := deepread.AskSenseDisputes(lit, key, questions,
			deepread.DisputeConfig{Budget: 5, Batch: 5}, slot, logger, senseNow)
		if err != nil {
			t.Fatalf("AskSenseDisputes: %v", err)
		}
		if len(outcomes) != 1 || outcomes[0].State != deepread.DisputeAnswered {
			t.Fatalf("состояние ответа = %+v, want answered", outcomes)
		}
		if outcomes[0].Value != "запирающее устройство" || outcomes[0].Category != "" {
			t.Errorf("смысл/категория = %q/%q, want запирающее устройство и пустая",
				outcomes[0].Value, outcomes[0].Category)
		}
		if got := slot.Calls(); got != 1 {
			t.Errorf("вызовов слота %d, want 1", got)
		}
		if got := counter.get(); got != 1 {
			t.Errorf("событий deepread.slot %d, want 1", got)
		}

		// Смысл вне категорий убирает вклад омонима: тег становится reject.
		tagged := senseTags(t, chunk, deepread.SenseDictionaries(dicts, outcomes), layer, cfg)
		if tag := senseTag(t, tagged, deepread.KeyArchitecture); tag.Score != 0 || tag.Band != deepread.BandReject {
			t.Errorf("после ответа: скор %g, полоса %q, want 0 и reject", tag.Score, tag.Band)
		}
		if n := len(dicts[deepread.KeyArchitecture].Lemmas); n != 1 {
			t.Errorf("словарь прогона правлен на месте: лемм %d, want 1", n)
		}

		// Ответ писан в forge_log отдельной строкой (шаг 3).
		if n := countRows(t, lit, `SELECT COUNT(*) FROM forge_log WHERE kind = ?`, deepread.KindSenseDispute); n != 1 {
			t.Errorf("строк forge_log kind=%q = %d, want 1", deepread.KindSenseDispute, n)
		}
	})

	t.Run("ответ в своей категории тег сохраняет", func(t *testing.T) {
		lit, _ := newTestLiterary(t)
		baseline := senseTags(t, chunk, dicts, layer, cfg)

		slot := testkit.NewFakeSlot()
		slot.SetResponse(deepread.DisputeSlotRole, `{"value":"строение","quote":"Замок","confidence":0.8}`)
		logger, _ := newDisputeJournal(t)
		outcomes, err := deepread.AskSenseDisputes(lit, senseKey("architecture"), questions,
			deepread.DisputeConfig{Budget: 5, Batch: 5}, slot, logger, senseNow)
		if err != nil {
			t.Fatalf("AskSenseDisputes: %v", err)
		}
		tagged := senseTags(t, chunk, deepread.SenseDictionaries(dicts, outcomes), layer, cfg)
		if got, want := senseTag(t, tagged, deepread.KeyArchitecture),
			senseTag(t, baseline, deepread.KeyArchitecture); !sameTag(got, want) {
			t.Errorf("тег после ответа %+v, want как до ответа %+v", got, want)
		}
	})

	t.Run("отклонённая цитата тег не меняет", func(t *testing.T) {
		lit, _ := newTestLiterary(t)
		baseline := senseTags(t, chunk, dicts, layer, cfg)

		slot := testkit.NewFakeSlot()
		slot.SetResponse(deepread.DisputeSlotRole, `{"value":"строение","quote":"цитата не из окна","confidence":0.9}`)
		logger, counter := newDisputeJournal(t)
		outcomes, err := deepread.AskSenseDisputes(lit, senseKey("rejected"), questions,
			deepread.DisputeConfig{Budget: 5, Batch: 5}, slot, logger, senseNow)
		if err != nil {
			t.Fatalf("AskSenseDisputes: %v", err)
		}
		if len(outcomes) != 1 || outcomes[0].State != deepread.DisputeFailed {
			t.Fatalf("состояние вопроса = %+v, want failed", outcomes)
		}
		// Провал проверки — повтор не более двух раз: всего три вызова и события.
		if got := slot.Calls(); got != 3 {
			t.Errorf("вызовов слота %d, want 3", got)
		}
		if got := counter.get(); got != 3 {
			t.Errorf("событий deepread.slot %d, want 3", got)
		}

		tagged := senseTags(t, chunk, deepread.SenseDictionaries(dicts, outcomes), layer, cfg)
		if got, want := senseTag(t, tagged, deepread.KeyArchitecture),
			senseTag(t, baseline, deepread.KeyArchitecture); !sameTag(got, want) {
			t.Errorf("тег после отказа %+v, want консервативный %+v", got, want)
		}
	})
}

// TestSenseDisputeBudget — гейт бюджета спорных смыслов (T-15-04, шаги 2–4):
// пакет вопросов режется общим deepread.slot_budget, споры сверх бюджета
// остаются uncertain, повторный прогон с тем же ключом пересчёта вопросов не
// повторяет и возвращает записанные ответы.
func TestSenseDisputeBudget(t *testing.T) {
	dir := sensePack(t)
	cfg := deepread.DefaultSalienceConfig()
	layer, err := deepread.LoadSenseLayer(dir, cfg)
	if err != nil {
		t.Fatalf("LoadSenseLayer: %v", err)
	}
	catalog, err := deepread.LoadSenseCatalog(dir)
	if err != nil {
		t.Fatalf("LoadSenseCatalog: %v", err)
	}
	chunks := []deepread.SalienceChunk{
		senseTextChunk(21, "Замок у реки.", "Замок", "река"),
		senseTextChunk(22, "Замок у реки.", "Замок", "река"),
		senseTextChunk(23, "Замок у реки.", "Замок", "река"),
	}
	questions, err := deepread.BuildSenseDisputes(deepread.SenseDisputeInput{
		Chunks: chunks, Layer: layer, Catalog: catalog,
	})
	if err != nil {
		t.Fatalf("BuildSenseDisputes: %v", err)
	}
	if len(questions) != len(chunks) {
		t.Fatalf("вопросов %d, want %d", len(questions), len(chunks))
	}

	// Общий бюджет: смысловые вопросы отнимают место у прочих спорных.
	if rest := deepread.RemainingDisputeConfig(deepread.DisputeConfig{Budget: 20, Batch: 5}, len(questions)); rest.Budget != 17 {
		t.Errorf("остаток бюджета %d, want 17", rest.Budget)
	}

	lit, _ := newTestLiterary(t)
	key := senseKey("budget")
	slot := testkit.NewFakeSlot()
	slot.SetResponse(deepread.DisputeSlotRole, `{"value":"строение","quote":"Замок","confidence":0.8}`)
	logger, counter := newDisputeJournal(t)

	outcomes, err := deepread.AskSenseDisputes(lit, key, questions,
		deepread.DisputeConfig{Budget: 2, Batch: 1}, slot, logger, senseNow)
	if err != nil {
		t.Fatalf("AskSenseDisputes: %v", err)
	}
	if got := slot.Calls(); got != 2 {
		t.Errorf("вызовов слота %d, want 2 по бюджету", got)
	}
	if got := counter.get(); got != 2 {
		t.Errorf("событий deepread.slot %d, want 2", got)
	}

	answered, uncertain := 0, 0
	for _, out := range outcomes {
		switch out.State {
		case deepread.DisputeAnswered:
			answered++
		case deepread.DisputeUncertain:
			uncertain++
		}
	}
	if answered != 2 || uncertain != 1 {
		t.Errorf("ответов/без ответа = %d/%d, want 2/1", answered, uncertain)
	}

	// Повторный прогон с тем же ключом пересчёта вопросов не повторяет.
	again, err := deepread.AskSenseDisputes(lit, key, questions,
		deepread.DisputeConfig{Budget: 2, Batch: 1}, slot, logger, senseNow)
	if err != nil {
		t.Fatalf("AskSenseDisputes (повтор): %v", err)
	}
	if got := slot.Calls(); got != 2 {
		t.Errorf("повторных вызовов слота %d, want 2", got)
	}
	if len(again) != len(outcomes) {
		t.Fatalf("повтор: итогов %d, want %d", len(again), len(outcomes))
	}
	for i := range outcomes {
		if again[i].State != outcomes[i].State || again[i].Value != outcomes[i].Value ||
			again[i].Question.Key != outcomes[i].Question.Key {
			t.Errorf("повтор[%d] = %q/%q/%q, want %q/%q/%q", i,
				again[i].Question.Key, again[i].State, again[i].Value,
				outcomes[i].Question.Key, outcomes[i].State, outcomes[i].Value)
		}
	}
	if n := countRows(t, lit, `SELECT COUNT(*) FROM forge_log WHERE kind = ?`, deepread.KindSenseDispute); n != 1 {
		t.Errorf("строк forge_log kind=%q = %d, want 1", deepread.KindSenseDispute, n)
	}

	// Без слота вопросы не задаются и в журнал не пишутся.
	silent, _ := newTestLiterary(t)
	outcomes, err = deepread.AskSenseDisputes(silent, senseKey("no-slot"), questions,
		deepread.DisputeConfig{Budget: 2, Batch: 1}, nil, nil, senseNow)
	if err != nil {
		t.Fatalf("AskSenseDisputes без слота: %v", err)
	}
	for _, out := range outcomes {
		if out.State != deepread.DisputeUncertain || out.Attempts != 0 {
			t.Errorf("без слота: вопрос %q состояние %q, want uncertain без вызова", out.Question.Key, out.State)
		}
	}
	if n := countRows(t, silent, `SELECT COUNT(*) FROM forge_log WHERE kind = ?`, deepread.KindSenseDispute); n != 0 {
		t.Errorf("без слота: строк forge_log %d, want 0", n)
	}
}

// senseNow — время записей forge_log в тестах.
const senseNow = "2026-11-23T00:00:00Z"

// senseKey — ключ пересчёта прогона в тестах: имя варианта в текстовом хеше.
func senseKey(tag string) deepread.Key {
	return deepread.Key{TextHash: "text-" + tag, DictVersion: "0123456789ab", EmbedModel: "fake-embed"}
}

// sensePack пишет senses.json во временный каталог пака: омоним «замок» с двумя
// смыслами — «строение» (architecture) и «запирающее устройство» (вне категорий).
func sensePack(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	body := `{
  "version": 1,
  "senses": [
    {"lemma": "замок", "pos": "NOUN", "sense": "строение", "category": "architecture",
     "triggers": [{"lemma": "ров", "weight": 1.2}, {"lemma": "решётка", "weight": 1.0},
                  {"lemma": "рыцарь", "weight": 1.0}]},
    {"lemma": "замок", "pos": "NOUN", "sense": "запирающее устройство", "category": "",
     "triggers": [{"lemma": "скважина", "weight": 1.2}, {"lemma": "засов", "weight": 1.0},
                  {"lemma": "защёлка", "weight": 0.9}]}
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "senses.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("senses.json: %v", err)
	}
	return dir
}

// senseDicts — словари категорий прогона: «замок» в architecture, «ключ» в nature.
func senseDicts() map[string]*deepread.Dictionary {
	return map[string]*deepread.Dictionary{
		deepread.KeyArchitecture: {
			Key:    deepread.KeyArchitecture,
			Lemmas: map[string]int{"замок": 2},
		},
		deepread.KeyNature: {
			Key:    deepread.KeyNature,
			Lemmas: map[string]int{"ключ": 1},
		},
	}
}

// senseTextChunk собирает чанк из текста и форм токенов по порядку: лемма —
// нижний регистр формы, часть речи — NOUN, строка одна.
func senseTextChunk(id int64, text string, forms ...string) deepread.SalienceChunk {
	lemmas := make([]deepread.Lemma, len(forms))
	for i, form := range forms {
		lemmas[i] = deepread.Lemma{Form: form, Lemma: strings.ToLower(form), POS: "NOUN", Line: 1, Index: i}
	}
	return deepread.SalienceChunk{ID: id, Text: text, Lemmas: lemmas}
}

// senseQuestions строит очередь спорных смыслов чанка.
func senseQuestions(t *testing.T, chunk deepread.SalienceChunk, layer *deepread.SenseLayer,
	catalog deepread.SenseCatalog) []deepread.SenseQuestion {
	t.Helper()
	questions, err := deepread.BuildSenseDisputes(deepread.SenseDisputeInput{
		Chunks:  []deepread.SalienceChunk{chunk},
		Layer:   layer,
		Catalog: catalog,
	})
	if err != nil {
		t.Fatalf("BuildSenseDisputes: %v", err)
	}
	return questions
}

// senseTags считает теги чанка по словарям и смысловому слою.
func senseTags(t *testing.T, chunk deepread.SalienceChunk, dicts map[string]*deepread.Dictionary,
	layer *deepread.SenseLayer, cfg deepread.SalienceConfig) deepread.ChunkTags {
	t.Helper()
	tags, err := deepread.TagChunksWithSenses([]deepread.SalienceChunk{chunk}, dicts, layer, cfg)
	if err != nil {
		t.Fatalf("TagChunksWithSenses: %v", err)
	}
	return tags[0]
}
