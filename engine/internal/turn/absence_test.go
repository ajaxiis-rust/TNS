package turn

import (
	"encoding/json"
	"testing"
	"time"

	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// TestAbsence — пауза без заморозки (T-03-13, «Проверка»): прогон паузы на
// подставных часах без LLM-вызовов продвигает векторы судьбы дальнего круга и
// срабатывает отложенные события; следы обсчётов видны в world(<slug>).db как
// события с признаком offline; число LLM-вызовов 0.
func TestAbsence(t *testing.T) {
	t.Run("пауза продвигает векторы судьбы и срабатывает отложенные события", func(t *testing.T) {
		p, _ := newTestPipeline(t)

		// Дальний круг: три лица без встреч в памяти мира.
		for _, id := range []int64{1, 2, 3} {
			if _, err := p.DB.Exec("characters",
				`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES (?, '', 'INTJ', 'крестьянин', '{}')`,
				id); err != nil {
				t.Fatalf("insert characters %d: %v", id, err)
			}
		}

		// Отложенное событие с дедлайном в прошлом и одно в будущем.
		if _, err := p.DB.Exec("deferred_events",
			`INSERT INTO deferred_events (kind, payload_json, due_day, status, created_at) VALUES (?, ?, ?, ?, ?)`,
			"corridor_stop", `{}`, 2000, deferredPending, "2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("insert deferred past: %v", err)
		}
		if _, err := p.DB.Exec("deferred_events",
			`INSERT INTO deferred_events (kind, payload_json, due_day, status, created_at) VALUES (?, ?, ?, ?, ?)`,
			"corridor_stop", `{}`, 999999, deferredPending, "2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("insert deferred future: %v", err)
		}

		// Подставной слот — доказательство «без LLM»: вызовов быть не должно.
		slot := testkit.NewFakeSlot()
		workshop.SetSlot(slot)
		defer workshop.SetSlot(nil)

		clock := testkit.NewClock()
		lastActive := clock.Now()
		clock.Advance(3 * 24 * time.Hour) // три дня отсутствия.

		rep, err := advanceAbsence(p.DB, clock, lastActive)
		if err != nil {
			t.Fatalf("advanceAbsence: %v", err)
		}

		// Три просвета: верх ранга каждый просвет, остальные ротацией.
		if rep.FatesAdvanced != 6 {
			t.Fatalf("продвинуто векторов судьбы = %d, want 6", rep.FatesAdvanced)
		}
		if rep.DeferredFired != 1 {
			t.Fatalf("сработало отложенных событий = %d, want 1", rep.DeferredFired)
		}

		// Следы обсчётов — события с признаком offline (D-08.4).
		var offline int
		if err := p.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE kind = ?`, fateEventKind).Scan(&offline); err != nil {
			t.Fatalf("COUNT(fate_vector): %v", err)
		}
		if offline != rep.FatesAdvanced {
			t.Fatalf("событий fate_vector = %d, want %d", offline, rep.FatesAdvanced)
		}

		var raw string
		if err := p.DB.QueryRow(`SELECT payload_json FROM events WHERE kind = ? ORDER BY id LIMIT 1`, fateEventKind).Scan(&raw); err != nil {
			t.Fatalf("fate_vector payload: %v", err)
		}
		var fc fateCommit
		if err := json.Unmarshal([]byte(raw), &fc); err != nil {
			t.Fatalf("fate payload: %v", err)
		}
		if !fc.Offline {
			t.Fatal("признак offline отсутствует в событии вектора судьбы")
		}
		if fc.Fate.CharacterID == 0 {
			t.Fatal("вектор судьбы без character_id")
		}

		// Состояние NPC вне ближнего круга не фиксируется (LT-12.3).
		var stateRows int
		if err := p.DB.QueryRow(`SELECT COUNT(*) FROM character_state WHERE character_id IN (1, 2, 3)`).Scan(&stateRows); err != nil {
			t.Fatalf("COUNT(character_state): %v", err)
		}
		if stateRows != 0 {
			t.Fatalf("состояние дальнего круга зафиксировано: %d строк, want 0", stateRows)
		}

		// Отложенное событие сработало, будущее — нет.
		var pastStatus string
		if err := p.DB.QueryRow(`SELECT status FROM deferred_events WHERE due_day = 2000`).Scan(&pastStatus); err != nil {
			t.Fatalf("deferred past query: %v", err)
		}
		if pastStatus != deferredFired {
			t.Fatalf("статус сработавшего = %q, want %q", pastStatus, deferredFired)
		}
		var futureStatus string
		if err := p.DB.QueryRow(`SELECT status FROM deferred_events WHERE due_day = 999999`).Scan(&futureStatus); err != nil {
			t.Fatalf("deferred future query: %v", err)
		}
		if futureStatus != deferredPending {
			t.Fatalf("статус будущего = %q, want %q", futureStatus, deferredPending)
		}

		// Без LLM: слот не вызывался.
		if slot.Calls() != 0 {
			t.Fatalf("вызовов слота = %d, want 0", slot.Calls())
		}
	})

	t.Run("короткое отсутствие даёт один просвет", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		if _, err := p.DB.Exec("characters",
			`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES (5, '', 'INTJ', 'крестьянин', '{}')`); err != nil {
			t.Fatalf("insert characters: %v", err)
		}

		clock := testkit.NewClock()
		lastActive := clock.Now()
		clock.Advance(30 * time.Minute)

		rep, err := advanceAbsence(p.DB, clock, lastActive)
		if err != nil {
			t.Fatalf("advanceAbsence: %v", err)
		}
		// Один просвет: один NPC — верх ранга, ротации нет (кроме него никого).
		if rep.FatesAdvanced != 1 {
			t.Fatalf("продвинуто векторов судьбы = %d, want 1", rep.FatesAdvanced)
		}
	})

	t.Run("нет отсутствия — обсчётов нет", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		clock := testkit.NewClock()
		rep, err := advanceAbsence(p.DB, clock, clock.Now())
		if err != nil {
			t.Fatalf("advanceAbsence: %v", err)
		}
		if rep.FatesAdvanced != 0 || rep.DeferredFired != 0 {
			t.Fatalf("пустая пауза дала обсчёты: %+v", rep)
		}
	})
}
