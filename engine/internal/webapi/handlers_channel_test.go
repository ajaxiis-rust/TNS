package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestChannelGetHidesToken — GET /api/settings не отдаёт значение токена и верно
// считает token_set/configured.
func TestChannelGetHidesToken(t *testing.T) {
	seedSettingsWorld(t)

	if err := writeSettingsKeys(settingsTOMLPath, map[string]string{"telegram": "secret-token"}); err != nil {
		t.Fatal(err)
	}

	rr := mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "secret-token") {
		t.Fatal("GET settings leaks the telegram token")
	}
	var out settingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if !out.Channel.TelegramTokenSet {
		t.Fatal("channel.telegram_token_set = false, want true")
	}
	if out.Channel.TelegramConfigured {
		t.Fatal("channel.telegram_configured = true, want false without chat_id")
	}
}

// TestChannelPutRoundTrip — PUT /api/settings/channel пишет edge/local/chat_id в
// config.yaml, токен в settings.toml, а GET отражает записанное без токена.
func TestChannelPutRoundTrip(t *testing.T) {
	seedSettingsWorld(t)

	body := []byte(`{"edge":"region2.v2.argotunnel.com","local":"http://127.0.0.1:8721","telegram_chat_id":"42","telegram_token":"tok-123"}`)
	rr := mcpRequest(t, handleSettingsChannel, http.MethodPut, settingsChannelPath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT channel: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	cfgData, err := os.ReadFile(settingsConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg := string(cfgData)
	for _, want := range []string{
		"edge: region2.v2.argotunnel.com",
		"local: http://127.0.0.1:8721",
		"telegram_chat_id: 42",
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("config.yaml missing %q\n%s", want, cfg)
		}
	}

	keys := readSettingsKeys(settingsTOMLPath)
	if keys["telegram"] != "tok-123" {
		t.Fatalf("settings.toml telegram = %q, want tok-123", keys["telegram"])
	}

	rr = mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "tok-123") {
		t.Fatal("GET settings leaks the telegram token after PUT")
	}
	var out settingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.Channel.Edge != "region2.v2.argotunnel.com" {
		t.Fatalf("channel.edge = %q, want region2.v2.argotunnel.com", out.Channel.Edge)
	}
	if out.Channel.Local != "http://127.0.0.1:8721" {
		t.Fatalf("channel.local = %q, want http://127.0.0.1:8721", out.Channel.Local)
	}
	if out.Channel.TelegramChatID != "42" {
		t.Fatalf("channel.telegram_chat_id = %q, want 42", out.Channel.TelegramChatID)
	}
	if !out.Channel.TelegramTokenSet || !out.Channel.TelegramConfigured {
		t.Fatalf("channel = %+v, want token_set и configured", out.Channel)
	}
}

// TestChannelRejectsBadInput — не-loopback local и край со схемой отвергаются 400.
func TestChannelRejectsBadInput(t *testing.T) {
	seedSettingsWorld(t)

	bad := []string{
		`{"edge":"region1.v2.argotunnel.com","local":"http://example.com:8720"}`,
		`{"edge":"https://region1.v2.argotunnel.com","local":"http://127.0.0.1:8720"}`,
		`{"edge":"region1.v2.argotunnel.com/path","local":"http://127.0.0.1:8720"}`,
	}
	for _, body := range bad {
		rr := mcpRequest(t, handleSettingsChannel, http.MethodPut, settingsChannelPath, []byte(body), "application/json")
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("PUT channel %s: status = %d, want 400; body=%s", body, rr.Code, rr.Body.String())
		}
	}
}

