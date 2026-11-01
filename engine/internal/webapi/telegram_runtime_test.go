package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"tns65/internal/journal"
)

// TestStartTelegramNotConfigured — без токена или chat_id StartTelegram
// возвращает ErrTelegramNotConfigured и ничего не запускает.
func TestStartTelegramNotConfigured(t *testing.T) {
	botRunning.Store(false)
	defer botRunning.Store(false)

	cases := []TunnelConfig{
		{Edge: "region1.v2.argotunnel.com", Local: "http://127.0.0.1:8720"},
		{Edge: "region1.v2.argotunnel.com", Local: "http://127.0.0.1:8720", TelegramToken: "tok"},
		{Edge: "region1.v2.argotunnel.com", Local: "http://127.0.0.1:8720", TelegramChatID: "42"},
	}
	for _, cfg := range cases {
		if err := StartTelegram(context.Background(), cfg, nil); !errors.Is(err, ErrTelegramNotConfigured) {
			t.Fatalf("StartTelegram(%+v) err = %v, want ErrTelegramNotConfigured", cfg, err)
		}
	}
	if botRunning.Load() {
		t.Fatal("botRunning = true after unconfigured StartTelegram, want false")
	}
}

// TestStartTelegramLifecycle — бот стартует на фейковых Telegram- и loopback-
// серверах, пишет одно событие telegram.connect, поднимает флаг botRunning и
// снимает его при отмене контекста.
func TestStartTelegramLifecycle(t *testing.T) {
	tgCh := make(chan requestCapture, 16)
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		select {
		case tgCh <- requestCapture{method: r.Method, path: r.URL.Path, body: string(b)}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": []telegramUpdate{}})
	}))
	defer tgSrv.Close()

	hub := NewHub()
	loopSrv := httptest.NewServer(accessLog(newRouter(hub), nil))
	defer loopSrv.Close()

	dir := t.TempDir()
	logger, err := journal.New(filepath.Join(dir, "engine.log"))
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	cfg := TunnelConfig{
		Edge:           "region1.v2.argotunnel.com",
		Local:          loopSrv.URL,
		TelegramToken:  "tok",
		TelegramChatID: "42",
	}

	oldFactory := newTelegramBot
	newTelegramBot = func(cfg TunnelConfig, logger *journal.Logger) *TelegramBot {
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
	defer func() { newTelegramBot = oldFactory }()

	botRunning.Store(false)
	defer botRunning.Store(false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := StartTelegram(ctx, cfg, logger); err != nil {
		t.Fatalf("StartTelegram: %v", err)
	}
	if !botRunning.Load() {
		t.Fatal("botRunning = false after StartTelegram, want true")
	}

	// Poll должен выполнить хотя бы один getUpdates к фейковому Telegram.
	deadline := time.Now().Add(2 * time.Second)
	for !telegramSawPath(tgCh, "/bot/tok/getUpdates") {
		if time.Now().After(deadline) {
			t.Fatal("Poll did not reach fake Telegram getUpdates")
		}
		time.Sleep(2 * time.Millisecond)
	}

	// Одно событие telegram.connect на старт канала.
	events := readJournalEvents(t, filepath.Join(dir, "engine.log"))
	connects := 0
	for _, ev := range events {
		if ev.Event == eventTelegramConnect {
			connects++
		}
	}
	if connects != 1 {
		t.Fatalf("telegram.connect events = %d, want 1", connects)
	}

	cancel()
	deadline = time.Now().Add(2 * time.Second)
	for botRunning.Load() {
		if time.Now().After(deadline) {
			t.Fatal("botRunning stayed true after cancel")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// telegramSawPath неблокирующе вычитывает канал и сообщает, была ли запись с
// заданным путём запроса.
func telegramSawPath(ch chan requestCapture, path string) bool {
	for {
		select {
		case r := <-ch:
			if r.path == path {
				return true
			}
		default:
			return false
		}
	}
}
