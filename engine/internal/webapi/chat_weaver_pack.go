//go:build !e2e

package webapi

import (
	"path/filepath"
	"strings"

	"tns65/internal/journal"
	"tns65/internal/models"
	"tns65/internal/turn"
)

// DefaultChatWeaver собирает слот Ткача по секции slots конфигурации экземпляра
// (config.yaml в корне работающей папки): local — локальная запаска :5001,
// внешний пресет — клиент провайдера. Ключ и выбранная модель читаются из
// settings.toml один раз при сборке слота (на старте ядра), в памяти; заводская
// модель при пустом выборе — из providers.toml. Сам ход эти файлы не
// перечитывает. Ошибка чтения конфигурации — nil и событие slot.wiring_fail в
// журнал: чат деградирует до статической заготовки, но это видно в журнале.
func DefaultChatWeaver(logger *journal.Logger) turn.ProseSlot {
	cfg, err := models.LoadSlotsConfig(filepath.Join(instanceRoot(), "config.yaml"))
	if err != nil {
		logSlotWiringFail(logger, err)
		return nil
	}
	keys := readSettingsKeys(settingsTOMLPath)
	provider := string(cfg.Default)
	model := keys[models.ModelSettingsKey(cfg.Default)]
	if strings.TrimSpace(model) == "" {
		model = readProviderCatalog(providersTOMLPath).defaultFor(provider)
	}
	slot, err := turn.ProviderSlot(cfg, keys[provider], model)
	if err != nil {
		logSlotWiringFail(logger, err)
		return nil
	}
	return slot
}

func logSlotWiringFail(logger *journal.Logger, err error) {
	if logger == nil {
		return
	}
	_ = logger.Event("webapi", journal.EventSlotWiring,
		journal.Field{Key: "role", Value: "weaver"},
		journal.Field{Key: "error", Value: err.Error()})
}
