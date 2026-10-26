package sediment_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/sediment"
	"tns65/internal/testkit"
)

// fakeCensor — словарный фильтр с заготовленным решением (Цензор T-04-11 в
// тестах подменяется).
type fakeCensor struct {
	ok   bool
	hits []string
}

func (c *fakeCensor) Check(string) (bool, []string) {
	return c.ok, c.hits
}

// openLiterary открывает литературную базу во временном каталоге хэндлом writer
// (порядок — как в конвейере: схема literary, затем хэндл с тегом писателя).
func openLiterary(t *testing.T, dir string, writer db.Writer, logger *journal.Logger) *db.Handle {
	t.Helper()
	path := filepath.Join(dir, "literary.db")

	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(raw, "literary"); err != nil {
		_ = raw.Close()
		t.Fatalf("Migrate(literary): %v", err)
	}
	_ = raw.Close()

	h, err := db.OpenWriterAs(path, db.DBLiterary, writer, logger)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func TestWrite(t *testing.T) {
	t.Run("запись через Цензор создаёт строку с trust_level=1", func(t *testing.T) {
		h := openLiterary(t, t.TempDir(), db.WriterSediment, nil)
		cen := &fakeCensor{ok: true}

		rec := sediment.SedimentRecord{
			SourceSceneID: 42,
			CleanedText:   "{NAME} вошёл в {PLACE}.",
			Verdict:       sediment.VerdictClean,
			TrustLevel:    sediment.TrustLevelUnchecked,
		}
		id, err := sediment.Write(h, cen, rec, 0, "2026-01-01T00:00:00Z")
		if err != nil {
			t.Fatalf("Write: %v", err)
		}

		var trustLevel int
		var sourceSceneID int64
		if err := h.QueryRow(`SELECT trust_level, source_scene_id FROM sediment_scene WHERE id = ?`, id).Scan(&trustLevel, &sourceSceneID); err != nil {
			t.Fatalf("SELECT sediment_scene: %v", err)
		}
		if trustLevel != sediment.TrustLevelUnchecked {
			t.Errorf("trust_level = %d, want %d", trustLevel, sediment.TrustLevelUnchecked)
		}
		if sourceSceneID != 42 {
			t.Errorf("source_scene_id = %d, want 42", sourceSceneID)
		}
	})

	t.Run("выставление trust_level=2 писателем оседания отклонено", func(t *testing.T) {
		h := openLiterary(t, t.TempDir(), db.WriterSediment, nil)
		cen := &fakeCensor{ok: true}

		rec := sediment.SedimentRecord{
			SourceSceneID: 7,
			CleanedText:   "{NAME} вошёл.",
			Verdict:       sediment.VerdictClean,
			TrustLevel:    sediment.TrustLevelVerified,
		}
		if _, err := sediment.Write(h, cen, rec, 0, "2026-01-01T00:00:00Z"); !errors.Is(err, sediment.ErrTrustEscalation) {
			t.Fatalf("Write(trust_level=2) = %v, want ErrTrustEscalation", err)
		}

		var n int
		if err := h.QueryRow(`SELECT COUNT(*) FROM sediment_scene`).Scan(&n); err != nil {
			t.Fatalf("COUNT(sediment_scene): %v", err)
		}
		if n != 0 {
			t.Errorf("строк sediment_scene = %d, want 0 (отказ не создаёт строку)", n)
		}
	})

	t.Run("хэндл core на sediment_scene получает ErrForeignWriter", func(t *testing.T) {
		logger, logPath := newTestLogger(t)
		h := openLiterary(t, t.TempDir(), db.WriterCore, logger)
		cen := &fakeCensor{ok: true}

		rec := sediment.SedimentRecord{
			SourceSceneID: 8,
			CleanedText:   "{NAME} вошёл.",
			Verdict:       sediment.VerdictClean,
			TrustLevel:    sediment.TrustLevelUnchecked,
		}
		if _, err := sediment.Write(h, cen, rec, 0, "2026-01-01T00:00:00Z"); !errors.Is(err, db.ErrForeignWriter) {
			t.Fatalf("Write(core) = %v, want ErrForeignWriter", err)
		}

		if events := journalEvents(t, logPath); !events["db.foreign_writer"] {
			t.Errorf("в журнале нет события db.foreign_writer")
		}
	})

	t.Run("после одного прогона конвейера одна строка", func(t *testing.T) {
		h := openLiterary(t, t.TempDir(), db.WriterSediment, nil)
		p := sediment.NewPipeline(
			testkit.NewFakeEmbed(),
			&fakeAutocriticSlot{responses: []string{`{"verdict":"clean"}`}},
			&fakeCensor{ok: true},
			nil,
		)

		scene := sediment.Scene{
			ID:     42,
			Status: sediment.SceneStatusClosed,
			Prose:  "Иван купил меч.",
		}
		bindings := []sediment.Binding{{Token: "[NPCname]", Value: "Иван"}}

		ws, err := p.Run(context.Background(), h, scene, bindings)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if ws.TrustLevel != sediment.TrustLevelUnchecked {
			t.Errorf("TrustLevel = %d, want %d", ws.TrustLevel, sediment.TrustLevelUnchecked)
		}
		if ws.SourceSceneID != 42 {
			t.Errorf("SourceSceneID = %d, want 42", ws.SourceSceneID)
		}

		var n int
		if err := h.QueryRow(`SELECT COUNT(*) FROM sediment_scene`).Scan(&n); err != nil {
			t.Fatalf("COUNT(sediment_scene): %v", err)
		}
		if n != 1 {
			t.Errorf("строк sediment_scene = %d, want 1", n)
		}
	})
}
