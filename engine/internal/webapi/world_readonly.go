package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"tns65/internal/db"
)

// Мировые поля: чтение замороженных полей анкеты и запись только настраиваемых
// параметров мира (T-08-04, UI-04, UI-08.10, W-18.1).

const (
	worldFieldsPath   = "/api/world"
	worldTunablesPath = "/api/world/tunables"
)

// frozenWorldFields — 14 ключей анкеты рождения из T-02-01 (UI-04.5). После
// рождения эти поля не редактируются: маршрутов записи на них нет.
var frozenWorldFields = []string{
	"slug",
	"title",
	"description",
	"synopsis",
	"prologue",
	"lang",
	"travel_mode",
	"genres",
	"world_rules",
	"social_order",
	"economy_mods",
	"magic",
	"hero_birth",
	"censorship",
}

// tunableFields — настраиваемые параметры мира (UI-08.10, W-18.1); единственный
// пишущий маршрут world_meta — /api/world/tunables. Это не замороженные поля.
var tunableFields = []string{
	"globalLuck",
	"maxMemoryEntries",
	"similarityThreshold",
	"halfLifeDays",
	"dayNormKm",
}

var frozenFieldSet = stringSet(frozenWorldFields)
var tunableFieldSet = stringSet(tunableFields)

// worldDBPath — путь оперативной базы мира по слагу (db.WorldsRoot/worlds/<slug>/
// <slug>.db). Переменная для подмены в тестах.
var worldDBPath = func(slug string) string {
	return filepath.Join(db.WorldsRoot, "worlds", slug, slug+".db")
}

// openWorldForge открывает оперативную базу мира хэндлом forge: world_meta
// пишет только forge по карте писателей (T-01-07, B-03.8).
var openWorldForge = func(slug string) (*db.Handle, error) {
	return db.OpenWriterAs(worldDBPath(slug), db.DBWorld, db.WriterForge, nil)
}

// writeRouteForWorldField возвращает путь маршрута, записывающего поле
// world_meta. Замороженные поля пути записи не имеют (""), настраиваемые
// параметры пишутся только маршрутом /api/world/tunables.
func writeRouteForWorldField(field string) string {
	if frozenFieldSet[field] {
		return ""
	}
	if tunableFieldSet[field] {
		return worldTunablesPath
	}
	return ""
}

// isFrozenWorldField сообщает, заморожено ли поле анкеты (UI-04.5).
func isFrozenWorldField(field string) bool { return frozenFieldSet[field] }

// worldTunablesRequest — тело PUT /api/world/tunables.
type worldTunablesRequest struct {
	World    string                     `json:"world"`
	Tunables map[string]json.RawMessage `json:"tunables"`
}

// handleWorldReadonly — GET /api/world?world=<slug>: чтение замороженных мировых
// полей из world_meta (T-08-04, шаг 3). Записи нет.
func handleWorldReadonly(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("world"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}

	d, err := db.OpenReader(worldDBPath(slug))
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "world not found")
		return
	}
	defer d.Close()

	rows, err := d.Query(`SELECT "key", value FROM world_meta`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "world meta read failed")
		return
	}
	defer rows.Close()

	fields := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "world meta scan failed")
			return
		}
		if isFrozenWorldField(key) {
			fields[key] = value
		}
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "world meta read failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"world":  slug,
		"fields": fields,
	})
}

// handleWorldTunables — PUT /api/world/tunables: запись настраиваемых параметров
// мира хэндлом forge в world_meta (T-08-04, шаг 4).
func handleWorldTunables(w http.ResponseWriter, r *http.Request) {
	var req worldTunablesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad tunables body")
		return
	}

	slug := strings.TrimSpace(req.World)
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	if len(req.Tunables) == 0 {
		writeError(w, http.StatusBadRequest, codeBadRequest, "tunables required")
		return
	}
	for key := range req.Tunables {
		if !tunableFieldSet[key] {
			writeError(w, http.StatusBadRequest, codeBadRequest, "unknown tunable: "+key)
			return
		}
	}

	h, err := openWorldForge(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "world not found")
		return
	}
	defer h.Close()

	if err := writeWorldTunables(h, req.Tunables); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "tunables write failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// writeWorldTunables записывает параметры в world_meta хэндлом forge. Значения
// хранятся JSON-строками (T-02-03, шаг 3).
func writeWorldTunables(h *db.Handle, tunables map[string]json.RawMessage) error {
	for key, raw := range tunables {
		if !tunableFieldSet[key] {
			return fmt.Errorf("webapi: неизвестный настраиваемый параметр %q", key)
		}
		if _, err := h.Exec("world_meta",
			`INSERT INTO world_meta ("key", value) VALUES (?, ?)
			 ON CONFLICT("key") DO UPDATE SET value = excluded.value`,
			key, string(raw)); err != nil {
			return err
		}
	}
	return nil
}

func stringSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, s := range items {
		m[s] = true
	}
	return m
}
