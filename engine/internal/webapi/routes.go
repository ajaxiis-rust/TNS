package webapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Коды ошибок — единый контракт ошибок (T-08-02, шаг 4): HTTP-код плюс тело
// {"error":{"code":…,"message":…}}.
const (
	codeNotFound         = "not_found"
	codeMethodNotAllowed = "method_not_allowed"
	codeBadRequest       = "bad_request"
	codeConflict         = "conflict"
	codeInternal         = "internal"
)

// errorBody — тело ошибки по единому контракту.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Fields  []errorField `json:"fields,omitempty"`
}

// errorField — поле формы и причина отказа (анкета рождения). UI подсвечивает
// поле и показывает причину рядом с ним.
type errorField struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// writeJSON пишет JSON-ответ с Content-Type.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError пишет ошибку по единому контракту (T-08-02, шаг 4).
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// writeFieldError пишет ошибку полей: сообщение плюс перечень field/reason,
// чтобы форма могла подсветить конкретные поля (анкета рождения).
func writeFieldError(w http.ResponseWriter, status int, code, message string, fields []errorField) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message, Fields: fields}})
}

// routeSpec — строка таблицы маршрутов: метод, путь и обработчик.
type routeSpec struct {
	method string
	path   string
	handle http.HandlerFunc
}

// routeSpecs — единственный источник маршрутов роутера. ROUTES.md и
// TestRouteTable сверяются с этим списком дословно (T-08-02, шаг 3): задача P7,
// добавляющая маршрут, правит только этот список, ROUTES.md и тест сверки.
func routeSpecs(hub *Hub) []routeSpec {
	mcpHub = hub
	monitorHub = hub
	chatHub = hub
	specs := []routeSpec{
		{method: http.MethodGet, path: "/api/health", handle: handleHealth},
		{method: http.MethodGet, path: "/", handle: serveIndex},
		{method: http.MethodGet, path: "/static/", handle: staticServe},
		{method: http.MethodGet, path: "/ws", handle: hub.serveHTTP},
		{method: http.MethodGet, path: "/api/world", handle: handleWorldReadonly},
		{method: http.MethodPut, path: "/api/world/tunables", handle: handleWorldTunables},
		{method: http.MethodPost, path: "/api/turn", handle: handleChatTurn},
		{method: http.MethodGet, path: "/api/state", handle: handleChatState},
		{method: http.MethodPost, path: "/api/scene/", handle: handleChatSignal},
		{method: http.MethodGet, path: "/atlas.html", handle: handleAtlasScreen},
		{method: http.MethodGet, path: "/history.html", handle: handleHistoryScreen},
		{method: http.MethodGet, path: "/mcp.html", handle: handleMcpScreen},
		{method: http.MethodGet, path: "/api/atlas/worlds", handle: handleAtlasList},
		{method: http.MethodGet, path: "/api/atlas/birth", handle: handleAtlasBirthOptions},
		{method: http.MethodPost, path: "/api/atlas/worlds/birth", handle: handleAtlasBirth},
		{method: http.MethodPost, path: "/api/atlas/worlds/active", handle: handleAtlasSwitch},
		{method: http.MethodPost, path: "/api/atlas/worlds/replay", handle: handleAtlasReplay},
		{method: http.MethodDelete, path: "/api/atlas/worlds/delete", handle: handleAtlasDelete},
		{method: http.MethodPost, path: "/api/atlas/worlds/forge", handle: handleAtlasForge},
		{method: http.MethodGet, path: "/api/mcp/dash", handle: handleMcpDash},
		{method: http.MethodGet, path: "/api/mcp/sources", handle: handleMcpSources},
		{method: http.MethodPost, path: "/api/mcp/import", handle: handleMcpImport},
		{method: http.MethodPost, path: "/api/mcp/forge", handle: handleMcpForge},
		{method: http.MethodDelete, path: "/api/mcp/source/", handle: handleMcpSourceDelete},
		{method: http.MethodGet, path: "/api/mcp/rings", handle: handleMcpRings},
		{method: http.MethodGet, path: "/api/mcp/coeffs", handle: handleMcpCoeffsGet},
		{method: http.MethodPut, path: "/api/mcp/coeffs", handle: handleMcpCoeffsPut},
		{method: http.MethodGet, path: "/api/mcp/lemmatizer", handle: handleMcpLemmatizerGet},
		{method: http.MethodPut, path: "/api/mcp/lemmatizer", handle: handleMcpLemmatizerPut},
		{method: http.MethodGet, path: "/settings.html", handle: handleSettingsScreen},
		{method: http.MethodGet, path: "/wizard.html", handle: handleWizardScreen},
		{method: http.MethodGet, path: "/api/settings", handle: handleSettingsGet},
		{method: http.MethodPut, path: "/api/settings/bindings", handle: handleSettingsBindings},
		{method: http.MethodPut, path: "/api/settings/config", handle: handleSettingsConfig},
		{method: http.MethodPut, path: "/api/settings/keys", handle: handleSettingsKeys},
		{method: http.MethodPut, path: "/api/settings/channel", handle: handleSettingsChannel},
		{method: http.MethodPost, path: "/api/settings/models/install", handle: handleSettingsModelInstall},
		{method: http.MethodGet, path: "/api/settings/models/status", handle: handleSettingsModelStatus},
		{method: http.MethodPost, path: "/api/settings/models/rollback", handle: handleSettingsModelRollback},
		{method: http.MethodGet, path: "/api/settings/llama/detect", handle: handleLlamaDetectGet},
		{method: http.MethodPost, path: "/api/settings/llama/detect", handle: handleLlamaDetectPost},
		{method: http.MethodPost, path: "/api/settings/llama/scan", handle: handleLlamaScan},
		{method: http.MethodPost, path: "/api/settings/llama/install", handle: handleLlamaInstall},
		{method: http.MethodGet, path: "/api/first-run", handle: handleFirstRunGet},
		{method: http.MethodPut, path: "/api/first-run/step", handle: handleFirstRunStep},
		{method: http.MethodGet, path: "/api/providers", handle: handleProviders},
		{method: http.MethodGet, path: "/api/monitor", handle: handleMonitor},
		{method: http.MethodGet, path: "/api/library", handle: handleLibraryList},
		{method: http.MethodPost, path: "/api/library/import", handle: handleLibraryImport},
		{method: http.MethodDelete, path: "/api/library/books/", handle: handleLibraryDelete},
	}
	return append(specs, e2eRoutes()...)
}

