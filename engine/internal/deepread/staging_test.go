package deepread_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"tns65/internal/db"
	"tns65/internal/deepread"
)

// TestStagingCandidates — гейт staging кандидатов словарей (W-28, приёмка
// T-14-12): кандидат без словарного покрытия пишется в dict_staging со статусом
// candidate, имя собственное, словарная лемма и лемма из чанка ниже порога
// косинуса кандидатами не становятся; повторный разбор поднимает repeats без
// дублей и не меняет статус promoted; файлы пака не изменились (sha256).
func TestStaging(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Fatal("корень движка не найден")
	}
	packDir := filepath.Join(root, "packs", "ru")
	before := packFileHashes(t, packDir)

	cfg, err := deepread.LoadStagingConfig(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatalf("LoadStagingConfig(engine/config.yaml): %v", err)
	}
	if cfg.Cosine != 0.8 {
		t.Fatalf("staging_cosine config.yaml = %g, ожидалось 0.8", cfg.Cosine)
	}

	lit, bookID := newTestLiterary(t)
	world := newTestWorld(t)

	// Фикстура: чанки 1–2 — confident nature с вектором A, чанк 3 — та же
	// семантика (A) с именем собственным, чанк 4 — ортогональный вектор B.
	fixture := stagingFixture()
	texts := make([]string, len(fixture))
	vecs := make([][]float32, len(fixture))
	for i, chunk := range fixture {
		texts[i] = chunk.text
		vecs[i] = chunk.vector
	}
	chunkIDs := insertStagingChunks(t, lit, world, bookID, texts, vecs)

	chunks := make([]deepread.SalienceChunk, len(fixture))
	vectors := make(map[int64][]float32, len(fixture))
	var tags []deepread.ChunkTags
	for i, chunk := range fixture {
		chunks[i] = stagingChunk(chunkIDs[i], chunk.text)
		vectors[chunkIDs[i]] = chunk.vector
		if chunk.natureBand == "" {
			continue
		}
		tags = append(tags, deepread.ChunkTags{
			ChunkID: chunkIDs[i],
			Tags: []deepread.ChunkTag{{
				Key: deepread.KeyNature, Score: 0.9, Band: chunk.natureBand,
			}},
		})
	}

	dicts := map[string]*deepread.Dictionary{
		deepread.KeyNature: {
			Key:    deepread.KeyNature,
			Lemmas: map[string]int{"лес": 1},
		},
	}
	in := deepread.StagingInput{
		BookID: bookID,
		Chunks: chunks,
		Tags:   tags,
		Dicts:  dicts,
		Config: cfg,
	}

	res, err := deepread.StageDictionaries(lit, world, in)
	if err != nil {
		t.Fatalf("StageDictionaries: %v", err)
	}

	// Шаг 1: якорь nature — центроид векторов confident-чанков.
	if len(res.Anchors) != 1 {
		t.Fatalf("якорей %d, ожидался 1 (nature)", len(res.Anchors))
	}
	anchor := res.Anchors[0]
	if anchor.Key != deepread.KeyNature || anchor.OwnerID != deepread.CategoryID(deepread.KeyNature) {
		t.Errorf("якорь = (%q, owner_id %d), want (%q, %d)",
			anchor.Key, anchor.OwnerID, deepread.KeyNature, deepread.CategoryID(deepread.KeyNature))
	}
	var anchorBlob []byte
	var anchorSource string
	if err := lit.QueryRow(
		`SELECT vec, source_text FROM vectors WHERE owner_table = ? AND owner_id = ?`,
		deepread.DictAnchorTable, deepread.CategoryID(deepread.KeyNature)).Scan(&anchorBlob, &anchorSource); err != nil {
		t.Fatalf("SELECT якорь dict_anchor: %v", err)
	}
	// Центроид одинаковых векторов A равен A.
	wantAnchor := testVectorBlob(testVector(map[int]float32{0: 1, 1: 1}))
	if !bytes.Equal(anchorBlob, wantAnchor) {
		t.Errorf("вектор якоря nature не равен центроиду confident-чанков")
	}
	if !strings.Contains(anchorSource, deepread.KeyNature) {
		t.Errorf("source_text якоря не называет ключ: %q", anchorSource)
	}

	// Шаг 2–3: единственный кандидат — «мшистый»; «лес» в словаре, «байкал» —
	// имя собственное, «вереск» — чанк ниже порога косинуса.
	if res.Inserted != 1 || res.Repeated != 0 {
		t.Fatalf("первый прогон: вставлено %d, повторов %d, ожидалось 1/0", res.Inserted, res.Repeated)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("кандидатов %d, ожидался 1: %+v", len(res.Candidates), res.Candidates)
	}
	cand := res.Candidates[0]
	if cand.DictKey != deepread.KeyNature || cand.Lemma != "мшистый" {
		t.Errorf("кандидат = (%q, %q), want (nature, мшистый)", cand.DictKey, cand.Lemma)
	}
	if cand.Weight != 3 || cand.Frequency != 4 {
		t.Errorf("кандидат: вес %d, частота %d; want 3 и 4", cand.Weight, cand.Frequency)
	}
	if !strings.Contains(chunks[0].Text, cand.Evidence) {
		t.Errorf("основание кандидата не подстрока чанка: %q", cand.Evidence)
	}
	for _, other := range res.Candidates {
		switch other.Lemma {
		case "байкал", "лес", "вереск", "камень":
			t.Errorf("лемма %q не должна быть кандидатом", other.Lemma)
		}
	}

	// Строка dict_staging: статус candidate, книга и счётчик первых вхождений.
	var lemma, status, evidence string
	var weight, repeats, rowBookID int64
	if err := lit.QueryRow(
		`SELECT lemma, weight, evidence, status, repeats, book_id FROM dict_staging WHERE dict_key = ?`,
		deepread.KeyNature).Scan(&lemma, &weight, &evidence, &status, &repeats, &rowBookID); err != nil {
		t.Fatalf("SELECT dict_staging: %v", err)
	}
	if lemma != "мшистый" || status != deepread.StagingStatusCandidate {
		t.Errorf("строка staging = (%q, %q), want (мшистый, candidate)", lemma, status)
	}
	if weight != 3 || repeats != 1 {
		t.Errorf("строка staging: вес %d, повторов %d; want 3 и 1", weight, repeats)
	}
	if rowBookID != bookID {
		t.Errorf("dict_staging.book_id = %d, want %d", rowBookID, bookID)
	}
	if evidence != cand.Evidence {
		t.Errorf("evidence в базе %q, want %q", evidence, cand.Evidence)
	}

	// Шаг 5: статус promoted ставит владелец; код его не перезаписывает.
	if _, err := lit.Exec("dict_staging",
		`UPDATE dict_staging SET status = ?, promoted_at = '2026-11-14T00:00:00Z' WHERE dict_key = ? AND lemma = ?`,
		deepread.StagingStatusPromoted, deepread.KeyNature, "мшистый"); err != nil {
		t.Fatalf("UPDATE promoted: %v", err)
	}

	// Повторный разбор: новая строка не появляется, repeats растёт.
	again, err := deepread.StageDictionaries(lit, world, in)
	if err != nil {
		t.Fatalf("повторный StageDictionaries: %v", err)
	}
	if again.Inserted != 0 || again.Repeated != 1 {
		t.Errorf("повторный прогон: вставлено %d, повторов %d, ожидалось 0/1",
			again.Inserted, again.Repeated)
	}
	if n := countStagingRows(t, lit); n != 1 {
		t.Fatalf("строк dict_staging %d, ожидалась 1", n)
	}
	var promotedAt string
	if err := lit.QueryRow(
		`SELECT status, repeats, promoted_at FROM dict_staging WHERE dict_key = ? AND lemma = ?`,
		deepread.KeyNature, "мшистый").Scan(&status, &repeats, &promotedAt); err != nil {
		t.Fatalf("SELECT после повтора: %v", err)
	}
	if status != deepread.StagingStatusPromoted {
		t.Errorf("после повтора статус %q, want promoted", status)
	}
	if repeats != 2 {
		t.Errorf("после повтора repeats = %d, want 2", repeats)
	}
	if promotedAt != "2026-11-14T00:00:00Z" {
		t.Errorf("после повтора promoted_at = %q, want дату владельца", promotedAt)
	}

	// Якорь заменяется, а не дублируется.
	var anchorRows int
	if err := lit.QueryRow(
		`SELECT COUNT(*) FROM vectors WHERE owner_table = ?`, deepread.DictAnchorTable).Scan(&anchorRows); err != nil {
		t.Fatalf("COUNT vectors dict_anchor: %v", err)
	}
	if anchorRows != 1 {
		t.Errorf("векторов якорей %d, ожидался 1", anchorRows)
	}

	// Шаг 4: файлы пака разбор не тронул.
	after := packFileHashes(t, packDir)
	if len(after) != len(before) {
		t.Errorf("файлов пака стало %d, было %d", len(after), len(before))
	}
	for name, sum := range before {
		if after[name] != sum {
			t.Errorf("файл пака изменился: %s", name)
		}
	}
}

