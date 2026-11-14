package turn

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// newsSceneProse — ответ слота с прямой речью: сцена вести с диалогом НПС.
const newsSceneProse = "— Слышал, что стряслось у дальнего круга?\n— Слышал.\n— Тогда иди и посмотри сам."

// runDigestScene прогоняет возврат и стадию сводки/вестей с заданным слотом
// сцены вестей; возвращает состояние хода для проверки.
func runDigestScene(t *testing.T, p *Pipeline, slot NewsSceneSlot, cfg NewsSceneConfig) *TurnState {
	t.Helper()
	clock := testkit.NewClock()
	lastActive := clock.Now()
	clock.Advance(30 * time.Minute) // короткое отсутствие: сводки нет, вести есть.

	st := &TurnState{
		Input: TurnInput{SessionID: 1, PlayerText: "Я вернулся.", At: time.Now()},
		Reply: "Он шагает навстречу.",
	}
	for _, s := range []Stage{
		&returnStage{db: p.DB, clock: clock, lastActive: lastActive},
		&digestStage{db: p.DB, scene: slot, sceneCfg: cfg},
	} {
		if err := s.Run(context.Background(), st); err != nil {
			t.Fatalf("%s: %v", s.Name(), err)
		}
	}
	return st
}

// countScenes считает строки scenes в оперативной базе мира.
func countScenes(t *testing.T, p *Pipeline) int {
	t.Helper()
	var n int
	if err := p.DB.QueryRow(`SELECT COUNT(*) FROM scenes`).Scan(&n); err != nil {
		t.Fatalf("COUNT(scenes): %v", err)
	}
	return n
}

// TestNewsSceneDelivery — весть-сцена (IMP-5): сцена создана штатным путём,
// диалог на месте, событие news сохранено.
func TestNewsSceneDelivery(t *testing.T) {
	p, _ := newTestPipeline(t)
	insertDigestCharacter(t, p, 1)
	insertDigestCharacter(t, p, 2)

	slot := testkit.NewFakeSlot()
	slot.SetResponse(newsSceneRole, newsSceneProse)

	st := runDigestScene(t, p, slot, DefaultNewsSceneConfig())

	// Две вести за возврат (два лица дальнего круга): сценой идёт последняя.
	if len(st.DigestNews) < 2 {
		t.Fatalf("вестей за возврат = %d, want не меньше 2", len(st.DigestNews))
	}
	if st.DigestScene.NewsIndex != len(st.DigestNews)-1 {
		t.Fatalf("NewsIndex = %d, want %d", st.DigestScene.NewsIndex, len(st.DigestNews)-1)
	}
	if st.DigestScene.SceneID == 0 {
		t.Fatal("сцена вести не записана: SceneID = 0")
	}
	if slot.Calls() != 1 {
		t.Fatalf("вызовов слота = %d, want 1 (только выбранная сцена)", slot.Calls())
	}

	var prose, status string
	if err := p.DB.QueryRow(`SELECT prose, status FROM scenes WHERE id = ?`, st.DigestScene.SceneID).
		Scan(&prose, &status); err != nil {
		t.Fatalf("SELECT scenes: %v", err)
	}
	if prose != newsSceneProse {
		t.Fatalf("prose сцены = %q, want %q", prose, newsSceneProse)
	}
	if !hasDirectSpeech(prose) {
		t.Fatalf("диалог в сцене не найден: %q", prose)
	}
	if status != sceneStatusOpen {
		t.Fatalf("status сцены = %q, want %q", status, sceneStatusOpen)
	}
	if n := countScenes(t, p); n != 1 {
		t.Fatalf("сцен в базе = %d, want 1", n)
	}

	// Весть остаётся в событиях журнала как раньше: тексты пачки не тронуты.
	var raw string
	if err := p.DB.QueryRow(`SELECT payload_json FROM events WHERE kind = ? ORDER BY id DESC LIMIT 1`,
		newsEventKind).Scan(&raw); err != nil {
		t.Fatalf("payload news: %v", err)
	}
	var batch struct {
		Texts []string `json:"texts"`
	}
	if err := json.Unmarshal([]byte(raw), &batch); err != nil {
		t.Fatalf("payload news: %v", err)
	}
	if strings.Join(batch.Texts, "\n") != strings.Join(st.DigestNews, "\n") {
		t.Fatalf("вести в events = %v, want %v", batch.Texts, st.DigestNews)
	}

	// Сцена подана игроку вместо текстовой строки этой вести.
	if !strings.Contains(st.Reply, newsSceneProse) {
		t.Fatalf("ответ не содержит сцену вести: %q", st.Reply)
	}
	if strings.Contains(st.Reply, st.DigestNews[len(st.DigestNews)-1]) {
		t.Fatalf("ответ содержит текстовую весть при сценовой подаче: %q", st.Reply)
	}
}

