package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"tns65/internal/journal"
	"tns65/internal/netgate"
)

// Provider — имя провайдера генерации из заводского реестра (T-02-11, шаг 3).
type Provider string

const (
	ProviderLocal       Provider = "local"
	ProviderGeminiFlash Provider = "gemini-flash"
	ProviderGeminiPro   Provider = "gemini-pro"
)

// DefaultProvider — пресет по умолчанию: локальная запаска local.
const DefaultProvider = ProviderLocal

// PathFallback — путь, которым помечается ход при отказе провайдера (шаг 4).
const PathFallback = "fallback"

// ErrProviderRefused — отказ провайдера (не сеть): статус не 2xx.
var ErrProviderRefused = errors.New("models: отказ провайдера")

// ErrNetwork — отказ сети: соединение не установлено.
var ErrNetwork = errors.New("models: отказ сети")

// ProviderPresets — заводской реестр пресетов (шаг 3): local, gemini-flash,
// gemini-pro. Сброс всегда возвращает local (ResetProvider).
func ProviderPresets() []Provider {
	return []Provider{ProviderLocal, ProviderGeminiFlash, ProviderGeminiPro}
}

// ResetProvider — сброс пресета: возвращает локальный пресет (шаг 3).
func ResetProvider() Provider { return ProviderLocal }

// ProviderByName приводит имя к пресету реестра; неизвестное имя — local.
func ProviderByName(name string) Provider {
	switch Provider(name) {
	case ProviderLocal, ProviderGeminiFlash, ProviderGeminiPro:
		return Provider(name)
	default:
		return ProviderLocal
	}
}

// DefaultGeminiModel — имя модели Gemini по умолчанию (B-4: flash по умолчанию).
const DefaultGeminiModel = "gemini-2.0-flash"

// ModelFor возвращает имя модели Gemini для пресета (B-4: flash по умолчанию,
// pro опцией).
func ModelFor(p Provider) string {
	if p == ProviderGeminiPro {
		return "gemini-2.5-pro"
	}
	return DefaultGeminiModel
}

// SlotsConfig — настройки слотов генерации из config.yaml (секция slots).
type SlotsConfig struct {
	Default    Provider // slots.default: выбранный пресет.
	GeminiHost string   // slots.gemini_host: внешний хост в белом списке netgate.
}

// DefaultSlotsConfig возвращает значения, совпадающие с engine/config.yaml.
func DefaultSlotsConfig() SlotsConfig {
	return SlotsConfig{Default: ProviderLocal, GeminiHost: "generativelanguage.googleapis.com"}
}

// LoadSlotsConfig читает секцию slots из файла конфигурации.
func LoadSlotsConfig(path string) (SlotsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SlotsConfig{}, err
	}
	return parseSlotsSection(string(data))
}

// parseSlotsSection разбирает топ-ключ `slots:` со скалярными полями default и
// gemini_host.
func parseSlotsSection(data string) (SlotsConfig, error) {
	cfg := DefaultSlotsConfig()

	inSlots := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inSlots = trimmed == "slots:"
			continue
		}
		if !inSlots {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		switch strings.TrimSpace(key) {
		case "default":
			cfg.Default = ProviderByName(value)
		case "gemini_host":
			cfg.GeminiHost = value
		}
	}
	if err := sc.Err(); err != nil {
		return SlotsConfig{}, err
	}
	return cfg, nil
}

// Chat — контракт провайдера генерации (шаг 1): чат со стримингом чанков и
// отменой по контексту. Реализации: ProviderClient (внешний провайдер), Weaver
// (локальная запаска :5001); тесты подменяют фейком.
type Chat interface {
	Stream(ctx context.Context, prompt string) (<-chan string, error)
}

// Usage — счётчики вызовов, токенов и денег слота (шаг 4). В базы не пишутся:
// счётчики уходят в engine/state/usage.jsonl (мониторинг и счётчики T-08-13).
type Usage struct {
	Calls  int64 `json:"calls"`
	Tokens int64 `json:"tokens"`
	Cost   int64 `json:"cost_millis"` // стоимость в миллях (долей цента).
}

// AddCall увеличивает счётчик вызовов.
func (u *Usage) AddCall(n int64) { u.Calls += n }

// AddTokens увеличивает счётчик токенов.
func (u *Usage) AddTokens(n int64) { u.Tokens += n }

// AddCost увеличивает счётчик денег (милли).
func (u *Usage) AddCost(n int64) { u.Cost += n }

// ProviderClient — клиент внешнего провайдера генерации. Весь трафик идёт только
// через сетевой гейт (netgate.Client): адрес провайдера в белом списке, иных
// адресов нет (F-05). Ключ провайдера вводится в UI (T-08-09) и хранится в
// settings.toml вне world_db/; живой контур settings.toml не читает (F-03.4) —
// ключ передаётся сюда владельцем настроек.
type ProviderClient struct {
	http  *http.Client
	url   string
	model string
	key   string
}

// NewProviderClient создаёт клиент внешнего провайдера. baseURL — https-адрес
// провайдера (host в белом списке netgate); model — имя модели Gemini; пусто —
// DefaultGeminiModel; apiKey — ключ из settings.toml.
func NewProviderClient(baseURL, model, apiKey string) (*ProviderClient, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("models: адрес провайдера: %w", err)
	}
	m := strings.TrimSpace(model)
	if m == "" {
		m = DefaultGeminiModel
	}
	return &ProviderClient{
		http:  netgate.Client(u.Host),
		url:   strings.TrimRight(baseURL, "/"),
		model: m,
		key:   apiKey,
	}, nil
}

