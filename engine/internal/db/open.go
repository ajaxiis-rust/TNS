package db

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

// OpenWriter открывает базу SQLite для записи: WAL, busy_timeout=5000,
// foreign_keys включены (FK из 0001_init.sql действуют только при этой прагме),
// один коннект на писателя.
func OpenWriter(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// OpenReader открывает базу SQLite только на чтение: WAL, busy_timeout=5000,
// query_only(1). Запись через такой хэндл даёт ошибку SQLite
// «attempt to write a readonly database».
func OpenReader(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=query_only(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
