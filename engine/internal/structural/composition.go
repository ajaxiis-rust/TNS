package structural

import (
	"fmt"
	"strings"
)

// Composition — основная единица выбора драмы (SCR-§2.2): primary-семья
// обязательна, остальные поля — желательные слагаемые композиции.
type Composition struct {
	FamilyPrimary    string
	PatternSecondary string
	Technique        string
	MoralStance      string
	Inversion        bool
	Mood             string
}

// Techniques — канонический закрытый список техник (SCR-§2.3, 10 значений).
// Значение вне списка — ошибка валидации; маппинга «к ближайшему» нет.
var Techniques = []string{
	"slow_revelation",
	"sudden_reversal",
	"procedural_pressure",
	"intimate_psychological",
	"social_panorama",
	"ironic_distance",
	"tragic_inevitability",
	"redemptive_arc",
	"oracular_declaration",
	"didactic_exhortation",
}

// MoralStances — канонический закрытый список моральных позиций (SCR-§2.3,
// 6 значений). Значение вне списка — ошибка валидации.
var MoralStances = []string{
	"redemptive",
	"tragic",
	"ironic",
	"ambivalent",
	"cynical",
	"moralistic",
}

// ValidTechnique сообщает, входит ли technique в закрытый список.
func ValidTechnique(s string) bool {
	return containsStr(Techniques, s)
}

// ValidMoralStance сообщает, входит ли moral_stance в закрытый список.
func ValidMoralStance(s string) bool {
	return containsStr(MoralStances, s)
}

// Validate проверяет жёсткие правила SCR-§3.2 для одной композиции:
// primary обязателен (правило 1), secondary ≠ primary (правило 2), technique и
// moral_stance — только из закрытых списков.
func (c Composition) Validate() error {
	if strings.TrimSpace(c.FamilyPrimary) == "" {
		return fmt.Errorf("structural: family_primary не указан")
	}
	if c.PatternSecondary != "" && c.PatternSecondary == c.FamilyPrimary {
		return fmt.Errorf("structural: pattern_secondary %q совпадает с family_primary", c.PatternSecondary)
	}
	if c.Technique != "" && !ValidTechnique(c.Technique) {
		return fmt.Errorf("structural: техника %q вне списка", c.Technique)
	}
	if c.MoralStance != "" && !ValidMoralStance(c.MoralStance) {
		return fmt.Errorf("structural: моральная позиция %q вне списка", c.MoralStance)
	}
	return nil
}

// containsStr — вхождение строки в список.
func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
