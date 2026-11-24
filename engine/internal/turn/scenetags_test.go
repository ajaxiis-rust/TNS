package turn

import (
	"path/filepath"
	"testing"

	"tns65/internal/bank"
	"tns65/internal/lemmarpc"
)

// sceneTagPackDirForTest — каталог пакета ru от корня движка.
func sceneTagPackDirForTest(t *testing.T) string {
	t.Helper()
	return filepath.Join(engineRootForTest(t), "packs", "ru")
}

// TestSceneCategoryTags — ключи категорий словарей берутся из смыслов фразы:
// «лес» даёт nature, намерение остаётся отдельным тегом сцены.
func TestSceneCategoryTags(t *testing.T) {
	packDir := sceneTagPackDirForTest(t)
	st := &TurnState{
		Prepath: Prepath{Intent: Intent{Name: "восприятие"}, Mood: ""},
		Cut: Cut{Segments: []Segment{{
			Text: "Я иду по лесу к реке.",
			Tokens: []lemmarpc.Token{
				{Form: "иду", Lemma: "идти", Pos: "VERB"},
				{Form: "лесу", Lemma: "лес", Pos: "NOUN"},
				{Form: "реке", Lemma: "река", Pos: "NOUN"},
			},
		}}},
	}

	tags := sceneTagsFrom(packDir, st)
	if !containsString(tags, "nature") {
		t.Fatalf("sceneTags = %v, want ключ категории nature", tags)
	}
	if !containsString(tags, "восприятие") {
		t.Fatalf("sceneTags = %v, want тег намерения", tags)
	}
}

// TestBankPicksRecordByScenePhrase — реальная сцена находит запись банка по
// тегам категории фразы; без моста намерение «восприятие» с тегами
// book_chunk_tag не пересекается.
func TestBankPicksRecordByScenePhrase(t *testing.T) {
	f := newCompositionFixture(t)
	if _, err := f.litForge.Exec("bank_pool",
		`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at) VALUES
		 ('routine', 'Запись о лесе.', 'ru', 'nature', 2, 'forge', '2026-01-01T00:00:00Z'),
		 ('routine', 'Запись о стенах.', 'ru', 'architecture', 2, 'forge', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert bank_pool: %v", err)
	}

	st := &TurnState{
		Prepath: Prepath{Intent: Intent{Name: "восприятие"}},
		Cut: Cut{Segments: []Segment{{
			Text: "Я иду по лесу к реке.",
			Tokens: []lemmarpc.Token{
				{Form: "лесу", Lemma: "лес", Pos: "NOUN"},
				{Form: "реке", Lemma: "река", Pos: "NOUN"},
			},
		}}},
	}
	tags := sceneTagsFrom(sceneTagPackDirForTest(t), st)

	shuttle := bank.New(f.litReader, nil, nil, nil)
	rec, err := shuttle.Pick(tags)
	if err != nil {
		t.Fatalf("Pick(%v): %v", tags, err)
	}
	if rec.Text != "Запись о лесе." {
		t.Fatalf("выбрана запись %q, want «Запись о лесе.»", rec.Text)
	}
}
