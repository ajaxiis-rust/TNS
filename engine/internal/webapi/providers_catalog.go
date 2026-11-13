package webapi

import (
	"fmt"
	"strings"

	"tns65/internal/models"
	"tns65/internal/providercfg"
)

// providersTOMLPath — внешний реестр провайдеров и цен рядом с settings.toml
// (корень работающей папки). Читается при каждом запросе: добавление провайдера,
// правка цены или заводской модели не требуют пересборки ядра.
var providersTOMLPath = "providers.toml"

// readProviderCatalog читает providers.toml; файла нет — встроенный минимум
// (gemini-flash, gemini-pro), чтобы станок работал до поставки файла.
func readProviderCatalog(path string) providercfg.Config {
	return providercfg.Load(path)
}

// priceForModel возвращает справочную цену модели; nil — модели нет в файле.
func priceForModel(cat providercfg.Config, id string) *providerModelPrice {
	m, ok := cat.Model(id)
	if !ok {
		return nil
	}
	return &providerModelPrice{InputUSDPerM: m.InputUSDPerM, OutputUSDPerM: m.OutputUSDPerM, Note: m.Note}
}

// priceLabel — короткая подпись цены для карточки провайдера.
func priceLabel(cat providercfg.Config, id string) string {
	price := priceForModel(cat, id)
	if price == nil {
		return ""
	}
	if price.InputUSDPerM == 0 && price.OutputUSDPerM == 0 {
		return price.Note
	}
	return fmt.Sprintf("$%.2f / $%.2f за 1M", price.InputUSDPerM, price.OutputUSDPerM)
}

// priceNote — примечание к цене; повторять его в подписи не нужно.
func priceNote(cat providercfg.Config, id string) string {
	price := priceForModel(cat, id)
	if price == nil || price.Note == "" || (price.InputUSDPerM == 0 && price.OutputUSDPerM == 0) {
		return ""
	}
	return price.Note
}

// selectedProviderModel — модель провайдера: выбор из settings.toml, иначе
// заводская из providers.toml, иначе встроенная для Gemini-пресетов.
func selectedProviderModel(keys map[string]string, cat providercfg.Config, name string) string {
	if model := strings.TrimSpace(keys[models.ModelSettingsKey(models.Provider(name))]); model != "" {
		return model
	}
	if model := cat.DefaultModel(name); model != "" {
		return model
	}
	if strings.HasPrefix(name, "gemini-") {
		return models.ModelFor(models.Provider(name))
	}
	return ""
}

// modelDisplayName — имя модели из справочника, если провайдер его не отдал.
func modelDisplayName(cat providercfg.Config, id string) string {
	if m, ok := cat.Model(id); ok {
		return m.DisplayName
	}
	return ""
}
