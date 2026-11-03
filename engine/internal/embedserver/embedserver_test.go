package embedserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"tns65/internal/models"
)

// newFakeEmbedder поднимает httptest-эмбеддер: /health отвечает healthCode,
// /v1/embeddings возвращает bag-of-words вектор по тексту запроса.
func newFakeEmbedder(t *testing.T, healthCode int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(healthCode)
			return
		case "/v1/embeddings":
			var req struct {
				Input []string `json:"input"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("декодирование запроса эмбеддера: %v", err)
			}
			text := ""
			if len(req.Input) > 0 {
				text = req.Input[0]
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []any{map[string]any{"embedding": bagEmbedding(text)}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	return srv
}

// bagEmbedding — детерминированный bag-of-words вектор размерности EmbedDim:
// близкие фразы ближе далёких по косинусу.
func bagEmbedding(text string) []float64 {
	vocab := []string{"кошка", "кот", "сидит", "окне", "грузовик", "едет", "шоссе", "на", "по"}
	vec := make([]float64, models.EmbedDim)
	for _, w := range strings.Fields(text) {
		for i, v := range vocab {
			if w == v {
				vec[i]++
			}
		}
	}
	return vec
}

func TestArgs(t *testing.T) {
	s := Spec{Bin: "bin/llama-server", ModelPath: "models/bge-m3-Q8_0.gguf", Threads: 8}
	got := s.Args()
	want := []string{
		"--model", "models/bge-m3-Q8_0.gguf",
		"--host", "127.0.0.1",
		"--port", "5002",
		"--embeddings",
		"--pooling", "cls",
		"-c", "8192",
		"--threads", "8",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Args = %v, want %v", got, want)
	}
}

func TestChild(t *testing.T) {
	s := Spec{Bin: "bin/llama-server", ModelPath: "models/bge-m3-Q8_0.gguf", Port: 5010}
	c := s.Child()
	if c.Name != "embed" || c.Cmd != "bin/llama-server" {
		t.Fatalf("child name/cmd = %q/%q", c.Name, c.Cmd)
	}
	if c.Port != 5010 || c.HealthPath != "/health" {
		t.Fatalf("child port/path = %d/%q", c.Port, c.HealthPath)
	}
	if !reflect.DeepEqual(c.Args, s.Args()) {
		t.Fatalf("child args = %v, want %v", c.Args, s.Args())
	}
}

func TestCheckOK(t *testing.T) {
	srv := newFakeEmbedder(t, http.StatusOK)
	defer srv.Close()

	if err := Check(context.Background(), srv.URL, nil); err != nil {
		t.Fatalf("Check: %v", err)
	}
	st := StatusOf(context.Background(), srv.URL, nil)
	if !st.Healthy || !st.PoolingOK {
		t.Fatalf("StatusOf = %+v, want healthy+pooling", st)
	}
}

func TestCheckBadPooling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/embeddings":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []any{map[string]any{"embedding": make([]float64, models.EmbedDim)}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := Check(context.Background(), srv.URL, nil)
	if err == nil {
		t.Fatal("ожидалась ошибка пулинга, получен nil")
	}
	if !strings.Contains(err.Error(), "пулинг не тот") {
		t.Fatalf("ошибка = %q, ожидалась «пулинг не тот»", err)
	}
}

func TestCheckHealthDown(t *testing.T) {
	srv := newFakeEmbedder(t, http.StatusInternalServerError)
	defer srv.Close()

	err := Check(context.Background(), srv.URL, nil)
	if err == nil {
		t.Fatal("ожидалась ошибка health, получен nil")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("ошибка = %q, ожидалась status 500", err)
	}
	st := StatusOf(context.Background(), srv.URL, nil)
	if st.Healthy || st.PoolingOK {
		t.Fatalf("StatusOf = %+v, want not healthy", st)
	}
}
