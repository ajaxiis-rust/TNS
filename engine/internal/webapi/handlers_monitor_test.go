package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"tns65/internal/journal"
)

// seedMonitor подменяет путь журнала станка на временный файл и пишет в него
// content; после теста восстанавливает прежний путь.
func seedMonitor(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "station.log")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	old := monitorJournalPath
	monitorJournalPath = path
	t.Cleanup(func() { monitorJournalPath = old })
	return path
}

func monitorRequest(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandler(Config{})
	return mcpRequest(t, h.ServeHTTP, method, path, nil, "")
}

// topLevelKeys возвращает порядок ключей верхнего уровня JSON-объекта.
func topLevelKeys(t *testing.T, body string) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(body))
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("decode first token: %v", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		t.Fatalf("first token = %v, want '{'", tok)
	}
	var keys []string
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			t.Fatalf("decode key: %v", err)
		}
		keys = append(keys, keyTok.(string))
		var v any
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("decode value: %v", err)
		}
	}
	return keys
}

// TestMonitorReadOnly — «Проверка» T-08-11: пять блоков в заданном порядке и
// отсутствие управляющих маршрутов раздела.
func TestMonitorReadOnly(t *testing.T) {
	seedMonitor(t, "")

	rr := monitorRequest(t, http.MethodGet, monitorPath)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", monitorPath, rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("GET %s: Content-Type = %q, want application/json", monitorPath, ct)
	}

	body := rr.Body.String()
	want := []string{"slots", "roles", "machines", "degradations", "journal_tail"}
	if got := topLevelKeys(t, body); !reflect.DeepEqual(got, want) {
		t.Fatalf("monitor blocks = %v, want %v", got, want)
	}

	var out monitorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.Slots == nil || out.Roles == nil || out.Machines == nil || out.JournalTail == nil {
		t.Fatalf("monitor blocks must be arrays, got %s", body)
	}

	var methods []string
	for _, spec := range routeSpecs(NewHub()) {
		if spec.path == monitorPath {
			methods = append(methods, spec.method)
		}
	}
	if !reflect.DeepEqual(methods, []string{http.MethodGet}) {
		t.Fatalf("/api/monitor methods = %v, want only GET", methods)
	}
}

// TestDegradationCounter — «Проверка» T-08-11: счётчик деградаций хода растёт
// после отказа слота (событие turn.degraded в журнале станка).
func TestDegradationCounter(t *testing.T) {
	path := seedMonitor(t, "")

	rr := monitorRequest(t, http.MethodGet, monitorPath)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", monitorPath, rr.Code, rr.Body.String())
	}
	var out monitorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.Degradations != 0 {
		t.Fatalf("degradations = %d, want 0", out.Degradations)
	}

	appendJournalEvent(t, path, journal.EventTurnDegraded, "slot")

	rr = monitorRequest(t, http.MethodGet, monitorPath)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", monitorPath, rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.Degradations != 1 {
		t.Fatalf("degradations = %d, want 1 after slot failure", out.Degradations)
	}
}

// TestMonitorRolesAndLimit — «Проверка» FIX-4: блок «Роли» и limit_remaining
// заполняются из журнала вызовов на фикстуре вызова слота.
func TestMonitorRolesAndLimit(t *testing.T) {
	seedMonitor(t, "")
	usagePath := seedProviderCounters(t)
	writeProviderUsage(t, usagePath, strings.Join([]string{
		`{"ts":"2026-11-14T10:00:00Z","role":"weaver","provider":"gemini-flash","calls":1,"duration_ms":120}`,
		`{"ts":"2026-11-14T10:05:00Z","role":"weaver","provider":"gemini-flash","calls":1,"duration_ms":80,"error":"slot degraded","limit_remaining":"1234"}`,
		`{"ts":"2026-11-14T10:06:00Z","role":"editor","calls":1,"duration_ms":40}`,
	}, "\n")+"\n")

	rr := monitorRequest(t, http.MethodGet, monitorPath)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", monitorPath, rr.Code, rr.Body.String())
	}
	var out monitorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}

	roles := map[string]monitorRole{}
	for _, r := range out.Roles {
		roles[r.Name] = r
	}
	weaver, ok := roles["weaver"]
	if !ok {
		t.Fatalf("в блоке «Роли» нет weaver: %s", rr.Body.String())
	}
	if weaver.Calls != 2 || weaver.Errors != 1 {
		t.Fatalf("weaver calls/errors = %d/%d, want 2/1", weaver.Calls, weaver.Errors)
	}
	if weaver.ErrorPercent != 50 {
		t.Fatalf("weaver error_percent = %v, want 50", weaver.ErrorPercent)
	}
	if weaver.AvgResponseMS != 100 {
		t.Fatalf("weaver avg_response_ms = %v, want 100", weaver.AvgResponseMS)
	}
	if weaver.LastCallAt != "2026-11-14T10:05:00Z" {
		t.Fatalf("weaver last_call_at = %q, want 2026-11-14T10:05:00Z", weaver.LastCallAt)
	}
	if editor := roles["editor"]; editor.Calls != 1 || editor.Errors != 0 {
		t.Fatalf("editor calls/errors = %d/%d, want 1/0", editor.Calls, editor.Errors)
	}

	var flash *monitorSlot
	for i := range out.Slots {
		if out.Slots[i].Name == "gemini-flash" {
			flash = &out.Slots[i]
		}
	}
	if flash == nil || flash.LimitRemaining != "1234" {
		t.Fatalf("у слота gemini-flash нет остатка лимита: %+v", out.Slots)
	}
	if !strings.Contains(rr.Body.String(), `"limit_remaining":"1234"`) {
		t.Fatalf("в ответе нет поля limit_remaining: %s", rr.Body.String())
	}
}

func appendJournalEvent(t *testing.T, path, event, reason string) {
	t.Helper()
	rec := map[string]any{
		"ts":     "2026-10-28T00:00:00Z",
		"level":  journal.LevelInfo,
		"source": "turn",
		"event":  event,
		"fields": []journal.Field{{Key: "reason", Value: reason}},
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		t.Fatal(err)
	}
}
