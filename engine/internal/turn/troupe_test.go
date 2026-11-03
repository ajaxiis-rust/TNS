package turn

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"tns65/internal/cast"
	"tns65/internal/db"
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

// newTestLiterary создаёт литературную базу мира для тестов описаний по имени.
func newTestLiterary(t *testing.T) *db.Handle {
	t.Helper()
	path := filepath.Join(t.TempDir(), "literary.db")
	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter(literary): %v", err)
	}
	if err := db.Migrate(raw, "literary"); err != nil {
		_ = raw.Close()
		t.Fatalf("Migrate(literary): %v", err)
	}
	_ = raw.Close()
	lit, err := db.OpenWriterAs(path, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	t.Cleanup(func() { _ = lit.Close() })

	res, err := lit.Exec("book",
		`INSERT INTO book (shelf_id, name, lang, genres, rating, chapters, status, created_at) VALUES ('hp-1997', 'Гарри Поттер', 'ru', 'фэнтези', '16+', 17, 'ready', '2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("INSERT book: %v", err)
	}
	if _, err := res.LastInsertId(); err != nil {
		t.Fatalf("book LastInsertId: %v", err)
	}
	return lit
}

// TestTroupeBookDescription — «Проверка» T-12-25 (пустая карточка по имени):
// пустая карточка отдаёт описание героя из книги, регистронезависимо.
func TestTroupeBookDescription(t *testing.T) {
	p, _ := newTestPipeline(t)
	lit := newTestLiterary(t)
	tr := NewTroupe(p.DB).WithLiterary(lit)

	if _, err := cast.InsertCharacter(lit, cast.BookCharacter{
		BookID:      1,
		Name:        "Снейп",
		Description: "мастер зелий",
		Status:      cast.StatusConfirmed,
	}); err != nil {
		t.Fatalf("InsertCharacter(Снейп): %v", err)
	}

	// Пустая карточка (без внешности) с именем героя книги.
	cardJSON, err := json.Marshal(card{})
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	if _, err := p.DB.Exec("characters",
		`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES (10, 'Снейп', 'ENTJ', 'купец', ?)`,
		string(cardJSON)); err != nil {
		t.Fatalf("insert characters: %v", err)
	}

	npc, err := tr.Spawn(10)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if npc.Group != "мастер зелий" {
		t.Errorf("внешность пустой карточки = %q, want «мастер зелий»", npc.Group)
	}

	if d, ok := tr.BookDescription("снейп"); !ok || d != "мастер зелий" {
		t.Errorf("BookDescription(снейп) = (%q, %v), want («мастер зелий», true)", d, ok)
	}
	if _, ok := tr.BookDescription("Гарри"); ok {
		t.Errorf("BookDescription(Гарри) нашёл отсутствующего героя")
	}
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
