package webapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"tns65/internal/journal"
)

// Инструменты станка вкладки «MCP» (T-08-10, UI-07.10–11): модалка коэффициентов
// математики ходов без LLM и карточка лемматизатора языкового пакета.
// Коэффициенты и переключатель лемматизатора — станковый конфиг: разрез общий
// для всех миров, правка применяется к будущим просчётам, история мира не
// пересчитывается. Писатель: нет записи в базы — коэффициенты живут в config.yaml,
// переключатель — в settings.toml, изменение пишется в журнал станка «было → стало».

const (
	coeffsPath     = "/api/mcp/coeffs"
	lemmatizerPath = "/api/mcp/lemmatizer"

	// coeffsChangedEvent — имя события журнала для правки коэффициентов.
	coeffsChangedEvent = "coeffs.changed"

	// lemmatizerEnabledKey — ключ settings.toml для переключателя «без лемматизатора».
	lemmatizerEnabledKey = "lemmatizer_enabled"
)

// coeffsConfigPath — станковый config.yaml с секциями коэффициентов; подменяется
// в тестах. Общий для всех миров.
var coeffsConfigPath = "config.yaml"

// coeffLogger — журнал станка для записи «было → стало»; подключается при сборке
// сервера, в тестах подменяется журналом во временный файл.
var coeffLogger *journal.Logger

// lemmatizerPackDir — каталог текущего языкового пакета (манифест в нем же);
// подменяется в тестах.
var lemmatizerPackDir = "packs/ru"

// coeffKeys — полные ключи section.key коэффициентов, редактируемых модалом.
var coeffKeys = []string{
	"confirm.dict_threshold",
	"confirm.vector_threshold",
	"confirm.margin_dict",
	"confirm.margin_vector",
	"sediment.dedup_threshold",
	"saw.rise_min",
	"saw.rise_max",
	"saw.peak",
	"saw.fall",
	"saw.flat",
	"time.night_threshold_min",
}

// coeffsResponse — GET /api/mcp/coeffs: все коэффициенты математики ходов без LLM.
type coeffsResponse struct {
	Confirm  confirmCoeffs  `json:"confirm"`
	Sediment sedimentCoeffs `json:"sediment"`
	Saw      sawCoeffs      `json:"saw"`
	Time     timeCoeffs     `json:"time"`
}

type confirmCoeffs struct {
	DictThreshold   float64 `json:"dict_threshold"`
	VectorThreshold float64 `json:"vector_threshold"`
	MarginDict      float64 `json:"margin_dict"`
	MarginVector    float64 `json:"margin_vector"`
}

type sedimentCoeffs struct {
	DedupThreshold float64 `json:"dedup_threshold"`
}

type sawCoeffs struct {
	RiseMin float64 `json:"rise_min"`
	RiseMax float64 `json:"rise_max"`
	Peak    float64 `json:"peak"`
	Fall    float64 `json:"fall"`
	Flat    float64 `json:"flat"`
}

type timeCoeffs struct {
	NightThresholdMin int64 `json:"night_threshold_min"`
}

// coeffsUpdate — PUT /api/mcp/coeffs: указатели различают отсутствующее поле и
// явный ноль.
type coeffsUpdate struct {
	Confirm  *confirmCoeffsUpdate  `json:"confirm"`
	Sediment *sedimentCoeffsUpdate `json:"sediment"`
	Saw      *sawCoeffsUpdate      `json:"saw"`
	Time     *timeCoeffsUpdate     `json:"time"`
}

type confirmCoeffsUpdate struct {
	DictThreshold   *float64 `json:"dict_threshold"`
	VectorThreshold *float64 `json:"vector_threshold"`
	MarginDict      *float64 `json:"margin_dict"`
	MarginVector    *float64 `json:"margin_vector"`
}

type sedimentCoeffsUpdate struct {
	DedupThreshold *float64 `json:"dedup_threshold"`
}

type sawCoeffsUpdate struct {
	RiseMin *float64 `json:"rise_min"`
	RiseMax *float64 `json:"rise_max"`
	Peak    *float64 `json:"peak"`
	Fall    *float64 `json:"fall"`
	Flat    *float64 `json:"flat"`
}

type timeCoeffsUpdate struct {
	NightThresholdMin *int64 `json:"night_threshold_min"`
}

// lemmatizerResponse — GET /api/mcp/lemmatizer: карточка языкового пакета.
type lemmatizerResponse struct {
	PackID     string         `json:"pack_id"`
	Name       string         `json:"name"`
	Lang       string         `json:"lang"`
	Version    string         `json:"version"`
	Enabled    bool           `json:"enabled"`
	Lemmatizer lemmatizerInfo `json:"lemmatizer"`
}

type lemmatizerInfo struct {
	Kind     string `json:"kind"`
	Protocol string `json:"protocol"`
	Binary   string `json:"binary,omitempty"`
	Dict     string `json:"dict,omitempty"`
}

