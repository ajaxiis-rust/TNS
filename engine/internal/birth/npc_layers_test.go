package birth

import (
	"encoding/json"
	"testing"
)

func TestNpcLayers(t *testing.T) {
	layers := NewLayers("ENTJ", DefaultMemoryParams())
	checkLayers(t, layers)

	// Каждая карточка пачки несёт слои (B-07): pack.go пишет их при создании.
	a := validAncketa()
	cards, err := Pack(a)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if len(cards) != 96 {
		t.Fatalf("карточек: %d, want 96", len(cards))
	}
	for i, c := range cards {
		checkLayers(t, c.Layers)
		if c.Layers.Hidden.Psychotype != c.Psychotype {
			t.Fatalf("карточка %d: скрытый психотип %q != психотипу %q", i, c.Layers.Hidden.Psychotype, c.Psychotype)
		}
	}

	// Повторное построение того же психотипа не меняет слои (детерминированно).
	again := NewLayers("ENTJ", DefaultMemoryParams())
	aRaw, err := json.Marshal(layers)
	if err != nil {
		t.Fatalf("маршалинг слоёв: %v", err)
	}
	bRaw, err := json.Marshal(again)
	if err != nil {
		t.Fatalf("маршалинг слоёв (повтор): %v", err)
	}
	if string(aRaw) != string(bRaw) {
		t.Fatalf("слои изменились при повторном построении: %s vs %s", aRaw, bRaw)
	}
}

// checkLayers проверяет ожидания «Проверки» T-02-06 для одного набора слоёв.
func checkLayers(t *testing.T, l Layers) {
	t.Helper()

	// Слой состояния: здоровье 0–100, девять навыков, память (B-07.1).
	if l.State.Health < 0 || l.State.Health > 100 {
		t.Fatalf("здоровье вне 0–100: %d", l.State.Health)
	}
	if got := len(l.State.Skills); got != 9 {
		t.Fatalf("навыков: %d, want 9", got)
	}
	mem := l.State.Memory
	if mem.HalfLifeDays <= 0 || mem.SimilarityThreshold <= 0 || mem.MaxMemoryEntries <= 0 {
		t.Fatalf("параметры памяти не заполнены (D-18): %+v", mem)
	}

	// Слой экономики: шесть статов, восемь пороков, четыре гумора (B-07.2).
	if got := len(l.Economy.Stats); got != 6 {
		t.Fatalf("статов: %d, want 6", got)
	}
	if got := len(l.Economy.Vices); got != 8 {
		t.Fatalf("пороков: %d, want 8", got)
	}
	if got := humoursCount(l.Economy.Temperament); got != 4 {
		t.Fatalf("гуморов: %d, want 4", got)
	}

	// Скрытый слой: мотивы пусты до назначения тайны (B-07.3, T-03-19) — явное
	// поле-заполнитель, не выдумывается.
	if l.Hidden.Motives == nil {
		t.Fatal("мотивы должны быть явным пустым полем-заполнителем, а не отсутствовать")
	}
	if got := len(l.Hidden.Motives); got != 0 {
		t.Fatalf("мотивы до назначения тайны непусты: %v", l.Hidden.Motives)
	}

	// Сериализация для игрока не содержит ключей скрытого слоя (шаг 5).
	raw, err := l.PlayerJSON()
	if err != nil {
		t.Fatalf("сериализация для игрока: %v", err)
	}
	var view map[string]json.RawMessage
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatalf("разбор сериализации для игрока: %v", err)
	}
	for _, key := range []string{"hidden", "psychotype", "motives"} {
		if _, ok := view[key]; ok {
			t.Fatalf("сериализация для игрока содержит ключ скрытого слоя %q", key)
		}
	}
	for _, key := range []string{"state", "economy"} {
		if _, ok := view[key]; !ok {
			t.Fatalf("сериализация для игрока не содержит слоя %q", key)
		}
	}
}

// humoursCount — число гуморов в JSON-представлении темперамента.
func humoursCount(h Humours) int {
	b, err := json.Marshal(h)
	if err != nil {
		return 0
	}
	var m map[string]float64
	if err := json.Unmarshal(b, &m); err != nil {
		return 0
	}
	return len(m)
}
