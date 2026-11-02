package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"tns65/internal/journal"
)

func TestTunnelOutboundOnly(t *testing.T) {
	// Исходящая конфигурация без слушающего адреса принята (F-04.3).
	ok := TunnelConfig{Edge: "region1.v2.argotunnel.com", Local: "http://127.0.0.1:8720"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("outbound tunnel config rejected: %v", err)
	}

	// Внешний слушающий адрес отвергнут (F-04.3, F-04.4).
	bad := TunnelConfig{Edge: "region1.v2.argotunnel.com", Local: "http://127.0.0.1:8720", Bind: "0.0.0.0:8720"}
	if err := bad.Validate(); !errors.Is(err, ErrExternalBind) {
		t.Fatalf("config with external bind: err = %v, want ErrExternalBind", err)
	}

	// Пробрасываемый не-loopback адрес тоже означает внешний слушающий адрес.
	nonLoop := TunnelConfig{Edge: "region1.v2.argotunnel.com", Local: "http://example.com:8720"}
	if err := nonLoop.Validate(); !errors.Is(err, ErrExternalBind) {
		t.Fatalf("config with non-loopback local: err = %v, want ErrExternalBind", err)
	}

	// Ключ bind в секции tunnel конфигурации отвергается на разборе.
	if _, err := parseTunnel("tunnel:\n  edge: region1.v2.argotunnel.com\n  local: http://127.0.0.1:8720\n  bind: 0.0.0.0:8720\n"); !errors.Is(err, ErrExternalBind) {
		t.Fatalf("parseTunnel with bind: err = %v, want ErrExternalBind", err)
	}

	// Telegram-параметры — исходящие, конфигурация остаётся валидной (F-04.4).
	tg, err := parseTunnel("tunnel:\n  edge: region1.v2.argotunnel.com\n  local: http://127.0.0.1:8720\n  telegram_token: tok\n  telegram_chat_id: 42\n")
	if err != nil {
		t.Fatalf("parseTunnel with telegram: %v", err)
	}
	if tg.TelegramToken != "tok" || tg.TelegramChatID != "42" {
		t.Fatalf("telegram params = %q/%q, want tok/42", tg.TelegramToken, tg.TelegramChatID)
	}
	if err := tg.Validate(); err != nil {
		t.Fatalf("telegram outbound config rejected: %v", err)
	}

	// Реальная секция tunnel в engine/config.yaml — только исходящие параметры.
	cfg, err := LoadTunnelConfig(filepath.Join("..", "..", "config.yaml"))
	if err != nil {
		t.Fatalf("LoadTunnelConfig(engine/config.yaml): %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("engine/config.yaml tunnel config rejected: %v", err)
	}
	if cfg.Bind != "" {
		t.Fatalf("engine/config.yaml tunnel has bind %q, want empty", cfg.Bind)
	}
	if cfg.TelegramToken != "" || cfg.TelegramChatID != "" {
		t.Fatalf("engine/config.yaml tunnel telegram configured = %q/%q, want empty (not configured)", cfg.TelegramToken, cfg.TelegramChatID)
	}

	// Команда запуска содержит край и пробрасываемый loopback-адрес, без bind.
	args := strings.Join(ok.Command(), " ")
	if !strings.Contains(args, "--url http://127.0.0.1:8720") {
		t.Fatalf("tunnel command = %q, want loopback --url", args)
	}

	// scripts/tunnel.sh не содержит внешнего слушающего адреса (Проверка).
	script := filepath.Join("..", "..", "scripts", "tunnel.sh")
	data, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("read scripts/tunnel.sh: %v", err)
	}
	if strings.Contains(string(data), "0.0.0.0") {
		t.Fatalf("scripts/tunnel.sh contains 0.0.0.0 (external bind)")
	}
}

