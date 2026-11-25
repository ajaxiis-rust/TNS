package models

// GenerateParams — параметры запроса генерации (П1 плана LLM-настроек):
// nil-поле в тело запроса не попадает, то есть действует дефолт самой модели.
// Значения заполняет слой настроек (П3); пустые params — прежний запрос.
type GenerateParams struct {
	Temperature *float64 // температура сэмплирования
	MaxTokens   *int     // потолок выхода в токенах; nil и 0 — без поля
	TopP        *float64 // nucleus-сэмплирование
	TopK        *int     // отсечение по top-k (не стандарт OpenAI, у совместимых есть)
	Reasoning   *int     // бюджет рассуждений thinking-модели, токены
}

// Empty сообщает, что ни один параметр не задан: тело запроса полей генерации
// не несёт (дефолт модели).
func (p GenerateParams) Empty() bool {
	return p.Temperature == nil && p.MaxTokens == nil && p.TopP == nil &&
		p.TopK == nil && p.Reasoning == nil
}

// reasoningEffort переводит бюджет рассуждений в reasoning_effort OpenAI-маршрута.
// Таблица gen_reasoning (П3): 0 — рассуждения выключены («none»), малый бюджет
// — low, средний — medium, крупный — high.
func reasoningEffort(budget int) string {
	switch {
	case budget <= 0:
		return "none"
	case budget < 2048:
		return "low"
	case budget < 8192:
		return "medium"
	default:
		return "high"
	}
}
