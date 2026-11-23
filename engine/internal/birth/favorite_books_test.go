package birth

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"tns65/internal/testkit"
)

// TestFavoriteBooksValidation — поле «любимые книги» необязательно и ограничено
// 200 рунами, как прочие текстовые поля анкеты (T-19-06, шаг 1).
func TestFavoriteBooksValidation(t *testing.T) {
	a := validAncketa()
	a.FavoriteBooks = ""
	if err := Validate(a); err != nil {
		t.Fatalf("анкета без любимых книг отвергнута: %v", err)
	}

	a.FavoriteBooks = strings.Repeat("я", favoriteBooksLimit)
	if err := Validate(a); err != nil {
		t.Fatalf("любимые книги на потолке отвергнуты: %v", err)
	}

	a.FavoriteBooks = strings.Repeat("я", favoriteBooksLimit+1)
	err := Validate(a)
	if err == nil {
		t.Fatal("любимые книги длиннее потолка не отвергнуты")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("ошибка не *ValidationError: %v", err)
	}
	if len(ve.Fields) != 1 || ve.Fields[0].Field != "favorite_books" {
		t.Fatalf("перечень полей: %+v, want ровно одно поле favorite_books", ve.Fields)
	}
	if !strings.Contains(ve.Fields[0].Reason, "200") {
		t.Fatalf("причина: %q, want содержит потолок 200", ve.Fields[0].Reason)
	}
}

// TestFavoriteBooksBirth — поле пишется в world_meta, входит в данные
// первичного анализа мастерской (W-12.4) и имеет непустых потребителей.
func TestFavoriteBooksBirth(t *testing.T) {
	dir := t.TempDir()
	a := validAncketa()
	a.FavoriteBooks = "«Граф Монте-Кристо», «Идиот»"

	res, err := Birth(a, Config{WorldsRoot: dir, BaseDBPath: testBaseDB(t, dir), Embed: testkit.NewFakeEmbed()})
	if err != nil {
		t.Fatalf("Birth с любимыми книгами: %v", err)
	}
	worldDB := openReader(t, res.WorldDB)

	// world_meta хранит значение JSON-строкой, как прочие ключи анкеты.
	var raw string
	if err := worldDB.QueryRow(`SELECT value FROM world_meta WHERE "key" = 'favorite_books'`).Scan(&raw); err != nil {
		t.Fatalf("favorite_books в world_meta: %v", err)
	}
	var books string
	if err := json.Unmarshal([]byte(raw), &books); err != nil {
		t.Fatalf("favorite_books не JSON-строка: %v", err)
	}
	if books != a.FavoriteBooks {
		t.Fatalf("favorite_books = %q, want %q", books, a.FavoriteBooks)
	}

	// Текст книг входит в данные первичного анализа: психотип выводится один раз.
	var payloadRaw string
	if err := worldDB.QueryRow(`SELECT payload_json FROM workshop_queue WHERE kind = 'analysis'`).Scan(&payloadRaw); err != nil {
		t.Fatalf("заказ analysis: %v", err)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
		t.Fatalf("payload заказа analysis не JSON: %v", err)
	}
	if payload["favorite_books"] != books {
		t.Fatalf("payload analysis.favorite_books = %q, want %q", payload["favorite_books"], books)
	}

	// Карта потребителей: ключ с непустыми потребителями; поле геройское, а не
	// замороженное мировое.
	c, err := LoadConsumers()
	if err != nil {
		t.Fatalf("LoadConsumers: %v", err)
	}
	if err := CheckConsumers(c, Fields); err != nil {
		t.Fatalf("CheckConsumers: %v", err)
	}
	if len(c.Fields["favorite_books"]) == 0 {
		t.Fatal("у favorite_books нет потребителей")
	}
	if contains(WorldFrozenFields, "favorite_books") {
		t.Fatal("favorite_books попало в замороженные мировые поля")
	}
}
