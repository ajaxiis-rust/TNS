package bank_test

import (
	"errors"
	"testing"

	"tns65/internal/bank"
	"tns65/internal/db"
)

// TestScenePattern — кэш сцен-узоров (T-19-12): один структурный ключ отдаёт
// заготовку из банка без слота, разные ключи идут обычным путём, счётчик
// попаданий узора растёт.
func TestScenePattern(t *testing.T) {
	_, clk, slot, litPath, logger := newTestBank(t)

	reader, err := db.OpenReaderAs(litPath, db.DBLiterary, db.WriterForge, logger)
	if err != nil {
		t.Fatalf("OpenReaderAs(literary): %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })

	cache := bank.NewPatternCache()
	if got := cache.Len(); got != 0 {
		t.Fatalf("новый кэш узоров не пуст: %d", got)
	}
	b := bank.New(reader, clk, slot, mapBinder{"[NPCname]": "Иван", "[PLACE]": "город"},
		bank.WithPatternCache(cache))

	// Узор торговли: участники, локация и тип сцены из оседания.
	trade := bank.ScenePattern{
		Participants: []string{"торговец", "герой"},
		Location:     "рынок",
		SceneType:    "торг",
	}

	// 1. Первый запрос — промах: обычный путь с записью нового узора, слот не вызван.
	if b.PatternKnown(trade) {
		t.Fatalf("узор известен до первого запроса")
	}
	first, err := b.PickScene(trade)
	if err != nil {
		t.Fatalf("первый запрос узора: %v", err)
	}
	if hasToken(first.Text) {
		t.Fatalf("первая отдача содержит токен-заменитель: %q", first.Text)
	}
	if got := b.PatternHits(trade); got != 0 {
		t.Fatalf("счётчик попаданий после промаха = %d, want 0", got)
	}
	if !b.PatternKnown(trade) {
		t.Fatalf("узор не записан после промаха")
	}

	// 2. Тот же ключ в другом порядке и регистре — отдача из банка без слота,
	// счётчик попаданий растёт.
	same := bank.ScenePattern{
		Participants: []string{" Герой ", "торговец"},
		Location:     "Рынок",
		SceneType:    "Торг",
	}
	second, err := b.PickScene(same)
	if err != nil {
		t.Fatalf("повторный запрос узора: %v", err)
	}
	if second.Text != first.Text {
		t.Fatalf("повтор узора дал другой текст: %q ≠ %q", second.Text, first.Text)
	}
	if hasToken(second.Text) {
		t.Fatalf("повторная отдача содержит токен-заменитель: %q", second.Text)
	}
	if got := b.PatternHits(trade); got != 1 {
		t.Fatalf("счётчик попаданий после повтора = %d, want 1", got)
	}

	// 3. Другой структурный ключ — обычный путь выбора: своя запись банка,
	// счётчик чужого узора не трогается.
	other := bank.ScenePattern{
		Participants: []string{"сторож"},
		Location:     "двор",
		SceneType:    "сенсорика",
	}
	third, err := b.PickScene(other)
	if err != nil {
		t.Fatalf("запрос другого узора: %v", err)
	}
	if third.Text == first.Text {
		t.Fatalf("другой узор отдал ту же запись: %q", third.Text)
	}
	if got := b.PatternHits(other); got != 0 {
		t.Fatalf("счётчик нового узора после промаха = %d, want 0", got)
	}
	if got := b.PatternHits(trade); got != 1 {
		t.Fatalf("счётчик первого узора сдвинулся: %d, want 1", got)
	}

	// 4. Повтор другого узора — попадание: счётчики узоров независимы.
	fourth, err := b.PickScene(other)
	if err != nil {
		t.Fatalf("повтор другого узора: %v", err)
	}
	if fourth.Text != third.Text {
		t.Fatalf("повтор другого узора дал другой текст: %q ≠ %q", fourth.Text, third.Text)
	}
	if b.PatternHits(other) != 1 || b.PatternHits(trade) != 1 {
		t.Fatalf("счётчики узоров = %d/%d, want 1/1", b.PatternHits(other), b.PatternHits(trade))
	}

	// 5. Возврат к первому узору — попадание, счётчик растёт дальше.
	fifth, err := b.PickScene(trade)
	if err != nil {
		t.Fatalf("возврат к первому узору: %v", err)
	}
	if fifth.Text != first.Text {
		t.Fatalf("возврат к узору дал другой текст: %q ≠ %q", fifth.Text, first.Text)
	}
	if got := b.PatternHits(trade); got != 2 {
		t.Fatalf("счётчик попаданий первого узора = %d, want 2", got)
	}
	if got := cache.Len(); got != 2 {
		t.Fatalf("узоров в кэше = %d, want 2", got)
	}

	// Слот генерации за все запросы узоров не вызван ни разу.
	if calls := slot.Calls(); calls != 0 {
		t.Fatalf("вызовов слота генерации = %d, want 0", calls)
	}

	// 6. Узор без структуры — ошибка, а не запись кэша.
	if _, err := b.PickScene(bank.ScenePattern{}); !errors.Is(err, bank.ErrEmptyPattern) {
		t.Fatalf("пустой узор = %v, want ErrEmptyPattern", err)
	}
}
