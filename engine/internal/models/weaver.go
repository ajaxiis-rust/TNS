package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"tns65/internal/netgate"
)

// Weaver — клиент слота прозы (генерации) на локальной запаске :5001 (T-03-08):
// эндпоинт /completion, стриминг чанков построчно. Весь HTTP идёт только через
// netgate.Local (loopback); интеграция внешнего провайдера Gemini — T-03-18, до
// неё внешний провайдер не подключён.
type Weaver struct {
	http *http.Client
	url  string
}

// NewWeaver создаёт клиент слота прозы по адресу weaverURL (loopback,
// netgate.Local — все вызовы слота только через сетевой гейт).
func NewWeaver(weaverURL string) (*Weaver, error) {
	u, err := url.Parse(weaverURL)
	if err != nil {
		return nil, fmt.Errorf("models: адрес слота прозы: %w", err)
	}
	return &Weaver{http: netgate.Local(u.Host), url: strings.TrimRight(weaverURL, "/")}, nil
}

// WeaverInstruction — инструкция движка Ткачу: мир считает код, модель
// проговаривает (T-03-08, шаг 1).
const WeaverInstruction = "Ты — Ткач мира. Отвечай прозой строго по фактам сцены, ничего не добавляя от себя."

// BuildPrompt собирает промпт Ткача из фактов сцены и инструкции движка. Факты
// уже закрыты биндером — без токенов-заменителей (T-03-03, шаг 2).
func BuildPrompt(facts, instruction string) string {
	return strings.TrimSpace(instruction) + "\n\n" + strings.TrimSpace(facts)
}

// HasReplacementTokens — остался ли в тексте токен-заменитель вида [Имя].
func HasReplacementTokens(text string) bool {
	return strings.Contains(text, "[") && strings.Contains(text, "]")
}

// completionRequest — тело запроса /completion: промпт, уже без токенов.
type completionRequest struct {
	Prompt string `json:"prompt"`
}

// Stream отправляет промпт слоту на /completion и возвращает канал чанков текста.
// Каждый чанк — непустая строка тела ответа (стриминг построчно); канал закрывается
// по концу тела. Отмена ctx немедленно гасит чтение (T-03-08, шаг 2).
func (w *Weaver) Stream(ctx context.Context, prompt string) (<-chan string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	body, err := json.Marshal(completionRequest{Prompt: prompt})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url+"/completion", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("models: слот прозы status %d", resp.StatusCode)
	}
	ch := make(chan string)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if line == "" {
				continue
			}
			select {
			case ch <- line:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}
