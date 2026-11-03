package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

	setModelJob("weaver", "downloading", 0.42, "")
	setModelJob("editor", "error", 0, "boom")

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

	rr = mcpRequest(t, handleSettingsModelStatus, http.MethodGet, settingsModelsStatusPath+"?role=weaver", nil, "")
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Jobs) != 1 || out.Jobs[0].Role != "weaver" || out.Jobs[0].Progress != 0.42 {
		t.Fatalf("jobs по роли = %+v", out.Jobs)
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
