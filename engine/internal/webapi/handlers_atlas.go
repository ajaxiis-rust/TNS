package webapi

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"tns65/internal/birth"
	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/models"
	web "tns65/web"
)

// Атлас (T-08-05): список миров карточками, анкета рождения, мастер рождения,
// переключение активного мира, переигровка isekai и удаление мира. Чтение
// карточек без записи; рождение и переигровка идут через birth (T-02-03,
// T-02-12); удаление пишет событие world.deleted в журнал станка.

const (
	atlasScreenPath = "/atlas.html"
	atlasWorldsPath = "/api/atlas/worlds"
	atlasBirthPath  = "/api/atlas/worlds/birth"
	atlasActivePath = "/api/atlas/worlds/active"
	atlasReplayPath = "/api/atlas/worlds/replay"
	atlasDeletePath = "/api/atlas/worlds/delete"
)

// eventWorldDeleted — событие журнала станка при удалении мира (T-08-05,
// «Писатель»: удаление разрешено, писатель core).
const eventWorldDeleted = "world.deleted"

// Точки подмены для тестов: корень миров, base.db, эмбеддер и журнал.
var (
	atlasRoot   = db.WorldsRoot
	atlasBaseDB = "" // пусто — <корень миров>/base/base.db
	atlasEmbed  models.Embedder
	atlasLogger *journal.Logger
)

// activeSlug — активный мир интерфейса (выбор игрока). До T-08-15 хранится в
// памяти webapi; список миров помечает карточку активного мира.
var (
	activeMu   sync.Mutex
	activeSlug string
)

func setActiveSlug(s string) {
	activeMu.Lock()
	activeSlug = s
	activeMu.Unlock()
}

func getActiveSlug() string {
	activeMu.Lock()
	defer activeMu.Unlock()
	return activeSlug
}

func atlasRootDir() string {
	if atlasRoot == "" {
		return db.WorldsRoot
	}
	return atlasRoot
}

// handleAtlasScreen — GET /atlas.html: экран Атласа из embed (T-08-05, статика
// из бинарника).
func handleAtlasScreen(w http.ResponseWriter, _ *http.Request) {
	data, err := fs.ReadFile(web.FS, "atlas.html")
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "atlas not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// atlasWorldCard — карточка мира в списке (UI-05.1): имя мира, счётчики
// сущностей и сессий, признак активного мира.
type atlasWorldCard struct {
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	Active     bool   `json:"active"`
	Characters int    `json:"characters"`
	Places     int    `json:"places"`
	Paths      int    `json:"paths"`
	Entities   int    `json:"entities"`
	Sessions   int    `json:"sessions"`
}

type atlasListResponse struct {
	Active string           `json:"active"`
	Worlds []atlasWorldCard `json:"worlds"`
}

// handleAtlasList — GET /api/atlas/worlds: готовые миры карточками.
func handleAtlasList(w http.ResponseWriter, _ *http.Request) {
	worlds, err := birth.List(atlasRootDir())
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "world list failed")
		return
	}

	active := getActiveSlug()
	out := atlasListResponse{Active: active, Worlds: make([]atlasWorldCard, 0, len(worlds))}
	for _, wd := range worlds {
		card := atlasWorldCard{Slug: wd.Slug, Title: wd.Title, Active: wd.Slug == active}
		card.Characters, card.Places, card.Paths, card.Sessions = countWorld(wd)
		card.Entities = card.Characters + card.Places + card.Paths
		out.Worlds = append(out.Worlds, card)
	}
	writeJSON(w, http.StatusOK, out)
}

// countWorld считает сущности и сессии мира по оперативной базе. Ошибки чтения
// дают нулевые счётчики: карточка мира должна отображаться, а не ломать список.
func countWorld(w birth.World) (characters, places, paths, sessions int) {
	d, err := db.OpenReader(filepath.Join(w.Dir, w.Slug+".db"))
	if err != nil {
		return 0, 0, 0, 0
	}
	defer d.Close()
	_ = d.QueryRow(`SELECT COUNT(*) FROM characters`).Scan(&characters)
	_ = d.QueryRow(`SELECT COUNT(*) FROM places`).Scan(&places)
	_ = d.QueryRow(`SELECT COUNT(*) FROM paths`).Scan(&paths)
	_ = d.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions)
	return
}

