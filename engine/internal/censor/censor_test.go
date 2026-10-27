package censor

import (
	"errors"
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

// errLemmatizer — подставной лемматизатор с постоянным отказом.
type errLemmatizer struct{}

func (errLemmatizer) Lemmatize(string) ([]lemmarpc.Token, error) {
	return nil, errors.New("лемматизатор недоступен")
}

// TestCheckErr — причина инфра-отказа наружу (E-01): CheckErr возвращает ошибку
// лемматизатора, Check остаётся fail-closed (false, nil) с тем же контрактом.
func TestCheckErr(t *testing.T) {
	checker := New(errLemmatizer{}, []string{"дурак"})

	if _, _, err := checker.CheckErr("любой текст"); err == nil {
		t.Fatal("CheckErr скрыл отказ лемматизатора: err=nil")
	}
	ok, hits := checker.Check("любой текст")
	if ok || hits != nil {
		t.Fatalf("Check при отказе лемматизатора: ok=%v hits=%v, want false nil", ok, hits)
	}

	clean := New(fakeLemmatizer{forms: forbiddenForms()}, []string{"дурак"})
	ok, hits, err := clean.CheckErr("добрый человек")
	if err != nil {
		t.Fatalf("CheckErr на чистом тексте: err=%v, want nil", err)
	}
	if !ok || len(hits) != 0 {
		t.Fatalf("CheckErr на чистом тексте: ok=%v hits=%v", ok, hits)
	}
}

// TestCheckerCache — чекер языка собирается один раз на процесс (E-01): повторный
// запрос даёт тот же экземпляр; язык без словаря/бинарника кэшируется отказом
// (nil) без пересбора.
func TestCheckerCache(t *testing.T) {
	defer Close()

	root := t.TempDir()
	packRU := filepath.Join(root, "packs", "ru")
	if err := os.MkdirAll(packRU, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packRU, stoplistFile), []byte("дурак\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"lemmatizer": {"binary": "несуществующий-резидент"}}`
	if err := os.WriteFile(filepath.Join(packRU, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	first := checkerFor(root, "ru")
	if first == nil {
		t.Fatal("checkerFor: чекер не собран при словаре и бинарнике в манифесте")
	}
	if second := checkerFor(root, "ru"); second != first {
		t.Fatal("checkerFor: повторный вызов собрал новый чекер, кэш не работает")
	}

	if c := checkerFor(root, "en"); c != nil {
		t.Fatalf("checkerFor: язык без пакета дал чекер %v, want nil", c)
	}
	if c := checkerFor(root, "en"); c != nil {
		t.Fatalf("checkerFor: повторный вызов для языка без пакета дал %v, want nil (кэш отказа)", c)
	}
}

// TestStoplistGolden — golden-согласованность словаря и лемматизатора (E-01):
// каждая строка packs/ru/stoplist.txt — одно слово, и реальный резидент пакета
// лемматизирует его в ту же лемму (идемпотентность). Ловит расхождение
// словаря с лемматизатором («ё»/«е», словоформа вместо леммы) — иначе стоп-слово
// тихо перестаёт ловиться. Без бинарника tns65-lemmaru тест пропускается.
func TestStoplistGolden(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Skip("корень движка не найден — golden пакета недоступен")
	}
	packDir := filepath.Join(root, "packs", "ru")
	binary := packLemmatizerBinary(root, packDir)
	if binary == "" {
		t.Skip("манифест пакета не задаёт бинарник лемматизатора")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Skipf("бинарник лемматизатора недоступен: %v", err)
	}

	stoplist, err := LoadStoplist(packDir)
	if err != nil {
		t.Fatalf("LoadStoplist: %v", err)
	}
	if len(stoplist) == 0 {
		t.Fatal("stoplist.txt пакета пуст")
	}

	client := lemmarpc.NewClient(binary, nil)
	defer client.Close()
	for _, line := range stoplist {
		if strings.ContainsAny(line, " \t") {
			t.Errorf("stoplist %q: строка содержит пробел — контракт «одна лемма в строке»", line)
			continue
		}
		tokens, err := client.Lemmatize(line)
		if err != nil {
			t.Errorf("stoplist %q: лемматизатор отказал: %v", line, err)
			continue
		}
		if len(tokens) == 0 {
			t.Errorf("stoplist %q: лемматизатор вернул 0 токенов", line)
			continue
		}
		if got := strings.ToLower(tokens[0].Lemma); got != line {
			t.Errorf("stoplist %q: лемматизатор вернул лемму %q — словарь и резидент разошлись", line, got)
		}
	}
}
