package webapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tns65/internal/db"
	"tns65/internal/library"
	"tns65/internal/structural"
	"tns65/internal/workshop"
)

// Синхронная ковка книги-источника фанфика после рождения (P11 §5.7, T-12-14).
// Рождение завершено состоянием ready; ковка книги идёт отдельным шагом до
// первого хода и ставит метку language_ready. Нет модели — предупреждение, мир
// открывается на сиде (language_ready ставится и при отказе ковки).

const (
	atlasForgePath    = "/api/atlas/worlds/forge"
	languageReadyFile = "language_ready"
)

// forgeBookRun — точка подмены ковки для тестов: та же цепочка RunForgeBook,
// что и однозадачная задача swarm-task forge-book (cmd/tns65/main.go).
var forgeBookRun = func(world, lit *db.Handle, inputPath string, progress func(done, total int)) error {
	fams, _ := structural.LoadFamilies(filepath.Join(db.BaseRoot, "archetypes.json"))
	_, err := workshop.RunForgeBook(context.Background(), world, lit, inputPath, workshop.ForgeBookOptions{
		Families: fams,
		Progress: progress,
	})
	return err
}

// languageReadyPath — путь метки language_ready мира.
func languageReadyPath(slug string) string {
	return filepath.Join(atlasRootDir(), "worlds", slug, languageReadyFile)
}

// markLanguageReady ставит метку language_ready: первый ход разрешён.
func markLanguageReady(slug string) error {
	return os.WriteFile(languageReadyPath(slug), []byte("1\n"), 0o644)
}

// worldLanguageReady сообщает, готов ли язык книги мира к первому ходу.
func worldLanguageReady(slug string) bool {
	_, err := os.Stat(languageReadyPath(slug))
	return err == nil
}

// forgeResponse — ответ синхронной ковки книги после рождения.
type forgeResponse struct {
	Status        string  `json:"status"`
	LanguageReady bool    `json:"language_ready"`
	Warning       string  `json:"warning,omitempty"`
	Progress      float64 `json:"progress"`
}

// handleAtlasForge — POST /api/atlas/worlds/forge: синхронная ковка книги
// фанфика после рождения (T-12-14). Не фанфик — метка language_ready сразу;
// отказ ковки (нет модели) — предупреждение, мир открывается на сиде.
func handleAtlasForge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		World string `json:"world"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad forge body")
		return
	}
	slug := strings.TrimSpace(req.World)
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	if !worldReady(slug) {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}

	out := forgeResponse{Status: "ok", LanguageReady: true, Progress: 1}

	sourceBookID, err := readSourceBookID(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "world open failed")
		return
	}
	if sourceBookID != 0 {
		if err := forgeSourceBook(slug, sourceBookID, &out.Progress); err != nil {
			out.Warning = "язык книги ещё готовится, играем на сиде: " + err.Error()
			out.Progress = 0
		}
	}

	if err := markLanguageReady(slug); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "language ready mark failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// readSourceBookID читает source_book_id из world_meta (записан при рождении
// хэндлом forge). Ключа нет — не фанфик, ковка не нужна.
func readSourceBookID(slug string) (int64, error) {
	d, err := db.OpenReader(worldDBPath(slug))
	if err != nil {
		return 0, err
	}
	defer d.Close()

	var raw string
	if err := d.QueryRow(`SELECT value FROM world_meta WHERE "key" = 'source_book_id'`).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil // ключа нет — не фанфик.
		}
		return 0, err
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("world_meta.source_book_id: %w", err)
	}
	return id, nil
}

// forgeSourceBook куёт книгу-источник по id реестра и репортит прогресс в
// манифест полки. Ошибка — отказ ковки (нет модели и т. п.), мир всё равно
// открывается на сиде.
func forgeSourceBook(slug string, bookID int64, progress *float64) error {
	world, err := db.OpenWriterAs(worldDBPath(slug), db.DBWorld, db.WriterCore, atlasLogger)
	if err != nil {
		return err
	}
	defer world.Close()

	lit, err := db.OpenWriterAs(litDBPath(slug), db.DBLiterary, db.WriterForge, atlasLogger)
	if err != nil {
		return err
	}
	defer lit.Close()

	book, err := workshop.BookByID(lit, bookID)
	if err != nil {
		return err
	}
	if book.ShelfID == "" {
		return errors.New("книга-источник не привязана к полке")
	}

	shelf, err := shelfBookByID(book.ShelfID)
	if err != nil {
		return err
	}
	inputPath := filepath.Join(libraryDir(), "books", shelf.ID, shelf.Name+"."+shelf.Format)
	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("файл книги: %w", err)
	}

	_ = updateShelfForge(shelf.ID, library.StatusForging, 0)
	err = forgeBookRun(world, lit, inputPath, func(done, total int) {
		p := 0.0
		if total > 0 {
			p = float64(done) / float64(total)
		}
		if progress != nil {
			*progress = p
		}
		_ = updateShelfForge(shelf.ID, library.StatusForging, p)
	})
	if err != nil {
		return err
	}
	_ = updateShelfForge(shelf.ID, library.StatusReady, 1)
	if progress != nil {
		*progress = 1
	}
	return nil
}

// shelfBookByID возвращает книгу полки по её id (shelf_id реестра книги).
func shelfBookByID(id string) (library.Book, error) {
	books, err := library.Load(libraryDir())
	if err != nil {
		return library.Book{}, err
	}
	for _, b := range books {
		if b.ID == id {
			return b, nil
		}
	}
	return library.Book{}, fmt.Errorf("книга полки %q не найдена", id)
}

// updateShelfForge обновляет статус и прогресс книги полки (library.json);
// ошибка записи ковку не гасит — прогресс вторичен.
func updateShelfForge(id, status string, progress float64) error {
	books, err := library.Load(libraryDir())
	if err != nil {
		return err
	}
	changed := false
	for i := range books {
		if books[i].ID != id {
			continue
		}
		books[i].Status = status
		books[i].Progress = progress
		changed = true
		break
	}
	if !changed {
		return nil
	}
	return library.Save(libraryDir(), books)
}
