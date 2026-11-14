package webapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"tns65/internal/db"
	"tns65/internal/geo"
	"tns65/internal/models"
	"tns65/internal/supervisor"
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
		"  local: true\n  sampling: greedy\n" +
		"tunnel:\n  edge: region1.v2.argotunnel.com\n  local: http://127.0.0.1:8720\n" +
		"  telegram_token: \"\"\n  telegram_chat_id: \"\"\n"
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

// TestSettingsSupervisorStatus — «статус» T-12-19: снимок надзирателя попадает
// в ответ настроек, когда ядро передало провайдера.
func TestSettingsSupervisorStatus(t *testing.T) {
	seedSettingsWorld(t)

	old := SupervisorStatus
	SupervisorStatus = func() []supervisor.ChildStatus {
		return []supervisor.ChildStatus{{Name: "llama", Running: true, Healthy: true, Degraded: 1, Port: 5001}}
	}
	t.Cleanup(func() { SupervisorStatus = old })

	rr := mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath, nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", settingsPath, rr.Code)
	}
	var resp settingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if len(resp.Supervisor) != 1 || resp.Supervisor[0].Name != "llama" || !resp.Supervisor[0].Healthy {
		t.Fatalf("Supervisor = %+v, want llama+healthy", resp.Supervisor)
	}
}

// TestSettingsSections — «Проверка» T-08-09: экран Настроек содержит все девять
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