// TestParseTunnelUnquotes — кавычки YAML снимаются: `""` становится пустой
// строкой (канал не настроен), одинарные и двойные кавычки токена снимаются.
func TestParseTunnelUnquotes(t *testing.T) {
	cfg, err := parseTunnel("tunnel:\n" +
		"  edge: region1.v2.argotunnel.com\n" +
		"  local: http://127.0.0.1:8720\n" +
		"  telegram_token: \"\"\n" +
		"  telegram_chat_id: \"\"\n")
	if err != nil {
		t.Fatalf("parseTunnel with quoted empty telegram: %v", err)
	}
	if cfg.TelegramToken != "" || cfg.TelegramChatID != "" {
		t.Fatalf("quoted empty telegram params = %q/%q, want empty", cfg.TelegramToken, cfg.TelegramChatID)
	}
	if cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
		t.Fatalf("telegram configured = true, want false")
	}

	for _, q := range []struct{ open, close string }{{`"`, `"`}, {`'`, `'`}} {
		cfg, err := parseTunnel("tunnel:\n" +
			"  edge: region1.v2.argotunnel.com\n" +
			"  local: http://127.0.0.1:8720\n" +
			"  telegram_token: " + q.open + "tok" + q.close + "\n" +
			"  telegram_chat_id: " + q.open + "42" + q.close + "\n")
		if err != nil {
			t.Fatalf("parseTunnel with %s quotes: %v", q.open, err)
		}
		if cfg.TelegramToken != "tok" || cfg.TelegramChatID != "42" {
			t.Fatalf("telegram params (%s quotes) = %q/%q, want tok/42", q.open, cfg.TelegramToken, cfg.TelegramChatID)
		}
	}
}

type requestCapture struct {
	method string
	path   string
	body   string
}

// newBot возвращает бота, подключённого к фейковым loopback- и Telegram-серверам
// (без живой сети: httptest на loopback). testkit-фейков для HTTP не требуется —
// реальных исходящих соединений нет.
func newBot(t *testing.T, cfg TunnelConfig, loopSrv, tgSrv *httptest.Server, logger *journal.Logger) *TelegramBot {
	t.Helper()
	return &TelegramBot{
		cfg:          cfg,
		logger:       logger,
		telegramBase: tgSrv.URL,
		localBase:    loopSrv.URL,
		telegram:     tgSrv.Client(),
		local:        loopSrv.Client(),
		dial:         (&net.Dialer{}).DialContext,
	}
}

func TestTelegramBotForwardsChat(t *testing.T) {
	loopCh := make(chan requestCapture, 4)
	loopSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		loopCh <- requestCapture{method: r.Method, path: r.URL.Path, body: string(b)}
		w.WriteHeader(http.StatusOK)
	}))
	defer loopSrv.Close()

	tgCh := make(chan requestCapture, 4)
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		tgCh <- requestCapture{method: r.Method, path: r.URL.Path, body: string(b)}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer tgSrv.Close()

	bot := newBot(t, TunnelConfig{
		Edge:           "region1.v2.argotunnel.com",
		Local:          loopSrv.URL,
		TelegramToken:  "tok",
		TelegramChatID: "42",
	}, loopSrv, tgSrv, nil)

	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })
	ctx := context.Background()

	// Telegram → чат loopback-сервера (локальный порт 8720/фейк).
	if _, err := bot.ForwardChat(ctx, "привет из телеграма"); err != nil {
		t.Fatalf("ForwardChat: %v", err)
	}
	got := <-loopCh
	if got.method != http.MethodPost || got.path != chatPath {
		t.Fatalf("forward chat request = %s %s, want POST %s", got.method, got.path, chatPath)
	}
	if !strings.Contains(got.body, "привет из телеграма") || !strings.Contains(got.body, `"world":"alpha"`) {
		t.Fatalf("forward chat body = %q, want world alpha and replica", got.body)
	}

	// Чат loopback → Telegram (SendChat).
	if err := bot.SendChat(ctx, "ответ из чата"); err != nil {
		t.Fatalf("SendChat: %v", err)
	}
	assertTelegramMessage(t, <-tgCh, "ответ из чата")

	// Вести станка loopback → Telegram (SendNews).
	if err := bot.SendNews(ctx, "весть станка"); err != nil {
		t.Fatalf("SendNews: %v", err)
	}
	assertTelegramMessage(t, <-tgCh, "весть станка")
}

func assertTelegramMessage(t *testing.T, got requestCapture, wantText string) {
	t.Helper()
	if got.method != http.MethodPost || got.path != "/bot/tok/sendMessage" {
		t.Fatalf("telegram request = %s %s, want POST /bot/tok/sendMessage", got.method, got.path)
	}
	vals, err := url.ParseQuery(got.body)
	if err != nil {
		t.Fatalf("parse telegram form: %v", err)
	}
	if vals.Get("chat_id") != "42" || vals.Get("text") != wantText {
		t.Fatalf("telegram form = %v, want chat_id=42 text=%q", vals, wantText)
	}
}