// geminiGenerateRequest — тело запроса Gemini generateContent (стриминг).
type geminiGenerateRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// Stream отправляет промпт провайдеру и возвращает канал чанков текста. Отмена
// ctx немедленно гасит чтение. Ошибки разделены (шаг 1): ErrProviderRefused
// (статус не 2xx), ErrNetwork (сеть), netgate.ErrDeniedHost (хост вне списка).
func (c *ProviderClient) Stream(ctx context.Context, prompt string) (<-chan string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	body, err := json.Marshal(geminiGenerateRequest{
		Contents: []geminiContent{{Role: "user", Parts: []geminiPart{{Text: prompt}}}},
	})
	if err != nil {
		return nil, err
	}
	endpoint := c.url + "/v1beta/models/" + c.model + ":streamGenerateContent"
	if c.key != "" {
		endpoint += "?key=" + url.QueryEscape(c.key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, classifyDoErr(err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%w: status %d", ErrProviderRefused, resp.StatusCode)
	}
	ch := make(chan string)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			text, ok := geminiChunkText(line)
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

// classifyDoErr разделяет ошибку соединения и отказ сети (шаг 1): хост вне
// белого списка возвращается как есть (netgate.ErrDeniedHost), остальное —
// ErrNetwork.
func classifyDoErr(err error) error {
	if errors.Is(err, netgate.ErrDeniedHost) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrNetwork, err)
}

// geminiChunkText извлекает текст из одной строки потока Gemini; не-JSON строка
// передаётся как есть (терпимо к подставному провайдеру в тестах).
func geminiChunkText(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}
	if strings.HasPrefix(line, "data:") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	}
	var resp geminiGenerateResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return line, true
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", false
	}
	return resp.Candidates[0].Content.Parts[0].Text, true
}

// DefaultWeaverURL — адрес локальной запаски :5001 (MD-07): при отказе внешнего
// провайдера ответ генерируется на этом станке.
const DefaultWeaverURL = "http://127.0.0.1:5001"

// fallbackChat — фабрика локальной запаски для деградации (MD-07). В проде —
// NewWeaver(DefaultWeaverURL); тесты подменяют на подставной слот, не занимая :5001.
var fallbackChat = func() (Chat, error) {
	return NewWeaver(DefaultWeaverURL)
}

// usageRecord — одна JSON-строка журнала вызовов (шаг 4, «Писатель»): роль слота
// и его счётчики вызовов, токенов и денег.
type usageRecord struct {
	Role   string `json:"role"`
	Calls  int64  `json:"calls"`
	Tokens int64  `json:"tokens"`
	Cost   int64  `json:"cost_millis"`
}

// AppendUsage дописывает счётчики слота role одной JSON-строкой в журнал вызовов
// path (engine/state/usage.jsonl). Каталог создаётся; пустой path — запись
// пропускается.
func AppendUsage(path, role string, u Usage) error {
	if path == "" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	b, err := json.Marshal(usageRecord{Role: role, Calls: u.Calls, Tokens: u.Tokens, Cost: u.Cost})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// CompleteChat — один чат-вызов провайдера с деградацией на локальную запаску
// (шаг 4, MD-07): при отказе провайдера, отказе сети или пустом ответе ответ
// генерируется на локальной запаске :5001 (models.NewWeaver) и доставляется с
// путём PathFallback — ход не прерван. Деградация пишется событием slot.degraded;
// вызов учитывается в usage и дописывается в журнал вызовов usagePath (роль path).
// Пустые logger/usagePath — событие и запись пропускаются.
func CompleteChat(ctx context.Context, chat Chat, prompt, path string, usage *Usage, logger *journal.Logger, usagePath string) (text string, outPath string) {
	if usage != nil {
		usage.AddCall(1)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if usagePath != "" && usage != nil {
		defer func() { _ = AppendUsage(usagePath, path, *usage) }()
	}

	if chat != nil {
		text, empty, err := collectStream(ctx, chat, prompt)
		if err == nil && !empty {
			return text, path
		}
	}
	return degradeChat(ctx, prompt, path, logger, usagePath)
}

// collectStream собирает весь текст стрима слота. empty — стрим закрылся без
// чанков (пустой ответ); nil-канал тоже считается пустым ответом.
func collectStream(ctx context.Context, chat Chat, prompt string) (text string, empty bool, err error) {
	ch, err := chat.Stream(ctx, prompt)
	if err != nil {
		return "", false, err
	}
	if ch == nil {
		return "", true, nil
	}
	var b strings.Builder
	for chunk := range ch {
		b.WriteString(chunk)
	}
	if b.Len() == 0 {
		return "", true, nil
	}
	return b.String(), false, nil
}

// degradeChat — деградация на локальную запаску :5001 (MD-07): событие
// slot.degraded в журнал, затем генерация ответа запаской. Запасной вызов слота
// local тоже дописывается в журнал вызовов.
func degradeChat(ctx context.Context, prompt, role string, logger *journal.Logger, usagePath string) (string, string) {
	if logger != nil {
		_ = logger.Event("models", journal.EventSlotDegraded,
			journal.Field{Key: "role", Value: role})
	}
	fb, err := fallbackChat()
	if err != nil {
		return "", PathFallback
	}
	text, empty, err := collectStream(ctx, fb, prompt)
	if err != nil || empty {
		return "", PathFallback
	}
	if usagePath != "" {
		_ = AppendUsage(usagePath, string(ProviderLocal), Usage{Calls: 1})
	}
	return text, PathFallback
}
