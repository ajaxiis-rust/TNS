package cast_test

import (
	"fmt"
	"testing"

	"tns65/internal/cast"
)

// TestPromoteTopConfirmed — «Проверка» T-12-25 (промоушен): топ-16 подтверждённых
// героев по частоте упоминаний; кандидаты и сверхлимитные не попадают.
func TestPromoteTopConfirmed(t *testing.T) {
	lit := newLit(t)
	defer lit.Close()
	bookID := insertBook(t, lit, "Гарри Поттер и философский камень")

	insert := func(name string, mentions int64, status string) {
		t.Helper()
		if _, err := cast.InsertCharacter(lit, cast.BookCharacter{
			BookID:   bookID,
			Name:     name,
			Mentions: mentions,
			Status:   status,
		}); err != nil {
			t.Fatalf("InsertCharacter(%s): %v", name, err)
		}
	}
	for i := 1; i <= 20; i++ {
		insert(fmt.Sprintf("Герой %02d", i), int64(i), cast.StatusConfirmed)
	}
	// Кандидат с большим числом упоминаний в промоушен не попадает.
	insert("Случайный кандидат", 1000, cast.StatusCandidate)

	top, err := cast.PromoteTopConfirmed(lit, bookID, 16)
	if err != nil {
		t.Fatalf("PromoteTopConfirmed: %v", err)
	}
	if len(top) != 16 {
		t.Fatalf("промоушен = %d героев, want 16", len(top))
	}
	if top[0].Name != "Герой 20" {
		t.Errorf("первый = %q, want «Герой 20» (чаще всех упоминается)", top[0].Name)
	}
	if top[15].Name != "Герой 05" {
		t.Errorf("последний = %q, want «Герой 05»", top[15].Name)
	}
	for _, c := range top {
		if c.Status != cast.StatusConfirmed {
			t.Errorf("в промоушен попал не подтверждённый: %+v", c)
		}
	}

	// Дефолтный потолок: n<=0 даёт те же 16.
	topDefault, err := cast.PromoteTopConfirmed(lit, bookID, 0)
	if err != nil {
		t.Fatalf("PromoteTopConfirmed(0): %v", err)
	}
	if len(topDefault) != 16 {
		t.Errorf("дефолтный промоушен = %d героев, want 16", len(topDefault))
	}
}

// TestPromoteTopConfirmedEmpty — подтверждённых нет: пустой результат без ошибки
// (ковка без карты не ломает открытие мира).
func TestPromoteTopConfirmedEmpty(t *testing.T) {
	lit := newLit(t)
	defer lit.Close()
	bookID := insertBook(t, lit, "Гарри Поттер и философский камень")

	top, err := cast.PromoteTopConfirmed(lit, bookID, 16)
	if err != nil {
		t.Fatalf("PromoteTopConfirmed(пусто): %v", err)
	}
	if len(top) != 0 {
		t.Errorf("промоушен без подтверждённых = %d героев, want 0", len(top))
	}
}

// TestDescriptionByName — «Проверка» T-12-25 (пустая карточка): описание героя
// по имени, регистронезависимо, кириллица; описание при пустом — внешность.
func TestDescriptionByName(t *testing.T) {
	lit := newLit(t)
	defer lit.Close()
	bookID := insertBook(t, lit, "Гарри Поттер и философский камень")

	if _, err := cast.InsertCharacter(lit, cast.BookCharacter{
		BookID:      bookID,
		Name:        "Снейп",
		Description: "мастер зелий",
		Status:      cast.StatusConfirmed,
	}); err != nil {
		t.Fatalf("InsertCharacter(Снейп): %v", err)
	}
	if _, err := cast.InsertCharacter(lit, cast.BookCharacter{
		BookID:     bookID,
		Name:       "Дамблдор",
		Appearance: "седой старик",
		Status:     cast.StatusConfirmed,
	}); err != nil {
		t.Fatalf("InsertCharacter(Дамблдор): %v", err)
	}

	if d, ok := cast.DescriptionByName(lit, "снейп"); !ok || d != "мастер зелий" {
		t.Errorf("DescriptionByName(снейп) = (%q, %v), want («мастер зелий», true)", d, ok)
	}
	if d, ok := cast.DescriptionByName(lit, "Снейп"); !ok || d != "мастер зелий" {
		t.Errorf("DescriptionByName(Снейп) = (%q, %v), want («мастер зелий», true)", d, ok)
	}
	if d, ok := cast.DescriptionByName(lit, "Дамблдор"); !ok || d != "седой старик" {
		t.Errorf("DescriptionByName(Дамблдор) = (%q, %v), want («седой старик», true)", d, ok)
	}
	if _, ok := cast.DescriptionByName(lit, "Гарри"); ok {
		t.Errorf("DescriptionByName(Гарри) нашёл отсутствующего героя")
	}
}
