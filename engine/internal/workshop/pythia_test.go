package workshop_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

func TestPythia(t *testing.T) {
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	litPath := filepath.Join(dir, "literary.db")

	newDB(t, worldPath, "world", "shared")
	newDB(t, litPath, "literary")

	// Стартовые данные карты — world_meta.hero_birth (писатель forge).
	seedPythiaMeta(t, worldPath, "простолюдин")

	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer lit.Close()

	// Пачка: карточка героя (id 0) с психотипом и паттерны прошедших сессий.
	seedPythiaHeroCard(t, world, "ENTJ")
	seedPythiaTurns(t, world)

	const day = 3

	slot := testkit.NewFakeSlot()
	slot.SetResponse("pythia", pythiaOutput("INTP"))
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	if err := workshop.RunPythia(world, lit, day); err != nil {
		t.Fatalf("RunPythia: %v", err)
	}
	if slot.Calls() < 1 {
		t.Fatal("ночной прогон не вызвал слот")
	}

	// После ночного прогона player_atlas пополнен строками интересов.
	if got := countAtlasTheme(t, world, day, "interest"); got != 2 {
		t.Fatalf("строк интересов в player_atlas: %d, want 2", got)
	}

	// Психотип в пачке не пересчитан, а возвращённый моделью психотип
	// в player_atlas не попал.
	assertPythiaPsychotype(t, world, "ENTJ")
	assertNoPsychotypeInAtlas(t, world, "INTP")

	// В forge_log одна запись pythia со статусом done.
	assertPythiaLog(t, lit)

	// Повторный прогон того же дня не добавил строк и не вызвал слот.
	before := countAtlasDay(t, world, day)
	beforeLog := countPythiaLog(t, lit)
	beforeCalls := slot.Calls()
	if err := workshop.RunPythia(world, lit, day); err != nil {
		t.Fatalf("RunPythia (повтор): %v", err)
	}
	if got := countAtlasDay(t, world, day); got != before {
		t.Fatalf("повторный прогон добавил строк player_atlas: %d → %d", before, got)
	}
	if got := countPythiaLog(t, lit); got != beforeLog {
		t.Fatalf("повторный прогон добавил записей forge_log: %d → %d", beforeLog, got)
	}
	if got := slot.Calls(); got != beforeCalls {
		t.Fatalf("повторный прогон вызвал слот: %d → %d", beforeCalls, got)
	}
}

func seedPythiaMeta(t *testing.T, worldPath, heroBirth string) {
	t.Helper()
	h, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	defer h.Close()

	raw, err := json.Marshal(heroBirth)
	if err != nil {
		t.Fatalf("marshal hero_birth: %v", err)
	}
	if _, err := h.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES ('hero_birth', ?)`, string(raw)); err != nil {
		t.Fatalf("INSERT world_meta.hero_birth: %v", err)
	}
}

func seedPythiaHeroCard(t *testing.T, h *db.Handle, psychotype string) {
	t.Helper()
	if _, err := h.Exec("characters",
		`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES (0, '', ?, '', '{}')`, psychotype); err != nil {
		t.Fatalf("INSERT characters (hero): %v", err)
	}
}

func seedPythiaTurns(t *testing.T, h *db.Handle) {
	t.Helper()
	for _, text := range []string{"хочу домой", "люблю море"} {
		if _, err := h.Exec("turns",
			`INSERT INTO turns (session_id, player_text, reply_text, saw_recommendation, trust_level, latency_ms, degraded) VALUES (0, ?, '', 0, 0, 0, 0)`, text); err != nil {
			t.Fatalf("INSERT turns: %v", err)
		}
	}
}

func pythiaOutput(psychotype string) string {
	return `{"mood":"спокойное","interests":["семья","рыбалка"],"habits":["прогулки"],"avoid":["насилие"],"psychotype":"` + psychotype + `"}`
}

func countAtlasTheme(t *testing.T, h *db.Handle, day int, theme string) int {
	t.Helper()
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM player_atlas WHERE day = ? AND theme = ?`, day, theme).Scan(&n); err != nil {
		t.Fatalf("COUNT player_atlas theme: %v", err)
	}
	return n
}

func countAtlasDay(t *testing.T, h *db.Handle, day int) int {
	t.Helper()
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM player_atlas WHERE day = ?`, day).Scan(&n); err != nil {
		t.Fatalf("COUNT player_atlas day: %v", err)
	}
	return n
}

func countPythiaLog(t *testing.T, h *db.Handle) int {
	t.Helper()
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM forge_log WHERE kind = 'pythia' AND status = 'done'`).Scan(&n); err != nil {
		t.Fatalf("COUNT forge_log pythia: %v", err)
	}
	return n
}

func assertPythiaPsychotype(t *testing.T, h *db.Handle, want string) {
	t.Helper()
	var got string
	if err := h.QueryRow(`SELECT psychotype FROM characters WHERE id = 0`).Scan(&got); err != nil {
		t.Fatalf("SELECT hero psychotype: %v", err)
	}
	if got != want {
		t.Fatalf("психотип героя = %q, want %q", got, want)
	}
}

func assertNoPsychotypeInAtlas(t *testing.T, h *db.Handle, psychotype string) {
	t.Helper()
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM player_atlas WHERE mood = ? OR theme = ? OR interest = ?`,
		psychotype, psychotype, psychotype).Scan(&n); err != nil {
		t.Fatalf("COUNT psychotype in player_atlas: %v", err)
	}
	if n != 0 {
		t.Fatalf("психотип %q попал в player_atlas", psychotype)
	}
}

func assertPythiaLog(t *testing.T, h *db.Handle) {
	t.Helper()
	if got := countPythiaLog(t, h); got != 1 {
		t.Fatalf("записей forge_log pythia/done: %d, want 1", got)
	}
}
