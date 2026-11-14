//go:build !e2e

package providerroute

import (
	"os"
	"path/filepath"

	"tns65/internal/journal"
	"tns65/internal/providercfg"
	"tns65/internal/turn"
)

// DefaultChatWeaver собирает слот Ткача по маршруту route_chat: маршруты,
// ключи и модели читаются один раз на старте ядра, изменения применяются
// после перезапуска. Пустой route_chat падает на slots.default. Ошибка чтения
// конфигурации — nil и событие slot.wiring_fail в журнал: чат деградирует до
// статической заготовки, но это видно в журнале.
func DefaultChatWeaver(logger *journal.Logger) turn.ProseSlot {
	root := instanceRoot()
	return ChatWeaver(
		filepath.Join(root, "config.yaml"),
		filepath.Join(root, "settings.toml"),
		filepath.Join(root, "providers.toml"),
		logger,
	)
}

// ChatWeaver собирает слот задачи chat по путям конфигурации; пути вынесены
// отдельной функцией для тестов проводки.
func ChatWeaver(configPath, settingsPath, providersPath string, logger *journal.Logger) *RoutedProseSlot {
	cfg, err := LoadSettings(configPath, settingsPath)
	if err != nil {
		logSlotWiringFail(logger, err)
		return nil
	}
	return NewRouter(cfg, providercfg.Load(providersPath), logger).Slot(TaskChat)
}

// instanceRoot — корень работающей папки: каталог с go.mod, а в развёрнутой
// папке без go.mod — рабочий каталог, рядом с которым лежат config.yaml,
// settings.toml и providers.toml.
func instanceRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// logSlotWiringFail пишет в журнал, что слот прозы не собрался из маршрута.
func logSlotWiringFail(logger *journal.Logger, err error) {
	if logger == nil {
		return
	}
	_ = logger.Event("webapi", journal.EventSlotWiring,
		journal.Field{Key: "role", Value: "weaver"},
		journal.Field{Key: "error", Value: err.Error()})
}
