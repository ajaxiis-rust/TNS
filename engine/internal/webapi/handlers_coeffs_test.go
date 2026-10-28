package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/journal"
)

const testCoeffsConfig = `confirm:
  dict_threshold: 0.6
  vector_threshold: 0.75
  margin_dict: 0.7
  margin_vector: 0.85
sediment:
  dedup_threshold: 0.85
saw:
  rise_min: 0.03
  rise_max: 0.08
  peak: 0.8
  fall: 0.35
  flat: 0.3
time:
  night_threshold_min: 480
`

// seedCoeffs подменяет пути коэффициентов, журнал и каталог языкового пакета на
// временные и восстанавливает их после теста.
func seedCoeffs(t *testing.T) (cfgPath string, settingsPath string, journalPath string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath = filepath.Join(dir, "config.yaml")
	settingsPath = filepath.Join(dir, "settings.toml")
	journalPath = filepath.Join(dir, "station.log")

	if err := os.WriteFile(cfgPath, []byte(testCoeffsConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	oldConfig := coeffsConfigPath
	oldSettings := settingsTOMLPath
	oldLogger := coeffLogger
	oldPack := lemmatizerPackDir
	coeffsConfigPath = cfgPath
	settingsTOMLPath = settingsPath
	lemmatizerPackDir = filepath.Join(dir, "pack")
	t.Cleanup(func() {
		coeffsConfigPath = oldConfig
		settingsTOMLPath = oldSettings
		coeffLogger = oldLogger
		lemmatizerPackDir = oldPack
	})
	return cfgPath, settingsPath, journalPath
}

func coeffsRequest(t *testing.T, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	// Запрос идёт через роутер, как в проде: маршруты coeffs/lemmatizer обязаны
	// быть в routeSpecs, иначе тест падает на 404.
	h := NewHandler(Config{Logger: coeffLogger})
	return mcpRequest(t, http.HandlerFunc(h.ServeHTTP), method, path, body, "application/json")
}

// TestCoeffsMath — «Проверка» T-08-10: чтение и правка коэффициентов. Разрез
// станковый и общий для всех миров — эндпоинты не принимают world.
func TestCoeffsMath(t *testing.T) {
	cfgPath, _, _ := seedCoeffs(t)

	rr := coeffsRequest(t, http.MethodGet, coeffsPath, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", coeffsPath, rr.Code, rr.Body.String())
	}
	var out coeffsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.Confirm.DictThreshold != 0.6 || out.Confirm.VectorThreshold != 0.75 {
		t.Fatalf("confirm = %+v, want dict 0.6 vector 0.75", out.Confirm)
	}
	if out.Sediment.DedupThreshold != 0.85 {
		t.Fatalf("dedup = %v, want 0.85", out.Sediment.DedupThreshold)
	}
	if out.Saw.Peak != 0.8 || out.Saw.RiseMin != 0.03 || out.Saw.RiseMax != 0.08 || out.Saw.Fall != 0.35 || out.Saw.Flat != 0.3 {
		t.Fatalf("saw = %+v, want дефолтные значения config.yaml", out.Saw)
	}
	if out.Time.NightThresholdMin != 480 {
		t.Fatalf("night = %d, want 480", out.Time.NightThresholdMin)
	}

	// Правка: словарь и порог ночи. Правка идёт только на будущее — читается
	// обратно и ложится в config.yaml.
	body, _ := json.Marshal(map[string]any{
		"confirm": map[string]float64{"dict_threshold": 0.7},
		"time":    map[string]int64{"night_threshold_min": 600},
	})
	rr = coeffsRequest(t, http.MethodPut, coeffsPath, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT %s: status = %d, want 200; body=%s", coeffsPath, rr.Code, rr.Body.String())
	}
	var saved map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &saved); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if saved["warning"] != "меняет пересчёт и логику ходов" {
		t.Fatalf("warning = %q, want предупреждение", saved["warning"])
	}

	rr = coeffsRequest(t, http.MethodGet, coeffsPath, nil)
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.Confirm.DictThreshold != 0.7 || out.Time.NightThresholdMin != 600 {
		t.Fatalf("после правки confirm.dict=%v night=%d, want 0.7/600", out.Confirm.DictThreshold, out.Time.NightThresholdMin)
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "dict_threshold: 0.7") || !strings.Contains(string(raw), "night_threshold_min: 600") {
		t.Fatalf("config.yaml не переписан: %s", raw)
	}

	// Ошибочные значения — 400.
	bad, _ := json.Marshal(map[string]any{"confirm": map[string]float64{"dict_threshold": 1.5}})
	if rr := coeffsRequest(t, http.MethodPut, coeffsPath, bad); rr.Code != http.StatusBadRequest {
		t.Fatalf("dict_threshold=1.5: status = %d, want 400", rr.Code)
	}
	bad, _ = json.Marshal(map[string]any{"time": map[string]int64{"night_threshold_min": 0}})
	if rr := coeffsRequest(t, http.MethodPut, coeffsPath, bad); rr.Code != http.StatusBadRequest {
		t.Fatalf("night_threshold_min=0: status = %d, want 400", rr.Code)
	}
	if rr := coeffsRequest(t, http.MethodPut, coeffsPath, []byte("{}")); rr.Code != http.StatusBadRequest {
		t.Fatalf("пустая правка: status = %d, want 400", rr.Code)
	}
}

// TestCoeffsJournal — «Проверка» T-08-10: предупреждение и запись «было → стало»
// в журнал станка при сохранении коэффициентов.
func TestCoeffsJournal(t *testing.T) {
	_, _, journalPath := seedCoeffs(t)

	logger, err := journal.New(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	coeffLogger = logger

	body, _ := json.Marshal(map[string]any{
		"confirm": map[string]float64{"dict_threshold": 0.7},
	})
	rr := coeffsRequest(t, http.MethodPut, coeffsPath, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT %s: status = %d, want 200; body=%s", coeffsPath, rr.Code, rr.Body.String())
	}
	var saved map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &saved); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if saved["warning"] != "меняет пересчёт и логику ходов" {
		t.Fatalf("warning = %q, want предупреждение", saved["warning"])
	}

	raw, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "coeffs.changed") {
		t.Fatalf("журнал не содержит событие coeffs.changed: %s", raw)
	}
	if !strings.Contains(string(raw), "confirm.dict_threshold: 0.6 → 0.7") {
		t.Fatalf("журнал не содержит «было → стало»: %s", raw)
	}
}

