package webapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/paganel"
	"tns65/internal/swarm"
	"tns65/internal/version"
	"tns65/internal/workshop"
	web "tns65/web"
)

// Вкладка «MCP» (T-08-08, UI-07): дашборд ковки, книги-источники и кольца
// Паганеля. webapi только запускает ковку и читает состояние; писатель ковки —
// Гефест/Forge, сырьё книг живёт файлами в папке мира, в базы входит только
// выкованное.

const (
	mcpDashPath    = "/api/mcp/dash"
	mcpSourcesPath = "/api/mcp/sources"
	mcpImportPath  = "/api/mcp/import"
	mcpForgePath   = "/api/mcp/forge"
	mcpSourcePath  = "/api/mcp/source"
	mcpRingsPath   = "/api/mcp/rings"
)

// Статусы карточки книги-источника (UI-07.3): в очереди / куется N% / готова /
// ошибка. Ошибка — сырьё не прошло конвейер.
const (
	sourceStatusQueued  = "queued"
	sourceStatusForging = "forging"
	sourceStatusDone    = "done"
	sourceStatusError   = "error"
)

// mcpRoot — корень миров; в тестах подменяется t.TempDir().
var mcpRoot = db.WorldsRoot

// mcpHub — WS-хаб для живой рассылки событий вкладки MCP (UI-07.2). Подключается
// при сборке роутера в routes.go.
var mcpHub *Hub

// mcpLogger — журнал станка для однозадачного запуска ковки; подключается при
// сборке сервера в server.go.
var mcpLogger *journal.Logger

// mcpTopic — WS-топик экрана MCP; клиент подписывается на него в app.js.
const mcpTopic = "mcp"

// forgeLaunch — запуск одной ковки через рой (T-05-06, шаг 4): одна ковка —
// один однозадачный запуск swarm-task forge-book. Точка подмены для тестов.
type forgeLaunchFunc func(logger *journal.Logger, spec swarm.Spec, coreVersion string) (swarm.Result, error)

var forgeLaunch forgeLaunchFunc = swarm.Launch

// broadcastMcp рассылает событие изменения MCP-данных подписчикам топика mcp.
func broadcastMcp(slug, eventType string) {
	if mcpHub == nil {
		return
	}
	payload, _ := json.Marshal(map[string]string{"world": slug})
	mcpHub.Broadcast(WSMessage{Topic: mcpTopic, EventType: eventType, Payload: payload})
}

// litDBPath — литературная база мира. Рядом с оперативной базой <slug>.db
// лежит literary.db (T-02-03).
var litDBPath = func(slug string) string {
	return filepath.Join(filepath.Dir(worldDBPath(slug)), "literary.db")
}

// mcpWorldDir возвращает папку мира по слагу.
func mcpWorldDir(slug string) string {
	return filepath.Join(mcpRoot, "worlds", slug)
}

// mcpSourcesDir возвращает папку сырья мира.
func mcpSourcesDir(slug string) string {
	return filepath.Join(mcpWorldDir(slug), "sources")
}

// sourceManifest — состояние сырья книг: имя, формат, статус конвейера,
// прогресс и счётчики глав/шаблонов. Живёт файлом в папке мира, базы не трогает.
type sourceManifest struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Format        string  `json:"format"`
	Status        string  `json:"status"`
	Progress      float64 `json:"progress"`
	Chapters      int64   `json:"chapters"`
	Templates     int64   `json:"templates"`
	CensorRejects int64   `json:"censor_rejects"`
	DelexReplaces int64   `json:"delex_replaces"`
}

const sourceManifestName = "sources.json"

// readSourceManifest читает манифест сырья; файла нет — пустой список.
func readSourceManifest(slug string) ([]sourceManifest, error) {
	data, err := os.ReadFile(filepath.Join(mcpSourcesDir(slug), sourceManifestName))
	if errors.Is(err, os.ErrNotExist) {
		return []sourceManifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out []sourceManifest
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []sourceManifest{}
	}
	return out, nil
}

// writeSourceManifest записывает манифест сырья атомарно: временный файл и
// rename, чтобы падение не оставило половину файла.
func writeSourceManifest(slug string, items []sourceManifest) error {
	dir := mcpSourcesDir(slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, sourceManifestName+".tmp")
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, sourceManifestName))
}

// mcpDashResponse — дашборд ковки: очередь и текущая работа плюс статистика
// ковки из literary.db.
type mcpDashResponse struct {
	World   string        `json:"world"`
	Queue   int           `json:"queue"`
	Current float64       `json:"current"`
	Stats   mcpForgeStats `json:"stats"`
}

type mcpForgeStats struct {
	BooksProcessed int64 `json:"books_processed"`
	ChaptersParsed int64 `json:"chapters_parsed"`
	Templates      int64 `json:"templates"`
	CensorRejects  int64 `json:"censor_rejects"`
	DelexReplaces  int64 `json:"delex_replaces"`
	Errors         int64 `json:"errors"`
}

