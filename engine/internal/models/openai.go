package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"tns65/internal/netgate"
)

// ErrProviderKeyRejected — провайдер не принял ключ (401/403).
var ErrProviderKeyRejected = errors.New("provider key rejected")

// OpenAIClient — клиент OpenAI-совместимого провайдера (/chat/completions).
// Весь трафик идёт через сетевой гейт: хост обязан быть в белом списке.
type OpenAIClient struct {
	http     *http.Client
	endpoint string
	model    string
	key      string
	name     string // имя провайдера для журнала вызовов (ProviderSlot).

	mu    sync.Mutex
	limit string // остаток лимита запросов из заголовков последнего ответа.
}

// NewOpenAIClient создаёт клиент OpenAI-совместимого провайдера. baseURL —
// базовый адрес без /chat/completions (например https://api.deepseek.com/v1);
// model — имя модели; apiKey — ключ из settings.toml (Authorization: Bearer).
func NewOpenAIClient(baseURL, model, apiKey string) (*OpenAIClient, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("models: адрес провайдера %q", baseURL)
	}
	return &OpenAIClient{
		http:     netgate.Client(u.Host),
		endpoint: base,
		model:    strings.TrimSpace(model),
		key:      apiKey,
	}, nil
}

// SetProvider задаёт имя провайдера для журнала вызовов слота (FIX-4).
func (c *OpenAIClient) SetProvider(name string) { c.name = name }

// ProviderName — имя провайдера для журнала вызовов; пусто — не задано.
func (c *OpenAIClient) ProviderName() string { return c.name }

// LimitRemaining — остаток лимита запросов из заголовков ответа провайдера;
// пусто, если провайдер лимита не отдал.
func (c *OpenAIClient) LimitRemaining() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.limit
}

func (c *OpenAIClient) setLimit(v string) {
	c.mu.Lock()
	c.limit = v
	c.mu.Unlock()
}

// openAIChatRequest — тело запроса /chat/completions со стримингом.
type openAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIStreamChunk — один объект SSE-потока; поддерживаются delta.content
// (стандарт), message.content и text.
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Text string `json:"text"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Stream отправляет промпт и возвращает канал чанков. Ошибки разделены:
// ErrProviderKeyRejected (401/403), ErrProviderRefused (прочие не-2xx),
// ErrNetwork (сеть), netgate.ErrDeniedHost (хост вне списка).
func (c *OpenAIClient) Stream(ctx context.Context, prompt string) (<-chan string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	body, err := json.Marshal(openAIChatRequest{
		Model:    c.model,
		Messages: []openAIMessage{{Role: "user", Content: prompt}},
		Stream:   true,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.http.Do(req) //nolint:bodyclose // тело закрывает горутина-читатель через defer resp.Body.Close().
	if err != nil {
		return nil, classifyDoErr(err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("%w: status %d", ErrProviderKeyRejected, resp.StatusCode)
		}
		return nil, fmt.Errorf("%w: status %d", ErrProviderRefused, resp.StatusCode)
	}
	c.setLimit(limitRemaining(resp.Header))
	ch := make(chan string)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
				continue
			}
			if line == "data: [DONE]" || line == "[DONE]" {
				return
			}
			text, ok := openAIChunkText(line)
			if !ok || text == "" {
				continue
			}
			select {
			case ch <- text:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// openAIChunkText извлекает текст из одной строки SSE; строка без префикса
// data: тоже принимается (терпимо к подставному провайдеру в тестах).
func openAIChunkText(line string) (string, bool) {
	line = strings.TrimSpace(line)
	line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if line == "" || line == "[DONE]" {
		return "", false
	}
	var chunk openAIStreamChunk
	if err := json.Unmarshal([]byte(line), &chunk); err != nil {
		return "", false
	}
	if chunk.Error != nil {
		return "", false
	}
	if len(chunk.Choices) == 0 {
		return "", false
	}
	choice := chunk.Choices[0]
	switch {
	case choice.Delta.Content != "":
		return choice.Delta.Content, true
	case choice.Message.Content != "":
		return choice.Message.Content, true
	default:
		return choice.Text, choice.Text != ""
	}
}

// openAIModelsResponse — ответ GET /models.
type openAIModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// ListOpenAIModels читает список моделей OpenAI-совместимого провайдера
// (GET <base>/models, Authorization: Bearer). Пустые имена отбрасываются.
func ListOpenAIModels(ctx context.Context, baseURL, key string) ([]string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("models: адрес провайдера %q", baseURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := netgate.Client(u.Host)
	resp, err := client.Do(req)
	if err != nil {
		return nil, classifyDoErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("%w: status %d", ErrProviderKeyRejected, resp.StatusCode)
		}
		return nil, fmt.Errorf("%w: list models: status %d", ErrProviderRefused, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNetwork, err)
	}
	var page openAIModelsResponse
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("models: список моделей: %w", err)
	}
	out := make([]string, 0, len(page.Data))
	for _, m := range page.Data {
		if id := strings.TrimSpace(m.ID); id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}
