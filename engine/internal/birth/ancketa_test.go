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
		"economy_mods", "magic", "hero_birth", "censorship", "source_book",
	}

	// ровно 15 полей с ключами из шага 1
	got := jsonKeys((*Ancketa)(nil))
	if len(got) != 15 {
		t.Fatalf("полей в анкете: %d, want 15", len(got))
	}
	if !sameSet(got, want) {
		t.Fatalf("ключи анкеты: %v, want %v", got, want)
	}
	if len(Fields) != 15 || !sameSet(Fields, want) {
		t.Fatalf("Fields: %v, want ровно 15 ключей %v", Fields, want)
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

	// жанр вне тридцати канонических отвергнут
	a = validAncketa()
	a.Genres = []string{"поэзия"}
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
	wantGenres := []string{
		"фэнтези", "тёмное фэнтези", "городское фэнтези",
		"научная фантастика", "космоопера", "киберпанк", "стимпанк",
		"постапокалипсис", "LitRPG",
		"мифология", "исторический", "альтернативная история",
		"хоррор", "мистика", "триллер", "детектив",
		"приключения", "морское приключение", "вестерн",
		"любовный", "семейная сага", "подростковый", "драма",
		"философский", "социальная фантастика", "антиутопия", "сатира", "юмор",
		"фанфик", "современная проза",
	}
	if len(d.Genres) != 30 {
		t.Fatalf("жанров: %d, want 30", len(d.Genres))
	}
	if !reflect.DeepEqual(d.Genres, wantGenres) {
		t.Fatalf("жанры: %v\nwant: %v", d.Genres, wantGenres)
	}
	wantSocialOrders := []string{
		"нет", "феодализм", "демократия", "анархия", "теократия",
		"коммунизм", "капитализм", "социализм", "трайбализм",
		"рабовладение", "меркантилизм", "абсолютная монархия",
		"конституционная монархия", "олигархия", "технократия",
		"корпоратократия", "военная диктатура", "кастовая система",
	}
	if len(d.SocialOrders) != 18 {
		t.Fatalf("строёв: %d, want 18", len(d.SocialOrders))
	}
	if !reflect.DeepEqual(d.SocialOrders, wantSocialOrders) {
		t.Fatalf("строи: %v\nwant: %v", d.SocialOrders, wantSocialOrders)
	}
	if len(d.TravelModes) != 3 {
		t.Fatalf("режимов походов: %d, want 3", len(d.TravelModes))
	}
	wantEconomyMods := []string{
		"золотой стандарт", "серебряная экономика", "бартер",
		"командная экономика", "фиатные деньги", "кредитная экономика",
		"ресурсная экономика", "гильдейская",
	}
	if len(d.EconomyMods) != 8 {
		t.Fatalf("экономических модификаторов: %d, want 8", len(d.EconomyMods))
	}
	if !reflect.DeepEqual(d.EconomyMods, wantEconomyMods) {
		t.Fatalf("экономики: %v\nwant: %v", d.EconomyMods, wantEconomyMods)
	}
	if len(d.Ratings) != 2 {
		t.Fatalf("рейтингов: %d, want 2", len(d.Ratings))
	}
	if !sameSet(d.Langs, []string{"ru", "en", "de", "fr", "es", "ja", "zh"}) {
		t.Fatalf("языки: %v", d.Langs)
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

func TestAncketaSourceBook(t *testing.T) {
	// фанфик без книги-источника отвергается полем source_book
	a := validAncketa()
	a.Genres = []string{"фанфик"}
	a.SourceBook = ""
	if err := Validate(a); err == nil || !hasFieldError(err, "source_book") {
		t.Fatalf("фанфик без книги-источника: ожидалась ошибка поля source_book, получено %v", err)
	}

	// книга-источник без жанра «фанфик» отвергается полем source_book
	a = validAncketa()
	a.SourceBook = "garry-potter"
	if err := Validate(a); err == nil || !hasFieldError(err, "source_book") {
		t.Fatalf("книга-источник без фанфика: ожидалась ошибка поля source_book, получено %v", err)
	}

	// фанфик с книгой-источником проходит
	a = validAncketa()
	a.Genres = []string{"фанфик"}
	a.SourceBook = "garry-potter"
	if err := Validate(a); err != nil {
		t.Fatalf("фанфик с книгой-источником отвергнут: %v", err)
	}
}

func hasFieldError(err error, field string) bool {
	var ve *ValidationError
	if !errors.As(err, &ve) {
		return false
	}
	for _, f := range ve.Fields {
		if f.Field == field {
			return true
		}
	}
	return false
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
