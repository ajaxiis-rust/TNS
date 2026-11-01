package webapi

import (
	"encoding/json"
	"net/http"
	"os"
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
