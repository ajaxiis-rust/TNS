package turn

import (
	"context"
	"strings"
	"testing"
	"time"

	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// runDigest прогоняет возврат и стадию сводки/вестей с подставными часами и
// моментом последней активности; возвращает состояние хода для проверки.
func runDigest(t *testing.T, p *Pipeline, clock ClockSource, lastActive time.Time, reply string) *TurnState {
	t.Helper()
	st := &TurnState{
		Input: TurnInput{SessionID: 1, PlayerText: "Я вернулся.", At: time.Now()},
		Reply: reply,
	}
	for _, s := range []Stage{
		&returnStage{db: p.DB, clock: clock, lastActive: lastActive},
		&digestStage{db: p.DB},
	} {
		if err := s.Run(context.Background(), st); err != nil {
			t.Fatalf("%s: %v", s.Name(), err)
		}
	}
	return st
}

func insertDigestCharacter(t *testing.T, p *Pipeline, id int64) {
	t.Helper()
	if _, err := p.DB.Exec("characters",
		`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES (?, '', 'INTJ', 'крестьянин', '{}')`,
		id); err != nil {
		t.Fatalf("insert characters %d: %v", id, err)
	}
}

// TestDigest — разделение сводки и вести (T-03-14, «Проверка»): при возврате
// после ночного отсутствия выдана сводка; при коротком отсутствии без отложенных
// событий сводка не формируется, накопленное приходит вестями; число LLM-вызовов 0.
func TestDigest(t *testing.T) {
	t.Run("стадия digest в реестре", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		found := false
		for _, s := range p.stages() {
			if s.Name() == "digest" {
				found = true
			}
		}
		if !found {
			t.Fatal("в реестре стадий нет digest")
		}
	})

	t.Run("ночное отсутствие формирует сводку", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		insertDigestCharacter(t, p, 1)

		slot := testkit.NewFakeSlot()
		workshop.SetSlot(slot)
		defer workshop.SetSlot(nil)

		clock := testkit.NewClock()
		lastActive := clock.Now()
		clock.Advance(9 * time.Hour) // дольше порога ночи (8 часов).

		st := runDigest(t, p, clock, lastActive, "Он шагает навстречу.")

		if st.DigestSummary == "" {
			t.Fatal("сводка не сформирована после ночного отсутствия")
		}
		if !strings.HasPrefix(st.Reply, st.DigestSummary) {
			t.Fatalf("сводка не легла перед ответом: %q", st.Reply)
		}

		var n int
		if err := p.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE kind = ?`, digestEventKind).Scan(&n); err != nil {
			t.Fatalf("COUNT(digest): %v", err)
		}
		if n != 1 {
			t.Fatalf("событий digest = %d, want 1", n)
		}

		if slot.Calls() != 0 {
			t.Fatalf("вызовов слота = %d, want 0", slot.Calls())
		}
	})

	t.Run("короткое отсутствие без отложенных событий не формирует сводку", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		insertDigestCharacter(t, p, 1)

		slot := testkit.NewFakeSlot()
		workshop.SetSlot(slot)
		defer workshop.SetSlot(nil)

		clock := testkit.NewClock()
		lastActive := clock.Now()
		clock.Advance(30 * time.Minute) // короче порога ночи, отложенных нет.

		st := runDigest(t, p, clock, lastActive, "Он шагает навстречу.")

		if st.DigestSummary != "" {
			t.Fatalf("сводка сформирована при коротком отсутствии без событий: %q", st.DigestSummary)
		}
		if len(st.DigestNews) == 0 {
			t.Fatal("накопленное не пришло вестями")
		}

		var d int
		if err := p.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE kind = ?`, digestEventKind).Scan(&d); err != nil {
			t.Fatalf("COUNT(digest): %v", err)
		}
		if d != 0 {
			t.Fatalf("событий digest = %d, want 0", d)
		}

		var news int
		if err := p.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE kind = ?`, newsEventKind).Scan(&news); err != nil {
			t.Fatalf("COUNT(news): %v", err)
		}
		if news == 0 {
			t.Fatal("вестей в events нет")
		}

		if slot.Calls() != 0 {
			t.Fatalf("вызовов слота = %d, want 0", slot.Calls())
		}
	})

	t.Run("сработавшее отложенное событие формирует сводку и без ночного порога", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		insertDigestCharacter(t, p, 1)
		if _, err := p.DB.Exec("deferred_events",
			`INSERT INTO deferred_events (kind, payload_json, due_day, status, created_at) VALUES (?, ?, ?, ?, ?)`,
			"corridor_stop", `{}`, 5, deferredPending, "2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("insert deferred: %v", err)
		}

		clock := testkit.NewClock()
		lastActive := clock.Now()
		clock.Advance(30 * time.Minute)

		st := runDigest(t, p, clock, lastActive, "Он шагает навстречу.")

		if st.DigestSummary == "" {
			t.Fatal("сводка не сформирована при сработавшем отложенном событии")
		}
	})
}
