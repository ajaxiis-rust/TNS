package sediment_test

import (
	"errors"
	"strings"
	"testing"

	"tns65/internal/sediment"
)

func TestClean(t *testing.T) {
	t.Run("полная сцена очищается в слоты", func(t *testing.T) {
		scene := sediment.Scene{
			ID:     42,
			Status: sediment.SceneStatusClosed,
			Prose:  "Иван купил меч за 30 монет в городе. Иван снова упомянул меч и город.",
			Facts: []sediment.Fact{
				{Token: sediment.TokenThing, Value: "меч"},
				{Token: sediment.TokenPrice, Value: "30 монет"},
			},
		}
		bindings := []sediment.Binding{
			{Token: "[NPCname]", Value: "Иван"},
			{Token: "[PLACE]", Value: "город"},
		}

		rec, err := sediment.Clean(scene, bindings)
		if err != nil {
			t.Fatalf("Clean: %v", err)
		}

		if rec.SourceSceneID != scene.ID {
			t.Fatalf("SourceSceneID = %d, want %d", rec.SourceSceneID, scene.ID)
		}

		for _, tok := range []string{"{NAME}", "{THING}", "{PRICE}", "{PLACE}"} {
			if !strings.Contains(rec.CleanedText, tok) {
				t.Errorf("cleaned_text не содержит слот-токен %q: %q", tok, rec.CleanedText)
			}
		}
		for _, value := range []string{"Иван", "меч", "30 монет", "город"} {
			if strings.Contains(rec.CleanedText, value) {
				t.Errorf("cleaned_text содержит значение %q: %q", value, rec.CleanedText)
			}
		}

		// Каждое значение заменено во всех вхождениях.
		for _, want := range []struct {
			token string
			count int
		}{
			{"{NAME}", 2},
			{"{THING}", 2},
			{"{PRICE}", 1},
			{"{PLACE}", 2},
		} {
			if got := strings.Count(rec.CleanedText, want.token); got != want.count {
				t.Errorf("%s встречается %d раз, want %d: %q", want.token, got, want.count, rec.CleanedText)
			}
		}

		// Структура сцены, действия и реплики остаются.
		for _, word := range []string{"купил", "снова упомянул", "за", "в"} {
			if !strings.Contains(rec.CleanedText, word) {
				t.Errorf("каркас сцены потерян: нет %q в %q", word, rec.CleanedText)
			}
		}
	})

	t.Run("значение вне привязок и фактов не заменяется", func(t *testing.T) {
		scene := sediment.Scene{
			ID:     1,
			Status: sediment.SceneStatusClosed,
			Prose:  "Иван купил меч.",
		}
		bindings := []sediment.Binding{
			{Token: "[NPCname]", Value: "Иван"},
		}

		rec, err := sediment.Clean(scene, bindings)
		if err != nil {
			t.Fatalf("Clean: %v", err)
		}
		if strings.Contains(rec.CleanedText, "Иван") {
			t.Errorf("значение из привязки не заменено: %q", rec.CleanedText)
		}
		// «меч» не в привязках и не в фактах — заменяться не должен.
		if !strings.Contains(rec.CleanedText, "меч") {
			t.Errorf("значение вне привязок и фактов заменено: %q", rec.CleanedText)
		}
	})

	t.Run("незавершённая сцена возвращает ErrNotFullScene", func(t *testing.T) {
		scene := sediment.Scene{
			ID:     1,
			Status: "open",
			Prose:  "Иван пошёл к реке.",
		}

		_, err := sediment.Clean(scene, nil)
		if !errors.Is(err, sediment.ErrNotFullScene) {
			t.Fatalf("Clean незавершённой сцены = %v, want ErrNotFullScene", err)
		}
	})
}
