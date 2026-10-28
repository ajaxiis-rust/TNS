package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/models"
)

// seedSettingsWorld готовит мир alpha (через seedMcpWorld) и подменяет пути
// config.yaml и settings.toml на временные файлы.
func seedSettingsWorld(t *testing.T) {
	t.Helper()
	seedMcpWorld(t)

	oldCfg := settingsConfigPath
	oldTOML := settingsTOMLPath
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	toml := filepath.Join(dir, "settings.toml")
	cfgBody := "web:\n  host: 127.0.0.1\n  port: 8720\n" +
		"paganel:\n  enabled: false\n  scout_enabled: false\n  key_ref: paganel\n" +
		"llm:\n  gen_addr: \":5001\"\n  gen_model: \"\"\n  gen_timeout: 30s\n" +
		"  gen_max_tokens: 0\n  gen_temperature: 0.0\n  gen_retries: 0\n  gen_concurrency: 1\n" +
		"  embed_addr: \":5002\"\n  embed_model: \"\"\n  embed_timeout: 30s\n" +
		"  local: true\n  sampling: greedy\n"
	if err := os.WriteFile(cfg, []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}
	settingsConfigPath = cfg
	settingsTOMLPath = toml
	t.Cleanup(func() {
		settingsConfigPath = oldCfg
		settingsTOMLPath = oldTOML
	})
}

// TestSettingsSections — «Проверка» T-08-09: экран Настроек содержит все восемь
// разделов.
func TestSettingsSections(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, settingsScreenPath, nil)
	handleSettingsScreen(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", settingsScreenPath, rr.Code)
	}
	body := rr.Body.String()
	for _, section := range settingsSections {
		if !strings.Contains(body, section) {
			t.Fatalf("settings screen missing section %q", section)
		}
	}
}