func TestTelegramBotLongPoll(t *testing.T) {
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("offset") == "2" {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": []telegramUpdate{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": []telegramUpdate{
				{UpdateID: 1, Message: &telegramMessage{Text: "реплика из телеграма", Chat: &telegramChat{ID: 42}}},
			},
		})
	}))
	defer tgSrv.Close()

	loopCh := make(chan requestCapture, 4)
	loopSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		loopCh <- requestCapture{method: r.Method, path: r.URL.Path, body: string(b)}
		w.WriteHeader(http.StatusOK)
	}))
	defer loopSrv.Close()

	bot := newBot(t, TunnelConfig{
		Edge:           "region1.v2.argotunnel.com",
		Local:          loopSrv.URL,
		TelegramToken:  "tok",
		TelegramChatID: "42",
	}, loopSrv, tgSrv, nil)

	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	// Один шаг long poll пересылает реплику в чат и двигает offset.
	next, err := bot.PollOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if next != 2 {
		t.Fatalf("next offset = %d, want 2", next)
	}
	got := <-loopCh
	if got.path != chatPath || !strings.Contains(got.body, "реплика из телеграма") {
		t.Fatalf("forwarded chat = %s %s, want %s with replica", got.method, got.path, chatPath)
	}
}

func TestLoadBotConfigMergesTelegramToken(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	tomlPath := filepath.Join(dir, "settings.toml")
	if err := os.WriteFile(cfgPath, []byte("tunnel:\n  edge: region1.v2.argotunnel.com\n  local: http://127.0.0.1:8720\n  telegram_chat_id: \"42\"\n  telegram_token: \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tomlPath, []byte("telegram = \"tok-from-toml\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadBotConfig(cfgPath, tomlPath)
	if err != nil {
		t.Fatalf("LoadBotConfig: %v", err)
	}
	if cfg.TelegramToken != "tok-from-toml" || cfg.TelegramChatID != "42" {
		t.Fatalf("cfg = %q/%q, want tok-from-toml/42", cfg.TelegramToken, cfg.TelegramChatID)
	}
}

func TestTelegramBotNoActiveWorld(t *testing.T) {
	setActiveSlug("")
	t.Cleanup(func() { setActiveSlug("") })

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
				{UpdateID: 1, Message: &telegramMessage{Text: "реплика без мира", Chat: &telegramChat{ID: 42}}},
			},
		})
	}))
	defer tgSrv.Close()

	loopCalled := make(chan struct{}, 1)
	loopSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		loopCalled <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer loopSrv.Close()

	bot := newBot(t, TunnelConfig{
		Edge:           "region1.v2.argotunnel.com",
		Local:          loopSrv.URL,
		TelegramToken:  "tok",
		TelegramChatID: "42",
	}, loopSrv, tgSrv, nil)

	next, err := bot.PollOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("PollOnce without active world: %v", err)
	}
	if next != 2 {
		t.Fatalf("next offset = %d, want 2", next)
	}
	select {
	case <-loopCalled:
		t.Fatal("loopback chat must not be called without an active world")
	default:
	}
	select {
	case got := <-tgCh:
		vals, err := url.ParseQuery(got.body)
		if err != nil {
			t.Fatalf("parse telegram form: %v", err)
		}
		if !strings.Contains(vals.Get("text"), "Атласе") {
			t.Fatalf("hint = %q, want Atlas hint", vals.Get("text"))
		}
	default:
		t.Fatal("no hint sent to Telegram")
	}
}

func TestTelegramBotLogsConnect(t *testing.T) {
	dir := t.TempDir()
	logger, err := journal.New(filepath.Join(dir, "engine.log"))
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	cfg := TunnelConfig{
		Edge:           "region1.v2.argotunnel.com",
		Local:          "http://127.0.0.1:8720",
		TelegramToken:  "tok",
		TelegramChatID: "42",
	}
	LogTunnelConnect(logger, cfg)
	LogTelegramConnect(logger, cfg)

	events := readJournalEvents(t, filepath.Join(dir, "engine.log"))
	want := map[string]bool{eventTunnelConnect: false, eventTelegramConnect: false}
	for _, ev := range events {
		if _, ok := want[ev.Event]; ok {
			want[ev.Event] = true
		}
	}
	for ev, seen := range want {
		if !seen {
			t.Fatalf("journal event %q not written", ev)
		}
	}
}

