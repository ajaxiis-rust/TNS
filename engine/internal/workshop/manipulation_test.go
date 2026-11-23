package workshop_test

import (
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// TestManipulationDetect — «Проверка» T-19-04: детект манипуляции (W-13.7) —
// резкий сдвиг паттернов против самопрезентации, поведение главнее слов.
func TestManipulationDetect(t *testing.T) {
	cfg := workshop.DefaultManipulationConfig()

	t.Run("резкий сдвиг", func(t *testing.T) {
		turns := []string{
			"купил хлеб на рынке",
			"приготовил ужин",
			"сходил на торг за солью",
			"починил печь во дворе",
			"пишу сенсорные стихи о запахе шёлка",
			"люблю бархат и нежный шёпот",
			"трепет от аромата полевых трав",
			"шёпот ветра и запах трав",
		}
		rep := workshop.DetectManipulation(turns, cfg)
		if !rep.Detected {
			t.Fatalf("резкий сдвиг не детектирован: %+v", rep)
		}
		if rep.MaxShift <= rep.Alpha {
			t.Fatalf("сдвиг %.2f не выше порога %.2f", rep.MaxShift, rep.Alpha)
		}
		if rep.Pattern != "сенсорика" {
			t.Fatalf("грань сдвига = %q, want %q", rep.Pattern, "сенсорика")
		}
		if rep.Behavior != "сенсорика" {
			t.Fatalf("поведение = %q, want %q", rep.Behavior, "сенсорика")
		}
	})

	t.Run("плавный сдвиг", func(t *testing.T) {
		// По одной сенсорной реплике на блок: дрейф внутри окна, шаг ниже порога.
		turns := []string{
			"купил хлеб на рынке",
			"приготовил ужин",
			"починил печь во дворе",
			"сходил на торг за солью",
			"занялся огородом и хозяйством",
			"убрал двор",
			"купил хлеб на рынке",
			"написал пару стихов о запахе шёлка",
			"приготовил ужин",
			"починил печь во дворе",
			"люблю бархат и нежный шёпот",
			"трепет от аромата полевых трав",
			"сходил на торг за солью",
			"пишу сенсорные стихи о запахе шёлка",
			"люблю бархат и нежный шёпот",
			"шёпот ветра и запах трав",
		}
		rep := workshop.DetectManipulation(turns, cfg)
		if rep.Detected {
			t.Fatalf("плавный сдвиг принят за манипуляцию: %+v", rep)
		}
		if rep.MaxShift > rep.Alpha {
			t.Fatalf("плавный сдвиг %.2f выше порога %.2f", rep.MaxShift, rep.Alpha)
		}
	})

	t.Run("поведение главнее слов", func(t *testing.T) {
		// Самопрезентация «технократ» во второй четверти, поведение — сенсорика.
		turns := []string{
			"купил хлеб на рынке",
			"приготовил ужин",
			"сходил на торг за солью",
			"я — технократ, человек системы",
			"по натуре я инженер, считаю себя человеком схем",
			"починил печь во дворе",
			"занялся огородом и хозяйством",
			"убрал двор",
			"купил хлеб на рынке",
			"пишу сенсорные стихи о запахе шёлка",
			"люблю бархат и нежный шёпот",
			"трепет от аромата полевых трав",
		}
		rep := workshop.DetectManipulation(turns, cfg)
		if !rep.Detected {
			t.Fatalf("сдвиг против самопрезентации не детектирован: %+v", rep)
		}
		if rep.Claimed != "техника" {
			t.Fatalf("самопрезентация = %q, want %q", rep.Claimed, "техника")
		}
		if rep.Behavior != "сенсорика" {
			t.Fatalf("поведение = %q, want %q", rep.Behavior, "сенсорика")
		}
		if !rep.BehaviorWins {
			t.Fatalf("поведение не победило слова: %+v", rep)
		}
	})

	t.Run("карта и журнал", func(t *testing.T) {
		dir := t.TempDir()
		worldPath := filepath.Join(dir, "world(a).db")
		litPath := filepath.Join(dir, "literary.db")
		newDB(t, worldPath, "world", "shared")
		newDB(t, litPath, "literary")

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

		seedPythiaHeroCard(t, world, "ENTJ")
		seedManipulationTurns(t, world, []string{
			"купил хлеб на рынке",
			"приготовил ужин",
			"сходил на торг за солью",
			"я — технократ, человек системы",
			"по натуре я инженер, считаю себя человеком схем",
			"починил печь во дворе",
			"занялся огородом и хозяйством",
			"убрал двор",
			"купил хлеб на рынке",
			"пишу сенсорные стихи о запахе шёлка",
			"люблю бархат и нежный шёпот",
			"трепет от аромата полевых трав",
		})

		const day = 4
		slot := testkit.NewFakeSlot()
		slot.SetResponse("pythia", pythiaOutput("INTP"))
		workshop.SetSlot(slot)
		defer workshop.SetSlot(nil)

		if err := workshop.RunPythia(world, lit, day); err != nil {
			t.Fatalf("RunPythia: %v", err)
		}

		// Пометка в карте игрока: интерпретация и метрика сдвига, без психотипа.
		if got := countAtlasTheme(t, world, day, "manipulation"); got != 1 {
			t.Fatalf("строк пометки манипуляции: %d, want 1", got)
		}
		var mark, markPsychotype string
		if err := world.QueryRow(`SELECT interest, psychotype FROM player_atlas WHERE day = ? AND theme = 'manipulation'`, day).
			Scan(&mark, &markPsychotype); err != nil {
			t.Fatalf("SELECT пометку player_atlas: %v", err)
		}
		for _, want := range []string{"поведение главнее слов", "сенсорика", "заявлено"} {
			if !strings.Contains(mark, want) {
				t.Fatalf("пометка карты %q не содержит %q", mark, want)
			}
		}
		if markPsychotype != "" {
			t.Fatalf("пометка манипуляции несёт психотип %q, want пусто", markPsychotype)
		}

		// Журнал Пифии: запись о сработавшем детекте.
		if got := countManipulationLog(t, lit); got != 1 {
			t.Fatalf("записей forge_log manipulation/done: %d, want 1", got)
		}
		if got := countPythiaLog(t, lit); got != 1 {
			t.Fatalf("записей forge_log pythia/done: %d, want 1", got)
		}

		// W-12.4 держится: психотип пачки не пересчитан, психотип модели не лёг.
		assertPythiaPsychotype(t, world, "ENTJ")
		assertAtlasPsychotype(t, world, "ENTJ")
		if got := countAtlasPsychotype(t, world, "INTP"); got != 0 {
			t.Fatalf("психотип модели %q лёг в карту: %d строк", "INTP", got)
		}
	})
}

// seedManipulationTurns кладёт окно реплик игрока в порядке хода.
func seedManipulationTurns(t *testing.T, h *db.Handle, turns []string) {
	t.Helper()
	for _, text := range turns {
		if _, err := h.Exec("turns",
			`INSERT INTO turns (session_id, player_text, reply_text, saw_recommendation, trust_level, latency_ms, degraded) VALUES (0, ?, '', 0, 0, 0, 0)`, text); err != nil {
			t.Fatalf("INSERT turns: %v", err)
		}
	}
}

// countManipulationLog — число записей детекта манипуляции в журнале Пифии.
func countManipulationLog(t *testing.T, h *db.Handle) int {
	t.Helper()
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM forge_log WHERE kind = 'manipulation' AND status = 'done'`).Scan(&n); err != nil {
		t.Fatalf("COUNT forge_log manipulation: %v", err)
	}
	return n
}
