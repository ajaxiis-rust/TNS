package cast_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/cast"
	"tns65/internal/lemmarpc"
)

// fakeLemmatizer — подставной лемматизатор скана: лемма и теги из карты форм,
// иначе — лемма-форма в нижнем регистре (как в censor_test).
type fakeLemmatizer struct {
	forms map[string]fakeLemma
}

type fakeLemma struct {
	lemma string
	pos   string
	tags  []string
}

func (f fakeLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error) {
	var tokens []lemmarpc.Token
	for _, w := range strings.Fields(text) {
		w = strings.Trim(w, ".,!?;:—–-«»\"'()[]")
		info, ok := f.forms[w]
		if !ok {
			info = fakeLemma{lemma: strings.ToLower(w), pos: "NOUN"}
		}
		tokens = append(tokens, lemmarpc.Token{Form: w, Lemma: info.lemma, Pos: info.pos, Tags: info.tags})
	}
	return tokens, nil
}

// scanForms — словоформы фикстуры: падежные формы сливаются по лемме,
// служебные слова и нарицательные отсеиваются профилем.
func scanForms() map[string]fakeLemma {
	return map[string]fakeLemma{
		"Снейп":     {lemma: "снейп", pos: "NOUN", tags: []string{"Surn"}},
		"Снейпа":    {lemma: "снейп", pos: "NOUN", tags: []string{"Surn"}},
		"Снейпу":    {lemma: "снейп", pos: "NOUN", tags: []string{"Surn"}},
		"Гарри":     {lemma: "гарри", pos: "NOUN", tags: []string{"Name"}},
		"Гермиона":  {lemma: "гермиона", pos: "NOUN", tags: []string{"Name"}},
		"Гермионе":  {lemma: "гермиона", pos: "NOUN", tags: []string{"Name"}},
		"Рон":       {lemma: "рон", pos: "NOUN", tags: []string{"Name"}},
		"Рона":      {lemma: "рон", pos: "NOUN", tags: []string{"Name"}},
		"И":         {lemma: "и", pos: "CONJ"},
		"В":         {lemma: "в", pos: "PR"},
		"На":        {lemma: "на", pos: "PR"},
		"Он":        {lemma: "он", pos: "NPRO"},
		"сказал":    {lemma: "сказать", pos: "VERB"},
		"говорили":  {lemma: "говорить", pos: "VERB"},
		"спросил":   {lemma: "спросить", pos: "VERB"},
		"Профессор": {lemma: "профессор", pos: "NOUN"},
		"профессор": {lemma: "профессор", pos: "NOUN"},
	}
}

// ruProfile — русский профиль скана (полный профиль в cast.Profile).
func ruProfile(t *testing.T) cast.LangProfile {
	t.Helper()
	p, ok := cast.Profile("ru")
	if !ok {
		t.Fatalf("профиль ru не найден")
	}
	return p
}

func findCandidate(cands []cast.Candidate, name string) *cast.Candidate {
	for i := range cands {
		if cands[i].Name == name {
			return &cands[i]
		}
	}
	return nil
}

func hasAlias(c *cast.Candidate, form string) bool {
	for _, a := range c.Aliases {
		if a == form {
			return true
		}
	}
	return false
}