type journalEvent struct {
	Source string `json:"source"`
	Event  string `json:"event"`
}

func readJournalEvents(t *testing.T, path string) []journalEvent {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var out []journalEvent
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var ev journalEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("bad journal line %q: %v", line, err)
		}
		out = append(out, ev)
	}
	return out
}

func TestTelegramBotSubscribeNews(t *testing.T) {
	hub := NewHub()
	wsSrv := httptest.NewServer(accessLog(newRouter(hub), nil))
	defer wsSrv.Close()

	tgCh := make(chan requestCapture, 4)
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		tgCh <- requestCapture{method: r.Method, path: r.URL.Path, body: string(b)}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer tgSrv.Close()

	logger, err := journal.New(filepath.Join(t.TempDir(), "engine.log"))
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	bot := newBot(t, TunnelConfig{
		Edge:           "region1.v2.argotunnel.com",
		Local:          wsSrv.URL,
		TelegramToken:  "tok",
		TelegramChatID: "42",
	}, wsSrv, tgSrv, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- bot.SubscribeNews(ctx, func(msg WSMessage) error {
			var p struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(msg.Payload, &p)
			return bot.SendNews(ctx, p.Text)
		})
	}()

	// Ждём, пока бот установил исходящее WS-подключение и подписался на топик.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if wsSubscribed(hub, newsTopic) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bot did not subscribe to station.news")
		}
		time.Sleep(2 * time.Millisecond)
	}

	payload, _ := json.Marshal(map[string]string{"text": "весть станка"})
	hub.Broadcast(WSMessage{Topic: newsTopic, EventType: "news", Payload: payload})

	select {
	case got := <-tgCh:
		assertTelegramMessage(t, got, "весть станка")
	case <-time.After(2 * time.Second):
		t.Fatal("news not forwarded to telegram")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SubscribeNews did not stop")
	}
}

// wsSubscribed сообщает, есть ли в хабе клиент, подписанный на топик.
func wsSubscribed(hub *Hub, topic string) bool {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for c := range hub.clients {
		c.mu.Lock()
		_, ok := c.topics[topic]
		c.mu.Unlock()
		if ok {
			return true
		}
	}
	return false
}

// TestTelegramBotRoundTrip — полный цикл: реплика из Telegram → POST /api/turn
// (world+text) → GET /api/state → sendMessage с ответом мира и клавиатурой сцены.
func TestTelegramBotRoundTrip(t *testing.T) {
	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	loopCh := make(chan requestCapture, 4)
	loopSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		loopCh <- requestCapture{method: r.Method, path: r.URL.RequestURI(), body: string(b)}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case chatPath:
			_ = json.NewEncoder(w).Encode(map[string]int64{"turn_id": 7})
		case chatStatePath:
			_ = json.NewEncoder(w).Encode(chatStateResponse{
				World: "alpha",
				Turns: []chatTurnItem{
					{TurnID: 7, SceneID: 9, PlayerText: "привет", ReplyText: "ответ мира"},
				},
			})
		default:
			http.NotFound(w, r)
		}
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
			"ok":     true,
			"result": []telegramUpdate{{UpdateID: 1, Message: &telegramMessage{Text: "привет", Chat: &telegramChat{ID: 42}}}},
		})
	}))
	defer tgSrv.Close()

	bot := newBot(t, TunnelConfig{
		Edge:           "region1.v2.argotunnel.com",
		Local:          loopSrv.URL,
		TelegramToken:  "tok",
		TelegramChatID: "42",
	}, loopSrv, tgSrv, nil)

	next, err := bot.PollOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if next != 2 {
		t.Fatalf("next = %d, want 2", next)
	}

	// Шаг 1: реплика ушла в POST /api/turn с миром и текстом.
	turnReq := <-loopCh
	if turnReq.method != http.MethodPost || turnReq.path != chatPath {
		t.Fatalf("turn request = %s %s, want POST %s", turnReq.method, turnReq.path, chatPath)
	}
	if !strings.Contains(turnReq.body, `"world":"alpha"`) || !strings.Contains(turnReq.body, "привет") {
		t.Fatalf("turn body = %q, want world alpha and replica", turnReq.body)
	}

	// Шаг 2: ответ читается из GET /api/state.
	stateReq := <-loopCh
	if stateReq.method != http.MethodGet || !strings.HasPrefix(stateReq.path, chatStatePath) {
		t.Fatalf("state request = %s %s, want GET %s", stateReq.method, stateReq.path, chatStatePath)
	}
	if !strings.Contains(stateReq.path, "world=alpha") {
		t.Fatalf("state path = %q, want world=alpha", stateReq.path)
	}

	// Шаг 3: ответ мира уходит в sendMessage с клавиатурой сигналов сцены.
	got := <-tgCh
	assertTelegramMessage(t, got, "ответ мира")
	vals, err := url.ParseQuery(got.body)
	if err != nil {
		t.Fatalf("parse telegram form: %v", err)
	}
	if mk := vals.Get("reply_markup"); !strings.Contains(mk, "signal:9:like") || !strings.Contains(mk, "signal:9:dislike") {
		t.Fatalf("reply_markup = %q, want scene signal buttons", mk)
	}
}

