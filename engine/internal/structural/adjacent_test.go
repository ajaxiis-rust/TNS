package structural

import (
	"math"
	"path/filepath"
	"testing"
)

func TestAdjacentPairs(t *testing.T) {
	cfg := DefaultConfig()
	q := Query{Keywords: []string{"escape"}}

	adj := Candidate{
		Composition: Composition{
			FamilyPrimary:    "escape_liberation",
			PatternSecondary: "rescue",
			Technique:        "slow_revelation",
			MoralStance:      "redemptive",
		},
		FamilyKeywords: []string{"escape", "flee"},
		TrustLevel:     2,
	}

	t.Run("пара штрафуется", func(t *testing.T) {
		nonadj := adj
		nonadj.PatternSecondary = "loyalty"
		diff := Score(nonadj, q, cfg) - Score(adj, q, cfg)
		if math.Abs(diff-cfg.AdjacentPenalty) > 1e-9 {
			t.Fatalf("штраф близкой пары = %.12f, want %.12f", diff, cfg.AdjacentPenalty)
		}
	})

	t.Run("независимая пара не штрафуется", func(t *testing.T) {
		nonadj := adj
		nonadj.PatternSecondary = "loyalty"
		plain := Score(nonadj, q, cfg)
		withBreak := Score(nonadj, Query{Keywords: q.Keywords, BreakExpectation: true}, cfg)
		if diff := math.Abs(plain - withBreak); diff > 1e-9 {
			t.Fatalf("независимая пара получила штраф: разность %.12f", diff)
		}
	})

	t.Run("режим слома ожидания снимает штраф", func(t *testing.T) {
		plain := Score(adj, q, cfg)
		withBreak := Score(adj, Query{Keywords: q.Keywords, BreakExpectation: true}, cfg)
		if diff := math.Abs((withBreak - plain) - cfg.AdjacentPenalty); diff > 1e-9 {
			t.Fatalf("снятие штрафа = %.12f, want %.12f", withBreak-plain, cfg.AdjacentPenalty)
		}
	})

	t.Run("вторая пара wisdom_counsel/teaching_discourse", func(t *testing.T) {
		c := Candidate{
			Composition: Composition{
				FamilyPrimary:    "wisdom_counsel",
				PatternSecondary: "teaching_discourse",
			},
			FamilyKeywords: []string{"wisdom"},
			TrustLevel:     2,
		}
		nonadj := c
		nonadj.PatternSecondary = "loyalty"
		diff := Score(nonadj, q, cfg) - Score(c, q, cfg)
		if math.Abs(diff-cfg.AdjacentPenalty) > 1e-9 {
			t.Fatalf("штраф второй близкой пары = %.12f, want %.12f", diff, cfg.AdjacentPenalty)
		}
	})

	t.Run("IsAdjacentPair симметрична", func(t *testing.T) {
		if !IsAdjacentPair("escape_liberation", "rescue") || !IsAdjacentPair("rescue", "escape_liberation") {
			t.Fatal("пара escape_liberation/rescue не распознана")
		}
		if IsAdjacentPair("escape_liberation", "loyalty") || IsAdjacentPair("escape_liberation", "") {
			t.Fatal("независимая/пустая пара распознана как близкая")
		}
	})

	t.Run("хеш реестра стабилен", func(t *testing.T) {
		a, err := LoadFamilies(filepath.Join("testdata", "archetypes.json"))
		if err != nil {
			t.Fatalf("LoadFamilies: %v", err)
		}
		b, err := LoadFamilies(filepath.Join("testdata", "archetypes.json"))
		if err != nil {
			t.Fatalf("LoadFamilies: %v", err)
		}
		ha, hb := RegistrySHA256(a), RegistrySHA256(b)
		if ha == "" || ha != hb {
			t.Fatalf("хеш реестра нестабилен: %q vs %q", ha, hb)
		}
		if len(ha) != 64 {
			t.Fatalf("длина хеша %d, want 64 (sha256 hex)", len(ha))
		}
		// Смена словаря меняет отпечаток (T-10-13, шаг 3).
		mod := append([]Family(nil), a...)
		mod[0].GlossEN = "Changed"
		if RegistrySHA256(mod) == ha {
			t.Fatal("отпечаток не изменился при правке реестра")
		}
	})
}
