package workshop_test

import (
	"math"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// TestGlobalLuck — globalLuck: вес удачи в событийных механиках (T-05-11,
// «Проверка»): таблица Weight для p=0.4; смена L меняет исход броска с
// фиксированным r; свершившееся событие не пересчитано; L=1.5 отклонено; в
// расчёте нет вызовов слота.
func TestGlobalLuck(t *testing.T) {
	// Таблица Weight для p=0.4 (шаг 2).
	table := []struct {
		L    float64
		want float64
	}{
		{0, 0.2},
		{0.4, 0.36},
		{0.5, 0.4},
		{1, 0.6},
	}
	for _, c := range table {
		if got := workshop.Weight(0.4, c.L); math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("Weight(0.4, %v) = %v, want %v", c.L, got, c.want)
		}
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "world(a).db")

	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(w, "world", "shared"); err != nil {
		w.Close()
		t.Fatalf("Migrate: %v", err)
	}
	w.Close()

	// world_meta пишет только forge (карта писателей T-01-07).
	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	defer h.Close()

	// Подставной слот — доказательство «без модели»: расчёт его не вызывает.
	slot := testkit.NewFakeSlot()
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	// Бросок детерминирован по слагу (шаг 2): один слаг — одна последовательность.
	other := workshop.NewLuck(h, "a")
	if other.Draw() != workshop.NewLuck(h, "a").Draw() {
		t.Fatal("ГПСЧ мира не детерминирован по слагу")
	}

	luck := workshop.NewLuck(h, "a")

	// Дефолт — нейтральный 0.5 (шаг 1).
	if got, err := luck.Get(); err != nil || got != workshop.DefaultLuck {
		t.Fatalf("Get() по умолчанию = (%v, %v), want (%v, nil)", got, err, workshop.DefaultLuck)
	}

	// Значение вне диапазона [0,1] отклоняется при записи (шаг 5).
	if err := luck.Set(1.5); err == nil {
		t.Fatal("Set(1.5) принят, want отказ")
	}
	if err := luck.Set(-0.1); err == nil {
		t.Fatal("Set(-0.1) принят, want отказ")
	}

	// Запись в диапазоне применяется (шаги 1, 4).
	if err := luck.Set(0.4); err != nil {
		t.Fatalf("Set(0.4): %v", err)
	}
	if got, err := luck.Get(); err != nil || math.Abs(got-0.4) > 1e-9 {
		t.Fatalf("Get() после Set(0.4) = (%v, %v), want (0.4, nil)", got, err)
	}

	// Смена L меняет исход броска с фиксированным r (шаг 2, «Проверка»): берём
	// детерминированный бросок r из ГСЧ мира и подбираем p так, чтобы веса при
	// L=0 и L=1 легли по разные стороны от r.
	if err := luck.Set(0); err != nil {
		t.Fatalf("Set(0): %v", err)
	}
	r := luck.Draw()
	p := r
	if p <= 0 {
		p = 0.5
	}
	if firedLow := r < workshop.Weight(p, 0); firedLow {
		t.Fatalf("L=0 должен быть против (r=%v, p=%v), got fired", r, p)
	}
	if firedHigh := r < workshop.Weight(p, 1); !firedHigh {
		t.Fatalf("L=1 должен быть за (r=%v, p=%v), got not fired", r, p)
	}

	// Свершившееся событие не пересчитывается при смене L (шаг 4): исход,
	// рассчитанный при старом L, остаётся прежним.
	if err := luck.Set(0); err != nil {
		t.Fatalf("Set(0) повторно: %v", err)
	}
	first, err := luck.Roll(0.4)
	if err != nil {
		t.Fatalf("Roll(0.4): %v", err)
	}
	if err := luck.Set(1); err != nil {
		t.Fatalf("Set(1): %v", err)
	}
	if first.Fired != (first.R < workshop.Weight(0.4, 0)) {
		t.Fatalf("свершившееся событие пересчитано: Fired=%v при r=%v", first.Fired, first.R)
	}

	// Без модели: в расчёте нет вызовов слота.
	if slot.Calls() != 0 {
		t.Fatalf("вызовов слота = %d, want 0", slot.Calls())
	}
}
