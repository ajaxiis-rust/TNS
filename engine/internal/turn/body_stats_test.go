package turn

import (
	"encoding/json"
	"testing"
)

// TestReadHeroBodyStats — восемь телесных статов героя читаются из того же слоя
// состояния, что и здоровье (T-19-14, B-07.1).
func TestReadHeroBodyStats(t *testing.T) {
	p, _ := newTestPipeline(t)

	seed := HeroBodyStats{
		Health: 77, Hunger: 12, Thirst: 34, Fatigue: 56,
		Comfort: 88, Spirit: 65, Bladder: 21, Bowel: 43,
	}
	raw, err := json.Marshal(heroStateLayer{
		Health: seed.Health, Hunger: seed.Hunger, Thirst: seed.Thirst, Fatigue: seed.Fatigue,
		Comfort: seed.Comfort, Spirit: seed.Spirit, Bladder: seed.Bladder, Bowel: seed.Bowel,
	})
	if err != nil {
		t.Fatalf("marshal hero body stats: %v", err)
	}
	if _, err := p.DB.Exec("character_layers",
		`INSERT INTO character_layers (character_id, layer, payload_json) VALUES (?, ?, ?)`,
		heroCharacterID, stateLayer, string(raw)); err != nil {
		t.Fatalf("insert hero body stats: %v", err)
	}

	got, ok, err := readHeroBodyStats(p.DB)
	if err != nil {
		t.Fatalf("readHeroBodyStats: %v", err)
	}
	if !ok {
		t.Fatal("слой состояния есть, а статы признаны неизвестными")
	}
	if got != seed {
		t.Fatalf("телесные статы героя: %+v, want %+v", got, seed)
	}

	health, ok, err := readHeroHealth(p.DB)
	if err != nil {
		t.Fatalf("readHeroHealth: %v", err)
	}
	if !ok || health != seed.Health {
		t.Fatalf("здоровье = %d (ok=%v), want %d из того же слоя", health, ok, seed.Health)
	}

	t.Run("слоя нет — статы неизвестны", func(t *testing.T) {
		empty, _ := newTestPipeline(t)
		if _, ok, err := readHeroBodyStats(empty.DB); err != nil || ok {
			t.Fatalf("без слоя состояния: ok=%v, err=%v, want false, nil", ok, err)
		}
		if _, ok, err := readHeroHealth(empty.DB); err != nil || ok {
			t.Fatalf("без слоя состояния здоровье: ok=%v, err=%v, want false, nil", ok, err)
		}
	})
}
