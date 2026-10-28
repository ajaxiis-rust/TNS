package webapi

import (
	"io/fs"
	"net/http"

	web "tns65/web"
)

// staticFS — подкаталог static встроенного веба. Статика отдаётся из embed.FS,
// не с диска (T-08-02, шаг 2).
var staticFS = func() fs.FS {
	sub, err := fs.Sub(web.FS, "static")
	if err != nil {
		panic(err)
	}
	return sub
}()

var staticHandler = http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))

// staticServe — обработчик маршрута GET /static/: файлы static/* из embed.
func staticServe(w http.ResponseWriter, r *http.Request) {
	staticHandler.ServeHTTP(w, r)
}

// serveIndex отдаёт index.html из embed (маршрут GET /).
func serveIndex(w http.ResponseWriter, _ *http.Request) {
	data, err := fs.ReadFile(web.FS, "index.html")
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "index not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
