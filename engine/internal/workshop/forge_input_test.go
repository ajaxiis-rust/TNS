package workshop_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/workshop"
)

func TestForgeInput(t *testing.T) {
	// fb2: титул и главы — в метаданных, тело — в чанках, без отдельного
	// парсера тела.
	fb2, err := workshop.ParseBookInput(filepath.Join("testdata", "sample.fb2"))
	if err != nil {
		t.Fatalf("ParseBookInput(fb2): %v", err)
	}
	if fb2.Meta.Title != "Сумрачный город" {
		t.Errorf("Meta.Title = %q, want %q", fb2.Meta.Title, "Сумрачный город")
	}
	if fb2.Meta.Author != "Иван Петров" {
		t.Errorf("Meta.Author = %q, want %q", fb2.Meta.Author, "Иван Петров")
	}
	if len(fb2.Meta.Chapters) != 2 {
		t.Fatalf("глав в метаданных fb2: %d, want 2", len(fb2.Meta.Chapters))
	}
	wantChapters := []string{"Глава 1. Встреча", "Глава 2. Погоня"}
	for i, want := range wantChapters {
		if fb2.Meta.Chapters[i].Title != want {
			t.Errorf("Meta.Chapters[%d].Title = %q, want %q", i, fb2.Meta.Chapters[i].Title, want)
		}
	}
	if len(fb2.Chunks) != 2 {
		t.Fatalf("чанков fb2: %d, want 2", len(fb2.Chunks))
	}
	if !strings.Contains(fb2.Chunks[0].Text, "Первый абзац первой главы.") {
		t.Errorf("чанк 0 не содержит тело первой главы: %q", fb2.Chunks[0].Text)
	}
	if !strings.Contains(fb2.Chunks[1].Text, "Первый абзац второй главы.") {
		t.Errorf("чанк 1 не содержит тело второй главы: %q", fb2.Chunks[1].Text)
	}

	// txt: тело подаётся в чанкер напрямую и режется по границам глав.
	txt, err := workshop.ParseBookInput(filepath.Join("testdata", "sample.txt"))
	if err != nil {
		t.Fatalf("ParseBookInput(txt): %v", err)
	}
	if len(txt.Chunks) != 2 {
		t.Fatalf("чанков txt: %d, want 2", len(txt.Chunks))
	}
	if txt.Chunks[0].Chapter != "Глава 1. Начало" {
		t.Errorf("Chunks[0].Chapter = %q, want %q", txt.Chunks[0].Chapter, "Глава 1. Начало")
	}
	if !strings.Contains(txt.Chunks[1].Text, "Текст второй главы.") {
		t.Errorf("чанк 1 не содержит тело второй главы: %q", txt.Chunks[1].Text)
	}

	// Незнакомый формат — отказ с причиной.
	unknown := filepath.Join(t.TempDir(), "book.epub")
	if err := os.WriteFile(unknown, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err = workshop.ParseBookInput(unknown)
	if !errors.Is(err, workshop.ErrUnsupportedFormat) {
		t.Fatalf("ошибка незнакомого формата = %v, want ErrUnsupportedFormat", err)
	}
	if !strings.Contains(err.Error(), ".epub") || !strings.Contains(err.Error(), "txt и fb2") {
		t.Fatalf("отказ не содержит причину: %v", err)
	}
}
