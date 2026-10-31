package workshop_test

import (
	"context"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/structural"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// TestForgeComposition — ковка с композицией (T-10-12, «Проверка»): семья
// классифицируется кодом, остальное извлекается слотом compose, все шесть колонок
// пишутся; нарушение secondary==primary отказывает ковку; мусор слота дважды —
// family-only и строка forge_log; повтор origin — без перезаписи.
func TestForgeComposition(t *testing.T) {
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

	workshop.SetCensor(passCensor{})
	defer workshop.SetCensor(nil)

	slot := testkit.NewFakeSlot()
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	fams := composeFamilies()
	ctx := context.Background()

	// Шаг 1: классификация семьи кодом по ключевым словам.
	if got := workshop.ClassifyFamily("They had to escape, then flee, and deliver the captives.", fams); got != "escape_liberation" {
		t.Fatalf("ClassifyFamily(escape/flee/deliver) = %q, want escape_liberation", got)
	}
	if got := workshop.ClassifyFamily("обычный текст без ключевых слов", fams); got != "" {
		t.Fatalf("ClassifyFamily(без слов) = %q, want пустую строку", got)
	}

	// Шаг 3–5: валидный JSON слота — все шесть колонок записаны.
	slot.SetResponse(workshop.ComposeRole,
		`{"family_primary":"escape_liberation","pattern_secondary":"rescue","technique":"slow_revelation","moral_stance":"tragic","inversion":false}`)
	good := workshop.ForgeMaterial{
		Origin: "forge-book:comp:1",
		Kind:   "drama",
		Lang:   "ru",
		Text:   "They planned to escape and flee to deliver the captives.",
	}
	id, err := workshop.ForgeComposition(ctx, world, lit, good, fams)
	if err != nil {
		t.Fatalf("ForgeComposition(valid): %v", err)
	}
	comp := readBankComposition(t, lit, id)
	if comp.familyPrimary != "escape_liberation" || comp.patternSecondary != "rescue" ||
		comp.technique != "slow_revelation" || comp.moralStance != "tragic" ||
		comp.inversionFlag != 0 || comp.usageCount != 0 {
		t.Fatalf("композиция записи = %+v, want escape_liberation/rescue/slow_revelation/tragic/0/0", comp)
	}

	// Шаг 5: secondary == primary — отказ ковки с причиной, записи нет.
	slot.SetResponse(workshop.ComposeRole,
		`{"family_primary":"escape_liberation","pattern_secondary":"escape_liberation","technique":"slow_revelation","moral_stance":"tragic","inversion":false}`)
	bad := workshop.ForgeMaterial{
		Origin: "forge-book:comp:2",
		Kind:   "drama",
		Lang:   "ru",
		Text:   "They planned to escape and flee.",
	}
	if _, err := workshop.ForgeComposition(ctx, world, lit, bad, fams); err == nil {
		t.Fatal("ForgeComposition(secondary==primary) не отказал ковку")
	}
	if got := countBankOrigin(t, lit, bad.Origin); got != 0 {
		t.Fatalf("запись с secondary==primary попала в банк: %d строк", got)
	}

	// Шаг 3: мусор слота дважды (≤2 повтора) — family-only и строка forge_log.
	slot.SetResponse(workshop.ComposeRole, "not json")
	callsBefore := slot.Calls()
	garbage := workshop.ForgeMaterial{
		Origin: "forge-book:comp:3",
		Kind:   "drama",
		Lang:   "ru",
		Text:   "They planned to escape and flee to deliver the captives.",
	}
	id3, err := workshop.ForgeComposition(ctx, world, lit, garbage, fams)
	if err != nil {
		t.Fatalf("ForgeComposition(мусор): %v", err)
	}
	if got := slot.Calls() - callsBefore; got != 3 {
		t.Fatalf("вызовов слота на мусор = %d, want 3 (1 попытка + 2 повтора)", got)
	}
	comp3 := readBankComposition(t, lit, id3)
	if comp3.familyPrimary != "escape_liberation" || comp3.patternSecondary != "" ||
		comp3.technique != "" || comp3.moralStance != "" || comp3.inversionFlag != 0 {
		t.Fatalf("композиция family-only = %+v, want family_primary=escape_liberation и пустые остальные", comp3)
	}
	if got := countComposeLog(t, lit, "fallback"); got != 1 {
		t.Fatalf("строк forge_log compose/fallback: %d, want 1", got)
	}

	// Шаг 6: повтор origin — та же запись, поля не перезаписываются, слот не зовётся.
	callsBefore = slot.Calls()
	slot.SetResponse(workshop.ComposeRole,
		`{"family_primary":"escape_liberation","pattern_secondary":"judgment_trial","technique":"ironic_distance","moral_stance":"ironic","inversion":true}`)
	idAgain, err := workshop.ForgeComposition(ctx, world, lit, good, fams)
	if err != nil {
		t.Fatalf("ForgeComposition(повтор): %v", err)
	}
	if idAgain != id {
		t.Fatalf("повтор origin вернул id=%d, want %d", idAgain, id)
	}
	if got := slot.Calls(); got != callsBefore {
		t.Fatalf("повтор origin вызвал слот: %d → %d", callsBefore, got)
	}
	if got := countBankOrigin(t, lit, good.Origin); got != 1 {
		t.Fatalf("строк банка с origin=%q: %d, want 1", good.Origin, got)
	}
	if again := readBankComposition(t, lit, id); again != comp {
		t.Fatalf("повтор origin перезаписал поля: %+v → %+v", comp, again)
	}
}

// composeFamilies — реестр семей фикстуры: ключевые слова для классификации кода.
func composeFamilies() []structural.Family {
	return []structural.Family{
		{Name: "escape_liberation", Keywords: []string{"escape", "flee", "pass through", "deliver", "rescue", "free", "liberate", "bondage"}},
		{Name: "rescue", Keywords: []string{"save", "deliver", "oppressed", "enemy", "battle", "war", "victory"}},
		{Name: "judgment_trial", Keywords: []string{"judge", "judgment", "decide", "dispute", "trial"}},
		{Name: "loyalty", Keywords: []string{"loyal", "follow", "faithful", "serve"}},
	}
}

type bankComposition struct {
	familyPrimary    string
	patternSecondary string
	technique        string
	moralStance      string
	inversionFlag    int
	usageCount       int
}

func readBankComposition(t *testing.T, lit *db.Handle, id int64) bankComposition {
	t.Helper()
	var c bankComposition
	if err := lit.QueryRow(`SELECT family_primary, pattern_secondary, technique, moral_stance, inversion_flag, usage_count FROM bank_pool WHERE id = ?`, id).
		Scan(&c.familyPrimary, &c.patternSecondary, &c.technique, &c.moralStance, &c.inversionFlag, &c.usageCount); err != nil {
		t.Fatalf("SELECT bank_pool composition: %v", err)
	}
	return c
}

func countComposeLog(t *testing.T, lit *db.Handle, status string) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM forge_log WHERE task = 'compose' AND status = ?`, status).Scan(&n); err != nil {
		t.Fatalf("COUNT forge_log compose: %v", err)
	}
	return n
}
