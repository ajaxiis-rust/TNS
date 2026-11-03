package webapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"tns65/internal/journal"
)

// settingsModelsDeletePath — удаление установленной модели роли (T-13-03).
const settingsModelsDeletePath = "/api/settings/models/delete"

// modelChildStop — остановка ребёнка роли перед снятием её файлов; роль без
// ребёнка — no-op. Устанавливается ядром в main.go, как и modelChildSwitch.
var modelChildStop func(role string) error

// SetModelChildStop подключает остановку ребёнка роли: без хука удаление
// снимает файлы, но запущенного ребёнка не трогает.
func SetModelChildStop(f func(role string) error) { modelChildStop = f }

// handleSettingsModelDelete — DELETE /api/settings/models/delete: снимает
// модель роли — файлы, манифест и задание реестра загрузок (MD-08.1, MD-08.4).
// Активная загрузка роли — 409, неустановленная роль — 404: иначе повторное
// удаление молча проходило бы.
func handleSettingsModelDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad delete body")
		return
	}
	role := strings.TrimSpace(req.Role)
	if role == "" || modelDirName(role) != role {
		writeError(w, http.StatusBadRequest, codeBadRequest, "role required")
		return
	}
	if modelDownloading(role) {
		writeError(w, http.StatusConflict, codeConflict, "model download already running")
		return
	}
	dir := modelDir(role)
	if !modelInstalledDir(dir) {
		writeError(w, http.StatusNotFound, codeNotFound, "model not installed")
		return
	}
	// Файлы снимаются только с погашенного ребёнка: иначе llama-server держит
	// открытым удаляемый файл.
	stopModelChildRole(role)
	removed, err := removeModelFiles(dir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "model delete failed")
		return
	}
	clearModelJob(role)
	logModelDeleted(role, removed)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "removed": removed})
}

// modelDownloading сообщает, что роль занята активной загрузкой.
func modelDownloading(role string) bool {
	j, ok := modelJob(role)
	return ok && j.Status == "downloading"
}

// modelInstalledDir сообщает, что в каталоге роли лежит манифест-указатель
// или манифест старой раскладки current.json.
func modelInstalledDir(dir string) bool {
	if _, ok := readModelManifest(dir); ok {
		return true
	}
	_, ok := readLegacyManifest(dir)
	return ok
}

// clearModelJob снимает задание роли из реестра загрузок.
func clearModelJob(role string) {
	modelJobsMu.Lock()
	delete(modelJobs, role)
	modelJobsMu.Unlock()
}

// removeModelFiles снимает каталог роли целиком и возвращает имена снятых
// файлов: по ним карточка показывает, что удалено.
func removeModelFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	removed := make([]string, 0, len(entries))
	for _, e := range entries {
		removed = append(removed, e.Name())
	}
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	return removed, nil
}

// stopModelChildRole гасит ребёнка роли перед удалением файлов; роль без
// запущенного ребёнка или без подключённого хука — no-op.
func stopModelChildRole(role string) {
	if modelChildStop == nil {
		return
	}
	if err := modelChildStop(role); err != nil {
		logModelChildEvent(role, "model.child.stop_fail", err.Error())
		return
	}
	logModelChildEvent(role, "model.child.stopped", "")
}

// logModelChildEvent пишет событие жизненного цикла ребёнка роли в журнал.
func logModelChildEvent(role, event, errMsg string) {
	if modelLogger == nil {
		return
	}
	fields := []journal.Field{{Key: "role", Value: role}}
	if errMsg != "" {
		fields = append(fields, journal.Field{Key: "error", Value: errMsg})
	}
	_ = modelLogger.Event("model", event, fields...)
}

// logModelDeleted пишет событие удаления модели в штатный журнал движка.
func logModelDeleted(role string, removed []string) {
	if modelLogger == nil {
		return
	}
	fields := []journal.Field{
		{Key: "role", Value: role},
		{Key: "files", Value: strconv.Itoa(len(removed))},
	}
	_ = modelLogger.Event("model", "model.deleted", fields...)
}

// fillModelCardSizes дописывает размер файла в карточки статус-листа: менеджер
// видит имя файла и размер без полной сверки sha256 (T-13-03, шаг 2).
func fillModelCardSizes(models []modelStatus) []modelStatus {
	for i := range models {
		dir := modelDir(models[i].Role)
		if m, ok := readModelManifest(dir); ok {
			models[i].Size = m.Size
			continue
		}
		if m, ok := readLegacyManifest(dir); ok {
			models[i].Size = m.Size
		}
	}
	return models
}
