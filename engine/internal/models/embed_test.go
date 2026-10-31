package models

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// embedReq — захваченное тело запроса к эмбеддеру (контракт /v1/embeddings).
type embedReq struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

// startEmbedServer поднимает loopback-эмбеддер, отвечающий body по пути
// /v1/embeddings; иной путь — 404 (контракт llama-server TUN-3). Тело запроса
// декодируется и кладётся в канал (без гонки с читателем).
func startEmbedServer(t *testing.T, body string) (*httptest.Server, <-chan embedReq) {
	t.Helper()
	reqs := make(chan embedReq, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		var req embedReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("декодирование запроса эмбеддера: %v", err)
		}
		reqs <- req
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	return srv, reqs
}

func embedding(dim int) []float64 {
	v := make([]float64, dim)
	for i := range v {
		v[i] = float64(i + 1)
	}
	return v
}

func TestEmbedV1(t *testing.T) {
	srv, reqs := startEmbedServer(t, `{"data":[{"embedding":[`+strings.TrimSuffix(
		strings.Repeat("1,", EmbedDim), ",")+`]}]}`)
	defer srv.Close()

	c, err := NewClient(srv.URL, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	vec, err := c.Embed(context.Background(), "текст")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	// Запрос ушёл по пути /v1/embeddings с массивом input и именем модели
	// (по умолчанию — bge-m3 из llm.embed_model, пусто → DefaultEmbedModel).
	req := <-reqs
	if len(req.Input) != 1 || req.Input[0] != "текст" {
		t.Fatalf("input = %#v, ожидался массив из одного текста", req.Input)
	}
	if req.Model != DefaultEmbedModel {
		t.Fatalf("model = %q, want %q", req.Model, DefaultEmbedModel)
	}

	// Ответ data[0].embedding длиной 1024 L2-нормируется.
	if len(vec) != EmbedDim {
		t.Fatalf("dim = %d, want %d", len(vec), EmbedDim)
	}
	var sumSq float64
	for _, x := range vec {
		sumSq += float64(x) * float64(x)
	}
	if norm := math.Sqrt(sumSq); math.Abs(norm-1) > 1e-6 {
		t.Fatalf("L2-норма = %v, want 1", norm)
	}
}

func TestEmbedV1ModelFromConfig(t *testing.T) {
	srv, reqs := startEmbedServer(t, `{"data":[{"embedding":[`+strings.TrimSuffix(
		strings.Repeat("0,", EmbedDim), ",")+`]}]}`)
	defer srv.Close()

	c, err := NewClient(srv.URL, nil, "bge-m3")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Embed(context.Background(), "текст"); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if req := <-reqs; req.Model != "bge-m3" {
		t.Fatalf("model = %q, want bge-m3", req.Model)
	}
}

func TestEmbedV1WrongDim(t *testing.T) {
	srv, _ := startEmbedServer(t, `{"data":[{"embedding":[`+strings.TrimSuffix(
		strings.Repeat("1,", 384), ",")+`]}]}`)
	defer srv.Close()

	c, err := NewClient(srv.URL, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = c.Embed(context.Background(), "текст")
	if err == nil {
		t.Fatal("ожидалась ошибка размерности, получен nil")
	}
	if !strings.Contains(err.Error(), "embedder returned dim 384, want 1024") {
		t.Fatalf("ошибка = %q, ожидалась размерность 384/1024", err)
	}
}

// TestEmbedV1Normalized проверяет, что вектор из data[0].embedding нормируется,
// а не просто копируется (контракт L2-нормализации, шаг 3).
func TestEmbedV1Normalized(t *testing.T) {
	v := embedding(EmbedDim)
	b, err := json.Marshal(map[string]any{"data": []any{map[string]any{"embedding": v}}})
	if err != nil {
		t.Fatal(err)
	}
	srv, _ := startEmbedServer(t, string(b))
	defer srv.Close()

	c, err := NewClient(srv.URL, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	vec, err := c.Embed(context.Background(), "текст")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	var sumSq float64
	for _, x := range vec {
		sumSq += float64(x) * float64(x)
	}
	if norm := math.Sqrt(sumSq); math.Abs(norm-1) > 1e-6 {
		t.Fatalf("L2-норма = %v, want 1", norm)
	}
	// Нормированный вектор не равен сырому (все компоненты 1..1024).
	if vec[0] == float32(v[0]) && vec[EmbedDim-1] == float32(v[EmbedDim-1]) {
		t.Fatal("вектор не нормирован — совпал с сырым входом")
	}
}
