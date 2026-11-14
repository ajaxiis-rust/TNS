package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed schema
var schemaFS embed.FS

// migrationsDDL — учёт миграций с составным ключом (version, name): общая
// миграция shared/0002_vectors делит номер 2 с literary/0002_sediment_idempotency,
// поэтому номер сам по себе идентичностью не является.
const migrationsDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER NOT NULL, name TEXT NOT NULL, applied_at TEXT NOT NULL, PRIMARY KEY (version, name))`

// Migrate применяет SQL-файлы из schemaFS (каталоги dirs, например "base", "shared")
// в порядке возрастания ведущего номера имени файла; при равных номерах — в порядке
// перечисления dirs. Уже применённые миграции (номер и имя файла) пропускаются.
// Каждая миграция выполняется в одной транзакции вместе со вставкой строки учёта
// в schema_migrations.
func Migrate(db *sql.DB, dirs ...string) error {
	if err := ensureMigrationsTable(db); err != nil {
		return err
	}

	type migration struct {
		version int
		name    string
		sql     string
		order   int
	}
	var migrations []migration

	for i, dir := range dirs {
		entries, err := fs.ReadDir(schemaFS, path.Join("schema", dir))
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
				continue
			}
			base := strings.TrimSuffix(e.Name(), ".sql")
			numStr := base
			if idx := strings.IndexByte(base, '_'); idx >= 0 {
				numStr = base[:idx]
			}
			num, err := strconv.Atoi(numStr)
			if err != nil {
				return fmt.Errorf("db: bad migration filename %q: %w", e.Name(), err)
			}
			content, err := schemaFS.ReadFile(path.Join("schema", dir, e.Name()))
			if err != nil {
				return err
			}
			migrations = append(migrations, migration{
				version: num,
				name:    base,
				sql:     string(content),
				order:   i,
			})
		}
	}

	sort.SliceStable(migrations, func(i, j int) bool {
		if migrations[i].version != migrations[j].version {
			return migrations[i].version < migrations[j].version
		}
		return migrations[i].order < migrations[j].order
	})

	for _, m := range migrations {
		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ? AND name = ?`, m.version, m.name).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(m.sql); err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
			m.version, m.name, time.Now().UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	var maxVersion int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&maxVersion); err != nil {
		return err
	}
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, maxVersion)); err != nil {
		return err
	}
	return nil
}

// ensureMigrationsTable создаёт таблицу учёта и перестраивает легаси-версию
// (PRIMARY KEY только по version, T-01-04) под составной ключ, сохраняя строки.
// Без перестройки общая миграция shared/0002_vectors не встала бы в литературную
// базу, где номер 2 уже занят 0002_sediment_idempotency.
func ensureMigrationsTable(db *sql.DB) error {
	if _, err := db.Exec(migrationsDDL); err != nil {
		return err
	}
	legacy, err := legacyMigrationsKey(db)
	if err != nil {
		return err
	}
	if !legacy {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, q := range []string{
		`ALTER TABLE schema_migrations RENAME TO schema_migrations_legacy`,
		migrationsDDL,
		`INSERT INTO schema_migrations (version, name, applied_at) SELECT version, name, applied_at FROM schema_migrations_legacy`,
		`DROP TABLE schema_migrations_legacy`,
	} {
		if _, err := tx.Exec(q); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// legacyMigrationsKey сообщает, что schema_migrations держит ключ только по version.
func legacyMigrationsKey(db *sql.DB) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(schema_migrations)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	pkVersion, pkName := false, false
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if pk > 0 {
			switch name {
			case "version":
				pkVersion = true
			case "name":
				pkName = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return pkVersion && !pkName, nil
}
