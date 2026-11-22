package db

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"slices"
	"testing"
)

// TestLiteraryVectors — общая миграция векторов shared/0002_vectors.sql
// подключена к литературной базе (T-14-02, D-20.2): таблица vectors с
// колонками DDL, вставка и чтение вектора dim=1024, учёт миграций не
// откатывается.
func TestLiteraryVectors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "literary.db")
	w, err := OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	defer w.Close()

	if err := Migrate(w, "literary", "shared"); err != nil {
		t.Fatalf("Migrate(literary, shared): %v", err)
	}

	// 1. Колонки vectors — те же, что в shared/0002_vectors.sql.
	wantCols := []string{"id", "owner_table", "owner_id", "model", "dim", "lang", "source_text", "vec", "created_at"}
	rows, err := w.Query(`PRAGMA table_info(vectors)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(vectors): %v", err)
	}
	var gotCols []string
	pkID := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("table_info(vectors) scan: %v", err)
		}
		gotCols = append(gotCols, name)
		if name == "id" && pk == 1 {
			pkID = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table_info(vectors) rows: %v", err)
	}
	rows.Close()

	if !slices.Equal(gotCols, wantCols) {
		t.Errorf("колонки vectors = %v, want %v", gotCols, wantCols)
	}
	if !pkID {
		t.Errorf("vectors.id не PRIMARY KEY")
	}
	if !indexHasCols(t, w, "idx_vectors_owner", "owner_table", "owner_id") {
		t.Errorf("индекс idx_vectors_owner (owner_table, owner_id) отсутствует или не по тем колонкам")
	}

	// 2. Вставка и чтение вектора dim=1024.
	vec := make([]byte, 4096)
	for i := range vec {
		vec[i] = byte(i % 251)
	}
	res, err := w.Exec(`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"book_character", 7, "deepread-1024", 1024, "ru", "Анна Каренина", vec, "2026-11-14T00:00:00Z")
	if err != nil {
		t.Fatalf("INSERT vectors: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	var owner, model, lang, source, created string
	var ownerID, dim int
	var back []byte
	if err := w.QueryRow(`SELECT owner_table, owner_id, model, dim, lang, source_text, vec, created_at
		FROM vectors WHERE id = ?`, id).
		Scan(&owner, &ownerID, &model, &dim, &lang, &source, &back, &created); err != nil {
		t.Fatalf("SELECT vectors: %v", err)
	}
	if owner != "book_character" || ownerID != 7 || dim != 1024 || lang != "ru" {
		t.Errorf("прочитано owner=%q owner_id=%d dim=%d lang=%q, want book_character/7/1024/ru", owner, ownerID, dim, lang)
	}
	if !bytes.Equal(back, vec) {
		t.Errorf("vec прочитан не равным вставленному (len %d != %d)", len(back), len(vec))
	}

	// 3. CHECK dim = 1024 из общей миграции действует.
	if _, err := w.Exec(`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"book_character", 8, "wrong-dim", 384, "ru", "текст", vec, "2026-11-14T00:00:00Z"); err == nil {
		t.Errorf("вставка dim=384 прошла, ожидалась ошибка CHECK dim=1024")
	}

	// 4. user_version не откатывается: общая миграция встала рядом с
	// литературной 0002_sediment_idempotency (тот же номер, другое имя).
	uv := userVersionOf(t, w)
	if uv < 5 {
		t.Errorf("user_version = %d, want >= 5", uv)
	}
	var sameVersion int
	if err := w.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 2`).Scan(&sameVersion); err != nil {
		t.Fatalf("schema_migrations version=2: %v", err)
	}
	if sameVersion != 2 {
		t.Errorf("миграций с версией 2 = %d, want 2 (0002_sediment_idempotency и 0002_vectors)", sameVersion)
	}

	// 5. Повторная миграция идемпотентна.
	if err := Migrate(w, "literary", "shared"); err != nil {
		t.Fatalf("повторный Migrate(literary, shared): %v", err)
	}
	if got := userVersionOf(t, w); got != uv {
		t.Errorf("user_version после повторной миграции = %d, want %d", got, uv)
	}
	var vectorsRows int
	if err := w.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = '0002_vectors'`).Scan(&vectorsRows); err != nil {
		t.Fatalf("schema_migrations 0002_vectors: %v", err)
	}
	if vectorsRows != 1 {
		t.Errorf("строк учёта 0002_vectors = %d, want 1", vectorsRows)
	}

	// 6. Мир, рождённый до T-14-02, несёт легаси-учёт (ключ только по version):
	// shared-миграция добавляется без перемиграции уже учтённых файлов.
	t.Run("legacy_accounting", func(t *testing.T) {
		lw, err := OpenWriter(filepath.Join(t.TempDir(), "legacy.db"))
		if err != nil {
			t.Fatalf("OpenWriter: %v", err)
		}
		defer lw.Close()

		// Схема мира на месте (иначе миграция данных 0007 не к чему приложить),
		// учёт — легаси-ключ только по version.
		if err := Migrate(lw, "literary"); err != nil {
			t.Fatalf("Migrate(literary): %v", err)
		}
		for _, q := range []string{
			`ALTER TABLE schema_migrations RENAME TO schema_migrations_new`,
			`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`,
			`INSERT INTO schema_migrations (version, name, applied_at) SELECT version, name, applied_at FROM schema_migrations_new`,
			`DROP TABLE schema_migrations_new`,
		} {
			if _, err := lw.Exec(q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
		legacy := []string{"0001_init", "0002_sediment_idempotency", "0003_composition", "0004_book", "0005_cast"}

		if err := Migrate(lw, "literary", "shared"); err != nil {
			t.Fatalf("Migrate поверх легаси-учёта: %v", err)
		}

		var oldRows int
		if err := lw.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name IN (?, ?, ?, ?, ?)`, legacy[0], legacy[1], legacy[2], legacy[3], legacy[4]).Scan(&oldRows); err != nil {
			t.Fatalf("schema_migrations легаси-строки: %v", err)
		}
		if oldRows != len(legacy) {
			t.Errorf("легаси-строк учёта = %d, want %d", oldRows, len(legacy))
		}
		var vectorsRows int
		if err := lw.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = '0002_vectors'`).Scan(&vectorsRows); err != nil {
			t.Fatalf("schema_migrations 0002_vectors: %v", err)
		}
		if vectorsRows != 1 {
			t.Errorf("строк учёта 0002_vectors после легаси-миграции = %d, want 1", vectorsRows)
		}
		// Поверх легаси-мира встаёт и 0006_book_deepread (T-14-03), и 0007_gefest_rename.
		if uv := userVersionOf(t, lw); uv != 7 {
			t.Errorf("user_version = %d, want 7", uv)
		}
	})
}

func userVersionOf(t *testing.T, w *sql.DB) int {
	t.Helper()
	var uv int
	if err := w.QueryRow(`PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	return uv
}
