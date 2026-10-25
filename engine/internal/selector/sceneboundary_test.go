package selector_test

import (
	"testing"

	"tns65/internal/selector"
)

func TestSceneBoundary(t *testing.T) {
	t.Run("три признака границы", func(t *testing.T) {
		d := selector.NewBoundaryDetector()
		base := selector.SceneSign{Place: 10, Time: 100, Participants: []int64{1, 2}}

		if b := d.Check(base); b.Changed {
			t.Fatalf("первый ход — не граница, получено %+v", b)
		}

		// Место: сдвиг положения кодом (T-03-15).
		if b := d.Check(selector.SceneSign{Place: 11, Time: 100, Participants: []int64{1, 2}}); !b.Changed || b.Reason != selector.ReasonPlace {
			t.Errorf("смена места: %+v", b)
		}

		// Глагол состояния: набор участников тот же — границы нет.
		if b := d.Check(selector.SceneSign{Place: 11, Time: 100, Participants: []int64{2, 1}}); b.Changed {
			t.Errorf("глагол состояния дал границу: %+v", b)
		}

		// Время: детектор времени (T-03-12).
		if b := d.Check(selector.SceneSign{Place: 11, Time: 200, Participants: []int64{1, 2}}); !b.Changed || b.Reason != selector.ReasonTime {
			t.Errorf("смена времени: %+v", b)
		}

		// Участники: новый деятель.
		if b := d.Check(selector.SceneSign{Place: 11, Time: 200, Participants: []int64{1, 2, 3}}); !b.Changed || b.Reason != selector.ReasonParticipants {
			t.Errorf("смена участников: %+v", b)
		}

		// Место: намерение «перемещение» из лексикона намерений (цитата).
		if b := d.Check(selector.SceneSign{Place: 11, Time: 200, Participants: []int64{1, 2, 3}, Move: "идти"}); !b.Changed || b.Reason != selector.ReasonPlace {
			t.Errorf("намерение перемещения: %+v", b)
		}
	})

	t.Run("граница обнуляет арку, Пила не обнуляется", func(t *testing.T) {
		h := openWorldCore(t, t.TempDir())
		cfg := selector.DefaultSawConfig()

		// Переменная Пилы по нити: K=0, три шага — 0.03 × 3 = 0.09.
		saw := selector.NewSaw(selector.Profile{"extraversion": -1, "rationality": -1}, cfg, 0, h, nil)
		for i := 0; i < 3; i++ {
			if _, err := saw.Step(1, selector.MoodNeutral); err != nil {
				t.Fatal(err)
			}
		}

		arc := selector.NewSceneArc(5, h)
		if err := arc.Grow(0.4); err != nil {
			t.Fatal(err)
		}
		if !near(arc.Value(), 0.4, 1e-9) {
			t.Fatalf("арка до границы = %v, want 0.4", arc.Value())
		}

		d := selector.NewBoundaryDetector()
		_ = d.Check(selector.SceneSign{Place: 10, Time: 100, Participants: []int64{1}})

		// Смена места — граница; арка обнуляется и растёт заново.
		if b := d.Check(selector.SceneSign{Place: 11, Time: 100, Participants: []int64{1}}); !b.Changed {
			t.Fatal("смена места не зафиксировала границу")
		}
		if err := arc.Reset(); err != nil {
			t.Fatal(err)
		}
		if arc.Value() != 0 {
			t.Errorf("арка после границы = %v, want 0", arc.Value())
		}

		// Переменная Пилы по нити границей не обнуляется.
		if !near(saw.ThreadValue(1), 0.09, 1e-9) {
			t.Errorf("нить Пилы после границы = %v, want 0.09", saw.ThreadValue(1))
		}

		// Арка растёт заново и пишется отдельной строкой scene_arc.
		if err := arc.Grow(0.2); err != nil {
			t.Fatal(err)
		}
		if !near(arc.Value(), 0.2, 1e-9) {
			t.Errorf("арка после роста = %v, want 0.2", arc.Value())
		}
		var v float64
		if err := h.QueryRow(`SELECT value FROM scene_arc WHERE scene_id = 5`).Scan(&v); err != nil {
			t.Fatalf("scene_arc query: %v", err)
		}
		if !near(v, 0.2, 1e-9) {
			t.Errorf("scene_arc.value = %v, want 0.2", v)
		}
	})
}
