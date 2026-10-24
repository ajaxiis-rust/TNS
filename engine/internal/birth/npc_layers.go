package birth

import (
	"encoding/json"
	"math/rand"
)

// MemoryParams — рабочие параметры памяти NPC (D-18): темы живут с полураспадом
// halfLifeDays, отбор записей к сцене — по порогу similarityThreshold, лимит
// нити отношений — maxMemoryEntries.
type MemoryParams struct {
	HalfLifeDays        float64 `json:"halfLifeDays"`
	SimilarityThreshold float64 `json:"similarityThreshold"`
	MaxMemoryEntries    int     `json:"maxMemoryEntries"`
}

// DefaultMemoryParams — заводские значения D-18. При рождении параметры приходят
// из world_meta; здесь — дефолты, совпадающие с настройками станка.
func DefaultMemoryParams() MemoryParams {
	return MemoryParams{
		HalfLifeDays:        7,
		SimilarityThreshold: 0.7,
		MaxMemoryEntries:    10000,
	}
}

// Humours — темперамент на четырёх гуморах (B-07.2).
type Humours struct {
	Sanguine    float64 `json:"sanguine"`
	Choleric    float64 `json:"choleric"`
	Melancholic float64 `json:"melancholic"`
	Phlegmatic  float64 `json:"phlegmatic"`
}

// StateLayer — слой состояния NPC (B-07.1): здоровье 0–100, настроение, девять
// навыков и память.
type StateLayer struct {
	Health int            `json:"health"`
	Mood   string         `json:"mood"`
	Skills map[string]int `json:"skills"`
	Memory MemoryParams   `json:"memory"`
}

// EconomyLayer — слой экономики NPC (B-07.2): шесть статов, восемь пороков,
// темперамент на четырёх гуморах, лояльность и семья.
type EconomyLayer struct {
	Stats       map[string]int `json:"stats"`
	Vices       map[string]int `json:"vices"`
	Temperament Humours        `json:"temperament"`
	Loyalty     float64        `json:"loyalty"`
	Family      string         `json:"family"`
}

// HiddenLayer — скрытый слой NPC (B-07.3): психотип Юнга и тайные мотивы.
// Игроку не показывается; мотивы — явное поле-заполнитель, пустое до назначения
// тайны (T-03-19), не выдумывается.
type HiddenLayer struct {
	Psychotype string   `json:"psychotype"`
	Motives    []string `json:"motives"`
}

// Layers — три слоя данных NPC (B-07): состояние, экономика и скрытый слой.
type Layers struct {
	State   StateLayer   `json:"state"`
	Economy EconomyLayer `json:"economy"`
	Hidden  HiddenLayer  `json:"hidden"`
}

// PlayerView — слои, видимые игроку: состояние и экономика, скрытый слой вырезан.
type PlayerView struct {
	State   StateLayer   `json:"state"`
	Economy EconomyLayer `json:"economy"`
}

// PlayerView возвращает сериализацию слоёв для игрока по белому списку полей:
// скрытый слой не попадает в ответ (B-07.3, шаг 5).
func (l Layers) PlayerView() PlayerView {
	return PlayerView{State: l.State, Economy: l.Economy}
}

// PlayerJSON — сериализация слоёв для интерфейса без ключей скрытого слоя.
func (l Layers) PlayerJSON() ([]byte, error) {
	return json.Marshal(l.PlayerView())
}

// Канонические перечни слоёв (B-07.1, B-07.2): имена — контракт структуры,
// значения детерминированно выводятся из психотипа.
var (
	skillNames = []string{
		"combat", "athletics", "perception", "stealth", "craft",
		"speech", "lore", "survival", "medicine",
	}

	statNames = []string{
		"strength", "dexterity", "constitution", "intelligence", "wisdom", "charisma",
	}

	viceNames = []string{
		"pride", "greed", "lust", "envy", "gluttony", "sloth", "cowardice", "deceit",
	}
)

// NewLayers строит три слоя NPC при создании карточки (B-07). Навыки, статы,
// пороки и гумора детерминированно выводятся из психотипа (без LLM); скрытый
// слой несёт психотип Юнга и пустые мотивы (B-07.3).
func NewLayers(psychotype string, mem MemoryParams) Layers {
	rng := rand.New(rand.NewSource(seedFromSlug(psychotype)))

	skills := make(map[string]int, len(skillNames))
	for _, name := range skillNames {
		skills[name] = 3 + rng.Intn(8) // 3..10
	}
	stats := make(map[string]int, len(statNames))
	for _, name := range statNames {
		stats[name] = 3 + rng.Intn(8)
	}
	vices := make(map[string]int, len(viceNames))
	for _, name := range viceNames {
		vices[name] = rng.Intn(4) // 0..3
	}

	h := Humours{
		Sanguine:    rng.Float64(),
		Choleric:    rng.Float64(),
		Melancholic: rng.Float64(),
		Phlegmatic:  rng.Float64(),
	}
	if sum := h.Sanguine + h.Choleric + h.Melancholic + h.Phlegmatic; sum > 0 {
		h.Sanguine /= sum
		h.Choleric /= sum
		h.Melancholic /= sum
		h.Phlegmatic /= sum
	}

	return Layers{
		State: StateLayer{
			Health: 100,
			Mood:   "спокойное",
			Skills: skills,
			Memory: mem,
		},
		Economy: EconomyLayer{
			Stats:       stats,
			Vices:       vices,
			Temperament: h,
			Loyalty:     0,
			Family:      "",
		},
		Hidden: HiddenLayer{
			Psychotype: psychotype,
			Motives:    []string{},
		},
	}
}
