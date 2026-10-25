// Package selector — выбор пути хода: Пила оценивает глубину момента и советует
// путь, Селектор решает (LT-05, LT-09). Здесь же живёт датчик границы сцены и
// отдельный сигнал арки сцены. Пила предлагает, Селектор решает.
package selector

import "strconv"

// Path — путь текста, который Селектор выбирает каждый ход (LT-05.1):
// Челнок из банка (рутина) или Ткач (генерация прозы); плюс служебные исходы
// стоп-крана, слеш-команды и страховки.
type Path string

const (
	PathAbort    Path = "abort"    // стоп-кран/отмена — ход прерывается.
	PathCommand  Path = "command"  // слеш-команда — служебный ответ.
	PathFallback Path = "fallback" // страховка — нейтральная запись из банка.
	PathBank     Path = "bank"     // Челнок — запись из пула банка.
	PathWeaver   Path = "weaver"   // Ткач — генерация прозы.
)

// Rule — правило таблицы решений Селектора (T-03-09, шаг 1). Номера совпадают с
// порядком проверки: первое сработавшее правило выигрывает, ниже по списку не
// проверяется.
type Rule int

const (
	RuleStopCrane         Rule = iota + 1 // 1: стоп-кран/отмена — прервать.
	RuleCommand                           // 2: слеш-команда — служебный ответ.
	RuleHardFallback                      // 3: жёсткое ограничение — страховка fallback.
	RuleRepeatable                        // 4: сцена повторяема — Челнок.
	RuleEmotion                           // 5: эмоция или уникальность — Ткач.
	RuleSawRecommendation                 // 6: есть рекомендация Пилы — следовать ей.
	RuleUncertain                         // 7: неопределённость — Челнок или страховка.
)

// Name — имя правила для поля turns.selector_decision (T-03-09, шаг 2).
func (r Rule) Name() string {
	switch r {
	case RuleStopCrane:
		return "stop_crane"
	case RuleCommand:
		return "command"
	case RuleHardFallback:
		return "hard_fallback"
	case RuleRepeatable:
		return "repeatable"
	case RuleEmotion:
		return "emotion"
	case RuleSawRecommendation:
		return "saw_recommendation"
	case RuleUncertain:
		return "uncertain"
	default:
		return "unknown"
	}
}

// Decision — решение Селектора: выбранный путь и сработавшее правило.
type Decision struct {
	Path Path
	Rule Rule
}

// Encode — представление решения для turns.selector_decision: номер сработавшего
// правила и его имя (T-03-09, шаг 2).
func (d Decision) Encode() string {
	return strconv.Itoa(int(d.Rule)) + " " + d.Rule.Name()
}

// Input — сигналы хода, которые Селектор сводит в единую таблицу решений.
type Input struct {
	Command      string         // непусто — слеш-команда.
	Stop         bool           // стоп-кран/отмена (неподтверждённый смысл).
	HardFallback bool           // жёсткое ограничение: банк пуст и слот мёртв.
	Repeatable   bool           // сцена повторяема (T-03-07).
	Emotional    bool           // эмоция или уникальность реплики (LT-05.4).
	Saw          Recommendation // рекомендация Пилы (T-03-06); советник, не судья.
	BankEmpty    bool           // банк пуст: неопределённость падает на страховку.
}

// Select применяет таблицу решений по порядку (T-03-09, шаг 1): стоп-кран →
// слеш-команда → жёсткое ограничение → повторяемое → эмоция/уникальность →
// рекомендация Пилы → неопределённость. Первое сработавшее правило выигрывает;
// рекомендация Пилы не обязательна к исполнению (LT-05.6) — её опережают
// повторяемое и эмоция.
func Select(in Input) Decision {
	switch {
	case in.Stop:
		return Decision{Path: PathAbort, Rule: RuleStopCrane}
	case in.Command != "":
		return Decision{Path: PathCommand, Rule: RuleCommand}
	case in.HardFallback:
		return Decision{Path: PathFallback, Rule: RuleHardFallback}
	case in.Repeatable:
		return Decision{Path: PathBank, Rule: RuleRepeatable}
	case in.Emotional:
		return Decision{Path: PathWeaver, Rule: RuleEmotion}
	case in.Saw == RecommendWeaver:
		return Decision{Path: PathWeaver, Rule: RuleSawRecommendation}
	case in.Saw == RecommendBank:
		return Decision{Path: PathBank, Rule: RuleSawRecommendation}
	default:
		if in.BankEmpty {
			return Decision{Path: PathFallback, Rule: RuleUncertain}
		}
		return Decision{Path: PathBank, Rule: RuleUncertain}
	}
}

// RecommendationCode — целочисленный код рекомендации Пилы для поля
// turns.saw_recommendation: 0 — нет рекомендации, 1 — Ткач, 2 — Челнок.
func RecommendationCode(r Recommendation) int {
	switch r {
	case RecommendWeaver:
		return 1
	case RecommendBank:
		return 2
	default:
		return 0
	}
}