// TestLLMSettingsUnquotesAddr — адрес слота из config.yaml с кавычками
// разбирается без кавычек, проходит validLLMAddr и round-trip через
// PUT /api/settings/config.
func TestLLMSettingsUnquotesAddr(t *testing.T) {
	seedSettingsWorld(t)

	cfg := defaultLLMSettings()
	parseLLMSection("llm:\n  gen_addr: \":5001\"\n", &cfg)
	if cfg.Gen.Addr != ":5001" {
		t.Fatalf("gen_addr = %q, want :5001", cfg.Gen.Addr)
	}
	if !validLLMAddr(cfg.Gen.Addr) {
		t.Fatalf("validLLMAddr(%q) = false, want true", cfg.Gen.Addr)
	}

	// Адрес из текущего config.yaml (seedSettingsWorld) читается без кавычек.
	rr := mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings: status = %d", rr.Code)
	}
	var out settingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.LLM.Gen.Addr != ":5001" {
		t.Fatalf("GET settings llm.gen.addr = %q, want :5001", out.LLM.Gen.Addr)
	}

	// Round-trip: PUT принимает адрес и GET возвращает его без кавычек.
	rr = mcpRequest(t, handleSettingsConfig, http.MethodPut, settingsConfigRoutePath,
		[]byte(`{"gen_addr":":5007"}`), "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT config gen_addr: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	rr = mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings после PUT: status = %d", rr.Code)
	}
	out = settingsResponse{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.LLM.Gen.Addr != ":5007" {
		t.Fatalf("GET settings llm.gen.addr после PUT = %q, want :5007", out.LLM.Gen.Addr)
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
	content := "model-v1"
	stubModelDownload(t, func() string { return content })
	t.Cleanup(func() { modelsRoot = oldRoot })

	install := func(source, modelRef string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{
			"world":     "alpha",
			"role":      "weaver",
			"source":    source,
			"model_ref": modelRef,
		})
		return mcpRequest(t, handleSettingsModelInstall, http.MethodPost, settingsModelsInstallPath, body, "application/json")
	}

	if rr := install("https://huggingface.co/org/model-v1.gguf", "org/model"); rr.Code != http.StatusOK {
		t.Fatalf("install: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	content = "model-v2"
	if rr := install("https://huggingface.co/org/model-v2.gguf", "org/model-v2"); rr.Code != http.StatusOK {
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

// TestModelInstallWithoutWorld — файл модели скачивается до рождения мира:
// каталог общий, привязка роли не пишется, ответ помечает bound=false.
func TestModelInstallWithoutWorld(t *testing.T) {
	oldRoot := modelsRoot
	modelsRoot = filepath.Join(t.TempDir(), "models")
	stubModelDownload(t, func() string { return "model-v1" })
	t.Cleanup(func() { modelsRoot = oldRoot })

	body, _ := json.Marshal(map[string]string{
		"role":      "weaver",
		"source":    "https://huggingface.co/org/model",
		"model_ref": "org/model",
	})
	rr := mcpRequest(t, handleSettingsModelInstall, http.MethodPost, settingsModelsInstallPath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("install без мира: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Status string `json:"status"`
		Bound  bool   `json:"bound"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" || out.Bound {
		t.Fatalf("install без мира = %+v, want ok/bound=false", out)
	}
	if _, err := os.Stat(modelManifestPath(modelDir("weaver"))); err != nil {
		t.Fatalf("манифест модели не создан: %v", err)
	}
}

// TestEmbedModelInstallsIntoEmbedDir — «Проверка» T-12-16: карточка BGE-M3
// ставит эмбеддинг-модель в world_db/models/embed/.
func TestEmbedModelInstallsIntoEmbedDir(t *testing.T) {
	oldRoot := modelsRoot
	modelsRoot = filepath.Join(t.TempDir(), "models")
	stubModelDownload(t, func() string { return "bge-m3" })
	t.Cleanup(func() { modelsRoot = oldRoot })

	body, _ := json.Marshal(map[string]string{
		"role":      "embed",
		"source":    "https://huggingface.co/gpustack/bge-m3-GGUF/resolve/main/bge-m3-Q8_0.gguf",
		"model_ref": "bge-m3-Q8_0",
	})
	rr := mcpRequest(t, handleSettingsModelInstall, http.MethodPost, settingsModelsInstallPath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("install embed: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	manifestPath := modelManifestPath(modelDir("embed"))
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("эмбеддинг-модель не легла в world_db/models/embed/: %v", err)
	}
	var m modelManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.Role != "embed" || m.ModelRef != "bge-m3-Q8_0" {
		t.Fatalf("манифест = %+v, want embed/bge-m3-Q8_0", m)
	}
}

// seedModelAdoptEnv подменяет каталог моделей на временный и очищает реестр
// заданий: регистрация файла не видит установок соседних тестов.
func seedModelAdoptEnv(t *testing.T) string {
	t.Helper()
	oldRoot, oldJobs, oldDownloads := modelsRoot, modelJobs, modelDownloadDirs
	modelsRoot = filepath.Join(t.TempDir(), "models")
	downloads := filepath.Join(filepath.Dir(modelsRoot), "downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatal(err)
	}
	modelDownloadDirs = func() []string { return []string{downloads} }
	modelJobsMu.Lock()
	modelJobs = map[string]modelJobState{}
	modelJobsMu.Unlock()
	t.Cleanup(func() {
		modelsRoot = oldRoot
		modelDownloadDirs = oldDownloads
		modelJobsMu.Lock()
		modelJobs = oldJobs
		modelJobsMu.Unlock()
	})
	return modelsRoot
}

// adoptDownloadsDir — «папка загрузок браузера» текущего теста adopt.
func adoptDownloadsDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(filepath.Dir(modelsRoot), "downloads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// putAdoptedFile кладёт файл «скачанный браузером» в каталог роли.
func putAdoptedFile(t *testing.T, role, name, content string) string {
	t.Helper()
	dir := modelDir(role)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// putDownloadedFile кладёт файл «в папку загрузок браузера», как это делает
// браузер по умолчанию.
func putDownloadedFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(adoptDownloadsDir(t), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// postAdopt шлёт тело регистрации скачанного файла в хэндл adopt.
func postAdopt(t *testing.T, body map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return mcpRequest(t, handleSettingsModelAdopt, http.MethodPost, settingsModelsAdoptPath, raw, "application/json")
}

// TestModelAdoptFile — регистрация файла, скачанного браузером: манифест,
// sha256, привязка роли и статус installed без повторной загрузки.
func TestModelAdoptFile(t *testing.T) {
	seedSettingsWorld(t)
	seedModelAdoptEnv(t)
	oldFetch := fetchModelToFile
	fetchModelToFile = func(_ context.Context, _ models.DownloadSpec) (models.DownloadResult, error) {
		t.Fatal("adopt не должен качать файл заново")
		return models.DownloadResult{}, nil
	}
	t.Cleanup(func() { fetchModelToFile = oldFetch })

	content := "adopted-model"
	path := putAdoptedFile(t, "weaver", "weaver-Q4_K_M.gguf", content)
	body := map[string]string{
		"world":     "alpha",
		"role":      "weaver",
		"source":    "https://huggingface.co/org/repo/resolve/main/weaver-Q4_K_M.gguf",
		"model_ref": "org/weaver",
	}
	rr := postAdopt(t, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("adopt: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Status   string `json:"status"`
		ModelRef string `json:"model_ref"`
		File     string `json:"file"`
		Bound    bool   `json:"bound"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" || out.ModelRef != "org/weaver" || out.File != "weaver-Q4_K_M.gguf" || !out.Bound {
		t.Fatalf("adopt = %+v, want ok/org/weaver/weaver-Q4_K_M.gguf/bound", out)
	}

	m, ok := readModelManifest(modelDir("weaver"))
	if !ok {
		t.Fatal("манифест роли не записан")
	}
	sum := sha256.Sum256([]byte(content))
	if m.File != "weaver-Q4_K_M.gguf" || m.SHA256 != hex.EncodeToString(sum[:]) || m.Size != int64(len(content)) {
		t.Fatalf("манифест = %+v, want файл, sha256 и размер скачанного файла", m)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("файл модели пропал: %v", err)
	}

	status := mcpRequest(t, handleSettingsModelStatus, http.MethodGet,
		settingsModelsStatusPath+"?role=weaver", nil, "")
	if status.Code != http.StatusOK {
		t.Fatalf("status: %d, want 200; body=%s", status.Code, status.Body.String())
	}
	var st struct {
		Models []modelStatus `json:"models"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Models) != 1 || st.Models[0].Role != "weaver" || st.Models[0].Status != "installed" || !st.Models[0].IntegrityOK {
		t.Fatalf("статус weaver = %+v, want installed с целым файлом", st.Models)
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
	if ref != "org/weaver" {
		t.Fatalf("привязка = %q, want org/weaver", ref)
	}
}

// TestModelAdoptMissingFile — файла нет: 409, а текст отказа называет
// абсолютный путь, куда его положить.
func TestModelAdoptMissingFile(t *testing.T) {
	root := seedModelAdoptEnv(t)
	rr := postAdopt(t, map[string]string{
		"role":      "pythia",
		"source":    "https://huggingface.co/org/repo/resolve/main/pythia-Q4_K_M.gguf",
		"model_ref": "org/pythia",
	})
	if rr.Code != http.StatusConflict {
		t.Fatalf("adopt без файла: status = %d, want 409; body=%s", rr.Code, rr.Body.String())
	}
	want := filepath.Join(root, "pythia", "pythia-Q4_K_M.gguf")
	if !strings.Contains(rr.Body.String(), want) {
		t.Fatalf("текст отказа = %s, want путь %s", rr.Body.String(), want)
	}
	if _, ok := readModelManifest(modelDir("pythia")); ok {
		t.Fatal("манифест записан без файла модели")
	}
}

// TestModelAdoptFromDownloads — браузер оставил файл в папке загрузок:
// «Подключить» переносит его в каталог роли и регистрирует без ручного
// копирования по пути.
func TestModelAdoptFromDownloads(t *testing.T) {
	seedSettingsWorld(t)
	seedModelAdoptEnv(t)
	content := "downloaded-by-browser"
	src := putDownloadedFile(t, "weaver-Q4_K_M.gguf", content)
	rr := postAdopt(t, map[string]string{
		"world":     "alpha",
		"role":      "weaver",
		"source":    "https://huggingface.co/org/repo/resolve/main/weaver-Q4_K_M.gguf",
		"model_ref": "org/weaver",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("adopt из загрузок: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("исходник в загрузках не перенесён: %v", err)
	}
	target := filepath.Join(modelDir("weaver"), "weaver-Q4_K_M.gguf")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("файл не перенесён в каталог роли: %v", err)
	}
	if _, ok := readModelManifest(modelDir("weaver")); !ok {
		t.Fatal("манифест роли не записан после переноса")
	}
}

// TestModelAdoptZeroByteInDownloads — файл ещё качается (нулевой): 409 с
// текстом «не докачан», манифест не пишется.
func TestModelAdoptZeroByteInDownloads(t *testing.T) {
	seedModelAdoptEnv(t)
	putDownloadedFile(t, "pythia-Q4_K_M.gguf", "")
	rr := postAdopt(t, map[string]string{
		"role":      "pythia",
		"source":    "https://huggingface.co/org/repo/resolve/main/pythia-Q4_K_M.gguf",
		"model_ref": "org/pythia",
	})
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "not fully downloaded") {
		t.Fatalf("нулевой файл: status = %d body=%s, want 409 not fully downloaded", rr.Code, rr.Body.String())
	}
	if _, ok := readModelManifest(modelDir("pythia")); ok {
		t.Fatal("недокачанный файл попал в манифест роли")
	}
}

// TestModelAdoptBadRequests — не-HF источник, битое тело и пустой файл до
// манифеста не доходят.
func TestModelAdoptBadRequests(t *testing.T) {
	seedModelAdoptEnv(t)

	if rr := postAdopt(t, map[string]string{
		"role":      "editor",
		"source":    "https://example.com/repo/resolve/main/editor.gguf",
		"model_ref": "org/editor",
	}); rr.Code != http.StatusBadRequest {
		t.Fatalf("не-HF source: status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}

	rr := mcpRequest(t, handleSettingsModelAdopt, http.MethodPost,
		settingsModelsAdoptPath, []byte("{"), "application/json")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("битое тело: status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}

	putAdoptedFile(t, "analysis", "analysis-Q4_K_M.gguf", "")
	rr = postAdopt(t, map[string]string{
		"role":      "analysis",
		"source":    "https://huggingface.co/org/repo/resolve/main/analysis-Q4_K_M.gguf",
		"model_ref": "org/analysis",
	})
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "not fully downloaded") {
		t.Fatalf("пустой файл: status = %d body=%s, want 409 not fully downloaded", rr.Code, rr.Body.String())
	}
	if _, ok := readModelManifest(modelDir("analysis")); ok {
		t.Fatal("пустой файл попал в манифест роли")
	}
}

// TestModelAdoptBusyRole — параллельная регистрация той же роли получает 409,
// как и параллельная установка (T-13-02, шаг 6).
func TestModelAdoptBusyRole(t *testing.T) {
	seedModelAdoptEnv(t)
	putAdoptedFile(t, "editor", "editor-Q4_K_M.gguf", "editor")

	modelJobsMu.Lock()
	modelJobs["editor"] = modelJobState{Role: "editor", ModelRef: "org/busy", Status: "downloading"}
	modelJobsMu.Unlock()

	rr := postAdopt(t, map[string]string{
		"role":      "editor",
		"source":    "https://huggingface.co/org/repo/resolve/main/editor-Q4_K_M.gguf",
		"model_ref": "org/editor",
	})
	if rr.Code != http.StatusConflict {
		t.Fatalf("занятая роль: status = %d, want 409; body=%s", rr.Code, rr.Body.String())
	}
	if _, ok := readModelManifest(modelDir("editor")); ok {
		t.Fatal("манифест записан при занятой роли")
	}
}

// TestModelStatusDirs — статус отдаёт абсолютные каталоги всех пяти ролей
// каталога: по ним браузерная карточка показывает, куда сохранить файл.
func TestModelStatusDirs(t *testing.T) {
	root := seedModelAdoptEnv(t)
	rr := mcpRequest(t, handleSettingsModelStatus, http.MethodGet, settingsModelsStatusPath, nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Dirs map[string]string `json:"dirs"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"weaver", "pythia", "editor", "analysis", "embed"} {
		dir, ok := out.Dirs[role]
		if !ok {
			t.Fatalf("dirs без роли %q: %v", role, out.Dirs)
		}
		if !filepath.IsAbs(dir) {
			t.Fatalf("dirs[%s] = %q, want абсолютный путь", role, dir)
		}
		if want := filepath.Join(root, role); dir != want {
			t.Fatalf("dirs[%s] = %q, want %q", role, dir, want)
		}
	}
}

// TestSettingsReportsRamGB — рекомендация по RAM (T-12-16): ответ настроек
// отдаёт объём памяти станка для подсветки подходящего варианта BGE-M3.
func TestSettingsReportsRamGB(t *testing.T) {
	seedSettingsWorld(t)
	rr := mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath, nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings: status = %d", rr.Code)
	}
	var out settingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.RamGB < 0 {
		t.Fatalf("ram_gb = %f, want >= 0", out.RamGB)
	}
}

// TestParseMemTotalKB — разбор строки MemTotal из /proc/meminfo.
func TestParseMemTotalKB(t *testing.T) {
	cases := []struct {
		line string
		want float64
	}{
		{"MemTotal: 16440624 kB", 16440624},
		{"MemTotal:  8388608 kB", 8388608},
		{"MemTotal: 16777216 kB", 16777216},
		{"MemTotal: неизвестно", 0},
		{"", 0},
	}
	for _, c := range cases {
		if got := parseMemTotalKB(c.line); got != c.want {
			t.Errorf("parseMemTotalKB(%q) = %f, want %f", c.line, got, c.want)
		}
	}
}

// TestModelStatusEndpoint — карточка модели читает прогресс загрузки отдельным
// маршрутом, пока POST установки ещё не вернулся.
func TestModelStatusEndpoint(t *testing.T) {
	modelJobsMu.Lock()
	oldJobs := modelJobs
	modelJobs = map[string]modelJobState{}
	modelJobsMu.Unlock()
	t.Cleanup(func() {
		modelJobsMu.Lock()
		modelJobs = oldJobs
		modelJobsMu.Unlock()
	})

	setModelJob("weaver", "Llama-3.2-3B-Instruct-Q4_K_M", "downloading", 0.42, "")
	setModelJob("editor", "", "error", 0, "boom")

	rr := mcpRequest(t, handleSettingsModelStatus, http.MethodGet, settingsModelsStatusPath, nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d, want 200", rr.Code)
	}
	var out struct {
		Jobs []modelJobState `json:"jobs"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Jobs) != 2 {
		t.Fatalf("jobs = %+v, want 2", out.Jobs)
	}
	if out.Jobs[0].Role != "editor" || out.Jobs[0].Status != "error" {
		t.Fatalf("jobs[0] = %+v", out.Jobs[0])
	}
	if out.Jobs[1].ModelRef != "Llama-3.2-3B-Instruct-Q4_K_M" {
		t.Fatalf("jobs[1].model_ref = %q, want вариант модели", out.Jobs[1].ModelRef)
	}

	rr = mcpRequest(t, handleSettingsModelStatus, http.MethodGet, settingsModelsStatusPath+"?role=weaver", nil, "")
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Jobs) != 1 || out.Jobs[0].Role != "weaver" || out.Jobs[0].Progress != 0.42 {
		t.Fatalf("jobs по роли = %+v", out.Jobs)
	}
}

// TestModelStatusJobVariant — задание загрузки привязано к варианту роли: у
// embed два квантования, и задание Q8_0 не перебивает статус установленной
// карточки Q4_K_M (T-13-11).
func TestModelStatusJobVariant(t *testing.T) {
	seedModelAdoptEnv(t)
	const (
		refQ4 = "bge-m3-Q4_K_M"
		refQ8 = "bge-m3-Q8_0"
		file  = "bge-m3-Q4_K_M.gguf"
	)
	dir := modelDir("embed")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("gguf")
	sum := sha256.Sum256(content)
	if err := os.WriteFile(filepath.Join(dir, file), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeModelManifest(dir, modelManifest{
		Role: "embed", ModelRef: refQ4, Source: "https://huggingface.co/org/" + file,
		File: file, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(content)),
	}); err != nil {
		t.Fatal(err)
	}

	// readStatus читает статус-маршрут: одно задание с wantJobRef и карточка embed.
	readStatus := func(wantJobRef string) (modelJobState, modelStatus) {
		t.Helper()
		rr := mcpRequest(t, handleSettingsModelStatus, http.MethodGet, settingsModelsStatusPath, nil, "")
		if rr.Code != http.StatusOK {
			t.Fatalf("status: %d, want 200", rr.Code)
		}
		var out struct {
			Jobs   []modelJobState `json:"jobs"`
			Models []modelStatus   `json:"models"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if len(out.Jobs) != 1 || out.Jobs[0].ModelRef != wantJobRef {
			t.Fatalf("jobs = %+v, want одно задание %q", out.Jobs, wantJobRef)
		}
		for _, m := range out.Models {
			if m.Role == "embed" {
				return out.Jobs[0], m
			}
		}
		t.Fatal("карточки embed нет в ответе")
		return modelJobState{}, modelStatus{}
	}

	// Задание чужого варианта: карточка Q4_K_M остаётся установленной.
	setModelJob("embed", refQ8, "downloading", 0.5, "")
	job, card := readStatus(refQ8)
	if job.Progress != 0.5 {
		t.Fatalf("job = %+v, want прогресс 0.5", job)
	}
	if card.ModelRef != refQ4 || card.Status != modelStatusInstalled || card.Progress != 1 {
		t.Fatalf("карточка %s = %+v, want installed: чужой вариант её не трогает", refQ4, card)
	}

	// Задание своего варианта: статус и прогресс карточки — от задания.
	setModelJob("embed", refQ4, "downloading", 0.25, "")
	_, card = readStatus(refQ4)
	if card.Status != "downloading" || card.Progress != 0.25 {
		t.Fatalf("карточка %s = %+v, want downloading/0.25", refQ4, card)
	}

	// Наследие: задание без ref матчится по роли.
	setModelJob("embed", "", "downloading", 0.75, "")
	_, card = readStatus("")
	if card.Status != "downloading" || card.Progress != 0.75 {
		t.Fatalf("карточка при задании без ref = %+v", card)
	}
}

// TestModelRollbackIntegrity — откат на повреждённую прежнюю версию отвергается.
func TestModelRollbackIntegrity(t *testing.T) {
	seedSettingsWorld(t)
	oldRoot := modelsRoot
	modelsRoot = filepath.Join(t.TempDir(), "models")
	stubModelDownload(t, func() string { return "model-v1" })
	t.Cleanup(func() { modelsRoot = oldRoot })

	install := func(source, modelRef string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{
			"world": "alpha", "role": "weaver",
			"source": source, "model_ref": modelRef,
		})
		return mcpRequest(t, handleSettingsModelInstall, http.MethodPost, settingsModelsInstallPath, body, "application/json")
	}

	if rr := install("https://huggingface.co/org/model-v1.gguf", "org/model"); rr.Code != http.StatusOK {
		t.Fatalf("install: status = %d", rr.Code)
	}
	if rr := install("https://huggingface.co/org/model-v2.gguf", "org/model-v2"); rr.Code != http.StatusOK {
		t.Fatalf("replace: status = %d", rr.Code)
	}

	dir := modelDir("weaver")
	m, ok := readModelManifest(dir)
	if !ok || m.Previous == "" {
		t.Fatalf("манифест = %+v, want прежнюю версию", m)
	}
	if err := os.WriteFile(filepath.Join(dir, m.Previous), []byte("повреждено"), 0o644); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{"world": "alpha", "role": "weaver"})
	rr := mcpRequest(t, handleSettingsModelRollback, http.MethodPost, settingsModelsRollbackPath, body, "application/json")
	if rr.Code != http.StatusConflict {
		t.Fatalf("rollback повреждённой версии: status = %d, want 409", rr.Code)
	}
}

// TestSettingsGetWithoutWorld — GET /api/settings без мира отдаёт станочные
// разделы с пустыми мировыми массивами и дефолтами Advanced; непустой
// несуществующий мир по-прежнему 404 (W4.4).
func TestSettingsGetWithoutWorld(t *testing.T) {
	seedSettingsWorld(t)

	rr := mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath, nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s без мира: status = %d, want 200; body=%s", settingsPath, rr.Code, rr.Body.String())
	}
	var out settingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.World != "" {
		t.Fatalf("world = %q, want пусто", out.World)
	}
	if len(out.Bindings) != 0 {
		t.Fatalf("bindings = %d, want 0", len(out.Bindings))
	}
	if len(out.Models) != 0 {
		t.Fatalf("models = %d, want 0", len(out.Models))
	}
	if out.EmbeddingDimension != models.EmbedDim || out.Advanced.EmbeddingDimension != 1024 {
		t.Fatalf("embedding_dimension = %d/%d, want 1024", out.EmbeddingDimension, out.Advanced.EmbeddingDimension)
	}
	if out.Advanced.GlobalLuck != 0.5 {
		t.Fatalf("globalLuck = %v, want 0.5", out.Advanced.GlobalLuck)
	}
	if out.Advanced.MaxMemoryEntries != 0 || out.Advanced.SimilarityThreshold != 0 || out.Advanced.HalfLifeDays != 0 {
		t.Fatalf("memory defaults = %+v, want нули", out.Advanced)
	}

	// Непустой несуществующий мир — по-прежнему 404.
	rr = mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=nope", nil, "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET %s?world=nope: status = %d, want 404", settingsPath, rr.Code)
	}
}

