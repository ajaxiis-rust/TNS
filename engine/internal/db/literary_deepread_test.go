package db

import (
	"path/filepath"
	"slices"
	"testing"
)

// TestLiteraryDeepreadSchema — миграция 0006_book_deepread (P13 §3, T-14-03):
// пять таблиц разбора, CHECK-ограничения полосы и статуса, аддитивность
// миграции на базе, рождённой до 0006 (D-19).
func TestLiteraryDeepreadSchema(t *testing.T) {
	w, err := OpenWriter(filepath.Join(t.TempDir(), "literary.db"))
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	defer w.Close()

	if err := Migrate(w, "literary", "shared"); err != nil {
		t.Fatalf("Migrate(literary, shared): %v", err)
	}

	// 1. Колонки пяти таблиц — дословно из §3 плана.
	wantCols := map[string][]string{
		"book_chunk_tag": {"chunk_id", "key", "score", "band", "anchors"},
		"book_place": {
			"id", "book_id", "name", "aliases", "kind", "first_chunk",
			"last_chunk", "mentions",
		},
		"book_scene": {
			"id", "book_id", "ord", "from_chunk", "to_chunk", "place_id",
			"time_marker", "participants", "tags", "emotion",
		},
		"book_style": {
			"book_id", "author", "features", "labels", "drama_profile",
			"computed_at",
		},
		"dict_staging": {
			"id", "dict_key", "lemma", "weight", "evidence", "book_id",
			"status", "repeats", "created_at", "promoted_at",
		},
	}
	for tbl, want := range wantCols {
		if got := colsOf(t, w, tbl); !slices.Equal(got, want) {
			t.Errorf("%s columns = %v, want %v", tbl, got, want)
		}
	}

	// 2. Учёт миграции: версия 8 (с 0008_bank_tags) и строка schema_migrations.
	if uv := userVersionOf(t, w); uv != 8 {
		t.Errorf("user_version = %d, want 8", uv)
	}
	var accounted int
	if err := w.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 6 AND name = '0006_book_deepread'`).Scan(&accounted); err != nil {
		t.Fatalf("schema_migrations 0006_book_deepread: %v", err)
	}
	if accounted != 1 {
		t.Errorf("строк учёта 0006_book_deepread = %d, want 1", accounted)
	}

	bookRes, err := w.Exec(`INSERT INTO book (shelf_id, name, lang, genres, rating, chapters, status, created_at) VALUES ('p13-2026', 'Книга разбора', 'ru', 'роман', '16+', 3, 'ready', '2026-11-14T00:00:00Z')`)
	if err != nil {
		t.Fatalf("INSERT book: %v", err)
	}
	bookID, err := bookRes.LastInsertId()
	if err != nil {
		t.Fatalf("book LastInsertId: %v", err)
	}
	chunkRes, err := w.Exec(`INSERT INTO book_chunk (book_id, ord, chapter, text_hash) VALUES (?, 1, '1', 'h1')`, bookID)
	if err != nil {
		t.Fatalf("INSERT book_chunk: %v", err)
	}
	chunkID, err := chunkRes.LastInsertId()
	if err != nil {
		t.Fatalf("book_chunk LastInsertId: %v", err)
	}

	// 3. book_chunk_tag: три полосы проходят, band='xxx' падает по CHECK,
	// повтор (chunk_id, key) падает по PK, якоря по умолчанию '[]'.
	insertTag := func(chunk int64, key, band string) error {
		_, err := w.Exec(`INSERT INTO book_chunk_tag (chunk_id, key, score, band) VALUES (?, ?, 0.7, ?)`, chunk, key, band)
		return err
	}
	for _, band := range []string{"confident", "reject", "uncertain"} {
		if err := insertTag(chunkID, "band_"+band, band); err != nil {
			t.Errorf("INSERT book_chunk_tag band=%q: %v", band, err)
		}
	}
	if err := insertTag(chunkID, "band_xxx", "xxx"); err == nil {
		t.Errorf("INSERT book_chunk_tag band='xxx' should fail CHECK (band)")
	}
	if err := insertTag(chunkID, "band_confident", "reject"); err == nil {
		t.Errorf("повтор (chunk_id, key) должен упасть по PK book_chunk_tag")
	}
	if err := insertTag(chunkID+999, "ghost", "confident"); err == nil {
		t.Errorf("INSERT book_chunk_tag с чужим chunk_id должен упасть по FK")
	}
	var anchors string
	if err := w.QueryRow(`SELECT anchors FROM book_chunk_tag WHERE chunk_id = ? AND key = 'band_confident'`, chunkID).Scan(&anchors); err != nil {
		t.Fatalf("SELECT book_chunk_tag anchors: %v", err)
	}
	if anchors != "[]" {
		t.Errorf("book_chunk_tag.anchors по умолчанию = %q, want \"[]\"", anchors)
	}

	// 4. book_place и book_scene: place_id NULL допустим, UNIQUE (book_id, ord)
	// держит, JSON-дефолты на месте.
	placeRes, err := w.Exec(`INSERT INTO book_place (book_id, name) VALUES (?, 'Хогвартс')`, bookID)
	if err != nil {
		t.Fatalf("INSERT book_place: %v", err)
	}
	placeID, err := placeRes.LastInsertId()
	if err != nil {
		t.Fatalf("book_place LastInsertId: %v", err)
	}
	insertScene := func(ord int, place any) error {
		_, err := w.Exec(`INSERT INTO book_scene (book_id, ord, from_chunk, to_chunk, place_id) VALUES (?, ?, 1, 2, ?)`, bookID, ord, place)
		return err
	}
	if err := insertScene(1, placeID); err != nil {
		t.Errorf("INSERT book_scene с местом: %v", err)
	}
	if err := insertScene(2, nil); err != nil {
		t.Errorf("INSERT book_scene с place_id NULL: %v", err)
	}
	if err := insertScene(1, nil); err == nil {
		t.Errorf("повтор (book_id, ord) должен упасть по UNIQUE book_scene")
	}
	var participants, sceneTags, emotion string
	if err := w.QueryRow(`SELECT participants, tags, emotion FROM book_scene WHERE book_id = ? AND ord = 2`, bookID).Scan(&participants, &sceneTags, &emotion); err != nil {
		t.Fatalf("SELECT book_scene defaults: %v", err)
	}
	if participants != "[]" || sceneTags != "[]" || emotion != "{}" {
		t.Errorf("book_scene defaults = (%q, %q, %q), want (\"[]\", \"[]\", \"{}\")", participants, sceneTags, emotion)
	}

	// 5. book_style: PK book_id, дефолты полей профиля.
	if _, err := w.Exec(`INSERT INTO book_style (book_id, computed_at) VALUES (?, '2026-11-14T00:00:00Z')`, bookID); err != nil {
		t.Fatalf("INSERT book_style: %v", err)
	}
	if _, err := w.Exec(`INSERT INTO book_style (book_id, computed_at) VALUES (?, '2026-11-14T00:00:00Z')`, bookID); err == nil {
		t.Errorf("повтор book_style.book_id должен упасть по PK")
	}
	var author, features, labels, drama string
	if err := w.QueryRow(`SELECT author, features, labels, drama_profile FROM book_style WHERE book_id = ?`, bookID).Scan(&author, &features, &labels, &drama); err != nil {
		t.Fatalf("SELECT book_style defaults: %v", err)
	}
	if author != "" || features != "{}" || labels != "[]" || drama != "[]" {
		t.Errorf("book_style defaults = (%q, %q, %q, %q)", author, features, labels, drama)
	}

	// 6. dict_staging: три статуса проходят, status='bogus' падает по CHECK,
	// повтор (dict_key, lemma) падает по UNIQUE, book_id NULL допустим.
	insertStaging := func(key, lemma, status string) error {
		_, err := w.Exec(`INSERT INTO dict_staging (dict_key, lemma, status, created_at) VALUES (?, ?, ?, '2026-11-14T00:00:00Z')`, key, lemma, status)
		return err
	}
	for _, status := range []string{"candidate", "promoted", "rejected"} {
		if err := insertStaging("nature", status, status); err != nil {
			t.Errorf("INSERT dict_staging status=%q: %v", status, err)
		}
	}
	if err := insertStaging("nature", "лишний", "bogus"); err == nil {
		t.Errorf("INSERT dict_staging status='bogus' should fail CHECK (status)")
	}
	if err := insertStaging("nature", "candidate", "candidate"); err == nil {
		t.Errorf("повтор (dict_key, lemma) должен упасть по UNIQUE dict_staging")
	}
	var bookNull int
	if err := w.QueryRow(`SELECT book_id IS NULL FROM dict_staging WHERE dict_key = 'nature' AND lemma = 'candidate'`).Scan(&bookNull); err != nil {
		t.Fatalf("SELECT dict_staging book_id: %v", err)
	}
	if bookNull != 1 {
		t.Errorf("dict_staging.book_id по умолчанию должен быть NULL")
	}

	// 7. Аддитивность (D-19): база мира P11 на 0005 с книгой и куском
	// открывается после 0006 без переноса записей.
	t.Run("additive_on_p11", func(t *testing.T) {
		lw, err := OpenWriter(filepath.Join(t.TempDir(), "world-p11.db"))
		if err != nil {
			t.Fatalf("OpenWriter: %v", err)
		}
		defer lw.Close()

		legacy := []string{"0001_init", "0002_sediment_idempotency", "0003_composition", "0004_book", "0005_cast"}
		for _, name := range legacy {
			content, err := schemaFS.ReadFile("schema/literary/" + name + ".sql")
			if err != nil {
				t.Fatalf("ReadFile %s: %v", name, err)
			}
			if _, err := lw.Exec(string(content)); err != nil {
				t.Fatalf("apply %s: %v", name, err)
			}
		}
		if _, err := lw.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
			t.Fatalf("create schema_migrations: %v", err)
		}
		for i, name := range legacy {
			if _, err := lw.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`, i+1, name, "2026-01-01T00:00:00Z"); err != nil {
				t.Fatalf("insert schema_migrations %s: %v", name, err)
			}
		}
		if _, err := lw.Exec(`PRAGMA user_version = 5`); err != nil {
			t.Fatalf("PRAGMA user_version = 5: %v", err)
		}
		oldBook, err := lw.Exec(`INSERT INTO book (shelf_id, name, lang, genres, rating, chapters, status, created_at) VALUES ('p11-2026', 'Старая книга', 'ru', 'роман', '12+', 1, 'ready', '2026-01-01T00:00:00Z')`)
		if err != nil {
			t.Fatalf("INSERT old book: %v", err)
		}
		oldBookID, err := oldBook.LastInsertId()
		if err != nil {
			t.Fatalf("old book LastInsertId: %v", err)
		}
		if _, err := lw.Exec(`INSERT INTO book_chunk (book_id, ord, chapter, text_hash) VALUES (?, 1, '1', 'old-hash')`, oldBookID); err != nil {
			t.Fatalf("INSERT old book_chunk: %v", err)
		}

		if err := Migrate(lw, "literary", "shared"); err != nil {
			t.Fatalf("Migrate поверх мира P11: %v", err)
		}

		var hash string
		if err := lw.QueryRow(`SELECT text_hash FROM book_chunk WHERE book_id = ?`, oldBookID).Scan(&hash); err != nil {
			t.Fatalf("SELECT old book_chunk: %v", err)
		}
		if hash != "old-hash" {
			t.Errorf("text_hash старого куска = %q, want old-hash", hash)
		}
		for _, tbl := range []string{"book_chunk_tag", "book_place", "book_scene", "book_style", "dict_staging"} {
			if _, err := lw.Exec(`SELECT COUNT(*) FROM ` + tbl); err != nil {
				t.Errorf("мир P11 после 0006: таблица %s недоступна: %v", tbl, err)
			}
		}
		for i, name := range legacy {
			var rows int
			if err := lw.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ? AND name = ?`, i+1, name).Scan(&rows); err != nil {
				t.Fatalf("schema_migrations %s: %v", name, err)
			}
			if rows != 1 {
				t.Errorf("строк учёта %s = %d, want 1", name, rows)
			}
		}
		if uv := userVersionOf(t, lw); uv != 8 {
			t.Errorf("user_version после 0006 на мире P11 = %d, want 8", uv)
		}
	})
}
