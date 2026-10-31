package structural

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config — веса формулы скоринга и параметры жёстких правил SCR-§3.2. Значения
// по умолчанию совпадают с engine/config.yaml (секция composition) и SCR-§3.1.
type Config struct {
	WFamily           float64 // w_family: primary-семья.
	WSecondary        float64 // w_secondary: secondary-паттерн.
	WTechnique        float64 // w_technique: техника.
	WMoral            float64 // w_moral: моральная позиция.
	WMood             float64 // w_mood: настроение.
	WQuality          float64 // w_quality: trust_level/2 записи банка.
	WFreshness        float64 // w_freshness: свежесть семьи в главе.
	FamilyOnlyPenalty float64 // family_only_penalty: штраф family-only при richer-кандидатах.
	InversionBoost    float64 // inversion_boost: бонус слома ожидания.
	AdjacentPenalty   float64 // adjacent_penalty: штраф близкой пары primary+secondary.
}

// AdjacentPairs — канонические близкие пары семей (SCR-§3.2.5, SCR-§7-Этап 4):
// пара не должна часто выступать primary + secondary друг друга без явной
// причины. Состав пар — часть канона SCR, поэтому он в коде, а не в конфиге
// (T-10-13, шаг 1).
var AdjacentPairs = [][2]string{
	{"escape_liberation", "rescue"},
	{"wisdom_counsel", "teaching_discourse"},
}

// IsAdjacentPair сообщает, образуют ли primary и secondary близкую пару
// (порядок пары не важен).
func IsAdjacentPair(primary, secondary string) bool {
	if secondary == "" {
		return false
	}
	for _, p := range AdjacentPairs {
		if (primary == p[0] && secondary == p[1]) || (primary == p[1] && secondary == p[0]) {
			return true
		}
	}
	return false
}

// familyWeightLimit — инвариант SCR-§3.1: вес primary-семьи не выше порога.
const familyWeightLimit = 0.30

// DefaultConfig возвращает веса и параметры, совпадающие со значениями
// engine/config.yaml (секция composition).
func DefaultConfig() Config {
	return Config{
		WFamily:           0.28,
		WSecondary:        0.18,
		WTechnique:        0.15,
		WMoral:            0.12,
		WMood:             0.10,
		WQuality:          0.10,
		WFreshness:        0.07,
		FamilyOnlyPenalty: 0.10,
		InversionBoost:    0.10,
		AdjacentPenalty:   0.15,
	}
}

// LoadCompositionConfig читает секцию composition из config.yaml и проверяет
// инвариант w_family ≤ 0.30; нарушение — ошибка старта.
func LoadCompositionConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return parseCompositionSection(string(data))
}

// parseCompositionSection разбирает топ-ключ `composition:` со скалярными
// полями весов и параметров жёстких правил SCR-§3.2.
func parseCompositionSection(data string) (Config, error) {
	cfg := DefaultConfig()

	inComposition := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inComposition = trimmed == "composition:"
			continue
		}
		if !inComposition {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(key) {
		case "w_family":
			cfg.WFamily = f
		case "w_secondary":
			cfg.WSecondary = f
		case "w_technique":
			cfg.WTechnique = f
		case "w_moral":
			cfg.WMoral = f
		case "w_mood":
			cfg.WMood = f
		case "w_quality":
			cfg.WQuality = f
		case "w_freshness":
			cfg.WFreshness = f
		case "family_only_penalty":
			cfg.FamilyOnlyPenalty = f
		case "inversion_boost":
			cfg.InversionBoost = f
		case "adjacent_penalty":
			cfg.AdjacentPenalty = f
		}
	}
	if err := sc.Err(); err != nil {
		return Config{}, err
	}
	if cfg.WFamily > familyWeightLimit {
		return Config{}, fmt.Errorf("structural: w_family %.3f превышает порог %.2f (SCR-§3.1)", cfg.WFamily, familyWeightLimit)
	}
	return cfg, nil
}

// Candidate — скоринговая карточка записи банка: композиция и числа, из которых
// складываются слагаемые формулы.
type Candidate struct {
	Composition
	FamilyKeywords []string // ключевые слова primary-семьи (реестр archetypes.json).
	TrustLevel     int      // trust_level записи банка (0..2).
	UsageChapter   int      // family_usage_chapter[family] (T-10-10).
}

// Query — запрос живого хода, по которому оценивается кандидат.
type Query struct {
	Keywords         []string // ключевые слова запроса (намерение/настроение).
	Secondary        string   // семейное ожидание secondary (для secondary_match).
	Techniques       []string // ожидаемые техники (для technique_fit).
	MoralStances     []string // ожидаемые моральные позиции (для moral_stance_fit).
	Mood             string   // настроение запроса.
	AnchorFamilies   []string // anchor_families мира (T-10-06).
	BreakExpectation bool     // флаг «ломать ожидание» (для inversion_boost).
	HasRicher        bool     // в пуле есть richer-кандидаты (для family-only штрафа).
}

