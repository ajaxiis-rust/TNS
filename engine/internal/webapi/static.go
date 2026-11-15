package webapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"tns65/internal/journal"
	web "tns65/web"
)

// Режимы источника веб-статики (IMP-12): каталог web/ рядом с бинарником либо
// встроенный фронт.
const (
	staticModeDisk  = "disk"
	staticModeEmbed = "embed"
)

// requiredWebFiles — полный состав фронта. Каталог web/ на диске считается
// источником, только если на месте каждая страница и static/app.js: неполный
// каталог (оборванный rsync) отдал бы 404 на экранах, поэтому — фолбэк на embed.
var requiredWebFiles = []string{
	"index.html",
	"atlas.html",
	"history.html",
	"mcp.html",
	"settings.html",
	"wizard.html",
	"static/app.js",
}

// staticWeb — источник статики фронта: каталог web/ на диске (disk) или
// встроенный embed.FS (embed).
type staticWeb struct {
	mode   string
	fsys   fs.FS
	static http.Handler
}

// currentStatic — действующий источник статики; до старта сервера — embed.
var currentStatic = embedStatic()

// embedStatic собирает источник из встроенного фронта.
func embedStatic() *staticWeb {
	sub, err := fs.Sub(web.FS, "static")
	if err != nil {
		panic(err)
	}
	return &staticWeb{
		mode:   staticModeEmbed,
		fsys:   web.FS,
		static: http.StripPrefix("/static/", http.FileServer(http.FS(sub))),
	}
}

// diskStatic собирает источник из каталога web/ на диске. firstMissing — первый
// отсутствующий файл фронта; каталога нет — возвращает пустую строку (штатная
// поставка без web/).
func diskStatic(dir string) (src *staticWeb, firstMissing string) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, ""
	}
	for _, name := range requiredWebFiles {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			return nil, name
		}
	}
	root := os.DirFS(dir)
	sub, err := fs.Sub(root, "static")
	if err != nil {
		return nil, "static"
	}
	return &staticWeb{
		mode:   staticModeDisk,
		fsys:   root,
		static: http.StripPrefix("/static/", http.FileServer(http.FS(sub))),
	}, ""
}

// resolveStatic выбирает источник статики и пишет в журнал web.static.source:
// полный каталог web/ (явно заданный или рядом с бинарником) — диск, иначе
// embed. В disk-режиме дополнительно сверяет файлы с манифестом сборки.
func resolveStatic(webDir string, logger *journal.Logger) *staticWeb {
	dir := webDir
	if dir == "" {
		if exe, err := os.Executable(); err == nil {
			dir = filepath.Join(filepath.Dir(exe), "web")
		}
	}
	src, missing := diskStatic(dir)
	if src == nil {
		src = embedStatic()
	}
	if logger != nil {
		fields := []journal.Field{{Key: "source", Value: src.mode}}
		if dir != "" {
			fields = append(fields, journal.Field{Key: "dir", Value: dir})
		}
		if missing != "" {
			fields = append(fields, journal.Field{Key: "missing", Value: missing})
		}
		_ = logger.Event("webapi", journal.EventWebStaticSource, fields...)
	}
	if src.mode == staticModeDisk {
		warnStaticDrift(src, logger)
	}
	return src
}

// webManifest — манифест фронта: sha256 каждого файла web/ на момент сборки
// (генерит scripts/build.sh, паттерн packs/ru/manifest.json).
type webManifest struct {
	ID    string            `json:"id"`
	Files map[string]string `json:"files"`
}

// warnStaticDrift пишет событие web.static.drift на каждый файл диска, чей хэш
// разошёлся с манифестом: фронт на станции новее бинарника. Раздача при этом не
// меняется — сигнал, а не отказ.
func warnStaticDrift(src *staticWeb, logger *journal.Logger) {
	if logger == nil {
		return
	}
	raw, err := fs.ReadFile(src.fsys, "manifest.json")
	if err != nil {
		return
	}
	var man webManifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return
	}
	names := make([]string, 0, len(man.Files))
	for name := range man.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		got := fsSHA256(src.fsys, name)
		if got == man.Files[name] {
			continue
		}
		_ = logger.Event("webapi", journal.EventWebStaticDrift,
			journal.Field{Key: "file", Value: name},
			journal.Field{Key: "want", Value: man.Files[name]},
			journal.Field{Key: "got", Value: got})
	}
}

// fsSHA256 — sha256 файла источника; файл не читается — пустая строка (хэш
// манифеста с ней не совпадёт, и дрейф виден в журнале).
func fsSHA256(fsys fs.FS, name string) string {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// staticServe — обработчик маршрута GET /static/: файлы static/* из источника
// (диск или embed).
// no-cache на обоих путях: у embed.FS нет валидаторов (mtime пустой), а копия на
// диске правится руками при живом движке — без заголовка браузер может годами
// держать старый app.css.
func staticServe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	currentStatic.static.ServeHTTP(w, r)
}

// serveScreen отдаёт html-экран фронта из источника статики (диск или embed)
// с no-cache; notFound — текст ошибки по контракту для этого экрана.
func serveScreen(w http.ResponseWriter, name, notFound string) {
	data, err := fs.ReadFile(currentStatic.fsys, name)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, notFound)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// serveIndex отдаёт index.html (маршрут GET /).
func serveIndex(w http.ResponseWriter, _ *http.Request) {
	serveScreen(w, "index.html", "index not found")
}

// handleHistoryScreen — GET /history.html: экран окна «Моя книга».
// app.js загружает вкладку через fetch("/history.html").
func handleHistoryScreen(w http.ResponseWriter, _ *http.Request) {
	serveScreen(w, "history.html", "history screen not found")
}
