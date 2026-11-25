package llmparams

import (
	"testing"
)

// TestParseSettingsKeys — новые ключи llm: разбираются с сохранением «поле не
// отправлять»: gen_max_tokens 0, gen_top_p 0, gen_top_k 0 и gen_reasoning -1
// дают nil, gen_reasoning 0 — указатель на ноль (выключить).
func TestParseSettingsKeys(t *testing.T) {
	data := "llm:\n" +
		"  gen_timeout: 45s\n" +
		"  gen_max_tokens: 256\n" +
		"  gen_temperature: 0.7\n" +
		"  gen_top_p: 0.9\n" +
		"  gen_top_k: 40\n" +
		"  gen_reasoning: 2048\n"
	cfg := ParseSettings(data)

	if cfg.Timeout.Seconds() != 45 {
		t.Fatalf("gen_timeout = %v, want 45s", cfg.Timeout)
	}
	if cfg.MaxTokens == nil || *cfg.MaxTokens != 256 {
		t.Fatalf("gen_max_tokens = %v, want 256", cfg.MaxTokens)
	}
	if cfg.Temperature == nil || *cfg.Temperature != 0.7 {
		t.Fatalf("gen_temperature = %v, want 0.7", cfg.Temperature)
	}
	if cfg.TopP == nil || *cfg.TopP != 0.9 {
		t.Fatalf("gen_top_p = %v, want 0.9", cfg.TopP)
	}
	if cfg.TopK == nil || *cfg.TopK != 40 {
		t.Fatalf("gen_top_k = %v, want 40", cfg.TopK)
	}
	if cfg.Reasoning == nil || *cfg.Reasoning != 2048 {
		t.Fatalf("gen_reasoning = %v, want 2048", cfg.Reasoning)
	}

	// Нули и -1 — «поля нет»: модель работает на своём дефолте.
	cfg = ParseSettings("llm:\n  gen_max_tokens: 0\n  gen_top_p: 0\n  gen_top_k: 0\n  gen_reasoning: -1\n")
	if cfg.MaxTokens != nil || cfg.TopP != nil || cfg.TopK != nil || cfg.Reasoning != nil {
		t.Fatalf("нулевые ключи должны быть nil: %+v", cfg)
	}

	// gen_reasoning 0 — рассуждения выключены, поле отправляется нулём.
	cfg = ParseSettings("llm:\n  gen_reasoning: 0\n")
	if cfg.Reasoning == nil || *cfg.Reasoning != 0 {
		t.Fatalf("gen_reasoning 0 = %v, want указатель на 0", cfg.Reasoning)
	}
}

// TestResolvePriority — температура роли главнее станковой; прочие параметры
// берутся из config.yaml; пустые оба источника дают пустые params.
func TestResolvePriority(t *testing.T) {
	data := "llm:\n  gen_max_tokens: 512\n  gen_temperature: 0.3\n  gen_top_p: 0.8\n  gen_top_k: 32\n  gen_reasoning: 1024\n"
	cfg := ParseSettings(data)

	params := Resolve(cfg, &RoleConfig{Temperature: 0.9, Prompts: "своя правка", Priority: 7})
	if params.Temperature == nil || *params.Temperature != 0.9 {
		t.Fatalf("temperature роли = %v, want 0.9", params.Temperature)
	}
	if params.MaxTokens == nil || *params.MaxTokens != 512 ||
		params.TopP == nil || *params.TopP != 0.8 ||
		params.TopK == nil || *params.TopK != 32 ||
		params.Reasoning == nil || *params.Reasoning != 1024 {
		t.Fatalf("станковые параметры потеряны: %+v", params)
	}

	// Роль без температуры — станковое значение остаётся.
	params = Resolve(cfg, &RoleConfig{})
	if params.Temperature == nil || *params.Temperature != 0.3 {
		t.Fatalf("temperature = %v, want станковую 0.3", params.Temperature)
	}

	// Нет ни роли, ни ключей — пустые params (дефолт модели).
	if params = Resolve(Config{}, nil); !params.Empty() {
		t.Fatalf("Resolve(пусто) = %+v, want пустые params", params)
	}
}

// TestParamsSnapshot — снимок процесса отдаёт params роли: состав роли из
// world_meta главнее станковых значений config.yaml.
func TestParamsSnapshot(t *testing.T) {
	SetConfig(ParseSettings("llm:\n  gen_temperature: 0.4\n  gen_max_tokens: 128\n"))
	SetRoleConfigs(map[string]RoleConfig{"weaver": {Temperature: 0.85}})
	t.Cleanup(func() {
		SetConfig(Config{})
		SetRoleConfigs(nil)
	})

	params := Params("weaver")
	if params.Temperature == nil || *params.Temperature != 0.85 {
		t.Fatalf("Params(weaver).Temperature = %v, want 0.85", params.Temperature)
	}
	if params.MaxTokens == nil || *params.MaxTokens != 128 {
		t.Fatalf("Params(weaver).MaxTokens = %v, want 128", params.MaxTokens)
	}
	params = Params("editor")
	if params.Temperature == nil || *params.Temperature != 0.4 {
		t.Fatalf("Params(editor).Temperature = %v, want станковую 0.4", params.Temperature)
	}
}

// TestParseRoleConfig — состав роли из world_meta: битый JSON не роняет чтение.
func TestParseRoleConfig(t *testing.T) {
	rc, ok := ParseRoleConfig(`{"temperature":0.6,"prompts":"правка","priority":3}`)
	if !ok || rc.Temperature != 0.6 || rc.Prompts != "правка" || rc.Priority != 3 {
		t.Fatalf("ParseRoleConfig = %+v, ok=%v", rc, ok)
	}
	if _, ok := ParseRoleConfig(`{`); ok {
		t.Fatal("битый JSON разобран как валидный")
	}
	if RoleConfigKey("weaver") != "role.weaver" {
		t.Fatalf("RoleConfigKey = %q", RoleConfigKey("weaver"))
	}
}
