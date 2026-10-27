package book

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/journal"
	"tns65/internal/testkit"
)

// countJournalEvents считает строки журнала с заданным событием.
func countJournalEvents(t *testing.T, logPath, event string) int {
	t.Helper()
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile журнала: %v", err)
	}
	return strings.Count(string(b), `"event":"`+event+`"`)
}

// TestDeepRead — «Проверка» T-07-03.
func TestDeepRead(t *testing.T) {
	cfg, err := LoadDeepReadConfig(filepath.Join(engineRoot(t), "config.yaml"))
	if err != nil {
		t.Fatalf("LoadDeepReadConfig: %v", err)
	}
	if cfg.ConfidenceThreshold != 0.6 {
		t.Fatalf("confidence_threshold = %v, want 0.6", cfg.ConfidenceThreshold)
	}

	world := newBookWorld(t)
	sessionID := insertOpenSession(t, world)
	ch := insertOpenChapter(t, world, sessionID)

	dir := t.TempDir()
	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	slot := testkit.NewFakeSlot()
	replica := "Я хочу найти выход из города до темноты."
	raw := `{"intent":{"value":"поиск выхода","quote":"хочу найти","confidence":0.9},` +
		`"plan":{"value":"идти к воротам","quote":"из города","confidence":0.8},` +
		`"foreboding":{"value":"опоздать","quote":"до темноты","confidence":0.5},` +
		`"open_thread":{"value":"запертые ворота","quote":"выход","confidence":0.7}}`
	slot.SetResponse(DeepReadRole, raw)

	// Первая реплика главы: разбор выполнен — слот вызван ровно раз.
	res1, err := DeepRead(world, ch, replica, cfg, slot, logger)
	if err != nil {
		t.Fatalf("DeepRead(первая реплика): %v", err)
	}
	if got := slot.Calls(); got != 1 {
		t.Fatalf("вызовов слота разбора на первую реплику = %d, want 1", got)
	}
	if got := countJournalEvents(t, logPath, "deepread.call"); got != 1 {
		t.Fatalf("событий deepread.call = %d, want 1", got)
	}

	// Каждое поле несёт цитату-подстроку реплики (после нормализации).
	normReplica := normalizeText(replica)
	for _, f := range res1.fields() {
		quote := normalizeText(f.Field.Quote)
		if quote == "" || !strings.Contains(normReplica, quote) {
			t.Fatalf("цитата поля %q = %q не является подстрокой реплики", f.Name, f.Field.Quote)
		}
	}

	// Полный ответ слота сохранён в raw_json.
	var rawStored string
	if err := world.QueryRow(`SELECT raw_json FROM deep_reads WHERE chapter_id = ?`, ch.ID).Scan(&rawStored); err != nil {
		t.Fatalf("SELECT deep_reads.raw_json: %v", err)
	}
	if rawStored != raw {
		t.Fatalf("raw_json = %q, want полный ответ слота %q", rawStored, raw)
	}

	// Поле ниже порога (foreboding, confidence 0.5 < 0.6) в разнос не пошло…
	var nForebodingAtlas int
	if err := world.QueryRow(`SELECT COUNT(*) FROM player_atlas WHERE theme = 'foreboding' AND source = ?`, deepReadSource).Scan(&nForebodingAtlas); err != nil {
		t.Fatalf("COUNT player_atlas foreboding: %v", err)
	}
	if nForebodingAtlas != 0 {
		t.Fatalf("поле ниже порога попало в карту игрока: строк = %d, want 0", nForebodingAtlas)
	}
	var nForebodingEvents int
	if err := world.QueryRow(`SELECT COUNT(*) FROM events WHERE kind = ? AND payload_json LIKE '%"field":"foreboding"%'`, deepReadFactKind).Scan(&nForebodingEvents); err != nil {
		t.Fatalf("COUNT events foreboding: %v", err)
	}
	if nForebodingEvents != 0 {
		t.Fatalf("поле ниже порога попало в факты Ткача: строк = %d, want 0", nForebodingEvents)
	}
	// …но сохранилось в deep_reads.
	var forebodingJSON string
	if err := world.QueryRow(`SELECT foreboding_json FROM deep_reads WHERE chapter_id = ?`, ch.ID).Scan(&forebodingJSON); err != nil {
		t.Fatalf("SELECT deep_reads.foreboding_json: %v", err)
	}
	if !strings.Contains(forebodingJSON, "опоздать") {
		t.Fatalf("поле ниже порога не сохранилось в deep_reads: %q", forebodingJSON)
	}

	// Поля выше порога пошли в разнос: три строки карты игрока и три факта Ткача.
	var nAtlas int
	if err := world.QueryRow(`SELECT COUNT(*) FROM player_atlas WHERE source = ?`, deepReadSource).Scan(&nAtlas); err != nil {
		t.Fatalf("COUNT player_atlas: %v", err)
	}
	if nAtlas != 3 {
		t.Fatalf("строк карты игрока = %d, want 3", nAtlas)
	}
	var nEvents int
	if err := world.QueryRow(`SELECT COUNT(*) FROM events WHERE kind = ?`, deepReadFactKind).Scan(&nEvents); err != nil {
		t.Fatalf("COUNT events deepread_fact: %v", err)
	}
	if nEvents != 3 {
		t.Fatalf("строк фактов Ткача = %d, want 3", nEvents)
	}

	// Вторая реплика той же главы: разбор не порождается — вызовов и событий не
	// прибавилось, возвращается сохранённый результат.
	res2, err := DeepRead(world, ch, "Совсем другая реплика.", cfg, slot, logger)
	if err != nil {
		t.Fatalf("DeepRead(вторая реплика): %v", err)
	}
	if got := slot.Calls(); got != 1 {
		t.Fatalf("вызовов слота разбора на вторую реплику = %d, want 1 (без нового вызова)", got)
	}
	if got := countJournalEvents(t, logPath, "deepread.call"); got != 1 {
		t.Fatalf("событий deepread.call после второй реплики = %d, want 1", got)
	}
	if res2.Intent != res1.Intent || res2.Plan != res1.Plan || res2.Foreboding != res1.Foreboding || res2.OpenThread != res1.OpenThread {
		t.Fatalf("повторный разбор вернул другой результат: %+v", res2)
	}
	var deepReadRows int
	if err := world.QueryRow(`SELECT COUNT(*) FROM deep_reads WHERE chapter_id = ?`, ch.ID).Scan(&deepReadRows); err != nil {
		t.Fatalf("COUNT deep_reads: %v", err)
	}
	if deepReadRows != 1 {
		t.Fatalf("записей deep_reads для главы = %d, want 1", deepReadRows)
	}
}
