package birth

import (
	"encoding/json"
	"testing"
)

// TestBodyStats — восемь статов тела в слое состояния NPC (T-19-14, B-07.1):
// статы стартуют состоянием покоя, не выводятся из психотипа и видны игроку.
func TestBodyStats(t *testing.T) {
	type bodyStats struct {
		Health  int `json:"health"`
		Hunger  int `json:"hunger"`
		Thirst  int `json:"thirst"`
		Fatigue int `json:"fatigue"`
		Comfort int `json:"comfort"`
		Spirit  int `json:"spirit"`
		Bladder int `json:"bladder"`
		Bowel   int `json:"bowel"`
	}
	want := bodyStats{
		Health: 100, Hunger: 0, Thirst: 0, Fatigue: 0,
		Comfort: 100, Spirit: 100, Bladder: 0, Bowel: 0,
	}

	extract := func(t *testing.T, l Layers) bodyStats {
		t.Helper()
		raw, err := l.PlayerJSON()
		if err != nil {
			t.Fatalf("сериализация для игрока: %v", err)
		}
		var view struct {
			State bodyStats `json:"state"`
		}
		if err := json.Unmarshal(raw, &view); err != nil {
			t.Fatalf("разбор состояния: %v", err)
		}
		return view.State
	}

	t.Run("стартовое состояние покоя", func(t *testing.T) {
		got := extract(t, NewLayers(validAncketa().Slug, "ENTJ", DefaultMemoryParams()))
		if got != want {
			t.Fatalf("телесные статы при рождении: %+v, want %+v", got, want)
		}
	})

	t.Run("из психотипа не выводятся", func(t *testing.T) {
		a := extract(t, NewLayers("mir-a", "INFP", DefaultMemoryParams()))
		b := extract(t, NewLayers("mir-b", "ESTJ", DefaultMemoryParams()))
		if a != want || b != want {
			t.Fatalf("статы зависят от психотипа: %+v / %+v, want %+v", a, b, want)
		}
	})

	t.Run("карточки пачки несут телесные статы", func(t *testing.T) {
		cards, err := Pack(validAncketa())
		if err != nil {
			t.Fatalf("Pack: %v", err)
		}
		for i, c := range cards {
			got := extract(t, c.Layers)
			if got != want {
				t.Fatalf("карточка %d: телесные статы %+v, want %+v", i, got, want)
			}
		}
	})
}
