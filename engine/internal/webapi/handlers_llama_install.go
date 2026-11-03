package webapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"tns65/internal/journal"
)

// Обнаружение и установка llama-server (T-13-06, MD-03, UI-08.6-бис). Шаг 0
// мастера сначала ищет уже собранный бинарник — конфиг, bin/ экземпляра, PATH,
// типовые каталоги сборки — и только при пустом результате предлагает
// скачивание скриптом поставки. Скачивание требует хостов GitHub в белом списке
// netgate: без них маршрут установки отвечает 409 и визард показывает команду
// ручного запуска скрипта.

const (
	llamaDetectRoute  = "/api/settings/llama/detect"
	llamaInstallRoute = "/api/settings/llama/install"
	llamaScanRoute    = "/api/settings/llama/scan"

	// llamaScanMaxDepth — глубина ограниченного поиска по домашнему каталогу.
	llamaScanMaxDepth = 5

	// Стадии установки скриптом: «скачивание», «проверка», «готово».
	llamaStageDownload = "download"
	llamaStageVerify   = "verify"
	llamaStageDone     = "done"
	llamaStageFailed   = "failed"
)

// Пути, тайм-ауты и среда обнаружения подменяются в тестах.
var (
	llamaConfigPath     = settingsConfigPath
	llamaInstallScript  = filepath.Join("scripts", "install-llama.sh")
	llamaBinDir         = "bin"
	llamaVersionTimeout = 5 * time.Second
	llamaInstallTimeout = 15 * time.Minute
	llamaScanTimeout    = 15 * time.Second
	llamaScanRoot       = defaultLlamaScanRoot()
	llamaCandidateDirs  = defaultLlamaCandidateDirs()
	llamaLookPath       = exec.LookPath
)

// llamaDownloadHosts — хосты поставки llama.cpp, нужные в белом списке netgate.
var llamaDownloadHosts = []string{"github.com", "objects.githubusercontent.com"}

// llamaServerStatus — решённый путь llama-server: шаг 0 мастера несёт путь и
// версию вместо прежнего буля (T-13-06).
type llamaServerStatus struct {
	Found   bool   `json:"found"`
	Path    string `json:"path"`
	Version string `json:"version"`
	Source  string `json:"source"`
}

// llamaDetection — ответ обнаружения: найденный бинарник и просмотренные пути.
type llamaDetection struct {
	llamaServerStatus
	Searched []string           `json:"searched"`
	Install  llamaInstallStatus `json:"install"`
}

// llamaInstallStatus — стадия установки скриптом; визард опрашивает её через
// GET /api/settings/llama/detect, пока POST установки ещё не ответил.
type llamaInstallStatus struct {
	Running bool   `json:"running"`
	Stage   string `json:"stage"`
	Error   string `json:"error"`
}

var (
	llamaInstallMu  sync.Mutex
	llamaInstallNow = llamaInstallStatus{}
)

// currentLlamaInstall отдаёт текущую стадию установки.
func currentLlamaInstall() llamaInstallStatus {
	llamaInstallMu.Lock()
	defer llamaInstallMu.Unlock()
	return llamaInstallNow
}

// beginLlamaInstall занимает установку: вторая параллельная получает отказ.
func beginLlamaInstall() bool {
	llamaInstallMu.Lock()
	defer llamaInstallMu.Unlock()
	if llamaInstallNow.Running {
		return false
	}
	llamaInstallNow = llamaInstallStatus{Running: true, Stage: llamaStageDownload}
	return true
}

// setLlamaInstallStage переключает стадию установки; пустая стадия — не переход.
func setLlamaInstallStage(stage string) {
	if stage == "" {
		return
	}
	llamaInstallMu.Lock()
	defer llamaInstallMu.Unlock()
	llamaInstallNow.Stage = stage
	llamaInstallNow.Running = stage == llamaStageDownload || stage == llamaStageVerify
}

// finishLlamaInstall снимает флаг установки и запоминает причину отказа.
func finishLlamaInstall(err error) {
	llamaInstallMu.Lock()
	defer llamaInstallMu.Unlock()
	llamaInstallNow.Running = false
	if err != nil {
		llamaInstallNow.Stage = llamaStageFailed
		llamaInstallNow.Error = err.Error()
	}
}

