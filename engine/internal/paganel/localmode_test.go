package paganel_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tns65/internal/netgate"
	"tns65/internal/paganel"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// TestLocalMode — режим «только локально» (T-06-11, «Проверка»): при пресете
// local счётчик внешних вызовов разведки равен нулю даже при scout_enabled=true;
// ковка использует локальную модель (счётчик внешних вызовов ковки ноль).
func TestLocalMode(t *testing.T) {
	t.Run("при пресете local переключатель разведки игнорируется", func(t *testing.T) {
		mode := paganel.NewLocalMode(workshop.ProviderLocal)
		if !mode.IsLocal() {
			t.Fatal("пресет local не распознан как локальный")
		}
		if mode.ScoutEnabled(true) {
			t.Fatal("ScoutEnabled(true) при локальном пресете = true, want false")
		}
		if mode.ScoutGate(true) {
			t.Fatal("ScoutGate(true) при локальном пресете = true, want false")
		}
		if got := mode.ScoutCalls(); got != 0 {
			t.Fatalf("счётчик внешних вызовов разведки = %d, want 0", got)
		}
	})

	t.Run("при пресете local вызов разведки не уходит наружу даже при scout_enabled=true", func(t *testing.T) {
		var calls int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(&calls, 1)
			_, _ = w.Write([]byte("наружу не выходим"))
		}))
		defer srv.Close()

		mode := paganel.NewLocalMode(workshop.ProviderLocal)
		cfg := mode.ScoutConfig(paganel.ScoutConfig{Enabled: true, Budget: 3, Timeout: time.Second})
		if cfg.Enabled {
			t.Fatal("ScoutConfig(Enabled=true) при локальном пресете оставил Enabled=true")
		}
		scout := paganel.NewScout(cfg, netgate.Local(strings.TrimPrefix(srv.URL, "http://")))
		scout.SetURL(srv.URL)

		if _, err := scout.Fetch(context.Background(), "город за холмом"); !errors.Is(err, paganel.ErrScoutDisabled) {
			t.Fatalf("Fetch при локальном пресете = %v, want ErrScoutDisabled", err)
		}
		if atomic.LoadInt32(&calls) != 0 {
			t.Fatalf("внешних вызовов = %d, want 0", calls)
		}
		if got := mode.ScoutCalls(); got != 0 {
			t.Fatalf("счётчик внешних вызовов разведки = %d, want 0", got)
		}
	})

	t.Run("ковка при пресете local идёт на локальной модели без внешних вызовов", func(t *testing.T) {
		mode := paganel.NewLocalMode(workshop.ProviderLocal)
		if !mode.ForgeIsLocal() {
			t.Fatal("ForgeIsLocal при локальном пресете = false, want true")
		}
		if mode.ForgeGate() {
			t.Fatal("ForgeGate при локальном пресете = true, want false")
		}
		if got := mode.ForgeCalls(); got != 0 {
			t.Fatalf("счётчик внешних вызовов ковки = %d, want 0", got)
		}

		path := newRingWorld(t)
		seedHeroForge(t, path, 0, 0)
		forge := openRingForge(t, path)

		slot := testkit.NewFakeSlot()
		slot.SetResponse(paganel.ForgeRole, forgeOutputJSON(
			[]paganel.ForgedPlace{{Name: "Лес", Essence: "лес", Owner: "", Prominence: "известное", Bearing: "север", DistanceDays: 1}},
			nil,
		))
		paganel.SetSlot(slot)
		defer paganel.SetSlot(nil)
		paganel.SetCensor(fakeCensor{})
		defer paganel.SetCensor(nil)

		if _, err := paganel.ForgeRing(forge); err != nil {
			t.Fatalf("ForgeRing на локальной модели: %v", err)
		}
		if slot.Calls() != 1 {
			t.Fatalf("вызовов локальной модели ковки = %d, want 1", slot.Calls())
		}
		if got := mode.ForgeCalls(); got != 0 {
			t.Fatalf("счётчик внешних вызовов ковки = %d, want 0", got)
		}
	})

	t.Run("внешний пресет разрешает разведку и ковку и учитывает вызовы", func(t *testing.T) {
		mode := paganel.NewLocalMode(workshop.ProviderGeminiFlash)
		if mode.IsLocal() {
			t.Fatal("внешний пресет распознан как локальный")
		}
		if !mode.ScoutEnabled(true) {
			t.Fatal("ScoutEnabled(true) при внешнем пресете = false, want true")
		}
		if !mode.ScoutGate(true) {
			t.Fatal("ScoutGate(true) при внешнем пресете = false, want true")
		}
		if got := mode.ScoutCalls(); got != 1 {
			t.Fatalf("счётчик внешних вызовов разведки = %d, want 1", got)
		}
		if mode.ScoutEnabled(false) {
			t.Fatal("ScoutEnabled(false) при внешнем пресете = true, want false")
		}

		if mode.ForgeIsLocal() {
			t.Fatal("ForgeIsLocal при внешнем пресете = true, want false")
		}
		if !mode.ForgeGate() {
			t.Fatal("ForgeGate при внешнем пресете = false, want true")
		}
		if got := mode.ForgeCalls(); got != 1 {
			t.Fatalf("счётчик внешних вызовов ковки = %d, want 1", got)
		}
	})
}
