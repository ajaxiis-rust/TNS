package turn

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"tns65/internal/models"
	"tns65/internal/testkit"
)

// fakeProseSlot — слот прозы для теста: готовый канал чанков, ошибка отказа и
// запись полученного промпта (для проверки «промпт без токенов-заменителей»).
type fakeProseSlot struct {
	mu      sync.Mutex
	ch      chan string
	err     error
	prompts []string
}

func (s *fakeProseSlot) Stream(_ context.Context, prompt string) (<-chan string, error) {
	s.mu.Lock()
	s.prompts = append(s.prompts, prompt)
	s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	return s.ch, nil
}

func (s *fakeProseSlot) lastPrompt() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.prompts) == 0 {
		return ""
	}
	return s.prompts[len(s.prompts)-1]
}

// notifiedClock — testkit.Clock, сигналящая о каждой подписке After: тест ждёт
// регистрации таймера порога и лишь затем двигает часы (без гонки с горутиной).
type notifiedClock struct {
	inner *testkit.Clock
	after chan struct{}
}

func newNotifiedClock() *notifiedClock {
	return &notifiedClock{inner: testkit.NewClock(), after: make(chan struct{}, 8)}
}

func (c *notifiedClock) After(d time.Duration) <-chan time.Time {
	c.after <- struct{}{}
	return c.inner.After(d)
}

func (c *notifiedClock) Advance(d time.Duration) { c.inner.Advance(d) }

// weaverResult — результат streamWeaver для передачи через канал.
type weaverResult struct {
	reply    string
	degraded bool
}

func TestSilenceThreshold(t *testing.T) {
	ctx := context.Background()

	t.Run("молчание дольше 60 секунд даёт запасную запись", func(t *testing.T) {
		clk := newNotifiedClock()
		ch := make(chan string, 1)
		ch <- "Один чанк, дальше тишина"
		slot := &fakeProseSlot{ch: ch}

		done := make(chan weaverResult, 1)
		go func() {
			reply, degraded := streamWeaver(ctx, slot, "промпт", clk, fallbackReply)
			done <- weaverResult{reply: reply, degraded: degraded}
		}()

		// Дождаться подписки таймера порога (после чтения первого чанка) и
		// продвинуть часы за 60 секунд: срабатывает запасная запись.
		<-clk.after
		<-clk.after
		clk.Advance(SilenceThreshold + time.Second)

		res := <-done
		if res.reply != fallbackReply {
			t.Fatalf("при молчании ответ = %q, ожидалась запасная запись %q", res.reply, fallbackReply)
		}
		if !res.degraded {
			t.Fatalf("при молчании слота признак деградации не выставлен")
		}
	})

	t.Run("после деградации слота ход доставлен", func(t *testing.T) {
		clk := newNotifiedClock()
		slot := &fakeProseSlot{err: errors.New("провайдер недоступен")}

		done := make(chan weaverResult, 1)
		go func() {
			reply, degraded := streamWeaver(ctx, slot, "промпт", clk, fallbackReply)
			done <- weaverResult{reply: reply, degraded: degraded}
		}()

		res := <-done
		if res.reply == "" {
			t.Fatalf("после деградации слота ход не доставлен (пустой ответ)")
		}
		if res.reply != fallbackReply {
			t.Fatalf("после деградации ответ = %q, ожидалась запасная запись %q", res.reply, fallbackReply)
		}
		if !res.degraded {
			t.Fatalf("после деградации слота признак деградации не выставлен")
		}
	})

	t.Run("порог 60 секунд и промпт без токенов через путь Ткача", func(t *testing.T) {
		clk := newNotifiedClock()
		slot := &fakeProseSlot{ch: make(chan string)} // слот молчит с самого старта.

		stage := &executeStage{weaver: slot, clock: clk}
		st := &TurnState{
			Path: pathWeaver,
			Prepath: Prepath{
				Prompt: "Иван смотрит на тебя у ворот.",
			},
		}

		done := make(chan error, 1)
		go func() {
			done <- stage.Run(ctx, st)
		}()

		// Слот молчит: регистрируется один таймер порога, затем — сдвиг часов.
		<-clk.after
		clk.Advance(SilenceThreshold + time.Second)

		if err := <-done; err != nil {
			t.Fatalf("Run: %v", err)
		}
		if st.Reply == "" {
			t.Fatalf("ход не доставлен (пустой ответ)")
		}
		if st.Reply != fallbackReply {
			t.Fatalf("при молчании ответ = %q, ожидалась запасная запись %q", st.Reply, fallbackReply)
		}
		if !st.Degraded {
			t.Fatalf("при молчании слота признак деградации не выставлен")
		}

		// Промпт, ушедший слоту, собран без токенов-заменителей.
		prompt := slot.lastPrompt()
		if prompt == "" {
			t.Fatalf("слот не получил промпт")
		}
		if models.HasReplacementTokens(prompt) {
			t.Fatalf("промпт слота содержит токен-заменитель: %q", prompt)
		}
		if !strings.Contains(prompt, models.WeaverInstruction) {
			t.Fatalf("промпт не содержит инструкцию движка: %q", prompt)
		}
	})

	t.Run("полная генерация возвращает собранный текст", func(t *testing.T) {
		clk := newNotifiedClock()
		ch := make(chan string, 2)
		ch <- "Смотрит "
		ch <- "на тебя."
		close(ch)
		slot := &fakeProseSlot{ch: ch}

		reply, degraded := streamWeaver(ctx, slot, "промпт", clk, fallbackReply)
		if reply != "Смотрит на тебя." {
			t.Fatalf("ответ = %q, ожидался %q", reply, "Смотрит на тебя.")
		}
		if degraded {
			t.Fatalf("полная генерация ошибочно помечена деградацией")
		}
	})
}
