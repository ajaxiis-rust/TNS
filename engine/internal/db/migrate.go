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

// Migrate применяет SQL-файлы из schemaFS (каталоги dirs, например "base", "shared")
// в порядке возрастания ведущего номера имени файла; при равных номерах — в порядке
// перечисления dirs. Уже применённые версии пропускаются. Каждая миграция выполняется
// в одной транзакции вместе со вставкой строки учёта в schema_migrations.
func Migrate(db *sql.DB, dirs ...string) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
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
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version).Scan(&exists); err != nil {
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
