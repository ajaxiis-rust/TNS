package sediment_test

import (
	"errors"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/sediment"
)

// openWorldCore открывает оперативную базу мира во временном каталоге, мигрирует
// её (world+shared) и возвращает писателя core — тот же порядок, что в конвейере.
func openWorldCore(t *testing.T, dir string) *db.Handle {
	t.Helper()
	path := filepath.Join(dir, "world(alpha).db")

	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(raw, "world", "shared"); err != nil {
		_ = raw.Close()
		t.Fatalf("Migrate: %v", err)
	}
	_ = raw.Close()

	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func TestSignalGate(t *testing.T) {
	dir := t.TempDir()
	h := openWorldCore(t, dir)

	t.Run("лайк поднимает приоритет до 70", func(t *testing.T) {
		if _, err := h.Exec("scene_signals",
			`INSERT INTO scene_signals (scene_id, kind, created_at) VALUES (7, 'like', '2026-01-01T00:00:00Z')`); err != nil {
			t.Fatalf("INSERT like: %v", err)
		}

		signals, err := sediment.SceneSignals(h, 7)
		if err != nil {
			t.Fatalf("SceneSignals: %v", err)
		}
		priority, err := sediment.SignalGate(signals)
		if err != nil {
			t.Fatalf("SignalGate(like): %v", err)
		}
		if priority != 70 {
			t.Errorf("приоритет = %d, want 70", priority)
		}
	})

	t.Run("дизлайк отклоняет сцену до очистки", func(t *testing.T) {
		if _, err := h.Exec("scene_signals",
			`INSERT INTO scene_signals (scene_id, kind, created_at) VALUES (8, 'dislike', '2026-01-01T00:00:00Z')`); err != nil {
			t.Fatalf("INSERT dislike: %v", err)
		}

		signals, err := sediment.SceneSignals(h, 8)
		if err != nil {
			t.Fatalf("SceneSignals: %v", err)
		}
		if _, err := sediment.SignalGate(signals); !errors.Is(err, sediment.ErrSceneDisliked) {
			t.Fatalf("SignalGate(dislike) = %v, want ErrSceneDisliked", err)
		}
	})

	t.Run("счёт строк scene_signals на t.TempDir()", func(t *testing.T) {
		var n int
		if err := h.QueryRow(`SELECT COUNT(*) FROM scene_signals`).Scan(&n); err != nil {
			t.Fatalf("COUNT(scene_signals): %v", err)
		}
		if n != 2 {
			t.Errorf("строк scene_signals = %d, want 2", n)
		}
	})
}