// llamaServerName — имя бинарника поставки на текущей платформе.
func llamaServerName() string {
	if runtime.GOOS == "windows" {
		return "llama-server.exe"
	}
	return "llama-server"
}

// defaultLlamaScanRoot — домашний каталог: полный обход диска не делается.
func defaultLlamaScanRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// defaultLlamaCandidateDirs — типовые каталоги сборки llama.cpp.
func defaultLlamaCandidateDirs() []string {
	home, _ := os.UserHomeDir()
	dirs := make([]string, 0, 5)
	if home != "" {
		dirs = append(dirs,
			filepath.Join(home, "llama.cpp", "build", "bin"),
			filepath.Join(home, "src", "llama.cpp", "build", "bin"),
			filepath.Join(home, ".local", "bin"))
	}
	return append(dirs, "/opt/llama.cpp/build/bin", "/usr/local/bin")
}

// readLlamaServerPath читает llm.llama_server_path из config.yaml; пусто —
// автоопределение.
func readLlamaServerPath() string {
	data, err := os.ReadFile(llamaConfigPath)
	if err != nil {
		return ""
	}
	return parseLlamaServerPath(string(data))
}

// parseLlamaServerPath разбирает ключ llama_server_path секции llm.
func parseLlamaServerPath(data string) string {
	inLLM := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inLLM = line == "llm:"
			continue
		}
		if !inLLM {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != "llama_server_path" {
			continue
		}
		return unquoteScalar(strings.TrimSpace(strings.SplitN(value, "#", 2)[0]))
	}
	return ""
}

// writeLlamaServerPath пишет llm.llama_server_path в config.yaml: заменяет
// строку, а в конфиге без поля вставляет её сразу после заголовка секции.
func writeLlamaServerPath(path string) error {
	data, err := os.ReadFile(llamaConfigPath)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	value := yamlQuoted(path)
	inLLM := false
	header := -1
	replaced := false
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inLLM = line == "llm:"
			if inLLM {
				header = i
			}
			continue
		}
		if !inLLM {
			continue
		}
		key, _, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != "llama_server_path" {
			continue
		}
		lines[i] = "  llama_server_path: " + value
		replaced = true
	}
	if !replaced {
		if header < 0 {
			return errors.New("webapi: config.yaml без секции llm")
		}
		lines = append(lines, "")
		copy(lines[header+2:], lines[header+1:])
		lines[header+1] = "  llama_server_path: " + value
	}
	return writeFileAtomic(llamaConfigPath, []byte(strings.Join(lines, "\n")+"\n"))
}

// yamlQuoted — скаляр в кавычках: пути с пробелами читаются обратно дословно.
func yamlQuoted(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// probeLlamaServer проверяет кандидата: файл существует, исполним (кроме
// Windows) и отвечает на --version (тайм-аут 5 с). Возвращает версию — первую
// непустую строку ответа.
func probeLlamaServer(path string) (bool, string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, ""
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false, ""
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return false, ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), llamaVersionTimeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if ctx.Err() != nil {
		return false, ""
	}
	version := firstNonEmptyLine(string(out))
	if version == "" {
		return false, ""
	}
	return true, version
}

// firstNonEmptyLine — первая непустая строка текста без хвостовых пробелов.
func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// detectLlamaServer ищет llama-server по контракту §2: значение из конфига →
// bin/ экземпляра → PATH → типовые каталоги сборки. Каждый кандидат проверяется
// запуском --version; первый валидный и есть решённый путь.
func detectLlamaServer() llamaDetection {
	det := llamaDetection{Searched: []string{}, Install: currentLlamaInstall()}
	probe := func(path, source string) bool {
		if path == "" {
			return false
		}
		det.Searched = append(det.Searched, path)
		if ok, version := probeLlamaServer(path); ok {
			det.Found = true
			det.Path = path
			det.Version = version
			det.Source = source
			return true
		}
		return false
	}
	if probe(readLlamaServerPath(), "config") {
		return det
	}
	if probe(filepath.Join(llamaBinDir, llamaServerName()), "instance") {
		return det
	}
	if path, err := llamaLookPath(llamaServerName()); err == nil && probe(path, "path") {
		return det
	}
	for _, dir := range llamaCandidateDirs {
		if probe(filepath.Join(dir, llamaServerName()), "candidates") {
			return det
		}
	}
	return det
}

