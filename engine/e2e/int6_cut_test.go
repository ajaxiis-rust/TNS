//go:build e2e

package e2e

import (
	"strings"
	"testing"

	"tns65/internal/book"
	"tns65/internal/db"
	"tns65/internal/lemmarpc"
	"tns65/internal/workshop"
)

// int6ChapterText — текст закрытой обработанной главы: содержит перестановку
// «холодный серый туман» (для заведомо не проходящей правки) и опечатку
// «хароший» (для проходящей правки орфографии).
const int6ChapterText = "Он въехал в город под холодный серый туман. Это был хароший вечер, и фонари горели тускло."

// int6Lemmatizer — подставной лемматизатор Цензора правок: каждое слово вне
// словаря (UNKN), поэтому все слова считаются обычными. Правки проверяются
// выравниванием и расстоянием Левенштейна без внешнего бинарника — тест
// детерминирован.
type int6Lemmatizer struct{}

func (int6Lemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error) {
	var out []lemmarpc.Token
	for _, w := range strings.Fields(text) {
		out = append(out, lemmarpc.Token{Form: w, Lemma: w, Pos: "UNKN"})
	}
	return out, nil
}

// TestINT6 — INT-6. Правка и Цензор (T-09-06): правка, не прошедшая Цензора,
// бракуется автоматически, не меняет текст главы и видна во вкладке правок;
// проходящая правка орфографии меняет текст, растит версию и подписывается
// игроком.
func TestINT6(t *testing.T) {
	workshop.SetLemmatizer(int6Lemmatizer{})
	defer workshop.SetLemmatizer(nil)

	bin := buildBinary(t, []string{"e2e"})
	dir := worldDir(t)

	slug := "pravka-i-cenzor"
	ancketa := writeAncketa(t, t.TempDir(), slug,
		"Герой приходит в город без света. Он ищет источник тьмы. Финал решает всё.")
	stdout, stderr, code := runCmd(t, dir, bin, "world", "birth", "--ancketa", ancketa, "--fake-embed", "--fake-slot")
	if code != 0 {
		t.Fatalf("world birth exit=%d, want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	worldDB := worldDBPath(dir, slug)
	h, err := db.OpenWriterAs(worldDB, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("открыть базу мира: %v", err)
	}
	defer h.Close()

	chapterID := seedPublishedChapter(t, h, int6ChapterText)

	report := map[string]any{}
	beforeText := publishedText(t, h, chapterID)

	// Шаги 1–2: правка, заведомо не проходящая Цензора (перестановка слов).
	res, err := book.ApplyPlayerEdits(h, chapterID, []book.PlayerEdit{
		{Before: "холодный серый туман", After: "серый холодный туман"},
	})
	if err != nil {
		t.Fatalf("ApplyPlayerEdits (отклонённая): %v", err)
	}
	if len(res.Rejected) != 1 || len(res.Applied) != 0 {
		t.Fatalf("отклонённая правка: Rejected=%d Applied=%d, want 1/0", len(res.Rejected), len(res.Applied))
	}

	// Текст главы в книге не изменился: действующая версия та же, версии игрока нет.
	report["chapter_text_unchanged"] = publishedText(t, h, chapterID) == beforeText &&
		countPlayerVersions(t, h, chapterID) == 0
	if !report["chapter_text_unchanged"].(bool) {
		t.Fatal("текст главы изменился или появилась версия игрока после отклонённой правки")
	}

	// Правка в статусе rejected и видна во вкладке правок окна «Моя книга».
	win, err := book.OpenWindow(h, book.EntryHistoryTab)
	if err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}
	report["rejected_visible"] = hasRejectedCut(win, "серый холодный туман")
	if !report["rejected_visible"].(bool) {
		t.Fatalf("отклонённая правка не видна во вкладке правок: %+v", win.CensorCuts)
	}

	// Шаг 3: проходящая правка орфографии.
	res2, err := book.ApplyPlayerEdits(h, chapterID, []book.PlayerEdit{
		{Before: "хароший", After: "хороший"},
	})
	if err != nil {
		t.Fatalf("ApplyPlayerEdits (одобренная): %v", err)
	}
	if len(res2.Applied) != 1 || len(res2.Rejected) != 0 {
		t.Fatalf("одобренная правка: Rejected=%d Applied=%d, want 0/1", len(res2.Rejected), len(res2.Applied))
	}

	report["accepted_applied"] = playerVersionApplied(t, h, chapterID, beforeText)
	if !report["accepted_applied"].(bool) {
		t.Fatal("проходящая правка не применилась: нет версии игрока с исправленным текстом")
	}

	writeReport(t, "int6", report)
}

// seedPublishedChapter вставляет обработанную (published) главу с действующей
// версией 1 в базу мира хэндлом core — сид «закрытой главы с текстом».
func seedPublishedChapter(t *testing.T, h *db.Handle, text string) int64 {
	t.Helper()
	res, err := h.Exec("book_chapters",
		`INSERT INTO book_chapters (day, title, status, kind, digest) VALUES (1, 'Глава 1', 'published', 'chapter', 'Глава закрыта сном.')`)
	if err != nil {
		t.Fatalf("INSERT book_chapters: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	if _, err := h.Exec("book_text",
		`INSERT INTO book_text (chapter_id, version, text, edited_by, published) VALUES (?, 1, ?, NULL, 1)`,
		id, text); err != nil {
		t.Fatalf("INSERT book_text: %v", err)
	}
	return id
}

// publishedText — текст действующей издательской версии главы (published=1).
func publishedText(t *testing.T, h *db.Handle, chapterID int64) string {
	t.Helper()
	var text string
	if err := h.QueryRow(
		`SELECT text FROM book_text WHERE chapter_id = ? AND published = 1 ORDER BY version DESC LIMIT 1`,
		chapterID).Scan(&text); err != nil {
		t.Fatalf("SELECT published text: %v", err)
	}
	return text
}

// countPlayerVersions — число версий текста главы, подписанных игроком.
func countPlayerVersions(t *testing.T, h *db.Handle, chapterID int64) int {
	t.Helper()
	var n int
	if err := h.QueryRow(
		`SELECT COUNT(*) FROM book_text WHERE chapter_id = ? AND edited_by = 'player'`,
		chapterID).Scan(&n); err != nil {
		t.Fatalf("COUNT player versions: %v", err)
	}
	return n
}

// hasRejectedCut сообщает, есть ли во вкладке правок отклонённая правка с
// заданным текстом результата.
func hasRejectedCut(win book.Window, after string) bool {
	for _, c := range win.CensorCuts {
		if c.Status == "rejected" && c.Text == after {
			return true
		}
	}
	return false
}

// playerVersionApplied проверяет, что проходящая правка создала версию игрока
// (edited_by='player', published=0, версия выросла до 2) с исправленным текстом.
func playerVersionApplied(t *testing.T, h *db.Handle, chapterID int64, beforeText string) bool {
	t.Helper()
	var version, published int64
	var editedBy, text string
	err := h.QueryRow(
		`SELECT version, edited_by, published, text FROM book_text WHERE chapter_id = ? AND edited_by = 'player' ORDER BY version DESC LIMIT 1`,
		chapterID).Scan(&version, &editedBy, &published, &text)
	if err != nil {
		t.Fatalf("SELECT player version: %v", err)
	}
	want := strings.Replace(beforeText, "хароший", "хороший", 1)
	return version == 2 && editedBy == "player" && published == 0 && text == want
}
