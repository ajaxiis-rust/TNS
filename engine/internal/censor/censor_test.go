package censor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/lemmarpc"
)

// fakeLemmatizer — подставной лемматизатор: лемма из карты форм, иначе — само слово.
type fakeLemmatizer struct {
	forms map[string]string
}

func (f fakeLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error) {
	var tokens []lemmarpc.Token
	for _, w := range strings.Fields(text) {
		lemma := f.forms[w]
		if lemma == "" {
			lemma = w
		}
		tokens = append(tokens, lemmarpc.Token{Form: w, Lemma: lemma, Pos: "NOUN"})
	}
	return tokens, nil
}

// forbiddenForms — словоформы трёх запретных слов фикстуры → их лемма.
func forbiddenForms() map[string]string {
	return map[string]string{
		"дурак":  "дурак",
		"дурака": "дурак", // косвенная форма, ловится по лемме.
		"кровь":  "кровь",
		"кровью": "кровь",
		"кража":  "кража",
		"кражу":  "кража",
	}
}

// fixtureStoplist пишет фикстуру stoplist.txt с тремя запретными леммами и
// возвращает каталог пакета. Словарь — файл пакета, не таблица базы (D-14.2).
func fixtureStoplist(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	content := "дурак\nкровь\nкража\n"
	if err := os.WriteFile(filepath.Join(dir, stoplistFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestCheck — чистый текст проходит с ok=true; запретное слово блокируется,
// словарь читается из stoplist.txt пакета, а не из базы.
func TestCheck(t *testing.T) {
	packDir := fixtureStoplist(t)
	stoplist, err := LoadStoplist(packDir)
	if err != nil {
		t.Fatalf("LoadStoplist: %v", err)
	}
	if len(stoplist) != 3 {
		t.Fatalf("словарь пакета = %v, want ровно 3 леммы", stoplist)
	}
	checker := New(fakeLemmatizer{forms: forbiddenForms()}, stoplist)

	ok, hits := checker.Check("добрый человек идёт по дороге")
	if !ok {
		t.Fatalf("чистый текст заблокирован: ok=%v hits=%v", ok, hits)
	}
	if len(hits) != 0 {
		t.Fatalf("чистый текст дал hits=%v, want пусто", hits)
	}

	ok, hits = checker.Check("дурак идёт")
	if ok {
		t.Fatalf("запретное слово прошло: ok=%v hits=%v", ok, hits)
	}
	if !contains(hits, "дурак") {
		t.Fatalf("hits=%v, want содержит «дурак»", hits)
	}
}

// TestLemmaForms — запретное слово в исходной форме и в словоформе ловится по
// лемме (T-04-11, шаг 3).
func TestLemmaForms(t *testing.T) {
	packDir := fixtureStoplist(t)
	stoplist, err := LoadStoplist(packDir)
	if err != nil {
		t.Fatalf("LoadStoplist: %v", err)
	}
	checker := New(fakeLemmatizer{forms: forbiddenForms()}, stoplist)

	cases := []struct {
		text string
		hit  string
	}{
		{"дурак", "дурак"},
		{"дурака", "дурак"},
		{"кровь", "кровь"},
		{"кровью", "кровь"},
		{"кража", "кража"},
		{"кражу", "кража"},
	}
	for _, tc := range cases {
		ok, hits := checker.Check(tc.text)
		if ok {
			t.Errorf("%q прошёл, want заблокирован", tc.text)
			continue
		}
		if !contains(hits, tc.hit) {
			t.Errorf("%q: hits=%v, want содержит %q", tc.text, hits, tc.hit)
		}
	}
}

func contains(items []string, want string) bool {
	for _, it := range items {
		if it == want {
			return true
		}
	}
	return false
}
