package webapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tns65/internal/db"
	"tns65/internal/embedserver"
	"tns65/internal/geo"
	"tns65/internal/journal"
	"tns65/internal/models"
	"tns65/internal/paganel"
	"tns65/internal/supervisor"
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
	settingsChannelPath        = "/api/settings/channel"
	settingsModelsInstallPath  = "/api/settings/models/install"
	settingsModelsStatusPath   = "/api/settings/models/status"
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
	"Канал",
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
	World              string                   `json:"world"`
	Sections           []string                 `json:"sections"`
	EmbeddingDimension int                      `json:"embedding_dimension"`
	General            generalSettings          `json:"general"`
	LLM                llmSettings              `json:"llm"`
	WorldSection       worldSettings            `json:"world_section"`
	Advanced           advancedSettings         `json:"advanced"`
	Channel            channelSettings          `json:"channel"`
	Bindings           []slotBinding            `json:"bindings"`
	Presets            []settingsPreset         `json:"presets"`
	Providers          []settingsProvider       `json:"providers"`
	Models             []modelStatus            `json:"models"`
	RamGB              float64                  `json:"ram_gb"`
	Embedder           embedserver.Status       `json:"embedder"`
	Supervisor         []supervisor.ChildStatus `json:"supervisor"`
}

// channelSettings — исходящий канал станка (T-08-03, F-04.3/F-04.4): край
// туннеля cloudflared, пробрасываемый loopback-адрес и Telegram-канал. Токен
// наружу не отдаётся — только признак; настроен бот, когда есть и токен, и
// chat_id.
type channelSettings struct {
	Edge               string `json:"edge"`
	Local              string `json:"local"`
	TelegramChatID     string `json:"telegram_chat_id"`
	TelegramTokenSet   bool   `json:"telegram_token_set"`
	TelegramConfigured bool   `json:"telegram_configured"`
	BotRunning         bool   `json:"bot_running"`
	Warning            string `json:"warning"`
}

// channelWarning — предупреждение Q-1: публичный туннель не аутентифицирует
// пришедшего; доступ получит любой, кто знает адрес.
const channelWarning = "Публичный туннель не имеет аутентификации"

// botRunning — запущен ли Telegram-бот. Флагом владеет задача W3 (цикл бота);
// экран Настроек читает его через Channel.BotRunning.
var botRunning atomic.Bool

// botGeneration — поколение запущенного канала. StartTelegram увеличивает его на
// старте; гасящая горутина снимает botRunning, только если она из текущего
// поколения (иначе перезапуск канала перекрыл бы флаг у нового бота).
var botGeneration atomic.Int64

// channelRestartWarning — последнее предупреждение перезапуска Telegram-канала,
// записанное PUT /api/settings/channel. Карточка Канал читает его в Warning.
var (
	channelRestartMu      sync.Mutex
	channelRestartWarning string
)

func setChannelRestartWarning(msg string) {
	channelRestartMu.Lock()
	channelRestartWarning = msg
	channelRestartMu.Unlock()
}

func getChannelRestartWarning() string {
	channelRestartMu.Lock()
	defer channelRestartMu.Unlock()
	return channelRestartWarning
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
	DayNormKm           float64  `json:"dayNormKm"`
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

	meta := map[string]string{}
	bindings := []slotBinding{}
	models := []modelStatus{}
	if slug != "" {
		world, err := db.OpenReaderAs(worldDBPath(slug), db.DBWorld, db.WriterCore, nil)
		if err != nil {
			writeError(w, http.StatusNotFound, codeNotFound, "world not found")
			return
		}
		defer world.Close()

		meta, err = readWorldMeta(world)
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "world meta read failed")
			return
		}
		bindings, err = readSlotBindings(world)
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "slot bindings read failed")
			return
		}
		bindings = enrichBindings(meta, bindings)
	}
	models = readModelStatuses()

	webCfg := Config{Host: DefaultHost, Port: DefaultPort}
	if cfg, err := LoadConfig(settingsConfigPath); err == nil {
		webCfg = cfg
	}
	pagCfg := paganel.DefaultPaganelConfig()
	if cfg, err := paganel.LoadPaganelConfig(settingsConfigPath); err == nil {
		pagCfg = cfg
	}
	tunCfg := DefaultTunnelConfig()
	if cfg, err := LoadTunnelConfig(settingsConfigPath); err == nil {
		tunCfg = cfg
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
			ExternalVectorSearch: keys["external_vector_search"] == "true",
		},
		LLM: readLLMSettings(settingsConfigPath, keys),
		WorldSection: worldSettings{
			Paganel: paganelSettings{
				Enabled:      pagCfg.Enabled,
				ScoutEnabled: pagCfg.Scout.Enabled,
				KeySet:       keys["paganel"] != "",
			},
		},
		Advanced:   buildAdvanced(meta),
		Channel:    buildChannelSettings(tunCfg, keys),
		Bindings:   bindings,
		Presets:    presets,
		Providers:  providers,
		Models:     models,
		RamGB:      hostRAMGB(),
		Embedder:   readEmbedderStatus(),
		Supervisor: readSupervisorStatus(),
	})
}

