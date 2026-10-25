package bank_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/bank"
	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/testkit"
)

// mapBinder — биндер фактов для теста: закрывает токены-заменители формы
// значениями мира (имена, места). Банк хранит форму, мир — факты (LT-08.5).
type mapBinder map[string]string

func (m mapBinder) Bind(text string) string {
	out := text
	for token, value := range m {
		out = strings.ReplaceAll(out, token, value)
	}
	return out
}

// hasToken — остался ли в тексте токен-заменитель формы.
func hasToken(text string) bool {
	return strings.Contains(text, "[") || strings.Contains(text, "]")
}

// newTestBank создаёт литературную базу во временном каталоге, наполняет её
// записями bank_pool хэндлом forge и возвращает Челнок над читателем этой базы.
func newTestBank(t *testing.T) (*bank.Bank, *testkit.Clock, *testkit.FakeSlot, string, *journal.Logger) {
	t.Helper()
	dir := t.TempDir()
	litPath := filepath.Join(dir, "literary.db")

	raw, err := db.OpenWriter(litPath)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(raw, "literary"); err != nil {
		_ = raw.Close()
		t.Fatalf("Migrate(literary): %v", err)
	}
	_ = raw.Close()

	logger, err := journal.New(filepath.Join(dir, "engine.log"))
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	// Сид bank_pool — только работа мастерской, писатель forge (T-01-07).
	forge, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, logger)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	for _, rec := range []struct{ kind, text, tags string }{
		{"routine", "[NPCname] приветствует тебя у [PLACE].", "приветствие,торг"},
		{"routine", "[NPCname] торгуется за прилавком.", "торг,сенсорика"},
		{"routine", "Пахнет травами и дымом.", "сенсорика"},
	} {
		if _, err := forge.Exec("bank_pool",
			`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at) VALUES (?, ?, 'ru', ?, 2, 'seed', '2026-01-01T00:00:00Z')`,
			rec.kind, rec.text, rec.tags); err != nil {
			t.Fatalf("seed bank_pool: %v", err)
		}
	}
	_ = forge.Close()

	reader, err := db.OpenReaderAs(litPath, db.DBLiterary, db.WriterForge, logger)
	if err != nil {
		t.Fatalf("OpenReaderAs(literary): %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })

	clk := testkit.NewClock()
	slot := testkit.NewFakeSlot()
	b := bank.New(reader, clk, slot, mapBinder{"[NPCname]": "Иван", "[PLACE]": "город"})
	return b, clk, slot, litPath, logger
}

func TestBank(t *testing.T) {
	b, clk, slot, litPath, logger := newTestBank(t)

	// 1. Два прохода по одной сцене дают идентичный текст из bank_pool.
	start := clk.Now()
	r1, err := b.Pick([]string{"торг", "приветствие"})
	if err != nil {
		t.Fatalf("первый проход: %v", err)
	}
	r2, err := b.Pick([]string{"торг", "приветствие"})
	if err != nil {
		t.Fatalf("второй проход: %v", err)
	}
	if r1.Text != r2.Text {
		t.Fatalf("два прохода дали разный текст: %q ≠ %q", r1.Text, r2.Text)
	}

	// 2. Выборка ≤ 100 мс на подставных часах.
	if elapsed := clk.Now().Sub(start); elapsed > bank.Budget {
		t.Fatalf("выборка заняла %v, больше бюджета %v", elapsed, bank.Budget)
	}

	// 3. Вызовов слота генерации 0.
	if calls := slot.Calls(); calls != 0 {
		t.Fatalf("вызовов слота генерации = %d, want 0", calls)
	}

	// 4. Токены-заменители в выданном тексте отсутствуют.
	if hasToken(r2.Text) {
		t.Fatalf("выданный текст содержит токен-заменитель: %q", r2.Text)
	}

	// Повторяемое: тот же набор тегов в пределах трёх последних ходов — та же
	// запись (T-03-07, шаг 1).
	_, _ = b.Pick([]string{"сенсорика"})
	_, _ = b.Pick([]string{"сенсорика"})
	r3, err := b.Pick([]string{"торг", "приветствие"})
	if err != nil {
		t.Fatalf("повторный проход после двух иных ходов: %v", err)
	}
	if r3.Text != r1.Text {
		t.Fatalf("повторяемое нарушено: %q ≠ %q", r3.Text, r1.Text)
	}

	// 5. Попытка записи в bank_pool хэндлом core даёт ErrForeignWriter.
	coreH, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterCore, logger)
	if err != nil {
		t.Fatalf("OpenWriterAs(core): %v", err)
	}
	t.Cleanup(func() { _ = coreH.Close() })
	if _, err := coreH.Exec("bank_pool",
		`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at) VALUES ('routine', 'x', 'ru', 'x', 2, 'seed', '2026-01-01T00:00:00Z')`); !errors.Is(err, db.ErrForeignWriter) {
		t.Fatalf("запись core в bank_pool = %v, want ErrForeignWriter", err)
	}
}
