package models

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// delayedSlot — подставной слот: держит запрос дольше тайм-аута клиента.
func delayedSlot(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("поздний ответ\n"))
	}))
}

// TestWeaverTimeout — gen_timeout обрывает генерацию: клиент слота возвращает
// ошибку тайм-аута, а не ждёт ответа бесконечно.
func TestWeaverTimeout(t *testing.T) {
	srv := delayedSlot(t)
	defer srv.Close()

	w, err := NewWeaverTimeout(srv.URL, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("NewWeaverTimeout: %v", err)
	}
	if _, err := w.StreamParams(context.Background(), "промпт", GenerateParams{}); err == nil {
		t.Fatal("StreamParams: ошибки нет, ожидался тайм-аут")
	} else if !strings.Contains(err.Error(), "Timeout") && !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("StreamParams: err = %v, ожидался тайм-аут", err)
	}
}

// TestOpenAITimeout — тот же тайм-аут на внешнем OpenAI-маршруте.
func TestOpenAITimeout(t *testing.T) {
	allowLoopback(t)
	srv := delayedSlot(t)
	defer srv.Close()

	c, err := NewOpenAIClientTimeout(srv.URL, "test-model", "", 20*time.Millisecond)
	if err != nil {
		t.Fatalf("NewOpenAIClientTimeout: %v", err)
	}
	if _, err := c.StreamParams(context.Background(), "промпт", GenerateParams{}); err == nil {
		t.Fatal("StreamParams: ошибки нет, ожидался тайм-аут")
	} else if !strings.Contains(err.Error(), "Timeout") && !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("StreamParams: err = %v, ожидался тайм-аут", err)
	}
}
