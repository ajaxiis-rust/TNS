package llmparams

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"tns65/internal/models"
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

// TestParseSettingsEmbed — ключи эмбеддера llm:embed_addr/model/timeout.
func TestParseSettingsEmbed(t *testing.T) {
	cfg := ParseSettings("llm:\n  embed_addr: \":5002\"\n  embed_model: \"bge-m3\"\n  embed_timeout: 45s\n")
	if cfg.EmbedAddr != ":5002" || cfg.EmbedModel != "bge-m3" || cfg.EmbedTimeout.Seconds() != 45 {
		t.Fatalf("embed keys = %+v", cfg)
	}

	// Пустой адрес — эмбеддер не подключён; нулевой тайм-аут игнорируется.
	cfg = ParseSettings("llm:\n  embed_addr: \"\"\n  embed_timeout: 0s\n")
	if cfg.EmbedAddr != "" || cfg.EmbedTimeout != 0 {
		t.Fatalf("пустой embed = %+v", cfg)
	}
}

// TestEmbedClient — пустой адрес даёт nil без падения; адрес+модель собирают
// клиент, который передаёт embed_model в тело запроса.
func TestEmbedClient(t *testing.T) {
	if c, err := EmbedClient(Config{}, nil); err != nil || c != nil {
		t.Fatalf("EmbedClient(пусто) = %v, %v; want nil, nil", c, err)
	}

	// Сервер сверяет model в теле запроса и отдаёт вектор нужной размерности.
	// Адрес передаём в форме ":5002" — EmbedClient приводит его к 127.0.0.1.
	var gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("тело запроса: %v", err)
		}
		gotModel = req.Model
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []any{map[string]any{"embedding": make([]float64, models.EmbedDim)}},
		})
	}))
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("адрес сервера: %v", err)
	}

	c, err := EmbedClient(Config{EmbedAddr: ":" + u.Port(), EmbedModel: "bge-m3-custom"}, nil)
	if err != nil {
		t.Fatalf("EmbedClient: %v", err)
	}
	if _, ok := c.(*models.Client); !ok {
		t.Fatalf("EmbedClient тип = %T, want *models.Client", c)
	}
	if _, err := c.Embed(context.Background(), "привет"); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if gotModel != "bge-m3-custom" {
		t.Fatalf("model в теле запроса = %q, want bge-m3-custom", gotModel)
	}
}

// TestEmbedClientTimeout — embed_timeout применяется: зависший эмбеддер
// обрывается ошибкой тайм-аута, а не ждётся бесконечно.
func TestEmbedClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("поздний ответ\n"))
	}))
	defer srv.Close()

	c, err := EmbedClient(Config{EmbedAddr: srv.URL, EmbedTimeout: 20 * time.Millisecond}, nil)
	if err != nil {
		t.Fatalf("EmbedClient: %v", err)
	}
	if _, err := c.Embed(context.Background(), "привет"); err == nil {
		t.Fatal("Embed: ошибки нет, ожидался тайм-аут")
	} else if !strings.Contains(err.Error(), "Timeout") && !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("Embed: err = %v, ожидался тайм-аут", err)
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
