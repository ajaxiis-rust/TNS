package webapi

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"tns65/internal/db"
	"tns65/internal/models"
	"tns65/internal/netgate"
	"tns65/internal/paganel"
	"tns65/internal/workshop"
	web "tns65/web"
)

// Вкладка «Настройки» (T-08-09, UI-08): всё станочное. Мировые параметры
// рождения сюда не входят; мировые настраиваемые параметры пишутся только через
// /api/world/tunables (T-08-04). Привязки слотов — model_bindings оперативной
// базы мира (писатель core); станочные настройки — config.yaml; ключи внешних
// провайдеров — settings.toml. Модели менеджера моделей (MD-08/MD-09) лежат
// файлами в world_db/models/ и загружаются только по явному запросу.

const (
	settingsScreenPath         = "/settings.html"
	settingsPath               = "/api/settings"
	settingsBindingsPath       = "/api/settings/bindings"
	settingsConfigRoutePath    = "/api/settings/config"
	settingsKeysPath           = "/api/settings/keys"
	settingsModelsInstallPath  = "/api/settings/models/install"
	settingsModelsRollbackPath = "/api/settings/models/rollback"
)

// embeddingDimension — размерность эмбеддинга BGE-M3 (D-16, MD-03.2).
const embeddingDimension = models.EmbedDim

// Имена ключей слотов в settings.toml (B-4): ключ слота генерации и эмбеддинга.
const (
	llmGenKey   = "llm_gen"
	llmEmbedKey = "llm_embed"
)

// settingsSections — состав вкладки «Настройки» (UI-08.1 без «Мониторинг»,
// который выделен отдельной задачей T-08-11).
var settingsSections = []string{
	"General",
	"LLM",
	"Провайдеры",
	"Модели",
	"Роли",
	"World",
	"Plugins",
	"Advanced",
}

// Пути конфигурации и ключей; подменяются в тестах.
var (
	settingsConfigPath = "config.yaml"
	settingsTOMLPath   = "settings.toml"
)

// modelsRoot — каталог моделей world_db/models/ (MD-09.5); подменяется в тестах.
var modelsRoot = filepath.Join(db.WorldsRoot, "models")

// slotBinding — привязка слота в model_bindings (T-02-11): роль, адрес модели и
// пресет-провайдер. Температура, промпты и приоритет — состав слота роли
// (T-08-09, шаг 5) — живут в world_meta под ключом role.<роль> (хэндл forge).
type slotBinding struct {
	Role        string  `json:"role"`
	ModelRef    string  `json:"model_ref"`
	Provider    string  `json:"provider"`
	Temperature float64 `json:"temperature,omitempty"`
	Prompts     string  `json:"prompts,omitempty"`
	Priority    int     `json:"priority,omitempty"`
}

// roleConfig — состав слота роли, который не помещается в model_bindings
// (там только role/model_ref/provider): температура, промпты, приоритет.
type roleConfig struct {
	Temperature float64 `json:"temperature"`
	Prompts     string  `json:"prompts"`
	Priority    int     `json:"priority"`
}

func roleConfigKey(role string) string { return "role." + role }

// llmSettings — слоты генерации (:5001) и эмбеддингов (:5002) из config.yaml
// (T-08-09, шаг 3, MD-04). Ключи слотов наружу не отдаются — только признак.
type llmSettings struct {
	Gen      llmSlotSettings `json:"gen"`
	Embed    llmSlotSettings `json:"embed"`
	Local    bool            `json:"local"`
	Sampling string          `json:"sampling"`
}

