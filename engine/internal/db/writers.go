package db

import (
	"database/sql"
	"errors"
	"fmt"

	"tns65/internal/journal"
)

// Writer — идентичность писателя по карте (T-01-07, шаг 1).
type Writer string

const (
	WriterCore     Writer = "core"
	WriterForge    Writer = "forge"
	WriterSediment Writer = "sediment"
	WriterBake     Writer = "bake"
)

// DB — идентичность базы в карте писателей.
type DB string

const (
	DBBase     DB = "base"
	DBLiterary DB = "literary"
	DBWorld    DB = "world"
)

// ErrForeignWriter — запись в таблицу не тем писателем (или таблица вне карты).
var ErrForeignWriter = errors.New("db: foreign writer")

// ForeignWriterError — конкретное нарушение карты писателей.
type ForeignWriterError struct {
	DB     DB
	Table  string
	Writer Writer
}

func (e *ForeignWriterError) Error() string {
	return fmt.Sprintf("db: writer %q cannot write table %q of db %s", e.Writer, e.Table, e.DB)
}

func (e *ForeignWriterError) Is(target error) bool {
	return target == ErrForeignWriter
}

// writerKey — таблица одной базы.
type writerKey struct {
	db    DB
	table string
}

// writerMap — «таблица → допустимый писатель» по трём базам (T-01-07, шаг 2).
// schema_migrations сюда не входит: её пишет только мигратор (инфраструктура),
// доменным писателям она не принадлежит.
var writerMap = map[writerKey]Writer{
	// base.db — все таблицы пишет bake (D-09.1).
	{DBBase, "narratives"}: WriterBake,
	{DBBase, "verses"}:     WriterBake,
	{DBBase, "patterns"}:   WriterBake,
	{DBBase, "characters"}: WriterBake,
	{DBBase, "vectors"}:    WriterBake,

	// literary.db.
	{DBLiterary, "bank_pool"}:       WriterForge,
	{DBLiterary, "lit_template"}:    WriterForge,
	{DBLiterary, "horoscope_issue"}: WriterForge,
	{DBLiterary, "forge_log"}:       WriterForge,
	{DBLiterary, "artifact"}:        WriterForge,
	{DBLiterary, "sediment_scene"}:  WriterSediment,

	// world db: places, paths, world_meta — forge; остальное — core (D-09.7).
	{DBWorld, "places"}:           WriterForge,
	{DBWorld, "paths"}:            WriterForge,
	{DBWorld, "world_meta"}:       WriterForge,
	{DBWorld, "vectors"}:          WriterCore,
	{DBWorld, "turns"}:            WriterCore,
	{DBWorld, "scenes"}:           WriterCore,
	{DBWorld, "events"}:           WriterCore,
	{DBWorld, "sessions"}:         WriterCore,
	{DBWorld, "characters"}:       WriterCore,
	{DBWorld, "character_layers"}: WriterCore,
	{DBWorld, "character_state"}:  WriterCore,
	{DBWorld, "memory_topics"}:    WriterCore,
	{DBWorld, "token_bindings"}:   WriterCore,
	{DBWorld, "model_bindings"}:   WriterCore,
	{DBWorld, "player_atlas"}:     WriterCore,
	{DBWorld, "player_map"}:       WriterCore,
	{DBWorld, "economy"}:          WriterCore,
	{DBWorld, "social_graph"}:     WriterCore,
	{DBWorld, "book_chapters"}:    WriterCore,
	{DBWorld, "book_text"}:        WriterCore,
	{DBWorld, "book_cuts"}:        WriterCore,
	{DBWorld, "collisions"}:       WriterCore,
	{DBWorld, "deferred_events"}:  WriterCore,
	{DBWorld, "rings"}:            WriterCore,
	{DBWorld, "corridors"}:        WriterCore,
}

// writerForTable возвращает допустимого писателя таблицы table базы dbName.
func writerForTable(dbName DB, table string) (Writer, bool) {
	w, ok := writerMap[writerKey{db: dbName, table: table}]
	return w, ok
}

// Exec выполняет запись в таблицу table. Запись проходит, только если писатель
// хэндла допущен к table по карте; иначе — ErrForeignWriter и событие
// db.foreign_writer (шаги 3–4).
func (h *Handle) Exec(table, query string, args ...any) (sql.Result, error) {
	if err := h.check(table); err != nil {
		return nil, err
	}
	return h.db.Exec(query, args...)
}

func (h *Handle) check(table string) error {
	allowed, ok := writerForTable(h.dbName, table)
	if !ok || allowed != h.writer {
		h.logForeign(table)
		return &ForeignWriterError{DB: h.dbName, Table: table, Writer: h.writer}
	}
	return nil
}

func (h *Handle) logForeign(table string) {
	if h.logger == nil {
		return
	}
	_ = h.logger.Event("db", journal.EventDBForeignWriter,
		journal.Field{Key: "table", Value: table},
		journal.Field{Key: "writer", Value: string(h.writer)})
}

// Tx — транзакция записей от писателя writer. Каждая запись в fn сверяется
// с картой; ошибка любой записи откатывает всю транзакцию (шаг 3).
func (h *Handle) Tx(writer Writer, fn func(tx *Tx) error) error {
	if writer != h.writer {
		return &ForeignWriterError{DB: h.dbName, Table: "", Writer: writer}
	}
	t, err := h.db.Begin()
	if err != nil {
		return err
	}
	tx := &Tx{tx: t, dbName: h.dbName, writer: writer, logger: h.logger}
	if err := fn(tx); err != nil {
		_ = t.Rollback()
		return err
	}
	return t.Commit()
}

// Tx — транзакционная запись в одну базу.
type Tx struct {
	tx     *sql.Tx
	dbName DB
	writer Writer
	logger *journal.Logger
}

// Exec выполняет запись в таблицу table внутри транзакции, сверяя с картой.
func (tx *Tx) Exec(table, query string, args ...any) (sql.Result, error) {
	allowed, ok := writerForTable(tx.dbName, table)
	if !ok || allowed != tx.writer {
		if tx.logger != nil {
			_ = tx.logger.Event("db", journal.EventDBForeignWriter,
				journal.Field{Key: "table", Value: table},
				journal.Field{Key: "writer", Value: string(tx.writer)})
		}
		return nil, &ForeignWriterError{DB: tx.dbName, Table: table, Writer: tx.writer}
	}
	return tx.tx.Exec(query, args...)
}
