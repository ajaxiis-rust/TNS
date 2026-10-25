package turn

import (
	"context"
	"strings"
	"testing"
	"time"

	"tns65/internal/lemmarpc"
	"tns65/internal/models"
)

// constEmbed — эмбеддер, возвращающий один и тот же единичный вектор: косинус
// между любыми двумя текстами равен 1, поэтому векторный голос всегда согласен.
type constEmbed struct{}

func (constEmbed) Embed(_ context.Context, _ string) ([]float32, error) {
	v := make([]float32, models.EmbedDim)
	v[0] = 1
	return v, nil
}

// wordLemmatizer — лемматизатор по словам: лемма — слово в нижнем регистре.
type wordLemmatizer struct{}

func (wordLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error) {
	var toks []lemmarpc.Token
	for _, w := range strings.Fields(text) {
		toks = append(toks, lemmarpc.Token{Form: w, Lemma: strings.ToLower(w)})
	}
	return toks, nil
}

// stopCraneStages — стадии конвейера в порядке реестра (T-03-01) с подставными
// лексиконом, эмбеддером и лемматизатором: так ход проходит резку, предпуть,
// подтверждение и стоп-кран коммита.
func stopCraneStages(p *Pipeline, lex *Lexicon, roster Roster, cfg ConfirmConfig) []Stage {
	return []Stage{
		&cutStage{logger: p.Logger},
		&prepathStage{db: p.DB, lexicon: lex, binder: NewSlotBinder(roster)},
		&confirmStage{Config: cfg},
		&selectStage{},
		&executeStage{},
		&safetyStage{},
		&commitStage{db: p.DB},
	}
}

func runStopCrane(t *testing.T, p *Pipeline, lex *Lexicon, roster Roster, cfg ConfirmConfig, text string) error {
	t.Helper()
	st := &TurnState{
		Input:      TurnInput{SessionID: 7, PlayerText: text, At: time.Now()},
		Embedder:   constEmbed{},
		Lemmatizer: wordLemmatizer{},
	}
	for _, s := range stopCraneStages(p, lex, roster, cfg) {
		if err := s.Run(context.Background(), st); err != nil {
			return err
		}
	}
	return nil
}

func stopCraneLexicon(t *testing.T) *Lexicon {
	t.Helper()
	dir := t.TempDir()
	writeLexicon(t, dir)
	lex, err := LoadLexicon(dir)
	if err != nil {
		t.Fatalf("LoadLexicon: %v", err)
	}
	return lex
}

func tableCount(t *testing.T, p *Pipeline, table string) int {
	t.Helper()
	var n int
	if err := p.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("COUNT(%s): %v", table, err)
	}
	return n
}

func TestStopCrane(t *testing.T) {
	lex := stopCraneLexicon(t)
	roster := Roster{
		NPCs:   []NPC{{ID: 1, Name: "Иван", Role: "кузнец", Group: "ремесленник"}},
		Places: []Place{{ID: 10, Name: "город"}},
	}
	cfg := DefaultConfirmConfig()

	t.Run("неподтверждённый ход не пишет", func(t *testing.T) {
		p, _ := newTestPipeline(t)

		// Реплика без лемм гипотезы: dict_match = 0 — подтверждения нет.
		if err := runStopCrane(t, p, lex, roster, cfg, "привет"); err != nil {
			t.Fatalf("run: %v", err)
		}

		for _, table := range []string{"turns", "scenes", "events", "vectors", "token_bindings", "character_state"} {
			if n := tableCount(t, p, table); n != 0 {
				t.Fatalf("%s: %d строк до подтверждения, want 0", table, n)
			}
		}
	})

	t.Run("правка сообщения даёт новую реплику", func(t *testing.T) {
		p, _ := newTestPipeline(t)

		// Прежняя реплика не подтверждена — следов не оставляет.
		if err := runStopCrane(t, p, lex, roster, cfg, "привет"); err != nil {
			t.Fatalf("run(прежняя): %v", err)
		}
		// Правка — новая реплика: полная перевекторизация, стоп-кран прежний.
		if err := runStopCrane(t, p, lex, roster, cfg, "идти ехать плыть бежать Иван в город"); err != nil {
			t.Fatalf("run(правка): %v", err)
		}

		if n := tableCount(t, p, "turns"); n != 1 {
			t.Fatalf("строк в turns = %d, want 1 (только новая реплика)", n)
		}
		var old, new int
		_ = p.DB.QueryRow(`SELECT COUNT(*) FROM turns WHERE player_text = ?`, "привет").Scan(&old)
		_ = p.DB.QueryRow(`SELECT COUNT(*) FROM turns WHERE player_text = ?`, "идти ехать плыть бежать Иван в город").Scan(&new)
		if old != 0 {
			t.Fatalf("следов прежней реплики = %d, want 0", old)
		}
		if new != 1 {
			t.Fatalf("следов новой реплики = %d, want 1", new)
		}

		// Подтверждённый ход зафиксирован во всех шести таблицах.
		for _, table := range []string{"scenes", "events", "character_state"} {
			if n := tableCount(t, p, table); n != 1 {
				t.Fatalf("%s: %d строк, want 1", table, n)
			}
		}
		if n := tableCount(t, p, "vectors"); n == 0 {
			t.Fatalf("vectors пуста после подтверждённого хода")
		}
		if n := tableCount(t, p, "token_bindings"); n == 0 {
			t.Fatalf("token_bindings пуста после подтверждённого хода")
		}
	})

	t.Run("отказ на второй таблице откатывает первую", func(t *testing.T) {
		p, _ := newTestPipeline(t)

		whole, _ := constEmbed{}.Embed(context.Background(), "x")
		st := &TurnState{
			Input:    TurnInput{SessionID: 7, PlayerText: "идти ехать плыть", At: time.Now()},
			Reply:    fallbackReply,
			Path:     "fallback",
			Confirm:  Confirmation{Decision: DecisionConfirmed},
			Prepath:  Prepath{Frame: SceneFrame{HeroPlaceID: 0}, Mood: "", LexiconLoaded: true},
			Embedder: constEmbed{},
			Cut:      Cut{Whole: whole},
		}

		stage := &commitStage{db: p.DB, FailTable: "scenes"}
		if err := stage.Run(context.Background(), st); err == nil {
			t.Fatal("отказ на scenes не вернул ошибку")
		}

		// Откат: строк первой таблицы (turns) не осталось.
		if n := tableCount(t, p, "turns"); n != 0 {
			t.Fatalf("строк turns после отката = %d, want 0", n)
		}
		if n := tableCount(t, p, "scenes"); n != 0 {
			t.Fatalf("строк scenes после отката = %d, want 0", n)
		}
	})
}
