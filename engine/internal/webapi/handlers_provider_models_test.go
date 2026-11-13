package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/models"
)

// seedProviderModels подменяет settings.toml, config.yaml и чтение списка
// моделей временными значениями.
func seedProviderModels(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	oldTOML := settingsTOMLPath
	oldConfig := settingsConfigPath
	oldProviders := providersTOMLPath
	oldList := listGeminiModels
	settingsTOMLPath = filepath.Join(dir, "settings.toml")
	settingsConfigPath = filepath.Join(dir, "config.yaml")
	providersTOMLPath = filepath.Join(dir, "providers.toml")
	t.Cleanup(func() {
		settingsTOMLPath = oldTOML
		settingsConfigPath = oldConfig
		providersTOMLPath = oldProviders
		listGeminiModels = oldList
	})
	if err := os.WriteFile(providersTOMLPath, []byte(`
[defaults]
"gemini-flash" = "gemini-3.8-flash"
"gemini-pro" = "gemini-3.1-pro-preview"

[[models]]
id = "gemini-3.8-flash"
input_usd_per_m = 0.75
output_usd_per_m = 3.75

[[models]]
id = "gemini-4-hydrogen"
input_usd_per_m = 1.50
output_usd_per_m = 7.50
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsTOMLPath, []byte("gemini-flash = \"sk-test\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsConfigPath,
		[]byte("slots:\n  default: local\n  gemini_host: generativelanguage.googleapis.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestProviderModelsList — GET /api/providers/models: список читается ключом
// провайдера, остаются текстовые модели, цены приходят из справочника.
func TestProviderModelsList(t *testing.T) {
	seedProviderModels(t)
	listGeminiModels = func(_ context.Context, host, key string) ([]geminiModelInfo, error) {
		if key != "sk-test" {
			t.Errorf("ключ провайдера в запросе = %q, want sk-test", key)
		}
		if host != "generativelanguage.googleapis.com" {
			t.Errorf("хост = %q", host)
		}
		return []geminiModelInfo{
			{
				Name:                       "models/gemini-3.8-flash",
				DisplayName:                "Gemini 3.8 Flash",
				InputTokenLimit:            1048576,
				OutputTokenLimit:           65536,
				SupportedGenerationMethods: []string{"generateContent", "countTokens"},
			},
			{
				Name:                       "models/gemini-3.8-flash-tts",
				DisplayName:                "Gemini 3.8 Flash TTS",
				SupportedGenerationMethods: []string{"generateContent"},
			},
			{
				Name:                       "models/gemini-embedding-2",
				DisplayName:                "Gemini Embedding 2",
				SupportedGenerationMethods: []string{"embedContent"},
			},
		}, nil
	}

	rr := httptest.NewRecorder()
	handleProviderModels(rr, httptest.NewRequest(http.MethodGet, "/api/providers/models?provider=gemini-flash", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var resp providerModelsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if resp.Provider != "gemini-flash" || resp.Model != "gemini-3.8-flash" {
		t.Fatalf("provider/model = %q/%q", resp.Provider, resp.Model)
	}
	if len(resp.Models) != 1 {
		t.Fatalf("моделей = %d, want 1 (tts и эмбеддинг отсеяны): %+v", len(resp.Models), resp.Models)
	}
	model := resp.Models[0]
	if model.ID != "gemini-3.8-flash" || model.InputTokens != 1048576 {
		t.Fatalf("карточка модели = %+v", model)
	}
	if model.Price == nil || model.Price.InputUSDPerM != 0.75 || model.Price.OutputUSDPerM != 3.75 {
		t.Fatalf("цена = %+v, want 0.75/3.75", model.Price)
	}

	// Без ключа список не читается: конфликт, а не пустой ответ.
	if err := os.WriteFile(settingsTOMLPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	handleProviderModels(rr, httptest.NewRequest(http.MethodGet, "/api/providers/models?provider=gemini-flash", nil))
	if rr.Code != http.StatusConflict {
		t.Fatalf("без ключа status = %d, want 409", rr.Code)
	}

	// local и неизвестный провайдер — ошибка запроса.
	rr = httptest.NewRecorder()
	handleProviderModels(rr, httptest.NewRequest(http.MethodGet, "/api/providers/models?provider=local", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("local status = %d, want 400", rr.Code)
	}

	// Ключ есть, но провайдер его отклонил — отдельный конфликт.
	if err := os.WriteFile(settingsTOMLPath, []byte("gemini-flash = \"bad\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	listGeminiModels = func(context.Context, string, string) ([]geminiModelInfo, error) {
		return nil, errProviderKeyRejected
	}
	rr = httptest.NewRecorder()
	handleProviderModels(rr, httptest.NewRequest(http.MethodGet, "/api/providers/models?provider=gemini-flash", nil))
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "provider key rejected") {
		t.Fatalf("отклонённый ключ: status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

// TestProviderModelSave — PUT /api/providers/model: выбранная модель пишется в
// settings.toml рядом с ключом и требует перезапуска ядра.
func TestProviderModelSave(t *testing.T) {
	seedProviderModels(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/providers/model",
		strings.NewReader(`{"provider":"gemini-flash","model":"gemini-4-hydrogen"}`))
	handleProviderModelSave(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Status          string              `json:"status"`
		Model           string              `json:"model"`
		RestartRequired bool                `json:"restart_required"`
		Price           *providerModelPrice `json:"price"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if resp.Status != "ok" || resp.Model != "gemini-4-hydrogen" || !resp.RestartRequired {
		t.Fatalf("ответ = %+v", resp)
	}
	if resp.Price == nil || resp.Price.InputUSDPerM != 1.50 {
		t.Fatalf("цена = %+v, want 1.50", resp.Price)
	}

	data, err := os.ReadFile(settingsTOMLPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `gemini-flash_model = "gemini-4-hydrogen"`) {
		t.Fatalf("settings.toml без выбранной модели:\n%s", data)
	}
	if !strings.Contains(string(data), `gemini-flash = "sk-test"`) {
		t.Fatalf("ключ провайдера потерян:\n%s", data)
	}

	// Недопустимое имя модели и чужой провайдер — ошибка запроса.
	for _, body := range []string{
		`{"provider":"gemini-flash","model":"Bad Model!"}`,
		`{"provider":"local","model":"gemini-3.8-flash"}`,
	} {
		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPut, "/api/providers/model", strings.NewReader(body))
		handleProviderModelSave(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400", body, rr.Code)
		}
	}
}

// TestProviderModelsOpenAI — список OpenAI-совместимого провайдера читается
// /models, служебные модели отсеиваются, цена — из providers.toml.
func TestProviderModelsOpenAI(t *testing.T) {
	seedProviderModels(t)
	if err := os.WriteFile(providersTOMLPath, []byte(`
[[providers]]
name = "deepseek"
protocol = "openai"
base_url = "https://api.deepseek.com/v1"
default_model = "deepseek-chat"

[[models]]
id = "deepseek-chat"
input_usd_per_m = 0.28
output_usd_per_m = 0.42
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsTOMLPath, []byte("deepseek = \"sk-test\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldList := listOpenAIModels
	listOpenAIModels = func(_ context.Context, baseURL, key string) ([]string, error) {
		if baseURL != "https://api.deepseek.com/v1" || key != "sk-test" {
			t.Errorf("base/key = %q/%q", baseURL, key)
		}
		return []string{"deepseek-chat", "whisper-1", "deepseek-embedding"}, nil
	}
	t.Cleanup(func() { listOpenAIModels = oldList })

	rr := httptest.NewRecorder()
	handleProviderModels(rr, httptest.NewRequest(http.MethodGet, "/api/providers/models?provider=deepseek", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp providerModelsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Model != "deepseek-chat" {
		t.Fatalf("model = %q", resp.Model)
	}
	if len(resp.Models) != 1 || resp.Models[0].ID != "deepseek-chat" {
		t.Fatalf("модели = %+v (служебные должны отсеяться)", resp.Models)
	}
	if resp.Models[0].Price == nil || resp.Models[0].Price.InputUSDPerM != 0.28 {
		t.Fatalf("цена = %+v", resp.Models[0].Price)
	}

	listOpenAIModels = func(context.Context, string, string) ([]string, error) {
		return nil, models.ErrProviderKeyRejected
	}
	rr = httptest.NewRecorder()
	handleProviderModels(rr, httptest.NewRequest(http.MethodGet, "/api/providers/models?provider=deepseek", nil))
	if rr.Code != http.StatusConflict {
		t.Fatalf("отклонённый ключ: status = %d, want 409", rr.Code)
	}
}

// TestProviderModelsCuratedList — курируемый список providers.toml (models = […])
// отдаётся без ключа и без сети.
func TestProviderModelsCuratedList(t *testing.T) {
	seedProviderModels(t)
	if err := os.WriteFile(providersTOMLPath, []byte(`
[[providers]]
name = "cloudflare"
protocol = "openai"
base_url = "https://api.cloudflare.com/client/v4/accounts/acc/ai/v1"
default_model = "@cf/meta/llama-3.3-70b-instruct-fp8-fast"
models = ["@cf/meta/llama-3.3-70b-instruct-fp8-fast", "@cf/qwen/qwen2.5-coder-32b-instruct"]

[[models]]
id = "@cf/meta/llama-3.3-70b-instruct-fp8-fast"
input_usd_per_m = 0.29
output_usd_per_m = 2.25

[[models]]
id = "@cf/qwen/qwen2.5-coder-32b-instruct"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsTOMLPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	handleProviderModels(rr, httptest.NewRequest(http.MethodGet, "/api/providers/models?provider=cloudflare", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp providerModelsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Models) != 2 || resp.Models[0].ID != "@cf/meta/llama-3.3-70b-instruct-fp8-fast" {
		t.Fatalf("модели = %+v", resp.Models)
	}
	if resp.Models[0].Price == nil || resp.Models[0].Price.OutputUSDPerM != 2.25 {
		t.Fatalf("цена = %+v", resp.Models[0].Price)
	}
}

// TestProviderModelIDSlashes — имена с vendor/model, @cf/… и : принимаются.
func TestProviderModelIDSlashes(t *testing.T) {
	for _, id := range []string{
		"deepseek-chat",
		"anthropic/claude-3.5-sonnet",
		"@cf/meta/llama-3.3-70b-instruct-fp8-fast",
		"accounts/fireworks/models/llama-v3p3-70b-instruct",
		"meta-llama/Llama-3.3-70B-Instruct",
		"gemini-3.8-flash",
		"model:v1",
	} {
		if !validProviderModelID(id) {
			t.Fatalf("имя %q отвергнуто", id)
		}
	}
	for _, id := range []string{"", "/leading", "with space", "знак", "bad!"} {
		if validProviderModelID(id) {
			t.Fatalf("имя %q принято", id)
		}
	}
}