// Score — формула SCR-§3.1 и жёсткие правила SCR-§3.2: детерминированная
// сумма без LLM, чистые функции — записи нет. Принадлежность primary/secondary
// и закрытые списки проверяются отдельно через Composition.Validate.
func Score(c Candidate, q Query, cfg Config) float64 {
	cfg = withDefaults(cfg)

	score := cfg.WFamily*familyMatch(c, q) +
		cfg.WSecondary*secondaryMatch(c, q) +
		cfg.WTechnique*techniqueFit(c, q) +
		cfg.WMoral*moralStanceFit(c, q) +
		cfg.WMood*moodMatch(c, q) +
		cfg.WQuality*quality(c) +
		cfg.WFreshness*freshness(c)

	// SCR-§3.2 правило 3: family-only получает штраф при наличии richer-кандидатов.
	if familyOnly(c) && q.HasRicher {
		score -= cfg.FamilyOnlyPenalty
	}
	// SCR-§3.2 правило 4: слом ожидания повышает приоритет только по флагу.
	if c.Inversion && q.BreakExpectation {
		score += cfg.InversionBoost
	}
	// SCR-§3.2 правило 5 (T-10-13): близкая пара primary+secondary штрафуется;
	// «явная причина» = слом ожидания — тогда штраф не применяется.
	if IsAdjacentPair(c.FamilyPrimary, c.PatternSecondary) && !q.BreakExpectation {
		score -= cfg.AdjacentPenalty
	}
	return score
}

// withDefaults заполняет нулевые поля значениями SCR-§3.1/§3.2.
func withDefaults(cfg Config) Config {
	if cfg.WFamily <= 0 {
		cfg.WFamily = 0.28
	}
	if cfg.WSecondary <= 0 {
		cfg.WSecondary = 0.18
	}
	if cfg.WTechnique <= 0 {
		cfg.WTechnique = 0.15
	}
	if cfg.WMoral <= 0 {
		cfg.WMoral = 0.12
	}
	if cfg.WMood <= 0 {
		cfg.WMood = 0.10
	}
	if cfg.WQuality <= 0 {
		cfg.WQuality = 0.10
	}
	if cfg.WFreshness <= 0 {
		cfg.WFreshness = 0.07
	}
	if cfg.FamilyOnlyPenalty <= 0 {
		cfg.FamilyOnlyPenalty = 0.10
	}
	if cfg.InversionBoost <= 0 {
		cfg.InversionBoost = 0.10
	}
	if cfg.AdjacentPenalty <= 0 {
		cfg.AdjacentPenalty = 0.15
	}
	return cfg
}

// familyMatch — пересечение ключевых слов запроса с ключевыми словами
// primary-семьи кандидата плюс бонус 0.2, если семья входит в anchor_families
// мира (T-10-06); результат клампится в [0,1].
func familyMatch(c Candidate, q Query) float64 {
	overlap := keywordOverlap(q.Keywords, c.FamilyKeywords)
	if containsStr(q.AnchorFamilies, c.FamilyPrimary) {
		overlap += 0.2
	}
	return clamp01(overlap)
}

// keywordOverlap — доля ключевых слов запроса, найденных в ключевых словах
// семьи; пустой запрос даёт 0.
func keywordOverlap(query, family []string) float64 {
	if len(query) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(family))
	for _, k := range family {
		set[k] = struct{}{}
	}
	matched := 0
	for _, k := range query {
		if _, ok := set[k]; ok {
			matched++
		}
	}
	return float64(matched) / float64(len(query))
}

// secondaryMatch — 1.0 при совпадении secondary с семейным ожиданием запроса,
// 0.25 при наличии корректного secondary (непустого и не равного primary),
// 0 при пустом.
func secondaryMatch(c Candidate, q Query) float64 {
	switch {
	case q.Secondary != "" && c.PatternSecondary == q.Secondary:
		return 1.0
	case c.PatternSecondary != "" && c.PatternSecondary != c.FamilyPrimary:
		return 0.25
	default:
		return 0
	}
}

// techniqueFit — 1.0, если техника в списке ожиданий запроса; 0.5, если задана;
// 0, если пуста.
func techniqueFit(c Candidate, q Query) float64 {
	if c.Technique != "" && containsStr(q.Techniques, c.Technique) {
		return 1.0
	}
	if c.Technique != "" {
		return 0.5
	}
	return 0
}

// moralStanceFit — 1.0/0.5/0 по той же таблице, что и techniqueFit.
func moralStanceFit(c Candidate, q Query) float64 {
	if c.MoralStance != "" && containsStr(q.MoralStances, c.MoralStance) {
		return 1.0
	}
	if c.MoralStance != "" {
		return 0.5
	}
	return 0
}

// moodMatch — 1.0 при совпадении с настроением запроса, иначе 0.5.
func moodMatch(c Candidate, q Query) float64 {
	if q.Mood != "" && c.Mood == q.Mood {
		return 1.0
	}
	return 0.5
}

// quality — trust_level записи банка пополам (0..2 → 0..1).
func quality(c Candidate) float64 {
	return float64(c.TrustLevel) / 2
}

// freshness — свежесть семьи в главе: 1/(1 + family_usage_chapter[family]).
func freshness(c Candidate) float64 {
	return 1 / (1 + float64(c.UsageChapter))
}

// familyOnly — запись только с primary-семьёй (нет secondary/техники/позиции).
func familyOnly(c Candidate) bool {
	return c.PatternSecondary == "" && c.Technique == "" && c.MoralStance == ""
}

// clamp01 ограничивает значение отрезком [0,1].
func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}
