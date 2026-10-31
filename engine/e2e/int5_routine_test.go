//go:build e2e

package e2e

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"tns65/internal/db"
)

// bankRoutineText — запись банка рутины, которую Челнок выдаёт ровному участку.
const bankRoutineText = "Ты в постели. Всё тихо, ночь бережёт сон."

// TestINT5 — INT-5. Скорость рутины (T-09-05): ровная реплика идёт путём Челнока
// (банк), внешний слот не вызывается, повтор выдаёт ту же запись банка побайтово,
// латентность укладывается в норматив 2000 мс.
func TestINT5(t *testing.T) {
	bin := buildBinary(t, []string{"e2e"})
	dir := worldDir(t)
	linkConfig(t, dir)
	linkPacks(t, dir)

	// Подставной слот генерации: считает запросы /completion. Путь Челнока его
	// не трогает — внешних вызовов ноль (INT-5, шаги 2 и 4).
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("штатная проза внешнего слота"))
	}))

	slug := "skorost-rutiny"
	ancketa := writeAncketa(t, t.TempDir(), slug,
		"Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.")
	stdout, stderr, code := runCmd(t, dir, bin, "world", "birth", "--ancketa", ancketa, "--fake-embed", "--fake-slot")
	if code != 0 {
		t.Fatalf("world birth exit=%d, want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	// Шаг 1: наполненный банк рутины — запись в bank_pool литературной базы
	// хэндлом forge (карта писателей T-01-07).
	literaryDB := filepath.Join(dir, "world_db", "worlds", slug, "literary.db")
	seedBankRoutine(t, literaryDB)

	worldDB := worldDBPath(dir, slug)
	c := startCore(t, dir, bin, "--fake-clock", "--fake-slot", srv.URL)

	// Ровный участок: намерение «сон» — рутина, банк отвечает без генерации.
	const replica = "Я хочу лечь спать в постель."
	c.postTurn(t, slug, replica)
	c.postTurn(t, slug, replica)

	report := map[string]any{}

	turns := lastTurns(t, worldDB, 2)
	if len(turns) != 2 {
		t.Fatalf("ходов в turns = %d, want 2", len(turns))
	}

	// Шаг 2: путь — банк; внешних вызовов слота ноль.
	report["path"] = turns[0].Path
	if turns[0].Path != "bank" || turns[1].Path != "bank" {
		t.Fatalf("path = %q/%q, want bank/bank", turns[0].Path, turns[1].Path)
	}
	external := atomic.LoadInt64(&calls)
	report["external_calls"] = external
	if external != 0 {
		t.Fatalf("внешних вызовов слота = %d, want 0", external)
	}

	// Шаг 3: повтор выдаёт ту же запись банка побайтово; дрейфа живой модели нет.
	report["same_record"] = turns[0].ReplyText == turns[1].ReplyText
	if !report["same_record"].(bool) {
		t.Fatalf("повтор выдал другой текст: %q ≠ %q", turns[0].ReplyText, turns[1].ReplyText)
	}
	// Ответ — именно запись банка, не страховка (путь Челнока реально сработал).
	if turns[0].ReplyText != bankRoutineText {
		t.Fatalf("ответ = %q, want запись банка %q", turns[0].ReplyText, bankRoutineText)
	}

	// Шаг 2: латентность рутины укладывается в норматив 2000 мс.
	latency := lastTurnLatencyMS(t, worldDB)
	report["latency_ms"] = latency
	if latency >= 2000 {
		t.Fatalf("latency_ms = %d, want < 2000", latency)
	}

	writeReport(t, "int5", report)
}

// seedBankRoutine кладёт одну запись банка рутины в bank_pool литературной базы
// хэндлом forge — сид банка пишет только мастерская (T-01-07).
func seedBankRoutine(t *testing.T, literaryDB string) {
	t.Helper()
	h, err := db.OpenWriterAs(literaryDB, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("открыть литературную базу для сида: %v", err)
	}
	defer h.Close()
	if _, err := h.Exec("bank_pool",
		`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at) VALUES ('routine', ?, 'ru', ?, 2, 'seed', '2026-01-01T00:00:00Z')`,
		bankRoutineText, "сон"); err != nil {
		t.Fatalf("seed bank_pool: %v", err)
	}
}

// lastTurnLatencyMS — латентность последнего хода из turns.
func lastTurnLatencyMS(t *testing.T, worldDB string) int64 {
	t.Helper()
	h := openWorldDB(t, worldDB)
	defer h.Close()
	var ms int64
	if err := h.QueryRow(`SELECT latency_ms FROM turns ORDER BY id DESC LIMIT 1`).Scan(&ms); err != nil {
		t.Fatal(err)
	}
	return ms
}
