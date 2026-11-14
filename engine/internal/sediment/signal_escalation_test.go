package sediment_test

import (
	"testing"

	"tns65/internal/sediment"
)

func TestDislikeEscalation(t *testing.T) {
	t.Run("два дизлайка подряд — откат к сырому ходу", func(t *testing.T) {
		latest := []sediment.SignalKind{sediment.SignalDislike, sediment.SignalDislike}
		if streak := sediment.DislikeStreak(latest); streak != 2 {
			t.Fatalf("DislikeStreak = %d, want 2", streak)
		}
		if level := sediment.Escalation(2); level != sediment.EscalationRollback {
			t.Fatalf("Escalation(2) = %d, want %d", level, sediment.EscalationRollback)
		}
	})

	t.Run("первый дизлайк — мягкая регенерация", func(t *testing.T) {
		latest := []sediment.SignalKind{sediment.SignalDislike, sediment.SignalLike}
		if streak := sediment.DislikeStreak(latest); streak != 1 {
			t.Fatalf("DislikeStreak = %d, want 1", streak)
		}
		if level := sediment.Escalation(1); level != sediment.EscalationSoft {
			t.Fatalf("Escalation(1) = %d, want %d", level, sediment.EscalationSoft)
		}
	})

	t.Run("лайк или отсутствие сигнала обрывают серию", func(t *testing.T) {
		for _, latest := range [][]sediment.SignalKind{
			nil,
			{sediment.SignalLike},
			{sediment.SignalLike, sediment.SignalDislike},
			{""},
		} {
			if streak := sediment.DislikeStreak(latest); streak != 0 {
				t.Fatalf("DislikeStreak(%v) = %d, want 0", latest, streak)
			}
		}
		if level := sediment.Escalation(0); level != sediment.EscalationNone {
			t.Fatalf("Escalation(0) = %d, want %d", level, sediment.EscalationNone)
		}
	})

	t.Run("длинная серия не поднимает уровень выше отката", func(t *testing.T) {
		latest := []sediment.SignalKind{
			sediment.SignalDislike, sediment.SignalDislike, sediment.SignalDislike,
		}
		if level := sediment.Escalation(sediment.DislikeStreak(latest)); level != sediment.EscalationRollback {
			t.Fatalf("уровень длинной серии = %d, want %d", level, sediment.EscalationRollback)
		}
	})
}
