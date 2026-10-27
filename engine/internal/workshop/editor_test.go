package workshop_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// editorLexicon — подставной лемматизатор фикстуры Редактора: имена для проверки
// брака Цензором, остальные слова — обычные вне словаря.
func editorLexicon() map[string]fakeEditLex {
	return map[string]fakeEditLex{
		"Иван":   {lemma: "иван", pos: "NOUN"},
		"Гастон": {lemma: "гастон", pos: "NOUN"},
	}
}

func insertEditorChapter(t *testing.T, h *db.Handle, id int64) {
	t.Helper()
	if _, err := h.Exec("book_chapters",
		`INSERT INTO book_chapters (id, day, title, status, kind) VALUES (?, 1, 'Глава 1', 'closed', 'chapter')`, id); err != nil {
		t.Fatalf("INSERT book_chapters: %v", err)
	}
}

func insertEditorText(t *testing.T, h *db.Handle, chapterID, version int64, text string) {
	t.Helper()
	if _, err := h.Exec("book_text",
		`INSERT INTO book_text (chapter_id, version, text, edited_by, published) VALUES (?, ?, ?, NULL, 0)`, chapterID, version, text); err != nil {
		t.Fatalf("INSERT book_text: %v", err)
	}
}

func editorText(t *testing.T, h *db.Handle, chapterID int64) string {
	t.Helper()
	var s string
	if err := h.QueryRow(`SELECT text FROM book_text WHERE chapter_id = ? ORDER BY version DESC LIMIT 1`, chapterID).Scan(&s); err != nil {
		t.Fatalf("SELECT book_text: %v", err)
	}
	return s
}

func editorDigest(t *testing.T, h *db.Handle, chapterID int64) string {
	t.Helper()
	var s string
	if err := h.QueryRow(`SELECT digest FROM book_chapters WHERE id = ?`, chapterID).Scan(&s); err != nil {
		t.Fatalf("SELECT book_chapters.digest: %v", err)
	}
	return s
}

func editorPublishedAndAuthor(t *testing.T, h *db.Handle, chapterID, version int64) (int, string) {
	t.Helper()
	var published int
	var editedBy string
	if err := h.QueryRow(`SELECT published, edited_by FROM book_text WHERE chapter_id = ? AND version = ?`, chapterID, version).Scan(&published, &editedBy); err != nil {
		t.Fatalf("SELECT book_text published/edited_by: %v", err)
	}
	return published, editedBy
}

// TestEditor — «Проверка» T-05-08: список правок вида «было → стало»; правка вне
// орфографии отбракована Цензором правок и не применена; книга обновлена точечно
// (не перепубликована); краткое содержание записано.
func TestEditor(t *testing.T) {
	workshop.SetLemmatizer(fakeEditLemmatizer{forms: editorLexicon()})
	defer workshop.SetLemmatizer(nil)

	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	newDB(t, worldPath, "world", "shared")
	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()

	insertEditorChapter(t, world, 1)
	const original = "Иван шёл по этому городу и увидел ёлку."
	insertEditorText(t, world, 1, 1, original)

	slot := testkit.NewFakeSlot()
	slot.SetResponse(workshop.EditorRole,
		`{"edits":[{"before":"по этому","after":"поэтому"},{"before":"ёлку.","after":"ёлку…"},{"before":"Иван","after":"Гастон"}]}`)
	slot.SetResponse(workshop.EditorDigestRole,
		`{"summary":"Иван гулял по городу и встретил ёлку."}`)

	res, err := workshop.EditChapter(context.Background(), world, slot, 1)
	if err != nil {
		t.Fatalf("EditChapter: %v", err)
	}

	// Список правок имеет вид «было → стало»: применены две допустимые правки,
	// правка имени отбракована и в список не попала.
	if len(res.Edits) != 2 {
		t.Fatalf("применённых правок: %d, want 2", len(res.Edits))
	}
	for _, e := range res.Edits {
		if e.Before == "" || e.After == "" || e.Before == e.After {
			t.Fatalf("правка не вида «было → стало»: %+v", e)
		}
	}
	if res.Edits[0].Before != "по этому" || res.Edits[0].After != "поэтому" {
		t.Fatalf("правка[0] = %q → %q, want по этому → поэтому", res.Edits[0].Before, res.Edits[0].After)
	}
	if res.Edits[1].Before != "ёлку." || res.Edits[1].After != "ёлку…" {
		t.Fatalf("правка[1] = %q → %q, want ёлку. → ёлку…", res.Edits[1].Before, res.Edits[1].After)
	}

	// Книга обновлена точечно: изменены только вхождения правок, остальное не
	// переписано; правка имени не применена.
	const want = "Иван шёл поэтому городу и увидел ёлку…"
	if got := editorText(t, world, 1); got != want {
		t.Fatalf("текст после редактуры = %q, want %q", got, want)
	}
	if strings.Contains(editorText(t, world, 1), "Гастон") {
		t.Fatal("правка имени применена, want отбракована Цензором правок")
	}

	// Редактура не перепубликует: версия 2 помечена автором editor и published=0.
	if published, author := editorPublishedAndAuthor(t, world, 1, 2); published != 0 || author != "editor" {
		t.Fatalf("версия 2: published=%d edited_by=%q, want published=0 edited_by=editor", published, author)
	}

	// Краткое содержание записано в book_chapters.digest.
	if got := editorDigest(t, world, 1); got != "Иван гулял по городу и встретил ёлку." {
		t.Fatalf("краткое содержание = %q", got)
	}
}

// scriptedSlot — слот с заранее заданной последовательностью ответов на роль
// (проверка политики отказов Редактора).
type scriptedSlot struct {
	replies []string
	calls   int
}

func (s *scriptedSlot) Generate(_ context.Context, _, _ string) (string, error) {
	i := s.calls
	s.calls++
	if i < len(s.replies) {
		return s.replies[i], nil
	}
	return "", nil
}

// TestEditorRetry — мусорный ответ Редактора повторяется не более двух раз
// (00-INDEX): два мусорных ответа и третий валидный — правки применены.
func TestEditorRetry(t *testing.T) {
	workshop.SetLemmatizer(fakeEditLemmatizer{forms: editorLexicon()})
	defer workshop.SetLemmatizer(nil)

	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	newDB(t, worldPath, "world", "shared")
	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()

	insertEditorChapter(t, world, 1)
	insertEditorText(t, world, 1, 1, "по этому городу")

	slot := &scriptedSlot{replies: []string{"мусор", "ещё мусор", `{"edits":[{"before":"по этому","after":"поэтому"}]}`, `{"summary":"кратко."}`}}
	res, err := workshop.EditChapter(context.Background(), world, slot, 1)
	if err != nil {
		t.Fatalf("EditChapter после двух мусорных ответов: %v", err)
	}
	if len(res.Edits) != 1 {
		t.Fatalf("применённых правок: %d, want 1", len(res.Edits))
	}
	if got := editorText(t, world, 1); got != "поэтому городу" {
		t.Fatalf("текст после редактуры = %q, want поэтому городу", got)
	}
	if slot.calls != 4 {
		t.Fatalf("вызовов слота: %d, want 4 (два мусорных + правка + содержание)", slot.calls)
	}
}
