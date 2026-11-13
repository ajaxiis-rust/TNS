package turn

import (
	"testing"

	"tns65/internal/models"
	"tns65/internal/providercfg"
)

// TestProviderSlotProtocols — слот Ткача собирается по протоколу из
// providers.toml: local — запаска, gemini — Gemini-клиент, openai —
// OpenAI-совместимый клиент.
func TestProviderSlotProtocols(t *testing.T) {
	local, err := ProviderSlot(providercfg.Provider{Name: "local", Protocol: providercfg.ProtocolLocal}, "", "")
	if err != nil {
		t.Fatalf("local: %v", err)
	}
	if _, ok := local.(*models.Weaver); !ok {
		t.Fatalf("local слот = %T, want *models.Weaver", local)
	}

	openai, err := ProviderSlot(providercfg.Provider{
		Name:         "deepseek",
		Protocol:     providercfg.ProtocolOpenAI,
		BaseURL:      "https://api.deepseek.com/v1",
		DefaultModel: "deepseek-chat",
	}, "sk-test", "")
	if err != nil {
		t.Fatalf("openai: %v", err)
	}
	if _, ok := openai.(*models.OpenAIClient); !ok {
		t.Fatalf("openai слот = %T, want *models.OpenAIClient", openai)
	}

	gemini, err := ProviderSlot(providercfg.Provider{
		Name:         "gemini-flash",
		Protocol:     providercfg.ProtocolGemini,
		BaseURL:      "https://generativelanguage.googleapis.com",
		DefaultModel: "gemini-3.8-flash",
	}, "sk-test", "")
	if err != nil {
		t.Fatalf("gemini: %v", err)
	}
	if _, ok := gemini.(*models.ProviderClient); !ok {
		t.Fatalf("gemini слот = %T, want *models.ProviderClient", gemini)
	}

	if _, err := ProviderSlot(providercfg.Provider{Name: "bad", Protocol: "ftp"}, "", ""); err == nil {
		t.Fatal("неизвестный протокол принят")
	}
}
