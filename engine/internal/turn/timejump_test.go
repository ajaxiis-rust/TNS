package turn

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// writeTimeDetector записывает detectors_time.json во временный каталог формата
// T-01-08 (шаг 6) и возвращает загруженный детектор.
func writeTimeDetector(t *testing.T, dir string) *TimeDetector {
	t.Helper()
	cfg := `{"units":{"час":"hours","день":"days"},"numerals":{"три":3},"prepositions":["через","спустя"]}`
	if err := os.WriteFile(filepath.Join(dir, "detectors_time.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := LoadTimeDetector(dir)
	if err != nil {
		t.Fatalf("LoadTimeDetector: %v", err)
	}
	return d
}

func TestTimeJump(t *testing.T) {
	t.Run("без декларации часы мира идут 1:1 на подставных часах", func(t *testing.T) {
		clock := testkit.NewClock()
		w := NewWorldClock(10, clock)

		clock.Advance(5 * time.Minute)
		if got := w.Now(); got != 15 {
			t.Fatalf("минута мира = %d, want 15", got)
		}
		clock.Advance(3 * time.Minute)
		if got := w.Now(); got != 18 {
			t.Fatalf("минута мира = %d, want 18", got)
		}
	})

	t.Run("спустя три дня даёт TimeSpan 4320 с цитатой", func(t *testing.T) {
		d := writeTimeDetector(t, t.TempDir())
		span, ok := d.Detect("спустя три дня")
		if !ok {
			t.Fatal("декларация не найдена")
		}
		if span.DurationMin != 4320 {
			t.Fatalf("DurationMin = %d, want 4320", span.DurationMin)
		}
		if !strings.Contains(strings.ToLower(span.RawQuote), "спустя три дня") {
			t.Fatalf("цитата = %q, want содержит «спустя три дня»", span.RawQuote)
		}
	})

	t.Run("скачок до суток мгновенный с просчётом ближнего круга", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		d := writeTimeDetector(t, t.TempDir())
		stage := &timejumpStage{db: p.DB, detector: d}

		st := &TurnState{
			Input: TurnInput{SessionID: 1, PlayerText: "через час", At: time.Now()},
			Reply: "место, где ты остановился",
		}
		if err := stage.Run(context.Background(), st); err != nil {
			t.Fatalf("timejump: %v", err)
		}

		var n int
		if err := p.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE kind = ?`, jumpEventKind).Scan(&n); err != nil {
			t.Fatalf("COUNT(events): %v", err)
		}
		if n != 1 {
			t.Fatalf("событий скачка = %d, want 1", n)
		}

		var raw string
		if err := p.DB.QueryRow(`SELECT payload_json FROM events WHERE kind = ?`, jumpEventKind).Scan(&raw); err != nil {
			t.Fatalf("events query: %v", err)
		}
		var ev jumpEvent
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			t.Fatalf("payload: %v", err)
		}
		if ev.DurationMin != 60 {
			t.Fatalf("duration = %d, want 60", ev.DurationMin)
		}
		if !ev.InnerCircleRecount {
			t.Fatal("просчёт ближнего круга не отмечен в событии")
		}

		// Заказа в мастерскую при мгновенном скачке нет.
		var orders int
		if err := p.DB.QueryRow(`SELECT COUNT(*) FROM workshop_queue`).Scan(&orders); err != nil {
			t.Fatalf("COUNT(workshop_queue): %v", err)
		}
		if orders != 0 {
			t.Fatalf("заказов в мастерской = %d, want 0", orders)
		}

		// Прыжок не пуст: краткая фактическая строка пути легла в ответ.
		if !strings.Contains(strings.ToLower(st.Reply), "долго и без событий") {
			t.Fatalf("в ответе нет строки пути: %q", st.Reply)
		}
	})

	t.Run("скачок 1440 минут идёт заказом в мастерскую", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		d := writeTimeDetector(t, t.TempDir())
		stage := &timejumpStage{db: p.DB, detector: d}

		st := &TurnState{
			Input: TurnInput{SessionID: 2, PlayerText: "спустя день", At: time.Now()},
			Reply: "у ворот города",
		}
		if err := stage.Run(context.Background(), st); err != nil {
			t.Fatalf("timejump: %v", err)
		}

		var n int
		if err := p.DB.QueryRow(`SELECT COUNT(*) FROM workshop_queue`).Scan(&n); err != nil {
			t.Fatalf("COUNT(workshop_queue): %v", err)
		}
		if n != 1 {
			t.Fatalf("заказов в мастерской = %d, want 1", n)
		}
		var kind string
		if err := p.DB.QueryRow(`SELECT kind FROM workshop_queue`).Scan(&kind); err != nil {
			t.Fatalf("workshop_queue query: %v", err)
		}
		if kind != string(workshop.KindAnalysis) {
			t.Fatalf("kind заказа = %q, want %q", kind, workshop.KindAnalysis)
		}

		// Мгновенного события скачка при заказе нет — пересчёт идёт в паузе.
		var events int
		if err := p.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE kind = ?`, jumpEventKind).Scan(&events); err != nil {
			t.Fatalf("COUNT(events): %v", err)
		}
		if events != 0 {
			t.Fatalf("событий скачка при заказе = %d, want 0", events)
		}

		if !strings.Contains(strings.ToLower(st.Reply), "долго и без событий") {
			t.Fatalf("в ответе нет строки пути: %q", st.Reply)
		}
	})

	t.Run("ночная константа читается из config", func(t *testing.T) {
		if got := DefaultTimeConfig().NightThresholdMin; got != NightThresholdMin {
			t.Fatalf("дефолт ночи = %d, want %d", got, NightThresholdMin)
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte("time:\n  night_threshold_min: 480\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadTimeConfig(path)
		if err != nil {
			t.Fatalf("LoadTimeConfig: %v", err)
		}
		if cfg.NightThresholdMin != 480 {
			t.Fatalf("night_threshold_min = %d, want 480", cfg.NightThresholdMin)
		}
	})
}
