package sediment_test

import (
	"context"
	"errors"
	"testing"

	"tns65/internal/db"
	"tns65/internal/sediment"
	"tns65/internal/testkit"
)

// fakePrefilter — подменный префильтр Паганеля (реализация T-06-14 в тестах не
// нужна): фиксированное решение «пропустить / отсеять» и счётчик вызовов.
type fakePrefilter struct {
	keep  bool
	calls int
}

func (f *fakePrefilter) Keep(sediment.Scene) bool {
	f.calls++
	return f.keep
}

func TestPrefilter(t *testing.T) {
	ctx := context.Background()

	t.Run("дефолтный префильтр пропускает сцену", func(t *testing.T) {
		h := openLiterary(t, t.TempDir(), db.WriterSediment, nil)
		p := sediment.NewPipeline(
			testkit.NewFakeEmbed(),
			&fakeAutocriticSlot{responses: []string{`{"verdict":"clean"}`}},
			&fakeCensor{ok: true},
			nil,
		)

		scene := sediment.Scene{
			ID:     1,
			Status: sediment.SceneStatusClosed,
			Prose:  "Иван купил меч.",
		}

		ws, err := p.Run(ctx, h, scene, nil)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if ws.SourceSceneID != 1 {
			t.Errorf("SourceSceneID = %d, want 1", ws.SourceSceneID)
		}

		var n int
		if err := h.QueryRow(`SELECT COUNT(*) FROM sediment_scene`).Scan(&n); err != nil {
			t.Fatalf("COUNT(sediment_scene): %v", err)
		}
		if n != 1 {
			t.Errorf("строк sediment_scene = %d, want 1", n)
		}
	})

	t.Run("фальшивый префильтр отсеивает сцену до очистки", func(t *testing.T) {
		h := openLiterary(t, t.TempDir(), db.WriterSediment, nil)
		p := sediment.NewPipeline(
			testkit.NewFakeEmbed(),
			&fakeAutocriticSlot{responses: []string{`{"verdict":"clean"}`}},
			&fakeCensor{ok: true},
			nil,
		)
		p.Prefilter = &fakePrefilter{keep: false}

		// Незавершённая сцена: дойди конвейер до очистки, Run вернул бы
		// ErrNotFullScene; префильтр должен отсеять её раньше.
		scene := sediment.Scene{ID: 2, Status: "", Prose: "Иван купил меч."}

		_, err := p.Run(ctx, h, scene, nil)
		if !errors.Is(err, sediment.ErrPrefiltered) {
			t.Fatalf("Run = %v, want ErrPrefiltered", err)
		}
		if errors.Is(err, sediment.ErrNotFullScene) {
			t.Fatalf("Run вернул ErrNotFullScene: очистка вызвана до префильтра")
		}
	})

	t.Run("Паганель не пишет в литературную базу", func(t *testing.T) {
		h := openLiterary(t, t.TempDir(), db.WriterSediment, nil)
		p := sediment.NewPipeline(
			testkit.NewFakeEmbed(),
			&fakeAutocriticSlot{responses: []string{`{"verdict":"clean"}`}},
			&fakeCensor{ok: true},
			nil,
		)
		pf := &fakePrefilter{keep: false}
		p.Prefilter = pf

		scene := sediment.Scene{
			ID:     3,
			Status: sediment.SceneStatusClosed,
			Prose:  "Иван купил меч.",
		}

		if _, err := p.Run(ctx, h, scene, nil); !errors.Is(err, sediment.ErrPrefiltered) {
			t.Fatalf("Run = %v, want ErrPrefiltered", err)
		}

		var n int
		if err := h.QueryRow(`SELECT COUNT(*) FROM sediment_scene`).Scan(&n); err != nil {
			t.Fatalf("COUNT(sediment_scene): %v", err)
		}
		if n != 0 {
			t.Errorf("строк sediment_scene = %d, want 0 (Паганель не пишет)", n)
		}
	})
}