// SupervisorStatus возвращает снимок надзирателя; устанавливается ядром в
// main.go. Пока не установлено — пустой срез (статус надзирателя недоступен).
var SupervisorStatus func() []supervisor.ChildStatus

// readSupervisorStatus — снимок детей надзирателя для UI (T-12-19): запущен ли
// ребёнок, здоров ли и сколько раз он падал.
func readSupervisorStatus() []supervisor.ChildStatus {
	if SupervisorStatus == nil {
		return nil
	}
	return SupervisorStatus()
}

// embedStatusTimeout — потолок времени на опрос эмбеддера из UI. Отказ на
// незанятом loopback-порту мгновенен, но GET не должен висеть на чужом сокете.
const embedStatusTimeout = 3 * time.Second

// readEmbedderStatus — снимок состояния эмбеддера :5002 для UI (T-12-17):
// здоров ли сервер и прошла ли косинус-проверка пулинга.
func readEmbedderStatus() embedserver.Status {
	keys := readSettingsKeys(settingsTOMLPath)
	llm := readLLMSettings(settingsConfigPath, keys)
	addr := "http://" + normalizeAddr(llm.Embed.Addr)
	ctx, cancel := context.WithTimeout(context.Background(), embedStatusTimeout)
	defer cancel()
	return embedserver.StatusOf(ctx, addr, nil)
}

// buildChannelSettings собирает карточку канала из секции tunnel config.yaml и
// settings.toml. Токен читается из ключа telegram, при его отсутствии — из
// прежнего tunnel.telegram_token; сам токен наружу не возвращается.
func buildChannelSettings(tun TunnelConfig, keys map[string]string) channelSettings {
	token := keys["telegram"]
	if token == "" {
		token = tun.TelegramToken
	}
	tokenSet := token != ""
	chatID := tun.TelegramChatID
	return channelSettings{
		Edge:               tun.Edge,
		Local:              tun.Local,
		TelegramChatID:     chatID,
		TelegramTokenSet:   tokenSet,
		TelegramConfigured: tokenSet && chatID != "",
		BotRunning:         botRunning.Load(),
		Warning:            channelWarningValue(),
	}
}

