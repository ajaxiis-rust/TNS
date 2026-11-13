// Package providercfg читает providers.toml — внешний реестр внешних слотов
// генерации: адрес, протокол, заводская модель и справочник цен. Файл лежит
// рядом с settings.toml и читается при каждом обращении, поэтому добавление
// провайдера или правка цены не требуют пересборки ядра.
package providercfg

import (
	"os"
	"strconv"
	"strings"
)

// Protocol — протокол общения с провайдером.
type Protocol string

const (
	// ProtocolLocal — локальная запаска :5001.
	ProtocolLocal Protocol = "local"
	// ProtocolGemini — Gemini generateContent.
	ProtocolGemini Protocol = "gemini"
	// ProtocolOpenAI — OpenAI-совместимый /chat/completions.
	ProtocolOpenAI Protocol = "openai"
)

// Provider — определение внешнего слота из providers.toml.
type Provider struct {
	Name         string
	Label        string
	Protocol     Protocol
	BaseURL      string
	DefaultModel string
	Models       []string // курируемый список; пусто — список читается у провайдера
}

// Model — запись справочника цен [[models]].
type Model struct {
	ID            string
	DisplayName   string
	InputUSDPerM  float64
	OutputUSDPerM float64
	Note          string
}

// Config — разобранный providers.toml.
type Config struct {
	providers []Provider
	models    map[string]Model
	defaults  map[string]string
}

// Providers возвращает определения в порядке файла.
func (c Config) Providers() []Provider { return c.providers }

// Provider ищет определение по имени (имя пресета и ключа settings.toml).
func (c Config) Provider(name string) (Provider, bool) {
	for _, p := range c.providers {
		if p.Name == name {
			return p, true
		}
	}
	return Provider{}, false
}

// Model ищет модель в справочнике цен.
func (c Config) Model(id string) (Model, bool) {
	m, ok := c.models[id]
	return m, ok
}

// DefaultModel — заводская модель провайдера; пусто, если ни файл, ни встроенные
// значения её не задают.
func (c Config) DefaultModel(name string) string {
	if p, ok := c.Provider(name); ok && strings.TrimSpace(p.DefaultModel) != "" {
		return p.DefaultModel
	}
	return ""
}

// Load читает providers.toml. Файла нет или провайдеров в нём нет — встроенный
// минимум (gemini-flash, gemini-pro), чтобы станок работал до поставки файла.
func Load(path string) Config {
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultConfig()
	}
	cfg := Parse(string(data))
	if len(cfg.providers) == 0 {
		cfg.providers = defaultProviders()
		for i := range cfg.providers {
			if cfg.providers[i].DefaultModel == "" {
				cfg.providers[i].DefaultModel = cfg.defaults[cfg.providers[i].Name]
			}
		}
	}
	return cfg
}

