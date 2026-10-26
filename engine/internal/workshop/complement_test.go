package workshop_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

func TestComplement(t *testing.T) {
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	litPath := filepath.Join(dir, "literary.db")

	newDB(t, worldPath, "world", "shared")
	newDB(t, litPath, "literary")

	arcs, err := workshop.LoadArcs("../birth/data/arcs.json")
	if err != nil {
		t.Fatalf("LoadArcs: %v", err)
	}
	if len(arcs) != 12 {
		t.Fatalf("арок в реестре: %d, want 12", len(arcs))
	}
	arcA := arcs[0]
	arcB := arcs[1]

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

	// При нехватке карточки под арку ставится заказ complement.
	scene1 := insertScene(t, world, "open")
	placed, err := workshop.EnsureComplementOrder(world, arcA, scene1, workshop.PauseHeavy)
	if err != nil {
		t.Fatalf("EnsureComplementOrder: %v", err)
	}
	if !placed {
		t.Fatal("заказ complement не поставлен при нехватке карточки")
	}
	assertComplementQueued(t, world, arcA.Arc, scene1, true)

	// Закрытие сцены-источника снимает заказ и не добавляет карточку.
	before := countCharacters(t, world)
	closeScene(t, world, scene1)
	if err := workshop.ReconcileComplement(world, arcs); err != nil {
		t.Fatalf("ReconcileComplement: %v", err)
	}
	assertComplementQueued(t, world, arcA.Arc, scene1, false)
	if got := countCharacters(t, world); got != before {
		t.Fatalf("после снятия заказа карточек: %d, want %d", got, before)
	}

	// Покрытие арки подходящей карточкой тоже снимает заказ.
	scene2 := insertScene(t, world, "open")
	if _, err := workshop.EnsureComplementOrder(world, arcA, scene2, workshop.PauseHeavy); err != nil {
		t.Fatalf("EnsureComplementOrder (повтор): %v", err)
	}
	insertCard(t, world, arcA.Archetypes[0])
	before2 := countCharacters(t, world)
	if err := workshop.ReconcileComplement(world, arcs); err != nil {
		t.Fatalf("ReconcileComplement (арка покрыта): %v", err)
	}
	assertComplementQueued(t, world, arcA.Arc, scene2, false)
	if got := countCharacters(t, world); got != before2 {
		t.Fatalf("после покрытия арки карточек: %d, want %d", got, before2)
	}

	// Исполнение: ковка идёт через swarm-задачу (фейковый слот) и пишет карточку.
	scene3 := insertScene(t, world, "open")
	if _, err := workshop.EnsureComplementOrder(world, arcB, scene3, workshop.PauseHeavy); err != nil {
		t.Fatalf("EnsureComplementOrder (исполнение): %v", err)
	}

	slot := testkit.NewFakeSlot()
	slot.SetResponse(workshop.ForgeCardRole, validCardJSON(arcB.Archetypes[0]))
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	before3 := countCharacters(t, world)
	cardJSON, err := workshop.RunForgeCard(lit, workshop.ComplementPayload{Arc: arcB.Arc, SceneID: scene3})
	if err != nil {
		t.Fatalf("RunForgeCard: %v", err)
	}
	if slot.Calls() < 1 {
		t.Fatal("ковка не вызвала слот (swarm-задачу)")
	}
	if err := workshop.WriteComplementCard(world, cardJSON); err != nil {
		t.Fatalf("WriteComplementCard: %v", err)
	}
	if got := countCharacters(t, world); got != before3+1 {
		t.Fatalf("карточек после исполнения: %d, want %d", got, before3+1)
	}
	checkCardLayers(t, lastCard(t, world))

	// Задача ковки оставила артефакт и строку forge_log (писатель forge).
	assertForgeArtifact(t, lit)
}

func newDB(t *testing.T, path string, dirs ...string) {
	t.Helper()
	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter(%s): %v", path, err)
	}
	defer w.Close()
	if err := db.Migrate(w, dirs...); err != nil {
		t.Fatalf("Migrate(%s, %v): %v", path, dirs, err)
	}
}

func insertScene(t *testing.T, h *db.Handle, status string) int64 {
	t.Helper()
	res, err := h.Exec("scenes",
		`INSERT INTO scenes (session_id, day, status, prose) VALUES (0, 1, ?, '')`, status)
	if err != nil {
		t.Fatalf("INSERT scenes: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("scenes id: %v", err)
	}
	return id
}

func closeScene(t *testing.T, h *db.Handle, sceneID int64) {
	t.Helper()
	if _, err := h.Exec("scenes", `UPDATE scenes SET status = 'closed' WHERE id = ?`, sceneID); err != nil {
		t.Fatalf("UPDATE scenes: %v", err)
	}
}

func countCharacters(t *testing.T, h *db.Handle) int {
	t.Helper()
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM characters`).Scan(&n); err != nil {
		t.Fatalf("COUNT characters: %v", err)
	}
	return n
}

func insertCard(t *testing.T, h *db.Handle, archetype string) {
	t.Helper()
	raw := `{"archetype":"` + archetype + `"}`
	if _, err := h.Exec("characters",
		`INSERT INTO characters (name, psychotype, estate, card_json) VALUES ('', '', '', ?)`, raw); err != nil {
		t.Fatalf("INSERT characters: %v", err)
	}
}

func lastCard(t *testing.T, h *db.Handle) string {
	t.Helper()
	var raw string
	if err := h.QueryRow(`SELECT card_json FROM characters ORDER BY id DESC LIMIT 1`).Scan(&raw); err != nil {
		t.Fatalf("SELECT last card: %v", err)
	}
	return raw
}

func assertComplementQueued(t *testing.T, h *db.Handle, arc string, sceneID int64, want bool) {
	t.Helper()
	raw, err := json.Marshal(workshop.ComplementPayload{Arc: arc, SceneID: sceneID})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM workshop_queue WHERE kind = 'complement' AND status = 'queued' AND payload_json = ?`,
		string(raw)).Scan(&n); err != nil {
		t.Fatalf("COUNT complement order: %v", err)
	}
	if want && n != 1 {
		t.Fatalf("заказ complement (arc=%s, scene=%d) не в очереди: %d", arc, sceneID, n)
	}
	if !want && n != 0 {
		t.Fatalf("заказ complement (arc=%s, scene=%d) остался в очереди", arc, sceneID)
	}
}

