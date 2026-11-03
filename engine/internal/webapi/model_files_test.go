package webapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/models"
)

// stubModelDownload подменяет загрузчик модели: пишет в Dest содержимое
// content() и возвращает размер с sha256, как models.DownloadFile. content —
// функция, чтобы тест менял версию между установками.
func stubModelDownload(t *testing.T, content func() string) {
	t.Helper()
	old := fetchModelToFile
	fetchModelToFile = func(_ context.Context, spec models.DownloadSpec) (models.DownloadResult, error) {
		data := []byte(content())
		if dir := filepath.Dir(spec.Dest); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return models.DownloadResult{}, err
			}
		}
		if err := os.WriteFile(spec.Dest, data, 0o644); err != nil {
			return models.DownloadResult{}, err
		}
		sum := sha256.Sum256(data)
		return models.DownloadResult{
			Path: spec.Dest, Size: int64(len(data)),
			SHA256: hex.EncodeToString(sum[:]), Workers: 1,
		}, nil
	}
	t.Cleanup(func() { fetchModelToFile = old })
}

// TestModelInstallFileNaming — «Проверка» T-13-02: установка кладёт файл под
// исходным именем из URL, манифест валиден, замена сохраняет прежний файл
// копией отката, откат возвращает предыдущую версию, вторая параллельная
// установка той же роли — 409; старая раскладка current.bin/current.json
// читается как установленная модель.
func TestModelInstallFileNaming(t *testing.T) {
	seedSettingsWorld(t)
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

	content := "model-v1"
	stubModelDownload(t, func() string { return content })

	const (
		srcV1 = "https://huggingface.co/org/m/resolve/main/bge-m3-Q4_K_M.gguf"
		srcV2 = "https://huggingface.co/org/m/resolve/main/Llama-3.2-3B-Instruct-Q4_K_M.gguf"
		file1 = "bge-m3-Q4_K_M.gguf"
		file2 = "Llama-3.2-3B-Instruct-Q4_K_M.gguf"
	)
	dir := modelDir("weaver")

	install := func(source, modelRef string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{
			"world": "alpha", "role": "weaver", "source": source, "model_ref": modelRef,
		})
		return mcpRequest(t, handleSettingsModelInstall, http.MethodPost, settingsModelsInstallPath, body, "application/json")
	}
	readManifest := func() modelManifest {
		t.Helper()
		m, ok := readModelManifest(dir)
		if !ok {
			t.Fatal("манифест модели не прочитан")
		}
		return m
	}
	fileContent := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("файл %s не найден: %v", name, err)
		}
		return string(data)
	}
	statusOf := func(role string) *modelStatus {
		t.Helper()
		for _, st := range readModelStatuses() {
			if st.Role == role {
				return &st
			}
		}
		return nil
	}

	if rr := install(srcV1, "org/m"); rr.Code != http.StatusOK {
		t.Fatalf("install: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if got := fileContent(file1); got != "model-v1" {
		t.Fatalf("файл %s = %q, want model-v1", file1, got)
	}
	for _, stale := range []string{"current.bin", "current.json", "previous.bin"} {
		if _, err := os.Stat(filepath.Join(dir, stale)); !os.IsNotExist(err) {
			t.Fatalf("файл старой раскладки %s остался в каталоге роли", stale)
		}
	}
	sum := sha256.Sum256([]byte("model-v1"))
	m := readManifest()
	if m.Role != "weaver" || m.ModelRef != "org/m" || m.Source != srcV1 {
		t.Fatalf("манифест = %+v, want weaver/org/m", m)
	}
	if m.File != file1 || m.SHA256 != hex.EncodeToString(sum[:]) || m.Size != int64(len("model-v1")) {
		t.Fatalf("манифест = %+v, want file=%s и sha256/size первой версии", m, file1)
	}
	if m.InstalledAt == "" || m.Previous != "" {
		t.Fatalf("манифест = %+v, want installed_at и пустой previous", m)
	}
	if st := statusOf("weaver"); st == nil || st.File != file1 || !st.IntegrityOK || st.HasPrevious {
		t.Fatalf("карточка weaver = %+v, want file=%s целостность без previous", st, file1)
	}

	content = "model-v2"
	if rr := install(srcV2, "org/m-v2"); rr.Code != http.StatusOK {
		t.Fatalf("replace: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if got := fileContent(file1); got != "model-v1" {
		t.Fatalf("прежний файл %s = %q, want model-v1 (копия отката)", file1, got)
	}
	if got := fileContent(file2); got != "model-v2" {
		t.Fatalf("новый файл %s = %q, want model-v2", file2, got)
	}
	m = readManifest()
	if m.File != file2 || m.ModelRef != "org/m-v2" || m.Previous != file1 {
		t.Fatalf("после замены манифест = %+v, want file=%s previous=%s", m, file2, file1)
	}
	if st := statusOf("weaver"); st == nil || st.File != file2 || !st.IntegrityOK || !st.HasPrevious {
		t.Fatalf("карточка weaver = %+v, want file=%s с previous", st, file2)
	}

	body, _ := json.Marshal(map[string]string{"world": "alpha", "role": "weaver"})
	rr := mcpRequest(t, handleSettingsModelRollback, http.MethodPost, settingsModelsRollbackPath, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("rollback: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	m = readManifest()
	if m.File != file1 || m.ModelRef != "org/m" || m.Previous != file2 {
		t.Fatalf("после отката манифест = %+v, want file=%s previous=%s", m, file1, file2)
	}
	if got := fileContent(file1); got != "model-v1" {
		t.Fatalf("после отката файл %s = %q, want model-v1", file1, got)
	}
	if st := statusOf("weaver"); st == nil || st.ModelRef != "org/m" || st.File != file1 || !st.IntegrityOK {
		t.Fatalf("карточка после отката = %+v, want org/m file=%s", st, file1)
	}
	world, err := db.OpenReaderAs(worldDBPath("alpha"), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer world.Close()
	var ref string
	if err := world.QueryRow(`SELECT model_ref FROM model_bindings WHERE role = 'weaver'`).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	if ref != "org/m" {
		t.Fatalf("binding после отката = %q, want org/m", ref)
	}

	// Вторая параллельная установка той же роли — 409.
	setModelJob("weaver", "org/m-v2", "downloading", 0.3, "")
	rr = install(srcV2, "org/m-v2")
	if rr.Code != http.StatusConflict {
		t.Fatalf("параллельная установка: status = %d, want 409; body=%s", rr.Code, rr.Body.String())
	}
	setModelJob("weaver", "org/m-v2", "installed", 1, "")

	// Старая раскладка читается как установленная; автопереименования нет.
	legacy := modelDir("embed")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, modelLegacyFile), []byte("bge-m3"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacySum := sha256.Sum256([]byte("bge-m3"))
	legacyManifest, _ := json.Marshal(modelManifest{
		Role: "embed", ModelRef: "bge-m3-Q8_0",
		Source: "https://huggingface.co/gpustack/bge-m3-GGUF/resolve/main/bge-m3-Q8_0.gguf",
		SHA256: hex.EncodeToString(legacySum[:]),
	})
	if err := os.WriteFile(filepath.Join(legacy, modelLegacyManifest), legacyManifest, 0o644); err != nil {
		t.Fatal(err)
	}
	st := statusOf("embed")
	if st == nil || st.Status != "installed" || st.File != "bge-m3-Q8_0.bin" || !st.IntegrityOK {
		t.Fatalf("карточка старой раскладки = %+v, want installed file=bge-m3-Q8_0.bin", st)
	}
	if _, err := os.Stat(filepath.Join(legacy, modelLegacyFile)); err != nil {
		t.Fatalf("старая раскладка переименована: %v", err)
	}
}
