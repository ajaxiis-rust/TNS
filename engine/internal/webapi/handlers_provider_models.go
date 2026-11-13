package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"tns65/internal/models"
	"tns65/internal/netgate"
	"tns65/internal/providercfg"
)

// Выбор внешней модели (T-02-11, шаг 3): реестр провайдеров и заводские модели —
// в providers.toml, выбранная модель — в settings.toml рядом с ключом. Список
// моделей читается по сохранённому ключу (Gemini ListModels или
// OpenAI-совместимый /models) либо берётся из курируемого списка файла. Цены
// API не отдаёт — они в providers.toml.

const (
	providerModelsTimeout = 12 * time.Second
	geminiModelsPageSize  = 1000
	geminiModelsMaxPages  = 3
)

// providerModelPrice — цена модели за 1M токенов, USD (вход/выход).
type providerModelPrice struct {
	InputUSDPerM  float64 `json:"input_usd_per_m"`
	OutputUSDPerM float64 `json:"output_usd_per_m"`
	Note          string  `json:"note,omitempty"`
}

// providerModelCard — модель в списке провайдера.
type providerModelCard struct {
	ID           string              `json:"id"`
	DisplayName  string              `json:"display_name"`
	InputTokens  int                 `json:"input_tokens,omitempty"`
	OutputTokens int                 `json:"output_tokens,omitempty"`
	Price        *providerModelPrice `json:"price,omitempty"`
}

// providerModelsResponse — ответ GET /api/providers/models.
type providerModelsResponse struct {
	Provider string              `json:"provider"`
	Model    string              `json:"model"`
	Models   []providerModelCard `json:"models"`
}

// externalProvider ищет внешний провайдер в реестре providers.toml; local и
// неизвестные имена отвергаются.
func externalProvider(cat providercfg.Config, name string) (providercfg.Provider, bool) {
	p, ok := cat.Provider(strings.TrimSpace(name))
	if !ok || p.Protocol == providercfg.ProtocolLocal || p.Name == "local" {
		return providercfg.Provider{}, false
	}
	return p, true
}