// Parse разбирает подмножество TOML, нужное реестру: секцию [defaults], массив
// таблиц [[providers]] (name, label, protocol, base_url, default_model, models)
// и массив таблиц [[models]] (id, display_name, input_usd_per_m,
// output_usd_per_m, note). Неизвестные секции и ключи игнорируются, битые
// строки пропускаются.
func Parse(data string) Config {
	cfg := Config{models: map[string]Model{}, defaults: map[string]string{}}
	section := ""
	var provider *Provider
	var model *Model
	flushProvider := func() {
		if provider != nil && provider.Name != "" {
			cfg.providers = append(cfg.providers, *provider)
		}
		provider = nil
	}
	flushModel := func() {
		if model != nil && model.ID != "" {
			cfg.models[model.ID] = *model
		}
		model = nil
	}
	for _, raw := range strings.Split(data, "\n") {
		line := strings.TrimSpace(stripComment(raw))
		if line == "" {
			continue
		}
		switch line {
		case "[defaults]":
			flushProvider()
			flushModel()
			section = "defaults"
			continue
		case "[[providers]]":
			flushProvider()
			flushModel()
			section = "providers"
			provider = &Provider{}
			continue
		case "[[models]]":
			flushProvider()
			flushModel()
			section = "models"
			model = &Model{}
			continue
		}
		if strings.HasPrefix(line, "[") {
			flushProvider()
			flushModel()
			section = ""
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch section {
		case "defaults":
			if name, modelID := unquote(key), unquote(val); name != "" && modelID != "" {
				cfg.defaults[name] = modelID
			}
		case "providers":
			if provider == nil {
				continue
			}
			applyProviderField(provider, key, val)
		case "models":
			if model == nil {
				continue
			}
			applyModelField(model, key, val)
		}
	}
	flushProvider()
	flushModel()

	for i := range cfg.providers {
		p := &cfg.providers[i]
		p.BaseURL = strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
		if strings.TrimSpace(p.DefaultModel) == "" {
			p.DefaultModel = cfg.defaults[p.Name]
		}
		p.Protocol = normalizeProtocol(p)
	}
	return cfg
}

// applyProviderField заполняет определение провайдера.
func applyProviderField(p *Provider, key, val string) {
	switch key {
	case "name":
		p.Name = unquote(val)
	case "label":
		p.Label = unquote(val)
	case "protocol":
		p.Protocol = Protocol(strings.ToLower(unquote(val)))
	case "base_url":
		p.BaseURL = unquote(val)
	case "default_model":
		p.DefaultModel = unquote(val)
	case "models":
		p.Models = parseStringArray(val)
	}
}

// applyModelField заполняет запись справочника цен.
func applyModelField(m *Model, key, val string) {
	switch key {
	case "id":
		m.ID = unquote(val)
	case "display_name":
		m.DisplayName = unquote(val)
	case "note":
		m.Note = unquote(val)
	case "input_usd_per_m":
		if f, err := strconv.ParseFloat(strings.TrimSpace(unquote(val)), 64); err == nil {
			m.InputUSDPerM = f
		}
	case "output_usd_per_m":
		if f, err := strconv.ParseFloat(strings.TrimSpace(unquote(val)), 64); err == nil {
			m.OutputUSDPerM = f
		}
	}
}

// normalizeProtocol: явный протокол остаётся, пустой выводится из имени и адреса.
func normalizeProtocol(p *Provider) Protocol {
	switch p.Protocol {
	case ProtocolLocal, ProtocolGemini, ProtocolOpenAI:
		return p.Protocol
	}
	if p.Name == "local" {
		return ProtocolLocal
	}
	if strings.HasPrefix(p.Name, "gemini-") || strings.Contains(p.BaseURL, "generativelanguage.googleapis.com") {
		return ProtocolGemini
	}
	return ProtocolOpenAI
}

// defaultProviders — встроенный минимум: два Gemini-пресета.
func defaultProviders() []Provider {
	return []Provider{
		{
			Name:         "gemini-flash",
			Protocol:     ProtocolGemini,
			BaseURL:      "https://generativelanguage.googleapis.com",
			DefaultModel: "gemini-3.8-flash",
		},
		{
			Name:         "gemini-pro",
			Protocol:     ProtocolGemini,
			BaseURL:      "https://generativelanguage.googleapis.com",
			DefaultModel: "gemini-3.1-pro-preview",
		},
	}
}

func defaultConfig() Config {
	return Config{providers: defaultProviders(), models: map[string]Model{}, defaults: map[string]string{}}
}

// parseStringArray разбирает TOML-массив строк в одну строку:
// ["a", "b/c"] → ["a", "b/c"]. Одиночная строка тоже принимается.
func parseStringArray(val string) []string {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	if !strings.HasPrefix(val, "[") {
		if s := unquote(val); s != "" {
			return []string{s}
		}
		return nil
	}
	val = strings.TrimSuffix(strings.TrimPrefix(val, "["), "]")
	out := []string{}
	for _, part := range splitArrayItems(val) {
		if s := unquote(part); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// splitArrayItems режет содержимое массива по запятым вне кавычек.
func splitArrayItems(val string) []string {
	var items []string
	start := 0
	inQuote := byte(0)
	for i := 0; i < len(val); i++ {
		c := val[i]
		switch {
		case inQuote != 0:
			if c == '\\' && inQuote == '"' {
				i++
			} else if c == inQuote {
				inQuote = 0
			}
		case c == '"' || c == '\'':
			inQuote = c
		case c == ',':
			items = append(items, val[start:i])
			start = i + 1
		}
	}
	items = append(items, val[start:])
	return items
}

// unquote снимает кавычки TOML-строки; строка без кавычек возвращается как есть.
func unquote(val string) string {
	val = strings.TrimSpace(val)
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		if s, err := strconv.Unquote(val); err == nil {
			return s
		}
	}
	if len(val) >= 2 && val[0] == '\'' && val[len(val)-1] == '\'' {
		return val[1 : len(val)-1]
	}
	return val
}

// stripComment отрезает комментарий, не трогая «#» внутри кавычек.
func stripComment(line string) string {
	inQuote := byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case inQuote != 0:
			if c == '\\' && inQuote == '"' {
				i++
			} else if c == inQuote {
				inQuote = 0
			}
		case c == '"' || c == '\'':
			inQuote = c
		case c == '#':
			return line[:i]
		}
	}
	return line
}
