package textlimit

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	tnsweb "tns65/web"
)

// tokenLimitMarker — строка-маркер с константами в web/static/tokenlimit.js.
const tokenLimitMarker = "// TOKENLIMIT-CONST "

// jsConst — JSON из маркера: веса, делитель и лимит по умолчанию.
type jsConst struct {
	CyrillicWeight int `json:"cyrillic_weight"`
	OtherWeight    int `json:"other_weight"`
	UnitsPerToken  int `json:"units_per_token"`
	DefaultLimit   int `json:"default_limit"`
}

// readTokenLimitJS читает вшитый в бинарник скрипт счётчика.
func readTokenLimitJS(t *testing.T) string {
	t.Helper()
	raw, err := tnsweb.FS.ReadFile("static/tokenlimit.js")
	if err != nil {
		t.Fatalf("нет web/static/tokenlimit.js в embed: %v", err)
	}
	return string(raw)
}

// parityTexts — набор текстов для сверки Go и JS: пустой, латиница,
// кириллица, смесь и суррогатная пара.
func parityTexts() []string {
	return []string{
		"",
		"hello world",
		strings.Repeat("я", 100),
		"Привет, world! Ёж идёт.",
		strings.Repeat("a", 8000),
		"😀 эмодзи и текст",
	}
}

// TestTokenLimitJSConstants — константы формулы и лимит в JS совпадают с Go.
func TestTokenLimitJSConstants(t *testing.T) {
	src := readTokenLimitJS(t)
	idx := strings.Index(src, tokenLimitMarker)
	if idx < 0 {
		t.Fatalf("в tokenlimit.js нет маркера %q", tokenLimitMarker)
	}
	rest := src[idx+len(tokenLimitMarker):]
	line, _, _ := strings.Cut(rest, "\n")

	var got jsConst
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &got); err != nil {
		t.Fatalf("маркер TOKENLIMIT-CONST не JSON: %v (%q)", err, line)
	}
	want := jsConst{
		CyrillicWeight: CyrillicWeight,
		OtherWeight:    OtherWeight,
		UnitsPerToken:  UnitsPerToken,
		DefaultLimit:   DefaultLimit,
	}
	if got != want {
		t.Fatalf("константы JS = %+v, want %+v", got, want)
	}
}

// TestTokenLimitJSExecution — исполнение tokenlimit.js в node даёт те же числа,
// что Go. Без node проверка исполнения пропускается с явной причиной (политика
// scripts/ui-check.sh: на машине игрока node нет).
func TestTokenLimitJSExecution(t *testing.T) {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Log("node недоступен: исполнение tokenlimit.js пропущено (константы сверены отдельным тестом)")
		return
	}

	src := readTokenLimitJS(t)
	jsPath := t.TempDir() + "/tokenlimit.js"
	if err := os.WriteFile(jsPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	texts := parityTexts()
	payload, err := json.Marshal(texts)
	if err != nil {
		t.Fatal(err)
	}
	const script = `const t = require(process.argv[1]);
const texts = JSON.parse(process.argv[2]);
console.log(JSON.stringify({
  values: texts.map(function (x) { return t.estimate(x); }),
  hint: t.hint(t.KINDS.playerMessage, 2001, 2000)
}));`
	out, err := exec.Command(nodeBin, "-e", script, jsPath, string(payload)).Output()
	if err != nil {
		t.Fatalf("node tokenlimit.js: %v", err)
	}

	var got struct {
		Values []int  `json:"values"`
		Hint   string `json:"hint"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("ответ node не JSON: %v (%s)", err, out)
	}
	if len(got.Values) != len(texts) {
		t.Fatalf("оценок из node: %d, want %d", len(got.Values), len(texts))
	}
	for i, text := range texts {
		if want := Estimate(text); got.Values[i] != want {
			t.Fatalf("текст %q: JS = %d, Go = %d", text, got.Values[i], want)
		}
	}
	if want := Hint(KindPlayerMessage, 2001, 2000); got.Hint != want {
		t.Fatalf("подсказка JS = %q, want %q", got.Hint, want)
	}
}