// TestTelegramBotLoopbackErrorDoesNotStopPoll — 500 от loopback не роняет цикл
// long poll: бот честно сообщает о сбое и продолжает опрос.
func TestTelegramBotLoopbackErrorDoesNotStopPoll(t *testing.T) {
	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	loopSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
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
			"ok":     true,
			"result": []telegramUpdate{{UpdateID: 1, Message: &telegramMessage{Text: "привет", Chat: &telegramChat{ID: 42}}}},
		})
	}))
	defer tgSrv.Close()

	bot := newBot(t, TunnelConfig{
		Edge:           "region1.v2.argotunnel.com",
		Local:          loopSrv.URL,
		TelegramToken:  "tok",
		TelegramChatID: "42",
	}, loopSrv, tgSrv, nil)

	next, err := bot.PollOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("PollOnce after loopback 500: %v", err)
	}
	if next != 2 {
		t.Fatalf("next = %d, want 2", next)
	}

	select {
	case got := <-tgCh:
		vals, err := url.ParseQuery(got.body)
		if err != nil {
			t.Fatalf("parse telegram form: %v", err)
		}
		if vals.Get("text") == "" {
			t.Fatalf("honest message missing after loopback 500")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no honest message sent after loopback 500")
	}
}

// TestTelegramBotStaleTurnReplyNotSent — ответ на устаревший turn_id не
// отправляется (хода уже нет в журнале GET /api/state).
func TestTelegramBotStaleTurnReplyNotSent(t *testing.T) {
	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	loopSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case chatPath:
			_ = json.NewEncoder(w).Encode(map[string]int64{"turn_id": 7})
		case chatStatePath:
			_ = json.NewEncoder(w).Encode(chatStateResponse{
				World: "alpha",
				Turns: []chatTurnItem{{TurnID: 6, ReplyText: "старый ответ"}},
			})
		default:
			http.NotFound(w, r)
		}
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
			"ok":     true,
			"result": []telegramUpdate{{UpdateID: 1, Message: &telegramMessage{Text: "привет", Chat: &telegramChat{ID: 42}}}},
		})
	}))
	defer tgSrv.Close()

	bot := newBot(t, TunnelConfig{
		Edge:           "region1.v2.argotunnel.com",
		Local:          loopSrv.URL,
		TelegramToken:  "tok",
		TelegramChatID: "42",
	}, loopSrv, tgSrv, nil)

	if _, err := bot.PollOnce(context.Background(), 0); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	select {
	case got := <-tgCh:
		t.Fatalf("stale turn reply must not be sent, got %q", got.body)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestSplitReply — длинный ответ режется по границам абзацев, внутри абзаца —
// по границе слова; куски не длиннее лимита и не рвут слово посередине.
func TestSplitReply(t *testing.T) {
	var paras []string
	for i := 0; i < 6; i++ {
		paras = append(paras, strings.Repeat("слово"+strconv.Itoa(i)+" ", 300))
	}
	text := strings.Join(paras, "\n")

	chunks := splitReply(text)
	if len(chunks) != len(paras) {
		t.Fatalf("chunks = %d, want %d (paragraph boundaries)", len(chunks), len(paras))
	}
	for i, c := range chunks {
		if c != strings.TrimSpace(paras[i]) {
			t.Fatalf("chunk %d = %q, want full paragraph %q", i, c, strings.TrimSpace(paras[i]))
		}
		if len(c) > maxTelegramText {
			t.Fatalf("chunk %d length = %d, want <= %d", i, len(c), maxTelegramText)
		}
	}

	// Один абзац длиннее лимита режется по границе слова, не по букве.
	long := strings.Repeat("слово ", 900)
	chunks = splitReply(long)
	if len(chunks) < 2 {
		t.Fatalf("long paragraph produced %d chunk, want > 1", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > maxTelegramText {
			t.Fatalf("chunk %d length = %d, want <= %d", i, len(c), maxTelegramText)
		}
	}
	if got := strings.Join(chunks, " "); got != strings.TrimSpace(long) {
		t.Fatalf("joined chunks = %q, want original words %q", got, strings.TrimSpace(long))
	}
}

// newCallbackBot собирает бота на фейковых серверах для callback-запросов:
// loopback пишет запросы в loopCh, Telegram отвечает заданным обновлением на
// getUpdates и пишет answerCallbackQuery в tgCh.
func newCallbackBot(t *testing.T, update telegramUpdate) (*TelegramBot, chan requestCapture, chan requestCapture) {
	t.Helper()
	loopCh := make(chan requestCapture, 8)
	loopSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		loopCh <- requestCapture{method: r.Method, path: r.URL.Path, body: string(b)}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	t.Cleanup(loopSrv.Close)

	tgCh := make(chan requestCapture, 8)
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "answerCallbackQuery") {
			b, _ := io.ReadAll(r.Body)
			tgCh <- requestCapture{method: r.Method, path: r.URL.Path, body: string(b)}
			_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": []telegramUpdate{update}})
	}))
	t.Cleanup(tgSrv.Close)

	bot := newBot(t, TunnelConfig{
		Edge:           "region1.v2.argotunnel.com",
		Local:          loopSrv.URL,
		TelegramToken:  "tok",
		TelegramChatID: "42",
	}, loopSrv, tgSrv, nil)
	return bot, loopCh, tgCh
}

