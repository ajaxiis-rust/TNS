package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"tns65/internal/db"
)

// seedProviderCounters подменяет журнал вызовов, settings.toml и источник
// лимитов на временные значения и восстанавливает их после теста.
func seedProviderCounters(t *testing.T) (usagePath string) {
	t.Helper()
	dir := t.TempDir()
	usagePath = filepath.Join(dir, "usage.jsonl")

	oldUsage := providerUsagePath
	oldTOML := settingsTOMLPath
	oldLimit := providerLimitSource
	providerUsagePath = usagePath
	settingsTOMLPath = filepath.Join(dir, "settings.toml")
	providerLimitSource = func(string) (int64, bool) { return 0, false }
	t.Cleanup(func() {
		providerUsagePath = oldUsage
		settingsTOMLPath = oldTOML
		providerLimitSource = oldLimit
	})
	return usagePath
}

func writeProviderUsage(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func providerCardsByName(cards []providerCard) map[string]providerCard {
	out := map[string]providerCard{}
	for _, c := range cards {
		out[c.Name] = c
	}
	return out
}

// TestProviderQuota — «Проверка» T-08-13: чтение лимитов вызовов и остатка
// квоты; провайдер без лимита помечается «недоступно», значение не выдумывается.
func TestProviderQuota(t *testing.T) {
	usagePath := seedProviderCounters(t)

	// Ключ введён только для gemini-flash; лимиты отдаёт API.
	if err := os.WriteFile(settingsTOMLPath, []byte("gemini-flash = \"sk-test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	providerLimitSource = func(name string) (int64, bool) {
		switch name {
		case "gemini-flash":
			return 100, true
		case "gemini-pro":
			return 50, true
		}
		return 0, false
	}
	writeProviderUsage(t, usagePath,
		`{"role":"gemini-flash","calls":40,"tokens":120,"cost_millis":0}`+"\n"+
			`{"role":"gemini-pro","calls":10,"tokens":0,"cost_millis":0}`+"\n")

	cards := providerCardsByName(providerCards())
	if len(cards) != 2 {
		t.Fatalf("карточек = %d, want 2 (внешние слоты без local)", len(cards))
	}
	if got := cards["gemini-flash"].Quota; got != "60" {
		t.Fatalf("gemini-flash quota = %q, want 60 (лимит 100 − вызовов 40)", got)
	}
	if got := cards["gemini-pro"].Quota; got != "40" {
		t.Fatalf("gemini-pro quota = %q, want 40 (лимит 50 − вызовов 10)", got)
	}
	if !cards["gemini-flash"].KeySet {
		t.Fatalf("gemini-flash key_set = false, want true (ключ в settings.toml)")
	}
	if cards["gemini-pro"].KeySet {
		t.Fatalf("gemini-pro key_set = true, want false (ключа нет)")
	}
	if got := cards["gemini-flash"].Model; got != "gemini-2.0-flash" {
		t.Fatalf("gemini-flash model = %q, want gemini-2.0-flash", got)
	}
	if got := cards["gemini-pro"].Model; got != "gemini-2.5-pro" {
		t.Fatalf("gemini-pro model = %q, want gemini-2.5-pro", got)
	}

	// Провайдер лимита не отдаёт — квота «недоступно», число не выдумывается.
	providerLimitSource = func(string) (int64, bool) { return 0, false }
	cards = providerCardsByName(providerCards())
	if got := cards["gemini-flash"].Quota; got != quotaUnavailable {
		t.Fatalf("gemini-flash quota без лимита = %q, want %q", got, quotaUnavailable)
	}

	// Хендлер отдаёт те же карточки JSON-ответом.
	rr := httptest.NewRecorder()
	handleProviders(rr, httptest.NewRequest(http.MethodGet, "/api/providers", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/providers: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Providers []providerCard `json:"providers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(resp.Providers) != 2 {
		t.Fatalf("providers в ответе = %d, want 2", len(resp.Providers))
	}
}

// TestProviderCost — «Проверка» T-08-13: чтение денег по тарифу из журнала
// вызовов; без журнала — ноль, значение не выдумывается; в базы не пишется.
func TestProviderCost(t *testing.T) {
	usagePath := seedProviderCounters(t)

	writeProviderUsage(t, usagePath,
		`{"role":"gemini-flash","calls":1,"tokens":0,"cost_millis":250}`+"\n"+
			`{"role":"gemini-flash","calls":1,"tokens":0,"cost_millis":750}`+"\n"+
			`{"role":"gemini-pro","calls":1,"tokens":0,"cost_millis":100}`+"\n")

	cards := providerCardsByName(providerCards())
	if got := cards["gemini-flash"].CostMillis; got != 1000 {
		t.Fatalf("gemini-flash cost = %d, want 1000 (250+750)", got)
	}
	if got := cards["gemini-pro"].CostMillis; got != 100 {
		t.Fatalf("gemini-pro cost = %d, want 100", got)
	}

	// Журнала нет — денег ноль, значение не выдумывается.
	missing := filepath.Join(t.TempDir(), "no", "usage.jsonl")
	oldUsage := providerUsagePath
	providerUsagePath = missing
	t.Cleanup(func() { providerUsagePath = oldUsage })
	cards = providerCardsByName(providerCards())
	if got := cards["gemini-flash"].CostMillis; got != 0 {
		t.Fatalf("cost без журнала = %d, want 0", got)
	}

	// «Отсутствие записи в базы»: счётчики — только чтение файлов, world_db не
	// трогается.
	oldWorlds := db.WorldsRoot
	db.WorldsRoot = filepath.Join(t.TempDir(), "world_db")
	t.Cleanup(func() { db.WorldsRoot = oldWorlds })
	_ = providerCards()
	if entries, err := os.ReadDir(db.WorldsRoot); err == nil && len(entries) != 0 {
		t.Fatalf("providerCards создал файлы в world_db: %v", entries)
	}
}
