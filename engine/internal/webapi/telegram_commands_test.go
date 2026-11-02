package webapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// captureServer — фейковый HTTP-сервер, записывающий запросы (метод, путь с
// query, тело) в порядке прихода; ответ задаётся обработчиком handle.
type captureServer struct {
	mu     sync.Mutex
	reqs   []requestCapture
	handle func(w http.ResponseWriter, r *http.Request)
}

func newCaptureServer(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) (*captureServer, *httptest.Server) {
	t.Helper()
	cs := &captureServer{handle: handle}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		cs.mu.Lock()
		cs.reqs = append(cs.reqs, requestCapture{method: r.Method, path: r.URL.RequestURI(), body: string(b)})
		cs.mu.Unlock()
		if cs.handle != nil {
			cs.handle(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	return cs, srv
}

func (cs *captureServer) requests() []requestCapture {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	out := make([]requestCapture, len(cs.reqs))
	copy(out, cs.reqs)
	return out
}

func testCommandCfg(loopSrv *httptest.Server) TunnelConfig {
	return TunnelConfig{
		Edge:           "region1.v2.argotunnel.com",
		Local:          loopSrv.URL,
		TelegramToken:  "tok",
		TelegramChatID: "42",
	}
}

// replyTexts собирает тексты sendMessage, отправленных ботом.
func replyTexts(t *testing.T, tgCS *captureServer) []string {
	t.Helper()
	var out []string
	for _, r := range tgCS.requests() {
		if !strings.Contains(r.path, "sendMessage") {
			continue
		}
		vals, err := url.ParseQuery(r.body)
		if err != nil {
			t.Fatalf("parse telegram form: %v", err)
		}
		out = append(out, vals.Get("text"))
	}
	return out
}

// TestTelegramBotForeignChatIgnored — сообщение из чужого чата не доходит до
// POST /api/turn и не порождает sendMessage, но offset всё равно двигается.
func TestTelegramBotForeignChatIgnored(t *testing.T) {
	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	loopCh := make(chan requestCapture, 4)
	loopSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		loopCh <- requestCapture{method: r.Method, path: r.URL.Path, body: string(b)}
		w.WriteHeader(http.StatusOK)
	}))
	defer loopSrv.Close()

	tgCh := make(chan requestCapture, 4)
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sendMessage") {
			b, _ := io.ReadAll(r.Body)
			tgCh <- requestCapture{method: r.Method, path: r.URL.Path, body: string(b)}
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": []telegramUpdate{
				{UpdateID: 1, Message: &telegramMessage{Text: "чужой", Chat: &telegramChat{ID: 999}}},
			},
		})
	}))
	defer tgSrv.Close()

	bot := newBot(t, testCommandCfg(loopSrv), loopSrv, tgSrv, nil)

	next, err := bot.PollOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if next != 2 {
		t.Fatalf("next offset = %d, want 2 (чужой чат двигает offset)", next)
	}

	select {
	case <-loopCh:
		t.Fatal("чужой чат не должен попадать в POST /api/turn")
	default:
	}
	select {
	case got := <-tgCh:
		t.Fatalf("чужой чат не должен получать sendMessage, got %q", got.body)
	default:
	}
}

