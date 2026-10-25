package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// chunkServer — loopback-слот прозы для теста: пишет чанки построчно с флашем,
// запоминая принятый промпт в канал (без гонки с читателем).
func chunkServer(t *testing.T, chunks []string, hang bool) (*httptest.Server, <-chan string) {
	t.Helper()
	prompts := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/completion" {
			http.NotFound(w, r)
			return
		}
		var req completionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("декодирование промпта: %v", err)
		}
		prompts <- req.Prompt
		fl, _ := w.(http.Flusher)
		for _, chunk := range chunks {
			_, _ = w.Write([]byte(chunk + "\n"))
			if fl != nil {
				fl.Flush()
			}
		}
		if hang {
			<-r.Context().Done()
		}
	}))
	return srv, prompts
}

func TestWeaverPath(t *testing.T) {
	srv, prompts := chunkServer(t, []string{"Смотрит ", "на ", "тебя."}, false)
	defer srv.Close()

	w, err := NewWeaver(srv.URL)
	if err != nil {
		t.Fatalf("NewWeaver: %v", err)
	}

	facts := "Иван смотрит на тебя у городских ворот."
	prompt := BuildPrompt(facts, WeaverInstruction)

	// Промпт собирается из фактов и инструкций движка без токенов-заменителей.
	if HasReplacementTokens(prompt) {
		t.Fatalf("промпт содержит токен-заменитель: %q", prompt)
	}
	if !strings.Contains(prompt, facts) || !strings.Contains(prompt, WeaverInstruction) {
		t.Fatalf("промпт не собрал факты и инструкцию: %q", prompt)
	}

	// Стриминг чанков построчно через loopback (netgate.Local).
	ch, err := w.Stream(context.Background(), prompt)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var got strings.Builder
	for chunk := range ch {
		got.WriteString(chunk)
	}
	if got.String() != "Смотрит на тебя." {
		t.Fatalf("чанки = %q, ожидалось %q", got.String(), "Смотрит на тебя.")
	}

	// Слоту ушёл собранный промпт, без токенов-заменителей.
	sent := <-prompts
	if sent != prompt {
		t.Fatalf("промпт в запросе = %q, ожидался %q", sent, prompt)
	}
	if HasReplacementTokens(sent) {
		t.Fatalf("промпт слота содержит токен-заменитель: %q", sent)
	}
}

func TestWeaverRejectsTokens(t *testing.T) {
	// Факты с незакрытым токеном биндера должны обнаруживаться стражем.
	if !HasReplacementTokens("[NPCname] смотрит на [PLACE].") {
		t.Fatalf("HasReplacementTokens не заметил токены-заменители")
	}
	if HasReplacementTokens("Иван смотрит на город.") {
		t.Fatalf("HasReplacementTokens ошибочно заметил токены в чистом тексте")
	}
}

func TestWeaverCancellation(t *testing.T) {
	srv, _ := chunkServer(t, []string{"первый"}, true)
	defer srv.Close()

	w, err := NewWeaver(srv.URL)
	if err != nil {
		t.Fatalf("NewWeaver: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := w.Stream(ctx, BuildPrompt("факт", WeaverInstruction))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got := <-ch; got != "первый" {
		t.Fatalf("первый чанк = %q, ожидался %q", got, "первый")
	}

	// Отмена гасит генерацию немедленно: чтение чанков завершается.
	cancel()
	for range ch {
	}
}
