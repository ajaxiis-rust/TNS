package selector_test

import (
	"testing"

	"tns65/internal/selector"
)

// TestSelector прогоняет таблицу решений Селектора (T-03-09, «Проверка»): на
// ровном участке — Челнок, на пике — Ткач; при конфликтной рекомендации
// исполняется решение Селектора; тест на каждое из семи правил даёт ожидаемый
// путь.
func TestSelector(t *testing.T) {
	cases := []struct {
		name string
		in   selector.Input
		want selector.Path
		rule selector.Rule
	}{
		{
			name: "1 стоп-кран прерывает ход",
			in:   selector.Input{Stop: true, Saw: selector.RecommendWeaver},
			want: selector.PathAbort,
			rule: selector.RuleStopCrane,
		},
		{
			name: "2 слеш-команда — служебный ответ",
			in:   selector.Input{Command: "/статус"},
			want: selector.PathCommand,
			rule: selector.RuleCommand,
		},
		{
			name: "3 жёсткое ограничение — страховка",
			in:   selector.Input{HardFallback: true, Saw: selector.RecommendWeaver},
			want: selector.PathFallback,
			rule: selector.RuleHardFallback,
		},
		{
			name: "4 повторяемое — Челнок",
			in:   selector.Input{Repeatable: true, Saw: selector.RecommendWeaver},
			want: selector.PathBank,
			rule: selector.RuleRepeatable,
		},
		{
			name: "5 эмоция или уникальность — Ткач",
			in:   selector.Input{Emotional: true, Saw: selector.RecommendBank},
			want: selector.PathWeaver,
			rule: selector.RuleEmotion,
		},
		{
			name: "6 пик — Ткач",
			in:   selector.Input{Saw: selector.RecommendWeaver},
			want: selector.PathWeaver,
			rule: selector.RuleSawRecommendation,
		},
		{
			name: "6 ровный участок — Челнок",
			in:   selector.Input{Saw: selector.RecommendBank},
			want: selector.PathBank,
			rule: selector.RuleSawRecommendation,
		},
		{
			name: "7 неопределённость — Челнок",
			in:   selector.Input{},
			want: selector.PathBank,
			rule: selector.RuleUncertain,
		},
		{
			name: "7 неопределённость при пустом банке — страховка",
			in:   selector.Input{BankEmpty: true},
			want: selector.PathFallback,
			rule: selector.RuleUncertain,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := selector.Select(c.in)
			if got.Path != c.want {
				t.Fatalf("Select() путь = %q, want %q", got.Path, c.want)
			}
			if got.Rule != c.rule {
				t.Fatalf("Select() правило = %v (%s), want %v (%s)",
					got.Rule, got.Rule.Name(), c.rule, c.rule.Name())
			}
		})
	}
}

// TestSelectorPriority проверяет порядок правил: первое сработавшее выигрывает,
// ниже по списку не проверяется (T-03-09, шаг 1).
func TestSelectorPriority(t *testing.T) {
	// Все сигналы сразу: побеждает правило 1 (стоп-кран).
	if got := selector.Select(selector.Input{
		Stop:         true,
		Command:      "/помощь",
		HardFallback: true,
		Repeatable:   true,
		Emotional:    true,
		Saw:          selector.RecommendWeaver,
	}); got.Rule != selector.RuleStopCrane {
		t.Fatalf("все сигналы сразу: правило = %v, want 1 (стоп-кран)", got.Rule)
	}

	// Конфликтная рекомендация: Пила советует Ткача, но сцена повторяема —
	// Селектор идёт Челноком (правило 4 раньше правила 6).
	got := selector.Select(selector.Input{Repeatable: true, Saw: selector.RecommendWeaver})
	if got.Path != selector.PathBank || got.Rule != selector.RuleRepeatable {
		t.Fatalf("конфликт (повторяемое + совет Ткача) = %q/%v, want bank/repeatable", got.Path, got.Rule)
	}

	// Конфликтная рекомендация: Пила советует банк, но реплика эмоциональна —
	// Селектор идёт Ткачом (правило 5 раньше правила 6).
	got = selector.Select(selector.Input{Emotional: true, Saw: selector.RecommendBank})
	if got.Path != selector.PathWeaver || got.Rule != selector.RuleEmotion {
		t.Fatalf("конфликт (эмоция + совет банка) = %q/%v, want weaver/emotion", got.Path, got.Rule)
	}

	// Жёсткое ограничение сильнее рекомендации Пилы.
	got = selector.Select(selector.Input{HardFallback: true, Saw: selector.RecommendBank})
	if got.Path != selector.PathFallback || got.Rule != selector.RuleHardFallback {
		t.Fatalf("жёсткое ограничение = %q/%v, want fallback/hard_fallback", got.Path, got.Rule)
	}
}

// TestSelectorEncode проверяет представление решения и код рекомендации для
// turns (T-03-09, шаг 2).
func TestSelectorEncode(t *testing.T) {
	d := selector.Decision{Path: selector.PathWeaver, Rule: selector.RuleSawRecommendation}
	if got := d.Encode(); got != "6 saw_recommendation" {
		t.Fatalf("Encode() = %q, want %q", got, "6 saw_recommendation")
	}

	if got := selector.RecommendationCode(selector.RecommendNone); got != 0 {
		t.Errorf("RecommendationCode(none) = %d, want 0", got)
	}
	if got := selector.RecommendationCode(selector.RecommendWeaver); got != 1 {
		t.Errorf("RecommendationCode(weaver) = %d, want 1", got)
	}
	if got := selector.RecommendationCode(selector.RecommendBank); got != 2 {
		t.Errorf("RecommendationCode(bank) = %d, want 2", got)
	}
}
