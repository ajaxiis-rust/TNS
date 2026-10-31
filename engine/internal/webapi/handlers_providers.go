package webapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tns65/internal/models"
	"tns65/internal/workshop"
)

// Счётчики квот и денег внешних слотов (T-08-13, F-07): индикация владельцу,
// не влияет на ход. Квоты и деньги читаются из журнала вызовов (usage.jsonl) и
// метаданных провайдера; в базы ничего не пишется — писателя у этих счётчиков нет.

// providerUsagePath — журнал вызовов engine/state/usage.jsonl (T-03-18);
// подменяется в тестах.
var providerUsagePath = filepath.Join("state", "usage.jsonl")

// quotaUnavailable — пометка квоты, когда провайдер лимита не отдаёт (шаг 2).
const quotaUnavailable = "недоступно"

// providerLimitSource — источник лимита вызовов провайдера: имя → лимит и
// признак того, что API его отдал. Заводской источник лимита не отдаёт — квота
// помечается «недоступно», значение не выдумывается. Подменяется в тестах.
var providerLimitSource = func(string) (int64, bool) { return 0, false }

// providerCard — карточка внешнего слота раздела «Провайдеры»: ключ, модель,
// остаток лимита вызовов и деньги по тарифу (стоимость из журнала вызовов).
type providerCard struct {
	Name       string `json:"name"`
	KeySet     bool   `json:"key_set"`
	Model      string `json:"model"`
	Quota      string `json:"quota"`       // остаток лимита; «недоступно» без лимита.
	CostMillis int64  `json:"cost_millis"` // деньги по тарифу из журнала вызовов.
}

// usageLine — одна JSON-строка журнала вызовов (роль слота и его счётчики).
type usageLine struct {
	Role  string `json:"role"`
	Calls int64  `json:"calls"`
	Cost  int64  `json:"cost_millis"`
}

// usageTotals — агрегаты журнала вызовов по роли слота.
type usageTotals struct {
	Calls int64
	Cost  int64
}

// readUsageTotals читает журнал вызовов и суммирует calls/cost по роли. Нет
// файла — пустая карта; битые строки пропускаются (журнал — снимок, не источник
// истины).
func readUsageTotals(path string) map[string]usageTotals {
	out := map[string]usageTotals{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var rec usageLine
		if json.Unmarshal([]byte(line), &rec) != nil || rec.Role == "" {
			continue
		}
		t := out[rec.Role]
		t.Calls += rec.Calls
		t.Cost += rec.Cost
		out[rec.Role] = t
	}
	return out
}

// providerCards собирает карточки внешних слотов (все пресеты реестра кроме
// local). Ключ — из settings.toml (readSettingsKeys), модель — models.ModelFor,
// квота — лимит из API минус вызовы журнала, деньги — сумма cost_millis журнала.
func providerCards() []providerCard {
	keys := readSettingsKeys(settingsTOMLPath)
	totals := readUsageTotals(providerUsagePath)
	out := []providerCard{}
	for _, p := range workshop.Presets() {
		if p.Provider == workshop.ProviderLocal {
			continue
		}
		name := string(p.Provider)
		card := providerCard{
			Name:   name,
			KeySet: keys[name] != "",
			Model:  models.ModelFor(models.Provider(name)),
		}
		t := totals[name]
		if limit, ok := providerLimitSource(name); ok {
			card.Quota = strconv.FormatInt(limit-t.Calls, 10)
		} else {
			card.Quota = quotaUnavailable
		}
		card.CostMillis = t.Cost
		out = append(out, card)
	}
	return out
}

// handleProviders — GET /api/providers: карточки внешних слотов с квотами и
// деньгами. Только чтение, без записи в базы.
func handleProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"providers": providerCards()})
}