// handleMcpCoeffsGet — GET /api/mcp/coeffs: чтение станковых коэффициентов.
func handleMcpCoeffsGet(w http.ResponseWriter, _ *http.Request) {
	raw, err := readCoeffRaw(coeffsConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "coeffs read failed")
		return
	}
	writeJSON(w, http.StatusOK, parseCoeffsRaw(raw))
}

// handleMcpCoeffsPut — PUT /api/mcp/coeffs: правка станковых коэффициентов.
// Сохранение идёт с предупреждением и записью в журнал «было → стало»; история
// мира не пересчитывается (правка применяется к будущим просчётам).
func handleMcpCoeffsPut(w http.ResponseWriter, r *http.Request) {
	var req coeffsUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad coeffs body")
		return
	}
	updates, err := collectCoeffUpdates(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, err.Error())
		return
	}
	if len(updates) == 0 {
		writeError(w, http.StatusBadRequest, codeBadRequest, "nothing to update")
		return
	}

	before, err := readCoeffRaw(coeffsConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "coeffs read failed")
		return
	}
	if err := updateConfigScalars(coeffsConfigPath, updates); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "coeffs write failed")
		return
	}
	journalCoeffsChange(before, updates)

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"warning": "меняет пересчёт и логику ходов",
	})
}

// collectCoeffUpdates перекладывает заданные поля в section.key и проверяет их.
// Доли и пороги обязаны лежать в [0,1]; порог ночи — целое число от минуты.
func collectCoeffUpdates(req coeffsUpdate) (map[string]string, error) {
	updates := map[string]string{}
	addFloat := func(key string, p *float64) error {
		if p == nil {
			return nil
		}
		if math.IsNaN(*p) || math.IsInf(*p, 0) || *p < 0 || *p > 1 {
			return fmt.Errorf("bad coeffs value: %s must be in [0,1]", key)
		}
		updates[key] = strconv.FormatFloat(*p, 'f', -1, 64)
		return nil
	}

	if req.Confirm != nil {
		fields := []struct {
			key string
			p   *float64
		}{
			{"confirm.dict_threshold", req.Confirm.DictThreshold},
			{"confirm.vector_threshold", req.Confirm.VectorThreshold},
			{"confirm.margin_dict", req.Confirm.MarginDict},
			{"confirm.margin_vector", req.Confirm.MarginVector},
		}
		for _, f := range fields {
			if err := addFloat(f.key, f.p); err != nil {
				return nil, err
			}
		}
	}
	if req.Sediment != nil {
		if err := addFloat("sediment.dedup_threshold", req.Sediment.DedupThreshold); err != nil {
			return nil, err
		}
	}
	if req.Saw != nil {
		fields := []struct {
			key string
			p   *float64
		}{
			{"saw.rise_min", req.Saw.RiseMin},
			{"saw.rise_max", req.Saw.RiseMax},
			{"saw.peak", req.Saw.Peak},
			{"saw.fall", req.Saw.Fall},
			{"saw.flat", req.Saw.Flat},
		}
		for _, f := range fields {
			if err := addFloat(f.key, f.p); err != nil {
				return nil, err
			}
		}
	}
	if req.Time != nil && req.Time.NightThresholdMin != nil {
		if *req.Time.NightThresholdMin < 1 {
			return nil, fmt.Errorf("bad coeffs value: time.night_threshold_min must be >= 1")
		}
		updates["time.night_threshold_min"] = strconv.FormatInt(*req.Time.NightThresholdMin, 10)
	}
	return updates, nil
}

// handleMcpLemmatizerGet — GET /api/mcp/lemmatizer: карточка лемматизатора из
// манифеста языкового пакета и переключатель режима без лемматизатора.
func handleMcpLemmatizerGet(w http.ResponseWriter, _ *http.Request) {
	info, id, name, lang, ver, err := readLemmatizerManifest(lemmatizerPackDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, codeNotFound, "language pack not found")
			return
		}
		writeError(w, http.StatusInternalServerError, codeInternal, "lemmatizer manifest read failed")
		return
	}
	keys := readSettingsKeys(settingsTOMLPath)
	writeJSON(w, http.StatusOK, lemmatizerResponse{
		PackID:     id,
		Name:       name,
		Lang:       lang,
		Version:    ver,
		Enabled:    lemmatizerEnabled(keys),
		Lemmatizer: info,
	})
}

// handleMcpLemmatizerPut — PUT /api/mcp/lemmatizer: переключатель режима без
// лемматизатора (true — лемматизатор включён, false — без лемматизатора).
func handleMcpLemmatizerPut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad lemmatizer body")
		return
	}
	if req.Enabled == nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "enabled required")
		return
	}
	keys := readSettingsKeys(settingsTOMLPath)
	keys[lemmatizerEnabledKey] = strconv.FormatBool(*req.Enabled)
	if err := writeSettingsKeys(settingsTOMLPath, keys); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "lemmatizer settings write failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readCoeffRaw читает известные section.key коэффициенты из config.yaml как
