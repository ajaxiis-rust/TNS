//go:build !e2e

package webapi

import (
	"path/filepath"

	"tns65/internal/journal"
	"tns65/internal/turn"
)

// DefaultChatWeaver собирает слот Ткача по секции slots конфигурации экземпляра
// (config.yaml в корне работающей папки): local — локальная запаска :5001,
// внешний пресет — клиент провайдера. Ошибка чтения конфигурации — nil и
// событие slot.wiring_fail в журнал: чат деградирует до статической заготовки,
// но это видно в журнале. Ключ внешнего провайдера хранится в settings.toml и
// живым контуром не читается (F-03.4); прод-пресет slots.default: local ключ не
// использует, поэтому передаётся пустая строка.
func DefaultChatWeaver(logger *journal.Logger) turn.ProseSlot {
	slot, err := turn.ProviderSlotFromConfig(filepath.Join(instanceRoot(), "config.yaml"), "")
	if err != nil {
		if logger != nil {
			_ = logger.Event("webapi", journal.EventSlotWiring,
				journal.Field{Key: "role", Value: "weaver"},
				journal.Field{Key: "error", Value: err.Error()})
		}
		return nil
	}
	return slot
}