// routeLines — канонические строки "METHOD PATH" для сверки с ROUTES.md.
func routeLines(hub *Hub) []string {
	specs := routeSpecs(hub)
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.method + " " + s.path
	}
	return out
}

// handleHealth — GET /api/health: живой JSON-ответ (T-08-02, шаг 7).
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// router — диспетчер маршрутов: статика по путям экранов и JSON /api/...
// (T-08-02, шаг 5).
type router struct {
	routes []routeSpec
}

func newRouter(hub *Hub) *router {
	return &router{routes: routeSpecs(hub)}
}

// ServeHTTP находит маршрут по методу и пути: неверный метод — 405, неизвестный
// путь — 404, обе ошибки по единому контракту.
func (rt *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pathMatched := false
	for _, spec := range rt.routes {
		if !pathMatch(spec.path, r.URL.Path) {
			continue
		}
		pathMatched = true
		if r.Method == spec.method {
			spec.handle(w, r)
			return
		}
	}
	if pathMatched {
		writeError(w, http.StatusMethodNotAllowed, codeMethodNotAllowed, "method not allowed")
		return
	}
	writeError(w, http.StatusNotFound, codeNotFound, "route not found")
}

// pathMatch — точное совпадение или префикс для каталога (путь длиннее "/" и
// оканчивается "/").
func pathMatch(pattern, path string) bool {
	if len(pattern) > 1 && strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(path, pattern)
	}
	return path == pattern
}
