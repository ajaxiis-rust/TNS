package workshop_test

import (
	"errors"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/workshop"
)

func TestBookRegistry(t *testing.T) {
	litPath := filepath.Join(t.TempDir(), "literary.db")
	newDB(t, litPath, "literary")
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer lit.Close()

	// 1. Запись реестра хэндлом forge: все поля кругом читаются обратно.
	id, err := workshop.InsertBook(lit, workshop.Book{
		ShelfID:  "hp-1997",
		Name:     "Гарри Поттер и философский камень",
		Lang:     "ru",
		Genres:   "фэнтези,подростковый",
		Rating:   "16+",
		Chapters: 17,
		Status:   "ready",
	})
	if err != nil {
		t.Fatalf("InsertBook: %v", err)
	}
	if id <= 0 {
		t.Fatalf("InsertBook id = %d, want > 0", id)
	}

	got, err := workshop.BookByID(lit, id)
	if err != nil {
		t.Fatalf("BookByID: %v", err)
	}
	if got.ShelfID != "hp-1997" || got.Name != "Гарри Поттер и философский камень" ||
		got.Lang != "ru" || got.Genres != "фэнтези,подростковый" ||
		got.Rating != "16+" || got.Chapters != 17 || got.Status != "ready" {
		t.Errorf("BookByID = %+v", got)
	}
	if got.CreatedAt == "" {
		t.Errorf("BookByID CreatedAt пуст, want заполнен")
	}

	// 2. Минимальные поля обязательны (канон §5.3).
	for name, b := range map[string]workshop.Book{
		"без имени":   {Lang: "ru", Status: "ready"},
		"без языка":   {Name: "t", Status: "ready"},
		"без статуса": {Name: "t", Lang: "ru"},
	} {
		if _, err := workshop.InsertBook(lit, b); err == nil {
			t.Errorf("InsertBook(%s) должен вернуть ошибку", name)
		}
	}

	// 3. Книга игрока (P11 §5.4): создаётся, повторный вызов возвращает тот же id.
	pid, err := workshop.EnsurePlayerBook(lit, "Мир Некроманта", "ru")
	if err != nil {
		t.Fatalf("EnsurePlayerBook: %v", err)
	}
	pid2, err := workshop.EnsurePlayerBook(lit, "Мир Некроманта", "ru")
	if err != nil {
		t.Fatalf("EnsurePlayerBook (повтор): %v", err)
	}
	if pid != pid2 {
		t.Errorf("EnsurePlayerBook id = %d и %d, want один id без дубля", pid, pid2)
	}
	player, err := workshop.BookByID(lit, pid)
	if err != nil {
		t.Fatalf("BookByID(player): %v", err)
	}
	if player.Status != workshop.BookStatusPlayer || player.Name != "Мир Некроманта" {
		t.Errorf("player book = %+v", player)
	}

	// 4. Несуществующая книга — ErrBookNotFound.
	if _, err := workshop.BookByID(lit, 999999); !errors.Is(err, workshop.ErrBookNotFound) {
		t.Errorf("BookByID(999999) = %v, want ErrBookNotFound", err)
	}
}
