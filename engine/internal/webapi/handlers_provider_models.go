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
	"tns65/internal/workshop"
)

// Выбор внешней модели (T-02-11, шаг 3): список моделей читается у провайдера
// по сохранённому ключу, выбранная модель хранится в settings.toml рядом с
// ключом и перекрывает заводское имя. Цены Gemini API не отдаёт — они берутся
// из справочной таблицы ниже и показываются как справка.

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

// modelPriceTable — справочник цен удалён в providers.toml: цены и заводские
// модели внешних провайдеров читаются из файла рядом с settings.toml (см.
// providers_catalog.go), поэтому правка не требует пересборки ядра.

// selectedProviderModel — модель провайдера: выбор из settings.toml, иначе
// заводская из providers.toml ([defaults]), иначе встроенная models.ModelFor.
func selectedProviderModel(keys map[string]string, cat providerCatalog, provider workshop.Provider) string {
	if model := strings.TrimSpace(keys[models.ModelSettingsKey(models.Provider(provider))]); model != "" {
		return model
	}
	return cat.defaultFor(string(provider))
}

// externalProvider приводит имя к внешнему пресету реестра; local и неизвестные
// имена отвергаются.
func externalProvider(name string) (workshop.Provider, bool) {
	trimmed := strings.TrimSpace(name)
	for _, p := range workshop.Presets() {
		if p.Provider != workshop.ProviderLocal && string(p.Provider) == trimmed {
			return p.Provider, true
		}
	}
	return "", false
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
var errProviderKeyRejected = errors.New("provider key rejected")

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
		"antigravity", "nano-banana", "omni", "decision",
	}
	for _, part := range blocked {
		if strings.Contains(id, part) {
			return false
		}
	}
	return true
}

// providerModelCards превращает ответ провайдера в карточки для UI: только
// текстовые модели, поддерживающие generateContent, с ценой из providers.toml.
func providerModelCards(raw []geminiModelInfo, cat providerCatalog) []providerModelCard {
	cards := make([]providerModelCard, 0, len(raw))
	for _, m := range raw {
		id := strings.TrimPrefix(m.Name, "models/")
		if id == "" || !supportsGenerateContent(m.SupportedGenerationMethods) || !isTextGenerationModel(id) {
			continue
		}
		displayName := m.DisplayName
		if displayName == "" {
			displayName = cat.displayName(id)
		}
		cards = append(cards, providerModelCard{
			ID:           id,
			DisplayName:  displayName,
			InputTokens:  m.InputTokenLimit,
			OutputTokens: m.OutputTokenLimit,
			Price:        cat.priceForModel(id),
		})
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].ID < cards[j].ID })
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

// handleProviderModels — GET /api/providers/models?provider=<имя>: список
// моделей внешнего провайдера по сохранённому ключу. Только чтение.
func handleProviderModels(w http.ResponseWriter, r *http.Request) {
	provider, ok := externalProvider(r.URL.Query().Get("provider"))
	if !ok {
		writeError(w, http.StatusBadRequest, codeBadRequest, "unknown external provider")
		return
	}
	keys := readSettingsKeys(settingsTOMLPath)
	cat := readProviderCatalog(providersTOMLPath)
	key := strings.TrimSpace(keys[string(provider)])
	if key == "" {
		writeError(w, http.StatusConflict, codeConflict, "provider key is not set")
		return
	}
	cfg, err := models.LoadSlotsConfig(settingsConfigPath)
	if err != nil {
		cfg = models.DefaultSlotsConfig()
	}
	ctx, cancel := context.WithTimeout(r.Context(), providerModelsTimeout)
	defer cancel()
	raw, err := listGeminiModels(ctx, cfg.GeminiHost, key)
	if err != nil {
		if errors.Is(err, errProviderKeyRejected) {
			writeError(w, http.StatusConflict, codeConflict, "provider key rejected")
			return
		}
		writeError(w, http.StatusInternalServerError, codeInternal, "provider models unavailable")
		return
	}
	writeJSON(w, http.StatusOK, providerModelsResponse{
		Provider: string(provider),
		Model:    selectedProviderModel(keys, cat, provider),
		Models:   providerModelCards(raw, cat),
	})
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
	provider, ok := externalProvider(req.Provider)
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
	keys[models.ModelSettingsKey(models.Provider(provider))] = model
	if err := writeSettingsKeys(settingsTOMLPath, keys); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "settings write failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"provider":         string(provider),
		"model":            model,
		"price":            readProviderCatalog(providersTOMLPath).priceForModel(model),
		"restart_required": true,
	})
}

// validProviderModelID — имя модели Gemini: строчная латиница, цифры, точка и
// дефис.
func validProviderModelID(model string) bool {
	if model == "" || len(model) > 120 {
		return false
	}
	for i, r := range model {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case (r == '.' || r == '-') && i > 0:
		default:
			return false
		}
	}
	return true
}
