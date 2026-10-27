package paganel_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/paganel"
	"tns65/internal/testkit"
)

// openRingForge — хэндл мира от писателя forge (места и пути пишет ковка).
func openRingForge(t *testing.T, path string) *db.Handle {
	t.Helper()
	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

// seedHeroForge ставит героя в character_state (0, 0) без привязки к месту —
// сырым писателем, до открытия forge-хэндла (карта писателей: character_state —
// core; тест сеет напрямую).
func seedHeroForge(t *testing.T, path string, x, y float64) {
	t.Helper()
	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	defer raw.Close()
	if _, err := raw.Exec(
		`INSERT INTO character_state (character_id, place_id, x, y) VALUES (0, 0, ?, ?)`,
		x, y); err != nil {
		t.Fatalf("INSERT character_state: %v", err)
	}
}

// forgeOutputJSON собирает строгий JSON ковки из мест и путей.
func forgeOutputJSON(places []paganel.ForgedPlace, paths []paganel.ForgedPath) string {
	raw, err := json.Marshal(paganel.ForgeOutput{Places: places, Paths: paths})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// fakeCensor — словарный фильтр фикстуры: текст с forbidden-словом отклоняется.
type fakeCensor struct {
	forbidden string
}

func (f fakeCensor) Check(text string) (bool, []string) {
	if f.forbidden != "" && strings.Contains(text, f.forbidden) {
		return false, []string{f.forbidden}
	}
	return true, nil
}

// placeStatus читает origin/status/координаты места по имени.
func placeStatus(t *testing.T, forge *db.Handle, name string) (origin, status string, x, y float64) {
	t.Helper()
	var nx, ny float64
	if err := forge.QueryRow(
		`SELECT origin, status, COALESCE(x, 0), COALESCE(y, 0) FROM places WHERE name = ?`, name).
		Scan(&origin, &status, &nx, &ny); err != nil {
		t.Fatalf("SELECT place %q: %v", name, err)
	}
	return origin, status, nx, ny
}

// countNamedPlaces возвращает число строк places с именем name.
func countNamedPlaces(t *testing.T, forge *db.Handle, name string) int {
	t.Helper()
	var n int
	if err := forge.QueryRow(`SELECT COUNT(*) FROM places WHERE name = ?`, name).Scan(&n); err != nil {
		t.Fatalf("COUNT places %q: %v", name, err)
	}
	return n
}

// TestForgeRing — ковка кольца (T-06-08, «Проверка»): после ковки в places
// строка origin='paganel' со status='draft', координаты назначены кодом от
// bearing/distance_days; после промоушена status='baked'; запись с запретным
// словом отклонена в промоушене (reforge); повторная ковка не создаёт дубль;
// обращений к literary.db нет.
func TestForgeRing(t *testing.T) {
	t.Run("ковка пишет draft-место с координатами от bearing/distance_days", func(t *testing.T) {
		path := newRingWorld(t)
		seedHeroForge(t, path, 0, 0)
		forge := openRingForge(t, path)

		slot := testkit.NewFakeSlot()
		slot.SetResponse(paganel.ForgeRole, forgeOutputJSON(
			[]paganel.ForgedPlace{
				{Name: "Лес", Essence: "древний лес на пути", Owner: "леший", Prominence: "известное", Bearing: "север", DistanceDays: 2},
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
		if slot.Calls() != 1 {
			t.Fatalf("вызовов слота = %d, want 1", slot.Calls())
		}

		origin, status, x, y := placeStatus(t, forge, "Лес")
		if origin != "paganel" || status != "draft" {
			t.Fatalf("origin/status = %q/%q, want paganel/draft", origin, status)
		}
		// «север» на 2 дня (норма 25 км/день → 50 км) — севернее героя: x≈0, y>0.
		if math.Abs(x) > 1e-6 || y <= 0 {
			t.Fatalf("координаты = (%v, %v), want север от героя (x≈0, y>0)", x, y)
		}
	})

	t.Run("ковка пишет draft-путь между извлечёнными местами", func(t *testing.T) {
		path := newRingWorld(t)
		seedHeroForge(t, path, 0, 0)
		forge := openRingForge(t, path)

		slot := testkit.NewFakeSlot()
		slot.SetResponse(paganel.ForgeRole, forgeOutputJSON(
			[]paganel.ForgedPlace{
				{Name: "Лес", Essence: "лес", Owner: "", Prominence: "известное", Bearing: "север", DistanceDays: 2},
				{Name: "Город", Essence: "город", Owner: "", Prominence: "известное", Bearing: "восток", DistanceDays: 2},
			},
			[]paganel.ForgedPath{{From: "Лес", To: "Город", Type: "trakt"}},
		))
		paganel.SetSlot(slot)
		defer paganel.SetSlot(nil)
		paganel.SetCensor(fakeCensor{})
		defer paganel.SetCensor(nil)

		if _, err := paganel.ForgeRing(forge); err != nil {
			t.Fatalf("ForgeRing: %v", err)
		}

		var pathType, origin, status string
		var fromID, toID int64
		var length float64
		if err := forge.QueryRow(
			`SELECT from_place_id, to_place_id, type, length_km, origin, status
			 FROM paths WHERE type = 'trakt'`).
			Scan(&fromID, &toID, &pathType, &length, &origin, &status); err != nil {
			t.Fatalf("SELECT path: %v", err)
		}
		if origin != "paganel" || status != "draft" {
			t.Fatalf("origin/status пути = %q/%q, want paganel/draft", origin, status)
		}
		if pathType != "trakt" || length <= 0 {
			t.Fatalf("путь = type %q length %v, want trakt и length>0", pathType, length)
		}
		if fromID == 0 || toID == 0 || fromID == toID {
			t.Fatalf("концы пути = (%d, %d), want разные известные места", fromID, toID)
		}
	})

	t.Run("промоушен переводит draft в baked", func(t *testing.T) {
		path := newRingWorld(t)
		seedHeroForge(t, path, 0, 0)
		forge := openRingForge(t, path)

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
		res, err := paganel.PromoteDrafts(forge)
		if err != nil {
			t.Fatalf("PromoteDrafts: %v", err)
		}
		if res.Promoted < 1 || res.Reforged != 0 {
			t.Fatalf("PromotionResult = %+v, want promoted≥1, reforged=0", res)
		}

		_, status, _, _ := placeStatus(t, forge, "Лес")
		if status != "baked" {
			t.Fatalf("статус после промоушена = %q, want baked", status)
		}
	})

	t.Run("запись с запретным словом в промоушене отклонена (reforge)", func(t *testing.T) {
		path := newRingWorld(t)
		seedHeroForge(t, path, 0, 0)
		forge := openRingForge(t, path)

		slot := testkit.NewFakeSlot()
		slot.SetResponse(paganel.ForgeRole, forgeOutputJSON(
			[]paganel.ForgedPlace{
				{Name: "дурак-лес", Essence: "лес", Owner: "", Prominence: "известное", Bearing: "север", DistanceDays: 1},
			},
			nil,
		))
		paganel.SetSlot(slot)
		defer paganel.SetSlot(nil)
		paganel.SetCensor(fakeCensor{forbidden: "дурак"})
		defer paganel.SetCensor(nil)

		if _, err := paganel.ForgeRing(forge); err != nil {
			t.Fatalf("ForgeRing: %v", err)
		}
		res, err := paganel.PromoteDrafts(forge)
		if err != nil {
			t.Fatalf("PromoteDrafts: %v", err)
		}
		if res.Reforged < 1 {
			t.Fatalf("PromotionResult = %+v, want reforged≥1", res)
		}

		_, status, _, _ := placeStatus(t, forge, "дурак-лес")
		if status != "reforge" {
			t.Fatalf("статус записи с запретным словом = %q, want reforge", status)
		}
	})

	t.Run("повторная ковка того же места не создаёт дубль", func(t *testing.T) {
		path := newRingWorld(t)
		seedHeroForge(t, path, 0, 0)
		forge := openRingForge(t, path)

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

		for i := 0; i < 2; i++ {
			if _, err := paganel.ForgeRing(forge); err != nil {
				t.Fatalf("ForgeRing #%d: %v", i+1, err)
			}
		}
		if n := countNamedPlaces(t, forge, "Лес"); n != 1 {
			t.Fatalf("мест с именем «Лес» = %d, want 1 (без дубля)", n)
		}
	})

	t.Run("обращений к literary.db нет", func(t *testing.T) {
		path := newRingWorld(t)
		dir := filepath.Dir(path)
		seedHeroForge(t, path, 0, 0)
		forge := openRingForge(t, path)

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
		if _, err := paganel.PromoteDrafts(forge); err != nil {
			t.Fatalf("PromoteDrafts: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "literary.db")); !os.IsNotExist(err) {
			t.Fatalf("literary.db создан/открыт ковкой: %v", err)
		}
	})
}
