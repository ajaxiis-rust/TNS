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
	name string // имя провайдера для журнала вызовов (ProviderSlot).
}

// SetProvider задаёт имя провайдера для журнала вызовов слота (FIX-4).
func (w *Weaver) SetProvider(name string) { w.name = name }

// ProviderName — имя провайдера для журнала вызовов; пусто — не задано.
func (w *Weaver) ProviderName() string { return w.name }

// LimitRemaining — локальная запаска лимита не отдаёт.
func (w *Weaver) LimitRemaining() string { return "" }

// NewWeaver создаёт клиент слота прозы по адресу weaverURL (loopback,
// netgate.Local — все вызовы слота только через сетевой гейт).
func NewWeaver(weaverURL string) (*Weaver, error) {
	u, err := url.Parse(weaverURL)
	if err != nil {
		return nil, fmt.Errorf("models: адрес слота прозы: %w", err)
	}
	return &Weaver{http: netgate.Local(u.Host), url: strings.TrimRight(weaverURL, "/")}, nil
}

// WeaverInstruction — инструкция движка слоту прозы: мир считает код, модель
// проговаривает (T-03-08, шаг 1). Внутренние роли движка (Ткач, Челнок, Пила,
// Гефест, Паганель) в промпт не попадают: модель принимала имя роли за образ
// рассказчика и начинала прозу словами «Я — Ткач мира».
const WeaverInstruction = "Опиши сцену прозой от второго лица, строго по фактам сцены, ничего не добавляя от себя. Не представляйся и не говори о себе."

// BuildPrompt собирает промпт Ткача из фактов сцены и инструкции движка. Факты
// уже закрыты биндером — без токенов-заменителей (T-03-03, шаг 2).
func BuildPrompt(facts, instruction string) string {
	return strings.TrimSpace(instruction) + "\n\n" + strings.TrimSpace(facts)
}

// HasReplacementTokens — остался ли в тексте токен-заменитель вида [Имя].
func HasReplacementTokens(text string) bool {
	return strings.Contains(text, "[") && strings.Contains(text, "]")
}

// completionRequest — тело запроса /completion: промпт, уже без токенов, и
// параметры генерации (nil-поле не отправляется — дефолт модели).
type completionRequest struct {
	Prompt          string   `json:"prompt"`
	NPredict        *int     `json:"n_predict,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"top_p,omitempty"`
	TopK            *int     `json:"top_k,omitempty"`
	ReasoningBudget *int     `json:"reasoning_budget,omitempty"`
}

// Stream отправляет промпт слоту без параметров генерации (обратная
// совместимость): пустые params — дефолт модели.
func (w *Weaver) Stream(ctx context.Context, prompt string) (<-chan string, error) {
	return w.StreamParams(ctx, prompt, GenerateParams{})
}

// StreamParams отправляет промпт и параметры генерации слоту на /completion и
// возвращает канал чанков текста. Каждый чанк — непустая строка тела ответа
// (стриминг построчно); канал закрывается по концу тела. Отмена ctx немедленно
// гасит чтение (T-03-08, шаг 2).
func (w *Weaver) StreamParams(ctx context.Context, prompt string, params GenerateParams) (<-chan string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	body, err := json.Marshal(completionRequest{
		Prompt:          prompt,
		NPredict:        params.MaxTokens,
		Temperature:     params.Temperature,
		TopP:            params.TopP,
		TopK:            params.TopK,
		ReasoningBudget: params.Reasoning,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url+"/completion", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.http.Do(req) //nolint:bodyclose // тело закрывает горутина-читатель через defer resp.Body.Close().
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
