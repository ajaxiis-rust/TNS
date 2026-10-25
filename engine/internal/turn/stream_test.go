package turn

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// gatedSlot — слот Ткача, чьи чанки отдаются только по команде теста: сигналом
// started помечается начало генерации, сам поток остаётся открытым до close(ch).
type gatedSlot struct {
	started chan struct{}
	ch      chan string
}

func (s *gatedSlot) Stream(_ context.Context, _ string) (<-chan string, error) {
	close(s.started)
	return s.ch, nil
}

func TestStream(t *testing.T) {
	t.Run("подкладка приходит до завершения генерации Ткача", func(t *testing.T) {
		clk := testkit.NewClock()
		slot := &gatedSlot{started: make(chan struct{}), ch: make(chan string)}
		st := &TurnState{Prepath: Prepath{Prompt: "Иван смотрит, как ты входишь в город. Судьба молчит."}}

		portions := streamScene(context.Background(), st, slot, "промпт", clk)

		// Первые порции — мгновенная подкладка, без единого чанка Ткача.
		var underlays []Portion
		for i, n := 0, len(underlayPortions(st)); i < n; i++ {
			p := <-portions
			if p.Kind != portionUnderlay {
				t.Fatalf("порция %d = %q, ожидалась подкладка", i, p.Kind)
			}
			if strings.TrimSpace(p.Text) == "" {
				t.Fatalf("подкладка %d пуста", i)
			}
			underlays = append(underlays, p)
		}

		// Ткач начал генерацию (Stream гарантированно вызван после подкладки), но ни
		// одного живого фрагмента ещё не отдал.
		<-slot.started
		select {
		case p := <-portions:
			if p.Kind == portionWeaver {
				t.Fatal("порция Ткача пришла до завершения генерации")
			}
		default:
		}

		// Завершаем генерацию: живой фрагмент доставляется следом за подкладкой.
		slot.ch <- "Он шагает навстречу."
		close(slot.ch)

		var weaver []Portion
		for p := range portions {
			if p.Kind == portionWeaver {
				weaver = append(weaver, p)
			}
		}
		if len(weaver) != 1 {
			t.Fatalf("живых порций Ткача = %d, ожидалась 1", len(weaver))
		}
		if weaver[0].Text != "Он шагает навстречу." {
			t.Fatalf("живой фрагмент = %q", weaver[0].Text)
		}
		if weaver[0].Degraded {
			t.Fatalf("успешная генерация ошибочно помечена деградацией")
		}
		if len(underlays) == 0 {
			t.Fatalf("подкладка не доставлена")
		}
	})

	t.Run("игрок получает читаемый поток", func(t *testing.T) {
		clk := testkit.NewClock()
		ch := make(chan string, 2)
		ch <- "Он смотрит "
		ch <- "в твою сторону."
		close(ch)
		slot := &fakeProseSlot{ch: ch}
		st := &TurnState{Prepath: Prepath{Prompt: "Иван смотрит, как ты входишь в город. Судьба молчит."}}

		var parts []string
		for p := range streamScene(context.Background(), st, slot, "промпт", clk) {
			parts = append(parts, p.Text)
		}
		reply := strings.Join(parts, "")
		if strings.TrimSpace(reply) == "" {
			t.Fatal("поток пуст")
		}
		if strings.Contains(reply, "[") && strings.Contains(reply, "]") {
			t.Fatalf("поток содержит токен-заменитель: %q", reply)
		}
		if !strings.Contains(reply, "Иван смотрит") {
			t.Fatalf("в потоке нет описания подкладки: %q", reply)
		}
		if !strings.Contains(reply, "Он смотрит в твою сторону.") {
			t.Fatalf("в потоке нет живого фрагмента Ткача: %q", reply)
		}
	})

	t.Run("невошедшая ветка не пишется в scenes и уходит заказом sediment", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		lex := stopCraneLexicon(t)
		cfg := DefaultConfirmConfig()
		roster := Roster{
			NPCs: []NPC{
				{ID: 1, Name: "Иван", Role: "кузнец", Group: "ремесленник"},
				{ID: 2, Name: "Судьба", Role: "судьба", Deity: true},
			},
			Places: []Place{{ID: 10, Name: "город"}},
		}
		ch := make(chan string, 1)
		ch <- "Он шагает навстречу."
		close(ch)
		slot := &fakeProseSlot{ch: ch}

		stages := []Stage{
			&cutStage{logger: p.Logger},
			&prepathStage{db: p.DB, lexicon: lex, binder: NewSlotBinder(roster)},
			&confirmStage{Config: cfg},
			&selectStage{weaver: slot},
			&executeStage{weaver: slot},
			&safetyStage{},
			&commitStage{db: p.DB},
		}

		st := &TurnState{
			Input:      TurnInput{SessionID: 42, PlayerText: "идти ехать плыть бежать Иван в город", At: time.Now()},
			Embedder:   constEmbed{},
			Lemmatizer: wordLemmatizer{},
		}
		for _, s := range stages {
			if err := s.Run(context.Background(), st); err != nil {
				t.Fatalf("%s: %v", s.Name(), err)
			}
		}

		// Невошедшая (неподтверждённая) ветка в scenes не пишется: только одна
		// сцена выбранного хода.
		if n := tableCount(t, p, "scenes"); n != 1 {
			t.Fatalf("строк в scenes = %d, ожидалась 1 (кандидат не пишется)", n)
		}

		// После закрытия сцены в workshop_queue стоит ровно один заказ sediment.
		var n int
		if err := p.DB.QueryRow(`SELECT COUNT(*) FROM workshop_queue WHERE kind = ?`, string(workshop.KindSediment)).Scan(&n); err != nil {
			t.Fatalf("COUNT(workshop_queue sediment): %v", err)
		}
		if n != 1 {
			t.Fatalf("заказов sediment = %d, ожидался 1", n)
		}

		var kind, payload string
		var priority int
		if err := p.DB.QueryRow(`SELECT kind, priority, payload_json FROM workshop_queue WHERE kind = ?`, string(workshop.KindSediment)).Scan(&kind, &priority, &payload); err != nil {
			t.Fatalf("workshop_queue sediment query: %v", err)
		}
		if priority != 50 {
			t.Fatalf("приоритет заказа sediment = %d, ожидался 50", priority)
		}

		// Payload — ссылка на сцену-кандидата: ветка, не вошедшая в ход.
		var c SedimentCandidate
		if err := json.Unmarshal([]byte(payload), &c); err != nil {
			t.Fatalf("payload заказа sediment не JSON: %v", err)
		}
		if c.TurnID == 0 {
			t.Fatalf("payload без turn_id: %s", payload)
		}
		if c.SessionID != st.Input.SessionID {
			t.Fatalf("session_id кандидата = %d, ожидался %d", c.SessionID, st.Input.SessionID)
		}
		if c.Kind != BranchBank {
			t.Fatalf("вид кандидата = %q, ожидался %q", c.Kind, BranchBank)
		}
	})
}
