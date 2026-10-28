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

	ctx := context.Background()

	// Telegram → чат loopback-сервера (локальный порт 8720/фейк).
	if err := bot.ForwardChat(ctx, "привет из телеграма"); err != nil {
		t.Fatalf("ForwardChat: %v", err)
	}
	got := <-loopCh
	if got.method != http.MethodPost || got.path != chatPath {
		t.Fatalf("forward chat request = %s %s, want POST %s", got.method, got.path, chatPath)
	}
	if !strings.Contains(got.body, "привет из телеграма") {
		t.Fatalf("forward chat body = %q, want replica", got.body)
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
				{UpdateID: 1, Message: &telegramMessage{Text: "реплика из телеграма"}},
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
