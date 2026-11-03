package workshop

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"tns65/internal/db"
	"tns65/internal/models"
	"tns65/internal/structural"
)

// ForgeBookStatusReady — статус выкованной книги в реестре (P11 §5.2).
const ForgeBookStatusReady = "ready"

// forgeBookLang — язык ковки книги: композиция, банк и векторы (P11 §5.6).
const forgeBookLang = "ru"

// forgeBookEmbedModel — имя эмбеддинг-модели векторов чанков книги (T-02-07).
const forgeBookEmbedModel = models.DefaultEmbedModel

// forgeBookOwnerTable — owner_table векторов чанков книги (P11 §5.6, шаг 4).
const forgeBookOwnerTable = "bank_pool"

// forgeBookOrigin — origin записи банка ковки книги (P11 §5.6, шаг 2): одна
// ковка — одна запись банка; повторный запуск по origin не дублирует.
func forgeBookOrigin(bookID int64, n int) string {
	return fmt.Sprintf("forge-book:%d:%d", bookID, n)
}

// ForgeBookResult — исход ковки одной книги.
type ForgeBookResult struct {
	BookID   int64
	Chunks   int
	Progress float64
}

// ForgeBookOptions — зависимости ковки книги: реестр семей композиции и
// репортёр прогресса по чанкам.
type ForgeBookOptions struct {
	Families []structural.Family
	Progress func(done, total int)
}

// RunForgeBook куёт одну книгу (P11 §5.6): разбор входа, регистрация книги в
// реестре, по чанку — ForgeComposition с origin forge-book:<book_id>:<n>,
// привязка book_id и вектор чанка в vectors (owner_table='bank_pool'). Прогресс
// репортится по мере чанков; повторный запуск по origin не дублирует ни записи
// банка, ни векторы.
func RunForgeBook(ctx context.Context, world, lit *db.Handle, inputPath string, opts ForgeBookOptions) (ForgeBookResult, error) {
	var res ForgeBookResult
	if ctx == nil {
		ctx = context.Background()
	}

	parsed, err := ParseBookInput(inputPath)
	if err != nil {
		return res, err
	}

	name := parsed.Meta.Title
	if name == "" {
		name = filepath.Base(inputPath)
	}
	bookID, err := ensureForgeBook(lit, name)
	if err != nil {
		return res, err
	}
	res.BookID = bookID
	res.Chunks = len(parsed.Chunks)

	report := func(done int) {
		if len(parsed.Chunks) == 0 {
			res.Progress = 1
		} else {
			res.Progress = float64(done) / float64(len(parsed.Chunks))
		}
		if opts.Progress != nil {
			opts.Progress(done, len(parsed.Chunks))
		}
	}
	if len(parsed.Chunks) == 0 {
		report(0)
		return res, nil
	}

	for i, chunk := range parsed.Chunks {
		bankID, err := ForgeComposition(ctx, world, lit, ForgeMaterial{
			Origin: forgeBookOrigin(bookID, i),
			Kind:   "routine",
			Lang:   forgeBookLang,
			Text:   chunk.Text,
		}, opts.Families)
		if err != nil {
			return res, fmt.Errorf("workshop: ковка чанка %d: %w", i, err)
		}
		if err := attachBookToBank(lit, bankID, bookID); err != nil {
			return res, err
		}
		if err := writeBankVector(ctx, world, bankID, chunk.Text); err != nil {
			return res, err
		}
		report(i + 1)
	}
	return res, nil
}

// ensureForgeBook находит строку реестра книги по имени (не книгу игрока) или
// создаёт её со статусом ready; повторный запуск той же книги дубля не создаёт.
func ensureForgeBook(lit *db.Handle, name string) (int64, error) {
	var id int64
	err := lit.QueryRow(
		`SELECT id FROM book WHERE name = ? AND status != ? LIMIT 1`, name, BookStatusPlayer).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return InsertBook(lit, Book{Name: name, Lang: forgeBookLang, Status: ForgeBookStatusReady})
}

// attachBookToBank привязывает выкованную запись банка к книге реестра (P11
// §5.6, шаг 3): ForgeComposition пишет запись без book_id, привязку ставит ковка.
func attachBookToBank(lit *db.Handle, bankID, bookID int64) error {
	_, err := lit.Exec("bank_pool",
		`UPDATE bank_pool SET book_id = ? WHERE id = ?`, bookID, bankID)
	return err
}

// writeBankVector пишет вектор чанка в vectors мира (owner_table='bank_pool',
// P11 §5.6, шаг 4). Эмбеддер не подключён — вектор пропускается (резка без
// векторов, как в ходе); повторная ковка по origin вектор не дублирует.
func writeBankVector(ctx context.Context, world *db.Handle, bankID int64, text string) error {
	var n int
	if err := world.QueryRow(
		`SELECT COUNT(*) FROM vectors WHERE owner_table = ? AND owner_id = ?`,
		forgeBookOwnerTable, bankID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	emb := embedderForge()
	if emb == nil {
		return nil
	}
	vec, err := emb.Embed(ctx, text)
	if err != nil {
		return fmt.Errorf("workshop: вектор чанка книги: %w", err)
	}
	if len(vec) != models.EmbedDim {
		return fmt.Errorf("workshop: вектор чанка книги dim=%d, want %d", len(vec), models.EmbedDim)
	}

	_, err = world.Exec("vectors",
		`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		forgeBookOwnerTable, bankID, forgeBookEmbedModel, models.EmbedDim, forgeBookLang, text, horoscopeVecBlob(vec), nowUTC())
	return err
}