// строки (без числового разбора). Файла нет или ключ отсутствует — без ошибки.
func readCoeffRaw(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(coeffKeys))
	for _, k := range coeffKeys {
		known[k] = true
	}
	out := map[string]string{}
	section := ""
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}
		key, val, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(strings.SplitN(val, "#", 2)[0])
		full := section + "." + key
		if known[full] {
			out[full] = val
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// parseCoeffsRaw строит ответ из сырых строк; пропущенный ключ берёт дефолт,
// совпадающий со значениями config.yaml.
func parseCoeffsRaw(raw map[string]string) coeffsResponse {
	out := defaultCoeffs()
	if v, ok := raw["confirm.dict_threshold"]; ok {
		out.Confirm.DictThreshold = parseCoeffFloat(v, out.Confirm.DictThreshold)
	}
	if v, ok := raw["confirm.vector_threshold"]; ok {
		out.Confirm.VectorThreshold = parseCoeffFloat(v, out.Confirm.VectorThreshold)
	}
	if v, ok := raw["confirm.margin_dict"]; ok {
		out.Confirm.MarginDict = parseCoeffFloat(v, out.Confirm.MarginDict)
	}
	if v, ok := raw["confirm.margin_vector"]; ok {
		out.Confirm.MarginVector = parseCoeffFloat(v, out.Confirm.MarginVector)
	}
	if v, ok := raw["sediment.dedup_threshold"]; ok {
		out.Sediment.DedupThreshold = parseCoeffFloat(v, out.Sediment.DedupThreshold)
	}
	if v, ok := raw["saw.rise_min"]; ok {
		out.Saw.RiseMin = parseCoeffFloat(v, out.Saw.RiseMin)
	}
	if v, ok := raw["saw.rise_max"]; ok {
		out.Saw.RiseMax = parseCoeffFloat(v, out.Saw.RiseMax)
	}
	if v, ok := raw["saw.peak"]; ok {
		out.Saw.Peak = parseCoeffFloat(v, out.Saw.Peak)
	}
	if v, ok := raw["saw.fall"]; ok {
		out.Saw.Fall = parseCoeffFloat(v, out.Saw.Fall)
	}
	if v, ok := raw["saw.flat"]; ok {
		out.Saw.Flat = parseCoeffFloat(v, out.Saw.Flat)
	}
	if v, ok := raw["time.night_threshold_min"]; ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			out.Time.NightThresholdMin = n
		}
	}
	return out
}

func parseCoeffFloat(s string, def float64) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return f
}

func defaultCoeffs() coeffsResponse {
	return coeffsResponse{
		Confirm: confirmCoeffs{
			DictThreshold:   0.6,
			VectorThreshold: 0.75,
			MarginDict:      0.7,
			MarginVector:    0.85,
		},
		Sediment: sedimentCoeffs{DedupThreshold: 0.85},
		Saw:      sawCoeffs{RiseMin: 0.03, RiseMax: 0.08, Peak: 0.8, Fall: 0.35, Flat: 0.3},
		Time:     timeCoeffs{NightThresholdMin: 480},
	}
}

// journalCoeffsChange пишет в журнал станка «было → стало» одним событием со
// списком изменений, отсортированным по ключу. Без журнала — ничего.
func journalCoeffsChange(before map[string]string, updates map[string]string) {
	if coeffLogger == nil {
		return
	}
	keys := make([]string, 0, len(updates))
	for k := range updates {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	changes := make([]string, 0, len(keys))
	for _, k := range keys {
		changes = append(changes, k+": "+before[k]+" → "+updates[k])
	}
	_ = coeffLogger.Event("webapi", coeffsChangedEvent,
		journal.Field{Key: "change", Value: strings.Join(changes, "; ")})
}

// readLemmatizerManifest читает манифест пакета напрямую, включая поля binary и
// dict, которых нет в срезанном langpack.Lemmatizer.
func readLemmatizerManifest(dir string) (lemmatizerInfo, string, string, string, string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return lemmatizerInfo{}, "", "", "", "", err
	}
	var m struct {
		ID         string         `json:"id"`
		Name       string         `json:"name"`
		Lang       string         `json:"lang"`
		Version    string         `json:"version"`
		Lemmatizer lemmatizerInfo `json:"lemmatizer"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return lemmatizerInfo{}, "", "", "", "", err
	}
	return m.Lemmatizer, m.ID, m.Name, m.Lang, m.Version, nil
}

// lemmatizerEnabled — режим без лемматизатора: ключа нет или true — включён,
// "false" — без лемматизатора.
func lemmatizerEnabled(keys map[string]string) bool {
	return keys[lemmatizerEnabledKey] != "false"
}
