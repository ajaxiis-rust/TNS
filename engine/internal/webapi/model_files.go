package webapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"tns65/internal/journal"
	"tns65/internal/models"
	"tns65/internal/netgate"
)

// Модели менеджера моделей (MD-08.1, MD-08.3, MD-09.5): файл роли лежит в
// world_db/models/<роль>/ под исходным именем из URL источника, рядом —
// manifest.json с указателем на текущую и прежнюю версию. Файлы не
// переименовываются: замена и откат меняют только указатель манифеста.

const (
	// modelManifestName — манифест-указатель версий роли.
	modelManifestName = "manifest.json"
	// modelLegacyManifest — манифест старой раскладки current.bin/current.json.
	modelLegacyManifest = "current.json"
	// modelLegacyFile — файл модели старой раскладки.
	modelLegacyFile = "current.bin"
)

// modelManifest — манифест world_db/models/<роль>/manifest.json: текущая
// версия в полях file/sha256, прежняя — имя файла в previous.
type modelManifest struct {
	Role        string `json:"role"`
	ModelRef    string `json:"model_ref"`
	Source      string `json:"source"`
	File        string `json:"file"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	InstalledAt string `json:"installed_at"`
	Previous    string `json:"previous"`
}

// modelVersion — учётная запись одного файла версии (<имя>.json): без неё у
// отката нет ни sha256 прежнего файла, ни его model_ref.
type modelVersion struct {
	Role        string `json:"role"`
	ModelRef    string `json:"model_ref"`
	Source      string `json:"source"`
	File        string `json:"file"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	InstalledAt string `json:"installed_at"`
}

// modelLogger — журнал движка для событий загрузчика моделей
// (model.download.start|done|fail, T-13-01).
var modelLogger *journal.Logger

// SetModelLogger подключает журнал движка к загрузчику моделей; вызывает
// main при старте (без этого события model.download.* не пишутся).
func SetModelLogger(l *journal.Logger) { modelLogger = l }

// modelDownloadFunc — загрузка файла модели Range-чанками в каталог роли.
type modelDownloadFunc func(ctx context.Context, spec models.DownloadSpec) (models.DownloadResult, error)

// fetchModelToFile — точка подмены загрузчика в тестах; в производстве —
// models.DownloadFile (T-13-01).
var fetchModelToFile modelDownloadFunc = models.DownloadFile

// Ошибки отката: разделены, чтобы хэндл отдал 409 на любую из них и 500 на
// сбой записи манифеста.
var (
	errModelNoPrevious    = errors.New("no previous model version")
	errModelPreviousGone  = errors.New("previous model file not found")
	errModelPrevIntegrity = errors.New("previous model integrity check failed")
)

// modelSourceName — исходное имя файла модели: последний сегмент пути URL
// источника, без транслитерации и смены расширения (решение владельца 2026-11-03).
func modelSourceName(source string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(source))
	if err != nil {
		return "", fmt.Errorf("webapi: имя модели: %w", err)
	}
	name := path.Base(u.Path)
	if name == "" || name == "." || name == "/" || name == ".." {
		return "", fmt.Errorf("webapi: имя модели: пустое имя в url %q", source)
	}
	return name, nil
}

// isHFSource сообщает, что источник модели — Hugging Face напрямую (MD-09.1).
func isHFSource(source string) bool {
	u, err := url.Parse(source)
	if err != nil {
		return false
	}
	return u.Scheme == "https" && (u.Host == "huggingface.co" || strings.HasSuffix(u.Host, ".huggingface.co"))
}

// modelDownloadClient — наружный клиент загрузчика: белый список netgate
// проверяет каждый хост, редирект разрешён только внутри домена Hugging Face
// (HF отдаёт файл редиректом на CDN, MD-09.1/2).
func modelDownloadClient() *http.Client {
	client := netgate.Client("")
	client.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		host := strings.ToLower(req.URL.Hostname())
		if host == "huggingface.co" || strings.HasSuffix(host, ".huggingface.co") ||
			host == "hf.co" || strings.HasSuffix(host, ".hf.co") {
			return nil
		}
		return netgate.ErrDeniedHost
	}
	return client
}

