package book

import (
	"database/sql"
	"testing"
)

// TestAssemble — «Проверка» T-07-04: после закрытия главы в book_text версия 1 с
// edited_by IS NULL, собранная из журнала дословно; повторная сборка той же главы
// не создаёт вторую версию 1 (идемпотентность).
func TestAssemble(t *testing.T) {
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

	var version, published int64
	var text string
	var editedBy sql.NullString
	if err := world.QueryRow(
		`SELECT version, text, edited_by, published FROM book_text WHERE chapter_id = ?`,
		ch.ID).Scan(&version, &text, &editedBy, &published); err != nil {
		t.Fatalf("SELECT book_text: %v", err)
	}
	if version != 1 {
		t.Fatalf("version = %d, want 1", version)
	}
	if editedBy.Valid {
		t.Fatalf("edited_by = %q, want NULL (собрано кодом)", editedBy.String)
	}
	if published != 0 {
		t.Fatalf("published = %d, want 0", published)
	}
	want := "Мы легли спать.\nДоброй ночи.\nНочь опустилась на город."
	if text != want {
		t.Fatalf("текст версии 1 = %q, want %q", text, want)
	}

	// Идемпотентность: повторная сборка не создаёт вторую версию 1.
	if err := AssembleChapter(world, ch); err != nil {
		t.Fatalf("повторная AssembleChapter: %v", err)
	}
	var count int
	if err := world.QueryRow(`SELECT COUNT(*) FROM book_text WHERE chapter_id = ?`, ch.ID).Scan(&count); err != nil {
		t.Fatalf("SELECT COUNT(book_text): %v", err)
	}
	if count != 1 {
		t.Fatalf("строк book_text = %d, want 1 (идемпотентность)", count)
	}
}

// TestPublish — «Проверка» T-07-04: журнал сессии (turns) не содержит следов
// правки; версии растут монотонно, edited_by фиксирует автора, published помечает
// действующую издательскую версию; она и дайджест читаются из world(<slug>).db.
func TestPublish(t *testing.T) {
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

	// Журнал сессии (turns) не содержит следов правки.
	var player, reply string
	if err := world.QueryRow(`SELECT player_text, reply_text FROM turns WHERE session_id = ?`, sessionID).Scan(&player, &reply); err != nil {
		t.Fatalf("SELECT turns: %v", err)
	}
	if player != "Мы легли спать." || reply != "Доброй ночи." {
		t.Fatalf("журнал сессии изменён: player=%q reply=%q", player, reply)
	}

	// Версия Редактора (T-05-08): version=2, edited_by='editor', published=0.
	editorText := "Мы легли спать.\nСпокойной ночи.\nНочь опустилась на город."
	if _, err := world.Exec("book_text",
		`INSERT INTO book_text (chapter_id, version, text, edited_by, published) VALUES (?, 2, ?, 'editor', 0)`,
		ch.ID, editorText); err != nil {
		t.Fatalf("INSERT book_text v2: %v", err)
	}
	if _, err := world.Exec("book_chapters",
		`UPDATE book_chapters SET digest = ? WHERE id = ?`, "Глава закрыта сном.", ch.ID); err != nil {
		t.Fatalf("UPDATE book_chapters.digest: %v", err)
	}

	if err := Publish(world, ch.ID, 2); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	var v2Published, v1Published int64
	if err := world.QueryRow(`SELECT published FROM book_text WHERE chapter_id = ? AND version = 2`, ch.ID).Scan(&v2Published); err != nil {
		t.Fatalf("SELECT book_text v2: %v", err)
	}
	if v2Published != 1 {
		t.Fatalf("published версии 2 = %d, want 1", v2Published)
	}
	if err := world.QueryRow(`SELECT published FROM book_text WHERE chapter_id = ? AND version = 1`, ch.ID).Scan(&v1Published); err != nil {
		t.Fatalf("SELECT book_text v1: %v", err)
	}
	if v1Published != 0 {
		t.Fatalf("published версии 1 = %d, want 0", v1Published)
	}

	// Действующая издательская версия и дайджест читаются из world(<slug>).db.
	pub, err := LoadPublished(world, ch.ID)
	if err != nil {
		t.Fatalf("LoadPublished: %v", err)
	}
	if pub.Text != editorText {
		t.Fatalf("текст издательской версии = %q, want %q", pub.Text, editorText)
	}
	if pub.Digest != "Глава закрыта сном." {
		t.Fatalf("дайджест = %q, want %q", pub.Digest, "Глава закрыта сном.")
	}
}
