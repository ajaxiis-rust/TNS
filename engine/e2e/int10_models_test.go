//go:build e2e

package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestINT10 — INT-10. Жизненный цикл модели (T-13-11, MD-08): карточка статуса
// следует варианту из манифеста (model_ref), а не роли; статус установки,
// целостность и размер видны по HTTP; удаление снимает файлы варианта, задание
// реестра и зовёт остановку ребёнка роли через надзиратель.
func TestINT10(t *testing.T) {
	bin := buildBinary(t, []string{"e2e"})
	dir := worldDir(t)
	linkConfig(t, dir)

	slug := "zhizn-modeli"
	ancketa := writeAncketa(t, t.TempDir(), slug,
		"Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.")
	if stdout, stderr, code := runCmd(t, dir, bin, "world", "birth", "--ancketa", ancketa, "--fake-embed", "--fake-slot"); code != 0 {
		t.Fatalf("world birth exit=%d, want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	c := startCore(t, dir, bin)
	report := map[string]any{}

	// Шаг 1: вариант Q4_K_M роли embed установлен (файл + учётная запись версии +
	// манифест-указатель — раскладка model_files.go).
	writeModelVariant(t, dir, "embed", "bge-m3-Q4_K_M", "bge-m3-Q4_K_M.gguf", "")
	cards := modelCards(t, c, "?role=embed")
	report["cards_embed"] = len(cards)
	if len(cards) != 1 {
		t.Fatalf("карточек embed = %d, want 1 (%+v)", len(cards), cards)
	}
	card := cards[0]
	report["model_ref_q4"] = card.ModelRef
	if card.ModelRef != "bge-m3-Q4_K_M" || card.File != "bge-m3-Q4_K_M.gguf" {
		t.Fatalf("карточка = %+v, want вариант Q4_K_M", card)
	}
	if card.Status != "installed" || card.Progress != 1 {
		t.Fatalf("статус установки = %q/%v, want installed/1", card.Status, card.Progress)
	}
	if !card.IntegrityOK {
		t.Fatalf("integrity_ok = false для целого файла: %+v", card)
	}

	// Шаг 2: выбран другой вариант той же роли — статус показывает новый
	// model_ref, а не имя роли: карточка привязана к варианту (T-13-11).
	writeModelVariant(t, dir, "embed", "bge-m3-Q8_0", "bge-m3-Q8_0.gguf", "bge-m3-Q4_K_M.gguf")
	cards = modelCards(t, c, "?role=embed")
	if len(cards) != 1 {
		t.Fatalf("карточек embed после смены варианта = %d, want 1", len(cards))
	}
	card = cards[0]
	report["model_ref_q8"] = card.ModelRef
	report["has_previous"] = card.HasPrevious
	if card.ModelRef != "bge-m3-Q8_0" || card.File != "bge-m3-Q8_0.gguf" {
		t.Fatalf("карточка после смены варианта = %+v, want Q8_0", card)
	}
	if !card.HasPrevious {
		t.Fatal("has_previous = false, want прежний вариант Q4_K_M")
	}

	// Шаг 3: вторая роль видна отдельной карточкой; фильтр по роли изолирует её.
	writeModelVariant(t, dir, "weaver", "llama-3.2-3b-q4", "llama-3.2-3b-q4.gguf", "")
	all := modelCards(t, c, "")
	report["cards_all"] = len(all)
	if len(all) != 2 || all[0].Role != "embed" || all[1].Role != "weaver" {
		t.Fatalf("карточки без фильтра = %+v, want embed+weaver", all)
	}
	if got := modelCards(t, c, "?role=weaver"); len(got) != 1 || got[0].ModelRef != "llama-3.2-3b-q4" {
		t.Fatalf("карточки weaver = %+v", got)
	}

	// Шаг 4: удаление роли embed — 200, сняты файлы обоих вариантов и манифест;
	// перед снятием файлов вызвана остановка ребёнка роли (хук надзирателя).
	var del struct {
		Status  string   `json:"status"`
		Removed []string `json:"removed"`
	}
	if code := coreJSON(t, c, http.MethodDelete, "/api/settings/models/delete",
		map[string]string{"role": "embed"}, &del); code != http.StatusOK {
		t.Fatalf("delete embed: status = %d, want 200", code)
	}
	report["removed"] = len(del.Removed)
	if del.Status != "ok" || len(del.Removed) < 3 {
		t.Fatalf("delete embed = %+v, want ok и снятые файлы", del)
	}
	if _, err := os.Stat(filepath.Join(dir, "world_db", "models", "embed")); !os.IsNotExist(err) {
		t.Fatalf("каталог embed после удаления на месте: %v", err)
	}
	journalText := readJournal(t, dir)
	report["child_stop_event"] = strings.Contains(journalText, `"event":"model.child.stopped"`)
	if !report["child_stop_event"].(bool) {
		t.Fatalf("в журнале нет остановки ребёнка роли:\n%s", journalText)
	}
	report["deleted_event"] = strings.Contains(journalText, `"event":"model.deleted"`)
	if !report["deleted_event"].(bool) {
		t.Fatalf("в журнале нет model.deleted:\n%s", journalText)
	}

	// Шаг 5: удалённого варианта больше нет в статусе, повторное удаление — 404.
	if got := modelCards(t, c, "?role=embed"); len(got) != 0 {
		t.Fatalf("карточки embed после удаления = %+v, want пусто", got)
	}
	if code := coreJSON(t, c, http.MethodDelete, "/api/settings/models/delete",
		map[string]string{"role": "embed"}, nil); code != http.StatusNotFound {
		t.Fatalf("повторное удаление: status = %d, want 404", code)
	}

	writeReport(t, "int10", report)
}

// e2eModelVariant — учётная запись варианта (<файл>.json) раскладки моделей.
type e2eModelVariant struct {
	Role        string `json:"role"`
	ModelRef    string `json:"model_ref"`
	Source      string `json:"source"`
	File        string `json:"file"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	InstalledAt string `json:"installed_at"`
}

// e2eModelManifest — манифест-указатель роли (manifest.json).
type e2eModelManifest struct {
	Role        string `json:"role"`
	ModelRef    string `json:"model_ref"`
	Source      string `json:"source"`
	File        string `json:"file"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	InstalledAt string `json:"installed_at"`
	Previous    string `json:"previous"`
}

// writeModelVariant кладёт файл варианта, его учётную запись и манифест-указатель
// роли — та же раскладка, что пишет установка модели (MD-08.1). previous —
// прежний файл роли, пусто — прежней версии нет.
func writeModelVariant(t *testing.T, dir, role, modelRef, file, previous string) {
	t.Helper()
	modelDir := filepath.Join(dir, "world_db", "models", role)
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("mkdir модели: %v", err)
	}
	content := []byte("e2e-model:" + modelRef)
	if err := os.WriteFile(filepath.Join(modelDir, file), content, 0o644); err != nil {
		t.Fatalf("write модели: %v", err)
	}
	sum := sha256.Sum256(content)
	const source = "https://huggingface.co/org/bge-m3/resolve/main/"
	const installedAt = "2026-11-13T09:00:00Z"
	ver := e2eModelVariant{
		Role: role, ModelRef: modelRef, Source: source + file, File: file,
		SHA256: hex.EncodeToString(sum[:]), Size: int64(len(content)), InstalledAt: installedAt,
	}
	raw, err := json.Marshal(ver)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, file+".json"), raw, 0o644); err != nil {
		t.Fatalf("write учётной записи: %v", err)
	}
	m := e2eModelManifest{
		Role: role, ModelRef: modelRef, Source: source + file, File: file,
		SHA256: ver.SHA256, Size: ver.Size, InstalledAt: installedAt, Previous: previous,
	}
	raw, err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "manifest.json"), raw, 0o644); err != nil {
		t.Fatalf("write манифеста: %v", err)
	}
}

