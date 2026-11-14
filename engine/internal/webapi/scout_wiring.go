package webapi

import (
	"tns65/internal/birth"
	"tns65/internal/models"
	"tns65/internal/paganel"
)

// atlasDeepScout собирает живую глубокую разведку при создании мира (DEC-4,
// B-14.4) из конфигурации экземпляра: config.yaml (секция paganel),
// providers.toml и settings.toml. nil — разведка выключена или провайдер/ключ не
// заданы: рождение от этого не зависит (B-14.3).
func atlasDeepScout() birth.DeepScout {
	scout := paganel.LoadDeepScout(settingsConfigPath, providersTOMLPath, settingsTOMLPath,
		models.CallJournalPath, atlasLogger)
	if scout == nil {
		return nil
	}
	return scout
}