// llamaStationStatus — результат обнаружения для шага 0 мастера.
func llamaStationStatus() llamaServerStatus {
	det := detectLlamaServer()
	return det.llamaServerStatus
}

// llamaDownloadAllowed сообщает, что в белом списке netgate есть оба хоста
// поставки; иначе скачивание идёт только руками, скриптом.
func llamaDownloadAllowed() bool {
	data, err := os.ReadFile(llamaConfigPath)
	if err != nil {
		return false
	}
	allows := netgateHosts(string(data))
	for _, host := range llamaDownloadHosts {
		if !allows(host) {
			return false
		}
	}
	return true
}

// netgateHosts разбирает netgate.allowed_hosts в предикат проверки хоста.
func netgateHosts(data string) func(string) bool {
	entries := []string{}
	inNetgate := false
	inHosts := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inNetgate = line == "netgate:"
			inHosts = false
			continue
		}
		if !inNetgate {
			continue
		}
		if line == "allowed_hosts:" {
			inHosts = true
			continue
		}
		if !inHosts {
			continue
		}
		if !strings.HasPrefix(line, "-") {
			inHosts = false
			continue
		}
		entry := unquoteScalar(strings.TrimSpace(strings.TrimPrefix(line, "-")))
		if entry != "" {
			entries = append(entries, strings.ToLower(entry))
		}
	}
	return func(host string) bool {
		host = strings.ToLower(strings.TrimSpace(host))
		for _, entry := range entries {
			if strings.HasPrefix(entry, "*.") {
				if strings.HasSuffix(host, entry[1:]) {
					return true
				}
				continue
			}
			if entry == host {
				return true
			}
		}
		return false
	}
}

// handleLlamaDetectGet — GET /api/settings/llama/detect: автообнаружение со
// списком просмотренных путей и стадией текущей установки.
func handleLlamaDetectGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, detectLlamaServer())
}

// handleLlamaDetectPost — POST /api/settings/llama/detect: проверка ручного
// пути запуском --version и запись в llm.llama_server_path; пустой путь
// очищает поле (снова автоопределение).
func handleLlamaDetectPost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad llama path body")
		return
	}
	path := expandHome(strings.TrimSpace(req.Path))
	if path == "" {
		if err := writeLlamaServerPath(""); err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "config write failed")
			return
		}
		writeJSON(w, http.StatusOK, llamaDetection{Searched: []string{}, Install: currentLlamaInstall()})
		return
	}
	ok, version := probeLlamaServer(path)
	if !ok {
		writeError(w, http.StatusBadRequest, codeBadRequest, "path is not a working llama-server (no answer to --version)")
		return
	}
	if err := writeLlamaServerPath(path); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "config write failed")
		return
	}
	writeJSON(w, http.StatusOK, llamaDetection{
		llamaServerStatus: llamaServerStatus{Found: true, Path: path, Version: version, Source: "config"},
		Searched:          []string{path},
		Install:           currentLlamaInstall(),
	})
}

// expandHome разворачивает ведущий "~/" в домашний каталог.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// handleLlamaScan — POST /api/settings/llama/scan: ограниченный поиск
// llama-server по домашнему каталогу (глубина 5, тайм-аут 15 с). Полный обход
// диска не делается; найденные пути предлагаются для ручного выбора.
func handleLlamaScan(w http.ResponseWriter, _ *http.Request) {
	paths := scanLlamaServer(llamaScanRoot, llamaScanMaxDepth, llamaScanTimeout)
	writeJSON(w, http.StatusOK, map[string]any{"paths": paths})
}

// scanLlamaServer обходит root до maxDepth и собирает исполнимые llama-server;
// каталоги без прав и скрытые каталоги пропускаются.
func scanLlamaServer(root string, maxDepth int, timeout time.Duration) []string {
	paths := []string{}
	root = strings.TrimSpace(root)
	if root == "" {
		return paths
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	name := llamaServerName()
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") || llamaPathDepth(root, path) >= maxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 || d.Name() != name {
			return nil
		}
		if ok, _ := probeLlamaServer(path); ok {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)
	return paths
}