// TestScanCandidates — «Проверка» T-12-22: синтетическая книга на 3 главы с 4
// заведомыми именами; все найдены, служебные слова отсеяны; «Снейп», «Снейпа»,
// «Снейпу» — одна запись.
func TestScanCandidates(t *testing.T) {
	chunks := []cast.Chunk{
		{Ord: 1, Chapter: "Глава 1", Text: "Снейп вошёл в зал. В зале сидел Гарри. Он сказал мало."},
		{Ord: 2, Chapter: "Глава 2", Text: "Снейпа видели у двери. Гермиона и Рон говорили. И Снейпу передали записку."},
		{Ord: 3, Chapter: "Глава 3", Text: "Профессор Снейп сказал Гермионе. На столе лежал пергамент."},
	}

	cands, err := cast.Scan(chunks, cast.ScanOptions{
		Lemmatizer: fakeLemmatizer{forms: scanForms()},
		Profile:    ruProfile(t),
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Все четыре заведомых имени найдены.
	for _, name := range []string{"Снейп", "Гарри", "Гермиона", "Рон"} {
		if findCandidate(cands, name) == nil {
			t.Errorf("имя %q не найдено; кандидаты: %+v", name, namesOf(cands))
		}
	}

	// Служебные слова и нарицательное отсеяны.
	for _, name := range []string{"И", "В", "На", "Он", "Профессор"} {
		if c := findCandidate(cands, name); c != nil {
			t.Errorf("служебное/нарицательное %q не отсеяно: %+v", name, c)
		}
	}

	// «Снейп», «Снейпа», «Снейпу» — одна запись, слитая по лемме.
	snape := findCandidate(cands, "Снейп")
	if snape == nil {
		t.Fatal("Снейп не найден")
	}
	if snape.Lemma != "снейп" {
		t.Errorf("Снейп.Lemma = %q, want «снейп»", snape.Lemma)
	}
	for _, form := range []string{"Снейп", "Снейпа", "Снейпу"} {
		if !hasAlias(snape, form) {
			t.Errorf("алиас %q отсутствует у Снейпа: %v", form, snape.Aliases)
		}
	}
	if snape.Mentions != 4 {
		t.Errorf("Снейп.Mentions = %d, want 4 (Снейп, Снейпа, Снейпу, Снейп)", snape.Mentions)
	}
	if snape.FirstChunk != 1 || snape.LastChunk != 3 {
		t.Errorf("Снейп first/last = %d/%d, want 1/3", snape.FirstChunk, snape.LastChunk)
	}
	if snape.ByChunk[1] != 1 || snape.ByChunk[2] != 2 || snape.ByChunk[3] != 1 {
		t.Errorf("Снейп.ByChunk = %v, want {1:1 2:2 3:1}", snape.ByChunk)
	}

	// Атрибуция реплики: «Снейп сказал» в главе 3 — глагол речи рядом.
	if snape.Speech == 0 {
		t.Errorf("Снейп.Speech = 0, want > 0 (рядом «сказал»)")
	}

	// Гермиона и Рон слиты по падежным формам в одну запись.
	herm := findCandidate(cands, "Гермиона")
	if herm == nil || herm.Mentions != 2 || !hasAlias(herm, "Гермионе") {
		t.Errorf("Гермиона = %+v, want 2 упоминания с формой «Гермионе»", herm)
	}
}

// TestScanImportsNames — импорт готового списка имён файлом добавляет
// пропущенного героя и подтверждает найденного.
func TestScanImportsNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "names.txt")
	if err := os.WriteFile(path, []byte("# канон\nСнейп\nДамблдор\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	names, err := cast.LoadNames(path)
	if err != nil {
		t.Fatalf("LoadNames: %v", err)
	}
	if len(names) != 2 || names[0] != "Снейп" || names[1] != "Дамблдор" {
		t.Fatalf("LoadNames = %v, want [Снейп Дамблдор]", names)
	}

	chunks := []cast.Chunk{{Ord: 1, Chapter: "Глава 1", Text: "Снейп вошёл в зал."}}
	cands, err := cast.Scan(chunks, cast.ScanOptions{
		Lemmatizer: fakeLemmatizer{forms: scanForms()},
		Profile:    ruProfile(t),
		Names:      names,
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	snape := findCandidate(cands, "Снейп")
	if snape == nil || !snape.Imported {
		t.Errorf("импортированный Снейп должен быть подтверждён: %+v", snape)
	}
	dumb := findCandidate(cands, "Дамблдор")
	if dumb == nil || !dumb.Imported || dumb.Mentions != 0 {
		t.Errorf("Дамблдор должен быть импортирован без упоминаний: %+v", dumb)
	}
}

// TestScanWithoutLemmatizer — без лемматизатора скан деградирует до заглавных
// (quick остаётся рабочим, слот не нужен).
func TestScanWithoutLemmatizer(t *testing.T) {
	chunks := []cast.Chunk{{Ord: 1, Chapter: "Глава 1", Text: "Снейп вошёл. И сказал."}}
	cands, err := cast.Scan(chunks, cast.ScanOptions{Profile: ruProfile(t)})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if findCandidate(cands, "Снейп") == nil {
		t.Errorf("Снейп не найден без лемматизатора: %+v", namesOf(cands))
	}
	if c := findCandidate(cands, "И"); c != nil {
		t.Errorf("служебное «И» не отсеяно без лемматизатора: %+v", c)
	}
}

func namesOf(cands []cast.Candidate) []string {
	out := make([]string, len(cands))
	for i := range cands {
		out[i] = cands[i].Name
	}
	return out
}