// TestDayNormKmTunable — dayNormKm читается в Advanced (дефолт 25) и пишется
// через PUT /api/world/tunables в world_meta (W4.3).
func TestDayNormKmTunable(t *testing.T) {
	seedSettingsWorld(t)

	rr := mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings: status = %d", rr.Code)
	}
	var out settingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.Advanced.DayNormKm != geo.DefaultDayNormKm {
		t.Fatalf("dayNormKm по умолчанию = %v, want %v", out.Advanced.DayNormKm, geo.DefaultDayNormKm)
	}

	body := []byte(`{"world":"alpha","tunables":{"dayNormKm":30}}`)
	rr = mcpRequest(t, handleWorldTunables, http.MethodPut, worldTunablesPath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT tunables dayNormKm: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if got := readWorldMetaValues(t, worldDBPath("alpha")); got["dayNormKm"] != "30" {
		t.Fatalf("world_meta dayNormKm = %q, want 30", got["dayNormKm"])
	}

	rr = mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings после записи: status = %d", rr.Code)
	}
	out = settingsResponse{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.Advanced.DayNormKm != 30 {
		t.Fatalf("dayNormKm после записи = %v, want 30", out.Advanced.DayNormKm)
	}
}

// TestFirstRunNoFileHashing — «Проверка» T-13-04: статус-лист читает только
// манифесты и размеры файлов. В каталоге роли манифест валиден, а файл модели
// заменён каталогом (чтение невозможно) — /api/first-run всё равно отвечает
// installed; ?verify=1 на той же роли отвечает integrity_ok=false.
func TestFirstRunNoFileHashing(t *testing.T) {
	seedFirstRun(t)
	oldRoot := modelsRoot
	modelsRoot = filepath.Join(t.TempDir(), "models")
	oldJobs := modelJobs
	modelJobs = map[string]modelJobState{}
	t.Cleanup(func() {
		modelsRoot = oldRoot
		modelJobsMu.Lock()
		modelJobs = oldJobs
		modelJobsMu.Unlock()
	})

	// seedModel кладёт манифест роли и её файл (asDir — файл заменён каталогом);
	// sha256 в манифесте считается от shaOf, размер — от size.
	seedModel := func(role, file string, content, shaOf []byte, size int64, asDir bool) {
		t.Helper()
		dir := modelDir(role)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(shaOf)
		raw, err := json.Marshal(modelManifest{
			Role: role, ModelRef: "org/" + role, Source: "https://huggingface.co/org/" + role,
			File: file, SHA256: hex.EncodeToString(sum[:]), Size: size,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, modelManifestName), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, file)
		if asDir {
			if err := os.MkdirAll(filepath.Join(path, "chunk"), 0o755); err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// embed: файл модели заменён каталогом — прочитать его нельзя.
	seedModel("embed", "bge-m3-Q8_0.gguf", []byte("bge"), []byte("bge"), 3, true)
	// weaver: размер совпадает, содержимое другое — размерная проверка его
	// пропускает, полная сверка sha256 — нет.
	seedModel("weaver", "weaver-Q4_K_M.gguf", []byte("AAAA"), []byte("BBBB"), 4, false)

	cards := map[string]modelStatus{}
	for _, m := range getFirstRun(t).Models {
		cards[m.Role] = m
	}
	if m, ok := cards["embed"]; !ok || m.Status != "installed" || m.File != "bge-m3-Q8_0.gguf" {
		t.Fatalf("карточка embed в /api/first-run = %+v (есть=%v), want installed bge-m3-Q8_0.gguf", m, ok)
	}
	if m, ok := cards["weaver"]; !ok || m.Status != "installed" || !m.IntegrityOK {
		t.Fatalf("карточка weaver в /api/first-run = %+v (есть=%v), want installed без чтения файла", m, ok)
	}

	readStatus := func(query string) map[string]modelStatus {
		t.Helper()
		rr := mcpRequest(t, handleSettingsModelStatus, http.MethodGet, settingsModelsStatusPath+query, nil, "")
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %s%s: status = %d, want 200; body=%s", settingsModelsStatusPath, query, rr.Code, rr.Body.String())
		}
		var out struct {
			Models []modelStatus `json:"models"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("bad JSON: %v", err)
		}
		byRole := map[string]modelStatus{}
		for _, m := range out.Models {
			byRole[m.Role] = m
		}
		return byRole
	}

	cheap := readStatus("")
	if m, ok := cheap["weaver"]; !ok || !m.IntegrityOK {
		t.Fatalf("статус без verify: weaver = %+v (есть=%v), want integrity_ok=true без чтения файла", m, ok)
	}

	full := readStatus("?verify=1")
	if m, ok := full["embed"]; !ok || m.Status != "installed" || m.IntegrityOK {
		t.Fatalf("?verify=1 embed = %+v (есть=%v), want installed integrity_ok=false", m, ok)
	}
	if m, ok := full["weaver"]; !ok || m.IntegrityOK {
		t.Fatalf("?verify=1 weaver = %+v (есть=%v), want integrity_ok=false", m, ok)
	}

	// Полная сверка идёт сразу после скачивания: файл с чужим sha256 не
	// попадает в манифест роли.
	oldFetch := fetchModelToFile
	fetchModelToFile = func(_ context.Context, spec models.DownloadSpec) (models.DownloadResult, error) {
		if err := os.MkdirAll(filepath.Dir(spec.Dest), 0o755); err != nil {
			return models.DownloadResult{}, err
		}
		data := []byte("rogue")
		if err := os.WriteFile(spec.Dest, data, 0o644); err != nil {
			return models.DownloadResult{}, err
		}
		return models.DownloadResult{
			Path: spec.Dest, Size: int64(len(data)), SHA256: strings.Repeat("0", 64),
		}, nil
	}
	t.Cleanup(func() { fetchModelToFile = oldFetch })

	body, _ := json.Marshal(map[string]string{
		"role":      "editor",
		"source":    "https://huggingface.co/org/m/resolve/main/editor-Q4_K_M.gguf",
		"model_ref": "org/editor",
	})
	rr := mcpRequest(t, handleSettingsModelInstall, http.MethodPost, settingsModelsInstallPath, body, "application/json")
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("установка битого файла: status = %d, want 500; body=%s", rr.Code, rr.Body.String())
	}
	if _, ok := readModelManifest(modelDir("editor")); ok {
		t.Fatal("битый файл попал в манифест роли")
	}
}

// writeChildConfig пишет config.yaml для детей станции: путь llama-server и
// адреса слотов.
func writeChildConfig(t *testing.T, llamaPath, genAddr, embedAddr string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "llm:\n" +
		"  gen_addr: \"" + genAddr + "\"\n" +
		"  embed_addr: \"" + embedAddr + "\"\n" +
		"  llama_server_path: \"" + llamaPath + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeChildModel кладёт файл модели и манифест-указатель роли.
func writeChildModel(t *testing.T, role, file, previous string) string {
	t.Helper()
	dir := modelDir(role)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, file)
	if err := os.WriteFile(path, []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if previous != "" {
		if err := os.WriteFile(filepath.Join(dir, previous), []byte("gguf"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := modelManifest{Role: role, ModelRef: role, File: file, Previous: previous}
	if err := writeModelManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	return path
}

// childEmbedder — подставной llama-server эмбеддера: первые четыре запроса
// отдают одинаковый вектор (пулинг провален), дальше — bag-of-words.
type childEmbedder struct {
	mu    sync.Mutex
	calls int
}

func newChildEmbedder(t *testing.T) *httptest.Server {
	t.Helper()
	fe := &childEmbedder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/embeddings":
			var req struct {
				Input []string `json:"input"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			text := ""
			if len(req.Input) > 0 {
				text = req.Input[0]
			}
			fe.mu.Lock()
			fe.calls++
			n := fe.calls
			fe.mu.Unlock()
			vec := childFlatVector()
			if n > 4 {
				vec = childBagVector(text)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []any{map[string]any{"embedding": vec}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func childFlatVector() []float64 {
	v := make([]float64, models.EmbedDim)
	v[0] = 1
	return v
}

func childBagVector(text string) []float64 {
	vocab := []string{"кошка", "кот", "сидит", "окне", "грузовик", "едет", "шоссе", "на", "по"}
	v := make([]float64, models.EmbedDim)
	for _, w := range strings.Fields(text) {
		for i, x := range vocab {
			if w == x {
				v[i]++
			}
		}
	}
	return v
}

// seedChildEnv подменяет каталог моделей и пути config.yaml на временные.
func seedChildEnv(t *testing.T, cfg string) {
	t.Helper()
	oldRoot, oldCfg, oldLlama := modelsRoot, settingsConfigPath, llamaConfigPath
	modelsRoot = filepath.Join(t.TempDir(), "models")
	settingsConfigPath, llamaConfigPath = cfg, cfg
	t.Cleanup(func() {
		modelsRoot, settingsConfigPath, llamaConfigPath = oldRoot, oldCfg, oldLlama
	})
}

// TestChildModelSpecs — «Проверка» T-13-07: дети :5002/:5001 собираются по
// манифестам, bin — решённый обнаружением путь, а не зашитая строка.
func TestChildModelSpecs(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin", "llama-server")
	writeFakeLlama(t, bin, "version: 7777 (stub)")
	seedChildEnv(t, writeChildConfig(t, bin, ":5001", ":5002"))

	embedFile := writeChildModel(t, "embed", "bge-m3-Q8_0.gguf", "")
	genFile := writeChildModel(t, "weaver", "Llama-3.2-3B-Instruct-Q4_K_M.gguf", "")

	children := ModelChildren()
	if len(children) != 2 {
		t.Fatalf("ModelChildren = %d детей, want 2: %+v", len(children), children)
	}
	embed, gen := children[0], children[1]
	if embed.Name != "embed" || embed.Port != 5002 || embed.Cmd != bin {
		t.Fatalf("эмбеддер = %+v, want embed :5002 %s", embed, bin)
	}
	if !strings.Contains(strings.Join(embed.Args, " "), embedFile) {
		t.Fatalf("аргументы эмбеддера = %v, want файл %s", embed.Args, embedFile)
	}
	if gen.Name != "llama" || gen.Port != 5001 || gen.Cmd != bin {
		t.Fatalf("генерация = %+v, want llama :5001 %s", gen, bin)
	}
	if len(gen.Args) < 2 || gen.Args[0] != "--model" || gen.Args[1] != genFile {
		t.Fatalf("аргументы генерации = %v, want --model %s", gen.Args, genFile)
	}
}

// TestChildStartAfterInstall — «Проверка» T-13-07: после установки модели
// ребёнок роли стартует в рантайме с новым файлом, без перезапуска ядра.
func TestChildStartAfterInstall(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin", "llama-server")
	writeFakeLlama(t, bin, "version: 7777 (stub)")
	seedChildEnv(t, writeChildConfig(t, bin, ":5001", ":5002"))
	genFile := writeChildModel(t, "weaver", "Llama-3.2-3B-Instruct-Q4_K_M.gguf", "")

	var switched []supervisor.Config
	oldSwitch := modelChildSwitch
	modelChildSwitch = func(spec supervisor.Config) error {
		switched = append(switched, spec)
		return nil
	}
	t.Cleanup(func() { modelChildSwitch = oldSwitch })

	if got := startInstalledModelChild("weaver"); got != "started" {
		t.Fatalf("старт ребёнка = %q, want started", got)
	}
	if len(switched) != 1 {
		t.Fatalf("переключений = %d, want 1", len(switched))
	}
	spec := switched[0]
	if spec.Name != "llama" || spec.Port != 5001 || spec.HealthPath != "/health" {
		t.Fatalf("ребёнок = %+v, want llama :5001 /health", spec)
	}
	if len(spec.Args) < 2 || spec.Args[1] != genFile {
		t.Fatalf("аргументы = %v, want модель %s", spec.Args, genFile)
	}
}

// TestChildEmbedRollback — «Проверка» T-13-07: провал косинус-проверки пулинга
// после установки возвращает эмбеддер на прежний файл манифеста.
func TestChildEmbedRollback(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin", "llama-server")
	writeFakeLlama(t, bin, "version: 7777 (stub)")
	srv := newChildEmbedder(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	seedChildEnv(t, writeChildConfig(t, bin, ":5001", addr))

	newFile := writeChildModel(t, "embed", "bge-m3-Q8_0.gguf", "bge-m3-Q4_K_M.gguf")
	oldFile := filepath.Join(modelDir("embed"), "bge-m3-Q4_K_M.gguf")

	var switched []supervisor.Config
	oldSwitch := modelChildSwitch
	modelChildSwitch = func(spec supervisor.Config) error {
		switched = append(switched, spec)
		return nil
	}
	t.Cleanup(func() { modelChildSwitch = oldSwitch })

	if got := startInstalledModelChild("embed"); got != "rolled_back" {
		t.Fatalf("старт эмбеддера = %q, want rolled_back", got)
	}
	if len(switched) != 2 {
		t.Fatalf("переключений = %d, want 2 (новый файл → прежний)", len(switched))
	}
	if switched[0].Args[1] != newFile || switched[1].Args[1] != oldFile {
		t.Fatalf("файлы = %q → %q, want %s → %s",
			switched[0].Args[1], switched[1].Args[1], newFile, oldFile)
	}
}

// TestSettingsBindingsValidated — экран Настроек отвечает реальной картиной:
// локальная привязка активна только при установленном файле, внешняя — при
// ключе провайдера. Старая запись без файла уходит в неактивные (решение
// владельца 2026-11-13).
func TestSettingsBindingsValidated(t *testing.T) {
	seedSettingsWorld(t)
	seedModelAdoptEnv(t)
	if err := os.WriteFile(settingsTOMLPath, []byte("openai = \"sk-test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	content := "weaver-model"
	dir := modelDir("weaver")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "weaver-Q4_K_M.gguf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	if err := installModelFile("weaver", "org/weaver", "https://huggingface.co/org/repo/resolve/main/weaver-Q4_K_M.gguf",
		"weaver-Q4_K_M.gguf", hex.EncodeToString(sum[:]), int64(len(content))); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"world":"alpha","bindings":[
		{"role":"weaver","model_ref":"org/weaver","provider":"local"},
		{"role":"pythia","model_ref":"org/pythia","provider":"local"},
		{"role":"analysis","model_ref":"gpt-4o-mini","provider":"openai"},
		{"role":"editor","model_ref":"claude","provider":"anthropic"}
	]}`)
	if rr := mcpRequest(t, handleSettingsBindings, http.MethodPut, settingsBindingsPath, body, "application/json"); rr.Code != http.StatusOK {
		t.Fatalf("PUT bindings: status = %d; body=%s", rr.Code, rr.Body.String())
	}

	rr := mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings: status = %d; body=%s", rr.Code, rr.Body.String())
	}
	var out settingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	got := map[string]slotBinding{}
	for _, b := range out.Bindings {
		got[b.Role] = b
	}
	if b := got["weaver"]; !b.Active || b.State != bindingStateOK {
		t.Fatalf("weaver = %+v, want active/ok", b)
	}
	if b := got["pythia"]; b.Active || b.State != bindingStateNoModel {
		t.Fatalf("pythia = %+v, want inactive/no_model", b)
	}
	if b := got["analysis"]; !b.Active || b.State != bindingStateOK {
		t.Fatalf("analysis = %+v, want active/ok по ключу openai", b)
	}
	if b := got["editor"]; b.Active || b.State != bindingStateNoKey || !strings.Contains(b.Problem, "anthropic") {
		t.Fatalf("editor = %+v, want inactive/no_key про anthropic", b)
	}

	checks := map[string]stationCheck{}
	for _, c := range out.Station.Checks {
		checks[c.Key] = c
	}
	if c := checks["world"]; !c.OK || c.Value != "alpha" {
		t.Fatalf("station.world = %+v, want alpha/ok", c)
	}
	if c := checks["models"]; !c.OK || !strings.Contains(c.Value, "1 установлено") {
		t.Fatalf("station.models = %+v, want 1 установлено/ok", c)
	}
	if c := checks["bindings"]; c.OK || !strings.Contains(c.Value, "активны: 2 · неактивны: 2") {
		t.Fatalf("station.bindings = %+v, want активны: 2 · неактивны: 2 и не ok", c)
	}

	// Файл модели пропал — привязка перестаёт быть рабочей без правки БД.
	if err := os.Remove(filepath.Join(dir, "weaver-Q4_K_M.gguf")); err != nil {
		t.Fatal(err)
	}
	rr = mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=alpha", nil, "")
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for _, b := range out.Bindings {
		if b.Role == "weaver" && (b.Active || b.State != bindingStateNoFile) {
			t.Fatalf("weaver без файла = %+v, want inactive/no_file", b)
		}
	}
}

// TestSettingsBindingForget — «Забыть» удаляет только строку model_bindings;
// файлы моделей не трогаются, неверная роль или мир — 400/404.
func TestSettingsBindingForget(t *testing.T) {
	seedSettingsWorld(t)
	seedModelAdoptEnv(t)
	putAdoptedFile(t, "pythia", "pythia.gguf", "data")

	body := []byte(`{"world":"alpha","bindings":[{"role":"pythia","model_ref":"org/pythia","provider":"local"}]}`)
	if rr := mcpRequest(t, handleSettingsBindings, http.MethodPut, settingsBindingsPath, body, "application/json"); rr.Code != http.StatusOK {
		t.Fatalf("PUT bindings: status = %d", rr.Code)
	}

	rr := mcpRequest(t, handleSettingsBindingForget, http.MethodPost, "/api/settings/bindings/forget",
		[]byte(`{"world":"alpha","role":"pythia"}`), "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("forget: status = %d; body=%s", rr.Code, rr.Body.String())
	}
	world, err := db.OpenReaderAs(worldDBPath("alpha"), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer world.Close()
	var n int
	if err := world.QueryRow(`SELECT COUNT(*) FROM model_bindings`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("привязок после forget = %d, want 0", n)
	}
	if _, err := os.Stat(filepath.Join(modelDir("pythia"), "pythia.gguf")); err != nil {
		t.Fatalf("файл модели удалён вместе с привязкой: %v", err)
	}

	if rr := mcpRequest(t, handleSettingsBindingForget, http.MethodPost, "/api/settings/bindings/forget",
		[]byte(`{"world":"alpha"}`), "application/json"); rr.Code != http.StatusBadRequest {
		t.Fatalf("forget без роли: status = %d, want 400", rr.Code)
	}
	if rr := mcpRequest(t, handleSettingsBindingForget, http.MethodPost, "/api/settings/bindings/forget",
		[]byte(`{"world":"nope","role":"pythia"}`), "application/json"); rr.Code != http.StatusNotFound {
		t.Fatalf("forget неизвестного мира: status = %d, want 404", rr.Code)
	}
}
