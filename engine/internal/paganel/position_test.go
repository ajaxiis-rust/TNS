package paganel_test

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/paganel"
)

// newPositionWorld — свежая база мира (world+shared) в t.TempDir() и путь к ней.
func newPositionWorld(t *testing.T) string {
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

// openCore — хэндл мира от писателя core (положение героя — character_state).
func openCore(t *testing.T, path string) *db.Handle {
	t.Helper()
	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(core): %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

// seedPlaces сеет места напрямую в places (карта писателей: places — forge;
// тест сеет сырым писателем до открытия core-хэндла).
func seedPlaces(t *testing.T, path string, places []paganel.Place) {
	t.Helper()
	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	defer raw.Close()
	for _, p := range places {
		if _, err := raw.Exec(
			`INSERT INTO places (name, kind, x, y) VALUES (?, 'city', ?, ?)`,
			p.Name, p.X, p.Y); err != nil {
			t.Fatalf("INSERT places %s: %v", p.Name, err)
		}
	}
}

// countPlaces возвращает число строк places в базе мира.
func countPlaces(t *testing.T, path string) int {
	t.Helper()
	d, err := db.OpenReader(path)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM places`).Scan(&n); err != nil {
		t.Fatalf("COUNT places: %v", err)
	}
	return n
}

// TestPosition — положение героя (T-06-04, «Проверка»): смена места в ходе
// обновляет place_id и координаты в character_state; триггер края читает
// положение из character_state; запись в places со стороны контура положения
// не выполняется.
func TestPosition(t *testing.T) {
	t.Run("смена места обновляет place_id и координаты", func(t *testing.T) {
		path := newPositionWorld(t)
		seedPlaces(t, path, []paganel.Place{{Name: "Альфа", X: 30, Y: 40}})
		h := openCore(t, path)

		pos, err := paganel.SetHeroPlace(h, 1)
		if err != nil {
			t.Fatalf("SetHeroPlace: %v", err)
		}
		if pos.PlaceID != 1 || pos.X != 30 || pos.Y != 40 {
			t.Fatalf("SetHeroPlace = %+v, want место 1 на (30, 40)", pos)
		}

		var placeID int64
		var x, y float64
		if err := h.QueryRow(
			`SELECT place_id, x, y FROM character_state WHERE character_id = 0`).
			Scan(&placeID, &x, &y); err != nil {
			t.Fatalf("character_state: %v", err)
		}
		if placeID != 1 || x != 30 || y != 40 {
			t.Fatalf("character_state = (%d, %v, %v), want (1, 30, 40)", placeID, x, y)
		}

		// places контуром положения не пишется: строка места осталась одна.
		if n := countPlaces(t, path); n != 1 {
			t.Fatalf("places после смены места = %d, want 1", n)
		}
	})

	t.Run("движение вне мест сдвигает координаты", func(t *testing.T) {
		path := newPositionWorld(t)
		seedPlaces(t, path, []paganel.Place{{Name: "Альфа", X: 30, Y: 40}})
		h := openCore(t, path)

		if _, err := paganel.SetHeroPlace(h, 1); err != nil {
			t.Fatalf("SetHeroPlace: %v", err)
		}
		pos, err := paganel.ShiftHeroPosition(h, 5, -3)
		if err != nil {
			t.Fatalf("ShiftHeroPosition: %v", err)
		}
		if pos.PlaceID != 1 || pos.X != 35 || pos.Y != 37 {
			t.Fatalf("ShiftHeroPosition = %+v, want место 1 на (35, 37)", pos)
		}
	})

	t.Run("триггер края читает положение из character_state", func(t *testing.T) {
		path := newPositionWorld(t)
		// Известны все 8 секторов вокруг (0,0).
		var places []paganel.Place
		for i := 0; i < 8; i++ {
			angle := float64(i)*45 + 45/2
			rad := angle * math.Pi / 180
			places = append(places, paganel.Place{
				Name: fmt.Sprintf("место%d", i),
				X:    0.3 * math.Sin(rad),
				Y:    0.3 * math.Cos(rad),
			})
		}
		seedPlaces(t, path, places)
		h := openCore(t, path)

		// Герой в character_state в (0,0) — все сектора заняты, края нет.
		if _, err := h.Exec("character_state",
			`INSERT INTO character_state (character_id, place_id, x, y) VALUES (0, 0, 0, 0)`); err != nil {
			t.Fatalf("INSERT character_state: %v", err)
		}
		near, err := paganel.NearEdge(h, 100)
		if err != nil {
			t.Fatalf("NearEdge: %v", err)
		}
		if near {
			t.Fatal("триггер края сработал при занятых секторах")
		}

		// Положение героя меняется только в character_state: у нового положения
		// известных мест рядом нет — край близко.
		if _, err := h.Exec("character_state",
			`UPDATE character_state SET x = 10, y = 10 WHERE character_id = 0`); err != nil {
			t.Fatalf("UPDATE character_state: %v", err)
		}
		near, err = paganel.NearEdge(h, 100)
		if err != nil {
			t.Fatalf("NearEdge после сдвига: %v", err)
		}
		if !near {
			t.Fatal("триггер края не сработал после смены положения в character_state")
		}
	})

	t.Run("запись в places со стороны контура положения не выполняется", func(t *testing.T) {
		path := newPositionWorld(t)
		h := openCore(t, path)
		if _, err := h.Exec("places",
			`INSERT INTO places (name, kind, x, y) VALUES ('чужое', 'city', 0, 0)`); !errors.Is(err, db.ErrForeignWriter) {
			t.Fatalf("places write error = %v, want ErrForeignWriter", err)
		}
	})
}
