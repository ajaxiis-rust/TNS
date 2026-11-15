package deepread_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/deepread"
	"tns65/internal/testkit"
)

// TestRunPipeline — приёмка T-14-13: полный прогон разбора книги пишет все пять
// таблиц разбора и векторы literary (персонажи, сцены, якоря), повторный прогон
// по неизменному ключу новых строк не даёт, сводка в forge_log kind='deepread'
// сходится с содержимым таблиц.
func TestRunPipeline(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Fatal("корень движка не найден")
	}
	packDir := filepath.Join(root, "packs", "ru")
	if residentBinary(t, packDir) == "" {
		t.Skip("бинарник резидента лемматизатора недоступен")
	}

	lit, bookID := newTestLiterary(t)
	world := newTestWorld(t)

	texts := []string{
		"Золотой дворец стоял на холме. Иван молчал от страха.",
		"Золотого дворца сияли окна. Иван ждал.",
		"Иван вышел из дворца. Он шёл тихо и задумчиво.",
		"Спустя день Тёмный лес шумел вокруг. Марья бежала.",
		"Тёмный лес молчал. Марья стояла тихо.",
		"Марья слушала лес. Она думала задумчиво.",
		"Спустя два дня Золотой дворец опустел. Иван вернулся тихо.",
		"Золотой дворец затих. Иван и Марья молчали задумчиво.",
	}
	chunkIDs := insertSceneChunks(t, lit, world, bookID, texts)

	// Карта персонажей: окна Ивана и Марьи не пересекаются — слияния нет.
	insertRunCharacter(t, lit, bookID, "Иван", map[int]int64{0: 2, 1: 3, 2: 1}, chunkIDs)
	insertRunCharacter(t, lit, bookID, "Марья", map[int]int64{3: 2, 4: 3, 5: 1}, chunkIDs)

	chunks := make([]deepread.RunChunk, len(texts))
	for i, id := range chunkIDs {
		chunks[i] = deepread.RunChunk{ID: id, Ord: i, Chapter: "1", Text: texts[i]}
	}

	slot := testkit.NewFakeSlot()
	first, err := deepread.RunPipeline(lit, world, deepread.RunInput{
		BookID:     bookID,
		Chunks:     chunks,
		PackDir:    packDir,
		ConfigPath: filepath.Join(root, "config.yaml"),
		Slot:       slot,
		Now:        "2026-11-14T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("RunPipeline: %v", err)
	}
	if first.Skipped {
		t.Fatalf("первый прогон пропущен: %s", first.Reason)
	}

	// Пять таблиц разбора: теги, места, сцены, стиль, staging.
	tagRows := countRows(t, lit, `SELECT COUNT(*) FROM book_chunk_tag`)
	if tagRows != len(texts)*len(deepread.DictionaryKeys) {
		t.Errorf("строк book_chunk_tag = %d, want %d (чанк × ключ словаря)", tagRows, len(texts)*len(deepread.DictionaryKeys))
	}
	if n := countRows(t, lit, `SELECT COUNT(*) FROM book_place WHERE book_id = ?`, bookID); n == 0 {
		t.Error("book_place: мест нет")
	}
	if n := countRows(t, lit, `SELECT COUNT(*) FROM book_scene WHERE book_id = ?`, bookID); n == 0 {
		t.Error("book_scene: сцен нет")
	}
	if n := countRows(t, lit, `SELECT COUNT(*) FROM book_style WHERE book_id = ?`, bookID); n != 1 {
		t.Errorf("строк book_style = %d, want 1", n)
	}
	if n := countRows(t, lit, `SELECT COUNT(*) FROM dict_staging WHERE book_id = ?`, bookID); n == 0 {
		t.Error("dict_staging: кандидатов нет")
	}

	// Векторы литературной базы: персонажи, сцены, якоря словарей.
	for _, table := range []string{deepread.CharacterVectorTable, "book_scene", deepread.DictAnchorTable} {
		if n := countVectors(t, lit, table); n == 0 {
			t.Errorf("векторов owner_table=%q нет", table)
		}
	}

	// Сводка сходится с содержимым таблиц и вызовами слота.
	summary := readRunSummary(t, lit)
	if summary.Chunks != len(texts) {
		t.Errorf("сводка: чанков %d, want %d", summary.Chunks, len(texts))
	}
	if summary.Scenes != countRows(t, lit, `SELECT COUNT(*) FROM book_scene WHERE book_id = ?`, bookID) {
		t.Errorf("сводка: сцен %d, таблица %d", summary.Scenes,
			countRows(t, lit, `SELECT COUNT(*) FROM book_scene WHERE book_id = ?`, bookID))
	}
	if summary.Characters != countRows(t, lit, `SELECT COUNT(*) FROM book_character WHERE book_id = ?`, bookID) {
		t.Errorf("сводка: персонажей %d, таблица %d", summary.Characters,
			countRows(t, lit, `SELECT COUNT(*) FROM book_character WHERE book_id = ?`, bookID))
	}
	if summary.Staging != countRows(t, lit, `SELECT COUNT(*) FROM dict_staging WHERE book_id = ?`, bookID) {
		t.Errorf("сводка: кандидатов staging %d, таблица %d", summary.Staging,
			countRows(t, lit, `SELECT COUNT(*) FROM dict_staging WHERE book_id = ?`, bookID))
	}
	bandTotal := 0
	for _, bands := range summary.Bands {
		for _, n := range bands {
			bandTotal += n
		}
	}
	if bandTotal != tagRows {
		t.Errorf("сводка: полосы %d, таблица book_chunk_tag %d", bandTotal, tagRows)
	}
	if summary.SlotCalls != slot.Calls() {
		t.Errorf("сводка: вызовов слота %d, слот вызван %d", summary.SlotCalls, slot.Calls())
	}
	if summary.Bands[deepread.KeyErotic] == nil {
		t.Error("сводка: нет полос категории erotic")
	}

	// Повторный прогон по неизменному ключу: пропуск и ни одной новой строки.
	before := countRows(t, lit, `SELECT COUNT(*) FROM book_chunk_tag`) +
		countRows(t, lit, `SELECT COUNT(*) FROM book_place`) +
		countRows(t, lit, `SELECT COUNT(*) FROM book_scene`) +
		countRows(t, lit, `SELECT COUNT(*) FROM book_style`) +
		countRows(t, lit, `SELECT COUNT(*) FROM dict_staging`) +
		countVectors(t, lit, deepread.CharacterVectorTable) +
		countVectors(t, lit, "book_scene") +
		countVectors(t, lit, deepread.DictAnchorTable)
	second, err := deepread.RunPipeline(lit, world, deepread.RunInput{
		BookID:     bookID,
		Chunks:     chunks,
		PackDir:    packDir,
		ConfigPath: filepath.Join(root, "config.yaml"),
		Slot:       slot,
	})
	if err != nil {
		t.Fatalf("RunPipeline (повтор): %v", err)
	}
	if !second.Skipped || second.Reason != deepread.ReasonUnchangedKey {
		t.Fatalf("повторный прогон: skipped=%v, reason=%q", second.Skipped, second.Reason)
	}
	after := countRows(t, lit, `SELECT COUNT(*) FROM book_chunk_tag`) +
		countRows(t, lit, `SELECT COUNT(*) FROM book_place`) +
		countRows(t, lit, `SELECT COUNT(*) FROM book_scene`) +
		countRows(t, lit, `SELECT COUNT(*) FROM book_style`) +
		countRows(t, lit, `SELECT COUNT(*) FROM dict_staging`) +
		countVectors(t, lit, deepread.CharacterVectorTable) +
		countVectors(t, lit, "book_scene") +
		countVectors(t, lit, deepread.DictAnchorTable)
	if after != before {
		t.Errorf("повторный прогон добавил строк: было %d, стало %d", before, after)
	}
	if n := countRunSummaries(t, lit); n != 1 {
		t.Errorf("сводок kind='deepread' в forge_log = %d, want 1", n)
	}
}

