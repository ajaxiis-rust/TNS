package workshop_test

import (
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/lemmarpc"
	"tns65/internal/workshop"
)

// fakeEditLex — разбор одного слова подставным лемматизатором.
type fakeEditLex struct {
	lemma string
	pos   string
	tags  []string
}

// fakeEditLemmatizer — подставной лемматизатор Цензора правок: разбор по карте
// форм; слово вне карты — UNKN (вне словаря).
type fakeEditLemmatizer struct {
	forms map[string]fakeEditLex
}

func (f fakeEditLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error) {
	var tokens []lemmarpc.Token
	for _, w := range strings.Fields(text) {
		lex, ok := f.forms[w]
		if !ok {
			tokens = append(tokens, lemmarpc.Token{Form: w, Lemma: w, Pos: "UNKN"})
			continue
		}
		tokens = append(tokens, lemmarpc.Token{Form: w, Lemma: lex.lemma, Pos: lex.pos, Tags: lex.tags})
	}
	return tokens, nil
}

// editCensorLexicon — карта форм фикстуры Цензора правок.
func editCensorLexicon() map[string]fakeEditLex {
	return map[string]fakeEditLex{
		"Иван":      {lemma: "иван", pos: "NOUN"},
		"Ивн":       {lemma: "ивн", pos: "NOUN"},
		"Гастон":    {lemma: "гастон", pos: "NOUN"},
		"не":        {lemma: "не", pos: "PRCL"},
		"ни":        {lemma: "ни", pos: "PRCL"},
		"хочу":      {lemma: "хотеть", pos: "VERB"},
		"пришёл":    {lemma: "прийти", pos: "VERB"},
		"шёл":       {lemma: "идти", pos: "VERB"},
		"олбанский": {lemma: "олбанский", pos: "UNKN"}, // вне словаря
		"привет":    {lemma: "привет", pos: "NOUN"},    // словарный сосед
	}
}

// insertBookText добавляет обработанную главу с текстом.
func insertBookText(t *testing.T, h *db.Handle, text string) {
	t.Helper()
	if _, err := h.Exec("book_text",
		`INSERT INTO book_text (chapter_id, version, text, published) VALUES (1, 1, ?, 1)`, text); err != nil {
		t.Fatalf("INSERT book_text: %v", err)
	}
}

// insertCharacter добавляет имя персонажа для фактического контроля.
func insertCharacter(t *testing.T, h *db.Handle, name string) {
	t.Helper()
	if _, err := h.Exec("characters",
		`INSERT INTO characters (name, psychotype, estate, card_json) VALUES (?, '', '', '{}')`, name); err != nil {
		t.Fatalf("INSERT characters: %v", err)
	}
}

// TestEditCensor — «Проверка» T-05-15: замена порядка слов и правка несущего
// слова отклоняются; «по этому»→«поэтому» принимается; правка разметки MD
// отклоняется; слово с тремя одинаковыми написаниями не правится и помечается
// кандидатом в лексикон.
func TestEditCensor(t *testing.T) {
	workshop.SetLemmatizer(fakeEditLemmatizer{forms: editCensorLexicon()})
	defer workshop.SetLemmatizer(nil)

	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	newDB(t, worldPath, "world", "shared")
	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()

	insertCharacter(t, world, "Иван")
	insertBookText(t, world, "олдбанский олдбанский олдбанский")
	insertBookText(t, world, "превед превед превед")

	t.Run("замена порядка слов отклонена", func(t *testing.T) {
		ok, _ := workshop.Validate(nil, "быстрый красный автомобиль", "красный быстрый автомобиль")
		if ok {
			t.Fatal("правка с заменой порядка слов прошла, want отклонена")
		}
	})

	t.Run("несущее слово отклонено", func(t *testing.T) {
		cases := []struct {
			before, after string
		}{
			{"Иван пришёл", "Гастон пришёл"},         // имя
			{"Я купил 3 яблока", "Я купил 4 яблока"}, // число
			{"не хочу", "ни хочу"},                   // отрицание
		}
		for _, tc := range cases {
			ok, reason := workshop.Validate(nil, tc.before, tc.after)
			if ok {
				t.Errorf("%q → %q прошла, want отклонена", tc.before, tc.after)
			}
			if reason == "" {
				t.Errorf("%q → %q: причина отказа пуста", tc.before, tc.after)
			}
		}
	})

	t.Run("слияние по этому в поэтому принято", func(t *testing.T) {
		ok, reason := workshop.Validate(nil, "по этому", "поэтому")
		if !ok {
			t.Fatalf("слияние «по этому»→«поэтому» отклонено: %s", reason)
		}
	})

	t.Run("разметка MD неприкосновенна", func(t *testing.T) {
		ok, _ := workshop.Validate(nil, "**смелый** герой", "*смелый* герой")
		if ok {
			t.Fatal("правка внутри разметки MD прошла, want отклонена")
		}
	})

	t.Run("фактический контроль", func(t *testing.T) {
		// «Иван»→«Ивн» — опечатка имени (проходит выравнивание), но «Ивн» не
		// сущность мира — отказ.
		ok, reason := workshop.Validate(world, "Иван шёл", "Ивн шёл")
		if ok {
			t.Fatal("правка с именем вне сущностей мира прошла, want отклонена")
		}
		if !strings.Contains(reason, "фактическое расхождение") {
			t.Fatalf("reason = %q, want фактическое расхождение", reason)
		}
	})

	t.Run("три одинаковых написания — кандидат в лексикон", func(t *testing.T) {
		ok, reason := workshop.Validate(world, "олдбанский", "олбанский")
		if ok {
			t.Fatalf("слово с тремя одинаковыми написаниями правлено, want не правлено")
		}
		if !strings.Contains(reason, "кандидат в лексикон") {
			t.Fatalf("reason = %q, want кандидат в лексикон", reason)
		}
	})

	t.Run("словарный сосед правится с пометкой", func(t *testing.T) {
		ok, reason := workshop.Validate(world, "превед", "привет")
		if !ok {
			t.Fatalf("правка со словарным соседом отклонена: %s", reason)
		}
		if !strings.Contains(reason, "возможно намеренное") {
			t.Fatalf("reason = %q, want возможно намеренное", reason)
		}
	})
}
