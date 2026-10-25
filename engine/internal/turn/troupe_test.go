package turn

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestTroupe(t *testing.T) {
	t.Run("новый NPC появляется без предыстории", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		tr := NewTroupe(p.DB)

		cardJSON, err := json.Marshal(card{
			Archetype:  "Trickster",
			Voice:      "скрипучий",
			Appearance: "низкий, коренастый",
		})
		if err != nil {
			t.Fatalf("marshal card: %v", err)
		}
		if _, err := p.DB.Exec("characters",
			`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES (3, '', 'ENTJ', 'купец', ?)`,
			string(cardJSON)); err != nil {
			t.Fatalf("insert characters: %v", err)
		}

		npc, err := tr.Spawn(3)
		if err != nil {
			t.Fatalf("Spawn: %v", err)
		}
		// Архетип, голос и внешность пришли из карточки; предыстории нет.
		if npc.Role != "Trickster" {
			t.Fatalf("архетип = %q, want Trickster", npc.Role)
		}
		if npc.Group != "низкий, коренастый" {
			t.Fatalf("внешность = %q, want %q", npc.Group, "низкий, коренастый")
		}
		if !containsTag(npc.Tags, "скрипучий") {
			t.Fatalf("в тегах нет голоса: %v", npc.Tags)
		}
		if npc.Backstory != "" {
			t.Fatalf("предыстория при первом появлении = %q, want пусто", npc.Backstory)
		}
	})

	t.Run("после встречи в memory_topics есть запись памяти реакций", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		tr := NewTroupe(p.DB)

		if err := tr.RecordMeeting(5, ReactionFed); err != nil {
			t.Fatalf("RecordMeeting: %v", err)
		}

		var n int
		if err := p.DB.QueryRow(`SELECT COUNT(*) FROM memory_topics WHERE character_id = 5`).Scan(&n); err != nil {
			t.Fatalf("COUNT(memory_topics): %v", err)
		}
		if n != 1 {
			t.Fatalf("записей памяти реакций = %d, want 1", n)
		}

		var theme string
		var weight float64
		if err := p.DB.QueryRow(`SELECT theme, weight FROM memory_topics WHERE character_id = 5`).
			Scan(&theme, &weight); err != nil {
			t.Fatalf("memory_topics query: %v", err)
		}
		if theme != ReactionFed {
			t.Fatalf("тема реакции = %q, want %q", theme, ReactionFed)
		}
		if weight != 1 {
			t.Fatalf("вес реакции = %v, want 1", weight)
		}
	})

	t.Run("при планировании нового элемента сначала проверен реестр ружей", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		tr := NewTroupe(p.DB)

		// Показанное ружьё зарегистрировано, но не заряжено.
		if err := tr.RegisterGun(gun{CharacterID: 7, Label: "крючок"}); err != nil {
			t.Fatalf("RegisterGun: %v", err)
		}

		// Планирование нового элемента сначала находит незаряженное ружьё и не
		// вводит новый элемент.
		g, err := tr.NextElement("")
		if err != nil {
			t.Fatalf("NextElement: %v", err)
		}
		if g.CharacterID != 7 {
			t.Fatalf("планирование вернуло character_id=%d, want 7 (незаряженное ружьё)", g.CharacterID)
		}
		if g.Label != "крючок" {
			t.Fatalf("планирование вернуло label=%q, want %q", g.Label, "крючок")
		}

		// Реестр по-прежнему содержит ровно одно ружьё.
		var n int
		if err := p.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE kind = ?`, gunKind).Scan(&n); err != nil {
			t.Fatalf("COUNT(events gun): %v", err)
		}
		if n != 1 {
			t.Fatalf("ружей в реестре = %d, want 1", n)
		}
	})
}

// containsTag проверяет, что тег встречается в списке.
func containsTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

// TestTroupeStage — труппа в реестре стадий (T-03-11, «Правится» stages.go):
// до предпути лиц мира появляется из карточек, после встречи названного лица в
// memory_topics остаётся запись памяти реакций.
func TestTroupeStage(t *testing.T) {
	p, _ := newTestPipeline(t)
	lex := stopCraneLexicon(t)
	cfg := DefaultConfirmConfig()

	cardJSON, err := json.Marshal(card{
		Archetype:  "Trickster",
		Voice:      "скрипучий",
		Appearance: "низкий, коренастый",
	})
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	if _, err := p.DB.Exec("characters",
		`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES (9, 'Иван', 'ENTJ', 'купец', ?)`,
		string(cardJSON)); err != nil {
		t.Fatalf("insert characters: %v", err)
	}

	stages := []Stage{
		&cutStage{logger: p.Logger},
		&troupeStage{db: p.DB},
		&prepathStage{db: p.DB, lexicon: lex},
		&confirmStage{Config: cfg},
		&selectStage{},
		&executeStage{},
		&safetyStage{},
		&commitStage{db: p.DB},
	}

	st := &TurnState{
		Input:      TurnInput{SessionID: 11, PlayerText: "идти ехать плыть бежать Иван", At: time.Now()},
		Embedder:   constEmbed{},
		Lemmatizer: wordLemmatizer{},
	}
	for _, s := range stages {
		if err := s.Run(context.Background(), st); err != nil {
			t.Fatalf("%s: %v", s.Name(), err)
		}
	}

	// Труппа наполнила биндер: предпуть увидел названное лицо и записал встречу.
	var n int
	if err := p.DB.QueryRow(`SELECT COUNT(*) FROM memory_topics WHERE character_id = 9`).Scan(&n); err != nil {
		t.Fatalf("COUNT(memory_topics): %v", err)
	}
	if n == 0 {
		t.Fatal("после встречи названного лица в memory_topics нет записи памяти реакций")
	}

	// Лицо появилось из карточки: привязка имени ушла в память хода до коммита.
	if !hasBinding(st.Bindings, TokenNPCName) {
		t.Fatalf("нет привязки %s: %+v", TokenNPCName, st.Bindings)
	}

	// Селектор не сломан труппой: решение и путь заполнены.
	if st.SelectorDecision == "" || st.Path == "" {
		t.Fatalf("селектор после труппы: decision=%q path=%q", st.SelectorDecision, st.Path)
	}
}
