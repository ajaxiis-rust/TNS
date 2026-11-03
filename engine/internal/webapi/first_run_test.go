package webapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// seedFirstRun подменяет firstRunStatePath на файл в t.TempDir() и возвращает
// путь. Файла изначально нет — это и есть «первый запуск».
func seedFirstRun(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "first-run.json")
	old := firstRunStatePath
	firstRunStatePath = path
	t.Cleanup(func() { firstRunStatePath = old })
	return path
}

func getFirstRun(t *testing.T) firstRunResponse {
	t.Helper()
	rr := mcpRequest(t, handleFirstRunGet, http.MethodGet, firstRunPathRoute, nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", firstRunPathRoute, rr.Code, rr.Body.String())
	}
	var out firstRunResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal first-run: %v", err)
	}
	return out
}

func putFirstRunStep(t *testing.T, step string) firstRunResponse {
	t.Helper()
	body := []byte(`{"step":"` + step + `"}`)
	rr := mcpRequest(t, handleFirstRunStep, http.MethodPut, firstRunStepRoute, body, "application/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT %s step %s: status = %d, want 200; body=%s", firstRunStepRoute, step, rr.Code, rr.Body.String())
	}
	var out firstRunResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal first-run step: %v", err)
	}
	return out
}

// TestFirstRunStartsIncomplete — «Проверка» T-12-20: без state/first-run.json
// мастер не завершён, шаги не пройдены, а файл ещё не создан.
func TestFirstRunStartsIncomplete(t *testing.T) {
	path := seedFirstRun(t)
	out := getFirstRun(t)
	if out.Complete {
		t.Fatalf("complete = true, want false without state file")
	}
	if len(out.Steps) != 0 {
		t.Fatalf("steps = %d, want 0 without state file", len(out.Steps))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("state file must not exist before any step; stat err=%v", err)
	}
}

// TestFirstRunStepIdempotent — повторная отметка уже пройденного шага не меняет
// состояние и не сдвигает отметку времени.
func TestFirstRunStepIdempotent(t *testing.T) {
	seedFirstRun(t)
	out := putFirstRunStep(t, "1")
	if !out.Steps["1"].Done || out.Steps["1"].At == "" {
		t.Fatalf("step 1 not marked done: %+v", out.Steps["1"])
	}
	first := out.Steps["1"].At
	out = putFirstRunStep(t, "1")
	if !out.Steps["1"].Done || out.Steps["1"].At != first {
		t.Fatalf("re-marking step 1 changed state: %+v", out.Steps["1"])
	}
	if out.Complete {
		t.Fatalf("complete = true after step 1, want false")
	}
}

// TestFirstRunCompletesAtStep4 — шаг 4 завершает мастера; повторный «запуск»
// (новый GET) читает состояние заново и пропускает пройденные шаги.
func TestFirstRunCompletesAtStep4(t *testing.T) {
	seedFirstRun(t)
	for _, s := range []string{"0", "1", "2", "3"} {
		putFirstRunStep(t, s)
	}
	out := putFirstRunStep(t, "4")
	if !out.Complete {
		t.Fatalf("complete = false after step 4, want true")
	}
	again := getFirstRun(t)
	if !again.Complete {
		t.Fatalf("re-read complete = false, want true")
	}
	for _, s := range []string{"0", "1", "2", "3", "4"} {
		if !again.Steps[s].Done {
			t.Fatalf("step %s lost after re-read", s)
		}
	}
}

// TestFirstRunRejectsBadStep — неверный шаг отвергается по контракту ошибок.
func TestFirstRunRejectsBadStep(t *testing.T) {
	seedFirstRun(t)
	rr := mcpRequest(t, handleFirstRunStep, http.MethodPut, firstRunStepRoute, []byte(`{"step":"9"}`), "application/json")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("PUT step 9: status = %d, want 400", rr.Code)
	}
}

// TestWizardScreenServed — экран мастера отдаётся из embed.
func TestWizardScreenServed(t *testing.T) {
	rr := mcpRequest(t, handleWizardScreen, http.MethodGet, wizardScreenPath, nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", wizardScreenPath, rr.Code)
	}
	if rr.Body.Len() == 0 {
		t.Fatalf("GET %s: empty body", wizardScreenPath)
	}
}