// mcpSourcesResponse — список карточек книг-источников.
type mcpSourcesResponse struct {
	World   string           `json:"world"`
	Sources []sourceManifest `json:"sources"`
}

// handleMcpDash — GET /api/mcp/dash?world=<slug>: живое состояние ковки.
func handleMcpDash(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("world"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	world, lit, err := openMcpReaders(slug)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	defer world.Close()
	defer lit.Close()

	items, err := readSourceManifest(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "source manifest read failed")
		return
	}
	current, err := readCurrentForgeProgress(world, items)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "forge progress read failed")
		return
	}
	stats, err := readForgeStats(lit, items)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "forge stats read failed")
		return
	}
	writeJSON(w, http.StatusOK, mcpDashResponse{
		World:   slug,
		Queue:   countQueuedSources(items),
		Current: current,
		Stats:   stats,
	})
}

// openMcpReaders открывает оперативную и литературную базы мира только на чтение.
func openMcpReaders(slug string) (*db.Handle, *db.Handle, error) {
	world, err := db.OpenReaderAs(worldDBPath(slug), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		return nil, nil, err
	}
	lit, err := db.OpenReaderAs(litDBPath(slug), db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		_ = world.Close()
		return nil, nil, err
	}
	return world, lit, nil
}

// countQueuedSources — очередь наполнения в штуках: книги-источники, ждущие
// ковки (статус queued). Ковка не ставит заказов в очередь мастерской.
func countQueuedSources(items []sourceManifest) int {
	n := 0
	for _, it := range items {
		if it.Status == sourceStatusQueued {
			n++
		}
	}
	return n
}

// readCurrentForgeProgress — процент текущей работы: прогресс книги в статусе
// forging, иначе прогресс текущего кольца Паганеля (0, если работы нет).
func readCurrentForgeProgress(world *db.Handle, items []sourceManifest) (float64, error) {
	for _, it := range items {
		if it.Status == sourceStatusForging {
			return it.Progress, nil
		}
	}
	var progress sql.NullFloat64
	err := world.QueryRow(
		`SELECT progress FROM rings WHERE status = 'running' ORDER BY id LIMIT 1`).Scan(&progress)
	if errors.Is(err, sql.ErrNoRows) || !progress.Valid {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return progress.Float64, nil
}

// readForgeStats считает статистику ковки из реальных источников: карточки
// книг-источников (манифест сырья), литературная база lit_template и журнал
// ковки fact_check/mismatch (единственный producer записей forge-book).
func readForgeStats(lit *db.Handle, items []sourceManifest) (mcpForgeStats, error) {
	var s mcpForgeStats
	for _, it := range items {
		switch it.Status {
		case sourceStatusDone:
			s.BooksProcessed++
			s.ChaptersParsed += it.Chapters
		case sourceStatusError:
			s.Errors++
		}
		s.CensorRejects += it.CensorRejects
		s.DelexReplaces += it.DelexReplaces
	}
	if err := lit.QueryRow(`SELECT COUNT(*) FROM lit_template`).Scan(&s.Templates); err != nil {
		return s, err
	}
	var mismatches int64
	if err := lit.QueryRow(
		`SELECT COUNT(*) FROM forge_log WHERE kind = 'forge-book' AND task = 'fact_check' AND status = 'mismatch'`).Scan(&mismatches); err != nil {
		return s, err
	}
	s.Errors += mismatches
	return s, nil
}

// handleMcpSources — GET /api/mcp/sources?world=<slug>: карточки книг-источников.
func handleMcpSources(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("world"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	if !mcpWorldReady(slug) {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	items, err := readSourceManifest(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "source manifest read failed")
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	writeJSON(w, http.StatusOK, mcpSourcesResponse{World: slug, Sources: items})
}

// handleMcpImport — POST /api/mcp/import (multipart/form-data, поле file):
// принимает скачанный файл книги txt/md/fb2, кладёт его в папку сырья мира.
// Повторный импорт того же файла — отказ с причиной, дублей нет.
func handleMcpImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad multipart body")
		return
	}
	slug := strings.TrimSpace(r.FormValue("world"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	if !mcpWorldReady(slug) {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "file field required")
		return
	}
	defer file.Close()

	name := filepath.Base(header.Filename)
	format, ok := sourceFormat(name)
	if !ok {
		writeError(w, http.StatusBadRequest, codeBadRequest, "unsupported format: only txt, md and fb2 are accepted")
		return
	}

	items, err := readSourceManifest(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "source manifest read failed")
		return
	}
	for _, it := range items {
		if it.Name == name {
			writeError(w, http.StatusConflict, codeConflict, "source already imported: "+name)
			return
		}
	}

	dir := mcpSourcesDir(slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "sources dir failed")
		return
	}
	dst := filepath.Join(dir, name)
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "source create failed")
		return
	}
	if _, err := io.Copy(f, file); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		writeError(w, http.StatusInternalServerError, codeInternal, "source write failed")
		return
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		writeError(w, http.StatusInternalServerError, codeInternal, "source write failed")
		return
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		writeError(w, http.StatusInternalServerError, codeInternal, "source write failed")
		return
	}

	items = append(items, sourceManifest{
		ID:       name,
		Name:     name,
		Format:   format,
		Status:   sourceStatusQueued,
		Progress: 0,
		Chapters: parseSourceChapters(dst, format),
	})
	if err := writeSourceManifest(slug, items); err != nil {
		_ = os.Remove(dst)
		writeError(w, http.StatusInternalServerError, codeInternal, "source manifest write failed")
		return
	}
	broadcastMcp(slug, "import")
	writeJSON(w, http.StatusOK, mcpSourcesResponse{World: slug, Sources: items})
}

