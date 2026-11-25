package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// floatPtr / intPtr — значения параметров для тестов.
func floatPtr(v float64) *float64 { return &v }
func intPtr(v int) *int           { return &v }

// requestBodies — подставной сервер: складывает тела запросов в канал (без
// гонки с читателем) и отвечает одной строкой.
func requestBodies(t *testing.T) (*httptest.Server, <-chan map[string]json.RawMessage) {
	t.Helper()
	bodies := make(chan map[string]json.RawMessage, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("декодирование тела: %v", err)
		}
		bodies <- body
		fl, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("ok\n"))
		if fl != nil {
			fl.Flush()
		}
	}))
	return srv, bodies
}

// drainCh — читает поток до конца.
func drainCh(ch <-chan string) {
	for range ch {
	}
}

// TestWeaverRequestParams — параметры генерации попадают в тело /completion;
// пустые params полей не добавляют (дефолт модели).
func TestWeaverRequestParams(t *testing.T) {
	srv, bodies := requestBodies(t)
	defer srv.Close()

	w, err := NewWeaver(srv.URL)
	if err != nil {
		t.Fatalf("NewWeaver: %v", err)
	}
	ch, err := w.StreamParams(context.Background(), "промпт", GenerateParams{
		Temperature: floatPtr(0.35),
		MaxTokens:   intPtr(512),
		TopP:        floatPtr(0.9),
		TopK:        intPtr(40),
		Reasoning:   intPtr(2048),
	})
	if err != nil {
		t.Fatalf("StreamParams: %v", err)
	}
	drainCh(ch)

	body := <-bodies
	for key, want := range map[string]float64{
		"n_predict": 512, "temperature": 0.35, "top_p": 0.9, "top_k": 40,
		"reasoning_budget": 2048,
	} {
		raw, ok := body[key]
		if !ok {
			t.Fatalf("в теле нет поля %q: %v", key, body)
		}
		var got float64
		if err := json.Unmarshal(raw, &got); err != nil || got != want {
			t.Fatalf("поле %q = %s, ожидалось %v", key, raw, want)
		}
	}

	// Пустые params — модель получает прежний запрос без полей генерации.
	ch, err = w.Stream(context.Background(), "промпт")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	drainCh(ch)
	body = <-bodies
	for _, key := range []string{"n_predict", "temperature", "top_p", "top_k", "reasoning_budget"} {
		if _, ok := body[key]; ok {
			t.Fatalf("при пустых params в теле есть поле %q: %v", key, body)
		}
	}
	if _, ok := body["prompt"]; !ok {
		t.Fatalf("промпт потерян при пустых params: %v", body)
	}
}

// TestOpenAIRequestParams — параметры генерации попадают в тело
// /chat/completions, бюджет рассуждений маппится в reasoning_effort; пустые
// params полей не добавляют.
func TestOpenAIRequestParams(t *testing.T) {
	allowLoopback(t)
	srv, bodies := requestBodies(t)
	defer srv.Close()

	c, err := NewOpenAIClient(srv.URL, "deepseek-chat", "")
	if err != nil {
		t.Fatalf("NewOpenAIClient: %v", err)
	}
	ch, err := c.StreamParams(context.Background(), "промпт", GenerateParams{
		Temperature: floatPtr(0.2),
		MaxTokens:   intPtr(800),
		TopP:        floatPtr(0.8),
		TopK:        intPtr(20),
		Reasoning:   intPtr(4096),
	})
	if err != nil {
		t.Fatalf("StreamParams: %v", err)
	}
	drainCh(ch)

	body := <-bodies
	for key, want := range map[string]float64{
		"max_tokens": 800, "temperature": 0.2, "top_p": 0.8, "top_k": 20,
	} {
		raw, ok := body[key]
		if !ok {
			t.Fatalf("в теле нет поля %q: %v", key, body)
		}
		var got float64
		if err := json.Unmarshal(raw, &got); err != nil || got != want {
			t.Fatalf("поле %q = %s, ожидалось %v", key, raw, want)
		}
	}
	var effort string
	if err := json.Unmarshal(body["reasoning_effort"], &effort); err != nil || effort != "medium" {
		t.Fatalf("reasoning_effort = %s, ожидалось medium", body["reasoning_effort"])
	}

	ch, err = c.Stream(context.Background(), "промпт")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	drainCh(ch)
	body = <-bodies
	for _, key := range []string{"max_tokens", "temperature", "top_p", "top_k", "reasoning_effort"} {
		if _, ok := body[key]; ok {
			t.Fatalf("при пустых params в теле есть поле %q: %v", key, body)
		}
	}
}

// TestGeminiRequestParams — параметры генерации попадают в generationConfig;
// пустые params не добавляют его вовсе.
func TestGeminiRequestParams(t *testing.T) {
	allowLoopback(t)
	srv, bodies := requestBodies(t)
	defer srv.Close()

	c, err := NewProviderClient(srv.URL, "", "")
	if err != nil {
		t.Fatalf("NewProviderClient: %v", err)
	}
	ch, err := c.StreamParams(context.Background(), "промпт", GenerateParams{
		Temperature: floatPtr(0.6),
		MaxTokens:   intPtr(1024),
		TopP:        floatPtr(0.95),
		TopK:        intPtr(64),
		Reasoning:   intPtr(0),
	})
	if err != nil {
		t.Fatalf("StreamParams: %v", err)
	}
	drainCh(ch)

	body := <-bodies
	var cfg struct {
		Temperature     *float64 `json:"temperature"`
		MaxOutputTokens *int     `json:"maxOutputTokens"`
		TopP            *float64 `json:"topP"`
		TopK            *int     `json:"topK"`
		ThinkingConfig  *struct {
			ThinkingBudget *int `json:"thinkingBudget"`
		} `json:"thinkingConfig"`
	}
	if err := json.Unmarshal(body["generationConfig"], &cfg); err != nil {
		t.Fatalf("generationConfig в теле = %s: %v", body["generationConfig"], err)
	}
	if cfg.Temperature == nil || *cfg.Temperature != 0.6 ||
		cfg.MaxOutputTokens == nil || *cfg.MaxOutputTokens != 1024 ||
		cfg.TopP == nil || *cfg.TopP != 0.95 ||
		cfg.TopK == nil || *cfg.TopK != 64 {
		t.Fatalf("generationConfig = %+v, ожидались все параметры", cfg)
	}
	if cfg.ThinkingConfig == nil || cfg.ThinkingConfig.ThinkingBudget == nil || *cfg.ThinkingConfig.ThinkingBudget != 0 {
		t.Fatalf("thinkingConfig = %+v, ожидался бюджет 0 (выключить рассуждения)", cfg.ThinkingConfig)
	}

	ch, err = c.Stream(context.Background(), "промпт")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	drainCh(ch)
	body = <-bodies
	if _, ok := body["generationConfig"]; ok {
		t.Fatalf("при пустых params в теле есть generationConfig: %v", body)
	}
}
