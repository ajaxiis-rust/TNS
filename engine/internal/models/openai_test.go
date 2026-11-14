package models

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIStream(t *testing.T) {
	allowLoopback(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("X-RateLimit-Remaining-Requests", "42")
		fl, _ := w.(http.Flusher)
		lines := []string{
			`data: {"choices":[{"delta":{"content":"Смотрит "}}]}`,
			"",
			`data: {"choices":[{"delta":{"content":"на "}}]}`,
			`data: {"choices":[{"delta":{"content":"тебя."}}]}`,
			"data: [DONE]",
		}
		for _, line := range lines {
			_, _ = w.Write([]byte(line + "\n"))
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer srv.Close()

	c, err := NewOpenAIClient(srv.URL+"/v1/", "deepseek-chat", "sk-test")
	if err != nil {
		t.Fatalf("NewOpenAIClient: %v", err)
	}
	ch, err := c.Stream(context.Background(), "промпт")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var got strings.Builder
	for chunk := range ch {
		got.WriteString(chunk)
	}
	if got.String() != "Смотрит на тебя." {
		t.Fatalf("текст = %q", got.String())
	}
	if got := c.LimitRemaining(); got != "42" {
		t.Fatalf("остаток лимита = %q, want 42", got)
	}
	if got := c.ProviderName(); got != "" {
		t.Fatalf("провайдер без SetProvider = %q, want пусто", got)
	}
}

func TestOpenAIStreamKeyRejected(t *testing.T) {
	allowLoopback(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c, err := NewOpenAIClient(srv.URL, "m", "bad")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Stream(context.Background(), "промпт")
	if !errors.Is(err, ErrProviderKeyRejected) {
		t.Fatalf("ошибка = %v, want ErrProviderKeyRejected", err)
	}
}

func TestListOpenAIModels(t *testing.T) {
	allowLoopback(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"deepseek-chat"},{"id":""},{"id":"deepseek-reasoner"}]}`))
	}))
	defer srv.Close()

	ids, err := ListOpenAIModels(context.Background(), srv.URL+"/v1", "sk-test")
	if err != nil {
		t.Fatalf("ListOpenAIModels: %v", err)
	}
	if len(ids) != 2 || ids[0] != "deepseek-chat" || ids[1] != "deepseek-reasoner" {
		t.Fatalf("ids = %+v", ids)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer bad.Close()
	if _, err := ListOpenAIModels(context.Background(), bad.URL, "bad"); !errors.Is(err, ErrProviderKeyRejected) {
		t.Fatalf("ошибка = %v, want ErrProviderKeyRejected", err)
	}
}

func TestNewOpenAIClientRejectsBadURL(t *testing.T) {
	for _, base := range []string{"", "ftp://example.com", "example.com/v1"} {
		if _, err := NewOpenAIClient(base, "m", "k"); err == nil {
			t.Fatalf("адрес %q принят", base)
		}
	}
}

func TestOpenAIChunkText(t *testing.T) {
	cases := []struct {
		line string
		want string
		ok   bool
	}{
		{`data: {"choices":[{"delta":{"content":"a"}}]}`, "a", true},
		{`{"choices":[{"delta":{"content":"b"}}]}`, "b", true},
		{`data: {"choices":[{"message":{"content":"c"}}]}`, "c", true},
		{`data: {"choices":[{"text":"d"}]}`, "d", true},
		{`data: [DONE]`, "", false},
		{`data: {"choices":[]}`, "", false},
		{`data: {"error":{"message":"x"}}`, "", false},
		{`event: ping`, "", false},
	}
	for _, c := range cases {
		got, ok := openAIChunkText(c.line)
		if got != c.want || ok != c.ok {
			t.Fatalf("%s → %q/%v, want %q/%v", c.line, got, ok, c.want, c.ok)
		}
	}
}