// geminiModelInfo — модель из ответа Gemini ListModels (только нужные поля).
type geminiModelInfo struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName"`
	InputTokenLimit            int      `json:"inputTokenLimit"`
	OutputTokenLimit           int      `json:"outputTokenLimit"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

// geminiModelsPage — одна страница ответа ListModels.
type geminiModelsPage struct {
	Models        []geminiModelInfo `json:"models"`
	NextPageToken string            `json:"nextPageToken"`
}

// listGeminiModels — подменяемая в тестах функция чтения списка моделей.
var listGeminiModels = fetchGeminiModels

// errProviderKeyRejected — провайдер не принял ключ (400/401/403).
var errProviderKeyRejected = models.ErrProviderKeyRejected

// listOpenAIModels — подменяемая в тестах функция чтения списка моделей
// OpenAI-совместимого провайдера.
var listOpenAIModels = models.ListOpenAIModels

// fetchGeminiModels читает список моделей Gemini ключом провайдера. Ключ идёт
// заголовком x-goog-api-key, чтобы не попасть в URL, журналы и тексты ошибок.
func fetchGeminiModels(ctx context.Context, host, key string) ([]geminiModelInfo, error) {
	if host == "" {
		host = models.DefaultSlotsConfig().GeminiHost
	}
	base := "https://" + host + "/v1beta/models"
	client := netgate.Client(host)
	out := make([]geminiModelInfo, 0, 64)
	pageToken := ""
	for page := 0; page < geminiModelsMaxPages; page++ {
		query := url.Values{}
		query.Set("pageSize", strconv.Itoa(geminiModelsPageSize))
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-goog-api-key", key)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		var body geminiModelsPage
		decodeErr := json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				return nil, fmt.Errorf("%w: status %d", errProviderKeyRejected, resp.StatusCode)
			}
			return nil, fmt.Errorf("gemini list models: status %d", resp.StatusCode)
		}
		if decodeErr != nil {
			return nil, decodeErr
		}
		out = append(out, body.Models...)
		pageToken = body.NextPageToken
		if pageToken == "" {
			break
		}
	}
	return out, nil
}

// isTextGenerationModel отсекает модели, которые не пишут прозу: генераторы
// картинок, речи, видео, эмбеддинги и служебные агенты.
func isTextGenerationModel(id string) bool {
	blocked := []string{
		"-tts", "-image", "-live", "-transcribe", "-audio", "imagen", "lyria",
		"veo", "embedding", "robotics", "computer-use", "deep-research",
		"antigravity", "nano-banana", "omni", "decision", "whisper", "dall-e",
		"rerank", "moderation",
	}
	for _, part := range blocked {
		if strings.Contains(id, part) {
			return false
		}
	}
	return true
}

// providerModelCards превращает ответ Gemini ListModels в карточки для UI:
// только текстовые модели, поддерживающие generateContent, с ценой и именем из
// providers.toml.
func providerModelCards(raw []geminiModelInfo, cat providercfg.Config) []providerModelCard {
	cards := make([]providerModelCard, 0, len(raw))
	for _, m := range raw {
		id := strings.TrimPrefix(m.Name, "models/")
		if id == "" || !supportsGenerateContent(m.SupportedGenerationMethods) || !isTextGenerationModel(id) {
			continue
		}
		displayName := m.DisplayName
		if displayName == "" {
			displayName = modelDisplayName(cat, id)
		}
		cards = append(cards, providerModelCard{
			ID:           id,
			DisplayName:  displayName,
			InputTokens:  m.InputTokenLimit,
			OutputTokens: m.OutputTokenLimit,
			Price:        priceForModel(cat, id),
		})
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].ID < cards[j].ID })
	return cards
}

// openAIModelCards превращает список /models OpenAI-совместимого провайдера в
// карточки: служебные модели (эмбеддинги, речь, картинки, модерация) отсеиваются,
// цена и имя — из providers.toml. Токен-лимиты OpenAI-совместимый /models не
// отдаёт.
func openAIModelCards(ids []string, cat providercfg.Config) []providerModelCard {
	cards := make([]providerModelCard, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || !isTextGenerationModel(id) {
			continue
		}
		cards = append(cards, providerModelCard{
			ID:          id,
			DisplayName: modelDisplayName(cat, id),
			Price:       priceForModel(cat, id),
		})
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].ID < cards[j].ID })
	return cards
}

// curatedModelCards собирает карточки из курируемого списка providers.toml
// (models = [...]): сеть и ключ не нужны, порядок файла сохраняется.
func curatedModelCards(ids []string, cat providercfg.Config) []providerModelCard {
	cards := make([]providerModelCard, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		cards = append(cards, providerModelCard{
			ID:          id,
			DisplayName: modelDisplayName(cat, id),
			Price:       priceForModel(cat, id),
		})
	}
	return cards
}

func supportsGenerateContent(methods []string) bool {
	for _, method := range methods {
		if method == "generateContent" {
			return true
		}
	}
	return false
}

// geminiHost достаёт хост Gemini из base_url реестра; пусто — хост из конфига.
func geminiHost(def providercfg.Provider) string {
	if u, err := url.Parse(def.BaseURL); err == nil && u.Host != "" {
		return u.Host
	}
	if cfg, err := models.LoadSlotsConfig(settingsConfigPath); err == nil {
		return cfg.GeminiHost
	}
	return models.DefaultSlotsConfig().GeminiHost
}

// handleProviderModels — GET /api/providers/models?provider=<имя>: список
// моделей внешнего провайдера. Курируемый список из providers.toml отдаётся без
// сети; иначе список читается у провайдера сохранённым ключом (Gemini ListModels
// или OpenAI-совместимый /models). Только чтение.
func handleProviderModels(w http.ResponseWriter, r *http.Request) {
	cat := readProviderCatalog(providersTOMLPath)
	def, ok := externalProvider(cat, r.URL.Query().Get("provider"))
	if !ok {
		writeError(w, http.StatusBadRequest, codeBadRequest, "unknown external provider")
		return
	}
	keys := readSettingsKeys(settingsTOMLPath)
	resp := providerModelsResponse{
		Provider: def.Name,
		Model:    selectedProviderModel(keys, cat, def.Name),
	}
	if len(def.Models) > 0 {
		resp.Models = curatedModelCards(def.Models, cat)
		writeJSON(w, http.StatusOK, resp)
		return
	}
	key := strings.TrimSpace(keys[def.Name])
	if key == "" {
		writeError(w, http.StatusConflict, codeConflict, "provider key is not set")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), providerModelsTimeout)
	defer cancel()
	switch def.Protocol {
	case providercfg.ProtocolLocal:
		writeError(w, http.StatusBadRequest, codeBadRequest, "local provider has no remote model list")
		return
	case providercfg.ProtocolGemini:
		raw, err := listGeminiModels(ctx, geminiHost(def), key)
		if err != nil {
			writeProviderModelsError(w, err)
			return
		}
		resp.Models = providerModelCards(raw, cat)
	case providercfg.ProtocolOpenAI:
		ids, err := listOpenAIModels(ctx, def.BaseURL, key)
		if err != nil {
			writeProviderModelsError(w, err)
			return
		}
		resp.Models = openAIModelCards(ids, cat)
	default:
		writeError(w, http.StatusBadRequest, codeBadRequest, "unknown provider protocol")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// writeProviderModelsError переводит ошибку чтения списка моделей в ответ:
// отклонённый ключ — 409, остальное — 500.
func writeProviderModelsError(w http.ResponseWriter, err error) {
	if errors.Is(err, models.ErrProviderKeyRejected) {
		writeError(w, http.StatusConflict, codeConflict, "provider key rejected")
		return
	}
	writeError(w, http.StatusInternalServerError, codeInternal, "provider models unavailable")
}

// handleProviderModelSave — PUT /api/providers/model: выбранная модель
// провайдера пишется в settings.toml рядом с ключом. Новый слот Ткача
// собирается при старте ядра, поэтому ответ помечает перезапуск.
func handleProviderModelSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad provider model body")
		return
	}
	cat := readProviderCatalog(providersTOMLPath)
	def, ok := externalProvider(cat, req.Provider)
	if !ok {
		writeError(w, http.StatusBadRequest, codeBadRequest, "unknown external provider")
		return
	}
	model := strings.TrimSpace(req.Model)
	if !validProviderModelID(model) {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad model id")
		return
	}
	keys := readSettingsKeys(settingsTOMLPath)
	keys[models.ModelSettingsKey(models.Provider(def.Name))] = model
	if err := writeSettingsKeys(settingsTOMLPath, keys); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "settings write failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"provider":         def.Name,
		"model":            model,
		"price":            priceForModel(cat, model),
		"restart_required": true,
	})
}

// validProviderModelID — имя модели провайдера: латиница, цифры, точка, дефис,
// подчёркивание, слэш (vendor/model), двоеточие и @ (Cloudflare). Начинается с
// буквы или цифры.
func validProviderModelID(model string) bool {
	if model == "" || len(model) > 160 {
		return false
	}
	for i, r := range model {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '@':
		case (r == '.' || r == '-' || r == '_' || r == '/' || r == ':') && i > 0:
		default:
			return false
		}
	}
	return true
}
