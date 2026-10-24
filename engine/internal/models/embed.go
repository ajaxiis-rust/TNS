package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"

	"tns65/internal/journal"
	"tns65/internal/netgate"
)

// EmbedDim — размерность вектора смысла (MD-03.2, D-16.2).
const EmbedDim = 1024

// Embedder — клиент эмбеддингов: текст → вектор смысла.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Client — HTTP-клиент эмбеддингов :5002 (BGE-M3): POST {"input":…}, ответ
// {"embedding":[1024 чисел]}. Вектор L2-нормализуется; ошибки — embed.fail.
type Client struct {
	http   *http.Client
	url    string
	logger *journal.Logger
}

// NewClient создаёт клиент эмбеддингов по адресу embedURL (loopback, netgate.Local).
func NewClient(embedURL string, logger *journal.Logger) (*Client, error) {
	u, err := url.Parse(embedURL)
	if err != nil {
		return nil, fmt.Errorf("models: адрес эмбеддера: %w", err)
	}
	return &Client{http: netgate.Local(u.Host), url: embedURL, logger: logger}, nil
}

type embedRequest struct {
	Input string `json:"input"`
}

type embedResponse struct {
	Embedding []float64 `json:"embedding"`
}

// Embed возвращает L2-нормированный вектор смысла текста размерности EmbedDim.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	body, err := json.Marshal(embedRequest{Input: text})
	if err != nil {
		return nil, c.fail(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, c.fail(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, c.fail(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.fail(fmt.Errorf("embedder status %d", resp.StatusCode))
	}
	var out embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, c.fail(err)
	}
	if len(out.Embedding) != EmbedDim {
		return nil, c.fail(fmt.Errorf("embedder returned dim %d, want %d", len(out.Embedding), EmbedDim))
	}
	return l2Normalize(out.Embedding), nil
}

// fail пишет ошибку в журнал event=embed.fail и возвращает её.
func (c *Client) fail(err error) error {
	if c.logger != nil {
		_ = c.logger.Event("models", journal.EventEmbedFail,
			journal.Field{Key: "err", Value: err.Error()})
	}
	return err
}

// l2Normalize приводит вектор к единичной L2-норме; нулевой вектор — ось x.
func l2Normalize(v []float64) []float32 {
	out := make([]float32, len(v))
	var sumSq float64
	for _, x := range v {
		sumSq += x * x
	}
	norm := math.Sqrt(sumSq)
	if norm == 0 {
		out[0] = 1
		return out
	}
	for i, x := range v {
		out[i] = float32(x / norm)
	}
	return out
}