type llmSlotSettings struct {
	Addr        string  `json:"addr"`
	KeySet      bool    `json:"key_set"`
	Model       string  `json:"model"`
	Timeout     string  `json:"timeout"`
	MaxTokens   int64   `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	Retries     int     `json:"retries"`
	Concurrency int     `json:"concurrency"`
}

// settingsResponse — GET /api/settings: станочные настройки, мировые
// переключатели, память NPC, удача мира, привязки слотов и модели.
type settingsResponse struct {
	World              string             `json:"world"`
	Sections           []string           `json:"sections"`
	EmbeddingDimension int                `json:"embedding_dimension"`
	General            generalSettings    `json:"general"`
	LLM                llmSettings        `json:"llm"`
	WorldSection       worldSettings      `json:"world_section"`
	Advanced           advancedSettings   `json:"advanced"`
	Bindings           []slotBinding      `json:"bindings"`
	Presets            []settingsPreset   `json:"presets"`
	Providers          []settingsProvider `json:"providers"`
	Models             []modelStatus      `json:"models"`
}

type settingsPreset struct {
	Provider  string `json:"provider"`
	GenAddr   string `json:"gen_addr"`
	EmbedAddr string `json:"embed_addr"`
}

type settingsProvider struct {
	Name   string `json:"name"`
	KeySet bool   `json:"key_set"`
}

type generalSettings struct {
	Lang                 string `json:"lang"`
	Theme                string `json:"theme"`
	ServerHost           string `json:"server_host"`
	ServerPort           int    `json:"server_port"`
	DBPath               string `json:"db_path"`
	RestartRequired      bool   `json:"restart_required"`
	Auth                 string `json:"auth"`
	ExternalVectorSearch bool   `json:"external_vector_search"`
}

type worldSettings struct {
	Paganel paganelSettings `json:"paganel"`
}

type paganelSettings struct {
	Enabled      bool `json:"enabled"`
	ScoutEnabled bool `json:"scout_enabled"`
	KeySet       bool `json:"key_set"`
}

type advancedSettings struct {
	FeatureFlags        []string `json:"feature_flags"`
	MaxMemoryEntries    int64    `json:"maxMemoryEntries"`
	SimilarityThreshold float64  `json:"similarityThreshold"`
	HalfLifeDays        float64  `json:"halfLifeDays"`
	EmbeddingDimension  int      `json:"embedding_dimension"`
	GlobalLuck          float64  `json:"globalLuck"`
}

// handleSettingsScreen — GET /settings.html: экран Настроек из embed (статика из
// бинарника, F-11.3).
func handleSettingsScreen(w http.ResponseWriter, _ *http.Request) {
	data, err := fs.ReadFile(web.FS, "settings.html")
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "settings screen not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleSettingsGet — GET /api/settings?world=<slug>: чтение станочных настроек,
// мировых переключателей, памяти NPC, удачи мира и привязок слотов.
func handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("world"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	world, err := db.OpenReaderAs(worldDBPath(slug), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	defer world.Close()

	meta, err := readWorldMeta(world)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "world meta read failed")
		return
	}
	bindings, err := readSlotBindings(world)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "slot bindings read failed")
		return
	}
	bindings = enrichBindings(meta, bindings)

	webCfg := Config{Host: DefaultHost, Port: DefaultPort}
	if cfg, err := LoadConfig(settingsConfigPath); err == nil {
		webCfg = cfg
	}
	pagCfg := paganel.DefaultPaganelConfig()
	if cfg, err := paganel.LoadPaganelConfig(settingsConfigPath); err == nil {
		pagCfg = cfg
	}
	keys := readSettingsKeys(settingsTOMLPath)

	presets, providers := buildSettingsPresets(keys)

	writeJSON(w, http.StatusOK, settingsResponse{
		World:              slug,
		Sections:           settingsSections,
		EmbeddingDimension: embeddingDimension,
		General: generalSettings{
			Lang:                 "ru",
			Theme:                "default",
			ServerHost:           webCfg.Host,
			ServerPort:           webCfg.Port,
			DBPath:               db.WorldsRoot,
			RestartRequired:      false,
			Auth:                 "none",
			ExternalVectorSearch: false,
		},
		LLM: readLLMSettings(settingsConfigPath, keys),
		WorldSection: worldSettings{
			Paganel: paganelSettings{
				Enabled:      pagCfg.Enabled,
				ScoutEnabled: pagCfg.Scout.Enabled,
				KeySet:       keys["paganel"] != "",
			},
		},
		Advanced:  buildAdvanced(meta),
		Bindings:  bindings,
		Presets:   presets,
		Providers: providers,
		Models:    readModelStatuses(),
	})
}

// buildSettingsPresets собирает пресеты реестра T-02-11 и карточки внешних
// провайдеров с признаком «ключ введён» (сам ключ наружу не отдаётся).
func buildSettingsPresets(keys map[string]string) ([]settingsPreset, []settingsProvider) {
	factory := workshop.Presets()
	presets := make([]settingsPreset, 0, len(factory))
	providers := make([]settingsProvider, 0, len(factory))
	for _, p := range factory {
		presets = append(presets, settingsPreset{
			Provider:  string(p.Provider),
			GenAddr:   p.GenAddr,
			EmbedAddr: p.EmbedAddr,
		})
		if p.Provider != workshop.ProviderLocal {
			providers = append(providers, settingsProvider{
				Name:   string(p.Provider),
				KeySet: keys[string(p.Provider)] != "",
			})
		}
	}
	return presets, providers
}

// handleSettingsBindings — PUT /api/settings/bindings: привязки слотов пишутся в
// model_bindings хэндлом core (карта писателей T-01-07), а состав роли
// (температура, промпты, приоритет) — в world_meta хэндлом forge.
func handleSettingsBindings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		World    string        `json:"world"`
		Bindings []slotBinding `json:"bindings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad bindings body")
		return
	}
	slug := strings.TrimSpace(req.World)
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	if len(req.Bindings) == 0 {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bindings required")
		return
	}
	for _, b := range req.Bindings {
		if strings.TrimSpace(b.Role) == "" || strings.TrimSpace(b.Provider) == "" {
			writeError(w, http.StatusBadRequest, codeBadRequest, "role and provider required")
			return
		}
	}

	h, err := db.OpenWriterAs(worldDBPath(slug), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	defer h.Close()

	forge, err := db.OpenWriterAs(worldDBPath(slug), db.DBWorld, db.WriterForge, nil)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	defer forge.Close()

	if err := writeSlotBindings(h, req.Bindings); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "bindings write failed")
		return
	}
	if err := writeRoleConfigs(forge, req.Bindings); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "role config write failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleSettingsConfig — PUT /api/settings/config: станочные настройки web,