// TestChannelQuotedEmptyToken — пустой токен в settings.toml (в кавычках) не
// делает канал настроенным даже при заданном chat_id.
func TestChannelQuotedEmptyToken(t *testing.T) {
	seedSettingsWorld(t)

	if err := writeSettingsKeys(settingsTOMLPath, map[string]string{"telegram": ""}); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"edge":"region1.v2.argotunnel.com","local":"http://127.0.0.1:8720","telegram_chat_id":"42"}`)
	rr := mcpRequest(t, handleSettingsChannel, http.MethodPut, settingsChannelPath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT channel: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	rr = mcpRequest(t, handleSettingsGet, http.MethodGet, settingsPath+"?world=alpha", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out settingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.Channel.TelegramTokenSet {
		t.Fatal("channel.telegram_token_set = true, want false for empty token")
	}
	if out.Channel.TelegramConfigured {
		t.Fatal("channel.telegram_configured = true, want false for empty token")
	}
}

// TestWriteSettingsKeysMode — settings.toml создаётся с правами 0600: файл хранит
// токен Telegram и ключи внешних провайдеров.
func TestWriteSettingsKeysMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	if err := writeSettingsKeys(path, map[string]string{"telegram": "secret"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("settings.toml mode = %o, want 600", got)
	}
}

// TestChannelPutRestartsBotOnce — новый токен вызывает хук перезапуска ровно
// один раз; повторный PUT с теми же значениями хука не вызывает.
func TestChannelPutRestartsBotOnce(t *testing.T) {
	seedSettingsWorld(t)
	oldRestart, oldStop := TelegramRestart, StopTelegram
	t.Cleanup(func() { TelegramRestart, StopTelegram = oldRestart, oldStop })

	calls := 0
	TelegramRestart = func(context.Context) error { calls++; return nil }
	StopTelegram = func() {}

	body := []byte(`{"edge":"region1.v2.argotunnel.com","local":"http://127.0.0.1:8720","telegram_chat_id":"42","telegram_token":"tok-1"}`)
	rr := mcpRequest(t, handleSettingsChannel, http.MethodPut, settingsChannelPath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT channel: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if calls != 1 {
		t.Fatalf("restart hook calls = %d, want 1", calls)
	}

	rr = mcpRequest(t, handleSettingsChannel, http.MethodPut, settingsChannelPath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT channel (same values): status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if calls != 1 {
		t.Fatalf("restart hook calls after same values = %d, want 1", calls)
	}
}

// TestChannelPutRestartErrorWarning — ошибка хука перезапуска уходит в ответ
// предупреждением, а не 500.
func TestChannelPutRestartErrorWarning(t *testing.T) {
	seedSettingsWorld(t)
	oldRestart, oldStop := TelegramRestart, StopTelegram
	t.Cleanup(func() { TelegramRestart, StopTelegram = oldRestart, oldStop })

	TelegramRestart = func(context.Context) error { return errors.New("restart boom") }
	StopTelegram = func() {}

	body := []byte(`{"edge":"region1.v2.argotunnel.com","local":"http://127.0.0.1:8720","telegram_chat_id":"42","telegram_token":"tok-1"}`)
	rr := mcpRequest(t, handleSettingsChannel, http.MethodPut, settingsChannelPath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT channel: status = %d, want 200 (not 500); body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "restart boom") {
		t.Fatalf("PUT channel body missing restart warning, got %s", rr.Body.String())
	}
}

// TestChannelPutClearTokenStopsBot — clear_token гасит канал и отвечает
// bot_running=false, не вызывая перезапуск.
func TestChannelPutClearTokenStopsBot(t *testing.T) {
	seedSettingsWorld(t)
	oldRestart, oldStop := TelegramRestart, StopTelegram
	t.Cleanup(func() { TelegramRestart, StopTelegram = oldRestart, oldStop })

	restarts := 0
	TelegramRestart = func(context.Context) error { restarts++; return nil }
	stopped := false
	StopTelegram = func() { stopped = true }

	cfg := []byte(`{"edge":"region1.v2.argotunnel.com","local":"http://127.0.0.1:8720","telegram_chat_id":"42","telegram_token":"tok-1"}`)
	if rr := mcpRequest(t, handleSettingsChannel, http.MethodPut, settingsChannelPath, cfg, "application/json"); rr.Code != http.StatusOK {
		t.Fatalf("PUT channel: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if restarts != 1 {
		t.Fatalf("restart hook calls = %d, want 1", restarts)
	}

	clear := []byte(`{"edge":"region1.v2.argotunnel.com","local":"http://127.0.0.1:8720","telegram_chat_id":"42","clear_token":true}`)
	rr := mcpRequest(t, handleSettingsChannel, http.MethodPut, settingsChannelPath, clear, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT clear_token: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !stopped {
		t.Fatal("StopTelegram not called after clear_token")
	}
	if restarts != 1 {
		t.Fatalf("restart hook calls after clear_token = %d, want 1 (no restart)", restarts)
	}
	var out channelUpdateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.BotRunning {
		t.Fatalf("bot_running = true after clear_token, want false; body=%s", rr.Body.String())
	}
}
