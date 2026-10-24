package db

import (
	"database/sql"

	"tns65/internal/journal"

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

// Handle — открытая база с тегом писателя (T-01-07). Запись — только через
// Handle.Exec/Tx (writers.go), которые сверяют таблицу с картой писателей.
type Handle struct {
	db     *sql.DB
	dbName DB
	writer Writer
	logger *journal.Logger
}

// OpenWriterAs открывает базу на запись и помечает хэндл тегом dbName/writer.
func OpenWriterAs(path string, dbName DB, writer Writer, logger *journal.Logger) (*Handle, error) {
	db, err := OpenWriter(path)
	if err != nil {
		return nil, err
	}
	return &Handle{db: db, dbName: dbName, writer: writer, logger: logger}, nil
}

// OpenReaderAs открывает базу только на чтение и помечает хэндл тегом.
func OpenReaderAs(path string, dbName DB, writer Writer, logger *journal.Logger) (*Handle, error) {
	db, err := OpenReader(path)
	if err != nil {
		return nil, err
	}
	return &Handle{db: db, dbName: dbName, writer: writer, logger: logger}, nil
}

// Close закрывает базу.
func (h *Handle) Close() error { return h.db.Close() }

// Query — чтение произвольным SQL (запись идёт только через Exec/Tx).
func (h *Handle) Query(query string, args ...any) (*sql.Rows, error) {
	return h.db.Query(query, args...)
}

// QueryRow — чтение одной строки.
func (h *Handle) QueryRow(query string, args ...any) *sql.Row {
	return h.db.QueryRow(query, args...)
}
