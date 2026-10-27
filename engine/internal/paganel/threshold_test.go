package paganel_test

import (
	"fmt"
	"math"
	"testing"

	"tns65/internal/db"
	"tns65/internal/paganel"
)

// seedSectorPlaces сеет известные места в сектора 0..n-1 вокруг героя (0,0) —
// в середине каждого сектора на расстоянии distDeg градусов.
func seedSectorPlaces(t *testing.T, path string, n int, distDeg float64) {
	t.Helper()
	var places []paganel.Place
	for i := 0; i < n; i++ {
		angle := float64(i)*45 + 45.0/2
		rad := angle * math.Pi / 180
		places = append(places, paganel.Place{
			Name: fmt.Sprintf("место%d", i),
			X:    distDeg * math.Sin(rad),
			Y:    distDeg * math.Cos(rad),
		})
	}
	seedPlaces(t, path, places)
}

// placeHero ставит героя в character_state в (x, y) без привязки к месту.
func placeHero(t *testing.T, h *db.Handle, x, y float64) {
	t.Helper()
	if _, err := h.Exec("character_state",
		`INSERT INTO character_state (character_id, place_id, x, y) VALUES (0, 0, ?, ?)`,
		x, y); err != nil {
		t.Fatalf("INSERT character_state: %v", err)
	}
}

// countQueuedRings возвращает число строк rings со статусом queued.
func countQueuedRings(t *testing.T, path string) int {
	t.Helper()
	d, err := db.OpenReader(path)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM rings WHERE status = 'queued'`).Scan(&n); err != nil {
		t.Fatalf("COUNT queued rings: %v", err)
	}
	return n
}

// TestThreshold — порог кольца (T-06-06, «Проверка»): при пустом секторе в
// пределах двух дней в rings поставлена строка queued; при известных местах во
// всех восьми секторах постановки нет; названное за краем место также ставит
// кольцо.
func TestThreshold(t *testing.T) {
	cfg := paganel.DefaultPaganelConfig()

	t.Run("пустой сектор в пределах двух дней ставит кольцо queued", func(t *testing.T) {
		path := newRingWorld(t)
		// Сектора 0–6 заняты (0.1° ≈ 11 км < 2 дней ≈ 50 км), сектор 7 пуст.
		seedSectorPlaces(t, path, 7, 0.1)
		h := openRingCore(t, path)
		placeHero(t, h, 0, 0)

		ok, err := paganel.EnsureRingOnEdge(h, cfg)
		if err != nil {
			t.Fatalf("EnsureRingOnEdge: %v", err)
		}
		if !ok {
			t.Fatal("триггер края не сработал при пустом секторе")
		}
		if n := countQueuedRings(t, path); n != 1 {
			t.Fatalf("queued колец = %d, want 1", n)
		}
	})

	t.Run("известные места во всех восьми секторах не ставят кольцо", func(t *testing.T) {
		path := newRingWorld(t)
		seedSectorPlaces(t, path, 8, 0.1)
		h := openRingCore(t, path)
		placeHero(t, h, 0, 0)

		ok, err := paganel.EnsureRingOnEdge(h, cfg)
		if err != nil {
			t.Fatalf("EnsureRingOnEdge: %v", err)
		}
		if ok {
			t.Fatal("триггер края сработал при занятых секторах")
		}
		if n := countRings(t, path); n != 0 {
			t.Fatalf("колец = %d, want 0", n)
		}
	})

	t.Run("названное за краем место ставит кольцо", func(t *testing.T) {
		path := newRingWorld(t)
		seedSectorPlaces(t, path, 8, 0.1) // все сектора заняты в пределах двух дней
		h := openRingCore(t, path)
		placeHero(t, h, 0, 0)

		// Место к северу (сектор 0) на 0.5° ≈ 55 км > 2 дней (50 км), дальше
		// известного «место0» (0.1°) — названо за краем.
		ok, err := paganel.EnsureRingForPlace(h, cfg, 0, 0.5)
		if err != nil {
			t.Fatalf("EnsureRingForPlace: %v", err)
		}
		if !ok {
			t.Fatal("место за краем не поставлено в очередь")
		}
		if n := countQueuedRings(t, path); n != 1 {
			t.Fatalf("queued колец = %d, want 1", n)
		}
	})
}
