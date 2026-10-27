package workshop_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/sediment"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// passCensor — словарный фильтр, пропускающий любой текст (Цензор T-04-11 в
// тестах подменяется).
type passCensor struct{}

func (passCensor) Check(string) (bool, []string) { return true, nil }

// insertSedimentScene ставит закрытую сцену кандидата оседания.
func insertSedimentScene(t *testing.T, h *db.Handle, turnID int64, prose string) int64 {
	t.Helper()
	res, err := h.Exec("scenes",
		`INSERT INTO scenes (session_id, day, status, prose, turn_id) VALUES (0, 1, 'closed', ?, ?)`, prose, turnID)
	if err != nil {
		t.Fatalf("INSERT scenes: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("scenes id: %v", err)
	}
	return id
}

// insertSedimentOrder ставит заказ sediment со статусом queued и заданным
// приоритетом; payload — ссылка на сцену по turn_id.
func insertSedimentOrder(t *testing.T, h *db.Handle, turnID int64, priority int, created string) int64 {
	t.Helper()
	payload, err := json.Marshal(workshop.SedimentOrderPayload{TurnID: turnID, SessionID: 1, Kind: "bank"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	res, err := h.Exec("workshop_queue",
		`INSERT INTO workshop_queue (kind, payload_json, priority, status, step, resumable, attempts, created_at) VALUES ('sediment', ?, ?, 'queued', 'light', 0, 0, ?)`,
		string(payload), priority, created)
	if err != nil {
		t.Fatalf("INSERT workshop_queue: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("workshop_queue id: %v", err)
	}
	return id
}

// orderStatus читает статус заказа из очереди.
func orderStatus(t *testing.T, h *db.Handle, id int64) string {
	t.Helper()
	var status string
	if err := h.QueryRow(`SELECT status FROM workshop_queue WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatalf("SELECT status: %v", err)
	}
	return status
}

// journalOutcomes возвращает исходы заказов оседания из журнала в порядке записи.
func journalOutcomes(t *testing.T, path string) []string {
	t.Helper()
	recs := sedimentJournalRecords(t, path)
	var out []string
	for _, r := range recs {
		if r.Event != "workshop.sediment" {
			continue
		}
		for _, f := range r.Fields {
			if f.Key == "outcome" {
				out = append(out, f.Value)
			}
		}
	}
	return out
}

type sedimentJournalEvent struct {
	Event  string          `json:"event"`
	Fields []journal.Field `json:"fields"`
}

func sedimentJournalRecords(t *testing.T, path string) []sedimentJournalEvent {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var out []sedimentJournalEvent
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var rec sedimentJournalEvent
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка журнала не JSON: %v\n%s", err, line)
		}
		out = append(out, rec)
	}
	return out
}

func TestSedimentRun(t *testing.T) {
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	litPath := filepath.Join(dir, "literary.db")
	journalPath := filepath.Join(dir, "engine.log")

	newDB(t, worldPath, "world", "shared")
	newDB(t, litPath, "literary")

	logger, err := journal.New(journalPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterSediment, logger)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer lit.Close()

	slot := testkit.NewFakeSlot()
	slot.SetResponse("autocritic", `{"verdict":"clean"}`)
	pipeline := sediment.NewPipeline(testkit.NewFakeEmbed(), slot, passCensor{}, logger)

	clk := testkit.NewClock()
	s := workshop.NewScheduler(world, clk, nil, logger)
	s.SetSedimentRunner(workshop.NewSedimentRunner(lit, pipeline))

	// Сцена без лайка — приоритет 50; сцена с лайком — 70 (T-04-06, +20).
	_ = insertSedimentScene(t, world, 11, "Он поднялся по тропе к воротам.")
	liked := insertSedimentScene(t, world, 12, "Она вошла в зал и огляделась.")

	orderUnliked := insertSedimentOrder(t, world, 11, 50, "2026-01-01T00:00:00Z")
	orderLiked := insertSedimentOrder(t, world, 12, 70, "2026-01-01T00:00:01Z")

	// Лёгкая ступень: заказы sediment не запускаются, очередь не тронута.
	clk.Advance(10 * time.Minute)
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run(light): %v", err)
	}
	if got := orderStatus(t, world, orderUnliked); got != string(workshop.StatusQueued) {
		t.Fatalf("на лёгкой ступени статус заказа sediment = %q, want queued", got)
	}
	if got := orderStatus(t, world, orderLiked); got != string(workshop.StatusQueued) {
		t.Fatalf("на лёгкой ступени статус заказа sediment с лайком = %q, want queued", got)
	}

	// Тяжёлая ступень: конвейер отработал, заказы закрыты исходом write; сцена с
	// лайком (приоритет 70) встала раньше сцены без лайка.
	clk.Advance(4 * time.Hour)
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run(heavy): %v", err)
	}
	if got := orderStatus(t, world, orderUnliked); got != string(workshop.StatusDone) {
		t.Fatalf("на тяжёлой ступени статус заказа sediment = %q, want done", got)
	}
	if got := orderStatus(t, world, orderLiked); got != string(workshop.StatusDone) {
		t.Fatalf("на тяжёлой ступени статус заказа sediment с лайком = %q, want done", got)
	}

	outcomes := journalOutcomes(t, journalPath)
	if len(outcomes) != 2 {
		t.Fatalf("исходов в журнале: %d, want 2", len(outcomes))
	}
	if outcomes[0] != string(workshop.SedimentWrite) || outcomes[1] != string(workshop.SedimentWrite) {
		t.Fatalf("исходы заказов = %v, want [write write]", outcomes)
	}

	// Осевшие записи в литературной базе: по одной на сцену.
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM sediment_scene`).Scan(&n); err != nil {
		t.Fatalf("COUNT(sediment_scene): %v", err)
	}
	if n != 2 {
		t.Fatalf("строк sediment_scene = %d, want 2", n)
	}

	// Порядок исполнения: заказ с лайком (приоритет 70) должен отработать раньше
	// заказа без лайка — его запись в литературной базе создана первой.
	var firstID int64
	if err := lit.QueryRow(`SELECT source_scene_id FROM sediment_scene ORDER BY id ASC LIMIT 1`).Scan(&firstID); err != nil {
		t.Fatalf("SELECT source_scene_id: %v", err)
	}
	if firstID != liked {
		t.Fatalf("первой осела сцена %d, want %d (с лайком)", firstID, liked)
	}
}