// TestParseCommand — /cmd@BotName и регистр команд терпимы, аргумент отделяется.
func TestParseCommand(t *testing.T) {
	cases := []struct {
		in       string
		wantName string
		wantArg  string
	}{
		{"/help", "/help", ""},
		{"/HELP", "/help", ""},
		{"/world@MyBot beta", "/world", "beta"},
		{"/world beta", "/world", "beta"},
		{"/model analysis gemini-flash", "/model", "analysis gemini-flash"},
		{"просто текст", "", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		name, arg := parseCommand(c.in)
		if name != c.wantName || arg != c.wantArg {
			t.Errorf("parseCommand(%q) = %q/%q, want %q/%q", c.in, name, arg, c.wantName, c.wantArg)
		}
	}
}

func TestCommandHelp(t *testing.T) {
	_, loopSrv := newCaptureServer(t, nil)
	defer loopSrv.Close()
	tgCS, tgSrv := newCaptureServer(t, nil)
	defer tgSrv.Close()

	bot := newBot(t, testCommandCfg(loopSrv), loopSrv, tgSrv, nil)
	bot.handleCommand(context.Background(), "/help")

	texts := replyTexts(t, tgCS)
	if len(texts) != 1 || !strings.Contains(texts[0], "/stats") {
		t.Fatalf("help reply = %v, want список команд со /stats", texts)
	}
}

func TestCommandUnknown(t *testing.T) {
	loopCS, loopSrv := newCaptureServer(t, nil)
	defer loopSrv.Close()
	tgCS, tgSrv := newCaptureServer(t, nil)
	defer tgSrv.Close()

	bot := newBot(t, testCommandCfg(loopSrv), loopSrv, tgSrv, nil)
	bot.handleCommand(context.Background(), "/foobar")

	texts := replyTexts(t, tgCS)
	if len(texts) != 1 || !strings.Contains(texts[0], "/help") {
		t.Fatalf("unknown command reply = %v, want ссылку на /help", texts)
	}
	if n := len(loopCS.requests()); n != 0 {
		t.Fatalf("unknown command loopback requests = %d, want 0", n)
	}
}

func TestCommandWorlds(t *testing.T) {
	loopCS, loopSrv := newCaptureServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, atlasListResponse{
			Active: "alpha",
			Worlds: []atlasWorldCard{{Slug: "alpha", Title: "Альфа", Active: true}},
		})
	})
	defer loopSrv.Close()
	tgCS, tgSrv := newCaptureServer(t, nil)
	defer tgSrv.Close()

	bot := newBot(t, testCommandCfg(loopSrv), loopSrv, tgSrv, nil)
	bot.handleCommand(context.Background(), "/worlds")

	reqs := loopCS.requests()
	if len(reqs) != 1 {
		t.Fatalf("loopback requests = %d, want 1", len(reqs))
	}
	if reqs[0].method != http.MethodGet || reqs[0].path != atlasWorldsPath {
		t.Fatalf("request = %s %s, want GET %s", reqs[0].method, reqs[0].path, atlasWorldsPath)
	}
	texts := replyTexts(t, tgCS)
	if len(texts) != 1 || !strings.Contains(texts[0], "Альфа") {
		t.Fatalf("worlds reply = %v, want карточку мира Альфа", texts)
	}
}

func TestCommandWorldSwitch(t *testing.T) {
	loopCS, loopSrv := newCaptureServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "active": "beta"})
	})
	defer loopSrv.Close()
	_, tgSrv := newCaptureServer(t, nil)
	defer tgSrv.Close()

	bot := newBot(t, testCommandCfg(loopSrv), loopSrv, tgSrv, nil)
	bot.handleCommand(context.Background(), "/world beta")

	reqs := loopCS.requests()
	if len(reqs) != 1 {
		t.Fatalf("loopback requests = %d, want 1", len(reqs))
	}
	r := reqs[0]
	if r.method != http.MethodPost || r.path != atlasActivePath {
		t.Fatalf("request = %s %s, want POST %s", r.method, r.path, atlasActivePath)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(r.body), &body); err != nil {
		t.Fatalf("bad switch body %q: %v", r.body, err)
	}
	if body["world"] != "beta" {
		t.Fatalf("switch body world = %q, want beta", body["world"])
	}
}

func TestCommandWorldCurrent(t *testing.T) {
	loopCS, loopSrv := newCaptureServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, atlasListResponse{
			Active: "alpha",
			Worlds: []atlasWorldCard{{Slug: "alpha", Title: "Альфа", Active: true}},
		})
	})
	defer loopSrv.Close()
	tgCS, tgSrv := newCaptureServer(t, nil)
	defer tgSrv.Close()

	bot := newBot(t, testCommandCfg(loopSrv), loopSrv, tgSrv, nil)
	bot.handleCommand(context.Background(), "/world")

	reqs := loopCS.requests()
	if len(reqs) != 1 {
		t.Fatalf("loopback requests = %d, want 1", len(reqs))
	}
	if reqs[0].method != http.MethodGet || reqs[0].path != atlasWorldsPath {
		t.Fatalf("request = %s %s, want GET %s", reqs[0].method, reqs[0].path, atlasWorldsPath)
	}
	texts := replyTexts(t, tgCS)
	if len(texts) != 1 || !strings.Contains(texts[0], "Альфа") {
		t.Fatalf("current world reply = %v, want Альфа", texts)
	}
}