// parseSourceChapters считает главы книги сразу при импорте через вход ковки
// (T-05-06): txt напрямую чанкером, fb2 вставкой перед чанкером. При отказе
// разбора счётчик остаётся нулём — карточка не ломается.
func parseSourceChapters(path, format string) int64 {
	if format != "txt" && format != "fb2" {
		return 0
	}
	parsed, err := workshop.ParseBookInput(path)
	if err != nil {
		return 0
	}
	return int64(len(parsed.Chunks))
}

// sourceFormat возвращает формат карточки по расширению (txt/md/fb2).
func sourceFormat(name string) (string, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt":
		return "txt", true
	case ".md":
		return "md", true
	case ".fb2":
		return "fb2", true
	default:
		return "", false
	}
}

// handleMcpForge — POST /api/mcp/forge: запускает ковку всех книг в очереди.
// Каждая книга — отдельный однозадачный запуск через рой (ForgeBookSpec →
// swarm.Launch), как forge-card (T-05-06); очередь мастерской не трогается.
func handleMcpForge(w http.ResponseWriter, r *http.Request) {
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
	if !mcpWorldReady(slug) {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}

	exe, err := os.Executable()
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "executable path failed")
		return
	}

	items, err := readSourceManifest(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "source manifest read failed")
		return
	}

	changed := false
	for i := range items {
		if items[i].Status != sourceStatusQueued {
			continue
		}
		spec := workshop.ForgeBookSpec(exe, workshop.ForgeBookPayload{
			InputPath: filepath.Join(mcpSourcesDir(slug), items[i].Name),
			WorldSlug: slug,
		})
		if _, err := forgeLaunch(mcpLogger, spec, version.Version); err != nil {
			items[i].Status = sourceStatusError
			items[i].Progress = 0
		} else {
			items[i].Status = sourceStatusDone
			items[i].Progress = 1
		}
		changed = true
	}
	if changed {
		if err := writeSourceManifest(slug, items); err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "source manifest write failed")
			return
		}
		broadcastMcp(slug, "forge")
	}
	writeJSON(w, http.StatusOK, mcpSourcesResponse{World: slug, Sources: items})
}

// handleMcpSourceDelete — DELETE /api/mcp/source/{id}?world=<slug>: удаляет
// файл сырья и его карточку; выкованное в базах не трогает.
func handleMcpSourceDelete(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("world"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	if !mcpWorldReady(slug) {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, mcpSourcePath+"/"))
	if id == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "source id required")
		return
	}

	items, err := readSourceManifest(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "source manifest read failed")
		return
	}
	idx := -1
	for i := range items {
		if items[i].ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, codeNotFound, "source not found")
		return
	}
	name := items[idx].Name
	if err := os.Remove(filepath.Join(mcpSourcesDir(slug), name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, codeInternal, "source remove failed")
		return
	}
	items = append(items[:idx], items[idx+1:]...)
	if err := writeSourceManifest(slug, items); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "source manifest write failed")
		return
	}
	broadcastMcp(slug, "delete")
	writeJSON(w, http.StatusOK, mcpSourcesResponse{World: slug, Sources: items})
}

// handleMcpRings — GET /api/mcp/rings?world=<slug>: журнал колец Паганеля и
// статистика draft → baked. Только чтение, переключателей в контракте нет.
func handleMcpRings(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("world"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	h, err := db.OpenReaderAs(worldDBPath(slug), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	defer h.Close()

	j, err := paganel.CollectRingJournal(h)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "ring journal read failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"world":   slug,
		"journal": j,
	})
}

// mcpWorldReady сообщает, существует ли мир с таким слагом: под ним есть папка
// worlds/<slug>/. Записей в базу нет — проверка только по раскладке каталога.
func mcpWorldReady(slug string) bool {
	_, err := os.Stat(filepath.Join(mcpRoot, "worlds", slug))
	return err == nil
}

// handleMcpScreen — GET /mcp.html: экран вкладки MCP из embed (статика из
// бинарника). app.js загружает вкладку через fetch("/mcp.html").
func handleMcpScreen(w http.ResponseWriter, _ *http.Request) {
	data, err := fs.ReadFile(web.FS, "mcp.html")
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "mcp screen not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