// assertAnswerCallback проверяет, что бот ответил на callback коротким текстом.
func assertAnswerCallback(t *testing.T, got requestCapture, wantID, wantText string) {
	t.Helper()
	if got.method != http.MethodPost || got.path != "/bot/tok/answerCallbackQuery" {
		t.Fatalf("answer callback = %s %s, want POST /bot/tok/answerCallbackQuery", got.method, got.path)
	}
	vals, err := url.ParseQuery(got.body)
	if err != nil {
		t.Fatalf("parse answerCallbackQuery form: %v", err)
	}
	if vals.Get("callback_query_id") != wantID || vals.Get("text") != wantText {
		t.Fatalf("answerCallbackQuery form = %v, want callback_query_id=%s text=%s", vals, wantID, wantText)
	}
}

// assertSignalRequest проверяет POST /api/scene/{id}/signal с телом kind.
func assertSignalRequest(t *testing.T, got requestCapture, wantID string, wantKind string) {
	t.Helper()
	wantPath := "/api/scene/" + wantID + "/signal"
	if got.method != http.MethodPost || got.path != wantPath {
		t.Fatalf("signal request = %s %s, want POST %s", got.method, got.path, wantPath)
	}
	var body chatSignalRequest
	if err := json.Unmarshal([]byte(got.body), &body); err != nil {
		t.Fatalf("bad signal body %q: %v", got.body, err)
	}
	if body.Kind != wantKind {
		t.Fatalf("signal kind = %q, want %s", body.Kind, wantKind)
	}
}

// TestTelegramBotLikeCallback — лайк из inline-кнопки уходит в
// POST /api/scene/{id}/signal с kind=like, а бот отвечает «принято».
func TestTelegramBotLikeCallback(t *testing.T) {
	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	bot, loopCh, tgCh := newCallbackBot(t, telegramUpdate{
		UpdateID: 1,
		CallbackQuery: &telegramCallbackQuery{
			ID:      "cb1",
			Data:    "signal:7:like",
			Message: &telegramMessage{Chat: &telegramChat{ID: 42}},
		},
	})

	next, err := bot.PollOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if next != 2 {
		t.Fatalf("next offset = %d, want 2", next)
	}

	assertSignalRequest(t, <-loopCh, "7", "like")
	assertAnswerCallback(t, <-tgCh, "cb1", "принято")
}

