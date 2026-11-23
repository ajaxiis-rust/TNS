package turn

import (
	"context"
	"testing"
	"time"
)

// TestWillSaved — завещание (T-19-03, «Проверка»): текст героя со сцены
// прощания сохраняется в мире слоем will; без сцены прощания или без текста
// завещания нет.
func TestWillSaved(t *testing.T) {
	t.Run("прощание записывает завещание", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		seedHeroHealth(t, p, 2)

		st := &TurnState{
			Input: TurnInput{SessionID: 1, PlayerText: "Прими печать и правь вместо меня.", At: time.Now()},
			Reply: "обычный ответ",
		}
		if err := (&deathStage{db: p.DB}).Run(context.Background(), st); err != nil {
			t.Fatalf("death: %v", err)
		}
		if !st.Farewell {
			t.Fatal("сцена прощания не выдана")
		}
		will, ok, err := readWill(p.DB)
		if err != nil {
			t.Fatalf("readWill: %v", err)
		}
		if !ok {
			t.Fatal("завещание не записано")
		}
		if will.Text != st.Input.PlayerText {
			t.Fatalf("завещание = %q, want %q", will.Text, st.Input.PlayerText)
		}
	})

	t.Run("последние слова перекрывают прежние", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		seedHeroHealth(t, p, 2)

		for _, text := range []string{"Первые слова.", "Последние слова."} {
			st := &TurnState{Input: TurnInput{SessionID: 1, PlayerText: text, At: time.Now()}}
			if err := (&deathStage{db: p.DB}).Run(context.Background(), st); err != nil {
				t.Fatalf("death: %v", err)
			}
		}
		will, ok, err := readWill(p.DB)
		if err != nil {
			t.Fatalf("readWill: %v", err)
		}
		if !ok || will.Text != "Последние слова." {
			t.Fatalf("завещание = %q (ok=%v), want «Последние слова.»", will.Text, ok)
		}
	})

	t.Run("без текста завещания нет", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		seedHeroHealth(t, p, 2)

		st := &TurnState{Input: TurnInput{SessionID: 1, At: time.Now()}}
		if err := (&deathStage{db: p.DB}).Run(context.Background(), st); err != nil {
			t.Fatalf("death: %v", err)
		}
		if _, ok, err := readWill(p.DB); err != nil || ok {
			t.Fatalf("завещание без слов героя: ok=%v err=%v", ok, err)
		}
	})

	t.Run("до порога смерти завещания нет", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		seedHeroHealth(t, p, 50)

		st := &TurnState{Input: TurnInput{SessionID: 1, PlayerText: "Я ещё жив.", At: time.Now()}}
		if err := (&deathStage{db: p.DB}).Run(context.Background(), st); err != nil {
			t.Fatalf("death: %v", err)
		}
		if _, ok, err := readWill(p.DB); err != nil || ok {
			t.Fatalf("завещание записано до порога: ok=%v err=%v", ok, err)
		}
	})
}

// TestWillSignal — сигнал Селектору (T-19-03, «Проверка»): наличие завещания
// стадия выбора пути видит крючком следующей сцены (сцена уникальна — Ткач);
// без завещания решение прежнее.
func TestWillSignal(t *testing.T) {
	t.Run("селектор видит завещание", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		if err := saveWill(p.DB, "Прими печать и правь."); err != nil {
			t.Fatalf("saveWill: %v", err)
		}
		ch := make(chan string)
		close(ch)
		st := &TurnState{Input: TurnInput{SessionID: 1, PlayerText: "Я молчу.", At: time.Now()}}
		if err := (&selectStage{db: p.DB, weaver: &fakeProseSlot{ch: ch}}).Run(context.Background(), st); err != nil {
			t.Fatalf("select: %v", err)
		}
		if !st.WillHook {
			t.Fatal("селектор не увидел завещание")
		}
		if st.Path != "weaver" {
			t.Fatalf("путь = %q, want weaver", st.Path)
		}
		if st.SelectorDecision != "5 emotion" {
			t.Fatalf("решение = %q, want «5 emotion»", st.SelectorDecision)
		}
	})

	t.Run("без завещания поведение прежнее", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		ch := make(chan string)
		close(ch)
		st := &TurnState{Input: TurnInput{SessionID: 1, PlayerText: "Я молчу.", At: time.Now()}}
		if err := (&selectStage{db: p.DB, weaver: &fakeProseSlot{ch: ch}}).Run(context.Background(), st); err != nil {
			t.Fatalf("select: %v", err)
		}
		if st.WillHook {
			t.Fatal("сигнал завещания без завещания")
		}
		if st.Path != "fallback" || st.SelectorDecision != "7 uncertain" {
			t.Fatalf("решение = %q (%s), want «7 uncertain» (fallback)", st.SelectorDecision, st.Path)
		}
	})
}
