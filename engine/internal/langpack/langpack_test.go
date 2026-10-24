package langpack

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// engineRoot возвращает корень engine/ — каталог с go.mod.
func engineRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("engine root (go.mod) not found")
		}
		dir = parent
	}
}

func TestManifest(t *testing.T) {
	dir := filepath.Join(engineRoot(t), "packs", "ru")
	p, err := Open(dir)
	if err != nil {
		t.Fatalf("Open(%s): %v", dir, err)
	}

	m := p.Manifest
	if m.ID != "ru" || m.Lang != "ru" || m.Version != "1.0.0" {
		t.Fatalf("манифест: id=%q lang=%q version=%q", m.ID, m.Lang, m.Version)
	}
	if m.Lemmatizer.Kind == "" || m.Lemmatizer.Protocol == "" {
		t.Fatalf("лемматизатор не зафиксирован: %+v", m.Lemmatizer)
	}
	if len(m.Files) != 4 {
		t.Fatalf("файлов в манифесте: %d, want 4", len(m.Files))
	}

	// Контракты форматов (шаг 6).
	intents := readLines(t, filepath.Join(dir, "lexicon_intents.jsonl"))
	moveSeen := false
	for _, line := range intents {
		if line == "" {
			continue
		}
		var e struct {
			Intent string   `json:"intent"`
			Lemmas []string `json:"lemmas"`
			Verbs  []string `json:"verbs"`
		}
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("строка лексикона не JSON: %v", err)
		}
		if e.Intent == "" || len(e.Lemmas) == 0 || len(e.Verbs) == 0 {
			t.Fatalf("строка лексикона без intent/lemmas/verbs: %s", line)
		}
		if e.Intent == "перемещение" {
			moveSeen = true
		}
	}
	if !moveSeen {
		t.Fatal("намерение «перемещение» отсутствует в lexicon_intents.jsonl")
	}

	for _, line := range readLines(t, filepath.Join(dir, "stoplist.txt")) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.ContainsAny(line, " \t") {
			t.Fatalf("stoplist.txt: в строке больше одной леммы: %q", line)
		}
	}

	var times struct {
		Units        map[string]string `json:"units"`
		Numerals     map[string]int    `json:"numerals"`
		Prepositions []string          `json:"prepositions"`
	}
	if err := readJSON(t, filepath.Join(dir, "detectors_time.json"), &times); err != nil {
		t.Fatal(err)
	}
	if times.Units["час"] != "hours" || times.Units["день"] != "days" {
		t.Fatalf("detectors_time.json units: %v", times.Units)
	}
	if times.Numerals["три"] != 3 {
		t.Fatalf("detectors_time.json numerals: %v", times.Numerals)
	}
	if !contains(times.Prepositions, "через") || !contains(times.Prepositions, "спустя") {
		t.Fatalf("detectors_time.json prepositions: %v", times.Prepositions)
	}

	var mood struct {
		Mood   string   `json:"mood"`
		Lemmas []string `json:"lemmas"`
	}
	if err := readJSON(t, filepath.Join(dir, "detectors_mood.json"), &mood); err != nil {
		t.Fatal(err)
	}
	if mood.Mood == "" || len(mood.Lemmas) == 0 {
		t.Fatalf("detectors_mood.json: mood=%q lemmas=%v", mood.Mood, mood.Lemmas)
	}
}

func TestManifestBrokenFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stoplist.txt"), []byte("и\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Manifest{
		ID:      "ru",
		Name:    "Русский",
		Lang:    "ru",
		Version: "1.0.0",
		Files:   map[string]string{"stoplist.txt": "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Open(dir)
	var bfe *BrokenFilesError
	if !errors.As(err, &bfe) {
		t.Fatalf("ожидался *BrokenFilesError, получен %T: %v", err, err)
	}
	if len(bfe.Files) != 1 || bfe.Files[0] != "stoplist.txt" {
		t.Fatalf("перечень битых файлов: %v", bfe.Files)
	}
}

func TestRollback(t *testing.T) {
	dir := t.TempDir()
	writeVersion(t, dir, "0.9.0", "старый")
	writeVersion(t, dir, "1.0.0", "новый")
	if err := writeCurrent(dir, "1.0.0"); err != nil {
		t.Fatal(err)
	}

	got, err := Rollback(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "0.9.0" {
		t.Fatalf("Rollback = %q, want 0.9.0", got)
	}
	cur, err := currentVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cur != "0.9.0" {
		t.Fatalf("current = %q, want 0.9.0", cur)
	}
}

func TestRollbackBrokenTarget(t *testing.T) {
	dir := t.TempDir()
	writeVersion(t, dir, "0.9.0", "старый")
	writeVersion(t, dir, "1.0.0", "новый")
	if err := writeCurrent(dir, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	// Портим файл целевой версии: сумма не сойдётся, откат обязан отказать.
	if err := os.WriteFile(filepath.Join(dir, "versions", "0.9.0", "stoplist.txt"), []byte("бито\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Rollback(dir); err == nil {
		t.Fatal("ожидался отказ отката на битой сумме")
	}
	cur, err := currentVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cur != "1.0.0" {
		t.Fatalf("указатель изменился при неудачном откате: %q", cur)
	}
}

func TestBuildHasTranslate(t *testing.T) {
	root := engineRoot(t)
	var hits []string
	buildHasCall := false
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Contains(b, []byte("translateSeed")) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		hits = append(hits, rel)
		if rel == "internal/langpack/build.go" && bytes.Contains(b, []byte("translateSeed(")) {
			buildHasCall = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(hits) != 1 || hits[0] != "internal/langpack/build.go" {
		t.Fatalf("вызов перевода должен жить только в internal/langpack/build.go, найден в: %v", hits)
	}
	if !buildHasCall {
		t.Fatal("в internal/langpack/build.go нет вызова translateSeed")
	}
}

// writeVersion создаёт каталог versions/<version> с манифестом и одним файлом.
func writeVersion(t *testing.T, dir, version, text string) {
	t.Helper()
	vd := filepath.Join(dir, "versions", version)
	if err := os.MkdirAll(vd, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(vd, "stoplist.txt")
	if err := os.WriteFile(path, []byte(text+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := sha256File(path)
	if err != nil {
		t.Fatal(err)
	}
	m := Manifest{
		ID:      "ru",
		Name:    "Русский",
		Lang:    "ru",
		Version: version,
		Files:   map[string]string{"stoplist.txt": sum},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vd, "manifest.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(b), "\n")
}

func readJSON(t *testing.T, path string, v any) error {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
