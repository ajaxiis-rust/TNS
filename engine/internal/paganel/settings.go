package paganel

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// PaganelConfig — настройки Паганеля из config.yaml (секция paganel, T-06-05,
// шаг 3). Здесь только переключатель «включён»; поля разведки добавляются в
// T-06-10.
type PaganelConfig struct {
	Enabled bool // paganel.enabled: ковка колец включена.
}

// DefaultPaganelConfig возвращает настройки, совпадающие со значениями
// config.yaml: ковка включена.
func DefaultPaganelConfig() PaganelConfig {
	return PaganelConfig{Enabled: true}
}

// LoadPaganelConfig читает секцию paganel из файла конфигурации.
func LoadPaganelConfig(path string) (PaganelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PaganelConfig{}, err
	}
	return parsePaganelSection(string(data))
}

// parsePaganelSection разбирает топ-ключ `paganel:` со скалярным полем enabled.
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
		if strings.TrimSpace(key) == "enabled" {
			if b, err := strconv.ParseBool(value); err == nil {
				cfg.Enabled = b
			}
		}
	}
	if err := sc.Err(); err != nil {
		return PaganelConfig{}, err
	}
	return cfg, nil
}