// TestNewsSceneDegradation — деградация (IMP-5): слот недоступен или дал брак —
// весть подаётся текстом, ход не валится.
func TestNewsSceneDegradation(t *testing.T) {
	cases := []struct {
		name string
		slot func() NewsSceneSlot
	}{
		{name: "слот не подключён", slot: func() NewsSceneSlot { return nil }},
		{name: "слот отказал", slot: func() NewsSceneSlot {
			s := testkit.NewFakeSlot()
			s.FailWith(errors.New("провайдер недоступен"))
			return s
		}},
		{name: "слот вернул прозу без диалога", slot: func() NewsSceneSlot {
			s := testkit.NewFakeSlot()
			s.SetResponse(newsSceneRole, "Мир изменился, а весть не дошла.")
			return s
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, _ := newTestPipeline(t)
			insertDigestCharacter(t, p, 1)
			insertDigestCharacter(t, p, 2)

			st := runDigestScene(t, p, c.slot(), DefaultNewsSceneConfig())

			if st.DigestScene.NewsIndex != -1 || st.DigestScene.SceneID != 0 {
				t.Fatalf("сцена при деградации: %+v", st.DigestScene)
			}
			if n := countScenes(t, p); n != 0 {
				t.Fatalf("сцен в базе = %d, want 0", n)
			}
			last := st.DigestNews[len(st.DigestNews)-1]
			if !strings.Contains(st.Reply, last) {
				t.Fatalf("ответ не содержит текстовую весть: %q", st.Reply)
			}
		})
	}
}

// TestNewsSceneChoiceDeterministic — выбор подачи детерминирован (IMP-5): повтор
// даёт ту же форму.
func TestNewsSceneChoiceDeterministic(t *testing.T) {
	cfg := DefaultNewsSceneConfig()
	news := []string{"Весть: дальний круг мира продвинулся (1).", "Весть: дальний круг мира продвинулся (2)."}

	first := sceneNewsIndex(news, cfg)
	if first != len(news)-1 {
		t.Fatalf("sceneNewsIndex = %d, want %d", first, len(news)-1)
	}
	if again := sceneNewsIndex(news, cfg); again != first {
		t.Fatalf("повторный выбор = %d, want %d", again, first)
	}
	if got := sceneNewsIndex(news[:1], cfg); got != -1 {
		t.Fatalf("одиночная весть подана сценой: index = %d, want -1", got)
	}
	if got := sceneNewsIndex(news, NewsSceneConfig{Enabled: false}); got != -1 {
		t.Fatalf("сцена выключена, но выбрана весть %d", got)
	}

	// Повтор прогона возврата: та же форма той же вести.
	p, _ := newTestPipeline(t)
	insertDigestCharacter(t, p, 1)
	insertDigestCharacter(t, p, 2)
	slot := testkit.NewFakeSlot()
	slot.SetResponse(newsSceneRole, newsSceneProse)
	one := runDigestScene(t, p, slot, cfg)
	two := runDigestScene(t, p, slot, cfg)
	if one.DigestScene.NewsIndex != two.DigestScene.NewsIndex {
		t.Fatalf("форма подачи изменилась между повторами: %d vs %d",
			one.DigestScene.NewsIndex, two.DigestScene.NewsIndex)
	}
}

// TestNewsScenePrompt — промпт сцены называет действующее лицо труппы и весть.
func TestNewsScenePrompt(t *testing.T) {
	roster := &Roster{NPCs: []NPC{
		{ID: heroCharacterID, Name: "Герой"},
		{ID: 2, Name: "Гаврила"},
	}}
	prompt := newsScenePrompt("Весть: беда у дальнего круга.", roster)
	for _, want := range []string{"Гаврила", "Весть: беда у дальнего круга."} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("промпт сцены без %q: %q", want, prompt)
		}
	}
	if strings.Contains(prompt, "Герой") {
		t.Fatalf("герой назван действующим лицом сцены: %q", prompt)
	}
	if got := newsScenePrompt("Весть.", nil); !strings.Contains(got, newsSceneNPCFallback) {
		t.Fatalf("промпт без труппы не подставил запасное лицо: %q", got)
	}
}

// TestNewsSceneRegistered — реестр стадий: digest подключён к слоту мастерской.
func TestNewsSceneRegistered(t *testing.T) {
	slot := testkit.NewFakeSlot()
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	p, _ := newTestPipeline(t)
	if workshop.GenerationSlot() != slot {
		t.Fatal("GenerationSlot не вернул подключённый слот мастерской")
	}
	for _, s := range p.stages() {
		if s.Name() != "digest" {
			continue
		}
		ds, ok := s.(*digestStage)
		if !ok {
			t.Fatalf("стадия digest не *digestStage: %T", s)
		}
		if ds.scene == nil {
			t.Fatal("стадия digest не подключена к слоту мастерской")
		}
		if !ds.sceneCfg.Enabled {
			t.Fatal("сценовая подача вестей выключена в реестре стадий")
		}
		return
	}
	t.Fatal("в реестре стадий нет digest")
}
