package structural

import (
	"math"
	"path/filepath"
	"testing"
)

func TestScoreComposition(t *testing.T) {
	t.Run("золотой пример (1e-9)", func(t *testing.T) {
		c := Candidate{
			Composition: Composition{
				FamilyPrimary:    "escape_liberation",
				PatternSecondary: "rescue",
				Technique:        "slow_revelation",
				MoralStance:      "redemptive",
				Inversion:        true,
				Mood:             "somber",
			},
			FamilyKeywords: []string{"escape", "flee", "deliver"},
			TrustLevel:     2,
			UsageChapter:   1,
		}
		q := Query{
			Keywords:         []string{"escape", "flee"},
			Secondary:        "rescue",
			Techniques:       []string{"slow_revelation"},
			MoralStances:     []string{"redemptive"},
			Mood:             "somber",
			AnchorFamilies:   []string{"escape_liberation"},
			BreakExpectation: true,
		}
		got := Score(c, q, DefaultConfig())
		const want = 1.065
		if diff := math.Abs(got - want); diff > 1e-9 {
			t.Fatalf("Score = %.12f, want %.12f (diff %.3g)", got, want, diff)
		}
	})

	t.Run("жёсткое правило 1: primary обязателен", func(t *testing.T) {
		if err := (Composition{PatternSecondary: "rescue"}).Validate(); err == nil {
			t.Fatal("пустой family_primary не отвергнут")
		}
	})

	t.Run("жёсткое правило 2: secondary != primary", func(t *testing.T) {
		if err := (Composition{
			FamilyPrimary:    "escape_liberation",
			PatternSecondary: "escape_liberation",
		}).Validate(); err == nil {
			t.Fatal("secondary == primary не отвергнут")
		}
	})

	t.Run("жёсткое правило 3: family-only штраф при richer-кандидатах", func(t *testing.T) {
		c := Candidate{
			Composition:    Composition{FamilyPrimary: "loyalty"},
			FamilyKeywords: []string{"loyal"},
			TrustLevel:     2,
		}
		q := Query{Keywords: []string{"loyal"}}
		base := Score(c, q, DefaultConfig())
		q.HasRicher = true
		penalized := Score(c, q, DefaultConfig())
		if diff := math.Abs((base - penalized) - DefaultConfig().FamilyOnlyPenalty); diff > 1e-9 {
			t.Fatalf("штраф = %.12f, want %.12f", base-penalized, DefaultConfig().FamilyOnlyPenalty)
		}
	})

	t.Run("жёсткое правило 4: inversion boost только при флаге", func(t *testing.T) {
		plain := Candidate{
			Composition:    Composition{FamilyPrimary: "betrayal", PatternSecondary: "rescue"},
			FamilyKeywords: []string{"betray"},
			TrustLevel:     2,
		}
		inv := plain
		inv.Inversion = true

		noBreak := Score(inv, Query{Keywords: []string{"betray"}}, DefaultConfig())
		plainScore := Score(plain, Query{Keywords: []string{"betray"}}, DefaultConfig())
		if diff := math.Abs(noBreak - plainScore); diff > 1e-9 {
			t.Fatalf("inversion без флага слома ожидания дал бонус: %.12f", noBreak-plainScore)
		}
		withBreak := Score(inv, Query{Keywords: []string{"betray"}, BreakExpectation: true}, DefaultConfig())
		if diff := math.Abs((withBreak - noBreak) - DefaultConfig().InversionBoost); diff > 1e-9 {
			t.Fatalf("бонус = %.12f, want %.12f", withBreak-noBreak, DefaultConfig().InversionBoost)
		}
	})

	t.Run("инвариант весов: w_family ≤ 0.30", func(t *testing.T) {
		if _, err := parseCompositionSection("composition:\n  w_family: 0.30\n"); err != nil {
			t.Fatalf("w_family=0.30 должен пройти: %v", err)
		}
		if _, err := parseCompositionSection("composition:\n  w_family: 0.301\n"); err == nil {
			t.Fatal("w_family=0.301 не отвергнут")
		}
	})
}

func TestCompositionValidate(t *testing.T) {
	valid := Composition{
		FamilyPrimary:    "escape_liberation",
		PatternSecondary: "rescue",
		Technique:        "slow_revelation",
		MoralStance:      "redemptive",
		Inversion:        true,
		Mood:             "somber",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("валидная композиция не прошла: %v", err)
	}

	for _, tc := range []struct {
		name string
		c    Composition
	}{
		{"техника вне списка", Composition{FamilyPrimary: "escape_liberation", Technique: "explosion"}},
		{"моральная позиция вне списка", Composition{FamilyPrimary: "escape_liberation", MoralStance: "hopeful"}},
	} {
		if err := tc.c.Validate(); err == nil {
			t.Errorf("%s не отвергнута", tc.name)
		}
	}
}

func TestCompositionConfigLoaded(t *testing.T) {
	cfg, err := LoadCompositionConfig(filepath.Join("..", "..", "config.yaml"))
	if err != nil {
		t.Fatalf("LoadCompositionConfig(engine/config.yaml): %v", err)
	}
	want := DefaultConfig()
	if cfg != want {
		t.Fatalf("engine/config.yaml composition = %+v, want %+v", cfg, want)
	}
}
