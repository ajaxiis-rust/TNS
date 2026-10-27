package paganel_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/paganel"
	"tns65/internal/sediment"
	"tns65/internal/testkit"
)

// prefilterCensor — словарный фильтр, пропускающий любой текст (Цензор T-04-11
// в тестах подменяется).
type prefilterCensor struct{}

func (prefilterCensor) Check(string) (bool, []string) { return true, nil }

// seedPrefilterWorld готовит базу мира с двумя местами: «Иерусалим» известен
// игроку (player_map), «Тир» выкован, но игроку ещё не известен.
func seedPrefilterWorld(t *testing.T) string {
	t.Helper()
	path := newRingWorld(t)
	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	stmts := []string{
		`INSERT INTO places (id, name, kind, x, y, origin, status) VALUES
			(1, 'Иерусалим', 'место', 0, 0, 'paganel', 'baked'),
			(2, 'Тир', 'место', 1, 1, 'paganel', 'baked')`,
		`INSERT INTO player_map (place_id, known_at, drawn) VALUES (1, '3', 0)`,
	}
	for _, s := range stmts {
		if _, err := raw.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return path
}

// openPrefilterLiterary открывает литературную базу хэндлом писателя sediment.
func openPrefilterLiterary(t *testing.T) *db.Handle {
	t.Helper()
	path := filepath.Join(t.TempDir(), "literary.db")
	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(raw, "literary"); err != nil {
		_ = raw.Close()
		t.Fatalf("Migrate(literary): %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	h, err := db.OpenWriterAs(path, db.DBLiterary, db.WriterSediment, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// TestPrefilterImpl — префильтр Паганеля (T-06-14, «Проверка»): сцена с местом
// вне географии мира отсечена до очистки; сцена в известной географии пропущена;
// записи в literary.db нет.
func TestPrefilterImpl(t *testing.T) {
	ctx := context.Background()
	world := openAtlasReader(t, seedPrefilterWorld(t))

	pf, err := paganel.NewPrefilter(world)
	if err != nil {
		t.Fatalf("NewPrefilter: %v", err)
	}

	slot := testkit.NewFakeSlot()
	slot.SetResponse("autocritic", `{"verdict":"clean"}`)

	t.Run("сцена вне географии мира отсечена до очистки", func(t *testing.T) {
		lit := openPrefilterLiterary(t)
		p := sediment.NewPipeline(testkit.NewFakeEmbed(), slot, prefilterCensor{}, nil, pf)

		// Незавершённая сцена: дойди конвейер до очистки, Run вернул бы
		// ErrNotFullScene; префильтр отсеивает её раньше.
		scene := sediment.Scene{ID: 1, Status: "", Prose: "Иван купил меч в Афинах."}

		_, err := p.Run(ctx, lit, scene, nil)
		if !errors.Is(err, sediment.ErrPrefiltered) {
			t.Fatalf("Run = %v, want ErrPrefiltered", err)
		}
		if errors.Is(err, sediment.ErrNotFullScene) {
			t.Fatal("Run вернул ErrNotFullScene: очистка вызвана до префильтра")
		}

		var n int
		if err := lit.QueryRow(`SELECT COUNT(*) FROM sediment_scene`).Scan(&n); err != nil {
			t.Fatalf("COUNT(sediment_scene): %v", err)
		}
		if n != 0 {
			t.Fatalf("строк sediment_scene = %d, want 0 (отсеянная сцена не пишется)", n)
		}
	})

	t.Run("сцена в известной географии пропущена", func(t *testing.T) {
		lit := openPrefilterLiterary(t)
		p := sediment.NewPipeline(testkit.NewFakeEmbed(), slot, prefilterCensor{}, nil, pf)

		scene := sediment.Scene{
			ID:     2,
			Status: sediment.SceneStatusClosed,
			Prose:  "Иван вошёл в Иерусалим.",
		}

		if !pf.Keep(scene) {
			t.Fatal("префильтр отсеял сцену в известной географии")
		}

		ws, err := p.Run(ctx, lit, scene, nil)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if ws.SourceSceneID != 2 {
			t.Fatalf("SourceSceneID = %d, want 2", ws.SourceSceneID)
		}

		var n int
		if err := lit.QueryRow(`SELECT COUNT(*) FROM sediment_scene`).Scan(&n); err != nil {
			t.Fatalf("COUNT(sediment_scene): %v", err)
		}
		if n != 1 {
			t.Fatalf("строк sediment_scene = %d, want 1", n)
		}
	})

	t.Run("место вне player_map, но в places, пропущено", func(t *testing.T) {
		scene := sediment.Scene{ID: 3, Status: sediment.SceneStatusClosed, Prose: "Иван шёл в Тир."}
		if !pf.Keep(scene) {
			t.Fatal("префильтр отсеял место, выкованное в places (не известное игроку)")
		}
	})
}
