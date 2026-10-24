package birth

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func validAncketa() *Ancketa {
	return &Ancketa{
		Slug:        "sumerechnyj-mir",
		Title:       "Тёмный мир",
		Description: "Мир, где свет угас навсегда.",
		Synopsis:    "Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.",
		Prologue:    "Всё началось с тишины.",
		Lang:        "ru",
		TravelMode:  "вымышленная",
		Genres:      []string{"фэнтези", "хоррор"},
		WorldRules:  []string{"магия: редкая и опасная"},
		SocialOrder: "феодализм",
		EconomyMods: []string{"бартер"},
		Magic:       "редкая",
		HeroBirth:   "простолюдин",
		Censorship:  "16+",
	}
}

func TestAncketa(t *testing.T) {
	want := []string{
		"slug", "title", "description", "synopsis", "prologue", "lang",
		"travel_mode", "genres", "world_rules", "social_order",
		"economy_mods", "magic", "hero_birth", "censorship",
	}

	// ровно 14 полей с ключами из шага 1
	got := jsonKeys((*Ancketa)(nil))
	if len(got) != 14 {
		t.Fatalf("полей в анкете: %d, want 14", len(got))
	}
	if !sameSet(got, want) {
		t.Fatalf("ключи анкеты: %v, want %v", got, want)
	}
	if len(Fields) != 14 || !sameSet(Fields, want) {
		t.Fatalf("Fields: %v, want ровно 14 ключей %v", Fields, want)
	}

	// валидная анкета проходит
	if err := Validate(validAncketa()); err != nil {
		t.Fatalf("валидная анкета отвергнута: %v", err)
	}

	// кириллический слаг отвергнут
	a := validAncketa()
	a.Slug = "тёмный-мир"
	if err := Validate(a); err == nil {
		t.Fatal("кириллический слаг не отвергнут")
	}

	// пустой синопсис отвергнут
	a = validAncketa()
	a.Synopsis = ""
	if err := Validate(a); err == nil {
		t.Fatal("пустой синопсис не отвергнут")
	}

	// жанр вне десяти отвергнут
	a = validAncketa()
	a.Genres = []string{"детектив"}
	if err := Validate(a); err == nil {
		t.Fatal("жанр вне канонического списка не отвергнут")
	}

	// режим походов не выводится из жанров
	a = validAncketa()
	a.Genres = []string{"исторический", "мифология"}
	a.TravelMode = ""
	if err := Validate(a); err != nil {
		t.Fatalf("анкета с явно пустым режимом походов отвергнута: %v", err)
	}
	if a.TravelMode != "" {
		t.Fatalf("режим походов выведен из жанров: %q", a.TravelMode)
	}
}

func TestAncketaData(t *testing.T) {
	d, err := LoadData()
	if err != nil {
		t.Fatalf("LoadData: %v", err)
	}
	if len(d.Genres) != 10 {
		t.Fatalf("жанров: %d, want 10", len(d.Genres))
	}
	if len(d.SocialOrders) != 11 {
		t.Fatalf("строёв: %d, want 11", len(d.SocialOrders))
	}
	if len(d.TravelModes) != 3 {
		t.Fatalf("режимов походов: %d, want 3", len(d.TravelModes))
	}
	if len(d.EconomyMods) != 4 {
		t.Fatalf("экономических модификаторов: %d, want 4", len(d.EconomyMods))
	}
	if len(d.Ratings) != 2 {
		t.Fatalf("рейтингов: %d, want 2", len(d.Ratings))
	}
	if !sameSet(d.TravelModes, []string{"реальная", "вымышленная", "смешанная"}) {
		t.Fatalf("режимы походов: %v", d.TravelModes)
	}
}

func TestAncketaWorldRules(t *testing.T) {
	// правило без двоеточия и с пустым описанием отвергаются
	a := validAncketa()
	a.WorldRules = []string{"нет двоеточия"}
	if err := Validate(a); err == nil {
		t.Fatal("правило мира без «название: описание» не отвергнуто")
	}

	a = validAncketa()
	a.WorldRules = []string{"название:"}
	if err := Validate(a); err == nil {
		t.Fatal("правило мира с пустым описанием не отвергнуто")
	}
}

func TestAncketaSynopsisSentences(t *testing.T) {
	a := validAncketa()
	a.Synopsis = "Одно. Два. Три. Четыре."
	if err := Validate(a); err == nil {
		t.Fatal("синопсис из четырёх предложений не отвергнут")
	}
}

func TestAncketaRejectNoFiles(t *testing.T) {
	a := validAncketa()
	a.Synopsis = ""
	var ve *ValidationError
	err := Validate(a)
	if !errors.As(err, &ve) {
		t.Fatalf("ожидался *ValidationError, получен %T: %v", err, err)
	}
	if len(ve.Fields) == 0 {
		t.Fatal("перечень полей с причинами пуст")
	}
	for _, f := range ve.Fields {
		if f.Field == "" || f.Reason == "" {
			t.Fatalf("причина отказа без поля или описания: %+v", f)
		}
	}
}

func jsonKeys(a any) []string {
	typ := reflect.TypeOf(a)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	var keys []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.PkgPath != "" {
			continue
		}
		tag := f.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		keys = append(keys, name)
	}
	return keys
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]int, len(a))
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		if m[s] == 0 {
			return false
		}
		m[s]--
	}
	return true
}
