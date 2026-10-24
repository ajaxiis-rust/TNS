package turn

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/testkit"
)

// writeLexicon создаёт во временном каталоге лексикон языкового пакета формата
// T-01-08: lexicon_intents.jsonl и detectors_mood.json.
func writeLexicon(t *testing.T, dir string) {
	t.Helper()
	intents := "{\"intent\":\"перемещение\",\"lemmas\":[\"идти\",\"ехать\",\"плыть\"],\"verbs\":[\"идти\",\"двигаться\"]}\n" +
		"{\"intent\":\"речь\",\"lemmas\":[\"говорить\",\"сказать\"],\"verbs\":[\"говорить\",\"спросить\"]}\n"
	if err := os.WriteFile(filepath.Join(dir, "lexicon_intents.jsonl"), []byte(intents), 0o644); err != nil {
		t.Fatal(err)
	}
	mood := "{\"mood\":\"тревога\",\"lemmas\":[\"бояться\",\"страх\",\"опасность\"]}\n"
	if err := os.WriteFile(filepath.Join(dir, "detectors_mood.json"), []byte(mood), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPrepath(t *testing.T) {
	p, _ := newTestPipeline(t)

	lexDir := t.TempDir()
	writeLexicon(t, lexDir)
	lex, err := LoadLexicon(lexDir)
	if err != nil {
		t.Fatalf("LoadLexicon: %v", err)
	}

	roster := Roster{
		NPCs: []NPC{
			{ID: 1, Name: "Иван", NameActive: "Ивана", NamePassive: "Иваном", Role: "кузнец", Group: "ремесленник"},
			{ID: 2, Name: "Судьба", Role: "судьба", Deity: true},
		},
		Places: []Place{{ID: 10, Name: "город"}},
	}

	embed := testkit.NewFakeEmbed()
	slot := testkit.NewFakeSlot()

	st := &TurnState{
		Input: TurnInput{SessionID: 1, PlayerText: "Иду в город к Ивану."},
		Cut: Cut{
			Segments: []Segment{{Text: "Иду в город к Ивану", Lemmas: []string{"идти", "город", "иван"}}},
		},
		Embedder: embed,
	}

	stage := &prepathStage{db: p.DB, lexicon: lex, binder: NewSlotBinder(roster)}
	if err := stage.Run(context.Background(), st); err != nil {
		t.Fatalf("prepath: %v", err)
	}

	// Гипотеза намерения взята из лексикона без LLM.
	if st.Prepath.Intent.Name != "перемещение" {
		t.Fatalf("намерение = %q, want перемещение", st.Prepath.Intent.Name)
	}
	if st.Prepath.Intent.Quote != "идти" {
		t.Fatalf("цитата намерения = %q, want идти", st.Prepath.Intent.Quote)
	}

	// Для реплики с именем и топонимом подготовлены привязки в памяти хода.
	if !hasBinding(st.Bindings, TokenNPCName) {
		t.Fatalf("нет привязки %s: %+v", TokenNPCName, st.Bindings)
	}
	if !hasBinding(st.Bindings, TokenPlace) {
		t.Fatalf("нет привязки %s: %+v", TokenPlace, st.Bindings)
	}

	// Промпт Ткача не содержит токенов-заменителей.
	if strings.Contains(st.Prepath.Prompt, "[") || strings.Contains(st.Prepath.Prompt, "]") {
		t.Fatalf("промпт содержит токен-заменитель: %q", st.Prepath.Prompt)
	}

	// Предвыбор завершился без внешних вызовов: ни слота генерации, ни эмбеддера.
	if slot.Calls() != 0 {
		t.Fatalf("вызовов слота генерации = %d, want 0", slot.Calls())
	}
	if embed.Calls() != 0 {
		t.Fatalf("вызовов эмбеддера в предпути = %d, want 0", embed.Calls())
	}
}

func TestSlotBinder(t *testing.T) {
	roster := Roster{
		NPCs: []NPC{
			{ID: 1, Name: "Иван", NameActive: "Ивана", NamePassive: "Иваном", Role: "кузнец", Group: "ремесленник"},
			{ID: 2, Name: "Судьба", Role: "судьба", Deity: true},
		},
		Places: []Place{{ID: 10, Name: "город"}},
	}
	b := NewSlotBinder(roster)

	mentions := b.DetectMentions("Иван пришёл в город")
	template := "[NPCname] кланяется. [NPCroleDeity] молчит."

	res, bindings := b.Bind(template, mentions, 0, 1)
	if strings.Contains(res, "[") || strings.Contains(res, "]") {
		t.Fatalf("закрытый текст содержит токен: %q", res)
	}

	// [NPCroleDeity] привязан к влиятельной сущности, не к смертному.
	deity := findBinding(bindings, TokenNPCRoleDeity)
	if deity == nil {
		t.Fatalf("нет привязки %s: %+v", TokenNPCRoleDeity, bindings)
	}
	if deity.CharacterID != 2 {
		t.Fatalf("%s привязан к character_id=%d, want 2 (не смертный)", TokenNPCRoleDeity, deity.CharacterID)
	}
	if deity.CharacterID == 1 {
		t.Fatalf("%s привязан к смертному Ивану", TokenNPCRoleDeity)
	}

	// Одинаковый токен в двух арках одной главы даёт две разные привязки.
	_, arcOne := b.Bind("[NPCname] здесь.", mentions, 1, 1)
	_, arcTwo := b.Bind("[NPCname] здесь.", mentions, 2, 1)
	n1 := findBinding(arcOne, TokenNPCName)
	n2 := findBinding(arcTwo, TokenNPCName)
	if n1 == nil || n2 == nil {
		t.Fatalf("нет привязки %s в арках: %+v / %+v", TokenNPCName, arcOne, arcTwo)
	}
	if n1.ID == n2.ID {
		t.Fatalf("одинаковый токен в двух арках дал один номер привязки: %d", n1.ID)
	}
}

func hasBinding(bindings []Binding, token string) bool {
	return findBinding(bindings, token) != nil
}

func findBinding(bindings []Binding, token string) *Binding {
	for i := range bindings {
		if bindings[i].Token == token {
			return &bindings[i]
		}
	}
	return nil
}
