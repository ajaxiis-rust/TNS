package deepread_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/deepread"
)

// mapSlot — подставной слот картографа: помнит роль и промпт, отдаёт заданный
// текст или отказ.
type mapSlot struct {
	role   string
	prompt string
	calls  int
	text   string
	err    error
}

func (s *mapSlot) Generate(_ context.Context, role, prompt string) (string, error) {
	s.calls++
	s.role = role
	s.prompt = prompt
	if s.err != nil {
		return "", s.err
	}
	return s.text, nil
}

// insertMapPlace вставляет место книги и отдаёт id строки book_place.
func insertMapPlace(t *testing.T, lit *db.Handle, bookID int64, name, aliases, kind string, first, last, mentions int64) int64 {
	t.Helper()
	res, err := lit.Exec("book_place",
		`INSERT INTO book_place (book_id, name, aliases, kind, first_chunk, last_chunk, mentions) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		bookID, name, aliases, kind, first, last, mentions)
	if err != nil {
		t.Fatalf("INSERT book_place(%s): %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("book_place LastInsertId(%s): %v", name, err)
	}
	return id
}

// insertMapScene вставляет сцену книги; placeID 0 — сцена без места.
func insertMapScene(t *testing.T, lit *db.Handle, bookID int64, ord, from, to int64, placeID int64, marker, participants, tags string) {
	t.Helper()
	var place any
	if placeID != 0 {
		place = placeID
	}
	if _, err := lit.Exec("book_scene",
		`INSERT INTO book_scene (book_id, ord, from_chunk, to_chunk, place_id, time_marker, participants, tags, emotion) VALUES (?, ?, ?, ?, ?, ?, ?, ?, '{}')`,
		bookID, ord, from, to, place, marker, participants, tags); err != nil {
		t.Fatalf("INSERT book_scene(%d): %v", ord, err)
	}
}

// insertMapBook заводит вторую книгу реестра.
func insertMapBook(t *testing.T, lit *db.Handle, name string) int64 {
	t.Helper()
	res, err := lit.Exec("book",
		`INSERT INTO book (shelf_id, name, lang, genres, rating, chapters, status, created_at) VALUES ('p13', ?, 'ru', '', '', 1, 'ready', '2026-01-01T00:00:00Z')`,
		name)
	if err != nil {
		t.Fatalf("INSERT book(%s): %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("book LastInsertId(%s): %v", name, err)
	}
	return id
}

// TestMapRoleByTravelMode — роль картографа по режиму походов (§6.5): реальная
// — Летописец, вымышленная — Картограф вымысла, смешанная — Штурман.
func TestMapRoleByTravelMode(t *testing.T) {
	cases := []struct {
		mode deepread.MapMode
		role string
	}{
		{deepread.MapModeReal, deepread.MapChroniclerRole},
		{deepread.MapModeFiction, deepread.MapFictionRole},
		{deepread.MapModeMixed, deepread.MapNavigatorRole},
	}
	for _, c := range cases {
		role, err := deepread.MapRole(c.mode)
		if err != nil {
			t.Fatalf("MapRole(%q): %v", c.mode, err)
		}
		if role != c.role {
			t.Errorf("MapRole(%q) = %q, want %q", c.mode, role, c.role)
		}
	}
	if _, err := deepread.MapRole("воздушная"); err == nil {
		t.Error("режим вне списка принят, want отказ")
	}
}

// TestLoadMapSource — вход карты читается только из book_place и book_scene:
// места по первому чанку, алиасы, число сцен места, сцены по ord с участниками
// и тегами; строки чужой книги не попадают.
func TestLoadMapSource(t *testing.T) {
	lit, bookID := newTestLiterary(t)
	capital := insertMapPlace(t, lit, bookID, "Москва", `["Москве","Москвой"]`, "city", 1, 12, 7)
	insertMapPlace(t, lit, bookID, "Усадьба", `[]`, "estate", 5, 9, 3)
	insertMapScene(t, lit, bookID, 1, 1, 4, capital, "Спустя три дня", `["Иван"]`, `["action_dramatic"]`)
	insertMapScene(t, lit, bookID, 2, 5, 9, 0, "", `[]`, `[]`)

	other := insertMapBook(t, lit, "Чужая книга")
	otherPlace := insertMapPlace(t, lit, other, "Париж", `[]`, "city", 1, 2, 1)
	insertMapScene(t, lit, other, 1, 1, 2, otherPlace, "утром", `[]`, `[]`)

	src, err := deepread.LoadMapSource(lit, bookID)
	if err != nil {
		t.Fatalf("LoadMapSource: %v", err)
	}
	if len(src.Places) != 2 {
		t.Fatalf("мест = %d, want 2", len(src.Places))
	}
	if src.Places[0].Name != "Москва" || src.Places[1].Name != "Усадьба" {
		t.Fatalf("порядок мест = %q, %q, want Москва, Усадьба", src.Places[0].Name, src.Places[1].Name)
	}
	if len(src.Places[0].Aliases) != 2 || src.Places[0].Aliases[0] != "Москве" {
		t.Errorf("алиасы Москвы = %v", src.Places[0].Aliases)
	}
	if src.Places[0].Scenes != 1 {
		t.Errorf("сцен у Москвы = %d, want 1", src.Places[0].Scenes)
	}
	if src.Places[0].Kind != "city" || src.Places[0].Mentions != 7 {
		t.Errorf("вид/упоминания Москвы = %q/%d", src.Places[0].Kind, src.Places[0].Mentions)
	}
	if len(src.Scenes) != 2 {
		t.Fatalf("сцен = %d, want 2", len(src.Scenes))
	}
	if src.Scenes[0].Ord != 1 || src.Scenes[0].Place != "Москва" || src.Scenes[0].TimeMarker != "Спустя три дня" {
		t.Errorf("сцена 1 = %+v", src.Scenes[0])
	}
	if len(src.Scenes[0].Participants) != 1 || src.Scenes[0].Participants[0] != "Иван" {
		t.Errorf("участники сцены 1 = %v", src.Scenes[0].Participants)
	}
	if len(src.Scenes[0].Tags) != 1 || src.Scenes[0].Tags[0] != "action_dramatic" {
		t.Errorf("теги сцены 1 = %v", src.Scenes[0].Tags)
	}
	if src.Scenes[1].Place != "" || src.Scenes[1].TimeMarker != "" {
		t.Errorf("сцена 2 без места и времени = %+v", src.Scenes[1])
	}
}

// TestBuildBookMap — критерий §12.6: карта строится по выписанным местам и
// временам без повторного чтения прозы, роль — по режиму походов, карта ложится
// в литературную базу и читается обратно.
func TestBuildBookMap(t *testing.T) {
	lit, bookID := newTestLiterary(t)
	capital := insertMapPlace(t, lit, bookID, "Москва", `["Москве"]`, "city", 1, 12, 7)
	insertMapPlace(t, lit, bookID, "Усадьба", `[]`, "estate", 5, 9, 3)
	insertMapScene(t, lit, bookID, 1, 1, 4, capital, "Спустя три дня", `["Иван"]`, `[]`)

	// Проза и соседние таблицы разбора в промпт не попадают: вход прохода —
	// только места и времена.
	res, err := lit.Exec("book_chunk",
		`INSERT INTO book_chunk (book_id, ord, chapter, text_hash) VALUES (?, 1, 'Глава тайная', 'hash-1')`, bookID)
	if err != nil {
		t.Fatalf("INSERT book_chunk: %v", err)
	}
	chunkID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("book_chunk LastInsertId: %v", err)
	}
	if _, err := lit.Exec("book_chunk_tag",
		`INSERT INTO book_chunk_tag (chunk_id, key, score, band, anchors) VALUES (?, 'secret_tag', 0.9, 'confident', '[]')`, chunkID); err != nil {
		t.Fatalf("INSERT book_chunk_tag: %v", err)
	}

	slot := &mapSlot{text: "  Москва — столица; путь на восток.  "}
	bm, err := deepread.BuildBookMap(context.Background(), lit, deepread.MapInput{
		BookID: bookID,
		Mode:   deepread.MapModeFiction,
		Slot:   slot,
		Now:    "2026-11-14T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("BuildBookMap: %v", err)
	}
	if slot.calls != 1 {
		t.Fatalf("вызовов слота = %d, want 1", slot.calls)
	}
	if slot.role != deepread.MapFictionRole {
		t.Errorf("роль = %q, want %q", slot.role, deepread.MapFictionRole)
	}
	for _, want := range []string{
		`"task":"book-map"`, `"name":"Москва"`, `"kind":"city"`,
		`"time_marker":"Спустя три дня"`, `"participants":["Иван"]`,
	} {
		if !strings.Contains(slot.prompt, want) {
			t.Errorf("промпт без %s: %s", want, slot.prompt)
		}
	}
	for _, forbidden := range []string{"Глава тайная", "secret_tag"} {
		if strings.Contains(slot.prompt, forbidden) {
			t.Errorf("промпт содержит прозу/соседнюю таблицу %q: %s", forbidden, slot.prompt)
		}
	}
	var doc struct {
		Task   string              `json:"task"`
		Mode   deepread.MapMode    `json:"mode"`
		Places []deepread.MapPlace `json:"places"`
		Scenes []deepread.MapScene `json:"scenes"`
	}
	if err := json.Unmarshal([]byte(slot.prompt), &doc); err != nil {
		t.Fatalf("промпт не JSON: %v", err)
	}
	if doc.Task != "book-map" || doc.Mode != deepread.MapModeFiction {
		t.Errorf("шапка промпта = %q/%q", doc.Task, doc.Mode)
	}
	if len(doc.Places) != 2 || len(doc.Scenes) != 1 {
		t.Errorf("промпт: мест %d, сцен %d, want 2/1", len(doc.Places), len(doc.Scenes))
	}

	if bm.BookID != bookID || bm.Role != deepread.MapFictionRole || bm.Mode != deepread.MapModeFiction {
		t.Errorf("карта = %+v", bm)
	}
	if bm.Text != "Москва — столица; путь на восток." {
		t.Errorf("текст карты = %q, want обрезанный ответ картографа", bm.Text)
	}
	if bm.BuiltAt != "2026-11-14T12:00:00Z" {
		t.Errorf("BuiltAt = %q", bm.BuiltAt)
	}

	saved, ok, err := deepread.LoadBookMap(lit, bookID)
	if err != nil {
		t.Fatalf("LoadBookMap: %v", err)
	}
	if !ok {
		t.Fatal("карта книги не легла в литературную базу")
	}
	if saved.Text != bm.Text || len(saved.Places) != 2 || len(saved.Scenes) != 1 {
		t.Errorf("сохранённая карта = %+v", saved)
	}
	if n := countRows(t, lit, `SELECT COUNT(*) FROM forge_log WHERE kind = ?`, deepread.KindBookMap); n != 1 {
		t.Errorf("строк forge_log book_map = %d, want 1", n)
	}
}

// TestBuildBookMapEmpty — выписок нет: проход пропускается, слот не вызывается.
func TestBuildBookMapEmpty(t *testing.T) {
	lit, bookID := newTestLiterary(t)
	slot := &mapSlot{text: "не должно вызываться"}
	if _, err := deepread.BuildBookMap(context.Background(), lit, deepread.MapInput{
		BookID: bookID,
		Mode:   deepread.MapModeReal,
		Slot:   slot,
	}); !errors.Is(err, deepread.ErrMapEmpty) {
		t.Fatalf("ошибка = %v, want ErrMapEmpty", err)
	}
	if slot.calls != 0 {
		t.Errorf("слот вызван %d раз при пустых таблицах", slot.calls)
	}
}

// TestBuildBookMapDegradation — отказ слота (нет ключей, все провайдеры
// отказали) и отсутствие слота: карта не пишется, ошибка уходит вызывающему.
func TestBuildBookMapDegradation(t *testing.T) {
	lit, bookID := newTestLiterary(t)
	insertMapPlace(t, lit, bookID, "Москва", `[]`, "city", 1, 2, 1)

	failing := &mapSlot{err: errors.New("providerroute: нет доступного провайдера")}
	if _, err := deepread.BuildBookMap(context.Background(), lit, deepread.MapInput{
		BookID: bookID,
		Mode:   deepread.MapModeReal,
		Slot:   failing,
	}); err == nil {
		t.Fatal("отказ слота не вернул ошибку")
	}
	if failing.calls != 1 {
		t.Errorf("вызовов слота = %d, want 1", failing.calls)
	}
	if bm, ok, err := deepread.LoadBookMap(lit, bookID); err != nil || ok {
		t.Errorf("LoadBookMap после отказа = (%v, %v, %v), want пусто", bm, ok, err)
	}

	if _, err := deepread.BuildBookMap(context.Background(), lit, deepread.MapInput{
		BookID: bookID,
		Mode:   deepread.MapModeReal,
	}); !errors.Is(err, deepread.ErrMapNoSlot) {
		t.Errorf("без слота ошибка = %v, want ErrMapNoSlot", err)
	}
	if _, err := deepread.BuildBookMap(context.Background(), lit, deepread.MapInput{
		BookID: bookID,
		Mode:   "воздушная",
		Slot:   &mapSlot{},
	}); err == nil {
		t.Error("режим вне списка принят, want отказ")
	}
}