// переключатели Паганеля и LLM-слоты в config.yaml. Изменение порта требует
// перезапуска — это сказано в UI, а не в контракте ответа.
func handleSettingsConfig(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad config body")
		return
	}
	updates, ok := validateSettingsConfig(raw)
	if !ok {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad config value")
		return
	}
	if len(updates) == 0 {
		writeError(w, http.StatusBadRequest, codeBadRequest, "nothing to update")
		return
	}
	if err := updateConfigScalars(settingsConfigPath, updates); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "config write failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// validateSettingsConfig проверяет и перекладывает JSON-ключи в скаляры
// config.yaml (section.key). Неизвестный ключ — ok=false.
func validateSettingsConfig(raw map[string]json.RawMessage) (map[string]string, bool) {
	updates := map[string]string{}
	for key, val := range raw {
		switch key {
		case "host":
			var s string
			if err := json.Unmarshal(val, &s); err != nil || !validSettingsHost(s) {
				return nil, false
			}
			updates["web.host"] = s
		case "port":
			var n int
			if err := json.Unmarshal(val, &n); err != nil || n < 1 || n > 65535 {
				return nil, false
			}
			updates["web.port"] = strconv.Itoa(n)
		case "paganel_enabled":
			var b bool
			if err := json.Unmarshal(val, &b); err != nil {
				return nil, false
			}
			updates["paganel.enabled"] = strconv.FormatBool(b)
		case "scout_enabled":
			var b bool
			if err := json.Unmarshal(val, &b); err != nil {
				return nil, false
			}
			updates["paganel.scout_enabled"] = strconv.FormatBool(b)
		case "gen_addr", "embed_addr":
			var s string
			if err := json.Unmarshal(val, &s); err != nil || !validLLMAddr(s) {
				return nil, false
			}
			updates["llm."+key] = s
		case "gen_model", "embed_model", "gen_timeout", "embed_timeout":
			var s string
			if err := json.Unmarshal(val, &s); err != nil {
				return nil, false
			}
			if key == "gen_timeout" || key == "embed_timeout" {
				d, err := time.ParseDuration(s)
				if err != nil || d <= 0 {
					return nil, false
				}
			}
			updates["llm."+key] = s
		case "gen_max_tokens", "gen_retries":
			var n int
			if err := json.Unmarshal(val, &n); err != nil || n < 0 {
				return nil, false
			}
			updates["llm."+key] = strconv.Itoa(n)
		case "gen_concurrency":
			var n int
			if err := json.Unmarshal(val, &n); err != nil || n < 1 {
				return nil, false
			}
			updates["llm."+key] = strconv.Itoa(n)
		case "gen_temperature":
			var f float64
			if err := json.Unmarshal(val, &f); err != nil || f < 0 {
				return nil, false
			}
			updates["llm."+key] = strconv.FormatFloat(f, 'f', -1, 64)
		case "local":
			var b bool
			if err := json.Unmarshal(val, &b); err != nil {
				return nil, false
			}
			updates["llm.local"] = strconv.FormatBool(b)
		case "sampling":
			var s string
			if err := json.Unmarshal(val, &s); err != nil || (s != "greedy" && s != "top_p") {
				return nil, false
			}
			updates["llm.sampling"] = s
		default:
			return nil, false
		}
	}
	return updates, true
}