// channelWarningValue — предупреждение карточки Канал: постоянное Q-1 плюс, если
// есть, последнее предупреждение перезапуска канала.
func channelWarningValue() string {
	if w := getChannelRestartWarning(); w != "" {
		return channelWarning + "; " + w
	}
	return channelWarning
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

// channelUpdateResponse — ответ PUT /api/settings/channel. warning появляется,
// когда перезапуск канала завершился ошибкой; bot_running — текущий флаг бота.
type channelUpdateResponse struct {
	Status     string `json:"status"`
	Warning    string `json:"warning,omitempty"`
	BotRunning bool   `json:"bot_running"`
}

// handleSettingsChannel — PUT /api/settings/channel: исходящий канал станка.
// edge/local/telegram_chat_id пишутся в секцию tunnel config.yaml, токен — в
// settings.toml под ключом telegram. Токен пишется только непустым; clear_token
// стирает его. Токен наружу не возвращается. Изменение токена, chat_id или
// clear_token перезапускает Telegram-канал; если канал после изменения не
// настроен — бот останавливается.
func handleSettingsChannel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Edge           string `json:"edge"`
		Local          string `json:"local"`
		TelegramChatID string `json:"telegram_chat_id"`
		TelegramToken  string `json:"telegram_token"`
		ClearToken     bool   `json:"clear_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad channel body")
		return
	}
	edge := strings.TrimSpace(req.Edge)
	if !validTunnelEdge(edge) {
		writeError(w, http.StatusBadRequest, codeBadRequest, "edge must be a bare hostname")
		return
	}
	tun := TunnelConfig{Edge: edge, Local: strings.TrimSpace(req.Local)}
	if err := tun.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "local must be a loopback address")
		return
	}

	oldChatID := ""
	if oldCfg, err := LoadTunnelConfig(settingsConfigPath); err == nil {
		oldChatID = strings.TrimSpace(oldCfg.TelegramChatID)
	}

	updates := map[string]string{
		"tunnel.edge":             edge,
		"tunnel.local":            tun.Local,
		"tunnel.telegram_chat_id": req.TelegramChatID,
	}
	if err := updateConfigScalars(settingsConfigPath, updates); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "channel config write failed")
		return
	}
	keys := readSettingsKeys(settingsTOMLPath)
	oldToken := keys["telegram"]
	newToken := oldToken
	if req.ClearToken {
		delete(keys, "telegram")
		newToken = ""
	} else if strings.TrimSpace(req.TelegramToken) != "" {
		keys["telegram"] = req.TelegramToken
		newToken = req.TelegramToken
	}
	if err := writeSettingsKeys(settingsTOMLPath, keys); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "token write failed")
		return
	}

	newChatID := strings.TrimSpace(req.TelegramChatID)
	if newToken == oldToken && newChatID == oldChatID {
		writeJSON(w, http.StatusOK, channelUpdateResponse{Status: "ok", BotRunning: botRunning.Load()})
		return
	}

	// Канал стал ненастроенным — гасим бота и честно сообщаем bot_running=false.
	if strings.TrimSpace(newToken) == "" || newChatID == "" {
		stopTelegramChannel()
		writeJSON(w, http.StatusOK, channelUpdateResponse{Status: "ok", BotRunning: false})
		return
	}

	// Канал настроен — перезапуск в горутине, ответ не ждёт сеть дольше секунды.
	writeJSON(w, http.StatusOK, channelUpdateResponse{
		Status:     "ok",
		Warning:    restartTelegramChannel(),
		BotRunning: botRunning.Load(),
	})
}

// stopTelegramChannel гасит текущий канал и снимает флаг bot_running. Хук
// StopTelegram пуст в тестах — тогда снимается только флаг.
func stopTelegramChannel() {
	if StopTelegram != nil {
		StopTelegram()
	}
	botRunning.Store(false)
	setChannelRestartWarning("")
}

// restartTelegramChannel запускает TelegramRestart в горутине и ждёт результата
// не дольше секунды, чтобы не блокировать HTTP-ответ. Ошибка хука возвращается
// предупреждением и запоминается для карточки Канал; без хука (тесты) — пусто.
func restartTelegramChannel() string {
	if TelegramRestart == nil {
		setChannelRestartWarning("")
		return ""
	}
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		done <- TelegramRestart(ctx)
	}()
	select {
	case err := <-done:
		if err == nil {
			setChannelRestartWarning("")
			return ""
		}
		msg := "перезапуск канала: " + err.Error()
		setChannelRestartWarning(msg)
		return msg
	case <-time.After(time.Second):
		setChannelRestartWarning("")
		return ""
	}
}

// validTunnelEdge сообщает, что край туннеля — bare hostname: без схемы,
// слэша и пробелов (Q-1). Пустая строка недопустима.
func validTunnelEdge(edge string) bool {
	if edge == "" {
		return false
	}
	if strings.ContainsAny(edge, "/ \t\r\n") {
		return false
	}
	return !strings.Contains(edge, "://")
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
		value = unquoteScalar(strings.TrimSpace(strings.SplitN(value, "#", 2)[0]))
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
	File        string  `json:"file"`
	Size        int64   `json:"size"`
	Status      string  `json:"status"`
	Progress    float64 `json:"progress"`
	IntegrityOK bool    `json:"integrity_ok"`
	HasPrevious bool    `json:"has_previous"`
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

// beginModelJob занимает роль под загрузку: одна активная установка на роль,
// вторая параллельная получает отказ (T-13-02, шаг 6).
func beginModelJob(role string) bool {
	modelJobsMu.Lock()
	defer modelJobsMu.Unlock()
	if j, ok := modelJobs[role]; ok && j.Status == "downloading" {
		return false
	}
	modelJobs[role] = modelJobState{Role: role, Status: "downloading"}
	return true
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

// Дети станции по установленным моделям (T-13-07): эмбеддер :5002 и генерация
// :5001. Роль эмбеддинга фиксирована, роли генерации — в порядке приоритета
// слота :5001.
const (
	embedModelRole = "embed"
	genModelChild  = "llama"
)

var generationModelRoles = []string{"weaver", "pythia", "editor", "analysis"}

// modelChildCheckTimeout — потолок health и косинус-проверки после установки.
const modelChildCheckTimeout = 30 * time.Second

// modelChildSwitch — переключение или запуск ребёнка станции в рантайме;
// устанавливается ядром в main.go (T-13-07, шаг 2).
var modelChildSwitch func(supervisor.Config) error

// SetModelChildSwitch подключает переключение детей станции: без хука
// установка модели детей не трогает.
func SetModelChildSwitch(f func(supervisor.Config) error) { modelChildSwitch = f }

// genServedRole — роль, которую сейчас обслуживает единственный ребёнок
// генерации; отмечается при каждой сборке его спеки (старт ядра, установка,
// откат установки — все пути идут через buildModelChildSpec).
var genServedRole atomic.Pointer[string]

// ModelChildName — имя живого ребёнка роли для остановки при удалении
// модели (T-13-03): «embed» для эмбеддера; «llama» — только когда ребёнок
// генерации обслуживает эту роль; пустая строка — ребёнка у роли нет.
func ModelChildName(role string) string {
	if role == embedModelRole {
		return (embedserver.Spec{}).Child().Name
	}
	if served := genServedRole.Load(); served != nil && *served == role {
		return genModelChild
	}
	return ""
}

// ModelChildren — резиденты станции по манифестам world_db/models/ (T-13-07,
// шаг 1): Spec.Bin — решённый обнаружением путь llama-server (T-13-06), не
// зашитая строка bin/llama-server. Без найденного бинарника детей нет.
func ModelChildren() []supervisor.Config {
	bin := detectLlamaServer().Path
	if bin == "" {
		return nil
	}
	llm := readLLMSettings(settingsConfigPath, readSettingsKeys(settingsTOMLPath))
	children := make([]supervisor.Config, 0, 2)
	if spec, ok := modelChildSpec(embedModelRole, bin, llm); ok {
		children = append(children, spec)
	}
	if role := generationModelRole(); role != "" {
		if spec, ok := modelChildSpec(role, bin, llm); ok {
			children = append(children, spec)
		}
	}
	return children
}

// modelCurrentFile — текущий файл роли по манифесту-указателю.
func modelCurrentFile(role string) (string, bool) {
	return modelManifestFile(role, func(m modelManifest) string { return m.File })
}

// modelPreviousFile — прежний файл роли из поля previous манифеста.
func modelPreviousFile(role string) (string, bool) {
	return modelManifestFile(role, func(m modelManifest) string { return m.Previous })
}

// modelManifestFile — существующий файл роли, выбранный из манифеста.
func modelManifestFile(role string, pick func(modelManifest) string) (string, bool) {
	dir := modelDir(role)
	m, ok := readModelManifest(dir)
	if !ok {
		return "", false
	}
	name := pick(m)
	if name == "" {
		return "", false
	}
	file := filepath.Join(dir, name)
	if info, err := os.Stat(file); err != nil || info.IsDir() {
		return "", false
	}
	return file, true
}

// generationModelRole — роль генерации для слота :5001: первая установленная
// из приоритетного списка, иначе любая установленная роль кроме эмбеддинга.
func generationModelRole() string {
	entries, err := os.ReadDir(modelsRoot)
	if err != nil {
		return ""
	}
	installed := make(map[string]bool, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, ok := modelCurrentFile(e.Name()); ok {
			installed[e.Name()] = true
		}
	}
	for _, role := range generationModelRoles {
		if installed[role] {
			return role
		}
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != embedModelRole && installed[e.Name()] {
			return e.Name()
		}
	}
	return ""
}

// modelChildSpec — спецификация ребёнка роли: эмбеддер — через
// embedserver.Spec.Child, генерация — флаги llama-server слота :5001.
func modelChildSpec(role, bin string, llm llmSettings) (supervisor.Config, bool) {
	file, ok := modelCurrentFile(role)
	if !ok {
		return supervisor.Config{}, false
	}
	return buildModelChildSpec(role, file, bin, llm), true
}

// buildModelChildSpec собирает ребёнка роли над конкретным файлом модели.
func buildModelChildSpec(role, file, bin string, llm llmSettings) supervisor.Config {
	if role == embedModelRole {
		host, port := slotHostPort(llm.Embed.Addr, embedserver.DefaultPort)
		return embedserver.Spec{Bin: bin, ModelPath: file, Host: host, Port: port}.Child()
	}
	host, port := slotHostPort(llm.Gen.Addr, 5001)
	genServedRole.Store(&role)
	return supervisor.Config{
		Name:       genModelChild,
		Cmd:        bin,
		Args:       []string{"--model", file, "--host", host, "--port", strconv.Itoa(port)},
		Port:       port,
		HealthPath: "/health",
	}
}

// slotHostPort разбирает адрес слота (:5001) в loopback host и порт.
func slotHostPort(addr string, defPort int) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return embedserver.DefaultHost, defPort
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return embedserver.DefaultHost, defPort
	}
	if host == "" {
		host = embedserver.DefaultHost
	}
	return host, port
}

// startInstalledModelChild — после установки модели перезапускает ребёнка роли
// без перезапуска ядра (T-13-07, шаг 2): health и, для эмбеддера, косинус
// пулинга; провал проверки — откат на прежний файл манифеста. Возвращает
// состояние ребёнка для ответа: started|rolled_back|failed, пусто — хука нет.
func startInstalledModelChild(role string) string {
	if modelChildSwitch == nil {
		return ""
	}
	bin := detectLlamaServer().Path
	if bin == "" {
		return ""
	}
	llm := readLLMSettings(settingsConfigPath, readSettingsKeys(settingsTOMLPath))
	spec, ok := modelChildSpec(role, bin, llm)
	if !ok {
		return ""
	}
	err := modelChildSwitch(spec)
	if err == nil && role == embedModelRole {
		err = checkEmbedChild(spec)
	}
	if err == nil {
		logModelChild(role, "model.child.started", spec, "")
		return "started"
	}
	logModelChild(role, "model.child.fail", spec, err.Error())
	prev, ok := modelPreviousFile(role)
	if !ok {
		return "failed"
	}
	prevSpec := buildModelChildSpec(role, prev, bin, llm)
	if switchErr := modelChildSwitch(prevSpec); switchErr != nil {
		logModelChild(role, "model.child.fail", prevSpec, switchErr.Error())
		return "failed"
	}
	if role == embedModelRole {
		if checkErr := checkEmbedChild(prevSpec); checkErr != nil {
			logModelChild(role, "model.child.fail", prevSpec, checkErr.Error())
			return "failed"
		}
	}
	logModelChild(role, "model.child.rolled_back", prevSpec, err.Error())
	return "rolled_back"
}

// checkEmbedChild — health и косинус-проверка пулинга запущенного эмбеддера.
func checkEmbedChild(spec supervisor.Config) error {
	host := embedserver.DefaultHost
	if len(spec.Args) > 0 {
		for i := 0; i+1 < len(spec.Args); i++ {
			if spec.Args[i] == "--host" {
				host = spec.Args[i+1]
				break
			}
		}
	}
	url := fmt.Sprintf("http://%s:%d", host, spec.Port)
	ctx, cancel := context.WithTimeout(context.Background(), modelChildCheckTimeout)
	defer cancel()
	return embedserver.Check(ctx, url, modelLogger)
}

// logModelChild пишет событие запуска ребёнка модели в журнал движка.
func logModelChild(role, event string, spec supervisor.Config, errMsg string) {
	if modelLogger == nil {
		return
	}
	fields := []journal.Field{
		{Key: "role", Value: role},
		{Key: "name", Value: spec.Name},
		{Key: "port", Value: strconv.Itoa(spec.Port)},
	}
	if len(spec.Args) > 1 && spec.Args[0] == "--model" {
		fields = append(fields, journal.Field{Key: "file", Value: spec.Args[1]})
	}
	if errMsg != "" {
		fields = append(fields, journal.Field{Key: "error", Value: errMsg})
	}
	_ = modelLogger.Event("model", event, fields...)
}

// handleSettingsModelStatus — GET /api/settings/models/status: карточки
// установленных моделей и текущие задания загрузки (MD-08.2). Карточка модели
// опрашивает этот маршрут во время установки и рисует прогресс внутри себя.
// role — необязательный фильтр; целостность полным sha256 — только по
// ?verify=1 (кнопка «Проверить», T-13-04).
func handleSettingsModelStatus(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	role := strings.TrimSpace(query.Get("role"))
	verify := query.Get("verify") == "1"
	modelJobsMu.Lock()
	jobs := make([]modelJobState, 0, len(modelJobs))
	for _, j := range modelJobs {
		if role == "" || j.Role == role {
			jobs = append(jobs, j)
		}
	}
	modelJobsMu.Unlock()
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Role < jobs[j].Role })
	models := fillModelCardSizes(readModelStatusesVerify(verify))
	if role != "" {
		kept := models[:0]
		for _, m := range models {
			if m.Role == role {
				kept = append(kept, m)
			}
		}
		models = kept
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs, "models": models})
}

// handleSettingsModelInstall — POST /api/settings/models/install: установка или
// замена модели роли из HF по явному запросу (MD-08.1, MD-09.3). Файл кладётся
// в world_db/models/<роль>/ под исходным именем из URL; привязка роли — в
// model_bindings хэндлом core.
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
	if role == "" || modelRef == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "role and model_ref required")
		return
	}
	if !isHFSource(source) {
		writeError(w, http.StatusBadRequest, codeBadRequest, "source must be a Hugging Face URL")
		return
	}
	file, err := modelSourceName(source)
	if err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "source must name a model file")
		return
	}

	// Мир необязателен: файлы моделей живут в world_db/models/ и скачиваются до
	// рождения мира. Привязка роли пишется только когда мир выбран.
	var h *db.Handle
	if slug != "" {
		h, err = db.OpenWriterAs(worldDBPath(slug), db.DBWorld, db.WriterCore, nil)
		if err != nil {
			writeError(w, http.StatusNotFound, codeNotFound, "world not found")
			return
		}
		defer h.Close()
	}

	// Одна активная загрузка на роль: вторая параллельная установка не гоняется
	// на файлах роли (T-13-02, шаг 6).
	if !beginModelJob(role) {
		writeError(w, http.StatusConflict, codeConflict, "model download already running")
		return
	}
	dir := modelDir(role)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		setModelJob(role, "error", 0, err.Error())
		writeError(w, http.StatusInternalServerError, codeInternal, "model dir create failed")
		return
	}
	res, err := fetchModelToFile(r.Context(), models.DownloadSpec{
		URL:    source,
		Dest:   filepath.Join(dir, file),
		Client: modelDownloadClient(),
		Logger: modelLogger,
		Progress: func(p models.DownloadProgress) {
			if p.Total > 0 {
				setModelJob(role, "downloading", float64(p.Done)/float64(p.Total), "")
			}
		},
	})
	if err != nil {
		setModelJob(role, "error", 0, err.Error())
		writeError(w, http.StatusInternalServerError, codeInternal, "model download failed")
		return
	}
	if res.Size == 0 {
		setModelJob(role, "error", 0, "empty model")
		writeError(w, http.StatusInternalServerError, codeInternal, "empty model")
		return
	}
	// Полная сверка sha256 сразу после скачивания (T-13-04): битый файл не
	// попадает в манифест, дальше статус-лист читает только размер.
	if !modelIntegrityOK(dir, file, res.SHA256) {
		_ = os.Remove(filepath.Join(dir, file))
		setModelJob(role, "error", 0, "model integrity check failed")
		writeError(w, http.StatusInternalServerError, codeInternal, "model integrity check failed")
		return
	}
	if err := installModelFile(role, modelRef, source, file, res.SHA256, res.Size); err != nil {
		setModelJob(role, "error", 0, err.Error())
		writeError(w, http.StatusInternalServerError, codeInternal, "model install failed")
		return
	}
	if h != nil {
		if err := writeSlotBindings(h, []slotBinding{{Role: role, ModelRef: modelRef, Provider: "local"}}); err != nil {
			setModelJob(role, "error", 0, err.Error())
			writeError(w, http.StatusInternalServerError, codeInternal, "binding write failed")
			return
		}
	}
	// T-13-07, шаг 2: ребёнок роли поднимается сразу, без перезапуска ядра.
	child := startInstalledModelChild(role)
	setModelJob(role, "installed", 1, "")
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "model_ref": modelRef, "file": file, "bound": h != nil, "child": child,
	})
}

// handleSettingsModelRollback — POST /api/settings/models/rollback: откат роли на
// прежнюю версию модели с проверкой целостности (MD-08.3). Файлы не
// переименовываются: меняется поле previous манифеста.
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

	m, err := rollbackModelFile(role)
	switch {
	case errors.Is(err, errModelNoPrevious), errors.Is(err, errModelPreviousGone):
		writeError(w, http.StatusConflict, codeConflict, "no previous model version")
		return
	case errors.Is(err, errModelPrevIntegrity):
		writeError(w, http.StatusConflict, codeConflict, "previous model integrity check failed")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, codeInternal, "model rollback failed")
		return
	}
	if err := writeSlotBindings(h, []slotBinding{{Role: role, ModelRef: m.ModelRef, Provider: "local"}}); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "binding write failed")
		return
	}
	setModelJob(role, "rolled_back", 1, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func nowSettingsUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// hostRAMGB — объём оперативной памяти станка в гигабайтах для подсказки, какой
// вариант BGE-M3 предложить (MD-03.2, UI-08). Читает /proc/meminfo; на системах
// без него (Windows, macOS) возвращает 0 — «неизвестно», карточка тогда не
// подсвечивает рекомендацию.
func hostRAMGB() float64 {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			return parseMemTotalKB(line) / (1024 * 1024)
		}
	}
	return 0
}

// parseMemTotalKB разбирает строку «MemTotal: 16440624 kB» в килобайты.
func parseMemTotalKB(line string) float64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	kb, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0
	}
	return kb
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
		DayNormKm:           tunableFloat(meta, "dayNormKm", geo.DefaultDayNormKm),
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
// переименовывает поверх target; падение не оставляет половину файла. Файл
// создаётся с правами 0600 — settings.toml хранит токен Telegram и ключи API.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
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

// Мастер первого запуска (T-12-20): экран из embed, состояние state/first-run.json,
// идемпотентные шаги 0–4. Файл состояния создаётся только после первого шага;
// его отсутствие и означает «первый запуск». Пройденные шаги при повторном
// запуске не переспрашиваются.

const (
	wizardScreenPath  = "/wizard.html"
	firstRunPathRoute = "/api/first-run"
	firstRunStepRoute = "/api/first-run/step"
)

// firstRunStatePath — путь состояния мастера; подменяется в тестах.
var firstRunStatePath = "state/first-run.json"

// firstRunStepSet — допустимые шаги мастера (0–4).
var firstRunStepSet = map[string]bool{"0": true, "1": true, "2": true, "3": true, "4": true}

// firstRunStepState — отметка одного шага мастера.
type firstRunStepState struct {
	Done bool   `json:"done"`
	At   string `json:"at,omitempty"`
}

// firstRunState — состояние мастера: пройденные шаги и признак завершения.
type firstRunState struct {
	Complete  bool                         `json:"complete"`
	Steps     map[string]firstRunStepState `json:"steps"`
	UpdatedAt string                       `json:"updated_at,omitempty"`
}

// firstRunStation — итог шага 0 «Проверяю станцию». llama_server несёт
// решённый путь и версию (T-13-06): визард показывает, что нашлось, и скрывает
// кнопку скачивания.
type firstRunStation struct {
	LlamaServer   llamaServerStatus `json:"llama_server"`
	GenPortFree   bool              `json:"gen_port_free"`
	EmbedPortFree bool              `json:"embed_port_free"`
	EmbedModel    string            `json:"embed_model"`
	Children      []stationChild    `json:"children"`
	RamGB         float64           `json:"ram_gb"`
}

// stationChild — живое состояние ребёнка станции (контракт §2, T-13-07):
// имя, порт и готовность.
type stationChild struct {
	Name  string `json:"name"`
	Port  int    `json:"port"`
	Ready bool   `json:"ready"`
}

// stationChildren — дети надзирателя для GET /api/first-run и «Настроек».
func stationChildren() []stationChild {
	statuses := readSupervisorStatus()
	out := make([]stationChild, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, stationChild{Name: s.Name, Port: s.Port, Ready: s.Running && s.Healthy})
	}
	return out
}

// firstRunResponse — GET/PUT /api/first-run: состояние плюс проверка станции
// и карточки установленных моделей (без чтения файлов, T-13-04).
type firstRunResponse struct {
	Complete bool                         `json:"complete"`
	Steps    map[string]firstRunStepState `json:"steps"`
	Station  firstRunStation              `json:"station"`
	Models   []modelStatus                `json:"models"`
}

// handleWizardScreen — GET /wizard.html: экран мастера первого запуска из embed.
func handleWizardScreen(w http.ResponseWriter, _ *http.Request) {
	data, err := fs.ReadFile(web.FS, "wizard.html")
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "wizard screen not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleFirstRunGet — GET /api/first-run: состояние мастера и проверка станции.
func handleFirstRunGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, buildFirstRunResponse(readFirstRunState()))
}

// handleFirstRunStep — PUT /api/first-run/step: помечает шаг пройденным
// (идемпотентно). Шаг 4 завершает мастера. Повторный вызов уже пройденного
// шага — успех без изменений.
func handleFirstRunStep(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Step string `json:"step"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad step body")
		return
	}
	step := strings.TrimSpace(req.Step)
	if !firstRunStepSet[step] {
		writeError(w, http.StatusBadRequest, codeBadRequest, "step must be one of 0..4")
		return
	}

	st := readFirstRunState()
	if st.Steps == nil {
		st.Steps = map[string]firstRunStepState{}
	}
	if !st.Steps[step].Done {
		st.Steps[step] = firstRunStepState{Done: true, At: nowSettingsUTC()}
	}
	if step == "4" {
		st.Complete = true
	}
	st.UpdatedAt = nowSettingsUTC()
	if err := writeFirstRunState(st); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "first-run state write failed")
		return
	}
	writeJSON(w, http.StatusOK, buildFirstRunResponse(st))
}

