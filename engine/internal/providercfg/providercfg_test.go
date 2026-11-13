package providercfg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseProviders(t *testing.T) {
	cfg := Parse(`
# реестр
[defaults]
"legacy-model" = "gemini-3.8-flash"

[[providers]]
name = "gemini-flash"
protocol = "gemini"
base_url = "https://generativelanguage.googleapis.com"
default_model = "gemini-3.8-flash"

[[providers]]
name = "deepseek"
label = "DeepSeek"
protocol = "openai"
base_url = "https://api.deepseek.com/v1/"
models = ["deepseek-chat", "deepseek-reasoner"]

[[providers]]
name = "legacy-model"
base_url = "https://api.example.com/v1"   # протокол выведется как openai

[[models]]
id = "deepseek-chat"
display_name = "DeepSeek Chat"
input_usd_per_m = 0.28
output_usd_per_m = 0.42
note = "cache miss / cache hit"

[[models]]
id = "gemini-3.8-flash"
input_usd_per_m = 0.75
output_usd_per_m = 3.75
`)
	if len(cfg.Providers()) != 3 {
		t.Fatalf("провайдеров = %d, want 3: %+v", len(cfg.Providers()), cfg.Providers())
	}
	gem, ok := cfg.Provider("gemini-flash")
	if !ok || gem.Protocol != ProtocolGemini || gem.DefaultModel != "gemini-3.8-flash" {
		t.Fatalf("gemini-flash = %+v", gem)
	}
	ds, ok := cfg.Provider("deepseek")
	if !ok {
		t.Fatal("deepseek не найден")
	}
	if ds.Protocol != ProtocolOpenAI || ds.Label != "DeepSeek" {
		t.Fatalf("deepseek = %+v", ds)
	}
	if ds.BaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("base_url = %q, want без хвостового слэша", ds.BaseURL)
	}
	if len(ds.Models) != 2 || ds.Models[0] != "deepseek-chat" || ds.Models[1] != "deepseek-reasoner" {
		t.Fatalf("curated models = %+v", ds.Models)
	}
	legacy, ok := cfg.Provider("legacy-model")
	if !ok || legacy.Protocol != ProtocolOpenAI {
		t.Fatalf("legacy-model = %+v", legacy)
	}
	if legacy.DefaultModel != "gemini-3.8-flash" {
		t.Fatalf("default_model из [defaults] = %q", legacy.DefaultModel)
	}
	m, ok := cfg.Model("deepseek-chat")
	if !ok || m.InputUSDPerM != 0.28 || m.OutputUSDPerM != 0.42 || m.DisplayName != "DeepSeek Chat" {
		t.Fatalf("цена deepseek-chat = %+v", m)
	}
	if m.Note != "cache miss / cache hit" {
		t.Fatalf("note = %q", m.Note)
	}
}

func TestParseProtocolInference(t *testing.T) {
	cfg := Parse(`
[[providers]]
name = "gemini-pro"
base_url = "https://generativelanguage.googleapis.com"

[[providers]]
name = "groq"
base_url = "https://api.groq.com/openai/v1"

[[providers]]
name = "local"
`)
	if p, _ := cfg.Provider("gemini-pro"); p.Protocol != ProtocolGemini {
		t.Fatalf("gemini-pro protocol = %q", p.Protocol)
	}
	if p, _ := cfg.Provider("groq"); p.Protocol != ProtocolOpenAI {
		t.Fatalf("groq protocol = %q", p.Protocol)
	}
	if p, _ := cfg.Provider("local"); p.Protocol != ProtocolLocal {
		t.Fatalf("local protocol = %q", p.Protocol)
	}
}

func TestParseSkipsBadLines(t *testing.T) {
	cfg := Parse(`
[[providers]]
name = "deepseek"
input_usd_per_m = не число
unknown_field = "ignored"

[[models]]
display_name = "без id"

[[models]]
id = "deepseek-chat"
input_usd_per_m = 0.28
`)
	if len(cfg.Providers()) != 1 {
		t.Fatalf("провайдеров = %d, want 1", len(cfg.Providers()))
	}
	if _, ok := cfg.Model(""); ok {
		t.Fatal("модель без id попала в справочник")
	}
	if m, ok := cfg.Model("deepseek-chat"); !ok || m.InputUSDPerM != 0.28 {
		t.Fatalf("deepseek-chat = %+v", m)
	}
}

func TestLoadMissingFileUsesBuiltins(t *testing.T) {
	cfg := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if len(cfg.Providers()) != 2 {
		t.Fatalf("встроенных провайдеров = %d, want 2", len(cfg.Providers()))
	}
	if cfg.DefaultModel("gemini-flash") != "gemini-3.8-flash" {
		t.Fatalf("default flash = %q", cfg.DefaultModel("gemini-flash"))
	}
	if _, ok := cfg.Model("gemini-3.8-flash"); ok {
		t.Fatal("без файла цен быть не должно")
	}
	if got := cfg.DefaultModel("unknown"); got != "" {
		t.Fatalf("default unknown = %q, want пусто", got)
	}
}

func TestLoadFileWithoutProvidersUsesBuiltinsAndInFilePrices(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.toml")
	if err := os.WriteFile(path, []byte(`
[[models]]
id = "gemini-3.8-flash"
input_usd_per_m = 1.25
output_usd_per_m = 6.00
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Load(path)
	if len(cfg.Providers()) != 2 {
		t.Fatalf("провайдеров = %d, want встроенные 2", len(cfg.Providers()))
	}
	if m, ok := cfg.Model("gemini-3.8-flash"); !ok || m.InputUSDPerM != 1.25 {
		t.Fatalf("цена из файла = %+v", m)
	}
}

func TestParseStringArray(t *testing.T) {
	cases := map[string][]string{
		`["a", "b/c"]`:       {"a", "b/c"},
		`["a,b"]`:            {"a,b"},
		`"solo"`:             {"solo"},
		`[]`:                 nil,
		`["a", ]`:            {"a"},
		`["a#b", "c"]`:       {"a#b", "c"},
		`["a\"b", "c,d"]`:    {`a"b`, "c,d"},
		`["x", "y", "z"]`:    {"x", "y", "z"},
		`["@cf/meta/llama"]`: {"@cf/meta/llama"},
	}
	for in, want := range cases {
		got := parseStringArray(in)
		if len(got) != len(want) {
			t.Fatalf("%s → %+v, want %+v", in, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s → %+v, want %+v", in, got, want)
			}
		}
	}
}
