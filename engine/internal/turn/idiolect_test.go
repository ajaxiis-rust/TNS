package turn

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tns65/internal/testkit"
)

// sceneChunk отдаёт готовую сцену одним чанком закрытого канала.
func sceneChunk(text string) chan string {
	ch := make(chan string, 1)
	ch <- text
	close(ch)
	return ch
}

// TestIdiolect — идиолект лица в труппе (B-15, T-19-08): лицо получает словарь,
// привычки и табу в теги, идиолект попадает в промпт сцены, а словарный гейт не
// пускает табу в прямую речь.
func TestIdiolect(t *testing.T) {
	const characterID = 4

	p, _ := newTestPipeline(t)
	tr := NewTroupe(p.DB)

	cardJSON, err := json.Marshal(card{
		Archetype: "Trickster",
		Voice:     "скрипучий",
		Idiolect: idiolect{
			Lexicon: []string{"скрип", "колодец"},
			Habits:  []string{"кряхтит, начиная говорить"},
			Taboo:   []string{"смерт"},
		},
	})
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	if _, err := p.DB.Exec("characters",
		`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES (?, 'Старик', 'ENTJ', 'крестьянин', ?)`,
		characterID, string(cardJSON)); err != nil {
		t.Fatalf("insert characters: %v", err)
	}

	wantTags := []string{
		"лексикон:скрип",
		"лексикон:колодец",
		"привычка:кряхтит, начиная говорить",
		"табу:смерт",
	}

	t.Run("лицо получает идиолект в теги", func(t *testing.T) {
		npc, err := tr.Spawn(characterID)
		if err != nil {
			t.Fatalf("Spawn: %v", err)
		}
		for _, want := range wantTags {
			if !containsTag(npc.Tags, want) {
				t.Fatalf("в тегах нет %q: %v", want, npc.Tags)
			}
		}
	})

	t.Run("ростер пробрасывает идиолект", func(t *testing.T) {
		roster, err := tr.LoadRoster()
		if err != nil {
			t.Fatalf("LoadRoster: %v", err)
		}
		var found bool
		for _, npc := range roster.NPCs {
			if npc.ID != characterID {
				continue
			}
			found = true
			for _, want := range wantTags {
				if !containsTag(npc.Tags, want) {
					t.Fatalf("в тегах ростера нет %q: %v", want, npc.Tags)
				}
			}
		}
		if !found {
			t.Fatalf("лицо %d не попало в ростер", characterID)
		}
	})

	t.Run("идиолект в промпте сцены", func(t *testing.T) {
		roster := Roster{NPCs: []NPC{{
			ID:   characterID,
			Name: "Старик",
			Tags: []string{"скрипучий", "лексикон:скрип", "привычка:кряхтит, начиная говорить", "табу:смерт"},
		}}}
		binder := NewSlotBinder(roster)
		prompt, _ := binder.Bind(promptTemplate(PreselectWeaver),
			binder.DetectMentions("Я подхожу к колодцу и спрашиваю Старика."), 0, 0)
		want := "Идиолект: лексикон — скрип; привычки — кряхтит, начиная говорить; табу — смерт."
		if !strings.Contains(prompt, want) {
			t.Fatalf("промпт сцены без идиолекта: %q", prompt)
		}
		if strings.Contains(prompt, "[") || strings.Contains(prompt, "]") {
			t.Fatalf("промпт содержит токен-заменитель: %q", prompt)
		}

		// Лицо без идиолекта оставляет промпт чистым: токен гаснет как незакрытый.
		plain := NewSlotBinder(Roster{NPCs: []NPC{{ID: 5, Name: "Гаврила"}}})
		prompt, _ = plain.Bind(promptTemplate(PreselectWeaver), plain.DetectMentions("Гаврила ждёт."), 0, 0)
		if strings.Contains(prompt, "Идиолект") || strings.Contains(prompt, "[") {
			t.Fatalf("промпт лица без идиолекта: %q", prompt)
		}
	})

	t.Run("табу не попадает в прямую речь", func(t *testing.T) {
		tags := idiolectTags(idiolect{
			Lexicon: []string{"скрип"},
			Habits:  []string{"кряхтит, начиная говорить"},
			Taboo:   []string{"смерт"},
		})
		if SpeechAllowed(tags, "— Смерти боишься, парень?") {
			t.Fatal("табу-слово в реплике, но гейт пропустил")
		}
		if !SpeechAllowed(tags, "— Смотри, колодец замёрз.") {
			t.Fatal("чистая реплика отклонена словарным гейтом")
		}
		if !SpeechAllowed(nil, "— Смерти боишься?") {
			t.Fatal("лицо без идиолекта отклонено словарным гейтом")
		}
	})

	t.Run("ход не доставляет реплику с табу", func(t *testing.T) {
		roster := Roster{NPCs: []NPC{{
			ID:   characterID,
			Name: "Старик",
			Tags: []string{"лексикон:скрип", "табу:смерт"},
		}}}
		scene := "Старик поднимает глаза.\n" +
			"— Смерти боишься, парень? — спрашивает он.\n" +
			"— Колодец замёрз, — отвечает он."
		stage := &executeStage{weaver: &fakeProseSlot{ch: sceneChunk(scene)}}
		st := &TurnState{Path: pathWeaver, Roster: &roster, Prepath: Prepath{Prompt: "Старик смотрит."}}

		reply, degraded := stage.weaverReply(context.Background(), st)
		if degraded {
			t.Fatalf("живой ответ слота помечен деградацией: %q", reply)
		}
		if strings.Contains(reply, "Смерти") {
			t.Fatalf("табу-основа дошла до игрока в прямой речи: %q", reply)
		}
		if !strings.Contains(reply, "Колодец замёрз") {
			t.Fatalf("чистая реплика лица потеряна: %q", reply)
		}
	})

	t.Run("сцена вести видит идиолект лица", func(t *testing.T) {
		roster := &Roster{NPCs: []NPC{
			{ID: heroCharacterID, Name: "Герой"},
			{ID: characterID, Name: "Старик",
				Tags: []string{"лексикон:скрип", "привычка:кряхтит, начиная говорить", "табу:смерт"}},
		}}
		prompt := newsScenePrompt("Весть: колодец замёрз.", roster)
		want := "Идиолект: лексикон — скрип; привычки — кряхтит, начиная говорить; табу — смерт."
		if !strings.Contains(prompt, want) {
			t.Fatalf("промпт сцены вести без идиолекта лица: %q", prompt)
		}
		if !strings.Contains(prompt, "Старик") {
			t.Fatalf("промпт сцены вести без имени лица: %q", prompt)
		}
		if got := newsScenePrompt("Весть.", &Roster{NPCs: []NPC{{ID: characterID, Name: "Старик"}}}); strings.Contains(got, "Идиолект") {
			t.Fatalf("промпт лица без идиолекта: %q", got)
		}
	})

	t.Run("сцена вести не доставляет реплику с табу", func(t *testing.T) {
		const tabooProse = "Гаврила снимает шапку у ворот.\n" +
			"— Смерти боишься, герой? — спрашивает он.\n" +
			"— Колодец замёрз, — добавляет он."

		p, _ := newTestPipeline(t)
		insertDigestCharacter(t, p, 1)
		insertDigestCharacter(t, p, 2)
		slot := testkit.NewFakeSlot()
		slot.SetResponse(newsSceneRole, tabooProse)

		clock := testkit.NewClock()
		lastActive := clock.Now()
		clock.Advance(30 * time.Minute) // короткое отсутствие: сводки нет, вести есть.
		st := &TurnState{Input: TurnInput{SessionID: 1, PlayerText: "Я вернулся.", At: time.Now()}, Reply: "Он шагает навстречу."}
		if err := (&returnStage{db: p.DB, clock: clock, lastActive: lastActive}).Run(context.Background(), st); err != nil {
			t.Fatalf("return: %v", err)
		}
		st.Roster = &Roster{NPCs: []NPC{{ID: 2, Name: "Гаврила", Tags: []string{"лексикон:скрип", "табу:смерт"}}}}
		ds := &digestStage{db: p.DB, scene: slot, sceneCfg: DefaultNewsSceneConfig()}
		if err := ds.Run(context.Background(), st); err != nil {
			t.Fatalf("digest: %v", err)
		}

		if st.DigestScene.SceneID == 0 {
			t.Fatal("сцена вести не создана: гейт не должен отменять чистую реплику")
		}
		if strings.Contains(st.Reply, "Смерти") {
			t.Fatalf("табу-основа дошла до игрока в сцене вести: %q", st.Reply)
		}
		if !strings.Contains(st.Reply, "Колодец замёрз") {
			t.Fatalf("чистая реплика лица потеряна в сцене вести: %q", st.Reply)
		}
		var prose string
		if err := p.DB.QueryRow(`SELECT prose FROM scenes WHERE id = ?`, st.DigestScene.SceneID).Scan(&prose); err != nil {
			t.Fatalf("SELECT scenes: %v", err)
		}
		if strings.Contains(prose, "Смерти") {
			t.Fatalf("табу-основа записана в сцену: %q", prose)
		}
	})
}
