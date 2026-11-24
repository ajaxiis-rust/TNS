package turn

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"tns65/internal/db"
)

// Инструменты героя (T-19-09, §55): именованные опоры — предметы, навыки-опоры и
// связи. Сила героя не выводит его из мира, а вплетает глубже, поэтому опора
// влияет на допустимость действий и факторы сцены, но её можно отнять.
// Набор живёт слоем tools в character_layers героя (character_id 0): стартовый
// набор выводится из world_meta.hero_birth рождения, дальше набор меняет только
// событие мира.

// toolsLayer — имя слоя инструментов героя в character_layers.
const toolsLayer = "tools"

// toolEventKind — вид события мира, меняющего набор инструментов героя.
const toolEventKind = "hero_tools"

// HeroToolKind — род опоры героя: предмет, навык-опора или связь.
type HeroToolKind string

const (
	ToolItem  HeroToolKind = "item"
	ToolSkill HeroToolKind = "skill"
	ToolBond  HeroToolKind = "bond"
)

// HeroTool — именованная опора героя.
type HeroTool struct {
	Kind HeroToolKind `json:"kind"`
	Name string       `json:"name"`
}

// HeroTools — набор инструментов героя. LastEventID — номер последнего
// применённого события мира: события с меньшим или равным номером повторно не
// применяются.
type HeroTools struct {
	Tools       []HeroTool `json:"tools"`
	LastEventID int64      `json:"last_event_id,omitempty"`
}

// heroToolEvent — изменение набора событием мира: опоры добавляются и снимаются.
type heroToolEvent struct {
	Add    []HeroTool `json:"add,omitempty"`
	Remove []HeroTool `json:"remove,omitempty"`
}

// heroBirthToolset — стартовые наборы по происхождению героя (hero_birth): род
// занятий даёт предмет, навык-опору и связь, которые мир видит и может отнять.
var heroBirthToolset = []struct {
	marks []string
	tools []HeroTool
}{
	{
		marks: []string{"аристократ", "дворян", "знат"},
		tools: []HeroTool{
			{Kind: ToolItem, Name: "родовая печать"},
			{Kind: ToolSkill, Name: "speech"},
			{Kind: ToolBond, Name: "покровительство рода"},
		},
	},
	{
		marks: []string{"воин", "солдат", "наёмн", "наемн", "рыцар"},
		tools: []HeroTool{
			{Kind: ToolItem, Name: "меч"},
			{Kind: ToolSkill, Name: "combat"},
			{Kind: ToolBond, Name: "отряд"},
		},
	},
	{
		marks: []string{"маг", "чародей", "волшеб", "ведьм"},
		tools: []HeroTool{
			{Kind: ToolItem, Name: "посох"},
			{Kind: ToolSkill, Name: "lore"},
			{Kind: ToolBond, Name: "наставник"},
		},
	},
	{
		marks: []string{"купец", "торгов", "ремесл", "мастеров"},
		tools: []HeroTool{
			{Kind: ToolItem, Name: "кошель"},
			{Kind: ToolSkill, Name: "craft"},
			{Kind: ToolBond, Name: "гильдия"},
		},
	},
	{
		marks: []string{"вор", "разбой", "бегл", "каторж"},
		tools: []HeroTool{
			{Kind: ToolItem, Name: "отмычка"},
			{Kind: ToolSkill, Name: "stealth"},
			{Kind: ToolBond, Name: "шайка"},
		},
	},
}

// defaultBirthTools — набор простолюдина: незнакомое происхождение не оставляет
// героя без опор.
var defaultBirthTools = []HeroTool{
	{Kind: ToolItem, Name: "рабочий нож"},
	{Kind: ToolSkill, Name: "survival"},
	{Kind: ToolBond, Name: "семья"},
}

// startingTools выводит стартовый набор из hero_birth. Первое совпадение метки
// задаёт набор; незнакомое происхождение даёт набор простолюдина.
func startingTools(heroBirth string) []HeroTool {
	birth := strings.ToLower(heroBirth)
	for _, set := range heroBirthToolset {
		for _, mark := range set.marks {
			if strings.Contains(birth, mark) {
				return append([]HeroTool(nil), set.tools...)
			}
		}
	}
	return append([]HeroTool(nil), defaultBirthTools...)
}

// toolsStage — инструменты героя (T-19-09): стартовый набор из hero_birth и
// события мира до сборки рамки сцены. Набор — состояние мира, а не результат
// хода, поэтому пишется стадией, а не коммитом (как завещание T-19-03).
type toolsStage struct {
	db *db.Handle
}

func (*toolsStage) Name() string { return "tools" }

