package birth

import (
	"reflect"
	"strings"
	"testing"
)

// TestIdiolect — идиолект карточки (B-15, T-19-08): данные покрывают каждый
// голос пачки, каждая карточка несёт идиолект своего голоса.
func TestIdiolect(t *testing.T) {
	voices, err := LoadVoices()
	if err != nil {
		t.Fatalf("LoadVoices: %v", err)
	}
	idios, err := LoadIdiolects()
	if err != nil {
		t.Fatalf("LoadIdiolects: %v", err)
	}

	t.Run("данные покрывают все голоса", func(t *testing.T) {
		for _, v := range voices {
			id, ok := idios[v]
			if !ok {
				t.Fatalf("голос %q без идиолекта", v)
			}
			if n := len(id.Lexicon); n < 30 || n > 50 {
				t.Fatalf("голос %q: лемм %d, want 30–50", v, n)
			}
			if n := len(id.Habits); n < 2 || n > 3 {
				t.Fatalf("голос %q: привычек %d, want 2–3", v, n)
			}
			if n := len(id.Taboo); n < 1 || n > 2 {
				t.Fatalf("голос %q: табу %d, want 1–2", v, n)
			}
			seen := map[string]bool{}
			for _, w := range id.Lexicon {
				w = strings.TrimSpace(w)
				if w == "" {
					t.Fatalf("голос %q: пустая лемма", v)
				}
				if seen[w] {
					t.Fatalf("голос %q: лемма %q повторяется", v, w)
				}
				seen[w] = true
			}
			for _, field := range [][]string{id.Habits, id.Taboo} {
				for _, s := range field {
					if strings.TrimSpace(s) == "" {
						t.Fatalf("голос %q: пустая запись идиолекта", v)
					}
				}
			}
		}
	})

	t.Run("карточка получает идиолект голоса", func(t *testing.T) {
		cards, err := Pack(validAncketa())
		if err != nil {
			t.Fatalf("Pack: %v", err)
		}
		for i, c := range cards {
			if c.Idiolect == nil {
				t.Fatalf("карточка %d (%q): идиолекта нет", i, c.Voice)
			}
			want := idios[c.Voice]
			if !reflect.DeepEqual(*c.Idiolect, want) {
				t.Fatalf("карточка %d: идиолект %+v, want идиолект голоса %q", i, *c.Idiolect, c.Voice)
			}
		}
	})
}
