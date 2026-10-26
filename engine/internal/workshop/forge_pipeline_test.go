package workshop_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/sediment"
	"tns65/internal/workshop"
)

// countingCensor — словарный фильтр фикстуры: текст с forbidden-словом
// отклоняется, каждый вызов считается (проверка «банк чистится один раз»).
type countingCensor struct {
	forbidden string
	calls     int
}

func (c *countingCensor) Check(text string) (bool, []string) {
	c.calls++
	if c.forbidden != "" && strings.Contains(text, c.forbidden) {
		return false, []string{c.forbidden}
	}
	return true, nil
}

func (c *countingCensor) Calls() int { return c.calls }

func TestForgePipeline(t *testing.T) {
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	litPath := filepath.Join(dir, "literary.db")

	newDB(t, worldPath, "world", "shared")
	newDB(t, litPath, "literary")

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

	// Мир для факт-контроля: имя и место, с которыми сверяется материал.
	seedForgeCharacter(t, world, "Иван")
	seedForgePlace(t, worldPath, "Киев")

	cen := &countingCensor{}
	workshop.SetCensor(cen)
	defer workshop.SetCensor(nil)

	// Шаги 1–3: чистая ковка — конкретные имена и места заменяются
	// слот-токенами во всех вхождениях, запись после Цензора попадает в банк.
	good := workshop.ForgeMaterial{
		Origin: "forge-book:test:1",
		Kind:   "drama",
		Lang:   "ru",
		Tags:   "scene",
		Text:   "Иван пришёл в Киев. Иван встретил друга.",
		Facts: []sediment.Fact{
			{Token: sediment.TokenName, Value: "Иван"},
			{Token: sediment.TokenPlace, Value: "Киев"},
		},
	}
	id, err := workshop.ForgeBankRecord(world, lit, good)
	if err != nil {
		t.Fatalf("ForgeBankRecord: %v", err)
	}

	text := bankText(t, lit, id)
	if strings.Contains(text, "Иван") || strings.Contains(text, "Киев") {
		t.Fatalf("конкретное имя или место попало в банк: %q", text)
	}
	if got := strings.Count(text, "{NAME}"); got != 2 {
		t.Fatalf("вхождений {NAME}: %d, want 2 (текст %q)", got, text)
	}
	if got := strings.Count(text, "{PLACE}"); got != 1 {
		t.Fatalf("вхождений {PLACE}: %d, want 1 (текст %q)", got, text)
	}
	if got := bankTrust(t, lit, id); got != sediment.TrustLevelVerified {
		t.Fatalf("trust_level ковки = %d, want %d", got, sediment.TrustLevelVerified)
	}
	if got := cen.Calls(); got != 1 {
		t.Fatalf("вызовов Цензора после первой ковки: %d, want 1", got)
	}

	// Повторная выдача той же записи Цензора заново не запускает.
	calls := cen.Calls()
	id2, err := workshop.ForgeBankRecord(world, lit, good)
	if err != nil {
		t.Fatalf("ForgeBankRecord (повтор): %v", err)
	}
	if id2 != id {
		t.Fatalf("повторная выдача вернула id=%d, want %d", id2, id)
	}
	if got := countBankOrigin(t, lit, good.Origin); got != 1 {
		t.Fatalf("строк банка с origin=%q: %d, want 1", good.Origin, got)
	}
	if got := cen.Calls(); got != calls {
		t.Fatalf("повторная выдача вызвала Цензора: %d → %d", calls, got)
	}

	// Запись с запретным словом в банк не попадает.
	cen.forbidden = "дурак"
	bad := workshop.ForgeMaterial{
		Origin: "forge-book:test:2",
		Kind:   "drama",
		Lang:   "ru",
		Text:   "он дурак",
	}
	if _, err := workshop.ForgeBankRecord(world, lit, bad); err == nil {
		t.Fatal("запись с запретным словом прошла Цензора")
	}
	if got := countBankOrigin(t, lit, bad.Origin); got != 0 {
		t.Fatalf("запретная запись попала в банк: %d строк", got)
	}
	cen.forbidden = ""

	// Расхождение фактов помечается в forge_log (шаг 1).
	mismatch := workshop.ForgeMaterial{
		Origin: "forge-book:test:3",
		Kind:   "drama",
		Lang:   "ru",
		Text:   "Пётр шёл.",
		Facts:  []sediment.Fact{{Token: sediment.TokenName, Value: "Пётр"}},
	}
	if _, err := workshop.ForgeBankRecord(world, lit, mismatch); err != nil {
		t.Fatalf("ForgeBankRecord (расхождение): %v", err)
	}
	if got := countFactMismatch(t, lit); got != 1 {
		t.Fatalf("строк forge_log fact_check/mismatch: %d, want 1", got)
	}

	// Переплавка осевшей записи (шаг 4): trust_level=1 → bank_pool trust_level=2.
	sceneID := seedSedimentScene(t, litPath, 100, "{NAME} вошёл в {PLACE}.", sediment.TrustLevelUnchecked)
	remeltID, err := workshop.RemeltScene(lit, sceneID)
	if err != nil {
		t.Fatalf("RemeltScene: %v", err)
	}
	if got := bankTrust(t, lit, remeltID); got != sediment.TrustLevelVerified {
		t.Fatalf("trust_level переплавки = %d, want %d", got, sediment.TrustLevelVerified)
	}
	if got := bankText(t, lit, remeltID); got != "{NAME} вошёл в {PLACE}." {
		t.Fatalf("текст переплавки = %q, want слотную заготовку", got)
	}

	// Переплавка записи не с trust_level=1 отклоняется: путь наверх один.
	sceneID2 := seedSedimentScene(t, litPath, 101, "{NAME} спит.", sediment.TrustLevelSettled)
	if _, err := workshop.RemeltScene(lit, sceneID2); !errors.Is(err, workshop.ErrRemeltNotUnchecked) {
		t.Fatalf("RemeltScene(trust_level=0) = %v, want ErrRemeltNotUnchecked", err)
	}
}

