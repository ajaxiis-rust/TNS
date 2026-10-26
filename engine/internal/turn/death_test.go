package turn

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestSecrets — тайны (T-03-19, «Проверка»): тайна назначена после рождения с
// флагами «намекать»/«раскрывать»; флаги при живом ходе не изменились.
func TestSecrets(t *testing.T) {
	t.Run("тайна назначена после рождения с флагами", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		s := NewSecrets(p.DB)

		sec, ok := SecretFor("переодетый принц")
		if !ok {
			t.Fatal("тайны «переодетый принц» нет в полотне")
		}
		if err := s.Assign(3, sec); err != nil {
			t.Fatalf("Assign: %v", err)
		}

		got, ok, err := s.Assigned(3)
		if err != nil {
			t.Fatalf("Assigned: %v", err)
		}
		if !ok {
			t.Fatal("тайна не назначена")
		}
		if got.Name != sec.Name {
			t.Fatalf("имя тайны = %q, want %q", got.Name, sec.Name)
		}
		if !got.Hint {
			t.Fatal("флаг «намекать» = false, want true")
		}
		if got.Reveal {
			t.Fatal("флаг «раскрывать» = true, want false")
		}
	})

	t.Run("флаги при живом ходе не изменились", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		s := NewSecrets(p.DB)

		sec, _ := SecretFor("переодетый принц")
		if err := s.Assign(3, sec); err != nil {
			t.Fatalf("Assign: %v", err)
		}

		cardJSON, err := json.Marshal(card{
			Archetype:  "Trickster",
			Voice:      "скрипучий",
			Appearance: "низкий, коренастый",
		})
		if err != nil {
			t.Fatalf("marshal card: %v", err)
		}
		if _, err := p.DB.Exec("characters",
			`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES (3, 'Иван', 'ENTJ', 'купец', ?)`,
			string(cardJSON)); err != nil {
			t.Fatalf("insert characters: %v", err)
		}

		st := &TurnState{
			Input: TurnInput{SessionID: 1, PlayerText: "Иван", At: time.Now()},
		}
		for _, stage := range []Stage{
			&troupeStage{db: p.DB},
			&secretStage{db: p.DB},
		} {
			if err := stage.Run(context.Background(), st); err != nil {
				t.Fatalf("%s: %v", stage.Name(), err)
			}
		}

		// Живой ход прочитал тайну в память, но флаги в базе не тронул.
		if len(st.Secrets) != 1 {
			t.Fatalf("тайн в памяти хода = %d, want 1", len(st.Secrets))
		}
		got, ok, err := s.Assigned(3)
		if err != nil {
			t.Fatalf("Assigned после хода: %v", err)
		}
		if !ok {
			t.Fatal("тайна пропала после живого хода")
		}
		if !got.Hint || got.Reveal {
			t.Fatalf("флаги после хода = намекать %v, раскрывать %v; want true, false", got.Hint, got.Reveal)
		}
	})
}

// TestDeathThreshold — порог смерти (T-03-19, «Проверка»): при здоровье героя 2%
// выдана сцена прощания; при здоровье выше порога смерть не навязана.
func TestDeathThreshold(t *testing.T) {
	t.Run("при здоровье 2% выдана сцена прощания", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		seedHeroHealth(t, p, 2)

		st := &TurnState{Reply: "обычный ответ"}
		if err := (&deathStage{db: p.DB}).Run(context.Background(), st); err != nil {
			t.Fatalf("death: %v", err)
		}
		if !st.Farewell {
			t.Fatal("сцена прощания не выдана")
		}
		if st.Reply != farewellReply {
			t.Fatalf("reply = %q, want %q", st.Reply, farewellReply)
		}
	})

	t.Run("при здоровье выше порога смерть не навязана", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		seedHeroHealth(t, p, 50)

		st := &TurnState{Reply: "обычный ответ"}
		if err := (&deathStage{db: p.DB}).Run(context.Background(), st); err != nil {
			t.Fatalf("death: %v", err)
		}
		if st.Farewell {
			t.Fatal("смерть навязана до порога")
		}
		if st.Reply != "обычный ответ" {
			t.Fatalf("reply изменён до порога: %q", st.Reply)
		}
	})

	t.Run("здоровье неизвестно — смерть не навязывается", func(t *testing.T) {
		p, _ := newTestPipeline(t)

		st := &TurnState{Reply: "обычный ответ"}
		if err := (&deathStage{db: p.DB}).Run(context.Background(), st); err != nil {
			t.Fatalf("death: %v", err)
		}
		if st.Farewell {
			t.Fatal("смерть навязана при неизвестном здоровье")
		}
	})
}

// seedHeroHealth сеет слой состояния героя в character_layers хэндлом core.
func seedHeroHealth(t *testing.T, p *Pipeline, health int) {
	t.Helper()
	raw, err := json.Marshal(heroStateLayer{Health: health})
	if err != nil {
		t.Fatalf("marshal hero health: %v", err)
	}
	if _, err := p.DB.Exec("character_layers",
		`INSERT INTO character_layers (character_id, layer, payload_json) VALUES (?, ?, ?)`,
		heroCharacterID, stateLayer, string(raw)); err != nil {
		t.Fatalf("insert hero health: %v", err)
	}
}
