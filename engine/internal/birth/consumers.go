package birth

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed data/consumers.json
var consumersFS embed.FS

// WorldFrozenFields — мировые поля, не переспрашиваемые при переигровке (B-02.15).
var WorldFrozenFields = []string{
	"description",
	"social_order",
	"economy_mods",
	"magic",
	"censorship",
}

// WorldReaskedFields — поля, задаваемые заново при переигровке (B-02.15).
var WorldReaskedFields = []string{
	"synopsis",
	"hero_birth",
}

// WorldFields — пометки переигровки (шаг 2): замороженные и задаваемые заново.
type WorldFields struct {
	Frozen  []string `json:"frozen"`
	Reasked []string `json:"reasked"`
}

// Consumers — таблица «ключ поля анкеты → потребители» (шаг 1).
type Consumers struct {
	Fields      map[string][]string `json:"fields"`
	WorldFields WorldFields         `json:"world_fields"`
}

// LoadConsumers возвращает таблицу потребителей из data/consumers.json.
func LoadConsumers() (Consumers, error) {
	b, err := consumersFS.ReadFile("data/consumers.json")
	if err != nil {
		return Consumers{}, err
	}
	var c Consumers
	if err := json.Unmarshal(b, &c); err != nil {
		return Consumers{}, err
	}
	return c, nil
}

// CheckConsumers проверяет инварианты таблицы потребителей против перечня
// ключей анкеты. Возвращает ошибку при первом нарушении (шаг 3).
func CheckConsumers(c Consumers, fields []string) error {
	for _, f := range fields {
		consumers, ok := c.Fields[f]
		if !ok {
			return fmt.Errorf("consumers: ключ %q без потребителя", f)
		}
		if len(consumers) == 0 {
			return fmt.Errorf("consumers: у ключа %q пустой список потребителей", f)
		}
	}
	for k := range c.Fields {
		if !contains(fields, k) {
			return fmt.Errorf("consumers: ключ %q отсутствует в анкете", k)
		}
	}
	if !sameStrings(c.WorldFields.Frozen, WorldFrozenFields) {
		return fmt.Errorf("consumers: мировые поля %v, want %v", c.WorldFields.Frozen, WorldFrozenFields)
	}
	if !sameStrings(c.WorldFields.Reasked, WorldReaskedFields) {
		return fmt.Errorf("consumers: поля переигровки %v, want %v", c.WorldFields.Reasked, WorldReaskedFields)
	}
	return nil
}

// sameStrings сравнивает два набора строк без учёта порядка.
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]int, len(a))
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		if m[s] == 0 {
			return false
		}
		m[s]--
	}
	return true
}