// insertRunCharacter пишет героя карты и его упоминания по чанкам.
func insertRunCharacter(t *testing.T, lit *db.Handle, bookID int64, name string, mentions map[int]int64, chunkIDs []int64) {
	t.Helper()
	var first, last, total int64
	for ord, count := range mentions {
		if first == 0 || chunkIDs[ord] < first {
			first = chunkIDs[ord]
		}
		if chunkIDs[ord] > last {
			last = chunkIDs[ord]
		}
		total += count
	}
	res, err := lit.Exec("book_character",
		`INSERT INTO book_character (book_id, name, aliases, confidence, first_chunk, last_chunk, mentions, status, created_at) VALUES (?, ?, '[]', 0.5, ?, ?, ?, 'candidate', '2026-01-01T00:00:00Z')`,
		bookID, name, first, last, total)
	if err != nil {
		t.Fatalf("INSERT book_character %s: %v", name, err)
	}
	characterID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("book_character LastInsertId: %v", err)
	}
	for ord, count := range mentions {
		if err := insertMention(lit, characterID, chunkIDs[ord], count); err != nil {
			t.Fatalf("INSERT mention %s: %v", name, err)
		}
	}
}

// countRows — число строк запроса.
func countRows(t *testing.T, h *db.Handle, query string, args ...any) int {
	t.Helper()
	var n int
	if err := h.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("COUNT (%s): %v", query, err)
	}
	return n
}

// countVectors — число векторов owner_table.
func countVectors(t *testing.T, h *db.Handle, ownerTable string) int {
	t.Helper()
	return countRows(t, h, `SELECT COUNT(*) FROM vectors WHERE owner_table = ?`, ownerTable)
}

// readRunSummary читает последнюю сводку разбора из forge_log.
func readRunSummary(t *testing.T, lit *db.Handle) deepread.Summary {
	t.Helper()
	rows, err := lit.Query(`SELECT task FROM forge_log WHERE kind = ? ORDER BY id DESC`, deepread.KindDeepread)
	if err != nil {
		t.Fatalf("SELECT forge_log: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("Scan forge_log: %v", err)
		}
		var summary deepread.Summary
		if err := json.Unmarshal([]byte(raw), &summary); err != nil || summary.Bands == nil {
			continue
		}
		return summary
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("forge_log rows: %v", err)
	}
	t.Fatal("сводка kind='deepread' в forge_log не найдена")
	return deepread.Summary{}
}

// countRunSummaries считает строки forge_log kind='deepread' со сводкой.
func countRunSummaries(t *testing.T, lit *db.Handle) int {
	t.Helper()
	rows, err := lit.Query(`SELECT task FROM forge_log WHERE kind = ?`, deepread.KindDeepread)
	if err != nil {
		t.Fatalf("SELECT forge_log: %v", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("Scan forge_log: %v", err)
		}
		var summary deepread.Summary
		if err := json.Unmarshal([]byte(raw), &summary); err == nil && summary.Bands != nil {
			n++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("forge_log rows: %v", err)
	}
	return n
}