func TestCommandStats(t *testing.T) {
	loopCS, loopSrv := newCaptureServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, chatStateResponse{
			World:      "alpha",
			WorldState: map[string]string{"title": "Альфа", "travel_mode": "пешком"},
			Hero: chatHeroState{
				Mood:        "спокоен",
				Health:      90,
				HealthKnown: true,
				Place:       "Таверна",
				PlaceKind:   "город",
			},
			System: chatSystemState{Mode: "normal", SessionID: 7, StartedAt: "2026-01-01T00:00:00Z"},
			Turns:  []chatTurnItem{{TurnID: 5}},
		})
	})
	defer loopSrv.Close()
	tgCS, tgSrv := newCaptureServer(t, nil)
	defer tgSrv.Close()

	bot := newBot(t, testCommandCfg(loopSrv), loopSrv, tgSrv, nil)
	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	bot.handleCommand(context.Background(), "/stats")

	reqs := loopCS.requests()
	if len(reqs) != 1 {
		t.Fatalf("loopback requests = %d, want 1", len(reqs))
	}
	if reqs[0].method != http.MethodGet || !strings.HasPrefix(reqs[0].path, chatStatePath) {
		t.Fatalf("request = %s %s, want GET %s", reqs[0].method, reqs[0].path, chatStatePath)
	}
	if !strings.Contains(reqs[0].path, "world=alpha") {
		t.Fatalf("stats path = %q, want world=alpha", reqs[0].path)
	}

	texts := replyTexts(t, tgCS)
	if len(texts) != 1 {
		t.Fatalf("stats replies = %d, want 1", len(texts))
	}
	text := texts[0]
	for _, want := range []string{
		"Мир: Альфа", "пешком", "Здоровье: Великолепное", "Настроение: спокоен",
		"Место: Таверна (город)", "Сессия: 7", "Последний ход: 5",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("stats reply missing %q; got:\n%s", want, text)
		}
	}
	if strings.Contains(text, "90") {
		t.Errorf("stats reply must not show raw health percent; got:\n%s", text)
	}
}

func TestCommandModelList(t *testing.T) {
	loopCS, loopSrv := newCaptureServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case providersPath:
			writeJSON(w, http.StatusOK, map[string]any{
				"providers": []providerCard{{Name: "gemini-flash", KeySet: true, Model: "gemini-2.0-flash", Quota: "100", CostMillis: 0}},
			})
		case settingsPath:
			writeJSON(w, http.StatusOK, map[string]any{
				"bindings": []slotBinding{{Role: "analysis", ModelRef: "gemini-2.0-flash", Provider: "gemini-flash"}},
			})
		default:
			http.NotFound(w, r)
		}
	})
	defer loopSrv.Close()
	tgCS, tgSrv := newCaptureServer(t, nil)
	defer tgSrv.Close()

	bot := newBot(t, testCommandCfg(loopSrv), loopSrv, tgSrv, nil)
	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	bot.handleCommand(context.Background(), "/model")

	reqs := loopCS.requests()
	if len(reqs) != 2 {
		t.Fatalf("loopback requests = %d, want 2", len(reqs))
	}
	if reqs[0].method != http.MethodGet || reqs[0].path != providersPath {
		t.Fatalf("request[0] = %s %s, want GET %s", reqs[0].method, reqs[0].path, providersPath)
	}
	if reqs[1].method != http.MethodGet || !strings.HasPrefix(reqs[1].path, settingsPath) {
		t.Fatalf("request[1] = %s %s, want GET %s", reqs[1].method, reqs[1].path, settingsPath)
	}
	texts := replyTexts(t, tgCS)
	if len(texts) != 1 || !strings.Contains(texts[0], "gemini-flash") || !strings.Contains(texts[0], "analysis") {
		t.Fatalf("model reply = %v, want провайдера и роль analysis", texts)
	}
}

