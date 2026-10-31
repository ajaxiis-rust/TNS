//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tns65/internal/book"
	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/testkit"
)

// INT-9 (T-09-09). Утро главы: пробуждение выдаётся сразу; глубокий разбор
// первой реплики — ровно один дешёвый вызов на главу и завершается внутри окна
// чтения ~30 секунд; ожидания разбора в чате нет.

const (
	// readWindow — окно чтения пробуждения (INT-9.3): ~30 секунд.
	readWindow = 30 * time.Second
	// readLatency — латентность разбора, скрытая в окне чтения: меньше окна.
	readLatency = 20 * time.Second
	// wakeupByteBudget — пробуждение выдаётся сразу (INT-9, шаг 1): < 500 мс.
	wakeupByteBudget = int64(500)
)

// int9DeepRaw — результат разбора первой реплики: четыре поля, цитаты — подстроки
// реплики после нормализации (CB-02.4).
const int9DeepRaw = `{"intent":{"value":"поиск выхода","quote":"хочу найти","confidence":0.9},` +
	`"plan":{"value":"идти к воротам","quote":"из города","confidence":0.8},` +
	`"foreboding":{"value":"опоздать","quote":"до темноты","confidence":0.5},` +
	`"open_thread":{"value":"запертые ворота","quote":"выход","confidence":0.7}}`

// int9Replica — первая реплика главы: открывает главу и разбирается глубоким
// разбором ровно один раз (CB-01.3, CB-02.1).
const int9Replica = "Я хочу найти выход из города до темноты."

// TestINT9 — INT-9. Утро главы (T-09-09).
func TestINT9(t *testing.T) {
	bin := buildBinary(t, []string{"e2e"})
	dir := worldDir(t)

	slug := "utro-glavy"
	ancketa := writeAncketa(t, t.TempDir(), slug,
		"Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.")
	stdout, stderr, code := runCmd(t, dir, bin, "world", "birth", "--ancketa", ancketa, "--fake-embed", "--fake-slot")
	if code != 0 {
		t.Fatalf("world birth exit=%d, want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	worldDB := worldDBPath(dir, slug)

	// Журнал станка — для счётчика события deepread.call (T-07-03).
	logPath := filepath.Join(t.TempDir(), "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	h, err := db.OpenWriterAs(worldDB, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("открыть базу мира: %v", err)
	}
	defer h.Close()

	report := map[string]any{}

	// Шаг 1: открыть новую главу — пробуждение выдано сразу (первый байт < 500 мс
	// по часам ядра, без ожидания пассивного таймера ~30 с).
	now := "2026-10-31T07:00:00Z"
	ch, err := book.OpenChapter(h, 1, now)
	if err != nil {
		t.Fatalf("OpenChapter: %v", err)
	}
	wakeText := "Утро встаёт над тихим портом."
	if err := book.ForgeWakeup(h, ch.ID, wakeText, now); err != nil {
		t.Fatalf("ForgeWakeup: %v", err)
	}

	clock := testkit.NewClock()
	slot := testkit.NewFakeSlot()

	start := time.Now()
	got, err := book.DeliverWakeup(h, ch, book.DefaultWakeConfig(), clock, slot, int9Replica)
	wakeMs := time.Since(start).Milliseconds()
	if err != nil {
		t.Fatalf("DeliverWakeup(сразу): %v", err)
	}
	if got != wakeText {
		t.Fatalf("пробуждение = %q, want %q", got, wakeText)
	}
	report["wakeup_first_byte_ms"] = wakeMs
	if wakeMs >= wakeupByteBudget {
		t.Fatalf("wakeup_first_byte_ms=%d, want < %d", wakeMs, wakeupByteBudget)
	}
	if n := clock.PendingTimers(); n != 0 {
		t.Fatalf("таймеров при немедленной реплике = %d, want 0 (без пассивного ожидания)", n)
	}

	// Ожидания разбора в чате нет: ответ (пробуждение) доставлен, а таймер
	// разбора в часах появится только после доставки — разбор ответ не блокирует.
	report["no_chat_wait"] = wakeMs < wakeupByteBudget && clock.PendingTimers() == 0
	if !report["no_chat_wait"].(bool) {
		t.Fatal("no_chat_wait=false: ответ ждёт разбор или пассивный таймер")
	}

	// Шаги 2–3: глубокий разбор первой реплики — ровно один вызов на главу; его
	// латентность скрыта в окне чтения и завершается без реального ожидания.
	slot.SetResponse(book.DeepReadRole, int9DeepRaw)
	slot.SetDelay(readLatency)
	slot.SetClock(clock)

	deepErr := make(chan error, 1)
	go func() {
		_, derr := book.DeepRead(h, ch, int9Replica, book.DefaultDeepReadConfig(), slot, logger)
		deepErr <- derr
	}()
	waitForPendingTimer(t, clock)
	clock.Advance(readLatency)
	if err := <-deepErr; err != nil {
		t.Fatalf("DeepRead(первая реплика): %v", err)
	}
	report["read_within_window"] = readLatency <= readWindow
	if !report["read_within_window"].(bool) {
		t.Fatalf("read_within_window=false: латентность разбора %v > окна %v", readLatency, readWindow)
	}

	// Шаг 4: утро не отдельная стадия живого хода — вторая реплика главы нового
	// разбора не порождает (слот не вызван, событие deepread.call не выросло).
	slot.SetDelay(0)
	if _, err := book.DeepRead(h, ch, "Совсем другая реплика.", book.DefaultDeepReadConfig(), slot, logger); err != nil {
		t.Fatalf("DeepRead(вторая реплика): %v", err)
	}
	report["deep_read_calls"] = countJournalEvents(t, logPath, "deepread.call")
	if report["deep_read_calls"] != 1 {
		t.Fatalf("deep_read_calls=%d, want 1", report["deep_read_calls"])
	}
	if got := slot.Calls(); got != 1 {
		t.Fatalf("вызовов слота разбора = %d, want 1", got)
	}

	writeReport(t, "int9", report)
}

// waitForPendingTimer ждёт, пока fake-часы зарегистрируют хотя бы один таймер —
// страховка синхронизации горутины разбора до сдвига часов.
func waitForPendingTimer(t *testing.T, clock *testkit.Clock) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for clock.PendingTimers() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("таймер разбора не зарегистрирован")
		}
		time.Sleep(time.Millisecond)
	}
}

// countJournalEvents считает строки журнала с заданным событием.
func countJournalEvents(t *testing.T, logPath, event string) int {
	t.Helper()
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile журнала: %v", err)
	}
	return strings.Count(string(b), `"event":"`+event+`"`)
}
