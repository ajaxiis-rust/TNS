//go:build e2e

package webapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// e2eClock — фейковые часы ядра (T-09-02); nil — фейковые часы не подключены.
var e2eClock *testkit.Clock

// SetE2EClock подключает фейковые часы ядра; их же берёт сервер хода (chatClock).
func SetE2EClock(c *testkit.Clock) {
	e2eClock = c
	chatClock = c
}

// e2eRoutes — маршруты только e2e-сборки: контрольный сдвиг часов.
func e2eRoutes() []routeSpec {
	return []routeSpec{
		{method: http.MethodPost, path: "/api/e2e/clock", handle: handleE2EClock},
	}
}

// e2eClockRequest — тело POST /api/e2e/clock: сдвиг часов в секундах.
type e2eClockRequest struct {
	AdvanceSeconds int64 `json:"advance_seconds"`
}

// handleE2EClock сдвигает фейковые часы и прогоняет таймер отложенных событий
// движка (INT-2: исполнитель — код таймера, не модель). Реального ожидания нет.
func handleE2EClock(w http.ResponseWriter, r *http.Request) {
	if e2eClock == nil {
		writeError(w, http.StatusNotFound, codeNotFound, "e2e clock not enabled")
		return
	}
	var req e2eClockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AdvanceSeconds <= 0 {
		writeError(w, http.StatusBadRequest, codeBadRequest, "advance_seconds required")
		return
	}
	e2eClock.Advance(time.Duration(req.AdvanceSeconds) * time.Second)
	fired := fireDueWorlds()
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "deferred_fired": fired})
}

// fireDueWorlds срабатывает отложенные события всех миров по текущим часам.
func fireDueWorlds() int {
	entries, err := os.ReadDir(filepath.Join(db.WorldsRoot, "worlds"))
	if err != nil {
		return 0
	}
	total := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		h, err := db.OpenWriterAs(worldDBPath(e.Name()), db.DBWorld, db.WriterCore, chatLogger)
		if err != nil {
			continue
		}
		if n, err := workshop.NewDeferredTimer(h, e2eClock).FireDue(); err == nil {
			total += n
		}
		_ = h.Close()
	}
	return total
}
