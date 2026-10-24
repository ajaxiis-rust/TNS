package birth

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand"
)

//go:embed data/estates.json data/voices.json data/appearances.json
var packFS embed.FS

// Estate — сословие пачки с весовыми признаками по строю (B-05.1). Число
// карточек от строя не меняется: веса — атрибуты сословия, а не квота.
type Estate struct {
	Estate        string             `json:"estate"`
	WeightByOrder map[string]float64 `json:"weight_by_order"`
}

// Card — карточка персонажа пачки (B-05.3). Тайны нет: поля для тайны в
// структуре не существует (B-05.4); предыстория может отсутствовать (omitempty).
type Card struct {
	Psychotype string  `json:"psychotype"`
	Estate     string  `json:"estate"`
	Archetype  string  `json:"archetype"`
	Voice      string  `json:"voice"`
	Appearance string  `json:"appearance"`
	Weight     float64 `json:"weight"`
	Backstory  string  `json:"backstory,omitempty"`
}

// LoadEstates возвращает шесть сословий из data/estates.json в порядке файла.
func LoadEstates() ([]Estate, error) {
	b, err := packFS.ReadFile("data/estates.json")
	if err != nil {
		return nil, err
	}
	var estates []Estate
	if err := json.Unmarshal(b, &estates); err != nil {
		return nil, err
	}
	return estates, nil
}

// LoadVoices возвращает кодовую таблицу голосов из data/voices.json.
func LoadVoices() ([]string, error) {
	b, err := packFS.ReadFile("data/voices.json")
	if err != nil {
		return nil, err
	}
	var voices []string
	if err := json.Unmarshal(b, &voices); err != nil {
		return nil, err
	}
	return voices, nil
}

// LoadAppearances возвращает кодовую таблицу внешностей из data/appearances.json.
func LoadAppearances() ([]string, error) {
	b, err := packFS.ReadFile("data/appearances.json")
	if err != nil {
		return nil, err
	}
	var appearances []string
	if err := json.Unmarshal(b, &appearances); err != nil {
		return nil, err
	}
	return appearances, nil
}

// Pack — пачка из 96 карточек: 16 психотипов × 6 сословий, без случайных
// пропусков и дублей пары (B-05.2). Голос и внешность — из кодовых таблиц,
// архетип — из двенадцати юнгианских, выбор детерминированным ГСЧ, засеянным
// от слага мира (B-05.3). LLM не участвует; тайна не назначается (B-05.4).
func Pack(a *Ancketa) ([]Card, error) {
	estates, err := LoadEstates()
	if err != nil {
		return nil, err
	}
	if len(estates) != 6 {
		return nil, fmt.Errorf("birth: сословий %d, want 6", len(estates))
	}

	voices, err := LoadVoices()
	if err != nil {
		return nil, err
	}
	if len(voices) == 0 {
		return nil, errors.New("birth: таблица голосов пуста")
	}

	appearances, err := LoadAppearances()
	if err != nil {
		return nil, err
	}
	if len(appearances) == 0 {
		return nil, errors.New("birth: таблица внешностей пуста")
	}

	archs, err := LoadArchetypes()
	if err != nil {
		return nil, err
	}
	if len(archs) == 0 {
		return nil, errors.New("birth: список архетипов пуст")
	}

	rng := rand.New(rand.NewSource(seedFromSlug(a.Slug)))
	codes := AllCodes()
	cards := make([]Card, 0, len(codes)*len(estates))
	for _, code := range codes {
		for _, e := range estates {
			cards = append(cards, Card{
				Psychotype: code.String(),
				Estate:     e.Estate,
				Archetype:  archs[rng.Intn(len(archs))],
				Voice:      voices[rng.Intn(len(voices))],
				Appearance: appearances[rng.Intn(len(appearances))],
				Weight:     estateWeight(e, a.SocialOrder),
			})
		}
	}
	return cards, nil
}

// estateWeight — весовой признак сословия под строй мира (B-05.5); строй без
// записи в таблице даёт нейтральный вес 1.0.
func estateWeight(e Estate, order string) float64 {
	if w, ok := e.WeightByOrder[order]; ok {
		return w
	}
	return 1.0
}

// seedFromSlug — детерминированное семя ГСЧ из слага мира (B-05.3).
func seedFromSlug(slug string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(slug))
	return int64(h.Sum64())
}
