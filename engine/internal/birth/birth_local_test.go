package birth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/journal"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// TestBirthLocalPreset — рождение в пресете local не зависит от внешних API
// (T-02-14): отказ локального слота не отменяет рождение и не теряет заказ, а
// внешних адресов рождение не касается.
func TestBirthLocalPreset(t *testing.T) {
	dir := t.TempDir()

	// Локальный слот :5001 подменён фейком testkit и отказывает — как в P1, пока
	// станок :5001 не поднят.
	slot := testkit.NewFakeSlot()
	slot.FailWith(errors.New("slot down"))
	workshop.SetSlot(slot)
	t.Cleanup(func() { workshop.SetSlot(nil) })

	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	res, err := Birth(validAncketa(), Config{
		WorldsRoot: dir,
		BaseDBPath: testBaseDB(t, dir),
		Embed:      testkit.NewFakeEmbed(),
		Logger:     logger,
	})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}
	if res == nil {
		t.Fatal("Birth: результат пуст")
	}
	if _, err := os.Stat(res.Dir); err != nil {
		t.Fatalf("мир не создан: %v", err)
	}

	// Шаг 1: оба заказа ушли через локальный слот — по одному вызову на заказ.
	if got := slot.Calls(); got != 2 {
		t.Fatalf("вызовов локального слота: %d, want 2 (analysis и forge_plan)", got)
	}

	// Шаг 2: заказы остались в очереди со статусом queued и растущим attempts.
	worldDB := openReader(t, res.WorldDB)
	type order struct {
		kind     string
		status   string
		attempts int
	}
	var orders []order
	rows, err := worldDB.Query(`SELECT kind, status, attempts FROM workshop_queue ORDER BY kind`)
	if err != nil {
		t.Fatalf("workshop_queue: %v", err)
	}
	for rows.Next() {
		var o order
		if err := rows.Scan(&o.kind, &o.status, &o.attempts); err != nil {
			t.Fatalf("workshop_queue scan: %v", err)
		}
		orders = append(orders, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("workshop_queue rows: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("заказов в очереди: %d, want 2", len(orders))
	}
	for _, o := range orders {
		if o.status != string(workshop.StatusQueued) {
			t.Errorf("статус заказа %s: %q, want queued", o.kind, o.status)
		}
		if o.attempts == 0 {
			t.Errorf("attempts заказа %s: 0, want > 0 (отказ локального слота)", o.kind)
		}
	}

	// Шаг 3: рождение не обращалось к внешним адресам — журнал без netgate.deny.
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("чтение журнала: %v", err)
	}
	if strings.Contains(string(raw), journal.EventNetgateDeny) {
		t.Fatalf("в журнале есть netgate.deny — рождение ходило наружу:\n%s", raw)
	}
}
