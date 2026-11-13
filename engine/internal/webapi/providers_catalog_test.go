package webapi

import (
	"os"
	"path/filepath"
	"testing"
)

// TestProviderCatalogMissingFile — файла нет: встроенный минимум провайдеров,
// заводская модель на месте, цена неизвестна.
func TestProviderCatalogMissingFile(t *testing.T) {
	cat := readProviderCatalog(filepath.Join(t.TempDir(), "absent.toml"))
	if _, ok := cat.Provider("gemini-flash"); !ok {
		t.Fatal("встроенный gemini-flash потерян")
	}
	if got := cat.DefaultModel("gemini-flash"); got != "gemini-3.8-flash" {
		t.Fatalf("DefaultModel без файла = %q", got)
	}
	if priceLabel(cat, "gemini-3.8-flash") != "" {
		t.Fatal("подпись цены без файла должна быть пустой")
	}
}

// TestProviderCatalogPriceForDisabledModel — у отключённой модели цена 0/0, в
// интерфейсе остаётся только примечание.
func TestProviderCatalogPriceForDisabledModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.toml")
	if err := os.WriteFile(path, []byte(`
[[models]]
id = "gemini-2.0-flash"
note = "модель отключена Google"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cat := readProviderCatalog(path)
	if got := priceLabel(cat, "gemini-2.0-flash"); got != "модель отключена Google" {
		t.Fatalf("priceLabel отключённой модели = %q", got)
	}
	if got := priceNote(cat, "gemini-2.0-flash"); got != "" {
		t.Fatalf("priceNote отключённой модели = %q, want пусто", got)
	}
}

// TestProviderCardsReadProvidersFileLive — карточки читают providers.toml при
// каждом вызове: правка цены и заводской модели применяется без пересборки и
// без перезапуска; выбор из settings.toml главнее файла.
func TestProviderCardsReadProvidersFileLive(t *testing.T) {
	seedProviderCounters(t)

	if err := os.WriteFile(providersTOMLPath, []byte(`
[[providers]]
name = "gemini-flash"
protocol = "gemini"
base_url = "https://generativelanguage.googleapis.com"
default_model = "gemini-3.8-flash"

[[models]]
id = "gemini-3.8-flash"
input_usd_per_m = 0.75
output_usd_per_m = 3.75
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cards := providerCardsByName(providerCards())
	if got := cards["gemini-flash"].Price; got != "$0.75 / $3.75 за 1M" {
		t.Fatalf("цена до правки = %q", got)
	}

	// Правка файла — следующая карточка уже с новой ценой и моделью.
	if err := os.WriteFile(providersTOMLPath, []byte(`
[[providers]]
name = "gemini-flash"
default_model = "gemini-4-hydrogen"

[[models]]
id = "gemini-4-hydrogen"
input_usd_per_m = 1.50
output_usd_per_m = 7.50
note = "есть бесплатный уровень"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cards = providerCardsByName(providerCards())
	if got := cards["gemini-flash"].Model; got != "gemini-4-hydrogen" {
		t.Fatalf("заводская модель после правки = %q", got)
	}
	if got := cards["gemini-flash"].Price; got != "$1.50 / $7.50 за 1M" {
		t.Fatalf("цена после правки = %q", got)
	}
	if got := cards["gemini-flash"].PriceNote; got != "есть бесплатный уровень" {
		t.Fatalf("примечание после правки = %q", got)
	}

	// Выбор в settings.toml перекрывает providers.toml.
	if err := os.WriteFile(settingsTOMLPath,
		[]byte("gemini-flash = \"sk-test\"\ngemini-flash_model = \"gemini-3.8-flash\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cards = providerCardsByName(providerCards())
	if got := cards["gemini-flash"].Model; got != "gemini-3.8-flash" {
		t.Fatalf("выбор из settings.toml = %q", got)
	}
	if got := cards["gemini-flash"].Price; got != "" {
		t.Fatalf("цена модели, которой нет в файле, = %q, want пусто", got)
	}
}

// TestProviderCardsDynamicProvider — провайдер из файла без правок кода
// появляется в карточках.
func TestProviderCardsDynamicProvider(t *testing.T) {
	seedProviderCounters(t)
	if err := os.WriteFile(providersTOMLPath, []byte(`
[[providers]]
name = "gemini-flash"
protocol = "gemini"
default_model = "gemini-3.8-flash"

[[providers]]
name = "deepseek"
label = "DeepSeek"
protocol = "openai"
base_url = "https://api.deepseek.com/v1"
default_model = "deepseek-chat"

[[models]]
id = "deepseek-chat"
input_usd_per_m = 0.28
output_usd_per_m = 0.42
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsTOMLPath, []byte("deepseek = \"sk-ds\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cards := providerCardsByName(providerCards())
	ds, ok := cards["deepseek"]
	if !ok {
		t.Fatalf("динамический провайдер не появился: %+v", cards)
	}
	if ds.Model != "deepseek-chat" || ds.Price != "$0.28 / $0.42 за 1M" || !ds.KeySet {
		t.Fatalf("карточка deepseek = %+v", ds)
	}
}

// TestProviderModelCardsCatalogDisplayName — если провайдер не отдал имя модели,
// берётся display_name из providers.toml.
func TestProviderModelCardsCatalogDisplayName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.toml")
	if err := os.WriteFile(path, []byte(`
[[models]]
id = "gemini-3.8-flash"
display_name = "Gemini 3.8 Flash"
input_usd_per_m = 0.75
output_usd_per_m = 3.75
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cat := readProviderCatalog(path)
	cards := providerModelCards([]geminiModelInfo{{
		Name:                       "models/gemini-3.8-flash",
		SupportedGenerationMethods: []string{"generateContent"},
	}}, cat)
	if len(cards) != 1 || cards[0].DisplayName != "Gemini 3.8 Flash" {
		t.Fatalf("карточки = %+v", cards)
	}
	if cards[0].Price == nil || cards[0].Price.InputUSDPerM != 0.75 {
		t.Fatalf("цена = %+v", cards[0].Price)
	}
}