// handleSettingsKeys — PUT /api/settings/keys: ключи внешних провайдеров и
// слотов LLM в settings.toml (B-4). Живой контур settings.toml не читает
// (F-03.4).
func handleSettingsKeys(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Keys map[string]string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad keys body")
		return
	}
	if len(req.Keys) == 0 {
		writeError(w, http.StatusBadRequest, codeBadRequest, "keys required")
		return
	}
	keys := readSettingsKeys(settingsTOMLPath)
	for k, v := range req.Keys {
		k = strings.TrimSpace(k)
		if k == "" || v == "" {
			writeError(w, http.StatusBadRequest, codeBadRequest, "key name and value required")
			return
		}
		keys[k] = v
	}
	if err := writeSettingsKeys(settingsTOMLPath, keys); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "keys write failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// validSettingsHost сообщает, что host — loopback из фиксированной таблицы.
func validSettingsHost(host string) bool {
	_, ok := allowedHosts[host]
	return ok
}

// validLLMAddr сообщает, что адрес слота — host:port с валидным портом
// (пустой host означает «все интерфейсы», например :5001).
func validLLMAddr(addr string) bool {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

// readLLMSettings читает секцию llm из config.yaml; при отсутствии секции —
// заводские значения (T-08-09, шаг 3, MD-05). Ключи слотов берутся из
// settings.toml и наружу не отдаются.
func readLLMSettings(path string, keys map[string]string) llmSettings {
	cfg := defaultLLMSettings()
	if data, err := os.ReadFile(path); err == nil {
		parseLLMSection(string(data), &cfg)
	}
	cfg.Gen.KeySet = keys[llmGenKey] != ""
	cfg.Embed.KeySet = keys[llmEmbedKey] != ""
	return cfg
}

func defaultLLMSettings() llmSettings {
	return llmSettings{
		Gen:      llmSlotSettings{Addr: ":5001", Timeout: "30s", Concurrency: 1},
		Embed:    llmSlotSettings{Addr: ":5002", Timeout: "30s"},
		Local:    true,
		Sampling: "greedy",
	}
}

// parseLLMSection разбирает топ-ключ `llm:` в config.yaml.
func parseLLMSection(data string, cfg *llmSettings) {
	inLLM := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inLLM = line == "llm:"
			continue
		}
		if !inLLM {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		switch strings.TrimSpace(key) {
		case "gen_addr":
			cfg.Gen.Addr = value
		case "gen_model":
			cfg.Gen.Model = value
		case "gen_timeout":
			cfg.Gen.Timeout = value
		case "gen_max_tokens":
			if n, err := strconv.ParseInt(value, 10, 64); err == nil {
				cfg.Gen.MaxTokens = n
			}
		case "gen_temperature":
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				cfg.Gen.Temperature = f
			}
		case "gen_retries":
			if n, err := strconv.Atoi(value); err == nil {
				cfg.Gen.Retries = n
			}
		case "gen_concurrency":
			if n, err := strconv.Atoi(value); err == nil {
				cfg.Gen.Concurrency = n
			}
		case "embed_addr":
			cfg.Embed.Addr = value
		case "embed_model":
			cfg.Embed.Model = value
		case "embed_timeout":
			cfg.Embed.Timeout = value
		case "local":
			if b, err := strconv.ParseBool(value); err == nil {
				cfg.Local = b
			}
		case "sampling":
			cfg.Sampling = value
		}
	}
}

