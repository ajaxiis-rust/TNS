package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/journal"
)

// TestModelDelete — «Проверка» T-13-03: удаление модели снимает файлы, манифест
// и задание реестра (200 и файл исчез), повторное удаление — 404, удаление во
// время активной загрузки роли — 409; карточка статуса несёт имя файла и
// размер, ребёнок роли перед удалением гасится.
func TestModelDelete(t *testing.T) {
	oldRoot := modelsRoot
	modelsRoot = filepath.Join(t.TempDir(), "models")
	oldJobs := modelJobs
	modelJobs = map[string]modelJobState{}
	t.Cleanup(func() {
		modelsRoot = oldRoot
		modelJobsMu.Lock()
		modelJobs = oldJobs
		modelJobsMu.Unlock()
	})

	journalPath := filepath.Join(t.TempDir(), "engine.log")
	logger, err := journal.New(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	oldLogger := modelLogger
	modelLogger = logger
	t.Cleanup(func() { modelLogger = oldLogger })

	stubModelDownload(t, func() string { return "model-v1" })

	const (
		source = "https://huggingface.co/org/m/resolve/main/bge-m3-Q4_K_M.gguf"
		file   = "bge-m3-Q4_K_M.gguf"
		role   = "embed"
	)
	dir := modelDir(role)

	var stopped []string
	oldStop := modelChildStop
	modelChildStop = func(r string) error {
		stopped = append(stopped, r)
		return nil
	}
	t.Cleanup(func() { modelChildStop = oldStop })

	install := func() {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"role": role, "source": source, "model_ref": "org/m"})
		rr := mcpRequest(t, handleSettingsModelInstall, http.MethodPost, settingsModelsInstallPath, body, "application/json")
		if rr.Code != http.StatusOK {
			t.Fatalf("install: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
		}
	}
	del := func() *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"role": role})
		return mcpRequest(t, handleSettingsModelDelete, http.MethodDelete, settingsModelsDeletePath, body, "application/json")
	}
	statusCard := func() modelStatus {
		t.Helper()
		rr := mcpRequest(t, handleSettingsModelStatus, http.MethodGet,
			settingsModelsStatusPath+"?role="+role, nil, "")
		if rr.Code != http.StatusOK {
			t.Fatalf("status: status = %d, want 200", rr.Code)
		}
		var body struct {
			Models []modelStatus `json:"models"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Models) != 1 {
			t.Fatalf("карточек = %d, want 1; body=%s", len(body.Models), rr.Body.String())
		}
		return body.Models[0]
	}

	install()
	if card := statusCard(); card.File != file || card.Size != int64(len("model-v1")) {
		t.Fatalf("карточка = %+v, want file=%s size=%d", card, file, len("model-v1"))
	}

	rr := del()
	if rr.Code != http.StatusOK {
		t.Fatalf("delete: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var ok struct {
		Status  string   `json:"status"`
		Removed []string `json:"removed"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &ok); err != nil {
		t.Fatal(err)
	}
	if ok.Status != "ok" {
		t.Fatalf("delete: status = %q, want ok", ok.Status)
	}
	for _, name := range []string{file, file + ".json", modelManifestName} {
		if !containsName(ok.Removed, name) {
			t.Fatalf("removed = %v, want %s", ok.Removed, name)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("каталог модели после удаления на месте: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, file)); !os.IsNotExist(err) {
		t.Fatalf("файл модели после удаления на месте: %v", err)
	}
	if len(stopped) != 1 || stopped[0] != role {
		t.Fatalf("останов ребёнка = %v, want [%s]", stopped, role)
	}
	if _, ok := modelJob(role); ok {
		t.Fatal("задание удалённой роли осталось в реестре")
	}
	logged, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), `"model.deleted"`) {
		t.Fatalf("журнал без события model.deleted: %s", logged)
	}

	// Повторное удаление — модель уже не установлена.
	if rr := del(); rr.Code != http.StatusNotFound {
		t.Fatalf("повторное удаление: status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}

	// Активная загрузка роли — отказ 409, файлы не трогаются.
	install()
	setModelJob(role, "bge-m3-Q4_K_M", "downloading", 0.2, "")
	if rr := del(); rr.Code != http.StatusConflict {
		t.Fatalf("удаление при загрузке: status = %d, want 409; body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
		t.Fatalf("удаление при активной загрузке снесло файл модели: %v", err)
	}
	setModelJob(role, "bge-m3-Q4_K_M", "installed", 1, "")
}

// containsName сообщает, что в списке снятых файлов есть name.
func containsName(list []string, name string) bool {
	for _, item := range list {
		if item == name {
			return true
		}
	}
	return false
}

// TestModelChildName — имя ребёнка для остановки при удалении (T-13-03):
// эмбеддер — всегда «embed», роль генерации — «llama» только пока ребёнок
// генерации обслуживает её, прочая роль — пусто (ребёнка нет, no-op).
func TestModelChildName(t *testing.T) {
	old := genServedRole.Load()
	t.Cleanup(func() { genServedRole.Store(old) })

	if got := ModelChildName(embedModelRole); got != "embed" {
		t.Fatalf("ModelChildName(embed) = %q, want embed", got)
	}
	if got := ModelChildName("qwen"); got != "" {
		t.Fatalf("ModelChildName(не обслуживается) = %q, want \"\"", got)
	}
	served := "qwen"
	genServedRole.Store(&served)
	if got := ModelChildName("qwen"); got != genModelChild {
		t.Fatalf("ModelChildName(обслуживается) = %q, want %q", got, genModelChild)
	}
}
