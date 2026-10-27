package paganel_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/paganel"
	"tns65/internal/testkit"
)

// corridorStopsCount — число отложенных событий corridor_stop в базе.
func corridorStopsCount(t *testing.T, h *db.Handle) int {
	t.Helper()
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM deferred_events WHERE kind = 'corridor_stop'`).Scan(&n); err != nil {
		t.Fatalf("COUNT corridor_stop: %v", err)
	}
	return n
}

// readCorridor читает строку corridors по id.
func readCorridor(t *testing.T, h *db.Handle, id int64) (waypoints, stops string, promised float64, status string) {
	t.Helper()
	if err := h.QueryRow(
		`SELECT waypoints_json, stops_json, promised_days, status FROM corridors WHERE id = ?`, id).
		Scan(&waypoints, &stops, &promised, &status); err != nil {
		t.Fatalf("SELECT corridor: %v", err)
	}
	return waypoints, stops, promised, status
}

// TestCorridor — коридор и достройка: отдельный бюджет (T-06-09, «Проверка»):
// коридор записан в corridors с опорными точками, каждая остановка — строка
// deferred_events с kind='corridor_stop'; исчерпание бюджета разведки не
// останавливает достройку кольца; исчерпание бюджета достройки не тратит
// вызовы разведки.
func TestCorridor(t *testing.T) {
	t.Run("коридор записан с опорными точками, остановки в deferred_events", func(t *testing.T) {
		path := newRingWorld(t)
		core := openRingCore(t, path)

		plan := paganel.CorridorPlan{
			RingID:      1,
			FromPlaceID: 1,
			ToPlaceID:   2,
			Waypoints: []paganel.Waypoint{
				{Name: "Лес", Day: 2},
				{Name: "Город", Day: 5},
			},
			Stops: []paganel.Stop{
				{Name: "Лес", Day: 2},
				{Name: "Город", Day: 5},
			},
			PromisedDays: 6,
			StartMin:     1000,
		}

		c, err := paganel.RecordCorridor(core, plan)
		if err != nil {
			t.Fatalf("RecordCorridor: %v", err)
		}
		if c.ID == 0 {
			t.Fatal("id коридора = 0")
		}

		waypoints, stops, promised, status := readCorridor(t, core, c.ID)
		if status != paganel.CorridorQueued {
			t.Fatalf("статус = %q, want %q", status, paganel.CorridorQueued)
		}
		if promised != 6 {
			t.Fatalf("promised_days = %v, want 6", promised)
		}

		var wpts []paganel.Waypoint
		if err := json.Unmarshal([]byte(waypoints), &wpts); err != nil {
			t.Fatalf("waypoints_json: %v", err)
		}
		if len(wpts) != 2 || wpts[0].Name != "Лес" || wpts[1].Name != "Город" {
			t.Fatalf("опорные точки = %+v, want [Лес, Город]", wpts)
		}
		var st []paganel.Stop
		if err := json.Unmarshal([]byte(stops), &st); err != nil {
			t.Fatalf("stops_json: %v", err)
		}
		if len(st) != 2 {
			t.Fatalf("остановок = %d, want 2", len(st))
		}

		// Каждая остановка — строка deferred_events с kind='corridor_stop'.
		if n := corridorStopsCount(t, core); n != 2 {
			t.Fatalf("deferred_events corridor_stop = %d, want 2", n)
		}
		var kind string
		var dueDay int64
		if err := core.QueryRow(
			`SELECT kind, due_day FROM deferred_events WHERE kind = 'corridor_stop' ORDER BY due_day LIMIT 1`).
			Scan(&kind, &dueDay); err != nil {
			t.Fatalf("deferred stop: %v", err)
		}
		if kind != "corridor_stop" {
			t.Fatalf("kind = %q, want corridor_stop", kind)
		}
		if dueDay != 1000+2*1440 {
			t.Fatalf("due_day = %d, want %d", dueDay, 1000+2*1440)
		}
	})

	t.Run("маршрут длиннее обещания отклоняется", func(t *testing.T) {
		path := newRingWorld(t)
		core := openRingCore(t, path)

		plan := paganel.CorridorPlan{
			RingID:       1,
			FromPlaceID:  1,
			ToPlaceID:    2,
			Waypoints:    []paganel.Waypoint{{Name: "Город", Day: 5}},
			Stops:        []paganel.Stop{{Name: "Город", Day: 5}},
			PromisedDays: 4,
		}
		if _, err := paganel.RecordCorridor(core, plan); err == nil {
			t.Fatal("RecordCorridor с обещанием меньше суммы дней = nil, want error")
		}
	})

	t.Run("исчерпание бюджета разведки не останавливает достройку кольца", func(t *testing.T) {
		path := newRingWorld(t)
		seedHeroForge(t, path, 0, 0)
		forge := openRingForge(t, path)

		slot := testkit.NewFakeSlot()
		slot.SetResponse(paganel.ForgeRole, forgeOutputJSON(
			[]paganel.ForgedPlace{
				{Name: "Лес", Essence: "древний лес", Owner: "", Prominence: "известное", Bearing: "север", DistanceDays: 2},
			},
			nil,
		))
		paganel.SetSlot(slot)
		defer paganel.SetSlot(nil)
		paganel.SetCensor(fakeCensor{})
		defer paganel.SetCensor(nil)

		// Бюджет разведки исчерпан (лимит 0), достройка — в запасе.
		budget := paganel.NewBudget(paganel.BudgetLimits{Scout: 0, Build: 10})
		if budget.ScoutSpend() {
			t.Fatal("ScoutSpend при лимите 0 = true, want false")
		}

		if _, err := paganel.BuildRing(forge, budget); err != nil {
			t.Fatalf("BuildRing при исчерпанной разведке: %v", err)
		}
		if slot.Calls() != 1 {
			t.Fatalf("вызовов ковки = %d, want 1", slot.Calls())
		}
		if budget.BuildSpent() != 1 || budget.ScoutSpent() != 0 {
			t.Fatalf("счётчики = (build %d, scout %d), want (1, 0)", budget.BuildSpent(), budget.ScoutSpent())
		}
	})

	t.Run("исчерпание бюджета достройки не тратит вызовы разведки", func(t *testing.T) {
		path := newRingWorld(t)
		seedHeroForge(t, path, 0, 0)
		forge := openRingForge(t, path)

		slot := testkit.NewFakeSlot()
		slot.SetResponse(paganel.ForgeRole, forgeOutputJSON(nil, nil))
		paganel.SetSlot(slot)
		defer paganel.SetSlot(nil)
		paganel.SetCensor(fakeCensor{})
		defer paganel.SetCensor(nil)

		// Бюджет достройки исчерпан (лимит 0), разведка — в запасе.
		budget := paganel.NewBudget(paganel.BudgetLimits{Scout: 5, Build: 0})

		if _, err := paganel.BuildRing(forge, budget); !errors.Is(err, paganel.ErrBuildBudgetExhausted) {
			t.Fatalf("BuildRing при исчерпанной достройке = %v, want ErrBuildBudgetExhausted", err)
		}
		if slot.Calls() != 0 {
			t.Fatalf("вызовов ковки = %d, want 0", slot.Calls())
		}
		if budget.ScoutSpent() != 0 {
			t.Fatalf("вызовов разведки = %d, want 0 (счётчики независимы)", budget.ScoutSpent())
		}
	})

	t.Run("бюджет достройки читается из config.yaml", func(t *testing.T) {
		n, err := paganel.ReadBuildBudget(filepath.Join("..", "..", "config.yaml"))
		if err != nil {
			t.Fatalf("ReadBuildBudget(engine/config.yaml): %v", err)
		}
		if n != paganel.BuildBudgetDefault {
			t.Fatalf("build_budget = %d, want %d", n, paganel.BuildBudgetDefault)
		}
	})
}