// stagingFixtureChunk — чанк фикстуры: текст, вектор и полоса nature.
type stagingFixtureChunk struct {
	text       string
	vector     []float32
	natureBand string
}

// stagingFixture — четыре чанка: два confident nature с вектором A, чанк с
// именем собственным и тем же вектором A, чанк с ортогональным вектором B.
func stagingFixture() []stagingFixtureChunk {
	a := testVector(map[int]float32{0: 1, 1: 1})
	b := testVector(map[int]float32{2: 1})
	return []stagingFixtureChunk{
		{
			text: strings.Join([]string{
				"Мшистый камень лежал у воды.",
				"Мшистый бор шумел вдали.",
				"Лес стоял тихо.",
			}, "\n"),
			vector:     a,
			natureBand: deepread.BandConfident,
		},
		{
			text: strings.Join([]string{
				"На мшистый склон падал свет.",
				"Мшистый мох блестел.",
				"Лес темнел.",
			}, "\n"),
			vector:     a,
			natureBand: deepread.BandConfident,
		},
		{
			text: strings.Join([]string{
				"Байкал синел вдали.",
				"Байкал шумел у скал.",
				"Байкал манил к себе.",
				"Лес стоял тихо.",
			}, "\n"),
			vector:     a,
			natureBand: deepread.BandReject,
		},
		{
			text: strings.Join([]string{
				"Вереск цвёл у тропы.",
				"Вереск пах.",
				"Вереск шумел.",
			}, "\n"),
			vector:     b,
			natureBand: deepread.BandReject,
		},
	}
}

