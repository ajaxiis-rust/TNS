package book

import (
	"strings"
	"testing"

	"tns65/internal/lemmarpc"
	"tns65/internal/workshop"
)

// editsLemmatizer — подставной лемматизатор Цензора правок: слово из карты
// получает лемму/часть речи, слово вне карты — UNKN (вне словаря).
type editsLemmatizer struct {
	forms map[string]lemmarpc.Token
}

func (l editsLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error) {
	var toks []lemmarpc.Token
	for _, w := range strings.Fields(text) {
		if t, ok := l.forms[w]; ok {
			toks = append(toks, lemmarpc.Token{Form: w, Lemma: t.Lemma, Pos: t.Pos, Tags: t.Tags})
			continue
		}
		toks = append(toks, lemmarpc.Token{Form: w, Lemma: w, Pos: "UNKN"})
	}
	return toks, nil
}

// editLexiconForms — карта форм фикстуры Цензора правок для правок игрока.
func editLexiconForms() map[string]lemmarpc.Token {
	return map[string]lemmarpc.Token{
		"Иван":       {Lemma: "иван", Pos: "NOUN"},
		"Гастон":     {Lemma: "гастон", Pos: "NOUN"},
		"пришёл":     {Lemma: "прийти", Pos: "VERB"},
		"превед":     {Lemma: "превед", Pos: "UNKN"}, // вне словаря
		"олдбанский": {Lemma: "олдбанский", Pos: "UNKN"},
		"олбанский":  {Lemma: "олбанский", Pos: "UNKN"},
		"привет":     {Lemma: "привет", Pos: "NOUN"}, // словарный сосед
	}
}