// atlasBirthConfig собирает конфигурацию рождения для webapi. Эмбеддер в тестах
// подменяется фейком testkit (T-08-05, шаг 5).
func atlasBirthConfig(isekai bool) birth.Config {
	root := atlasRootDir()
	base := atlasBaseDB
	if base == "" {
		base = filepath.Join(root, "base", "base.db")
	}
	return birth.Config{
		WorldsRoot: root,
		PacksRoot:  "packs",
		BaseDBPath: base,
		Embed:      atlasEmbed,
		Logger:     atlasLogger,
		Isekai:     isekai,
	}
}

// decodeAncketa читает анкету рождения из тела запроса (14 ключей T-02-01).
func decodeAncketa(w http.ResponseWriter, r *http.Request) (birth.Ancketa, bool) {
	var a birth.Ancketa
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad ancketa body")
		return a, false
	}
	return a, true
}

// handleAtlasBirth — POST /api/atlas/worlds/birth: рождение нового мира по
// анкете (T-02-03). Ветка нового мира: повторного создания баз нет.
func handleAtlasBirth(w http.ResponseWriter, r *http.Request) {
	a, ok := decodeAncketa(w, r)
	if !ok {
		return
	}
	res, err := birth.Birth(&a, atlasBirthConfig(false))
	if err != nil {
		writeBirthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"world":  map[string]string{"slug": res.Slug, "title": a.Title},
	})
}

// handleAtlasReplay — POST /api/atlas/worlds/replay: переигровка isekai в
// существующем мире (T-02-12). Папка и базы не пересоздаются.
func handleAtlasReplay(w http.ResponseWriter, r *http.Request) {
	a, ok := decodeAncketa(w, r)
	if !ok {
		return
	}
	res, err := birth.Birth(&a, atlasBirthConfig(true))
	if err != nil {
		writeBirthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"world":  map[string]string{"slug": res.Slug, "title": a.Title},
	})
}

// atlasSwitchRequest — тело POST /api/atlas/worlds/active.
type atlasSwitchRequest struct {
	World string `json:"world"`
}

// handleAtlasSwitch — POST /api/atlas/worlds/active: переключение активного
// мира.
func handleAtlasSwitch(w http.ResponseWriter, r *http.Request) {
	var req atlasSwitchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad switch body")
		return
	}
	slug := strings.TrimSpace(req.World)
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	if !worldReady(slug) {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world not found")
		return
	}
	setActiveSlug(slug)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "active": slug})
}

// handleAtlasDelete — DELETE /api/atlas/worlds/delete?world=<slug>&confirm=<slug>:
// удаление мира целиком только по явному подтверждению (совпадение confirm и
// slug). Писатель core: каталог world_db/worlds/<slug>/ удаляется, событие
// world.deleted пишется в журнал.
func handleAtlasDelete(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("world"))
	confirm := strings.TrimSpace(r.URL.Query().Get("confirm"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	if confirm != slug {
		writeError(w, http.StatusBadRequest, codeBadRequest, "confirmation required")
		return
	}
	if !worldReady(slug) {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}

	if err := os.RemoveAll(filepath.Join(atlasRootDir(), "worlds", slug)); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "world delete failed")
		return
	}
	if getActiveSlug() == slug {
		setActiveSlug("")
	}
	if atlasLogger != nil {
		_ = atlasLogger.Event("webapi", eventWorldDeleted,
			journal.Field{Key: "world", Value: slug})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// worldReady сообщает, существует ли готовый мир с таким слагом.
func worldReady(slug string) bool {
	worlds, err := birth.List(atlasRootDir())
	if err != nil {
		return false
	}
	for _, w := range worlds {
		if w.Slug == slug {
			return true
		}
	}
	return false
}

// writeBirthError переводит ошибки рождения в контракт ошибок webapi: невалидная
// анкета — 400, повторный слаг — 409, мира для переигровки нет — 404.
func writeBirthError(w http.ResponseWriter, err error) {
	var ee *birth.ExitError
	if errors.As(err, &ee) {
		switch ee.Code {
		case birth.ExitInvalid:
			writeError(w, http.StatusBadRequest, codeBadRequest, err.Error())
		case birth.ExitDuplicate:
			writeError(w, http.StatusConflict, codeConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, codeInternal, err.Error())
		}
		return
	}
	if errors.Is(err, birth.ErrNoWorld) {
		writeError(w, http.StatusNotFound, codeNotFound, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, codeInternal, err.Error())
}
