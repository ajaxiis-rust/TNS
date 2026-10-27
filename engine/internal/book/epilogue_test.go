package book

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
)

// insertScene заводит закрытую сцену главы и возвращает её scene_id.
func insertScene(t *testing.T, h *db.Handle, sessionID int64) int64 {
	t.Helper()
	res, err := h.Exec("scenes",
		`INSERT INTO scenes (session_id, day, status, prose) VALUES (?, 1, 'closed', 'Ночь опустилась на город.')`,
		sessionID)
	if err != nil {
		t.Fatalf("INSERT scenes: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("scenes id: %v", err)
	}
	return id
}

// TestEpilogue — «Проверка» T-07-13.
func TestEpilogue(t *testing.T) {
	t.Run("эпилог сохраняется как глава игрока без обработки движком", func(t *testing.T) {
		world := newBookWorld(t)
		const title = "Эпилог"
		const text = "И жили они долго и счастливо."

		ep, err := SaveEpilogue(world, title, text)
		if err != nil {
			t.Fatalf("SaveEpilogue: %v", err)
		}

		var day int64
		var gotTitle, status, kind string
		if err := world.QueryRow(
			`SELECT day, title, status, kind FROM book_chapters WHERE id = ?`, ep.ID).
			Scan(&day, &gotTitle, &status, &kind); err != nil {
			t.Fatalf("SELECT book_chapters: %v", err)
		}
		if day != 0 {
			t.Errorf("day эпилога = %d, want 0 (не назначается)", day)
		}
		if gotTitle != title || status != string(StatusPublished) || kind != string(KindEpilogue) {
			t.Errorf("book_chapters = (%q, %q, %q), want (%q, %q, %q)",
				gotTitle, status, kind, title, StatusPublished, KindEpilogue)
		}

		var gotText, editedBy string
		var published int
		if err := world.QueryRow(
			`SELECT text, edited_by, published FROM book_text WHERE chapter_id = ?`, ep.ID).
			Scan(&gotText, &editedBy, &published); err != nil {
			t.Fatalf("SELECT book_text: %v", err)
		}
		if gotText != text || editedBy != "player" || published != 1 {
			t.Errorf("book_text = (%q, %q, %d), want (%q, player, 1)", gotText, editedBy, published, text)
		}

		// Редактор и Цензор эпилог не обрабатывали: ни версии редактора, ни
		// задачи редактуры в очереди.
		var editorRows int
		if err := world.QueryRow(
			`SELECT COUNT(*) FROM book_text WHERE chapter_id = ? AND edited_by = 'editor'`, ep.ID).
			Scan(&editorRows); err != nil {
			t.Fatalf("COUNT editor rows: %v", err)
		}
		if editorRows != 0 {
			t.Errorf("версий редактора = %d, want 0", editorRows)
		}
		var queueRows int
		if err := world.QueryRow(`SELECT COUNT(*) FROM workshop_queue`).Scan(&queueRows); err != nil {
			t.Fatalf("COUNT workshop_queue: %v", err)
		}
		if queueRows != 0 {
			t.Errorf("задач в очереди = %d, want 0", queueRows)
		}
	})

	t.Run("метка сцены читается из book_peaks по scene_id", func(t *testing.T) {
		world := newBookWorld(t)
		sessionID := insertOpenSession(t, world)
		ch := insertOpenChapter(t, world, sessionID)
		sceneID := insertScene(t, world, sessionID)

		peak, err := MarkPeak(world, ch.ID, sceneID, PeakScene, "поворот", "2026-10-27T22:00:00Z")
		if err != nil {
			t.Fatalf("MarkPeak(scene): %v", err)
		}
		if _, err := MarkPeak(world, ch.ID, 0, PeakChapter, "глава-пик", "2026-10-27T22:00:00Z"); err != nil {
			t.Fatalf("MarkPeak(chapter): %v", err)
		}

		peaks, err := ScenePeaks(world, sceneID)
		if err != nil {
			t.Fatalf("ScenePeaks: %v", err)
		}
		if len(peaks) != 1 {
			t.Fatalf("меток сцены = %d, want 1", len(peaks))
		}
		if peaks[0].ID != peak.ID || peaks[0].SceneID != sceneID || peaks[0].Kind != PeakScene || peaks[0].Note != "поворот" {
			t.Errorf("метка сцены = %+v, want {ID:%d SceneID:%d Kind:%q Note:%q}",
				peaks[0], peak.ID, sceneID, PeakScene, "поворот")
		}
	})

	t.Run("в экспорте книги эпилог — последняя глава", func(t *testing.T) {
		const slug = "alpha-world"
		const synopsis = "Герой ищет дорогу домой."
		const epilogueText = "И жили они долго и счастливо."

		world := newExportWorld(t, slug, synopsis)
		root := t.TempDir()

		seedAndPublishChapter(t, world, insertOpenSession(t, world), 1, "Глава 1")
		seedAndPublishChapter(t, world, insertOpenSession(t, world), 2, "Глава 2")
		if _, err := SaveEpilogue(world, "Эпилог", epilogueText); err != nil {
			t.Fatalf("SaveEpilogue: %v", err)
		}

		bookPath, err := ExportBook(world, root, FormatMarkdown)
		if err != nil {
			t.Fatalf("ExportBook: %v", err)
		}
		book, err := os.ReadFile(bookPath)
		if err != nil {
			t.Fatalf("ReadFile книги: %v", err)
		}
		s := string(book)
		for _, heading := range []string{"# Глава 1", "# Глава 2", "# Эпилог"} {
			if !strings.Contains(s, heading) {
				t.Fatalf("в книге нет заголовка %q: %q", heading, s)
			}
		}
		if !strings.Contains(s, epilogueText) {
			t.Fatalf("в книге нет текста эпилога %q: %q", epilogueText, s)
		}

		iEp := strings.Index(s, "# Эпилог")
		if iEp < 0 {
			t.Fatal("эпилог не найден в книге")
		}
		if strings.Index(s, "# Глава 1") > iEp || strings.Index(s, "# Глава 2") > iEp {
			t.Fatalf("эпилог идёт не последней главой: %q", s)
		}
		if strings.LastIndex(s, "# ") != iEp {
			t.Fatalf("после эпилога есть другой заголовок: %q", s)
		}

		// Экспорт книги оставляет файл на диске по слагу (T-07-12 не тронут).
		wantBookPath := filepath.Join(root, "worlds", slug, "export", slug+"-book.md")
		if bookPath != wantBookPath {
			t.Fatalf("путь книги = %q, want %q", bookPath, wantBookPath)
		}
	})

	t.Run("приоритет оседания сцены с меткой выше", func(t *testing.T) {
		world := newBookWorld(t)
		sessionID := insertOpenSession(t, world)
		ch := insertOpenChapter(t, world, sessionID)
		sceneID := insertScene(t, world, sessionID)

		base, err := SedimentPriority(world, sceneID)
		if err != nil {
			t.Fatalf("SedimentPriority без метки: %v", err)
		}
		if base != peakSedimentBase {
			t.Fatalf("приоритет без метки = %d, want %d", base, peakSedimentBase)
		}

		if _, err := MarkPeak(world, ch.ID, sceneID, PeakScene, "пик", "2026-10-27T22:00:00Z"); err != nil {
			t.Fatalf("MarkPeak: %v", err)
		}
		boosted, err := SedimentPriority(world, sceneID)
		if err != nil {
			t.Fatalf("SedimentPriority с меткой: %v", err)
		}
		if boosted != peakSedimentBase+peakSedimentBoost {
			t.Fatalf("приоритет с меткой = %d, want %d", boosted, peakSedimentBase+peakSedimentBoost)
		}
	})
}