// TestSlotBindings — «Проверка» T-08-09: привязки слотов читаются и пишутся в
// model_bindings хэндлом core.
func TestSlotBindings(t *testing.T) {
	seedSettingsWorld(t)

	body := []byte(`{"world":"alpha","bindings":[
		{"role":"analysis","model_ref":":5001","provider":"local"},
		{"role":"pythia","model_ref":"","provider":"gemini-flash"}
	]}`)
	rr := mcpRequest(t, handleSettingsBindings, http.MethodPut, settingsBindingsPath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT %s: status = %d, want 200; body=%s", settingsBindingsPath, rr.Code, rr.Body.String())
	}

	rr = mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", settingsPath, rr.Code, rr.Body.String())
	}
	var out settingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	got := map[string]slotBinding{}
	for _, b := range out.Bindings {
		got[b.Role] = b
	}
	if len(got) != 2 {
		t.Fatalf("bindings = %d, want 2", len(got))
	}
	if got["analysis"].ModelRef != ":5001" || got["analysis"].Provider != "local" {
		t.Fatalf("analysis binding = %+v, want :5001/local", got["analysis"])
	}
	if got["pythia"].ModelRef != "" || got["pythia"].Provider != "gemini-flash" {
		t.Fatalf("pythia binding = %+v, want /gemini-flash", got["pythia"])
	}

	// Повторная запись не дублирует строку, а обновляет привязку.
	body = []byte(`{"world":"alpha","bindings":[{"role":"analysis","model_ref":":5002","provider":"local"}]}`)
	rr = mcpRequest(t, handleSettingsBindings, http.MethodPut, settingsBindingsPath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT повторной привязки: status = %d, want 200", rr.Code)
	}
	world, err := db.OpenReaderAs(worldDBPath("alpha"), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer world.Close()
	var n int
	if err := world.QueryRow(`SELECT COUNT(*) FROM model_bindings WHERE role = 'analysis'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("строк model_bindings role=analysis = %d, want 1", n)
	}
}

// TestEmbedDim1024 — «Проверка» T-08-09: embeddingDimension из ответа равен
// 1024 и эндпоинта записи размерности нет.
func TestEmbedDim1024(t *testing.T) {
	seedSettingsWorld(t)

	rr := mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", settingsPath, rr.Code)
	}
	var out settingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.EmbeddingDimension != models.EmbedDim {
		t.Fatalf("embedding_dimension = %d, want %d", out.EmbeddingDimension, models.EmbedDim)
	}
	if out.Advanced.EmbeddingDimension != 1024 {
		t.Fatalf("advanced.embedding_dimension = %d, want 1024", out.Advanced.EmbeddingDimension)
	}

	// Маршрутов записи размерности нет.
	for _, s := range routeSpecs(NewHub()) {
		if strings.Contains(s.path, "embedding") || strings.Contains(s.path, "dimension") {
			t.Fatalf("unexpected dimension write route %s %s", s.method, s.path)
		}
	}

	// Писатель config.yaml отвергает embeddingDimension как неизвестный ключ.
	rr = mcpRequest(t, handleSettingsConfig, http.MethodPut, settingsConfigRoutePath,
		[]byte(`{"embedding_dimension":2048}`), "application/json")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("PUT config embedding_dimension: status = %d, want 400", rr.Code)
	}
}

// TestTunablesWrite — «Проверка» T-08-09: globalLuck и параметры памяти NPC
// меняются через /api/world/tunables и пишутся хэндлом forge.
func TestTunablesWrite(t *testing.T) {
	seedSettingsWorld(t)

	body := []byte(`{"world":"alpha","tunables":{
		"globalLuck":0.9,
		"maxMemoryEntries":120,
		"similarityThreshold":0.7,
		"halfLifeDays":14
	}}`)
	rr := mcpRequest(t, handleWorldTunables, http.MethodPut, worldTunablesPath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT %s: status = %d, want 200; body=%s", worldTunablesPath, rr.Code, rr.Body.String())
	}

	got := readWorldMetaValues(t, worldDBPath("alpha"))
	for key, want := range map[string]string{
		"globalLuck":          "0.9",
		"maxMemoryEntries":    "120",
		"similarityThreshold": "0.7",
		"halfLifeDays":        "14",
	} {
		if got[key] != want {
			t.Fatalf("world_meta %s = %q, want %q", key, got[key], want)
		}
	}

	// world_meta пишет только forge: хэндл core отклоняется картой писателей.
	core, err := db.OpenWriterAs(worldDBPath("alpha"), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	err = writeWorldTunables(core, map[string]json.RawMessage{"globalLuck": json.RawMessage("0.6")})
	if !errors.Is(err, db.ErrForeignWriter) {
		t.Fatalf("writeWorldTunables(core): err = %v, want ErrForeignWriter", err)
	}
}

// TestLLMSettingsConfig — станочные LLM-настройки пишутся в config.yaml и
// читаются из GET /api/settings; ключи слотов пишутся в settings.toml.
func TestLLMSettingsConfig(t *testing.T) {
	seedSettingsWorld(t)

	body := []byte(`{
		"gen_addr":":5003","gen_model":"weaver-q4","gen_timeout":"45s",
		"gen_max_tokens":128,"gen_temperature":0.7,"gen_retries":2,"gen_concurrency":3,
		"embed_addr":":5004","embed_model":"bge-m3","embed_timeout":"20s",
		"local":false,"sampling":"top_p"
	}`)
	rr := mcpRequest(t, handleSettingsConfig, http.MethodPut, settingsConfigRoutePath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT %s: status = %d, want 200; body=%s", settingsConfigRoutePath, rr.Code, rr.Body.String())
	}

	rr = mcpRequest(t, handleSettingsKeys, http.MethodPut, settingsKeysPath,
		[]byte(`{"keys":{"llm_gen":"slot-key-1"}}`), "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT keys: status = %d, want 200", rr.Code)
	}

	rr = mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings: status = %d", rr.Code)
	}
	var out settingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.LLM.Gen.Addr != ":5003" || out.LLM.Gen.Model != "weaver-q4" || out.LLM.Gen.Timeout != "45s" {
		t.Fatalf("llm.gen = %+v", out.LLM.Gen)
	}
	if out.LLM.Gen.MaxTokens != 128 || out.LLM.Gen.Temperature != 0.7 || out.LLM.Gen.Retries != 2 || out.LLM.Gen.Concurrency != 3 {
		t.Fatalf("llm.gen числа = %+v", out.LLM.Gen)
	}
	if !out.LLM.Gen.KeySet {
		t.Fatal("llm.gen.key_set = false, want true после записи ключа")
	}
	if out.LLM.Embed.Addr != ":5004" || out.LLM.Embed.Model != "bge-m3" || out.LLM.Embed.Timeout != "20s" {
		t.Fatalf("llm.embed = %+v", out.LLM.Embed)
	}
	if out.LLM.Local {
		t.Fatal("llm.local = true, want false")
	}
	if out.LLM.Sampling != "top_p" {
		t.Fatalf("llm.sampling = %q, want top_p", out.LLM.Sampling)
	}

	// Семплирование вне фиксированного набора отвергается.
	rr = mcpRequest(t, handleSettingsConfig, http.MethodPut, settingsConfigRoutePath,
		[]byte(`{"sampling":"temperature"}`), "application/json")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("PUT config sampling=temperature: status = %d, want 400", rr.Code)
	}
}

// TestRolePromptsPersisted — состав роли (провайдер, модель, температура,
// промпты, приоритет) читается и пишется (T-08-09, шаг 5).
func TestRolePromptsPersisted(t *testing.T) {
	seedSettingsWorld(t)

	body := []byte(`{"world":"alpha","bindings":[{
		"role":"editor","model_ref":"gemini-flash","provider":"gemini-flash",
		"temperature":0.8,"prompts":"пиши коротко","priority":3
	}]}`)
	rr := mcpRequest(t, handleSettingsBindings, http.MethodPut, settingsBindingsPath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT bindings: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	rr = mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings: status = %d", rr.Code)
	}
	var out settingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	var got *slotBinding
	for i := range out.Bindings {
		if out.Bindings[i].Role == "editor" {
			got = &out.Bindings[i]
		}
	}
	if got == nil {
		t.Fatal("роль editor не найдена в bindings")
	}
	if got.Temperature != 0.8 || got.Prompts != "пиши коротко" || got.Priority != 3 {
		t.Fatalf("editor = %+v, want temperature=0.8 prompts=«пиши коротко» priority=3", got)
	}

	meta := readWorldMetaValues(t, worldDBPath("alpha"))
	if !strings.Contains(meta["role.editor"], "пиши коротко") {
		t.Fatalf("world_meta role.editor = %q, want промпт", meta["role.editor"])
	}
}

// TestModelInstallRollback — менеджер моделей (MD-08/MD-09): установка, замена,
// прогресс, целостность, откат и загрузка только из HF в world_db/models/.
func TestModelInstallRollback(t *testing.T) {
	seedSettingsWorld(t)
	oldRoot := modelsRoot
	modelsRoot = filepath.Join(t.TempDir(), "models")
	oldFetch := fetchModelFile
	content := "model-v1"
	fetchModelFile = func(_ context.Context, _ string, progress func(float64)) ([]byte, error) {
		progress(0.5)
		return []byte(content), nil
	}
	t.Cleanup(func() {
		modelsRoot = oldRoot
		fetchModelFile = oldFetch
	})

	install := func(modelRef string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{
			"world":     "alpha",
			"role":      "weaver",
			"source":    "https://huggingface.co/org/model",
			"model_ref": modelRef,
		})
		return mcpRequest(t, handleSettingsModelInstall, http.MethodPost, settingsModelsInstallPath, body, "application/json")
	}

	if rr := install("org/model"); rr.Code != http.StatusOK {
		t.Fatalf("install: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	content = "model-v2"
	if rr := install("org/model-v2"); rr.Code != http.StatusOK {
		t.Fatalf("replace: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	rr := mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings: status = %d", rr.Code)
	}
	var out settingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	var model *modelStatus
	for i := range out.Models {
		if out.Models[i].Role == "weaver" {
			model = &out.Models[i]
		}
	}
	if model == nil {
		t.Fatal("модель weaver не найдена")
	}
	if model.ModelRef != "org/model-v2" || model.Status != "installed" || model.Progress != 1 {
		t.Fatalf("model = %+v, want org/model-v2 installed 1", model)
	}
	if !model.IntegrityOK || !model.HasPrevious {
		t.Fatalf("model = %+v, want целостность ок и прежнюю версию", model)
	}

	body, _ := json.Marshal(map[string]string{"world": "alpha", "role": "weaver"})
	rr = mcpRequest(t, handleSettingsModelRollback, http.MethodPost, settingsModelsRollbackPath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("rollback: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	world, err := db.OpenReaderAs(worldDBPath("alpha"), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer world.Close()
	var ref string
	if err := world.QueryRow(`SELECT model_ref FROM model_bindings WHERE role = 'weaver'`).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	if ref != "org/model" {
		t.Fatalf("binding после отката = %q, want org/model", ref)
	}

	// Источник не-HF отвергается (MD-09.1).
	bad, _ := json.Marshal(map[string]string{
		"world": "alpha", "role": "weaver",
		"source": "https://example.com/model", "model_ref": "org/model",
	})
	rr = mcpRequest(t, handleSettingsModelInstall, http.MethodPost, settingsModelsInstallPath, bad, "application/json")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("install не-HF: status = %d, want 400", rr.Code)
	}
}

// TestModelRollbackIntegrity — откат на повреждённую прежнюю версию отвергается.
func TestModelRollbackIntegrity(t *testing.T) {
	seedSettingsWorld(t)
	oldRoot := modelsRoot
	modelsRoot = filepath.Join(t.TempDir(), "models")
	oldFetch := fetchModelFile
	fetchModelFile = func(_ context.Context, _ string, progress func(float64)) ([]byte, error) {
		progress(1)
		return []byte("model-v1"), nil
	}
	t.Cleanup(func() {
		modelsRoot = oldRoot
		fetchModelFile = oldFetch
	})

	install := func(modelRef string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{
			"world": "alpha", "role": "weaver",
			"source": "https://huggingface.co/org/model", "model_ref": modelRef,
		})
		return mcpRequest(t, handleSettingsModelInstall, http.MethodPost, settingsModelsInstallPath, body, "application/json")
	}

	if rr := install("org/model"); rr.Code != http.StatusOK {
		t.Fatalf("install: status = %d", rr.Code)
	}
	if rr := install("org/model-v2"); rr.Code != http.StatusOK {
		t.Fatalf("replace: status = %d", rr.Code)
	}

	prev := filepath.Join(modelDir("weaver"), "previous.bin")
	if err := os.WriteFile(prev, []byte("повреждено"), 0o644); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{"world": "alpha", "role": "weaver"})
	rr := mcpRequest(t, handleSettingsModelRollback, http.MethodPost, settingsModelsRollbackPath, body, "application/json")
	if rr.Code != http.StatusConflict {
		t.Fatalf("rollback повреждённой версии: status = %d, want 409", rr.Code)
	}
}
