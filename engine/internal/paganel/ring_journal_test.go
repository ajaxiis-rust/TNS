package paganel_test

import (
	"encoding/json"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/paganel"
)

// seedJournal готовит базу с текущим кольцом (running, progress>0, счётчики),
// очередью из двух колец и местами/путями в статусах draft/baked.
func seedJournal(t *testing.T) (string, *db.Handle, int64) {
	t.Helper()
	path := newRingWorld(t)
	h := openRingCore(t, path)
	cfg := paganel.DefaultPaganelConfig()

	a, err := paganel.EnqueueRing(h, cfg)
	if err != nil {
		t.Fatalf("EnqueueRing: %v", err)
	}
	if _, err := paganel.StartRing(h, a.ID); err != nil {
		t.Fatalf("StartRing: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := paganel.EnqueueRing(h, cfg); err != nil {
			t.Fatalf("EnqueueRing(очередь): %v", err)
		}
	}
	if _, err := h.Exec("rings",
		`UPDATE rings SET progress = ?, places_count = ?, paths_count = ?, people_count = ? WHERE id = ?`,
		0.4, 3, 2, 5, a.ID); err != nil {
		t.Fatalf("UPDATE rings: %v", err)
	}

	forge, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	for _, s := range []struct{ name, status string }{
		{"лес", "draft"}, {"река", "draft"}, {"город", "baked"},
	} {
		if _, err := forge.Exec("places",
			`INSERT INTO places (name, kind, origin, status) VALUES (?, ?, ?, ?)`,
			s.name, "место", "paganel", s.status); err != nil {
			_ = forge.Close()
			t.Fatalf("INSERT places: %v", err)
		}
	}
	for i, status := range []string{"draft", "baked", "baked"} {
		if _, err := forge.Exec("paths",
			`INSERT INTO paths (from_place_id, to_place_id, type, length_km, friction, origin, status)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			int64(i+1), int64(i+2), "trakt", 10.0, 1.0, "paganel", status); err != nil {
			_ = forge.Close()
			t.Fatalf("INSERT paths: %v", err)
		}
	}
	if err := forge.Close(); err != nil {
		t.Fatalf("forge.Close: %v", err)
	}

	return path, h, a.ID
}

// TestRingJournal — журнал колец для вкладки MCP (T-06-07, «Проверка»).
func TestRingJournal(t *testing.T) {
	t.Run("текущее кольцо running с progress>0, очередь из N колец, счётчики мест и путей", func(t *testing.T) {
		_, h, runningID := seedJournal(t)

		j, err := paganel.CollectRingJournal(h)
		if err != nil {
			t.Fatalf("CollectRingJournal: %v", err)
		}
		if j.Current.ID != runningID {
			t.Fatalf("Current.ID = %d, want %d", j.Current.ID, runningID)
		}
		if j.Current.Status != paganel.RingRunning {
			t.Fatalf("Current.Status = %q, want running", j.Current.Status)
		}
		if j.Current.Progress <= 0 {
			t.Fatalf("Current.Progress = %v, want > 0", j.Current.Progress)
		}
		if len(j.Queue) != 2 {
			t.Fatalf("Queue = %d, want 2", len(j.Queue))
		}
		if j.Current.PlacesCount != 3 || j.Current.PathsCount != 2 {
			t.Fatalf("счётчики мест/путей = (%d, %d), want (3, 2)",
				j.Current.PlacesCount, j.Current.PathsCount)
		}
		if j.Forge.PlacesDraft != 2 || j.Forge.PlacesBaked != 1 {
			t.Fatalf("статистика мест = (%d draft, %d baked), want (2, 1)",
				j.Forge.PlacesDraft, j.Forge.PlacesBaked)
		}
		if j.Forge.PathsDraft != 1 || j.Forge.PathsBaked != 2 {
			t.Fatalf("статистика путей = (%d draft, %d baked), want (1, 2)",
				j.Forge.PathsDraft, j.Forge.PathsBaked)
		}
	})

	t.Run("в контракте нет полей переключателей", func(t *testing.T) {
		_, h, _ := seedJournal(t)
		j, err := paganel.CollectRingJournal(h)
		if err != nil {
			t.Fatalf("CollectRingJournal: %v", err)
		}
		b, err := json.Marshal(j)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		s := string(b)
		if strings.Contains(s, "enabled") {
			t.Fatalf("в контракте журнала есть поле переключателя: %s", s)
		}
		for _, want := range []string{`"status"`, `"progress"`, `"places_count"`, `"paths_count"`} {
			if !strings.Contains(s, want) {
				t.Fatalf("в контракте нет поля %s: %s", want, s)
			}
		}
	})
}
