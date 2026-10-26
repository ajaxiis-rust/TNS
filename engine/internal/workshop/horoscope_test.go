package workshop_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

func TestHoroscopeForge(t *testing.T) {
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

	sceneID := insertScene(t, world, "open")

	slot := testkit.NewFakeSlot()
	slot.SetResponse(workshop.HoroscopeRole, horoscopeOutput(""))
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	workshop.SetEmbedder(testkit.NewFakeEmbed())
	defer workshop.SetEmbedder(nil)

	workshop.SetCensor(fakeCensor{forbidden: "дурак"})
	defer workshop.SetCensor(nil)

	payload := workshop.HoroscopePayload{SceneID: sceneID, Pythia: "карта игрока", Forecast: "echo"}

	// Шаг 1: заказ ковки гороскопа стоит в очереди хэндлом core.
	if placed, err := workshop.EnsureHoroscopeOrder(world, payload); err != nil || !placed {
		t.Fatalf("EnsureHoroscopeOrder: placed=%v err=%v", placed, err)
	}
	assertHoroscopeQueued(t, world, payload)

	// Шаг 2–4: ковка Гефестом по заказу Пифии.
	issueID, err := workshop.RunHoroscopeForge(world, lit, payload)
	if err != nil {
		t.Fatalf("RunHoroscopeForge: %v", err)
	}
	if slot.Calls() < 1 {
		t.Fatal("ковка не вызвала слот")
	}

	// horoscope_issue: одна запись со ссылкой на сцену, три ветки и прогноз Пилы.
	assertHoroscopeIssue(t, lit, sceneID, issueID)

	// У каждой ветки есть probe_text и вектор dim=1024 в vectors.
	assertHoroscopeVectors(t, world, issueID, 3)

	// forge_log: строка horoscope со статусом done.
	assertHoroscopeLog(t, lit)

	// Запись с запретным словом не прошла Цензора: базы не изменились.
	beforeIssue := countHoroscopeIssue(t, lit)
	beforeVectors := countHoroscopeVectors(t, world, issueID)
	beforeLog := countHoroscopeLog(t, lit)

	slot.SetResponse(workshop.HoroscopeRole, horoscopeOutput("дурак"))
	if _, err := workshop.RunHoroscopeForge(world, lit, payload); err == nil {
		t.Fatal("запись с запретным словом прошла Цензора")
	}
	if got := countHoroscopeIssue(t, lit); got != beforeIssue {
		t.Fatalf("после отказа Цензора horoscope_issue: %d, want %d", got, beforeIssue)
	}
	if got := countHoroscopeVectors(t, world, issueID); got != beforeVectors {
		t.Fatalf("после отказа Цензора векторов: %d, want %d", got, beforeVectors)
	}
	if got := countHoroscopeLog(t, lit); got != beforeLog {
		t.Fatalf("после отказа Цензора forge_log: %d, want %d", got, beforeLog)
	}
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

// horoscopeOutput — строгий JSON ковки гороскопа: три ветки и прогноз Пилы.
func horoscopeOutput(forbidden string) string {
	theme := "надежда"
	if forbidden != "" {
		theme = "он " + forbidden
	}
	out := workshop.HoroscopeOutput{
		Branches: []workshop.HoroscopeBranch{
			{ID: "pos", Kind: "weaver_prompt", Ref: "продолжить тему", Theme: theme, ProbeText: "герой идёт вперёд"},
			{ID: "neg", Kind: "bank_ref", Ref: "bank://12", Theme: "опасность", ProbeText: "герой отступает"},
			{ID: "aside", Kind: "bank_ref", Ref: "bank://7", Theme: "отвлечение", ProbeText: "путник замечает птицу"},
		},
		Forecast: "echo",
	}
	raw, _ := json.Marshal(out)
	return string(raw)
}

func assertHoroscopeQueued(t *testing.T, h *db.Handle, payload workshop.HoroscopePayload) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM workshop_queue WHERE kind = 'horoscope' AND status = 'queued' AND payload_json = ?`,
		string(raw)).Scan(&n); err != nil {
		t.Fatalf("COUNT horoscope order: %v", err)
	}
	if n != 1 {
		t.Fatalf("заказов horoscope в очереди: %d, want 1", n)
	}
}

func countHoroscopeIssue(t *testing.T, lit *db.Handle) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM horoscope_issue`).Scan(&n); err != nil {
		t.Fatalf("COUNT horoscope_issue: %v", err)
	}
	return n
}

func assertHoroscopeIssue(t *testing.T, lit *db.Handle, sceneID, issueID int64) {
	t.Helper()
	if got := countHoroscopeIssue(t, lit); got != 1 {
		t.Fatalf("строк horoscope_issue: %d, want 1", got)
	}
	var gotScene int64
	var raw string
	if err := lit.QueryRow(`SELECT scene_id, issue_json FROM horoscope_issue WHERE id = ?`, issueID).Scan(&gotScene, &raw); err != nil {
		t.Fatalf("SELECT horoscope_issue: %v", err)
	}
	if gotScene != sceneID {
		t.Fatalf("horoscope_issue.scene_id = %d, want %d", gotScene, sceneID)
	}
	var out workshop.HoroscopeOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("issue_json не JSON: %v", err)
	}
	if len(out.Branches) != 3 {
		t.Fatalf("веток в гороскопе: %d, want 3", len(out.Branches))
	}
	if out.Forecast != "echo" {
		t.Fatalf("прогноз Пилы = %q, want echo", out.Forecast)
	}
	for _, b := range out.Branches {
		if b.ProbeText == "" {
			t.Fatal("ветка без probe_text")
		}
	}
}

func countHoroscopeVectors(t *testing.T, world *db.Handle, issueID int64) int {
	t.Helper()
	var n int
	if err := world.QueryRow(`SELECT COUNT(*) FROM vectors WHERE owner_table = 'horoscope_issue' AND owner_id = ?`, issueID).Scan(&n); err != nil {
		t.Fatalf("COUNT vectors horoscope: %v", err)
	}
	return n
}

func assertHoroscopeVectors(t *testing.T, world *db.Handle, issueID int64, want int) {
	t.Helper()
	if got := countHoroscopeVectors(t, world, issueID); got != want {
		t.Fatalf("векторов веток (owner_table='horoscope_issue', owner_id=%d): %d, want %d", issueID, got, want)
	}
	rows, err := world.Query(`SELECT dim, vec FROM vectors WHERE owner_table = 'horoscope_issue' AND owner_id = ?`, issueID)
	if err != nil {
		t.Fatalf("SELECT vectors horoscope: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var dim int
		var blob []byte
		if err := rows.Scan(&dim, &blob); err != nil {
			t.Fatalf("scan vectors horoscope: %v", err)
		}
		if dim != 1024 {
			t.Fatalf("вектор ветки dim=%d, want 1024", dim)
		}
		if len(blob) != 1024*4 {
			t.Fatalf("BLOB вектора ветки = %d байт, want %d", len(blob), 1024*4)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows vectors horoscope: %v", err)
	}
}

func countHoroscopeLog(t *testing.T, lit *db.Handle) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM forge_log WHERE kind = 'horoscope' AND status = 'done'`).Scan(&n); err != nil {
		t.Fatalf("COUNT forge_log horoscope: %v", err)
	}
	return n
}

func assertHoroscopeLog(t *testing.T, lit *db.Handle) {
	t.Helper()
	if got := countHoroscopeLog(t, lit); got != 1 {
		t.Fatalf("строк forge_log horoscope/done: %d, want 1", got)
	}
}
