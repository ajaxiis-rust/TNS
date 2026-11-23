package workshop_test

import (
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/workshop"
)

// TestReplayEvents — снапшот ГСЧ и реплей бросков от него (T-19-05,
// «Проверка»): броски удачи пишутся в журнал событий мира, снапшот ГСЧ ложится
// в чекпоинт мира, а реплей от снапшота воспроизводит ту же последовательность
// бросков.
func TestReplayEvents(t *testing.T) {
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

	// Журнал событий пишет core, world_meta — forge (карта писателей T-01-07).
	core, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(core): %v", err)
	}
	defer core.Close()
	forge, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	defer forge.Close()

	luck := workshop.NewLuck(forge, "alpha")
	start := luck.Snapshot()
	if start.Draws != 0 {
		t.Fatalf("стартовый снапшот: бросков %d, want 0", start.Draws)
	}

	const rolls = 5
	for i := 0; i < rolls; i++ {
		r, err := luck.Roll(0.4)
		if err != nil {
			t.Fatalf("Roll %d: %v", i, err)
		}
		if err := workshop.RecordRoll(core, 1, 0.4, r); err != nil {
			t.Fatalf("RecordRoll %d: %v", i, err)
		}
	}

	// Снапшот после бросков лежит в чекпоинте мира.
	if err := luck.SaveCheckpoint(); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	saved, ok, err := luck.LoadCheckpoint()
	if err != nil || !ok {
		t.Fatalf("LoadCheckpoint = (%+v, %v, %v)", saved, ok, err)
	}
	if saved != luck.Snapshot() {
		t.Fatalf("чекпоинт %+v != снапшоту %+v", saved, luck.Snapshot())
	}
	if saved.Draws != rolls {
		t.Fatalf("бросков в снапшоте %d, want %d", saved.Draws, rolls)
	}

	// Продолжение от чекпоинта — тот же поток: новый экземпляр мира не
	// расходится с живым.
	next := luck.Draw()
	restored := workshop.NewLuck(forge, "alpha")
	restored.Restore(saved)
	if got := restored.Draw(); got != next {
		t.Fatalf("продолжение от снапшота %v != живому потоку %v", got, next)
	}

	// Реплей журнала от стартового снапшота воспроизводит броски удачи.
	events, err := workshop.LoadRollEvents(core)
	if err != nil {
		t.Fatalf("LoadRollEvents: %v", err)
	}
	if len(events) != rolls {
		t.Fatalf("бросков в журнале %d, want %d", len(events), rolls)
	}
	if got := workshop.ReplayRolls(start, events); got != rolls {
		t.Fatalf("реплей от снапшота совпал в %d бросках из %d", got, rolls)
	}

	// Снапшот значим: от чекпоинта после бросков тот же журнал не повторяется.
	if got := workshop.ReplayRolls(saved, events); got == rolls {
		t.Fatal("реплей от снапшота после бросков повторил журнал: снапшот не учтён")
	}
}
