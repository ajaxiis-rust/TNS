package webapi

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"tns65/internal/models"
)

// providersTOMLPath — внешний справочник моделей и цен рядом с settings.toml
// (корень работающей папки). Читается при каждом запросе: правка файла
// применяется без пересборки ядра.
var providersTOMLPath = "providers.toml"

// providerCatalogModel — одна модель в справочнике providers.toml.
type providerCatalogModel struct {
	ID            string
	DisplayName   string
	InputUSDPerM  float64
	OutputUSDPerM float64
	Note          string
}

// providerCatalog — содержимое providers.toml: заводские модели пресетов
// (секция [defaults]) и справочник моделей с ценами ([[models]]). API Gemini
// цен не отдаёт, поэтому цены живут только в этом файле.
type providerCatalog struct {
	defaults map[string]string
	models   map[string]providerCatalogModel
}

// readProviderCatalog читает providers.toml. Файла нет или он пуст — пустой
// справочник: заводская модель берётся из models.ModelFor, цена показывается
// как неизвестная. Битые строки пропускаются, чтобы одна опечатка не убирала
// весь справочник.
func readProviderCatalog(path string) providerCatalog {
	data, err := os.ReadFile(path)
	if err != nil {
		return providerCatalog{defaults: map[string]string{}, models: map[string]providerCatalogModel{}}
	}
	return parseProviderCatalog(string(data))
}

// parseProviderCatalog разбирает подмножество TOML, нужное справочнику:
// секция [defaults] с парами key = "value" и массив таблиц [[models]] с полями
// id, display_name, input_usd_per_m, output_usd_per_m, note. Неизвестные
// секции и ключи игнорируются.
func parseProviderCatalog(data string) providerCatalog {
	cat := providerCatalog{
		defaults: map[string]string{},
		models:   map[string]providerCatalogModel{},
	}
	section := ""
	var current *providerCatalogModel
	flush := func() {
		if current != nil && current.ID != "" {
			cat.models[current.ID] = *current
		}
		current = nil
	}
	for _, raw := range strings.Split(data, "\n") {
		line := strings.TrimSpace(stripTOMLComment(raw))
		if line == "" {
			continue
		}
		switch line {
		case "[defaults]":
			flush()
			section = "defaults"
			continue
		case "[[models]]":
			flush()
			section = "models"
			current = &providerCatalogModel{}
			continue
		}
		if strings.HasPrefix(line, "[") {
			flush()
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
			if name, model, ok := providerDefaultPair(key, val); ok {
				cat.defaults[name] = model
			}
		case "models":
			if current == nil {
				continue
			}
			applyCatalogField(current, key, val)
		}
	}
	flush()
	return cat
}

// providerDefaultPair разбирает строку [defaults]: имя пресета = заводская
// модель. Оба значения — строки в кавычках или без.
func providerDefaultPair(key, val string) (string, string, bool) {
	name := strings.Trim(tomlUnquote(key), " ")
	model := tomlUnquote(val)
	if name == "" || model == "" {
		return "", "", false
	}
	return name, model, true
}

// applyCatalogField заполняет запись [[models]]; неизвестные поля и битые
// числа пропускаются.
func applyCatalogField(m *providerCatalogModel, key, val string) {
	switch key {
	case "id":
		m.ID = tomlUnquote(val)
	case "display_name":
		m.DisplayName = tomlUnquote(val)
	case "note":
		m.Note = tomlUnquote(val)
	case "input_usd_per_m":
		if f, err := strconv.ParseFloat(strings.TrimSpace(tomlUnquote(val)), 64); err == nil {
			m.InputUSDPerM = f
		}
	case "output_usd_per_m":
		if f, err := strconv.ParseFloat(strings.TrimSpace(tomlUnquote(val)), 64); err == nil {
			m.OutputUSDPerM = f
		}
	}
}

// tomlUnquote снимает кавычки TOML-строки; строка без кавычек возвращается как
// есть.
func tomlUnquote(val string) string {
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

// stripTOMLComment отрезает комментарий от строки, не трогая «#» внутри
// кавычек.
func stripTOMLComment(line string) string {
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

// priceForModel возвращает справочную цену модели; nil — модели нет в файле.
func (c providerCatalog) priceForModel(id string) *providerModelPrice {
	m, ok := c.models[id]
	if !ok {
		return nil
	}
	return &providerModelPrice{InputUSDPerM: m.InputUSDPerM, OutputUSDPerM: m.OutputUSDPerM, Note: m.Note}
}

// displayName возвращает имя модели из справочника, если провайдер его не отдал.
func (c providerCatalog) displayName(id string) string {
	return c.models[id].DisplayName
}

// defaultFor — заводская модель провайдера: значение из [defaults], иначе
// встроенная models.ModelFor. Выбор из settings.toml перекрывает оба.
func (c providerCatalog) defaultFor(provider string) string {
	if model := strings.TrimSpace(c.defaults[provider]); model != "" {
		return model
	}
	return models.ModelFor(models.Provider(provider))
}

// priceLabel — короткая подпись цены для карточки провайдера.
func priceLabel(cat providerCatalog, id string) string {
	price := cat.priceForModel(id)
	if price == nil {
		return ""
	}
	if price.InputUSDPerM == 0 && price.OutputUSDPerM == 0 {
		return price.Note
	}
	return fmt.Sprintf("$%.2f / $%.2f за 1M", price.InputUSDPerM, price.OutputUSDPerM)
}

// priceNote — примечание к цене; повторять его в подписи не нужно.
func priceNote(cat providerCatalog, id string) string {
	price := cat.priceForModel(id)
	if price == nil || price.Note == "" || (price.InputUSDPerM == 0 && price.OutputUSDPerM == 0) {
		return ""
	}
	return price.Note
}
