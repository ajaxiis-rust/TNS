package paganel

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// ScoutBudgetDefault — бюджет разведки на сессию по умолчанию (W-10.4).
const ScoutBudgetDefault = 3

// ScoutTimeoutDefault — тайм-аут целевого вызова разведки по умолчанию (W-10.3).
const ScoutTimeoutDefault = 30 * time.Second

// PaganelConfig — настройки Паганеля из config.yaml (секция paganel, T-06-05,
// шаг 3). Переключатель «включён» — ковка колец; поля разведки наружу (T-06-10)
// лежат в поле Scout.
type PaganelConfig struct {
	Enabled bool        // paganel.enabled: ковка колец включена.
	Scout   ScoutConfig // paganel: разведка наружу (W-10).
}

// ScoutConfig — настройки разведки наружу (T-06-10, W-10): переключатель
// «походы наружу», бюджет сессии, тайм-аут целевого вызова и ссылка на ключ
// внешней модели (сам ключ — в settings.toml, B-4).
type ScoutConfig struct {
	Enabled bool          // scout_enabled: разрешение «походы наружу» (F-08).
	Budget  int           // scout_budget: вызовов разведки на сессию.
	Timeout time.Duration // scout_timeout: тайм-аут целевого вызова.
	KeyRef  string        // key_ref: ссылка на ключ в settings.toml.
}

// DefaultScoutConfig возвращает значения по умолчанию из config.yaml:
// разведка выключена, бюджет 3 вызова, тайм-аут 30 с.
func DefaultScoutConfig() ScoutConfig {
	return ScoutConfig{Enabled: false, Budget: ScoutBudgetDefault, Timeout: ScoutTimeoutDefault}
}

// DefaultPaganelConfig возвращает настройки, совпадающие со значениями
// config.yaml: ковка включена, разведка выключена.
func DefaultPaganelConfig() PaganelConfig {
	return PaganelConfig{Enabled: true, Scout: DefaultScoutConfig()}
}

// LoadPaganelConfig читает секцию paganel из файла конфигурации.
func LoadPaganelConfig(path string) (PaganelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PaganelConfig{}, err
	}
	return parsePaganelSection(string(data))
}

// parsePaganelSection разбирает топ-ключ `paganel:` со скалярными полями:
// enabled, build_budget, scout_enabled, scout_budget, scout_timeout, key_ref.
func parsePaganelSection(data string) (PaganelConfig, error) {
	cfg := DefaultPaganelConfig()

	inPaganel := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inPaganel = trimmed == "paganel:"
			continue
		}
		if !inPaganel {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		switch strings.TrimSpace(key) {
		case "enabled":
			if b, err := strconv.ParseBool(value); err == nil {
				cfg.Enabled = b
			}
		case "scout_enabled":
			if b, err := strconv.ParseBool(value); err == nil {
				cfg.Scout.Enabled = b
			}
		case "scout_budget":
			if n, err := strconv.Atoi(value); err == nil && n >= 0 {
				cfg.Scout.Budget = n
			}
		case "scout_timeout":
			if d, err := time.ParseDuration(value); err == nil && d > 0 {
				cfg.Scout.Timeout = d
			}
		case "key_ref":
			cfg.Scout.KeyRef = value
		}
	}
	if err := sc.Err(); err != nil {
		return PaganelConfig{}, err
	}
	return cfg, nil
}
