package webapi

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseProviderCatalog — разбор providers.toml: секция [defaults], массив
// таблиц [[models]], комментарии и «#» внутри кавычек; чужие секции и битые
// строки не мешают.
func TestParseProviderCatalog(t *testing.T) {
	cat := parseProviderCatalog(`
# комментарий
[defaults]
"gemini-flash" = "gemini-3.8-flash"   # рядом с ключом
gemini-pro = "gemini-3.1-pro-preview"

[other]
ignored = "да"

[[models]]
id = "gemini-3.8-flash"
display_name = "Gemini 3.8 Flash # не комментарий"
input_usd_per_m = 0.75
output_usd_per_m = 3.75
note = "до 31.12.2026"

[[models]]
id = "broken-model"
input_usd_per_m = не число

[[models]]
display_name = "Без id — запись пропускается"
`)

	if got := cat.defaultFor("gemini-flash"); got != "gemini-3.8-flash" {
		t.Fatalf("defaults gemini-flash = %q", got)
	}
	if got := cat.defaultFor("gemini-pro"); got != "gemini-3.1-pro-preview" {
		t.Fatalf("defaults gemini-pro = %q", got)
	}
	price := cat.priceForModel("gemini-3.8-flash")
	if price == nil || price.InputUSDPerM != 0.75 || price.OutputUSDPerM != 3.75 {
		t.Fatalf("цена gemini-3.8-flash = %+v", price)
	}
	if cat.displayName("gemini-3.8-flash") != "Gemini 3.8 Flash # не комментарий" {
		t.Fatalf("display_name = %q", cat.displayName("gemini-3.8-flash"))
	}
	if _, ok := cat.models["broken-model"]; !ok {
		t.Fatal("запись с битым числом потеряна целиком")
	}
	if cat.models["broken-model"].InputUSDPerM != 0 {
		t.Fatal("битое число записалось")
	}
	if _, ok := cat.models[""]; ok {
		t.Fatal("запись без id попала в справочник")
	}
	if priceLabel(cat, "gemini-3.8-flash") != "$0.75 / $3.75 за 1M" {
		t.Fatalf("priceLabel = %q", priceLabel(cat, "gemini-3.8-flash"))
	}
	if priceNote(cat, "gemini-3.8-flash") != "до 31.12.2026" {
		t.Fatalf("priceNote = %q", priceNote(cat, "gemini-3.8-flash"))
	}
}

// TestProviderCatalogMissingFile — файла нет: справочник пуст, заводская
// модель берётся из встроенной models.ModelFor, цена неизвестна.
func TestProviderCatalogMissingFile(t *testing.T) {
	cat := readProviderCatalog(filepath.Join(t.TempDir(), "absent.toml"))
	if got := cat.defaultFor("gemini-flash"); got != "gemini-3.8-flash" {
		t.Fatalf("defaultFor без файла = %q, want встроенную модель", got)
	}
	if cat.priceForModel("gemini-3.8-flash") != nil {
		t.Fatal("цена без файла должна быть неизвестна")
	}
	if priceLabel(cat, "gemini-3.8-flash") != "" {
		t.Fatal("подпись цены без файла должна быть пустой")
	}
}

// TestProviderCatalogPriceForDisabledModel — у отключённой модели цена 0/0, в
// интерфейсе остаётся только примечание.
func TestProviderCatalogPriceForDisabledModel(t *testing.T) {
	cat := parseProviderCatalog(`
[[models]]
id = "gemini-2.0-flash"
note = "модель отключена Google"
`)
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
[defaults]
"gemini-flash" = "gemini-3.8-flash"

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
[defaults]
"gemini-flash" = "gemini-4-hydrogen"

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

// TestProviderModelCardsCatalogDisplayName — если провайдер не отдал имя модели,
// берётся display_name из providers.toml.
func TestProviderModelCardsCatalogDisplayName(t *testing.T) {
	cat := parseProviderCatalog(`
[[models]]
id = "gemini-3.8-flash"
display_name = "Gemini 3.8 Flash"
input_usd_per_m = 0.75
output_usd_per_m = 3.75
`)
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
