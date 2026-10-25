package selector_test

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/selector"
)

// near — сравнение чисел с плавающей точкой с допуском.
func near(a, b, eps float64) bool {
	return math.Abs(a-b) <= eps
}

// openWorldCore открывает оперативную базу мира, мигрирует её и возвращает
// писателя core (тот же порядок, что в конвейере хода, T-03-05).
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

	logger, err := journal.New(filepath.Join(dir, "engine.log"))
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, logger)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func TestSaw(t *testing.T) {
	cfg := selector.DefaultSawConfig()

	t.Run("K по профилю без профилировщика", func(t *testing.T) {
		// Пила берёт профиль готовым: K вычисляется из двух метрик карточки,
		// профилировщик (birth.ProfileFromCode) не вызывается — входом служит
		// уже посчитанный Profile.
		if k := selector.K(selector.Profile{"extraversion": -1, "rationality": -1}); k != 0 {
			t.Errorf("K(интроверт-этик) = %v, want 0", k)
		}
		if k := selector.K(selector.Profile{"extraversion": 1, "rationality": 1}); k != 1 {
			t.Errorf("K(экстраверт-логик) = %v, want 1", k)
		}
	})

	t.Run("отдельная переменная по нити и сумма", func(t *testing.T) {
		// K=0 (rise=0.03): нить 1 — один шаг, нить 2 — два шага.
		saw := selector.NewSaw(selector.Profile{"extraversion": -1, "rationality": -1}, cfg, 0, nil, nil)
		for _, th := range []int64{1, 2, 2} {
			if _, err := saw.Step(th, selector.MoodNeutral); err != nil {
				t.Fatal(err)
			}
		}
		if !near(saw.ThreadValue(1), 0.03, 1e-9) {
			t.Errorf("нить 1 = %v, want 0.03", saw.ThreadValue(1))
		}
		if !near(saw.ThreadValue(2), 0.06, 1e-9) {
			t.Errorf("нить 2 = %v, want 0.06", saw.ThreadValue(2))
		}
		if !near(saw.Total(), 0.09, 1e-9) {
			t.Errorf("сумма = %v, want 0.09", saw.Total())
		}
	})

	t.Run("сумма насыщается при 1", func(t *testing.T) {
		saw := selector.NewSaw(selector.Profile{"extraversion": 1, "rationality": 1}, cfg, 0, nil, nil)
		// K=1 (rise=0.08): по 9 шагов на две нити — 0.72 каждая, сумма 1.44 → 1.
		for i := 0; i < 9; i++ {
			if _, err := saw.Step(1, selector.MoodNeutral); err != nil {
				t.Fatal(err)
			}
			if _, err := saw.Step(2, selector.MoodNeutral); err != nil {
				t.Fatal(err)
			}
		}
		if !near(saw.Total(), 1.0, 1e-9) {
			t.Errorf("сумма = %v, want 1 (насыщение)", saw.Total())
		}
	})

	t.Run("config.yaml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		content := "saw:\n  rise_min: 0.03\n  rise_max: 0.08\n  peak: 0.8\n  fall: 0.35\n  flat: 0.3\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := selector.LoadSawConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if want := selector.DefaultSawConfig(); got != want {
			t.Errorf("LoadSawConfig = %+v, want %+v", got, want)
		}
	})

	t.Run("ряд K=0 не даёт пика", func(t *testing.T) {
		saw := selector.NewSaw(selector.Profile{"extraversion": -1, "rationality": -1}, cfg, 0, nil, nil)
		for i := 0; i < 5; i++ {
			rec, err := saw.Step(1, selector.MoodNeutral)
			if err != nil {
				t.Fatal(err)
			}
			if rec != selector.RecommendBank {
				t.Errorf("реплика %d: рекомендация %q, want bank", i+1, rec)
			}
		}
		if !near(saw.Total(), 0.15, 1e-9) {
			t.Errorf("сумма после 5 спокойных реплик K=0 = %v, want 0.15", saw.Total())
		}
	})

	t.Run("ряд K=1 с эхом даёт пик", func(t *testing.T) {
		saw := selector.NewSaw(selector.Profile{"extraversion": 1, "rationality": 1}, cfg, 0, nil, nil)
		peakReply := 0
		for i := 1; i <= 12; i++ {
			rec, err := saw.Step(1, selector.MoodEcho)
			if err != nil {
				t.Fatal(err)
			}
			if rec == selector.RecommendWeaver {
				peakReply = i
				break
			}
		}
		if peakReply == 0 {
			t.Fatal("пик не наступил за 12 реплик")
		}
		if peakReply < 4 {
			t.Errorf("пик на %d-й реплике, не раньше 4-й", peakReply)
		}
		// rise = 0.08 × 1.25 (эхо) = 0.1; порог 0.8 пересекается на 8-й реплике в
		// точной арифметике и на 9-й с учётом накопления float64.
		if peakReply > 9 {
			t.Errorf("пик на %d-й реплике, ожидался в [4,9]", peakReply)
		}
	})

	t.Run("сбой фазы меняет форму сигнала", func(t *testing.T) {
		dir := t.TempDir()
		logger, err := journal.New(filepath.Join(dir, "engine.log"))
		if err != nil {
			t.Fatal(err)
		}
		saw := selector.NewSaw(selector.Profile{"extraversion": 1, "rationality": 1}, cfg, 0, nil, logger)
		for i := 0; i < 5; i++ {
			if _, err := saw.Step(1, selector.MoodNeutral); err != nil {
				t.Fatal(err)
			}
		}
		before := saw.ThreadValue(1)
		saw.PhaseBreak()
		if after := saw.ThreadValue(1); !near(after, before*0.5, 1e-9) {
			t.Errorf("сбой фазы: %v → %v, want %v", before, after, before*0.5)
		}

		b, err := os.ReadFile(filepath.Join(dir, "engine.log"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"event":"`+journal.EventSawPhaseBreak+`"`) {
			t.Errorf("в журнале нет события %s", journal.EventSawPhaseBreak)
		}
	})
}

func TestSawTable(t *testing.T) {
	h := openWorldCore(t, t.TempDir())

	// K=0 (rise=0.03): пять спокойных реплик — 0.15 по нити.
	saw := selector.NewSaw(selector.Profile{"extraversion": -1, "rationality": -1}, selector.DefaultSawConfig(), 0, h, nil)
	for i := 0; i < 5; i++ {
		if _, err := saw.Step(1, selector.MoodNeutral); err != nil {
			t.Fatalf("Step: %v", err)
		}
	}

	var v float64
	if err := h.QueryRow(`SELECT value FROM saw_state WHERE character_id = 0 AND thread_id = 1`).Scan(&v); err != nil {
		t.Fatalf("saw_state query: %v", err)
	}
	if !near(v, 0.15, 1e-9) {
		t.Errorf("saw_state.value = %v, want 0.15", v)
	}

	// Суммарное значение по базе совпадает с суммой значений нитей.
	var sum float64
	if err := h.QueryRow(`SELECT SUM(value) FROM saw_state WHERE character_id = 0`).Scan(&sum); err != nil {
		t.Fatalf("SUM(saw_state.value): %v", err)
	}
	if !near(sum, 0.15, 1e-9) {
		t.Errorf("SUM(saw_state.value) = %v, want 0.15", sum)
	}
}