func TestCommandModelSwitch(t *testing.T) {
	loopCS, loopSrv := newCaptureServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case providersPath:
			writeJSON(w, http.StatusOK, map[string]any{
				"providers": []providerCard{{Name: "gemini-flash", KeySet: true, Model: "gemini-2.0-flash", Quota: "100"}},
			})
		case settingsBindingsPath:
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		default:
			http.NotFound(w, r)
		}
	})
	defer loopSrv.Close()
	_, tgSrv := newCaptureServer(t, nil)
	defer tgSrv.Close()

	bot := newBot(t, testCommandCfg(loopSrv), loopSrv, tgSrv, nil)
	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	bot.handleCommand(context.Background(), "/model analysis gemini-flash")

	reqs := loopCS.requests()
	if len(reqs) != 2 {
		t.Fatalf("loopback requests = %d, want 2", len(reqs))
	}
	if reqs[0].method != http.MethodGet || reqs[0].path != providersPath {
		t.Fatalf("request[0] = %s %s, want GET %s", reqs[0].method, reqs[0].path, providersPath)
	}
	r := reqs[1]
	if r.method != http.MethodPut || r.path != settingsBindingsPath {
		t.Fatalf("request[1] = %s %s, want PUT %s", r.method, r.path, settingsBindingsPath)
	}
	var body modelSwitchRequest
	if err := json.Unmarshal([]byte(r.body), &body); err != nil {
		t.Fatalf("bad bindings body %q: %v", r.body, err)
	}
	if body.World != "alpha" || len(body.Bindings) != 1 {
		t.Fatalf("bindings body = %+v, want world alpha и одну привязку", body)
	}
	b := body.Bindings[0]
	if b.Role != "analysis" || b.Provider != "gemini-flash" || b.ModelRef != "gemini-2.0-flash" {
		t.Fatalf("binding = %+v, want analysis/gemini-flash/gemini-2.0-flash", b)
	}
}

func TestCommandModelBadProvider(t *testing.T) {
	loopCS, loopSrv := newCaptureServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case providersPath:
			writeJSON(w, http.StatusOK, map[string]any{
				"providers": []providerCard{{Name: "gemini-flash", KeySet: true, Model: "gemini-2.0-flash", Quota: "100"}},
			})
		default:
			http.NotFound(w, r)
		}
	})
	defer loopSrv.Close()
	tgCS, tgSrv := newCaptureServer(t, nil)
	defer tgSrv.Close()

	bot := newBot(t, testCommandCfg(loopSrv), loopSrv, tgSrv, nil)
	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	bot.handleCommand(context.Background(), "/model analysis nope")

	for _, r := range loopCS.requests() {
		if r.path == settingsBindingsPath {
			t.Fatalf("PUT %s must not be called for a bad provider", settingsBindingsPath)
		}
	}
	texts := replyTexts(t, tgCS)
	if len(texts) != 1 || !strings.Contains(texts[0], "провайдера") {
		t.Fatalf("bad provider reply = %v, want понятное сообщение о провайдере", texts)
	}
}

func TestCommandModelBadRole(t *testing.T) {
	loopCS, loopSrv := newCaptureServer(t, nil)
	defer loopSrv.Close()
	tgCS, tgSrv := newCaptureServer(t, nil)
	defer tgSrv.Close()

	bot := newBot(t, testCommandCfg(loopSrv), loopSrv, tgSrv, nil)
	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	bot.handleCommand(context.Background(), "/model nonexistent gemini-flash")

	if n := len(loopCS.requests()); n != 0 {
		t.Fatalf("bad role loopback requests = %d, want 0", n)
	}
	texts := replyTexts(t, tgCS)
	if len(texts) != 1 || !strings.Contains(texts[0], "роли") {
		t.Fatalf("bad role reply = %v, want понятное сообщение о роли", texts)
	}
}

func TestHealthWord(t *testing.T) {
	cases := []struct {
		health int
		want   string
	}{
		{100, "Великолепное"},
		{86, "Великолепное"},
		{85, "Отличное"},
		{71, "Отличное"},
		{70, "Хорошее"},
		{56, "Хорошее"},
		{55, "Среднее"},
		{41, "Среднее"},
		{40, "Не хорошее"},
		{26, "Не хорошее"},
		{25, "Плохое"},
		{11, "Плохое"},
		{10, "Критическое"},
		{1, "Критическое"},
		{0, "Критическое"},
	}
	for _, c := range cases {
		if got := healthWord(c.health); got != c.want {
			t.Errorf("healthWord(%d) = %q, want %q", c.health, got, c.want)
		}
	}
}
