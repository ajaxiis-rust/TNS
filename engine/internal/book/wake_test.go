package book

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"tns65/internal/db"
	"tns65/internal/testkit"
)

// newWakeWorld — оперативная база мира с миграциями и полем «Пролог» в
// world_meta (пишет forge по карте писателей).
func newWakeWorld(t *testing.T, prologue string) *db.Handle {
	t.Helper()
	path := filepath.Join(t.TempDir(), "world(a).db")

	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(w, "world", "shared"); err != nil {
		_ = w.Close()
		t.Fatalf("Migrate(world, shared): %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	fh, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	raw, err := json.Marshal(prologue)
	if err != nil {
		_ = fh.Close()
		t.Fatalf("json.Marshal(prologue): %v", err)
	}
	if _, err := fh.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES ('prologue', ?)`, string(raw)); err != nil {
		_ = fh.Close()
		t.Fatalf("INSERT world_meta prologue: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close(forge): %v", err)
	}

	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// waitFor ждёт условия по реальному времени — страховка синхронизации горутин;
// таймеры в тесте движутся подставными часами testkit.
func waitFor(t *testing.T, name string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("не дождались: %s", name)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestWake — «Проверка» T-07-02.
func TestWake(t *testing.T) {
	prologue := "Раннее утро в порту. Холодный свет ложится на доски причала."
	world := newWakeWorld(t, prologue)

	// Таймер пассивного открытия — из config (секции wakeup в config.yaml нет,
	// поэтому дефолт 30 с).
	cfg, err := LoadWakeConfig(filepath.Join(engineRoot(t), "config.yaml"))
	if err != nil {
		t.Fatalf("LoadWakeConfig: %v", err)
	}
	if cfg.PassiveTimeout != 30*time.Second {
		t.Fatalf("passive_timeout = %v, want 30s", cfg.PassiveTimeout)
	}

	// Первая глава открывается текстом из Пролога (CB-01.4, B-02.4).
	gotPrologue, err := PrologueText(world)
	if err != nil {
		t.Fatalf("PrologueText: %v", err)
	}
	if gotPrologue != prologue {
		t.Fatalf("PrologueText = %q, want %q", gotPrologue, prologue)
	}

	now := "2026-10-27T07:00:00Z"
	ch1, err := OpenChapter(world, 1, now)
	if err != nil {
		t.Fatalf("OpenChapter(день 1): %v", err)
	}
	if ch1.Day != 1 || ch1.Status != StatusOpen || ch1.SessionID <= 0 {
		t.Fatalf("глава 1 = %+v, want day=1 open с сессией", ch1)
	}

	if err := ForgeWakeup(world, ch1.ID, prologue, now); err != nil {
		t.Fatalf("ForgeWakeup(первая глава): %v", err)
	}

	// Текст пробуждения не содержит крючков (CB-01.5).
	if HasHooks(prologue) {
		t.Fatalf("текст пробуждения содержит крючки: %q", prologue)
	}

	clock := testkit.NewClock()
	slot := testkit.NewFakeSlot()

	// Пассивное открытие: без реплики 30 с на подставных часах пробуждение из базы.
	textCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		text, err := DeliverWakeup(world, ch1, cfg, clock, slot, "")
		textCh <- text
		errCh <- err
	}()
	waitFor(t, "таймер пассивного открытия зарегистрирован", func() bool {
		return clock.PendingTimers() >= 1
	})
	clock.Advance(cfg.PassiveTimeout)
	if text := <-textCh; text != prologue {
		t.Fatalf("пассивное пробуждение = %q, want %q (из Пролога)", text, prologue)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("DeliverWakeup(пассивно): %v", err)
	}
	if got := slot.Calls(); got != 0 {
		t.Fatalf("вызовов живого слота при готовом пробуждении = %d, want 0", got)
	}

	// Пробуждение подано из базы: ready с delivered_at.
	w1, err := LoadWakeup(world, ch1.ID)
	if err != nil {
		t.Fatalf("LoadWakeup(глава 1): %v", err)
	}
	if w1.Status != WakeReady || w1.DeliveredAt == "" {
		t.Fatalf("пробуждение из базы = %+v, want ready с delivered_at", w1)
	}

	// Повторный вход не создаёт вторую сессию.
	if _, err := OpenChapter(world, 1, now); err != nil {
		t.Fatalf("повторный вход: %v", err)
	}
	var sessions int
	if err := world.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatalf("SELECT COUNT(sessions): %v", err)
	}
	if sessions != 1 {
		t.Fatalf("сессий после повторного входа = %d, want 1", sessions)
	}

	// Повторная ковка той же главы не создаёт вторую запись wakeups (UNIQUE).
	if err := ForgeWakeup(world, ch1.ID, "Перекованное утро.", now); err != nil {
		t.Fatalf("ForgeWakeup(повторная ковка): %v", err)
	}
	var wakeCount int
	if err := world.QueryRow(`SELECT COUNT(*) FROM wakeups WHERE chapter_id = ?`, ch1.ID).Scan(&wakeCount); err != nil {
		t.Fatalf("SELECT COUNT(wakeups): %v", err)
	}
	if wakeCount != 1 {
		t.Fatalf("записей wakeups для главы 1 = %d, want 1 (UNIQUE)", wakeCount)
	}

	// Закрытие главы.
	if err := ch1.Close(world, "2026-10-27T22:00:00Z"); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Следующая глава открывается приходом игрока; ковка её пробуждения ставится
	// после закрытия предыдущей главы (CB-01.2).
	ch2, err := OpenChapter(world, 2, "2026-10-28T07:00:00Z")
	if err != nil {
		t.Fatalf("OpenChapter(день 2): %v", err)
	}
	if err := EnqueueWakeForge(world, ch2.ID); err != nil {
		t.Fatalf("EnqueueWakeForge: %v", err)
	}

	// Ковка пробуждения главы 2 — со статусом forging; в очереди стоит заказ wakeup.
	w2, err := LoadWakeup(world, ch2.ID)
	if err != nil {
		t.Fatalf("LoadWakeup(глава 2): %v", err)
	}
	if w2.Status != WakeForging {
		t.Fatalf("статус ковки пробуждения = %q, want forging", w2.Status)
	}
	var wakeKind, rawPayload string
	if err := world.QueryRow(`SELECT kind, payload_json FROM workshop_queue WHERE kind = 'wakeup'`).Scan(&wakeKind, &rawPayload); err != nil {
		t.Fatalf("SELECT workshop_queue wakeup: %v", err)
	}
	if wakeKind != "wakeup" {
		t.Fatalf("kind заказа = %q, want wakeup", wakeKind)
	}
	var payload WakeForgePayload
	if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
		t.Fatalf("payload заказа wakeup: %v", err)
	}
	if payload.ChapterID != ch2.ID {
		t.Fatalf("payload.ChapterID = %d, want %d", payload.ChapterID, ch2.ID)
	}

	// При непрогретой ковке сработал живой запасной путь (CB-01.2): реплика
	// игрока открывает главу, мир отвечает живой генерацией.
	live := "Живое пробуждение: утро наступает."
	slot.SetResponse("wakeup", live)
	got, err := DeliverWakeup(world, ch2, cfg, clock, slot, "Я проснулся.")
	if err != nil {
		t.Fatalf("DeliverWakeup(живой запасной путь): %v", err)
	}
	if got != live {
		t.Fatalf("живое пробуждение = %q, want %q", got, live)
	}
	if got := slot.Calls(); got != 1 {
		t.Fatalf("вызовов живого слота при непрогретой ковке = %d, want 1", got)
	}
}
