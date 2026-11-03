package webapi

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// llamaAllowedHosts — хосты поставки, без которых установка отвечает 409.
var llamaAllowedHosts = []string{
	"huggingface.co",
	"*.huggingface.co",
	"github.com",
	"objects.githubusercontent.com",
}

// llamaInstallStub — заглушка скрипта поставки: кладёт фейковый llama-server в
// каталог --bin-dir и печатает стадии так же, как настоящий скрипт.
const llamaInstallStub = `#!/bin/sh
set -eu
dir=bin
while [ "$#" -gt 0 ]; do
    case "$1" in
        --bin-dir) dir="$2"; shift ;;
    esac
    shift
done
mkdir -p "$dir"
cat > "$dir/llama-server" <<'INNER'
#!/bin/sh
echo "version: 7777 (stub)"
INNER
chmod 0755 "$dir/llama-server"
echo "скачиваю фейковый архив"
echo "проверяю sha256"
echo "готово: $dir/llama-server"
`

// llamaEnv — временная среда обнаружения и установки.
type llamaEnv struct {
	cfg        string
	binDir     string
	candidates string
	scanRoot   string
	script     string
}

// seedLlamaEnv подменяет config.yaml, bin/, каталоги кандидатов, корень поиска
// и скрипт поставки на временные; PATH-поиск выключен.
func seedLlamaEnv(t *testing.T, hosts []string, script string) *llamaEnv {
	t.Helper()
	dir := t.TempDir()
	env := &llamaEnv{
		cfg:        filepath.Join(dir, "config.yaml"),
		binDir:     filepath.Join(dir, "bin"),
		candidates: filepath.Join(dir, "candidates"),
		scanRoot:   filepath.Join(dir, "home"),
		script:     filepath.Join(dir, "install-llama.sh"),
	}
	for _, d := range []string{env.binDir, env.candidates, env.scanRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	body := "netgate:\n  allowed_hosts:\n"
	for _, h := range hosts {
		body += "    - " + h + "\n"
	}
	body += "llm:\n  gen_addr: \":5001\"\n  llama_server_path: \"\"\n"
	if err := os.WriteFile(env.cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if script != "" {
		if err := os.WriteFile(env.script, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	oldCfg, oldBin := llamaConfigPath, llamaBinDir
	oldCand, oldScript, oldScan := llamaCandidateDirs, llamaInstallScript, llamaScanRoot
	oldLook, oldNow := llamaLookPath, llamaInstallNow
	llamaConfigPath = env.cfg
	llamaBinDir = env.binDir
	llamaCandidateDirs = []string{env.candidates}
	llamaInstallScript = env.script
	llamaScanRoot = env.scanRoot
	llamaLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	llamaInstallNow = llamaInstallStatus{}
	t.Cleanup(func() {
		llamaConfigPath = oldCfg
		llamaBinDir = oldBin
		llamaCandidateDirs = oldCand
		llamaInstallScript = oldScript
		llamaScanRoot = oldScan
		llamaLookPath = oldLook
		llamaInstallNow = oldNow
	})
	return env
}

// writeFakeLlama кладёт исполняемую заглушку llama-server с ответом --version.
func writeFakeLlama(t *testing.T, path, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\necho \"" + version + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// postLlamaPath выполняет POST /api/settings/llama/detect с ручным путём.
func postLlamaPath(t *testing.T, path string) (int, llamaDetection) {
	t.Helper()
	body := []byte(`{"path":` + strconvQuote(path) + `}`)
	rr := mcpRequest(t, handleLlamaDetectPost, http.MethodPost, llamaDetectRoute, body, "application/json")
	var det llamaDetection
	if rr.Body.Len() > 0 {
		_ = json.Unmarshal(rr.Body.Bytes(), &det)
	}
	return rr.Code, det
}

// strconvQuote — JSON-строка для тела запроса.
func strconvQuote(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

// TestLlamaInstallDetectsCandidateBinary — «Проверка» T-13-06: обнаружение
// находит фейковый бинарник в типовых каталогах сборки и отдаёт путь с версией.
func TestLlamaInstallDetectsCandidateBinary(t *testing.T) {
	env := seedLlamaEnv(t, llamaAllowedHosts, "")
	fake := filepath.Join(env.candidates, llamaServerName())
	writeFakeLlama(t, fake, "version: 9999 (candidate)")

	det := detectLlamaServer()
	if !det.Found || det.Path != fake || det.Source != "candidates" {
		t.Fatalf("detect = %+v, want found %s from candidates", det, fake)
	}
	if !strings.Contains(det.Version, "9999") {
		t.Fatalf("version = %q, want --version answer", det.Version)
	}
	if len(det.Searched) == 0 {
		t.Fatalf("searched is empty, want examined paths")
	}

	rr := mcpRequest(t, handleLlamaDetectGet, http.MethodGet, llamaDetectRoute, nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", llamaDetectRoute, rr.Code)
	}
	var out llamaDetection
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal detect: %v", err)
	}
	if !out.Found || out.Path != fake || !strings.Contains(out.Version, "9999") {
		t.Fatalf("GET detect = %+v, want %s", out, fake)
	}
}

// TestLlamaInstallDetectsPathBinary — бинарник из PATH находится, источник path.
func TestLlamaInstallDetectsPathBinary(t *testing.T) {
	seedLlamaEnv(t, llamaAllowedHosts, "")
	fake := filepath.Join(t.TempDir(), llamaServerName())
	writeFakeLlama(t, fake, "version: 8888 (path)")
	llamaLookPath = func(name string) (string, error) {
		if name == llamaServerName() {
			return fake, nil
		}
		return "", exec.ErrNotFound
	}

	det := detectLlamaServer()
	if !det.Found || det.Path != fake || det.Source != "path" {
		t.Fatalf("detect = %+v, want %s from path", det, fake)
	}
	if !strings.Contains(det.Version, "8888") {
		t.Fatalf("version = %q, want 8888", det.Version)
	}
}

// TestLlamaInstallDetectEmpty — пустое обнаружение: found=false, а просмотренные
// пути перечислены.
func TestLlamaInstallDetectEmpty(t *testing.T) {
	seedLlamaEnv(t, llamaAllowedHosts, "")
	det := detectLlamaServer()
	if det.Found {
		t.Fatalf("detect found = true (%s), want empty", det.Path)
	}
	if len(det.Searched) < 2 {
		t.Fatalf("searched = %v, want bin/ and candidate dirs", det.Searched)
	}
}

// TestLlamaInstallInstallWithoutNetwork — «Проверка» T-13-06: найдено —
// установка отвечает без сети, source=detect, путь записан в конфиг, шаг 0
// несёт путь и версию.
func TestLlamaInstallInstallWithoutNetwork(t *testing.T) {
	env := seedLlamaEnv(t, llamaAllowedHosts, "")
	fake := filepath.Join(env.binDir, llamaServerName())
	writeFakeLlama(t, fake, "version: 4242 (instance)")

	rr := mcpRequest(t, handleLlamaInstall, http.MethodPost, llamaInstallRoute, nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST %s: status = %d, want 200; body=%s", llamaInstallRoute, rr.Code, rr.Body.String())
	}
	var out struct {
		Installed bool   `json:"installed"`
		Path      string `json:"path"`
		Version   string `json:"version"`
		Source    string `json:"source"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal install: %v", err)
	}
	if !out.Installed || out.Path != fake || out.Source != "detect" {
		t.Fatalf("install = %+v, want installed %s from detect", out, fake)
	}
	if !strings.Contains(out.Version, "4242") {
		t.Fatalf("install version = %q, want 4242", out.Version)
	}
	if got := readLlamaServerPath(); got != fake {
		t.Fatalf("llm.llama_server_path = %q, want %q", got, fake)
	}

	st := probeFirstRunStation()
	if !st.LlamaServer.Found || st.LlamaServer.Path != fake || st.LlamaServer.Version == "" {
		t.Fatalf("station llama_server = %+v, want path and version", st.LlamaServer)
	}
}

// TestLlamaInstallManualPath — «Проверка» T-13-06: ручной путь валидируется
// --version и сохраняется, несуществующий — 400, пустой — очищает поле.
func TestLlamaInstallManualPath(t *testing.T) {
	env := seedLlamaEnv(t, llamaAllowedHosts, "")
	fake := filepath.Join(env.candidates, llamaServerName())
	writeFakeLlama(t, fake, "version: 5555 (manual)")

	code, det := postLlamaPath(t, fake)
	if code != http.StatusOK || !det.Found || det.Path != fake {
		t.Fatalf("POST detect = %d %+v, want 200 found %s", code, det, fake)
	}
	if got := readLlamaServerPath(); got != fake {
		t.Fatalf("llm.llama_server_path = %q, want %q", got, fake)
	}

	code, _ = postLlamaPath(t, filepath.Join(env.candidates, "nope"))
	if code != http.StatusBadRequest {
		t.Fatalf("POST detect missing path: status = %d, want 400", code)
	}

	code, _ = postLlamaPath(t, "")
	if code != http.StatusOK {
		t.Fatalf("POST detect empty path: status = %d, want 200", code)
	}
	if got := readLlamaServerPath(); got != "" {
		t.Fatalf("llm.llama_server_path = %q after clear, want empty", got)
	}
}

// TestLlamaInstallRunsScriptWhenEmpty — «Проверка» T-13-06: при пустом
// обнаружении установка запускает скрипт поставки и отвечает source=download.
func TestLlamaInstallRunsScriptWhenEmpty(t *testing.T) {
	env := seedLlamaEnv(t, llamaAllowedHosts, llamaInstallStub)

	rr := mcpRequest(t, handleLlamaInstall, http.MethodPost, llamaInstallRoute, nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST %s: status = %d, want 200; body=%s", llamaInstallRoute, rr.Code, rr.Body.String())
	}
	var out struct {
		Installed bool   `json:"installed"`
		Path      string `json:"path"`
		Version   string `json:"version"`
		Source    string `json:"source"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal install: %v", err)
	}
	want := filepath.Join(env.binDir, llamaServerName())
	if !out.Installed || out.Source != "download" || out.Path != want {
		t.Fatalf("install = %+v, want installed %s from download", out, want)
	}
	if !strings.Contains(out.Version, "7777") {
		t.Fatalf("install version = %q, want stub version", out.Version)
	}
	if got := readLlamaServerPath(); got != want {
		t.Fatalf("llm.llama_server_path = %q, want %q", got, want)
	}
	if st := currentLlamaInstall(); st.Running || st.Stage != llamaStageDone || st.Error != "" {
		t.Fatalf("install status = %+v, want finished done", st)
	}
}

// TestLlamaInstallConflictWithoutNetgate — без github.com и
// objects.githubusercontent.com в белом списке установка отвечает 409.
func TestLlamaInstallConflictWithoutNetgate(t *testing.T) {
	env := seedLlamaEnv(t, []string{"huggingface.co"}, llamaInstallStub)

	rr := mcpRequest(t, handleLlamaInstall, http.MethodPost, llamaInstallRoute, nil, "")
	if rr.Code != http.StatusConflict {
		t.Fatalf("POST %s: status = %d, want 409; body=%s", llamaInstallRoute, rr.Code, rr.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error body: %v", err)
	}
	if body.Error.Code != codeConflict {
		t.Fatalf("error code = %q, want %q", body.Error.Code, codeConflict)
	}
	if _, err := os.Stat(filepath.Join(env.binDir, llamaServerName())); !os.IsNotExist(err) {
		t.Fatalf("binary must not appear on 409; stat err=%v", err)
	}
}

// TestLlamaInstallScanFindsBinary — поиск по диску отдаёт найденные пути,
// пропускает скрытые каталоги, слишком глубокие и неисполнимые файлы.
func TestLlamaInstallScanFindsBinary(t *testing.T) {
	env := seedLlamaEnv(t, llamaAllowedHosts, "")
	near := filepath.Join(env.scanRoot, "build", "bin")
	deep := filepath.Join(env.scanRoot, "a", "b", "bin")
	writeFakeLlama(t, filepath.Join(near, llamaServerName()), "version: 1111 (near)")
	writeFakeLlama(t, filepath.Join(deep, llamaServerName()), "version: 2222 (deep)")
	hidden := filepath.Join(env.scanRoot, ".cache", llamaServerName())
	writeFakeLlama(t, hidden, "version: 3333 (hidden)")
	tooDeep := filepath.Join(env.scanRoot, "a", "b", "c", "d", "e", "f")
	writeFakeLlama(t, filepath.Join(tooDeep, llamaServerName()), "version: 4444 (deep)")
	if err := os.WriteFile(filepath.Join(near, "llama-server.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	rr := mcpRequest(t, handleLlamaScan, http.MethodPost, llamaScanRoute, nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST %s: status = %d, want 200", llamaScanRoute, rr.Code)
	}
	var out struct {
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal scan: %v", err)
	}
	if len(out.Paths) != 2 {
		t.Fatalf("scan paths = %v, want near and deep binaries only", out.Paths)
	}
	if out.Paths[0] != filepath.Join(deep, llamaServerName()) || out.Paths[1] != filepath.Join(near, llamaServerName()) {
		t.Fatalf("scan paths = %v, want sorted deep and near", out.Paths)
	}
}

// TestLlamaInstallKeepsConfigOnDetect — config-first: значение из
// llm.llama_server_path проверяется первым и даёт source=config.
func TestLlamaInstallKeepsConfigOnDetect(t *testing.T) {
	env := seedLlamaEnv(t, llamaAllowedHosts, "")
	fake := filepath.Join(t.TempDir(), llamaServerName())
	writeFakeLlama(t, fake, "version: 6060 (config)")
	if err := writeLlamaServerPath(fake); err != nil {
		t.Fatalf("write llama path: %v", err)
	}
	if got := readLlamaServerPath(); got != fake {
		t.Fatalf("llm.llama_server_path = %q, want %q", got, fake)
	}

	det := detectLlamaServer()
	if !det.Found || det.Path != fake || det.Source != "config" {
		t.Fatalf("detect = %+v, want %s from config", det, fake)
	}
	if data, err := os.ReadFile(env.cfg); err != nil || !strings.Contains(string(data), "llama_server_path") {
		t.Fatalf("config.yaml lost llama_server_path: err=%v", err)
	}
}
