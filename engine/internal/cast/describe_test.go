package cast_test

import (
	"reflect"
	"testing"

	"tns65/internal/cast"
)

// loadRuDict — словарь описаний русского пакета из packs/ru/descriptions.json.
func loadRuDict(t *testing.T) *cast.DescribeDict {
	t.Helper()
	d, err := cast.LoadDescriptions("../../packs/ru/descriptions.json")
	if err != nil {
		t.Fatalf("LoadDescriptions: %v", err)
	}
	return d
}

// describeForms — словоформы фикстуры: прилагательные с леммой и частью речи
// ADJF, глаголы — VERB, отрицание «не» — PRCL; имена — собственные.
func describeForms() map[string]fakeLemma {
	return map[string]fakeLemma{
		"Снейп":   {lemma: "снейп", pos: "NOUN", tags: []string{"Surn"}},
		"Гарри":   {lemma: "гарри", pos: "NOUN", tags: []string{"Name"}},
		"высокий": {lemma: "высокий", pos: "ADJF"},
		"Высокий": {lemma: "высокий", pos: "ADJF"},
		"тучный":  {lemma: "тучный", pos: "ADJF"},
		"тучного": {lemma: "тучный", pos: "ADJF"},
		"худой":   {lemma: "худой", pos: "ADJF"},
		"не":      {lemma: "не", pos: "PRCL"},
		"был":     {lemma: "быть", pos: "VERB"},
		"вошёл":   {lemma: "войти", pos: "VERB"},
		"сел":     {lemma: "сесть", pos: "VERB"},
		"курил":   {lemma: "курить", pos: "VERB"},
	}
}

func describeCands(names ...string) []cast.Candidate {
	out := make([]cast.Candidate, len(names))
	for i, n := range names {
		out[i] = cast.Candidate{Name: n, Lemma: n}
	}
	return out
}

func findDescribed(got []cast.Described, name string) *cast.Described {
	for i := range got {
		if got[i].Name == name {
			return &got[i]
		}
	}
	return nil
}

// TestDescribeLoadDict — словарь описаний читается из файла и содержит оси
// «рост» и «телосложение» с ожидаемыми леммами.
func TestDescribeLoadDict(t *testing.T) {
	d := loadRuDict(t)

	height := d.Axes["рост"]
	if len(height.Values["высокий"]) == 0 || len(height.Values["низкий"]) == 0 {
		t.Fatalf("ось «рост» неполная: %+v", height.Values)
	}
	build := d.Axes["телосложение"]
	if len(build.Values["тучный"]) == 0 || len(build.Values["худой"]) == 0 {
		t.Fatalf("ось «телосложение» неполная: %+v", build.Values)
	}
	if !d.Axes["привычки"].Verbs {
		t.Fatalf("ось «привычки» должна принимать глаголы")
	}
}

