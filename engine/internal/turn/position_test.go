package turn

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"tns65/internal/db"
	"tns65/internal/journal"
)

// newPositionPipeline — пайплайн с core-хэндлом и путь к базе мира: тест сдвига
// сеет places хэндлом forge (карта писателей), поэтому путь нужен отдельно.
func newPositionPipeline(t *testing.T) (*Pipeline, string) {
	t.Helper()
	dir := t.TempDir()
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

	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, logger)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	return &Pipeline{Logger: logger, DB: h}, path
}

// insertHeroPosition сеет строку героя в character_state хэндлом core.
func insertHeroPosition(t *testing.T, p *Pipeline, placeID int64, x, y float64) {
	t.Helper()
	if _, err := p.DB.Exec("character_state",
		`INSERT INTO character_state (character_id, place_id, x, y, mood) VALUES (?, ?, ?, ?, '')`,
		heroCharacterID, placeID, x, y); err != nil {
		t.Fatalf("insert hero position: %v", err)
	}
}

// seedPlaces сеет известные места хэндлом forge (карта писателей: places — forge).
func seedPlaces(t *testing.T, path string, places []PlaceCoord) {
	t.Helper()
	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	defer h.Close()
	for i, p := range places {
		if _, err := h.Exec("places",
			`INSERT INTO places (name, kind, x, y) VALUES (?, 'city', ?, ?)`,
			fmt.Sprintf("место%d", i), p.X, p.Y); err != nil {
			t.Fatalf("insert place %d: %v", i, err)
		}
	}
}

// positionStages — минимальный конвейер для проверки сдвига: стадия сдвига и
// коммит.
func positionStages(p *Pipeline) []Stage {
	return []Stage{
		&positionStage{db: p.DB},
		&commitStage{db: p.DB},
	}
}

func runPosition(t *testing.T, p *Pipeline, st *TurnState) error {
	t.Helper()
	for _, s := range positionStages(p) {
		if err := s.Run(context.Background(), st); err != nil {
			return err
		}
	}
	return nil
}

// TestPosition — сдвиг положения героя событием (T-03-15, «Проверка»): после
// события сдвига одна транзакция содержит и ответ хода, и новые x/y в
// character_state; запись в places со стороны хода не выполняется; триггер
// близости к краю срабатывает от приблизительного положения.
func TestPosition(t *testing.T) {
	t.Run("сдвиг пишется одной транзакцией с ответом", func(t *testing.T) {
		p, _ := newPositionPipeline(t)
		insertHeroPosition(t, p, 1, 10, 20)

		st := &TurnState{
			Input:         TurnInput{SessionID: 1, PlayerText: "буря уносит меня", At: time.Now()},
			StartedAt:     time.Now(),
			Reply:         "Буря сдвинула тебя.",
			Path:          "fallback",
			Confirm:       Confirmation{Decision: DecisionConfirmed},
			Prepath:       Prepath{Frame: SceneFrame{HeroPlaceID: 1}, LexiconLoaded: true},
			PositionShift: &PositionShift{DX: 5, DY: -3},
		}
		if err := runPosition(t, p, st); err != nil {
			t.Fatalf("run: %v", err)
		}

		var reply string
		if err := p.DB.QueryRow(`SELECT reply_text FROM turns`).Scan(&reply); err != nil {
			t.Fatalf("turns: %v", err)
		}
		if reply != "Буря сдвинула тебя." {
			t.Fatalf("reply = %q, want %q", reply, "Буря сдвинула тебя.")
		}

		var x, y float64
		if err := p.DB.QueryRow(
			`SELECT x, y FROM character_state WHERE character_id = ?`, heroCharacterID).
			Scan(&x, &y); err != nil {
			t.Fatalf("character_state: %v", err)
		}
		if x != 15 || y != 17 {
			t.Fatalf("положение = (%v, %v), want (15, 17)", x, y)
		}
	})

	t.Run("отказ на character_state откатывает ответ", func(t *testing.T) {
		p, _ := newPositionPipeline(t)
		insertHeroPosition(t, p, 1, 10, 20)

		st := &TurnState{
			Input:         TurnInput{SessionID: 1, PlayerText: "буря", At: time.Now()},
			StartedAt:     time.Now(),
			Reply:         "Буря сдвинула тебя.",
			Path:          "fallback",
			Confirm:       Confirmation{Decision: DecisionConfirmed},
			Prepath:       Prepath{Frame: SceneFrame{HeroPlaceID: 1}, LexiconLoaded: true},
			PositionShift: &PositionShift{DX: 5, DY: -3},
		}

		if err := (&positionStage{db: p.DB}).Run(context.Background(), st); err != nil {
			t.Fatalf("position: %v", err)
		}
		stage := &commitStage{db: p.DB, FailTable: "character_state"}
		if err := stage.Run(context.Background(), st); err == nil {
			t.Fatal("отказ на character_state не вернул ошибку")
		}

		// Ответ хода и сдвиг пишутся одной транзакцией: отказ на координатах
		// откатывает и строку хода.
		if n := tableCount(t, p, "turns"); n != 0 {
			t.Fatalf("строк turns после отката = %d, want 0", n)
		}
		if n := tableCount(t, p, "character_state"); n != 1 {
			t.Fatalf("строк character_state после отката = %d, want 1 (исходная)", n)
		}
	})

	t.Run("запись в places со стороны хода не выполняется", func(t *testing.T) {
		p, _ := newPositionPipeline(t)
		_, err := p.DB.Exec("places",
			`INSERT INTO places (name, kind, x, y) VALUES ('чужое', 'city', 0, 0)`)
		if !errors.Is(err, db.ErrForeignWriter) {
			t.Fatalf("places write error = %v, want ErrForeignWriter", err)
		}
	})

	t.Run("триггер близости к краю срабатывает от приблизительного положения", func(t *testing.T) {
		p, path := newPositionPipeline(t)
		insertHeroPosition(t, p, 1, 0, 0)
		// Известны только восток и юг — остальные сектора пусты, край близко.
		seedPlaces(t, path, []PlaceCoord{{X: 0.5, Y: 0}, {X: 0, Y: -0.5}})

		near, err := nearEdge(p.DB, 100)
		if err != nil {
			t.Fatalf("nearEdge: %v", err)
		}
		if !near {
			t.Fatal("триггер края не сработал при пустых секторах")
		}
	})

	t.Run("триггер края не срабатывает при занятых секторах", func(t *testing.T) {
		p, path := newPositionPipeline(t)
		insertHeroPosition(t, p, 1, 0, 0)
		var places []PlaceCoord
		for i := 0; i < edgeSectors; i++ {
			angle := float64(i)*edgeSectorDeg + edgeSectorDeg/2
			rad := angle * math.Pi / 180
			places = append(places, PlaceCoord{X: 0.3 * math.Sin(rad), Y: 0.3 * math.Cos(rad)})
		}
		seedPlaces(t, path, places)

		near, err := nearEdge(p.DB, 100)
		if err != nil {
			t.Fatalf("nearEdge: %v", err)
		}
		if near {
			t.Fatal("триггер края сработал при занятых секторах")
		}
	})
}