// modelStatusCard — карточка модели из GET /api/settings/models/status.
type modelStatusCard struct {
	Role        string  `json:"role"`
	ModelRef    string  `json:"model_ref"`
	Source      string  `json:"source"`
	File        string  `json:"file"`
	Size        int64   `json:"size"`
	Status      string  `json:"status"`
	Progress    float64 `json:"progress"`
	IntegrityOK bool    `json:"integrity_ok"`
	HasPrevious bool    `json:"has_previous"`
}

// modelCards читает карточки статуса моделей (query — готовые параметры).
func modelCards(t *testing.T, c *core, query string) []modelStatusCard {
	t.Helper()
	var out struct {
		Models []modelStatusCard `json:"models"`
	}
	if code := coreJSON(t, c, http.MethodGet, "/api/settings/models/status"+query, nil, &out); code != http.StatusOK {
		t.Fatalf("GET models/status%s: status = %d, want 200", query, code)
	}
	return out.Models
}

// coreJSON выполняет запрос к ядру и при необходимости разбирает JSON-ответ.
func coreJSON(t *testing.T, c *core, method, path string, body, out any) int {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	var req *http.Request
	var err error
	if reader != nil {
		req, err = http.NewRequest(method, c.url+path, reader)
	} else {
		req, err = http.NewRequest(method, c.url+path, nil)
	}
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
	return resp.StatusCode
}
