package workshop_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/sediment"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// countingSource — ResearchSource-шпион: считает вызовы Fetch и возвращает
// заготовленный материал (локальная и внешняя реализации различаются счётчиком).
type countingSource struct {
	material string
	calls    int
}

func (s *countingSource) Fetch(_ context.Context, _ string) (string, error) {
	s.calls++
	return s.material, nil
}

// epochCensor — словарный фильтр фикстуры: текст с forbidden-словом отклоняется.
type epochCensor struct {
	forbidden string
}

func (c epochCensor) Check(text string) (bool, []string) {
	if c.forbidden != "" && strings.Contains(text, c.forbidden) {
		return false, []string{c.forbidden}
	}
	return true, nil
}

// TestEpoch — эпоха-справочник (T-05-12, «Проверка»): запись эпохи-справочника
// появляется в bank_pool только после цензуры; запись несёт trust_level; источник —
// локальная реализация ResearchSource (внешних вызовов 0); реалии для Летописца
// читаются из банка.
func TestEpoch(t *testing.T) {
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	litPath := filepath.Join(dir, "literary.db")

	newDB(t, worldPath, "world", "shared")
	newDB(t, litPath, "literary")

	seedEpochMeta(t, worldPath, "Средневековая Англия, IX век. Дороги грязные, замки каменные.")

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

	// Шаг 1: эпоха выводится кодом из описания мира в world_meta.
	epoch, err := workshop.EpochFromWorld(world)
	if err != nil {
		t.Fatalf("EpochFromWorld: %v", err)
	}
	if epoch != "Средневековая Англия, IX век" {
		t.Fatalf("эпоха = %q, want %q", epoch, "Средневековая Англия, IX век")
	}

	slot := testkit.NewFakeSlot()
	slot.SetResponse("autocritic", `{"verdict":"clean"}`)
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	workshop.SetCensor(epochCensor{forbidden: "дурак"})
	defer workshop.SetCensor(nil)

	ctx := context.Background()

	// Запись с запретным словом в банк не попадает (W-19.3, «Проверка»).
	slot.SetResponse(workshop.EpochRole, "Он сказал дурак.")
	if _, err := workshop.ForgeEpoch(ctx, world, lit, nil); !errors.Is(err, workshop.ErrCensorReject) {
		t.Fatalf("запись с запретным словом = %v, want ErrCensorReject", err)
	}
	if got := countEpochRecords(t, lit); got != 0 {
		t.Fatalf("записей справочника после отказа Цензора: %d, want 0", got)
	}

	// Ковка чистого справочника через локальную реализацию ResearchSource:
	// локальный источник вызывается один раз, внешних вызовов нет (шаги 2–3).
	localSrc := &countingSource{material: "реалии из книг-источников"}
	external := &countingSource{material: "внешняя разведка"}
	slot.SetResponse(workshop.EpochRole, "Реалии эпохи: дороги, замки, рынки.")

	id, err := workshop.ForgeEpoch(ctx, world, lit, localSrc)
	if err != nil {
		t.Fatalf("ForgeEpoch: %v", err)
	}
	if localSrc.calls != 1 {
		t.Fatalf("вызовов локального ResearchSource: %d, want 1", localSrc.calls)
	}
	if external.calls != 0 {
		t.Fatalf("внешних вызовов ResearchSource: %d, want 0", external.calls)
	}

	// Запись несёт trust_level как у банка-ресурса (шаг 5).
	if got := epochTrust(t, lit, id); got != sediment.TrustLevelVerified {
		t.Fatalf("trust_level записи справочника = %d, want %d", got, sediment.TrustLevelVerified)
	}

	// Реалии для Летописца читаются из банка, а не из головы модели (W-19.5).
	refs, err := workshop.ReadEpochReference(lit)
	if err != nil {
		t.Fatalf("ReadEpochReference: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("записей справочника для Летописца: %d, want 1", len(refs))
	}
	if refs[0].Text != "Реалии эпохи: дороги, замки, рынки." {
		t.Fatalf("текст справочника = %q, want реалии из банка", refs[0].Text)
	}
	if refs[0].Epoch != epoch {
		t.Fatalf("эпоха записи = %q, want %q", refs[0].Epoch, epoch)
	}
	if refs[0].TrustLevel != sediment.TrustLevelVerified {
		t.Fatalf("trust_level в чтении = %d, want %d", refs[0].TrustLevel, sediment.TrustLevelVerified)
	}

	// Повторная ковка той же эпохи не создаёт дубль и не перевызывает источник.
	reSrc := &countingSource{material: "ещё материал"}
	id2, err := workshop.ForgeEpoch(ctx, world, lit, reSrc)
	if err != nil {
		t.Fatalf("ForgeEpoch (повтор): %v", err)
	}
	if id2 != id {
		t.Fatalf("повторная ковка вернула id=%d, want %d", id2, id)
	}
	if reSrc.calls != 0 {
		t.Fatalf("повторная ковка вызвала источник: %d, want 0", reSrc.calls)
	}
	if got := countEpochRecords(t, lit); got != 1 {
		t.Fatalf("записей справочника после повторной ковки: %d, want 1", got)
	}
}

func seedEpochMeta(t *testing.T, worldPath, description string) {
	t.Helper()
	h, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	defer h.Close()

	raw, err := json.Marshal(description)
	if err != nil {
		t.Fatalf("marshal description: %v", err)
	}
	if _, err := h.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES ('description', ?)`, string(raw)); err != nil {
		t.Fatalf("INSERT world_meta.description: %v", err)
	}
}

func countEpochRecords(t *testing.T, lit *db.Handle) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM bank_pool WHERE tags = 'epoch'`).Scan(&n); err != nil {
		t.Fatalf("COUNT bank_pool epoch: %v", err)
	}
	return n
}

func epochTrust(t *testing.T, lit *db.Handle, id int64) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT trust_level FROM bank_pool WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("SELECT trust_level: %v", err)
	}
	return n
}
