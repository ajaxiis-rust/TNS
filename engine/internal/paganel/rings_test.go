package paganel_test

import (
	"errors"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/paganel"
)

// newRingWorld — свежая база мира (world+shared) в t.TempDir() и путь к ней.
func newRingWorld(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "world(alpha).db")
	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(raw, "world", "shared"); err != nil {
		raw.Close()
		t.Fatalf("Migrate: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return path
}

// openRingCore — хэндл мира от писателя core (кольца пишет ядро).
func openRingCore(t *testing.T, path string) *db.Handle {
	t.Helper()
	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(core): %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

// countRings возвращает число строк rings в базе мира.
func countRings(t *testing.T, path string) int {
	t.Helper()
	d, err := db.OpenReader(path)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM rings`).Scan(&n); err != nil {
		t.Fatalf("COUNT rings: %v", err)
	}
	return n
}

// TestRings — кольца: очередь, текущая работа, прогресс и счётчики
// (T-06-05, «Проверка»).
func TestRings(t *testing.T) {
	t.Run("постановка даёт queued с progress=0", func(t *testing.T) {
		path := newRingWorld(t)
		h := openRingCore(t, path)

		r, err := paganel.EnqueueRing(h, paganel.DefaultPaganelConfig())
		if err != nil {
			t.Fatalf("EnqueueRing: %v", err)
		}
		if r.Status != paganel.RingQueued || r.Progress != 0 {
			t.Fatalf("кольцо = %+v, want queued/progress=0", r)
		}

		got, err := paganel.ReadRing(h, r.ID)
		if err != nil {
			t.Fatalf("ReadRing: %v", err)
		}
		if got.Status != paganel.RingQueued || got.Progress != 0 {
			t.Fatalf("rings = %+v, want queued/progress=0", got)
		}
	})

	t.Run("перевод running → done обновляет finished_at и progress", func(t *testing.T) {
		path := newRingWorld(t)
		h := openRingCore(t, path)

		r, err := paganel.EnqueueRing(h, paganel.DefaultPaganelConfig())
		if err != nil {
			t.Fatalf("EnqueueRing: %v", err)
		}
		running, err := paganel.StartRing(h, r.ID)
		if err != nil {
			t.Fatalf("StartRing: %v", err)
		}
		if running.Status != paganel.RingRunning {
			t.Fatalf("статус после запуска = %q, want running", running.Status)
		}

		done, err := paganel.FinishRing(h, r.ID, 1.0)
		if err != nil {
			t.Fatalf("FinishRing: %v", err)
		}
		if done.Status != paganel.RingDone {
			t.Fatalf("статус = %q, want done", done.Status)
		}
		if done.FinishedAt == "" {
			t.Fatal("finished_at не обновлён")
		}
		if done.Progress != 1.0 {
			t.Fatalf("progress = %v, want 1", done.Progress)
		}
	})

	t.Run("при enabled=false строка не создаётся", func(t *testing.T) {
		path := newRingWorld(t)
		h := openRingCore(t, path)

		if _, err := paganel.EnqueueRing(h, paganel.PaganelConfig{Enabled: false}); !errors.Is(err, paganel.ErrForgeDisabled) {
			t.Fatalf("EnqueueRing(disabled) = %v, want ErrForgeDisabled", err)
		}
		if n := countRings(t, path); n != 0 {
			t.Fatalf("rings = %d, want 0", n)
		}
	})

	t.Run("счётчики мест/путей/людей растут по мере ковки", func(t *testing.T) {
		path := newRingWorld(t)
		h := openRingCore(t, path)

		r, err := paganel.EnqueueRing(h, paganel.DefaultPaganelConfig())
		if err != nil {
			t.Fatalf("EnqueueRing: %v", err)
		}
		c1, err := paganel.UpdateRingCounters(h, r.ID, paganel.RingCounters{Places: 1, Paths: 1, People: 2})
		if err != nil {
			t.Fatalf("UpdateRingCounters: %v", err)
		}
		if c1.PlacesCount != 1 || c1.PathsCount != 1 || c1.PeopleCount != 2 {
			t.Fatalf("счётчики = (%d, %d, %d), want (1, 1, 2)", c1.PlacesCount, c1.PathsCount, c1.PeopleCount)
		}
		c2, err := paganel.UpdateRingCounters(h, r.ID, paganel.RingCounters{Places: 3, Paths: 2, People: 4})
		if err != nil {
			t.Fatalf("UpdateRingCounters: %v", err)
		}
		if c2.PlacesCount <= c1.PlacesCount || c2.PathsCount <= c1.PathsCount || c2.PeopleCount <= c1.PeopleCount {
			t.Fatalf("счётчики не выросли: %+v → %+v", c1, c2)
		}
	})

	t.Run("переключатель читается из config.yaml", func(t *testing.T) {
		cfg, err := paganel.LoadPaganelConfig(filepath.Join("..", "..", "config.yaml"))
		if err != nil {
			t.Fatalf("LoadPaganelConfig(engine/config.yaml): %v", err)
		}
		if !cfg.Enabled {
			t.Fatal("engine/config.yaml paganel.enabled = false, want true")
		}
	})
}