// TestTelegramBotDislikeCallback — дизлайк: kind=dislike и тот же короткий ответ.
func TestTelegramBotDislikeCallback(t *testing.T) {
	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	bot, loopCh, tgCh := newCallbackBot(t, telegramUpdate{
		UpdateID: 1,
		CallbackQuery: &telegramCallbackQuery{
			ID:      "cb2",
			Data:    "signal:8:dislike",
			Message: &telegramMessage{Chat: &telegramChat{ID: 42}},
		},
	})

	next, err := bot.PollOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if next != 2 {
		t.Fatalf("next offset = %d, want 2", next)
	}

	assertSignalRequest(t, <-loopCh, "8", "dislike")
	assertAnswerCallback(t, <-tgCh, "cb2", "принято")
}

// TestTelegramBotForeignCallbackIgnored — callback из чужого чата не доходит ни
// до loopback, ни до answerCallbackQuery, но offset двигается.
func TestTelegramBotForeignCallbackIgnored(t *testing.T) {
	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	bot, loopCh, tgCh := newCallbackBot(t, telegramUpdate{
		UpdateID: 1,
		CallbackQuery: &telegramCallbackQuery{
			ID:      "cb1",
			Data:    "signal:7:like",
			Message: &telegramMessage{Chat: &telegramChat{ID: 999}},
		},
	})

	next, err := bot.PollOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if next != 2 {
		t.Fatalf("next offset = %d, want 2", next)
	}

	select {
	case got := <-loopCh:
		t.Fatalf("foreign callback must not reach loopback, got %s %s", got.method, got.path)
	default:
	}
	select {
	case got := <-tgCh:
		t.Fatalf("foreign callback must not get answerCallbackQuery, got %q", got.body)
	default:
	}
}

// TestTelegramBotUnknownCallbackDataIgnored — незнакомая callback_data молча
// игнорируется: ни loopback, ни answerCallbackQuery, ни ошибки.
func TestTelegramBotUnknownCallbackDataIgnored(t *testing.T) {
	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	bot, loopCh, tgCh := newCallbackBot(t, telegramUpdate{
		UpdateID: 1,
		CallbackQuery: &telegramCallbackQuery{
			ID:      "cb1",
			Data:    "garbage",
			Message: &telegramMessage{Chat: &telegramChat{ID: 42}},
		},
	})

	next, err := bot.PollOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if next != 2 {
		t.Fatalf("next offset = %d, want 2", next)
	}

	select {
	case got := <-loopCh:
		t.Fatalf("unknown callback data must not reach loopback, got %s %s", got.method, got.path)
	default:
	}
	select {
	case got := <-tgCh:
		t.Fatalf("unknown callback data must not get answerCallbackQuery, got %q", got.body)
	default:
	}
}

// TestTelegramBotSignalErrorAnswer — сбой loopback сигнала превращается в
// короткий ответ «ошибка» вместо тишины.
func TestTelegramBotSignalErrorAnswer(t *testing.T) {
	setActiveSlug("alpha")
	t.Cleanup(func() { setActiveSlug("") })

	loopSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer loopSrv.Close()

	tgCh := make(chan requestCapture, 8)
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "answerCallbackQuery") {
			b, _ := io.ReadAll(r.Body)
			tgCh <- requestCapture{method: r.Method, path: r.URL.Path, body: string(b)}
			_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": []telegramUpdate{{UpdateID: 1, CallbackQuery: &telegramCallbackQuery{
				ID:      "cb1",
				Data:    "signal:7:like",
				Message: &telegramMessage{Chat: &telegramChat{ID: 42}},
			}}},
		})
	}))
	defer tgSrv.Close()

	bot := newBot(t, TunnelConfig{
		Edge:           "region1.v2.argotunnel.com",
		Local:          loopSrv.URL,
		TelegramToken:  "tok",
		TelegramChatID: "42",
	}, loopSrv, tgSrv, nil)

	if _, err := bot.PollOnce(context.Background(), 0); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	assertAnswerCallback(t, <-tgCh, "cb1", "ошибка")
}