// TestDescribeWindowExtraction — «Проверка» T-12-23: «высокий, тучный» рядом со
// Снейпом даёт рост и телосложение (порог два попадания).
func TestDescribeWindowExtraction(t *testing.T) {
	dict := loadRuDict(t)
	chunks := []cast.Chunk{{Ord: 1, Chapter: "Глава 1", Text: "Снейп вошёл в зал. Он был высокий, тучный. Высокий тучный человек осмотрел комнату."}}

	got, err := cast.Describe(chunks, describeCands("Снейп"), dict, cast.DescribeOptions{
		Lemmatizer: fakeLemmatizer{forms: describeForms()},
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}

	snape := findDescribed(got, "Снейп")
	if snape == nil {
		t.Fatal("Снейп не описан")
	}
	if snape.Traits["рост"] != "высокий" {
		t.Errorf("рост = %q, want «высокий»; traits: %v", snape.Traits["рост"], snape.Traits)
	}
	if snape.Traits["телосложение"] != "тучный" {
		t.Errorf("телосложение = %q, want «тучный»; traits: %v", snape.Traits["телосложение"], snape.Traits)
	}
	if len(snape.Evidence) < 2 {
		t.Errorf("оснований мало: %d, want >= 2", len(snape.Evidence))
	}
}

// TestDescribeNegation — «не тучный» гасит маркер в окне двух токенов.
func TestDescribeNegation(t *testing.T) {
	dict := loadRuDict(t)
	chunks := []cast.Chunk{{Ord: 1, Chapter: "Глава 1", Text: "Снейп вошёл в зал. Он был не тучный. Снейп снова был не тучный."}}

	got, err := cast.Describe(chunks, describeCands("Снейп"), dict, cast.DescribeOptions{
		Lemmatizer: fakeLemmatizer{forms: describeForms()},
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}

	snape := findDescribed(got, "Снейп")
	if snape == nil {
		t.Fatal("Снейп не описан")
	}
	if v, ok := snape.Traits["телосложение"]; ok {
		t.Errorf("«не тучный» дал телосложение = %q; traits: %v", v, snape.Traits)
	}
}

// TestDescribeNeighborNotAttributed — описание соседнего героя за окном ±2
// предложения не приписывается Снейпу.
func TestDescribeNeighborNotAttributed(t *testing.T) {
	dict := loadRuDict(t)
	text := "Снейп вошёл в зал. Снейп сел. Дальше. Ещё. И ещё. Гарри был высокий. Гарри был высокий и тучный. Конец."
	chunks := []cast.Chunk{{Ord: 1, Chapter: "Глава 1", Text: text}}

	got, err := cast.Describe(chunks, describeCands("Снейп", "Гарри"), dict, cast.DescribeOptions{
		Lemmatizer: fakeLemmatizer{forms: describeForms()},
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}

	snape := findDescribed(got, "Снейп")
	if snape == nil {
		t.Fatal("Снейп не описан")
	}
	if len(snape.Traits) != 0 {
		t.Errorf("описание соседнего героя приписано Снейпу: %v", snape.Traits)
	}

	harry := findDescribed(got, "Гарри")
	if harry == nil {
		t.Fatal("Гарри не описан")
	}
	if harry.Traits["рост"] != "высокий" {
		t.Errorf("рост Гарри = %q, want «высокий»; traits: %v", harry.Traits["рост"], harry.Traits)
	}
}

// TestDescribeDeterministic — повторный прогон не меняет результат.
func TestDescribeDeterministic(t *testing.T) {
	dict := loadRuDict(t)
	chunks := []cast.Chunk{{Ord: 1, Chapter: "Глава 1", Text: "Снейп был высокий. Высокий Снейп вошёл. Он был не тучный, а худой."}}
	opts := cast.DescribeOptions{Lemmatizer: fakeLemmatizer{forms: describeForms()}}

	first, err := cast.Describe(chunks, describeCands("Снейп"), dict, opts)
	if err != nil {
		t.Fatalf("Describe (первый прогон): %v", err)
	}
	second, err := cast.Describe(chunks, describeCands("Снейп"), dict, opts)
	if err != nil {
		t.Fatalf("Describe (второй прогон): %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("повторный прогон изменил результат:\nпервый: %+v\nвторой: %+v", first, second)
	}
}

// TestDescribeHabitVerb — глагольный маркер засчитывается только осью привычек.
func TestDescribeHabitVerb(t *testing.T) {
	dict := loadRuDict(t)
	chunks := []cast.Chunk{{Ord: 1, Chapter: "Глава 1", Text: "Снейп курил трубку. Снейп снова курил у окна."}}

	got, err := cast.Describe(chunks, describeCands("Снейп"), dict, cast.DescribeOptions{
		Lemmatizer: fakeLemmatizer{forms: describeForms()},
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}

	snape := findDescribed(got, "Снейп")
	if snape == nil {
		t.Fatal("Снейп не описан")
	}
	if snape.Traits["привычки"] != "курит" {
		t.Errorf("привычки = %q, want «курит»; traits: %v", snape.Traits["привычки"], snape.Traits)
	}
}