// modelManifestPath — путь манифеста-указателя роли.
func modelManifestPath(dir string) string { return filepath.Join(dir, modelManifestName) }

// modelVersionPath — путь учётной записи файла версии.
func modelVersionPath(dir, file string) string { return filepath.Join(dir, file+".json") }

// readJSONFile читает JSON-файл в v; отсутствие или порча файла — false.
func readJSONFile(path string, v any) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, v) == nil
}

// writeJSONFile пишет JSON атомарно, чтобы обрыв не оставил половину манифеста.
func writeJSONFile(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, raw)
}

// readModelManifest читает указатель версий роли; манифест старой раскладки
// (без file) указателем не считается.
func readModelManifest(dir string) (modelManifest, bool) {
	var m modelManifest
	if !readJSONFile(modelManifestPath(dir), &m) || m.File == "" {
		return modelManifest{}, false
	}
	return m, true
}

// writeModelManifest пишет указатель версий роли.
func writeModelManifest(dir string, m modelManifest) error {
	return writeJSONFile(modelManifestPath(dir), m)
}

// readModelVersion читает учётную запись файла версии.
func readModelVersion(dir, file string) (modelVersion, bool) {
	var v modelVersion
	if !readJSONFile(modelVersionPath(dir, file), &v) || v.File == "" {
		return modelVersion{}, false
	}
	return v, true
}

// writeModelVersion пишет учётную запись файла версии.
func writeModelVersion(dir string, v modelVersion) error {
	return writeJSONFile(modelVersionPath(dir, v.File), v)
}

// readLegacyManifest читает манифест старой раскладки current.bin/current.json:
// модель показывается установленной, автопереименования не делаем.
func readLegacyManifest(dir string) (modelManifest, bool) {
	var m modelManifest
	if !readJSONFile(filepath.Join(dir, modelLegacyManifest), &m) {
		return modelManifest{}, false
	}
	return m, true
}

// legacyModelFileName — имя файла для старой раскладки: исходное имя утеряно,
// берётся model_ref из манифеста плюс .bin (T-13-02, шаг 4).
func legacyModelFileName(m modelManifest) string { return m.ModelRef + ".bin" }

// installModelFile фиксирует скачанный файл под исходным именем: пишет учётную
// запись версии и манифест-указатель. Прежний файл остаётся под своим именем
// копией отката; файлы роли, на которые не указывают ни file, ни previous,
// удаляются (T-13-02, шаги 1, 3, 5).
func installModelFile(role, modelRef, source, file, sha string, size int64) error {
	dir := modelDir(role)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	installedAt := nowSettingsUTC()
	ver := modelVersion{
		Role: role, ModelRef: modelRef, Source: source, File: file,
		SHA256: sha, Size: size, InstalledAt: installedAt,
	}
	if err := writeModelVersion(dir, ver); err != nil {
		return err
	}
	previous := ""
	if old, ok := readModelManifest(dir); ok {
		previous = old.File
		// Тот же файл переустановлен: прежней остаётся более ранняя версия.
		if previous == file {
			previous = old.Previous
		}
	}
	m := modelManifest{
		Role: role, ModelRef: modelRef, Source: source, File: file,
		SHA256: sha, Size: size, InstalledAt: installedAt, Previous: previous,
	}
	if err := writeModelManifest(dir, m); err != nil {
		return err
	}
	pruneModelDir(dir, file, previous)
	return nil
}

