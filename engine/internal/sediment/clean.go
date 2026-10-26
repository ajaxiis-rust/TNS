package sediment

import (
	"errors"
	"sort"
	"strings"
)

// SceneStatusClosed — статус полной завершённой сцены: только такие сцены идут
// в очистку (W-03.1). Незавершённая сцена в оседание не поступает.
const SceneStatusClosed = "closed"

// ErrNotFullScene — сцена не полная или не завершена (W-03.1).
var ErrNotFullScene = errors.New("sediment: сцена не завершена")

// Scene — полная завершённая сцена для очистки: проза, статус и факты хода,
// зафиксированные в сцене.
type Scene struct {
	ID     int64
	Status string
	Prose  string
	Facts  []Fact
}

// Fact — факт хода, зафиксированный в сцене: заменяемое значение и слот-токен
// набора, которым оно замещается.
type Fact struct {
	Token Token
	Value string
}

// Binding — привязка биндера сцены (token_bindings): токен-заменитель биндера и
// подставленное движком значение.
type Binding struct {
	Token string
	Value string
}

// Clean вырезает из прозы подставленные движком значения и ставит на их место
// слот-токены набора. Источник каждого заменяемого значения — только привязки
// биндера и факты хода, зафиксированные в сцене; значение вне этих источников не
// заменяется, структура сцены, действия и реплики остаются. Принимает только
// полную завершённую сцену, иначе ErrNotFullScene.
func Clean(scene Scene, bindings []Binding) (SedimentRecord, error) {
	if scene.Status != SceneStatusClosed {
		return SedimentRecord{}, ErrNotFullScene
	}

	type replacement struct {
		value string
		slot  Token
	}
	seen := make(map[string]bool)
	var repls []replacement

	add := func(value string, slot Token) {
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		repls = append(repls, replacement{value: value, slot: slot})
	}

	for _, b := range bindings {
		if slot, ok := slotForBinder(b.Token); ok {
			add(b.Value, slot)
		}
	}
	for _, f := range scene.Facts {
		if knownSlot(f.Token) {
			add(f.Value, f.Token)
		}
	}

	// Сначала длинные значения: вхождение «Иван Иванов» не ломает замену «Иван».
	sort.SliceStable(repls, func(i, j int) bool {
		return len(repls[i].value) > len(repls[j].value)
	})

	text := scene.Prose
	used := make(map[Token]bool)
	for _, r := range repls {
		if !strings.Contains(text, r.value) {
			continue
		}
		text = strings.ReplaceAll(text, r.value, string(r.slot))
		used[r.slot] = true
	}

	return SedimentRecord{
		SourceSceneID: scene.ID,
		CleanedText:   text,
		Slots:         usedSlotStrings(used),
	}, nil
}

// slotForBinder возвращает слот-токен оседания для токена-заменителя биндера из
// реестра; для токена вне реестра — false.
func slotForBinder(binder string) (Token, bool) {
	for _, b := range slotBindings {
		if b.Binder == binder {
			return b.Slot, true
		}
	}
	return "", false
}

// knownSlot сообщает, входит ли токен в закрытый набор слот-токенов.
func knownSlot(t Token) bool {
	for _, b := range slotBindings {
		if b.Slot == t {
			return true
		}
	}
	return false
}

// usedSlotStrings — использованные слот-токены в порядке реестра, строкой.
func usedSlotStrings(used map[Token]bool) []string {
	out := make([]string, 0, len(used))
	for _, t := range SlotTokens() {
		if used[t] {
			out = append(out, string(t))
		}
	}
	return out
}