// readFirstRunState читает state/first-run.json; отсутствующий или битый файл —
// пустое состояние (первый запуск).
func readFirstRunState() firstRunState {
	data, err := os.ReadFile(firstRunStatePath)
	if err != nil {
		return firstRunState{Steps: map[string]firstRunStepState{}}
	}
	var st firstRunState
	if json.Unmarshal(data, &st) != nil {
		return firstRunState{Steps: map[string]firstRunStepState{}}
	}
	if st.Steps == nil {
		st.Steps = map[string]firstRunStepState{}
	}
	return st
}

// writeFirstRunState пишет состояние атомарно, создавая каталог state/ при
// необходимости (первый запуск до старта ядра каталог уже есть, но в тестах
// путь указывает в t.TempDir()).
func writeFirstRunState(st firstRunState) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(firstRunStatePath), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(firstRunStatePath, append(data, '\n'))
}

// buildFirstRunResponse собирает ответ: состояние, проверка станции и карточки
// моделей. Карточки берутся дешёвым статус-листом — экран мастера опрашивает
// маршрут каждые 2 с, полный sha256 остаётся за ?verify=1 (T-13-04).
func buildFirstRunResponse(st firstRunState) firstRunResponse {
	return firstRunResponse{
		Complete: st.Complete, Steps: st.Steps,
		Station: probeFirstRunStation(), Models: readModelStatuses(),
	}
}

// probeFirstRunStation собирает итог шага 0: найден ли llama-server (путь и
// версия), свободны ли порты :5001/:5002, установлена ли модель эмбеддингов.
// Ошибки проверок дают «ложь» — шаг 0 показывает игроку, чего не хватает, но не
// блокирует экран.
func probeFirstRunStation() firstRunStation {
	st := firstRunStation{RamGB: hostRAMGB()}
	st.LlamaServer = llamaStationStatus()
	st.GenPortFree = supervisor.CheckPortFree(5001) == nil
	st.EmbedPortFree = supervisor.CheckPortFree(5002) == nil
	st.Children = stationChildren()
	for _, m := range readModelStatuses() {
		if m.Role == "embed" && m.Status == "installed" {
			st.EmbedModel = m.ModelRef
			break
		}
	}
	return st
}
