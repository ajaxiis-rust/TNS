package prompts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/langpack"
	"tns65/internal/llmparams"
)

// reset снимает снимок файла и составы ролей после теста.
func reset(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		Set(nil)
		llmparams.SetRoleConfigs(nil)
	})
}

// TestLoadFile — загрузка prompts.json пакета: текст ключа идёт из файла.
func TestLoadFile(t *testing.T) {
	reset(t)
	dir := t.TempDir()
	body := `{"compose":"{\"task\":\"compose\",\"chunk\":\"{chunk}\",\"note\":\"файл\"}","weaver_instruction":"из файла"}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
		t.Fatalf("запись prompts.json: %v", err)
	}
	if err := Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := Text("weaver_instruction", "дефолт"); got != "из файла" {
		t.Fatalf("Text = %q, want текст файла", got)
	}
	got := Render("weaver", "compose", "дефолт", map[string]any{"chunk": "кусок"})
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("промпт не JSON: %v: %s", err, got)
	}
	if m["chunk"] != "кусок" || m["note"] != "файл" || m["task"] != "compose" {
		t.Fatalf("промпт файла не отрендерен: %s", got)
	}
}

// TestFallbackWithoutFile — файла нет: остаются дефолты Go, Load не падает.
func TestFallbackWithoutFile(t *testing.T) {
	reset(t)
	if err := Load(t.TempDir()); err != nil {
		t.Fatalf("Load без файла: %v", err)
	}
	if got := Text("compose", "дефолт"); got != "дефолт" {
		t.Fatalf("Text = %q, want дефолт Go", got)
	}
	if got := Render("weaver", "missing", "дефолт {step}", map[string]any{"step": 2}); got != "дефолт 2" {
		t.Fatalf("Render неизвестного ключа = %q", got)
	}
}

// TestBrokenFile — битый JSON файла виден на старте пакета.
func TestBrokenFile(t *testing.T) {
	reset(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{"), 0o644); err != nil {
		t.Fatalf("запись prompts.json: %v", err)
	}
	if err := Load(dir); err == nil {
		t.Fatal("битый prompts.json принят как валидный")
	}
}

// TestRoleOverride — правка world_meta role.<роль>.prompts главнее файла: строка
// дополняет JSON-промпт полем extra, JSON-объект заменяет его целиком.
func TestRoleOverride(t *testing.T) {
	reset(t)
	Set(map[string]string{"compose": `{"task":"compose","chunk":"{chunk}"}`})
	llmparams.SetRoleConfigs(map[string]llmparams.RoleConfig{
		"compose": {Prompts: "пиши короче"},
	})

	got := Render("compose", "compose", "", map[string]any{"chunk": "кусок"})
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("промпт не JSON: %v: %s", err, got)
	}
	if m["extra"] != "пиши короче" || m["chunk"] != "кусок" {
		t.Fatalf("правка роли не дошла до промпта: %s", got)
	}

	llmparams.SetRoleConfigs(map[string]llmparams.RoleConfig{
		"compose": {Prompts: `{"task":"compose","chunk":"{chunk}","own":true}`},
	})
	got = Render("compose", "compose", "", map[string]any{"chunk": "кусок"})
	if !strings.Contains(got, `"own":true`) || strings.Contains(got, "extra") {
		t.Fatalf("JSON-правка роли не заменила шаблон: %s", got)
	}
}

// TestRenderValues — заготовки {chunk}/{text} и значения-списки подставляются
// без потери формы промпта.
func TestRenderValues(t *testing.T) {
	reset(t)
	got := Render("", "cast", `{"task":"cast-role","hero":"{hero}","chunk":"{chunk}","roles":"{roles}"}`,
		map[string]any{"hero": "Иван", "chunk": "кусок", "roles": []string{"наставник", "союзник"}})
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("промпт не JSON: %v: %s", err, got)
	}
	roles, ok := m["roles"].([]any)
	if !ok || len(roles) != 2 || roles[0] != "наставник" {
		t.Fatalf("список ролей потерян: %s", got)
	}
	if m["hero"] != "Иван" {
		t.Fatalf("герой не подставлен: %s", got)
	}

	if got := Substitute("Путь: {text}", map[string]any{"text": "а"}); got != "Путь: а" {
		t.Fatalf("текстовая подстановка = %q", got)
	}
}

// TestJSONShape — форма промпта сохраняется: JSON-обёртка с полями в порядке
// map-marshal, как строили прежние сборщики промптов.
func TestJSONShape(t *testing.T) {
	reset(t)
	if got, want := Render("", "compose", `{"task":"compose","chunk":"{chunk}"}`, map[string]any{"chunk": "кусок"}),
		`{"chunk":"кусок","task":"compose"}`; got != want {
		t.Fatalf("compose = %s, want %s", got, want)
	}
	if got, want := Render("", "editor", `{"task":"editor","scene":"{scene}","punct":"{punct}"}`,
		map[string]any{"scene": "с", "punct": []string{"тире", "ёлочки"}}),
		`{"punct":["тире","ёлочки"],"scene":"с","task":"editor"}`; got != want {
		t.Fatalf("editor = %s, want %s", got, want)
	}
}

// TestManifestPin — prompts.json пакета ru закреплён в manifest.json по sha256,
// как словари.
func TestManifestPin(t *testing.T) {
	packDir := filepath.Join("..", "..", "packs", "ru")
	raw, err := os.ReadFile(filepath.Join(packDir, FileName))
	if err != nil {
		t.Skipf("packs/ru/%s нет: %v", FileName, err)
	}
	m, err := langpack.LoadManifest(filepath.Join(packDir, "manifest.json"))
	if err != nil {
		t.Fatalf("manifest.json: %v", err)
	}
	sum := sha256.Sum256(raw)
	want := hex.EncodeToString(sum[:])
	if m.Files[FileName] != want {
		t.Fatalf("пин %s = %q, want %q", FileName, m.Files[FileName], want)
	}
	if err := Load(packDir); err != nil {
		t.Fatalf("Load пакета: %v", err)
	}
	for _, key := range []string{
		"weaver_instruction", "compose", "ladder.step1", "ladder.step2", "ladder.step3",
		"cast", "horoscope", "pythia", "editor", "editor_digest", "style",
		"style_block_header", "epoch", "forge_card", "forge_ring", "atlas_view",
		"scout", "autocritic", "news_scene", "idiolect",
		"turn.weaver_preset", "turn.bank_preset",
		"book.claim", "book.wake", "book.world_notes",
		"deepread.book", "deepread.bookmap",
		"dispute", "sense_dispute", "geo.travel",
	} {
		if got := Text(key, ""); got == "" {
			t.Fatalf("в prompts.json нет ключа %q", key)
		}
	}
}