// readSlotBindings читает привязки слотов из model_bindings по роли.
func readSlotBindings(world *db.Handle) ([]slotBinding, error) {
	rows, err := world.Query(`SELECT role, model_ref, provider FROM model_bindings ORDER BY role`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []slotBinding{}
	for rows.Next() {
		var b slotBinding
		if err := rows.Scan(&b.Role, &b.ModelRef, &b.Provider); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// enrichBindings дополняет привязки составом роли из world_meta (температура,
// промпты, приоритет — T-08-09, шаг 5).
func enrichBindings(meta map[string]string, bindings []slotBinding) []slotBinding {
	out := make([]slotBinding, 0, len(bindings))
	for _, b := range bindings {
		if raw, ok := meta[roleConfigKey(b.Role)]; ok {
			var rc roleConfig
			if json.Unmarshal([]byte(raw), &rc) == nil {
				b.Temperature = rc.Temperature
				b.Prompts = rc.Prompts
				b.Priority = rc.Priority
			}
		}
		out = append(out, b)
	}
	return out
}

// writeSlotBindings вставляет или обновляет строки model_bindings по роли.
// Повторная привязка не дублирует строку.
func writeSlotBindings(h *db.Handle, bindings []slotBinding) error {
	for _, b := range bindings {
		var n int
		if err := h.QueryRow(`SELECT COUNT(*) FROM model_bindings WHERE role = ?`, b.Role).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := h.Exec("model_bindings",
				`INSERT INTO model_bindings (role, model_ref, provider, updated_at) VALUES (?, ?, ?, ?)`,
				b.Role, b.ModelRef, b.Provider, nowSettingsUTC()); err != nil {
				return err
			}
			continue
		}
		if _, err := h.Exec("model_bindings",
			`UPDATE model_bindings SET model_ref = ?, provider = ?, updated_at = ? WHERE role = ?`,
			b.ModelRef, b.Provider, nowSettingsUTC(), b.Role); err != nil {
			return err
		}
	}
	return nil
}

// writeRoleConfigs пишет состав роли в world_meta хэндлом forge. Пустой состав
// не пишется — строка не создаётся без данных.
func writeRoleConfigs(forge *db.Handle, bindings []slotBinding) error {
	for _, b := range bindings {
		if b.Temperature == 0 && b.Prompts == "" && b.Priority == 0 {
			continue
		}
		raw, err := json.Marshal(roleConfig{Temperature: b.Temperature, Prompts: b.Prompts, Priority: b.Priority})
		if err != nil {
			return err
		}
		if _, err := forge.Exec("world_meta",
			`INSERT INTO world_meta ("key", value) VALUES (?, ?)
			 ON CONFLICT("key") DO UPDATE SET value = excluded.value`,
			roleConfigKey(b.Role), string(raw)); err != nil {
			return err
		}
	}
	return nil
}

// modelStatus — карточка модели менеджера моделей (MD-08): роль, модель,
// источник, статус, прогресс загрузки, целостность и наличие прежней версии.
type modelStatus struct {
	Role        string  `json:"role"`
	ModelRef    string  `json:"model_ref"`
	Source      string  `json:"source"`
	Status      string  `json:"status"`
	Progress    float64 `json:"progress"`
	IntegrityOK bool    `json:"integrity_ok"`
	HasPrevious bool    `json:"has_previous"`
}

// modelManifest — манифест установленной модели в world_db/models/<роль>/.
type modelManifest struct {
	Role        string `json:"role"`
	ModelRef    string `json:"model_ref"`
	Source      string `json:"source"`
	SHA256      string `json:"sha256"`
	InstalledAt string `json:"installed_at"`
}

// modelJobState — текущий статус загрузки модели (MD-08.2). Реестр живёт в
// памяти процесса; загрузка идёт только по явному запросу владельца (MD-09.3).
type modelJobState struct {
	Role     string  `json:"role"`
	Status   string  `json:"status"`
	Progress float64 `json:"progress"`
	Error    string  `json:"error,omitempty"`
}

var (
	modelJobsMu sync.Mutex
	modelJobs   = map[string]modelJobState{}
)

func setModelJob(role, status string, progress float64, errMsg string) {
	modelJobsMu.Lock()
	defer modelJobsMu.Unlock()
	modelJobs[role] = modelJobState{Role: role, Status: status, Progress: progress, Error: errMsg}
}

func modelJob(role string) (modelJobState, bool) {
	modelJobsMu.Lock()
	defer modelJobsMu.Unlock()
	j, ok := modelJobs[role]
	return j, ok
}

// modelFetchFunc — загрузка байтов модели с прогрессом (0..1). Точка подмены в
// тестах; в производстве — исходящее соединение к HF через netgate (MD-09.1/2).
type modelFetchFunc func(ctx context.Context, source string, progress func(float64)) ([]byte, error)

var fetchModelFile modelFetchFunc = defaultFetchModelFile

func defaultFetchModelFile(ctx context.Context, source string, progress func(float64)) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	client := netgate.Client("huggingface.co")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("webapi: model source status %d", resp.StatusCode)
	}
	var buf bytes.Buffer
	total := resp.ContentLength
	var read int64
	tmp := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			_, _ = buf.Write(tmp[:n])
			read += int64(n)
			if total > 0 {
				progress(float64(read) / float64(total))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	progress(1)
	return buf.Bytes(), nil
}

// isHFSource сообщает, что источник модели — Hugging Face напрямую (MD-09.1).
func isHFSource(source string) bool {
	u, err := url.Parse(source)
	if err != nil {
		return false
	}
	return u.Scheme == "https" && (u.Host == "huggingface.co" || strings.HasSuffix(u.Host, ".huggingface.co"))
}

func modelDirName(role string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, role)
}

