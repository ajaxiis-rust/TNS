package deepread_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tns65/internal/db"
	"tns65/internal/deepread"
	"tns65/internal/models"
)

// TestDeepreadKey — ключ пересчёта разбора и гейт идемпотентности (D-20.3,
// T-14-11): ключ — тройка (хеш текста книги, версия словарей из sha256
// манифеста, модель эмбеддинга из конфига); прогон с ключом done в журнале
// ковки пропускается с причиной «неизменный ключ» и не добавляет строк
// book_chunk_tag/book_scene, смена dict_version — полный пересчёт.
func TestDeepreadKey(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Fatal("корень движка не найден")
	}

	rawManifest, err := os.ReadFile(filepath.Join(root, "packs", "ru", "manifest.json"))
	if err != nil {
		t.Fatalf("manifest.json: %v", err)
	}
	pack := t.TempDir()
	if err := os.WriteFile(filepath.Join(pack, "manifest.json"), rawManifest, 0o644); err != nil {
		t.Fatalf("запись manifest.json: %v", err)
	}

	cfgPath := writeKeyConfig(t, "bge-m3-Q8_0")
	const bookText = "Лес виднелся за рекой, ветер гнул траву. Анна вошла в комнату и заперла дверь."

	t.Run("ключ из манифеста, конфига и текста", func(t *testing.T) {
		key, err := deepread.NewKey(pack, cfgPath, bookText)
		if err != nil {
			t.Fatalf("NewKey: %v", err)
		}
		if !key.Valid() {
			t.Fatalf("ключ неполон: %s", key)
		}
		if want := sha256Hex(rawManifest)[:deepread.DictVersionLen]; key.DictVersion != want {
			t.Errorf("dict_version = %q, want %q (12 символов sha256 манифеста)", key.DictVersion, want)
		}
		if len(key.DictVersion) != deepread.DictVersionLen {
			t.Errorf("длина dict_version = %d, want %d", len(key.DictVersion), deepread.DictVersionLen)
		}
		if want := sha256Hex([]byte(bookText)); key.TextHash != want {
			t.Errorf("text_hash = %q, want %q", key.TextHash, want)
		}
		if key.EmbedModel != "bge-m3-Q8_0" {
			t.Errorf("embed_model = %q, want bge-m3-Q8_0 (llm.embed_model конфига)", key.EmbedModel)
		}

		// Модель слота embed из настоящего config.yaml читается и не пуста.
		realModel, err := deepread.LoadEmbedModel(filepath.Join(root, "config.yaml"))
		if err != nil {
			t.Fatalf("LoadEmbedModel(config.yaml): %v", err)
		}
		if realModel == "" {
			t.Error("embed_model настоящего config.yaml пуст")
		}

		// Пустой embed_model конфига — модель по умолчанию bge-m3.
		defKey, err := deepread.NewKey(pack, writeKeyConfig(t, ""), bookText)
		if err != nil {
			t.Fatalf("NewKey(пустая модель): %v", err)
		}
		if defKey.EmbedModel != models.DefaultEmbedModel {
			t.Errorf("пустой embed_model: %q, want %q", defKey.EmbedModel, models.DefaultEmbedModel)
		}
		if defKey.Equal(key) {
			t.Error("смена модели эмбеддинга не изменила ключ")
		}

		// Правка текста книги меняет хеш и ключ целиком.
		textKey, err := deepread.NewKey(pack, cfgPath, bookText+" Эпилог.")
		if err != nil {
			t.Fatalf("NewKey(новый текст): %v", err)
		}
		if textKey.TextHash == key.TextHash {
			t.Error("правка текста книги не изменила text_hash")
		}
		if textKey.Equal(key) {
			t.Error("правка текста книги не изменила ключ")
		}

		// Правка манифеста меняет версию словарей — ключ другой.
		editedPack := t.TempDir()
		edited := append(append([]byte{}, rawManifest...), '\n')
		if err := os.WriteFile(filepath.Join(editedPack, "manifest.json"), edited, 0o644); err != nil {
			t.Fatalf("запись правленого manifest.json: %v", err)
		}
		dictKey, err := deepread.NewKey(editedPack, cfgPath, bookText)
		if err != nil {
			t.Fatalf("NewKey(правленый манифест): %v", err)
		}
		if dictKey.DictVersion == key.DictVersion {
			t.Error("правка manifest.json не изменила dict_version")
		}
		if want := sha256Hex(edited)[:deepread.DictVersionLen]; dictKey.DictVersion != want {
			t.Errorf("dict_version правленого манифеста = %q, want %q", dictKey.DictVersion, want)
		}
		if dictKey.Equal(key) {
			t.Error("правка manifest.json не изменила ключ")
		}
	})

	t.Run("идемпотентность прогона", func(t *testing.T) {
		key, err := deepread.NewKey(pack, cfgPath, bookText)
		if err != nil {
			t.Fatalf("NewKey: %v", err)
		}
		lit, bookID := newTestLiterary(t)

		parts := []string{"Лес виднелся за рекой.", "Анна вошла в комнату."}
		ids := make([]int64, len(parts))
		for i, part := range parts {
			res, err := lit.Exec("book_chunk",
				`INSERT INTO book_chunk (book_id, ord, chapter, text_hash) VALUES (?, ?, '1', ?)`,
				bookID, i+1, deepread.TextHash(part))
			if err != nil {
				t.Fatalf("INSERT book_chunk %d: %v", i+1, err)
			}
			if ids[i], err = res.LastInsertId(); err != nil {
				t.Fatalf("book_chunk LastInsertId: %v", err)
			}
		}

		// Разбор: теги чанков и сцена на каждый вызов; скор тега виден в базе.
		calls, score := 0, 0.9
		analyze := func() (int, error) {
			calls++
			tags := make([]deepread.ChunkTags, 0, len(ids))
			for _, id := range ids {
				tags = append(tags, deepread.ChunkTags{ChunkID: id, Tags: []deepread.ChunkTag{{
					Key: deepread.KeyNature, Score: score, Band: deepread.BandConfident, Anchors: []string{"лес"},
				}}})
			}
			if err := deepread.WriteChunkTags(lit, tags); err != nil {
				return 0, err
			}
			if _, err := lit.Exec("book_scene",
				`INSERT INTO book_scene (book_id, ord, from_chunk, to_chunk) VALUES (?, ?, ?, ?)`,
				bookID, calls, ids[0], ids[len(ids)-1]); err != nil {
				return 0, err
			}
			return len(ids), nil
		}

		// Первый прогон: разбор выполнен, строка журнала с ключом, чанками и датой.
		out, err := deepread.RunOnce(lit, key, analyze)
		if err != nil {
			t.Fatalf("RunOnce(первый): %v", err)
		}
		if out.Skipped {
			t.Fatalf("первый прогон пропущен: %q", out.Reason)
		}
		if out.Chunks != len(ids) {
			t.Errorf("чанков первого прогона %d, want %d", out.Chunks, len(ids))
		}
		if calls != 1 {
			t.Errorf("вызовов разбора %d, want 1", calls)
		}
		if n := countChunkTags(t, lit); n != len(ids) {
			t.Errorf("строк book_chunk_tag %d, want %d", n, len(ids))
		}
		if n := countScenes(t, lit, bookID); n != 1 {
			t.Errorf("строк book_scene %d, want 1", n)
		}
		if n := countDeepreadLog(t, lit); n != 1 {
			t.Fatalf("строк forge_log deepread/done %d, want 1", n)
		}

		var kind, status, task, started, finished, logPath, pkg string
		if err := lit.QueryRow(
			`SELECT kind, status, task, started_at, finished_at, log_path, package_version FROM forge_log WHERE kind = 'deepread' ORDER BY id LIMIT 1`).
			Scan(&kind, &status, &task, &started, &finished, &logPath, &pkg); err != nil {
			t.Fatalf("SELECT forge_log: %v", err)
		}
		if kind != deepread.KindDeepread || status != "done" {
			t.Errorf("строка журнала: kind=%q status=%q, want deepread/done", kind, status)
		}
		var rec struct {
			Key    deepread.Key `json:"key"`
			Chunks int          `json:"chunks"`
		}
		if err := json.Unmarshal([]byte(task), &rec); err != nil {
			t.Fatalf("task строки журнала не JSON (%q): %v", task, err)
		}
		if !rec.Key.Equal(key) {
			t.Errorf("ключ в журнале %s, want %s", rec.Key, key)
		}
		if rec.Chunks != len(ids) {
			t.Errorf("чанков в журнале %d, want %d", rec.Chunks, len(ids))
		}
		for _, stamp := range []string{started, finished} {
			if _, err := time.Parse(time.RFC3339, stamp); err != nil {
				t.Errorf("дата журнала %q: %v", stamp, err)
			}
		}
		if pkg == "" {
			t.Error("package_version строки журнала пуст")
		}

		// Второй прогон тем же ключом: пропуск и ни одной новой строки разбора.
		tagsBefore, scenesBefore, logBefore := countChunkTags(t, lit), countScenes(t, lit, bookID), countDeepreadLog(t, lit)
		out, err = deepread.RunOnce(lit, key, analyze)
		if err != nil {
			t.Fatalf("RunOnce(повтор): %v", err)
		}
		if !out.Skipped {
			t.Fatal("повторный прогон с неизменным ключом не пропущен")
		}
		if out.Reason != deepread.ReasonUnchangedKey {
			t.Errorf("причина пропуска %q, want %q", out.Reason, deepread.ReasonUnchangedKey)
		}
		if out.Chunks != len(ids) {
			t.Errorf("чанков пропущенного прогона %d, want %d", out.Chunks, len(ids))
		}
		if calls != 1 {
			t.Errorf("повторный прогон вызвал разбор: вызовов %d, want 1", calls)
		}
		if n := countChunkTags(t, lit); n != tagsBefore {
			t.Errorf("повторный прогон добавил строк book_chunk_tag: %d → %d", tagsBefore, n)
		}
		if n := countScenes(t, lit, bookID); n != scenesBefore {
			t.Errorf("повторный прогон добавил строк book_scene: %d → %d", scenesBefore, n)
		}
		if n := countDeepreadLog(t, lit); n != logBefore {
			t.Errorf("повторный прогон добавил строк forge_log: %d → %d", logBefore, n)
		}

		// Смена dict_version — новый ключ, полный пересчёт: сцена добавлена,
		// теги переписаны.
		changed := key
		changed.DictVersion = sha256Hex([]byte("правка словаря"))[:deepread.DictVersionLen]
		if changed.Equal(key) {
			t.Fatal("новый dict_version совпал с прежним")
		}
		score = 0.7
		scenesBefore = countScenes(t, lit, bookID)
		out, err = deepread.RunOnce(lit, changed, analyze)
		if err != nil {
			t.Fatalf("RunOnce(смена dict_version): %v", err)
		}
		if out.Skipped {
			t.Fatal("смена dict_version не пересчитала разбор")
		}
		if calls != 2 {
			t.Errorf("вызовов разбора %d, want 2", calls)
		}
		if n := countScenes(t, lit, bookID); n != scenesBefore+1 {
			t.Errorf("смена dict_version не добавила строк book_scene: %d → %d", scenesBefore, n)
		}
		if got := tagScore(t, lit, ids[0]); math.Abs(got-0.7) > 1e-9 {
			t.Errorf("пересчёт не переписал теги: скор %g, want 0.7", got)
		}
		if n := countDeepreadLog(t, lit); n != 2 {
			t.Errorf("строк forge_log deepread/done %d, want 2", n)
		}
		if rec, found, err := deepread.FindRun(lit, changed); err != nil || !found {
			t.Errorf("строка журнала нового ключа не найдена: found=%v, err=%v", found, err)
		} else if rec.Chunks != len(ids) {
			t.Errorf("чанков нового ключа %d, want %d", rec.Chunks, len(ids))
		}

		// Оборванный прогон не отмечается done: строка журнала не пишется.
		failed := key
		failed.TextHash = deepread.TextHash("другая книга")
		if _, err := deepread.RunOnce(lit, failed, func() (int, error) {
			return 0, errors.New("обрыв разбора")
		}); err == nil {
			t.Error("RunOnce(обрыв): ошибка потеряна")
		}
		if _, found, err := deepread.FindRun(lit, failed); err != nil || found {
			t.Errorf("оборванный прогон отмечен в журнале: found=%v, err=%v", found, err)
		}
		if n := countDeepreadLog(t, lit); n != 2 {
			t.Errorf("оборванный прогон добавил строк forge_log: %d, want 2", n)
		}
	})
}

// writeKeyConfig пишет временный config.yaml с заданным llm.embed_model.
func writeKeyConfig(t *testing.T, model string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "llm:\n  gen_addr: \":5001\"\n  embed_model: \"" + model + "\"\n  embed_timeout: 30s\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("запись config.yaml: %v", err)
	}
	return path
}

// countDeepreadLog — число завершённых строк forge_log о разборе.
func countDeepreadLog(t *testing.T, lit *db.Handle) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(
		`SELECT COUNT(*) FROM forge_log WHERE kind = 'deepread' AND status = 'done'`).Scan(&n); err != nil {
		t.Fatalf("COUNT forge_log deepread: %v", err)
	}
	return n
}

// tagScore — скор тега чанка по ключу категории.
func tagScore(t *testing.T, lit *db.Handle, chunkID int64) float64 {
	t.Helper()
	var score float64
	if err := lit.QueryRow(
		`SELECT score FROM book_chunk_tag WHERE chunk_id = ? AND key = ?`,
		chunkID, deepread.KeyNature).Scan(&score); err != nil {
		t.Fatalf("SELECT score book_chunk_tag: %v", err)
	}
	return score
}

// sha256Hex — sha256 данных в hex тем же способом, что ключ и book_chunk.text_hash.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