// pruneModelDir удаляет файлы роли вне file и previous, а также недокачанные
// каталоги <имя>.part (T-13-02, шаг 5).
func pruneModelDir(dir, file, previous string) {
	keep := map[string]bool{
		modelManifestName: true,
		file:              true,
		file + ".json":    true,
	}
	if previous != "" {
		keep[previous] = true
		keep[previous+".json"] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if keep[e.Name()] {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
}

// rollbackModelFile переключает манифест на прежнюю версию с проверкой
// целостности её файла: файлы не переименовываются, previous становится
// текущим файлом — откат обратим (MD-08.3).
func rollbackModelFile(role string) (modelManifest, error) {
	dir := modelDir(role)
	m, ok := readModelManifest(dir)
	if !ok || m.Previous == "" {
		return modelManifest{}, errModelNoPrevious
	}
	prev, ok := readModelVersion(dir, m.Previous)
	if !ok {
		return modelManifest{}, errModelPreviousGone
	}
	if !modelIntegrityOK(dir, prev.File, prev.SHA256) {
		return modelManifest{}, errModelPrevIntegrity
	}
	next := modelManifest{
		Role: prev.Role, ModelRef: prev.ModelRef, Source: prev.Source, File: prev.File,
		SHA256: prev.SHA256, Size: prev.Size, InstalledAt: prev.InstalledAt,
		Previous: m.File,
	}
	if next.Role == "" {
		next.Role = role
	}
	if err := writeModelManifest(dir, next); err != nil {
		return modelManifest{}, err
	}
	return next, nil
}

// readModelStatuses читает карточки установленных моделей из world_db/models/.
// Целостность здесь дешёвая — манифест и размер файла: полный sha256 в
// горячем пути не считается (T-13-04).
func readModelStatuses() []modelStatus { return readModelStatusesVerify(false) }

// readModelStatusesVerify — тот же статус-лист; verify=true дочитывает файл
// роли и сверяет полный sha256 с манифестом (кнопка «Проверить» и проверка
// сразу после скачивания, T-13-04).
func readModelStatusesVerify(verify bool) []modelStatus {
	entries, err := os.ReadDir(modelsRoot)
	if err != nil {
		return nil
	}
	out := []modelStatus{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		st, ok := modelStatusIn(filepath.Join(modelsRoot, e.Name()), verify)
		if !ok {
			continue
		}
		if st.Role == "" {
			st.Role = e.Name()
		}
		if j, ok := modelJob(st.Role); ok {
			st.Status = j.Status
			st.Progress = j.Progress
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Role < out[j].Role })
	return out
}

// modelStatusIn собирает карточку роли по манифесту-указателю, а при его
// отсутствии — по старой раскладке current.bin/current.json. verify включает
// полную сверку sha256, без него проверяются только манифест и размер.
func modelStatusIn(dir string, verify bool) (modelStatus, bool) {
	if m, ok := readModelManifest(dir); ok {
		return modelStatus{
			Role: m.Role, ModelRef: m.ModelRef, Source: m.Source, File: m.File,
			Status: "installed", Progress: 1,
			IntegrityOK: modelStatusIntegrity(dir, m.File, m.SHA256, m.Size, verify),
			HasPrevious: m.Previous != "",
		}, true
	}
	m, ok := readLegacyManifest(dir)
	if !ok {
		return modelStatus{}, false
	}
	return modelStatus{
		Role: m.Role, ModelRef: m.ModelRef, Source: m.Source, File: legacyModelFileName(m),
		Status: "installed", Progress: 1,
		IntegrityOK: modelStatusIntegrity(dir, modelLegacyFile, m.SHA256, m.Size, verify),
	}, true
}

// modelStatusIntegrity — проверка целостности для карточки роли: с verify
// файл читается и сверяется полный sha256, без него хватает размера (T-13-04).
func modelStatusIntegrity(dir, file, want string, size int64, verify bool) bool {
	if verify {
		return modelIntegrityOK(dir, file, want)
	}
	return modelIntegrityCheap(dir, file, size)
}

// modelIntegrityCheap сверяет манифест с файлом без чтения содержимого: файл
// на месте, это обычный файл, размер совпадает. Размер неизвестен (старая
// раскладка или чужой манифест) — проверяется только наличие файла.
func modelIntegrityCheap(dir, file string, size int64) bool {
	info, err := os.Stat(filepath.Join(dir, file))
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return size <= 0 || info.Size() == size
}

// modelIntegrityOK сверяет sha256 файла модели с манифестом (MD-08.3).
func modelIntegrityOK(dir, file, want string) bool {
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return false
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) == want
}
