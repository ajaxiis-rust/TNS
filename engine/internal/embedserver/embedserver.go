// Package embedserver — запуск локального эмбеддера :5002 (llama-server, BGE-M3):
// флаги сервера, health-проверка и косинус-проверка пулинга (T-12-17).
package embedserver

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strconv"

	"tns65/internal/journal"
	"tns65/internal/models"
	"tns65/internal/supervisor"
)

// Параметры запуска эмбеддера по умолчанию (MD-03.2, TUN-3).
const (
	DefaultHost = "127.0.0.1"
	DefaultPort = 5002
	// DefaultContext — размер контекста llama-server (-c) для эмбеддера.
	DefaultContext = 8192
)

// Spec — параметры запуска эмбеддера llama-server.
type Spec struct {
	Bin       string // путь к llama-server (bin/llama-server)
	ModelPath string // путь к BGE-M3 GGUF
	Host      string // пусто — 127.0.0.1
	Port      int    // 0 — 5002
	Threads   int    // 0 — llama выберет сам
}

func (s Spec) host() string {
	if s.Host == "" {
		return DefaultHost
	}
	return s.Host
}

func (s Spec) port() int {
	if s.Port == 0 {
		return DefaultPort
	}
	return s.Port
}

// Args собирает флаги llama-server эмбеддера: --model, --host, --port,
// --embeddings, --pooling cls, -c и, если задан, --threads (шаг 1).
func (s Spec) Args() []string {
	args := []string{
		"--model", s.ModelPath,
		"--host", s.host(),
		"--port", strconv.Itoa(s.port()),
		"--embeddings",
		"--pooling", "cls",
		"-c", strconv.Itoa(DefaultContext),
	}
	if s.Threads > 0 {
		args = append(args, "--threads", strconv.Itoa(s.Threads))
	}
	return args
}

// Child — спецификация ребёнка надзирателя: надзиратель запускает, лечит падения
// и по сигналу гасит эмбеддер в обратном порядке.
func (s Spec) Child() supervisor.Config {
	return supervisor.Config{
		Name:       "embed",
		Cmd:        s.Bin,
		Args:       s.Args(),
		Port:       s.port(),
		HealthPath: "/health",
	}
}

// URL — адрес эмбеддера для клиента моделей.
func (s Spec) URL() string {
	return fmt.Sprintf("http://%s:%d", s.host(), s.port())
}

// Status — снимок состояния эмбеддера для UI (T-12-17).
type Status struct {
	Addr      string `json:"addr"`
	Healthy   bool   `json:"healthy"`
	PoolingOK bool   `json:"pooling_ok"`
	Error     string `json:"error,omitempty"`
}

// Check — health-проверка и косинус-проверка пулинга по baseURL (шаги 2–3).
func Check(ctx context.Context, baseURL string, logger *journal.Logger) error {
	if err := checkHealth(ctx, baseURL); err != nil {
		return err
	}
	return checkPooling(ctx, baseURL, logger)
}

// StatusOf — снимок состояния эмбеддера для UI; недоступность сервера — не
// ошибка, а признак в снимке.
func StatusOf(ctx context.Context, baseURL string, logger *journal.Logger) Status {
	st := Status{Addr: baseURL}
	if err := checkHealth(ctx, baseURL); err != nil {
		st.Error = err.Error()
		return st
	}
	st.Healthy = true
	if err := checkPooling(ctx, baseURL, logger); err != nil {
		st.Error = err.Error()
		return st
	}
	st.PoolingOK = true
	return st
}

// checkHealth — health-запрос loopback-эмбеддера через надзирателя; ctx может
// прервать висящий опрос, не дожидаясь dial-тайм-аута netgate.
func checkHealth(ctx context.Context, baseURL string) error {
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("embedserver: %w", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return fmt.Errorf("embedserver: bad base url %q", baseURL)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	spec := supervisor.Config{Port: port, HealthPath: "/health"}
	errCh := make(chan error, 1)
	go func() { errCh <- supervisor.CheckHealth(spec) }()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Фразы косинус-проверки пулинга (шаг 3): две близкие и две далёкие.
var (
	nearA = "кошка сидит на окне"
	nearB = "кот сидит на окне"
	farA  = "кошка сидит на окне"
	farB  = "грузовик едет по шоссе"
)

// checkPooling — косинус-проверка: близкие фразы должны быть ближе далёких.
// Иначе — понятная ошибка «пулинг не тот» (пулинг не CLS).
func checkPooling(ctx context.Context, baseURL string, logger *journal.Logger) error {
	c, err := models.NewClient(baseURL, logger)
	if err != nil {
		return err
	}
	a, err := c.Embed(ctx, nearA)
	if err != nil {
		return err
	}
	b, err := c.Embed(ctx, nearB)
	if err != nil {
		return err
	}
	f1, err := c.Embed(ctx, farA)
	if err != nil {
		return err
	}
	f2, err := c.Embed(ctx, farB)
	if err != nil {
		return err
	}
	cosNear := cosine(a, b)
	cosFar := cosine(f1, f2)
	if cosNear <= cosFar {
		return fmt.Errorf("embedserver: пулинг не тот: близкие фразы %.3f не ближе далёких %.3f", cosNear, cosFar)
	}
	return nil
}

// cosine — косинусная близость двух векторов одинаковой размерности; расхождение
// размерности или нулевая норма дают 0.
func cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