func (s *toolsStage) Run(_ context.Context, st *TurnState) error {
	if s.db == nil {
		return nil
	}
	if err := seedHeroTools(s.db); err != nil {
		return err
	}
	cur, _, err := readHeroTools(s.db)
	if err != nil {
		return err
	}
	next, changed, err := applyHeroToolEvents(s.db, cur)
	if err != nil {
		return err
	}
	if changed {
		if err := writeHeroTools(s.db, next); err != nil {
			return err
		}
	}
	st.HeroTools = next
	return nil
}

// seedHeroTools записывает стартовый набор героя из hero_birth, если слоя ещё
// нет: набор фиксируется до первого хода героя и дальше меняется только
// событием мира.
func seedHeroTools(h *db.Handle) error {
	if h == nil {
		return nil
	}
	if _, ok, err := readHeroTools(h); err != nil || ok {
		return err
	}
	birth, err := readHeroBirth(h)
	if err != nil {
		return err
	}
	return writeHeroTools(h, HeroTools{Tools: startingTools(birth)})
}

// readHeroBirth читает hero_birth из world_meta; значение пишется рождением
// JSON-строкой, голая строка принимается как есть.
func readHeroBirth(h *db.Handle) (string, error) {
	var raw string
	err := h.QueryRow(`SELECT value FROM world_meta WHERE "key" = 'hero_birth'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var birth string
	if json.Unmarshal([]byte(raw), &birth) == nil {
		return birth, nil
	}
	return strings.TrimSpace(raw), nil
}

// readHeroTools читает набор инструментов героя; false — слоя нет.
func readHeroTools(h *db.Handle) (HeroTools, bool, error) {
	if h == nil {
		return HeroTools{}, false, nil
	}
	var raw string
	err := h.QueryRow(
		`SELECT payload_json FROM character_layers WHERE character_id = ? AND layer = ?`,
		heroCharacterID, toolsLayer).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return HeroTools{}, false, nil
	}
	if err != nil {
		return HeroTools{}, false, err
	}
	var tools HeroTools
	if err := json.Unmarshal([]byte(raw), &tools); err != nil {
		return HeroTools{}, false, err
	}
	return tools, true, nil
}

// writeHeroTools перекрывает набор инструментов героя слоем tools.
func writeHeroTools(h *db.Handle, tools HeroTools) error {
	if h == nil {
		return nil
	}
	if tools.Tools == nil {
		tools.Tools = []HeroTool{}
	}
	raw, err := json.Marshal(tools)
	if err != nil {
		return err
	}
	if _, err := h.Exec("character_layers",
		`DELETE FROM character_layers WHERE character_id = ? AND layer = ?`,
		heroCharacterID, toolsLayer); err != nil {
		return err
	}
	_, err = h.Exec("character_layers",
		`INSERT INTO character_layers (character_id, layer, payload_json) VALUES (?, ?, ?)`,
		heroCharacterID, toolsLayer, string(raw))
	return err
}

// applyHeroToolEvents применяет события мира к набору: опоры из add добавляются,
// из remove снимаются; LastEventID отмечает обработанные события, чтобы повторный
// ход не снимал и не добавлял опору дважды.
func applyHeroToolEvents(h *db.Handle, cur HeroTools) (HeroTools, bool, error) {
	if h == nil {
		return cur, false, nil
	}
	rows, err := h.Query(
		`SELECT id, payload_json FROM events WHERE kind = ? AND id > ? ORDER BY id`,
		toolEventKind, cur.LastEventID)
	if err != nil {
		return cur, false, err
	}
	defer rows.Close()

	next := HeroTools{Tools: append([]HeroTool(nil), cur.Tools...), LastEventID: cur.LastEventID}
	changed := false
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return cur, false, err
		}
		var ev heroToolEvent
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			return cur, false, err
		}
		for _, tool := range ev.Remove {
			next = next.remove(tool)
		}
		for _, tool := range ev.Add {
			next = next.add(tool)
		}
		next.LastEventID = id
		changed = true
	}
	if err := rows.Err(); err != nil {
		return cur, false, err
	}
	return next, changed, nil
}

// add добавляет опору, если её ещё нет: опора уникальна парой род+имя.
func (t HeroTools) add(tool HeroTool) HeroTools {
	if tool.Name == "" {
		return t
	}
	for _, have := range t.Tools {
		if have == tool {
			return t
		}
	}
	t.Tools = append(t.Tools, tool)
	return t
}

// remove снимает опору: отнятая опора уходит из набора, повторное снятие ничего
// не меняет.
func (t HeroTools) remove(tool HeroTool) HeroTools {
	out := t.Tools[:0]
	for _, have := range t.Tools {
		if have != tool {
			out = append(out, have)
		}
	}
	t.Tools = out
	return t
}