// TestEdits — «Проверка» T-07-05: правка, изменившая порядок слов или несущее
// слово, отклонена и появилась в book_cuts; отклонённых правок в book_text нет;
// слово с тремя одинаковыми написаниями не правлено и записано в world_lexicon
// кандидатом; слово с соседом в пределах двух букв правлено с пометкой.
func TestEdits(t *testing.T) {
	workshop.SetLemmatizer(editsLemmatizer{forms: editLexiconForms()})
	defer workshop.SetLemmatizer(nil)

	world := newBookWorld(t)
	sessionID := insertOpenSession(t, world)
	ch := insertOpenChapter(t, world, sessionID)
	insertJournal(t, world, sessionID)

	if err := ch.Close(world, "2026-10-27T22:00:00Z"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := AssembleChapter(world, ch); err != nil {
		t.Fatalf("AssembleChapter: %v", err)
	}

	// Версия Редактора (T-05-08) и публикация действующей версии.
	editorText := "Иван пришёл. превед олдбанский. Ночь опустилась на город."
	if _, err := world.Exec("book_text",
		`INSERT INTO book_text (chapter_id, version, text, edited_by, published) VALUES (?, 2, ?, 'editor', 0)`,
		ch.ID, editorText); err != nil {
		t.Fatalf("INSERT book_text v2: %v", err)
	}
	if _, err := world.Exec("book_chapters",
		`UPDATE book_chapters SET digest = ? WHERE id = ?`, "Глава закрыта сном.", ch.ID); err != nil {
		t.Fatalf("UPDATE digest: %v", err)
	}
	if err := Publish(world, ch.ID, 2); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Ещё две обработанные записи другой главы доводят «превед» и «олдбанский»
	// до трёх одинаковых написаний в published=1 (CB-06.9).
	if _, err := world.Exec("book_text",
		`INSERT INTO book_text (chapter_id, version, text, published) VALUES (999, 1, 'превед превед олдбанский олдбанский', 1)`); err != nil {
		t.Fatalf("INSERT book_text other: %v", err)
	}

	edits := []PlayerEdit{
		{Before: "быстрый красный автомобиль", After: "красный быстрый автомобиль"}, // порядок слов
		{Before: "Иван пришёл", After: "Гастон пришёл"},                             // несущее слово
		{Before: "олдбанский", After: "олбанский"},                                  // кандидат в лексикон
		{Before: "превед", After: "привет"},                                         // словарный сосед
	}

	res, err := ApplyPlayerEdits(world, ch.ID, edits)
	if err != nil {
		t.Fatalf("ApplyPlayerEdits: %v", err)
	}
	if len(res.Applied) != 1 {
		t.Fatalf("Applied = %d, want 1 (%v)", len(res.Applied), res.Applied)
	}
	if len(res.Rejected) != 3 {
		t.Fatalf("Rejected = %d, want 3 (%v)", len(res.Rejected), res.Rejected)
	}

	// Отклонённые правки появились в book_cuts.
	var cuts []struct {
		cutText string
		reason  string
	}
	rows, err := world.Query(`SELECT cut_text, reason FROM book_cuts ORDER BY id`)
	if err != nil {
		t.Fatalf("SELECT book_cuts: %v", err)
	}
	for rows.Next() {
		var c struct {
			cutText string
			reason  string
		}
		if err := rows.Scan(&c.cutText, &c.reason); err != nil {
			rows.Close()
			t.Fatalf("scan book_cuts: %v", err)
		}
		cuts = append(cuts, c)
	}
	rows.Close()
	if len(cuts) != 3 {
		t.Fatalf("строк book_cuts = %d, want 3", len(cuts))
	}
	wantCuts := map[string]string{
		"красный быстрый автомобиль": "замена или перестановка слова",
		"Гастон пришёл":              "несущее имя изменено",
		"олбанский":                  "кандидат в лексикон мира",
	}
	for _, c := range cuts {
		wantReason, ok := wantCuts[c.cutText]
		if !ok {
			t.Errorf("неожиданная отклонённая правка в book_cuts: %q (%s)", c.cutText, c.reason)
			continue
		}
		if !strings.Contains(c.reason, wantReason) {
			t.Errorf("reason правки %q = %q, want содержит %q", c.cutText, c.reason, wantReason)
		}
	}

	// Отклонённых правок в book_text нет: новая версия игрока содержит только
	// одобренную правку «превед» → «привет», остальной текст не тронут.
	var version int64
	var editedBy string
	var published int64
	var text string
	if err := world.QueryRow(
		`SELECT version, edited_by, published, text FROM book_text WHERE chapter_id = ? AND edited_by = 'player'`,
		ch.ID).Scan(&version, &editedBy, &published, &text); err != nil {
		t.Fatalf("SELECT book_text (player): %v", err)
	}
	if version != 3 {
		t.Fatalf("version = %d, want 3", version)
	}
	if published != 0 {
		t.Fatalf("published = %d, want 0 (правки не публикуют)", published)
	}
	wantText := "Иван пришёл. привет олдбанский. Ночь опустилась на город."
	if text != wantText {
		t.Fatalf("текст версии игрока = %q, want %q", text, wantText)
	}
	for _, rejected := range []string{"красный быстрый автомобиль", "Гастон", "олбанский"} {
		if strings.Contains(text, rejected) {
			t.Errorf("отклонённая правка %q попала в book_text", rejected)
		}
	}

	// Слово с тремя одинаковыми написаниями не правлено и записано кандидатом.
	var wlWord, wlStatus string
	if err := world.QueryRow(`SELECT word, status FROM world_lexicon WHERE word = 'олдбанский'`).
		Scan(&wlWord, &wlStatus); err != nil {
		t.Fatalf("SELECT world_lexicon: %v", err)
	}
	if wlWord != "олдбанский" || wlStatus != "candidate" {
		t.Fatalf("world_lexicon = (%q, %q), want (олдбанский, candidate)", wlWord, wlStatus)
	}

	// Слово со словарным соседом правлено с пометкой в дайджесте.
	var digest string
	if err := world.QueryRow(`SELECT digest FROM book_chapters WHERE id = ?`, ch.ID).Scan(&digest); err != nil {
		t.Fatalf("SELECT digest: %v", err)
	}
	if !strings.Contains(digest, "возможно намеренное") || !strings.Contains(digest, "превед") {
		t.Fatalf("digest = %q, want пометку «возможно намеренное» со словом «превед»", digest)
	}

	// Слово из world_lexicon не правится: повторная правка кандидата отклоняется
	// до Цензора и не создаёт новую версию book_text.
	res2, err := ApplyPlayerEdits(world, ch.ID, []PlayerEdit{{Before: "олдбанский", After: "олбанский"}})
	if err != nil {
		t.Fatalf("ApplyPlayerEdits (лексикон): %v", err)
	}
	if len(res2.Applied) != 0 || len(res2.Rejected) != 1 {
		t.Fatalf("повторная правка кандидата: Applied=%d Rejected=%d, want 0/1", len(res2.Applied), len(res2.Rejected))
	}
	var cutReason string
	if err := world.QueryRow(`SELECT reason FROM book_cuts ORDER BY id DESC LIMIT 1`).Scan(&cutReason); err != nil {
		t.Fatalf("SELECT last book_cut: %v", err)
	}
	if !strings.Contains(cutReason, "слово из лексикона мира") {
		t.Fatalf("reason повторной правки = %q, want содержит «слово из лексикона мира»", cutReason)
	}
	var playerVersions int
	if err := world.QueryRow(`SELECT COUNT(*) FROM book_text WHERE chapter_id = ? AND edited_by = 'player'`, ch.ID).Scan(&playerVersions); err != nil {
		t.Fatalf("COUNT player versions: %v", err)
	}
	if playerVersions != 1 {
		t.Fatalf("версий игрока = %d, want 1 (правка лексикона не создаёт версию)", playerVersions)
	}
}