// llamaPathDepth — глубина path относительно root (root — 0).
func llamaPathDepth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return len(strings.Split(rel, string(filepath.Separator)))
}

// handleLlamaInstall — POST /api/settings/llama/install: сначала обнаружение;
// найдено — путь записывается и ответ без сети (source=detect). Пустое
// обнаружение при разрешённых хостах netgate запускает скрипт поставки с
// проверкой sha256 (source=download), иначе 409 с командой ручного запуска.
func handleLlamaInstall(w http.ResponseWriter, r *http.Request) {
	det := detectLlamaServer()
	if det.Found {
		if err := writeLlamaServerPath(det.Path); err != nil {
			writeError(w, http.StatusInternalServerError, codeInternal, "config write failed")
			return
		}
		logLlamaInstalled(det.Path, det.Version, "detect")
		writeJSON(w, http.StatusOK, llamaInstalledResponse(det.Path, det.Version, "detect"))
		return
	}
	if !llamaDownloadAllowed() {
		writeError(w, http.StatusConflict, codeConflict,
			"netgate whitelist misses github.com/objects.githubusercontent.com; run "+llamaInstallScript+" manually")
		return
	}
	if !beginLlamaInstall() {
		writeError(w, http.StatusConflict, codeConflict, "llama.cpp install already running")
		return
	}
	if err := runLlamaInstallScript(); err != nil {
		finishLlamaInstall(err)
		writeError(w, http.StatusInternalServerError, codeInternal, "llama.cpp install failed: "+err.Error())
		return
	}
	det = detectLlamaServer()
	if !det.Found {
		err := errors.New("llama-server not found after install")
		finishLlamaInstall(err)
		writeError(w, http.StatusInternalServerError, codeInternal, err.Error())
		return
	}
	if err := writeLlamaServerPath(det.Path); err != nil {
		finishLlamaInstall(err)
		writeError(w, http.StatusInternalServerError, codeInternal, "config write failed")
		return
	}
	setLlamaInstallStage(llamaStageDone)
	finishLlamaInstall(nil)
	logLlamaInstalled(det.Path, det.Version, "download")
	writeJSON(w, http.StatusOK, llamaInstalledResponse(det.Path, det.Version, "download"))
}

// llamaInstalledResponse — ответ установки по контракту §2.
func llamaInstalledResponse(path, version, source string) map[string]any {
	return map[string]any{"installed": true, "path": path, "version": version, "source": source}
}

// runLlamaInstallScript запускает закреплённый скрипт поставки и переводит
// стадии по его строкам: «скачиваю» → download, «sha256/проверяю» → verify,
// «готово» → done. Скрипт сам сверяет sha256 скачанного ассета.
func runLlamaInstallScript() error {
	binDir := llamaBinDir
	if !filepath.IsAbs(binDir) {
		abs, err := filepath.Abs(binDir)
		if err == nil {
			binDir = abs
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), llamaInstallTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", llamaInstallScript, "--bin-dir", binDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		setLlamaInstallStage(llamaStageForLine(sc.Text()))
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return errors.New("install script timed out")
		}
		if line := firstNonEmptyLine(stderr.String()); line != "" {
			return fmt.Errorf("%v: %s", err, line)
		}
		return err
	}
	return nil
}

// llamaStageForLine — стадия установки по строке вывода скрипта.
func llamaStageForLine(line string) string {
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "скачива") || strings.Contains(l, "download"):
		return llamaStageDownload
	case strings.Contains(l, "sha256") || strings.Contains(l, "проверя"):
		return llamaStageVerify
	case strings.Contains(l, "готово") || strings.Contains(l, "done"):
		return llamaStageDone
	}
	return ""
}

// logLlamaInstalled пишет событие llama.installed в журнал движка.
func logLlamaInstalled(path, version, source string) {
	if modelLogger == nil {
		return
	}
	_ = modelLogger.Event("llama", "llama.installed",
		journal.Field{Key: "path", Value: path},
		journal.Field{Key: "version", Value: version},
		journal.Field{Key: "source", Value: source})
}
