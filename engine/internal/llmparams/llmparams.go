// Параметры генерации слота из настроек станка и состава роли (П3 плана
// GEN-PARAMS-PROMPTS): config.yaml `llm:` и world_meta `role.<роль>`.
// Значения живут в общем пакете, чтобы webapi (парсинг, экран настроек) и
// мастерская (тело запроса модели) читали один и тот же источник.
package llmparams

import (
	"bufio"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"tns65/internal/db"
	"tns65/internal/models"
)

// ReasoningUnset — значение gen_reasoning «поле не отправлять»: рассуждения
// остаются дефолтом модели.
const ReasoningUnset = -1

// roleKeyPrefix — префикс ключа world_meta с составом роли.
const roleKeyPrefix = "role."

// RoleConfig — состав слота роли из world_meta role.<роль>: температура,
// промпты и приоритет. Нулевая температура — «не задана»: значение берётся из
// config.yaml llm.gen_temperature.
type RoleConfig struct {
	Temperature float64 `json:"temperature"`
	Prompts     string  `json:"prompts"`
	Priority    int     `json:"priority"`
}

// RoleConfigKey — ключ world_meta состава роли.
func RoleConfigKey(role string) string { return roleKeyPrefix + role }

// ParseRoleConfig разбирает JSON состава роли; битый JSON — ok=false.
func ParseRoleConfig(raw string) (RoleConfig, bool) {
	var rc RoleConfig
	if err := json.Unmarshal([]byte(raw), &rc); err != nil {
		return RoleConfig{}, false
	}
	return rc, true
}

// ReadRoleConfigs читает составы всех ролей мира из world_meta.
func ReadRoleConfigs(h *db.Handle) (map[string]RoleConfig, error) {
	rows, err := h.Query(`SELECT "key", value FROM world_meta WHERE "key" LIKE 'role.%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]RoleConfig{}
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		rc, ok := ParseRoleConfig(raw)
		if !ok {
			continue
		}
		out[strings.TrimPrefix(key, roleKeyPrefix)] = rc
	}
	return out, rows.Err()
}

// LoadRoleConfigs читает составы ролей из файла оперативной базы мира.
func LoadRoleConfigs(path string) (map[string]RoleConfig, error) {
	h, err := db.OpenReaderAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		return nil, err
	}
	defer h.Close()
	return ReadRoleConfigs(h)
}

// Config — параметры генерации станка из config.yaml секции `llm:`. nil-поле
// означает «ключа нет»: соответствующее поле запроса не отправляется, модель
// работает на своём дефолте.
type Config struct {
	Timeout     time.Duration // gen_timeout: тайм-аут HTTP-клиента слота
	MaxTokens   *int          // gen_max_tokens; 0 — без ограничения (не отправлять)
	Temperature *float64      // gen_temperature
	TopP        *float64      // gen_top_p; 0 — не отправлять
	TopK        *int          // gen_top_k; 0 — не отправлять
	Reasoning   *int          // gen_reasoning: -1 — не отправлять, 0 — выключить, >0 — бюджет
}

// LoadSettings читает секцию llm из config.yaml; файла нет — пустая настройка
// (все поля запроса не отправляются).
func LoadSettings(path string) Config {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}
	}
	return ParseSettings(string(data))
}

// ParseSettings разбирает топ-ключ `llm:` в config.yaml.
func ParseSettings(data string) Config {
	var cfg Config
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
		value = scalar(value)
		switch strings.TrimSpace(key) {
		case "gen_timeout":
			if d, err := time.ParseDuration(value); err == nil && d > 0 {
				cfg.Timeout = d
			}
		case "gen_max_tokens":
			if n, err := strconv.Atoi(value); err == nil && n > 0 {
				cfg.MaxTokens = &n
			}
		case "gen_temperature":
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				cfg.Temperature = &f
			}
		case "gen_top_p":
			if f, err := strconv.ParseFloat(value, 64); err == nil && f > 0 {
				cfg.TopP = &f
			}
		case "gen_top_k":
			if n, err := strconv.Atoi(value); err == nil && n > 0 {
				cfg.TopK = &n
			}
		case "gen_reasoning":
			if n, err := strconv.Atoi(value); err == nil && n >= 0 {
				cfg.Reasoning = &n
			}
		}
	}
	return cfg
}

// scalar обрезает комментарий и кавычки у значения YAML.
func scalar(v string) string {
	v = strings.TrimSpace(strings.SplitN(v, "#", 2)[0])
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// Resolve собирает параметры запроса: состав роли главнее config.yaml, пустое —
// дефолт модели. У роли есть только температура; прочие параметры — станковые.
func Resolve(cfg Config, role *RoleConfig) models.GenerateParams {
	params := models.GenerateParams{
		MaxTokens:   cfg.MaxTokens,
		Temperature: cfg.Temperature,
		TopP:        cfg.TopP,
		TopK:        cfg.TopK,
		Reasoning:   cfg.Reasoning,
	}
	if role != nil && role.Temperature != 0 {
		t := role.Temperature
		params.Temperature = &t
	}
	return params
}

// Снимок настроек процесса: станковые значения и составы ролей активного мира.
// Проводка ставит его на старте (config.yaml) и при открытии мира (world_meta).
var (
	mu    sync.RWMutex
	cfg   Config
	roles map[string]RoleConfig
)

// SetConfig задаёт станковые параметры генерации; пустая настройка снимает их.
func SetConfig(c Config) {
	mu.Lock()
	cfg = c
	mu.Unlock()
}

// SetRoleConfigs задаёт составы ролей активного мира; nil снимает их.
func SetRoleConfigs(m map[string]RoleConfig) {
	mu.Lock()
	roles = m
	mu.Unlock()
}

// Params возвращает параметры генерации роли по снимку настроек процесса.
func Params(role string) models.GenerateParams {
	mu.RLock()
	c := cfg
	rc, ok := roles[role]
	mu.RUnlock()
	if ok {
		return Resolve(c, &rc)
	}
	return Resolve(c, nil)
}