func assertForgeArtifact(t *testing.T, lit *db.Handle) {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM forge_log WHERE kind = 'forge-card' AND status = 'done'`).Scan(&n); err != nil {
		t.Fatalf("COUNT forge_log: %v", err)
	}
	if n != 1 {
		t.Fatalf("строк forge_log forge-card: %d, want 1", n)
	}
	var a int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM artifact WHERE kind = 'forge-card'`).Scan(&a); err != nil {
		t.Fatalf("COUNT artifact: %v", err)
	}
	if a != 1 {
		t.Fatalf("артефактов forge-card: %d, want 1", a)
	}
}

func validCardJSON(archetype string) string {
	return `{"psychotype":"ENTJ","estate":"ремесленник","archetype":"` + archetype + `",` +
		`"voice":"тихий","appearance":"неприметный","weight":1.0,` +
		`"layers":{"state":{"health":100,"mood":"спокойное",` +
		`"skills":{"combat":5,"athletics":5,"perception":5,"stealth":5,"craft":5,"speech":5,"lore":5,"survival":5,"medicine":5},` +
		`"memory":{"halfLifeDays":7,"similarityThreshold":0.7,"maxMemoryEntries":10000}},` +
		`"economy":{"stats":{"strength":5,"dexterity":5,"constitution":5,"intelligence":5,"wisdom":5,"charisma":5},` +
		`"vices":{"pride":0,"greed":0,"lust":0,"envy":0,"gluttony":0,"sloth":0,"cowardice":0,"deceit":0},` +
		`"temperament":{"sanguine":0.25,"choleric":0.25,"melancholic":0.25,"phlegmatic":0.25},"loyalty":0,"family":""},` +
		`"hidden":{"psychotype":"ENTJ","motives":[]}}}`
}

func checkCardLayers(t *testing.T, raw string) {
	t.Helper()
	var card map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &card); err != nil {
		t.Fatalf("card json: %v", err)
	}
	layersRaw, ok := card["layers"]
	if !ok {
		t.Fatal("карточка без ключа layers")
	}
	var layers map[string]json.RawMessage
	if err := json.Unmarshal(layersRaw, &layers); err != nil {
		t.Fatalf("layers: %v", err)
	}
	for _, key := range []string{"state", "economy", "hidden"} {
		if _, ok := layers[key]; !ok {
			t.Fatalf("нет слоя %q", key)
		}
	}

	var state struct {
		Health int            `json:"health"`
		Skills map[string]int `json:"skills"`
		Memory struct {
			HalfLifeDays        float64 `json:"halfLifeDays"`
			SimilarityThreshold float64 `json:"similarityThreshold"`
			MaxMemoryEntries    int     `json:"maxMemoryEntries"`
		} `json:"memory"`
	}
	if err := json.Unmarshal(layers["state"], &state); err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.Health < 0 || state.Health > 100 {
		t.Fatalf("здоровье вне 0–100: %d", state.Health)
	}
	if got := len(state.Skills); got != 9 {
		t.Fatalf("навыков: %d, want 9", got)
	}
	if state.Memory.HalfLifeDays <= 0 || state.Memory.SimilarityThreshold <= 0 || state.Memory.MaxMemoryEntries <= 0 {
		t.Fatal("параметры памяти не заполнены")
	}

	var economy struct {
		Stats       map[string]int     `json:"stats"`
		Vices       map[string]int     `json:"vices"`
		Temperament map[string]float64 `json:"temperament"`
	}
	if err := json.Unmarshal(layers["economy"], &economy); err != nil {
		t.Fatalf("economy: %v", err)
	}
	if got := len(economy.Stats); got != 6 {
		t.Fatalf("статов: %d, want 6", got)
	}
	if got := len(economy.Vices); got != 8 {
		t.Fatalf("пороков: %d, want 8", got)
	}
	if got := len(economy.Temperament); got != 4 {
		t.Fatalf("гуморов: %d, want 4", got)
	}

	var hidden struct {
		Psychotype string   `json:"psychotype"`
		Motives    []string `json:"motives"`
	}
	if err := json.Unmarshal(layers["hidden"], &hidden); err != nil {
		t.Fatalf("hidden: %v", err)
	}
	if hidden.Psychotype == "" {
		t.Fatal("скрытый слой без психотипа")
	}
	if hidden.Motives == nil {
		t.Fatal("скрытый слой без явных пустых мотивов")
	}
}