// stagingChunk размечает чанк токенами: форма сохраняет регистр текста, POS
// задаётся по лемме.
func stagingChunk(id int64, text string) deepread.SalienceChunk {
	chunk := deepread.SalienceChunk{ID: id, Text: text}
	for lineNo, line := range strings.Split(text, "\n") {
		index := 0
		for _, form := range stagingWords(line) {
			lemma := strings.ToLower(form)
			pos := "NOUN"
			switch lemma {
			case "байкал":
				pos = "PROPN"
			case "мшистый":
				pos = "ADJ"
			}
			chunk.Lemmas = append(chunk.Lemmas, deepread.Lemma{
				Form: form, Lemma: lemma, POS: pos, Line: lineNo + 1, Index: index,
			})
			index++
		}
	}
	return chunk
}

// stagingWords выделяет слова строки, сохраняя регистр.
func stagingWords(line string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range line {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

// insertStagingChunks раскладывает чанки книги, записи банка ковки и векторы
// чанков в оперативной базе.
func insertStagingChunks(t *testing.T, lit, world *db.Handle, bookID int64,
	texts []string, vecs [][]float32) []int64 {
	t.Helper()
	chunkIDs := make([]int64, len(texts))
	for i, text := range texts {
		res, err := lit.Exec("book_chunk",
			`INSERT INTO book_chunk (book_id, ord, chapter, text_hash) VALUES (?, ?, '1', ?)`,
			bookID, i, fmt.Sprintf("h%d", i))
		if err != nil {
			t.Fatalf("INSERT book_chunk %d: %v", i, err)
		}
		if chunkIDs[i], err = res.LastInsertId(); err != nil {
			t.Fatalf("book_chunk LastInsertId: %v", err)
		}

		res, err = lit.Exec("bank_pool",
			`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at, book_id) VALUES ('routine', ?, 'ru', 't', 2, ?, '2026-01-01T00:00:00Z', ?)`,
			text, fmt.Sprintf("forge-book:%d:%d", bookID, i), bookID)
		if err != nil {
			t.Fatalf("INSERT bank_pool %d: %v", i, err)
		}
		bankID, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("bank_pool LastInsertId: %v", err)
		}
		if _, err := world.Exec("vectors",
			`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES ('bank_pool', ?, 'bge-m3', 1024, 'ru', ?, ?, '2026-01-01T00:00:00Z')`,
			bankID, text, testVectorBlob(vecs[i])); err != nil {
			t.Fatalf("INSERT vectors чанка %d: %v", i, err)
		}
	}
	return chunkIDs
}

// countStagingRows считает строки dict_staging.
func countStagingRows(t *testing.T, lit *db.Handle) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM dict_staging`).Scan(&n); err != nil {
		t.Fatalf("COUNT dict_staging: %v", err)
	}
	return n
}

// packFileHashes возвращает sha256 каждого файла каталога пака по
// относительному пути.
func packFileHashes(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("sha256 пака %s: %v", dir, err)
	}
	return out
}