func seedForgeCharacter(t *testing.T, world *db.Handle, name string) {
	t.Helper()
	if _, err := world.Exec("characters",
		`INSERT INTO characters (name, psychotype, estate, card_json) VALUES (?, '', '', '{}')`, name); err != nil {
		t.Fatalf("INSERT characters: %v", err)
	}
}

func seedForgePlace(t *testing.T, worldPath, name string) {
	t.Helper()
	h, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	defer h.Close()
	if _, err := h.Exec("places",
		`INSERT INTO places (name, kind) VALUES (?, 'city')`, name); err != nil {
		t.Fatalf("INSERT places: %v", err)
	}
}

func seedSedimentScene(t *testing.T, litPath string, sourceSceneID int64, cleanedText string, trustLevel int) int64 {
	t.Helper()
	h, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterSediment, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(sediment): %v", err)
	}
	defer h.Close()

	res, err := h.Exec("sediment_scene",
		`INSERT INTO sediment_scene (source_scene_id, cleaned_text, vector_id, verdict, trust_level, created_at) VALUES (?, ?, NULL, 'clean', ?, '2026-01-01T00:00:00Z')`,
		sourceSceneID, cleanedText, trustLevel)
	if err != nil {
		t.Fatalf("INSERT sediment_scene: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("sediment_scene id: %v", err)
	}
	return id
}

func countBankOrigin(t *testing.T, lit *db.Handle, origin string) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM bank_pool WHERE origin = ?`, origin).Scan(&n); err != nil {
		t.Fatalf("COUNT bank_pool origin: %v", err)
	}
	return n
}

func bankText(t *testing.T, lit *db.Handle, id int64) string {
	t.Helper()
	var s string
	if err := lit.QueryRow(`SELECT text FROM bank_pool WHERE id = ?`, id).Scan(&s); err != nil {
		t.Fatalf("SELECT bank_pool text: %v", err)
	}
	return s
}

func bankTrust(t *testing.T, lit *db.Handle, id int64) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT trust_level FROM bank_pool WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("SELECT bank_pool trust_level: %v", err)
	}
	return n
}

func countFactMismatch(t *testing.T, lit *db.Handle) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM forge_log WHERE task = 'fact_check' AND status = 'mismatch'`).Scan(&n); err != nil {
		t.Fatalf("COUNT forge_log fact_check: %v", err)
	}
	return n
}
