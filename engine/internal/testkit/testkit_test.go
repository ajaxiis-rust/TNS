package testkit

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestClock(t *testing.T) {
	c := NewClock()
	start := c.Now()

	c.Advance(5 * time.Second)
	if got := c.Now(); !got.Equal(start.Add(5 * time.Second)) {
		t.Fatalf("после Advance(5s): now = %v, want %v", got, start.Add(5*time.Second))
	}

	ch := c.After(10 * time.Second)
	c.Advance(9 * time.Second)
	select {
	case <-ch:
		t.Fatalf("After(10s) сработал после 9s")
	default:
	}
	c.Advance(1 * time.Second)
	select {
	case got := <-ch:
		if want := start.Add(15 * time.Second); !got.Equal(want) {
			t.Fatalf("таймер сработал в %v, want %v", got, want)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("таймер не сработал после пересечения дедлайна")
	}
}

func TestFakeSlot(t *testing.T) {
	s := NewFakeSlot()
	s.SetResponse("weaver", "текст от ткача")
	s.SetResponse("", "ответ по умолчанию")

	got, err := s.Generate(context.Background(), "weaver", "промпт")
	if err != nil {
		t.Fatalf("Generate(weaver): %v", err)
	}
	if got != "текст от ткача" {
		t.Fatalf("Generate(weaver) = %q, want %q", got, "текст от ткача")
	}

	got, err = s.Generate(context.Background(), "unknown", "промпт")
	if err != nil {
		t.Fatalf("Generate(unknown): %v", err)
	}
	if got != "ответ по умолчанию" {
		t.Fatalf("Generate(unknown) = %q, want %q", got, "ответ по умолчанию")
	}

	if s.Calls() != 2 {
		t.Fatalf("Calls = %d, want 2", s.Calls())
	}

	s.FailWith(ErrTimeout)
	if _, err := s.Generate(context.Background(), "weaver", "промпт"); !errors.Is(err, ErrTimeout) {
		t.Fatalf("после FailWith(ErrTimeout): err = %v", err)
	}
	if s.Calls() != 3 {
		t.Fatalf("Calls после отказа = %d, want 3", s.Calls())
	}

	s.FailWith(nil)
	c := NewClock()
	s.SetClock(c)
	s.SetDelay(60 * time.Second)

	done := make(chan string, 1)
	go func() {
		out, _ := s.Generate(context.Background(), "weaver", "промпт")
		done <- out
	}()

	// До продвижения часов ответа нет: слот выжидает задержку.
	waitPendingTimer(t, c, 1)
	select {
	case <-done:
		t.Fatalf("Generate вернулся до продвижения часов")
	default:
	}

	c.Advance(60 * time.Second)
	select {
	case out := <-done:
		if out != "текст от ткача" {
			t.Fatalf("после задержки: got %q, want %q", out, "текст от ткача")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Generate не вернулся после продвижения часов")
	}
}

func TestFakeEmbed(t *testing.T) {
	e := NewFakeEmbed()
	v1, err := e.Embed(context.Background(), "синопсис мира")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(v1) != EmbedDim {
		t.Fatalf("len(vector) = %d, want %d", len(v1), EmbedDim)
	}
	if got := l2Norm(v1); math.Abs(got-1) > 1e-4 {
		t.Fatalf("норма вектора = %v, want 1", got)
	}

	v2, _ := e.Embed(context.Background(), "синопсис мира")
	if !reflect.DeepEqual(v1, v2) {
		t.Fatalf("тот же текст дал другой вектор")
	}

	v3, _ := e.Embed(context.Background(), "другой текст")
	if reflect.DeepEqual(v1, v3) {
		t.Fatalf("разные тексты дали одинаковый вектор")
	}

	if e.Calls() != 3 {
		t.Fatalf("Calls = %d, want 3", e.Calls())
	}
}

func TestSlotSpy(t *testing.T) {
	inner := NewFakeSlot()
	inner.SetResponse("weaver", "текст")
	spy := NewSlotSpy(inner)

	if _, err := spy.Generate(context.Background(), "weaver", "промпт"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if spy.Calls() != 1 || spy.Timeouts() != 0 {
		t.Fatalf("Calls=%d Timeouts=%d, want 1/0", spy.Calls(), spy.Timeouts())
	}

	inner.FailWith(ErrTimeout)
	if _, err := spy.Generate(context.Background(), "weaver", "промпт"); !errors.Is(err, ErrTimeout) {
		t.Fatalf("ожидался ErrTimeout, got %v", err)
	}
	if spy.Calls() != 2 || spy.Timeouts() != 1 {
		t.Fatalf("Calls=%d Timeouts=%d, want 2/1", spy.Calls(), spy.Timeouts())
	}
}

func l2Norm(v []float32) float64 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}

// waitPendingTimer ждёт, пока часы зарегистрируют не менее n таймеров: после
// этого Advance гарантированно сработает по подписке, без гонки с горутиной.
func waitPendingTimer(t *testing.T, c *Clock, n int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.Lock()
		got := len(c.timers)
		c.mu.Unlock()
		if got >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("часы не зарегистрировали таймер")
		}
		time.Sleep(time.Millisecond)
	}
}
