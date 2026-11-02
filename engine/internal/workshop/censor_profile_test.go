package workshop_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// seedCensorship пишет рейтинг цензуры в world_meta.censorship (писатель forge,
// значение — JSON-строка, T-02-03, шаг 3).
func seedCensorship(t *testing.T, worldPath, rating string) {
	t.Helper()
	h, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	defer h.Close()
	raw, err := json.Marshal(rating)
	if err != nil {
		t.Fatalf("marshal rating: %v", err)
	}
	if _, err := h.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES ('censorship', ?)`, string(raw)); err != nil {
		t.Fatalf("INSERT world_meta.censorship: %v", err)
	}
}

func openForgeWorld(t *testing.T, worldPath string) *db.Handle {
	t.Helper()
	h, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func openForgeLiterary(t *testing.T, litPath string) *db.Handle {
	t.Helper()
	h, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// TestWorldRating — рейтинг читается из world_meta.censorship; отсутствие записи
// даёт консервативный дефолт 16+ (T-12-07).
func TestWorldRating(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   string // пусто — записи нет
		expect string
	}{
		{"16+", workshop.Rating16Plus, workshop.Rating16Plus},
		{"18+", workshop.Rating18Plus, workshop.Rating18Plus},
		{"отсутствует", "", workshop.Rating16Plus},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			worldPath := filepath.Join(dir, "world(a).db")
			newDB(t, worldPath, "world", "shared")
			if tc.seed != "" {
				seedCensorship(t, worldPath, tc.seed)
			}
			world := openForgeWorld(t, worldPath)
			got, err := workshop.WorldRating(world)
			if err != nil {
				t.Fatalf("WorldRating: %v", err)
			}
			if got != tc.expect {
				t.Fatalf("WorldRating = %q, want %q", got, tc.expect)
			}
		})
	}
}

// TestProviderForRating — 18+ сводит любой пресет к локальному; 16+ оставляет
// запрошенный (T-12-07, «Проверка»).
func TestProviderForRating(t *testing.T) {
	for _, requested := range []workshop.Provider{
		workshop.ProviderGeminiFlash,
		workshop.ProviderGeminiPro,
		workshop.ProviderLocal,
	} {
		if got := workshop.ProviderForRating(workshop.Rating18Plus, requested); got != workshop.ProviderLocal {
			t.Fatalf("18+: ProviderForRating(%q) = %q, want local", requested, got)
		}
	}
	if got := workshop.ProviderForRating(workshop.Rating16Plus, workshop.ProviderGeminiFlash); got != workshop.ProviderGeminiFlash {
		t.Fatalf("16+: ProviderForRating(gemini-flash) = %q, want gemini-flash", got)
	}
}

// TestProviderForWorldRating18RestrictsToLocal — тест с фейковым слотом: при 18+
// внешний провайдер не выбирается (сводится к локальному), T-12-07.
func TestProviderForWorldRating18RestrictsToLocal(t *testing.T) {
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	newDB(t, worldPath, "world", "shared")
	seedCensorship(t, worldPath, workshop.Rating18Plus)
	world := openForgeWorld(t, worldPath)

	slot := testkit.NewFakeSlot()
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	for _, requested := range []workshop.Provider{
		workshop.ProviderGeminiFlash,
		workshop.ProviderGeminiPro,
		workshop.ProviderLocal,
	} {
		got, err := workshop.ProviderForWorld(world, requested)
		if err != nil {
			t.Fatalf("ProviderForWorld(%q): %v", requested, err)
		}
		if got != workshop.ProviderLocal {
			t.Fatalf("18+: ProviderForWorld(%q) = %q, want local (внешний провайдер выбран)", requested, got)
		}
	}
}

// TestRunHoroscopeForgeRating18UsesLocalSlot — при 18+ ковка гороскопа идёт
// через локальный слот: фейковый слот вызывается, внешний пресет не поднимается.
func TestRunHoroscopeForgeRating18UsesLocalSlot(t *testing.T) {
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	litPath := filepath.Join(dir, "literary.db")
	newDB(t, worldPath, "world", "shared")
	newDB(t, litPath, "literary")
	seedCensorship(t, worldPath, workshop.Rating18Plus)

	world := openForgeWorld(t, worldPath)
	lit := openForgeLiterary(t, litPath)
	sceneID := insertScene(t, world, "open")

	slot := testkit.NewFakeSlot()
	slot.SetResponse(workshop.HoroscopeRole, horoscopeOutput(""))
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)
	workshop.SetEmbedder(testkit.NewFakeEmbed())
	defer workshop.SetEmbedder(nil)
	workshop.SetCensor(fakeCensor{})
	defer workshop.SetCensor(nil)

	payload := workshop.HoroscopePayload{SceneID: sceneID, Pythia: "карта игрока", Forecast: "echo"}
	if _, err := workshop.RunHoroscopeForge(world, lit, payload); err != nil {
		t.Fatalf("RunHoroscopeForge: %v", err)
	}
	if slot.Calls() < 1 {
		t.Fatal("18+: ковка не выбрала локальный слот")
	}
}

// TestForgeBankRecordRatingDictionary — 16+ расширяет словарь файлом
// stoplist_16.txt: слово «дурак» (только в 16+) отклоняется при 16+ и проходит
// при 18+ (база его не содержит), T-12-07.
func TestForgeBankRecordRatingDictionary(t *testing.T) {
	if lemmatizerBinary(t) == "" {
		t.Skip("бинарник лемматизатора недоступен — реальный словарь не проверяется")
	}

	material := func(origin string) workshop.ForgeMaterial {
		return workshop.ForgeMaterial{Origin: origin, Kind: "drama", Lang: "ru", Text: "он дурак"}
	}

	// 16+: «дурак» только в stoplist_16 — расширенный словарь отклоняет.
	dir16 := t.TempDir()
	world16 := filepath.Join(dir16, "world(a).db")
	lit16 := filepath.Join(dir16, "literary.db")
	newDB(t, world16, "world", "shared")
	newDB(t, lit16, "literary")
	seedCensorship(t, world16, workshop.Rating16Plus)
	if _, err := workshop.ForgeBankRecord(openForgeWorld(t, world16), openForgeLiterary(t, lit16), material("forge:16")); !errors.Is(err, workshop.ErrCensorReject) {
		t.Fatalf("16+: «дурак» = %v, want ErrCensorReject", err)
	}

	// 18+: база не содержит «дурак» — запись проходит.
	dir18 := t.TempDir()
	world18 := filepath.Join(dir18, "world(a).db")
	lit18 := filepath.Join(dir18, "literary.db")
	newDB(t, world18, "world", "shared")
	newDB(t, lit18, "literary")
	seedCensorship(t, world18, workshop.Rating18Plus)
	if _, err := workshop.ForgeBankRecord(openForgeWorld(t, world18), openForgeLiterary(t, lit18), material("forge:18")); err != nil {
		t.Fatalf("18+: «дурак» = %v, want ok", err)
	}
}

// lemmatizerBinary возвращает путь бинарника лемматизатора пакета ru или пустую
// строку, если он недоступен (для golden-проверки реального словаря).
func lemmatizerBinary(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		p := filepath.Join(dir, "bin", "tns65-lemmaru")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
