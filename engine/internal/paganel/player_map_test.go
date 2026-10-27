package paganel_test

import (
	"testing"

	"tns65/internal/db"
	"tns65/internal/paganel"
	"tns65/internal/testkit"
)

// playerMapPlace читает место из player_map по place_id; false — места нет.
func playerMapPlace(t *testing.T, core *db.Handle, placeID int64) (paganel.PlayerMapPlace, bool) {
	t.Helper()
	places, err := paganel.ReadPlayerMap(core)
	if err != nil {
		t.Fatalf("ReadPlayerMap: %v", err)
	}
	for _, p := range places {
		if p.PlaceID == placeID {
			return p, true
		}
	}
	return paganel.PlayerMapPlace{}, false
}

// placeID reads id места из places по имени.
func placeID(t *testing.T, h *db.Handle, name string) int64 {
	t.Helper()
	var id int64
	if err := h.QueryRow(`SELECT id FROM places WHERE name = ? LIMIT 1`, name).Scan(&id); err != nil {
		t.Fatalf("SELECT places %q: %v", name, err)
	}
	return id
}

// TestPlayerMap — карта мира игрока (T-06-12, «Проверка»): после ковки кольца
// известные места в player_map с known_at; место за краем в player_map
// отсутствует; после посещения места known_at обновлён днём визита.
func TestPlayerMap(t *testing.T) {
	t.Run("после ковки кольца известные места в player_map", func(t *testing.T) {
		path := newRingWorld(t)
		seedHeroForge(t, path, 0, 0)
		forge := openRingForge(t, path)
		core := openRingCore(t, path)

		slot := testkit.NewFakeSlot()
		slot.SetResponse(paganel.ForgeRole, forgeOutputJSON(
			[]paganel.ForgedPlace{
				{Name: "Лес", Essence: "древний лес", Owner: "", Prominence: "известное", Bearing: "север", DistanceDays: 1},
				{Name: "Город", Essence: "город", Owner: "", Prominence: "известное", Bearing: "восток", DistanceDays: 1},
			},
			nil,
		))
		paganel.SetSlot(slot)
		defer paganel.SetSlot(nil)
		paganel.SetCensor(fakeCensor{})
		defer paganel.SetCensor(nil)

		out, err := paganel.ForgeRing(forge)
		if err != nil {
			t.Fatalf("ForgeRing: %v", err)
		}
		if len(out.Places) != 2 {
			t.Fatalf("мест в ковке = %d, want 2", len(out.Places))
		}

		// Ковка сделала место известным: ядро помечает его в player_map днём.
		forestID := placeID(t, forge, "Лес")
		if err := paganel.MarkPlaceKnown(core, forestID, 3, false); err != nil {
			t.Fatalf("MarkPlaceKnown: %v", err)
		}

		got, ok := playerMapPlace(t, core, forestID)
		if !ok {
			t.Fatal("место «Лес» отсутствует в player_map")
		}
		if got.KnownAt != "3" {
			t.Fatalf("known_at = %q, want 3", got.KnownAt)
		}
	})

	t.Run("место за краем в player_map отсутствует", func(t *testing.T) {
		path := newRingWorld(t)
		seedHeroForge(t, path, 0, 0)
		forge := openRingForge(t, path)
		core := openRingCore(t, path)

		slot := testkit.NewFakeSlot()
		slot.SetResponse(paganel.ForgeRole, forgeOutputJSON(
			[]paganel.ForgedPlace{
				{Name: "Лес", Essence: "древний лес", Owner: "", Prominence: "известное", Bearing: "север", DistanceDays: 1},
				{Name: "ЗаКраем", Essence: "дальний край", Owner: "", Prominence: "известное", Bearing: "север", DistanceDays: 1},
			},
			nil,
		))
		paganel.SetSlot(slot)
		defer paganel.SetSlot(nil)
		paganel.SetCensor(fakeCensor{})
		defer paganel.SetCensor(nil)

		if _, err := paganel.ForgeRing(forge); err != nil {
			t.Fatalf("ForgeRing: %v", err)
		}

		forestID := placeID(t, forge, "Лес")
		// Известным становится только место у героя; место за краем не помечаем.
		if err := paganel.MarkPlaceKnown(core, forestID, 3, false); err != nil {
			t.Fatalf("MarkPlaceKnown: %v", err)
		}

		if _, ok := playerMapPlace(t, core, forestID); !ok {
			t.Fatal("место «Лес» отсутствует в player_map")
		}

		beyondID := placeID(t, forge, "ЗаКраем")
		if _, ok := playerMapPlace(t, core, beyondID); ok {
			t.Fatal("место за краем попало в player_map")
		}
	})

	t.Run("после посещения места known_at обновлён днём визита", func(t *testing.T) {
		path := newRingWorld(t)
		seedHeroForge(t, path, 0, 0)
		forge := openRingForge(t, path)
		core := openRingCore(t, path)

		slot := testkit.NewFakeSlot()
		slot.SetResponse(paganel.ForgeRole, forgeOutputJSON(
			[]paganel.ForgedPlace{
				{Name: "Лес", Essence: "древний лес", Owner: "", Prominence: "известное", Bearing: "север", DistanceDays: 1},
			},
			nil,
		))
		paganel.SetSlot(slot)
		defer paganel.SetSlot(nil)
		paganel.SetCensor(fakeCensor{})
		defer paganel.SetCensor(nil)

		if _, err := paganel.ForgeRing(forge); err != nil {
			t.Fatalf("ForgeRing: %v", err)
		}
		forestID := placeID(t, forge, "Лес")
		if err := paganel.MarkPlaceKnown(core, forestID, 3, false); err != nil {
			t.Fatalf("MarkPlaceKnown: %v", err)
		}

		// Фактическое посещение: герой у места, известность обновляется днём
		// визита (T-06-04 SetHeroPlace пишет character_state, T-06-12 — карту).
		if _, err := paganel.SetHeroPlace(core, forestID); err != nil {
			t.Fatalf("SetHeroPlace: %v", err)
		}
		if _, err := paganel.MarkHeroPlaceKnown(core, 7, false); err != nil {
			t.Fatalf("MarkHeroPlaceKnown: %v", err)
		}

		got, ok := playerMapPlace(t, core, forestID)
		if !ok {
			t.Fatal("место «Лес» отсутствует в player_map после визита")
		}
		if got.KnownAt != "7" {
			t.Fatalf("known_at после визита = %q, want 7", got.KnownAt)
		}
	})
}