// TestLemmatizerCard — «Проверка» T-08-10: чтение параметров лемматизатора из
// манифеста языкового пакета и переключатель режима без лемматизатора.
func TestLemmatizerCard(t *testing.T) {
	seedCoeffs(t)

	manifest := `{
  "id": "ru",
  "name": "Русский",
  "lang": "ru",
  "version": "1.0.0",
  "lemmatizer": {
    "kind": "gomorphy",
    "protocol": "stdio-json",
    "binary": "tns65-lemmaru",
    "dict": "packs/ru/lemmatizer"
  },
  "files": {}
}`
	if err := os.MkdirAll(lemmatizerPackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lemmatizerPackDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	// Режим без лемматизатора выключен (ключ "false").
	if err := os.WriteFile(settingsTOMLPath, []byte("lemmatizer_enabled = \"false\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rr := coeffsRequest(t, http.MethodGet, lemmatizerPath, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", lemmatizerPath, rr.Code, rr.Body.String())
	}
	var out lemmatizerResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.PackID != "ru" || out.Lang != "ru" || out.Version != "1.0.0" {
		t.Fatalf("пакет = %+v, want ru/ru/1.0.0", out)
	}
	if out.Enabled {
		t.Fatalf("enabled = true, want false (без лемматизатора)")
	}
	if out.Lemmatizer.Kind != "gomorphy" || out.Lemmatizer.Protocol != "stdio-json" ||
		out.Lemmatizer.Binary != "tns65-lemmaru" || out.Lemmatizer.Dict != "packs/ru/lemmatizer" {
		t.Fatalf("лемматизатор = %+v, want gomorphy/stdio-json/tns65-lemmaru/packs/ru/lemmatizer", out.Lemmatizer)
	}

	// Переключатель включается обратно и сохраняется в settings.toml.
	body, _ := json.Marshal(map[string]bool{"enabled": true})
	rr = coeffsRequest(t, http.MethodPut, lemmatizerPath, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT %s: status = %d, want 200; body=%s", lemmatizerPath, rr.Code, rr.Body.String())
	}
	raw, err := os.ReadFile(settingsTOMLPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "lemmatizer_enabled = \"true\"") {
		t.Fatalf("settings.toml не сохранил переключатель: %s", raw)
	}
}
