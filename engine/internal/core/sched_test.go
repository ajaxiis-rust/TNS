package core_test

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"tns65/internal/core"
	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// TestTurnPriority — бюджет четырёх ядер с приоритетом хода (T-03-17, «Проверка»):
// при активном ходе запросов мастерской к слоту ноль, семафор задач не выше трёх,
// после хода задачи продолжаются. Тест на семафоре и подставном слоте testkit;
// задержки движутся подставными часами, а не реальным временем.
func TestTurnPriority(t *testing.T) {
	t.Run("активный ход держит слот: запросов к слоту ноль, после хода задачи идут", func(t *testing.T) {
		h := openWorkshopHandle(t)
		sched := core.NewScheduler()
		slot := testkit.NewFakeSlot()
		slot.SetResponse("", "ок")

		workshop.SetScheduler(sched)
		t.Cleanup(func() { workshop.SetScheduler(nil) })
		workshop.SetSlot(slot)
		t.Cleanup(func() { workshop.SetSlot(nil) })

		guard := sched.BeginTurn()

		var wg sync.WaitGroup
		kinds := []workshop.Kind{workshop.KindSediment, workshop.KindStyle}
		for _, k := range kinds {
			wg.Add(1)
			go func(k workshop.Kind) {
				defer wg.Done()
				_ = workshop.Enqueue(h, k, workshop.StepLight, workshop.DefaultProvider, map[string]string{"kind": string(k)})
			}(k)
		}

		// Обе задачи дошли до планировщика и стоят у сжатого семафора.
		waitFor(t, "две задачи ждут слот", func() bool {
			return sched.WorkshopWaiting() == 2
		})

		if got := slot.Calls(); got != 0 {
			t.Fatalf("запросов к слоту при активном ходе = %d, want 0", got)
		}
		if got := sched.WorkshopBusy(); got != 0 {
			t.Fatalf("занятых слотов при активном ходе = %d, want 0", got)
		}

		guard.End()
		wg.Wait()

		if got := slot.Calls(); got != 2 {
			t.Fatalf("запросов к слоту после хода = %d, want 2", got)
		}
		if got := sched.WorkshopBusy(); got != 0 {
			t.Fatalf("занятых слотов после хода = %d, want 0", got)
		}
		if got := sched.WorkshopWaiting(); got != 0 {
			t.Fatalf("ожидающих слот после хода = %d, want 0", got)
		}
	})

	t.Run("семафор мастерской не выше трёх задач", func(t *testing.T) {
		h := openWorkshopHandle(t)
		sched := core.NewScheduler()
		clock := testkit.NewClock()
		slot := testkit.NewFakeSlot()
		slot.SetResponse("", "ок")
		slot.SetClock(clock)
		slot.SetDelay(time.Second)

		workshop.SetScheduler(sched)
		t.Cleanup(func() { workshop.SetScheduler(nil) })
		workshop.SetSlot(slot)
		t.Cleanup(func() { workshop.SetSlot(nil) })

		kinds := []workshop.Kind{
			workshop.KindAnalysis, workshop.KindForgePlan, workshop.KindComplement,
			workshop.KindHoroscope, workshop.KindPythia,
		}
		var wg sync.WaitGroup
		for _, k := range kinds {
			wg.Add(1)
			go func(k workshop.Kind) {
				defer wg.Done()
				_ = workshop.Enqueue(h, k, workshop.StepLight, workshop.DefaultProvider, map[string]string{"kind": string(k)})
			}(k)
		}

		// Три слота заняты, две задачи стоят в очереди к семафору.
		waitFor(t, "три слота заняты и две задачи ждут", func() bool {
			return sched.WorkshopBusy() == core.WorkshopSlots && sched.WorkshopWaiting() == 2
		})

		// Слот считает вызов до подписки на часы, поэтому перед Advance ждём
		// фактической регистрации таймеров трёх занятых слотов — иначе Advance
		// пройдёт мимо ещё не зарегистрированного таймера и задача не выпустится.
		waitFor(t, "три таймера занятых слотов зарегистрированы", func() bool {
			return clock.PendingTimers() >= core.WorkshopSlots
		})
		if got := slot.Calls(); got != core.WorkshopSlots {
			t.Fatalf("вошло в слот = %d, want %d", got, core.WorkshopSlots)
		}

		// Выпускаем первую тройку по подставным часам; следом входят две оставшиеся.
		clock.Advance(time.Second)
		waitFor(t, "все пять задач вошли в слот", func() bool {
			return slot.Calls() == len(kinds)
		})
		// Та же гонка для второй пары: ждём регистрации двух новых таймеров.
		waitFor(t, "два новых таймера зарегистрированы", func() bool {
			return clock.PendingTimers() >= len(kinds)-core.WorkshopSlots
		})
		clock.Advance(time.Second)
		wg.Wait()

		if got := slot.Calls(); got != len(kinds) {
			t.Fatalf("всего запросов к слоту = %d, want %d", got, len(kinds))
		}
		if got := sched.WorkshopBusy(); got != 0 {
			t.Fatalf("занятых слотов в конце = %d, want 0", got)
		}
		if got := sched.WorkshopWaiting(); got != 0 {
			t.Fatalf("ожидающих слот в конце = %d, want 0", got)
		}
	})

	t.Run("ход не ждёт освобождения слота: BeginTurn при занятых трёх", func(t *testing.T) {
		h := openWorkshopHandle(t)
		sched := core.NewScheduler()
		slot := testkit.NewFakeSlot()
		slot.SetResponse("", "ок")

		workshop.SetScheduler(sched)
		t.Cleanup(func() { workshop.SetScheduler(nil) })
		workshop.SetSlot(slot)
		t.Cleanup(func() { workshop.SetSlot(nil) })

		// Заняли все три слота напрямую, не трогая слот генерации.
		release := []func(){sched.AcquireWorkshop(), sched.AcquireWorkshop(), sched.AcquireWorkshop()}
		if got := sched.WorkshopBusy(); got != core.WorkshopSlots {
			t.Fatalf("занято слотов = %d, want %d", got, core.WorkshopSlots)
		}

		// BeginTurn при занятых слотах возвращается немедленно: ход не ждёт ковки.
		guardCh := make(chan *core.TurnGuard, 1)
		go func() { guardCh <- sched.BeginTurn() }()
		var guard *core.TurnGuard
		select {
		case guard = <-guardCh:
		case <-time.After(time.Second):
			t.Fatalf("BeginTurn ждёт освобождения слотов мастерской")
		}

		// Пока идёт ход, слот генерации не получает ни одного запроса.
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = workshop.Enqueue(h, workshop.KindStyle, workshop.StepLight, workshop.DefaultProvider, map[string]string{"kind": string(workshop.KindStyle)})
		}()
		waitFor(t, "задача ждёт конца хода", func() bool {
			return sched.WorkshopWaiting() == 1
		})
		if got := slot.Calls(); got != 0 {
			t.Fatalf("запросов к слоту при ходе = %d, want 0", got)
		}

		// После End хода и освобождения слотов задача продолжается.
		guard.End()
		for _, rel := range release {
			rel()
		}
		wg.Wait()
		if got := slot.Calls(); got != 1 {
			t.Fatalf("запросов к слоту после хода = %d, want 1", got)
		}
	})
}

// openWorkshopHandle — фикстурная оперативная база мира с миграциями (как в
// workshop.TestQueueKinds): запись через хэндл core, без рождения.
func openWorkshopHandle(t *testing.T) *db.Handle {
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
	_ = w.Close()

	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// waitFor ждёт условия с ограничением по реальному времени. Задержки в тесте
// движутся подставными часами; реальное время здесь — лишь страховка синхронизации
// горутин, работающих с БД и слотом.
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