func modelDir(role string) string { return filepath.Join(modelsRoot, modelDirName(role)) }

// handleSettingsModelInstall — POST /api/settings/models/install: установка или
// замена модели роли из HF по явному запросу (MD-08.1, MD-09.3). Байты кладутся
// в world_db/models/<роль>/, привязка роли — в model_bindings хэндлом core.
func handleSettingsModelInstall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		World    string `json:"world"`
		Role     string `json:"role"`
		Source   string `json:"source"`
		ModelRef string `json:"model_ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad install body")
		return
	}
	slug := strings.TrimSpace(req.World)
	role := strings.TrimSpace(req.Role)
	source := strings.TrimSpace(req.Source)
	modelRef := strings.TrimSpace(req.ModelRef)
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	if role == "" || modelRef == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "role and model_ref required")
		return
	}
	if !isHFSource(source) {
		writeError(w, http.StatusBadRequest, codeBadRequest, "source must be a Hugging Face URL")
		return
	}

	h, err := db.OpenWriterAs(worldDBPath(slug), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	defer h.Close()

	setModelJob(role, "downloading", 0, "")
	data, err := fetchModelFile(r.Context(), source, func(p float64) {
		setModelJob(role, "downloading", p, "")
	})
	if err != nil {
		setModelJob(role, "error", 0, err.Error())
		writeError(w, http.StatusInternalServerError, codeInternal, "model download failed")
		return
	}
	if len(data) == 0 {
		setModelJob(role, "error", 0, "empty model")
		writeError(w, http.StatusInternalServerError, codeInternal, "empty model")
		return
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	if err := installModelFile(role, modelRef, source, sha, data); err != nil {
		setModelJob(role, "error", 0, err.Error())
		writeError(w, http.StatusInternalServerError, codeInternal, "model install failed")
		return
	}
	if err := writeSlotBindings(h, []slotBinding{{Role: role, ModelRef: modelRef, Provider: "local"}}); err != nil {
		setModelJob(role, "error", 0, err.Error())
		writeError(w, http.StatusInternalServerError, codeInternal, "binding write failed")
		return
	}
	setModelJob(role, "installed", 1, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "model_ref": modelRef})
}

// installModelFile атомарно кладёт байты модели и манифест, а текущую версию
// сохраняет как прежнюю для отката (MD-08.3).
func installModelFile(role, modelRef, source, sha string, data []byte) error {
	dir := modelDir(role)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cur := filepath.Join(dir, "current.bin")
	curMan := filepath.Join(dir, "current.json")
	prev := filepath.Join(dir, "previous.bin")
	prevMan := filepath.Join(dir, "previous.json")
	if _, err := os.Stat(cur); err == nil {
		if err := os.Rename(cur, prev); err != nil {
			return err
		}
		if err := os.Rename(curMan, prevMan); err != nil {
			return err
		}
	}
	if err := writeFileAtomic(cur, data); err != nil {
		return err
	}
	manifest := modelManifest{Role: role, ModelRef: modelRef, Source: source, SHA256: sha, InstalledAt: nowSettingsUTC()}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(curMan, raw)
}

// handleSettingsModelRollback — POST /api/settings/models/rollback: откат роли на
// прежнюю версию модели с проверкой целостности (MD-08.3).
func handleSettingsModelRollback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		World string `json:"world"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad rollback body")
		return
	}
	slug := strings.TrimSpace(req.World)
	role := strings.TrimSpace(req.Role)
	if slug == "" || role == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world and role required")
		return
	}

	h, err := db.OpenWriterAs(worldDBPath(slug), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	defer h.Close()

	dir := modelDir(role)
	prev := filepath.Join(dir, "previous.bin")
	prevMan := filepath.Join(dir, "previous.json")
	raw, err := os.ReadFile(prevMan)
	if err != nil {
		writeError(w, http.StatusConflict, codeConflict, "no previous model version")
		return
	}
	var manifest modelManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		writeError(w, http.StatusConflict, codeConflict, "bad previous manifest")
		return
	}
	data, err := os.ReadFile(prev)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "previous model read failed")
		return
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != manifest.SHA256 {
		writeError(w, http.StatusConflict, codeConflict, "previous model integrity check failed")
		return
	}

	cur := filepath.Join(dir, "current.bin")
	curMan := filepath.Join(dir, "current.json")
	if err := os.Rename(prev, cur); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "model rollback failed")
		return
	}
	if err := os.Rename(prevMan, curMan); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "model rollback failed")
		return
	}
	if err := writeSlotBindings(h, []slotBinding{{Role: role, ModelRef: manifest.ModelRef, Provider: "local"}}); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "binding write failed")
		return
	}
	setModelJob(role, "rolled_back", 1, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readModelStatuses читает карточки установленных моделей из world_db/models/.
func readModelStatuses() []modelStatus {
	entries, err := os.ReadDir(modelsRoot)
	if err != nil {
		return nil
	}
	out := []modelStatus{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(modelsRoot, e.Name())
		raw, err := os.ReadFile(filepath.Join(dir, "current.json"))
		if err != nil {
			continue
		}
		var m modelManifest
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		st := modelStatus{
			Role:        m.Role,
			ModelRef:    m.ModelRef,
			Source:      m.Source,
			Status:      "installed",
			Progress:    1,
			IntegrityOK: modelIntegrityOK(dir, m),
		}
		if _, err := os.Stat(filepath.Join(dir, "previous.json")); err == nil {
			st.HasPrevious = true
		}
		if j, ok := modelJob(m.Role); ok {
			st.Status = j.Status
			st.Progress = j.Progress
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Role < out[j].Role })
	return out
}

// modelIntegrityOK сверяет sha256 файла текущей модели с манифестом (MD-08.3).
func modelIntegrityOK(dir string, m modelManifest) bool {
	data, err := os.ReadFile(filepath.Join(dir, "current.bin"))
	if err != nil {
		return false
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) == m.SHA256
}

func nowSettingsUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// readWorldMeta читает world_meta как карту «ключ → значение» (только чтение).
func readWorldMeta(h *db.Handle) (map[string]string, error) {
	rows, err := h.Query(`SELECT "key", value FROM world_meta`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func buildAdvanced(meta map[string]string) advancedSettings {
	return advancedSettings{
		FeatureFlags:        []string{},
		MaxMemoryEntries:    tunableInt(meta, "maxMemoryEntries", 0),
		SimilarityThreshold: tunableFloat(meta, "similarityThreshold", 0),
		HalfLifeDays:        tunableFloat(meta, "halfLifeDays", 0),
		EmbeddingDimension:  embeddingDimension,
		GlobalLuck:          tunableFloat(meta, "globalLuck", 0.5),
	}
}

func tunableFloat(meta map[string]string, key string, def float64) float64 {
	v, ok := meta[key]
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func tunableInt(meta map[string]string, key string, def int64) int64 {
	v, ok := meta[key]
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

// readSettingsKeys читает settings.toml как top-level строки key = "value";
// файла нет — пустая карта.
func readSettingsKeys(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	out := map[string]string{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if unquoted, err := strconv.Unquote(val); err == nil {
			val = unquoted
		} else {
			val = strings.Trim(val, `"'`)
		}
		if key != "" {
			out[key] = val
		}
	}
	return out
}

// writeSettingsKeys записывает settings.toml атомарно: временный файл и rename.
func writeSettingsKeys(path string, keys map[string]string) error {
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, k := range names {
		b.WriteString(k)
		b.WriteString(" = ")
		b.WriteString(strconv.Quote(keys[k]))
		b.WriteString("\n")
	}
	return writeFileAtomic(path, []byte(b.String()))
}

// updateConfigScalars меняет заданные top-level скаляры (section.key) в
// config.yaml, сохраняя остальные строки, и перезаписывает файл атомарно.
func updateConfigScalars(path string, updates map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	section := ""
	applied := map[string]bool{}
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}
		key, _, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		full := section + "." + key
		val, want := updates[full]
		if !want {
			continue
		}
		lines[i] = strings.Repeat(" ", indent) + key + ": " + val
		applied[full] = true
	}
	for full := range updates {
		if !applied[full] {
			return fmt.Errorf("webapi: config key %s not found", full)
		}
	}
	return writeFileAtomic(path, []byte(strings.Join(lines, "\n")))
}

// writeFileAtomic записывает данные во временный файл в том же каталоге и
// переименовывает поверх target; падение не оставляет половину файла.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
