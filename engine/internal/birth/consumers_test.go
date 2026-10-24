package birth

import (
	"strings"
	"testing"
)

func TestConsumers(t *testing.T) {
	c, err := LoadConsumers()
	if err != nil {
		t.Fatalf("LoadConsumers: %v", err)
	}

	// набор ключей consumers.json совпадает с анкетой; 14 из 14 имеют потребителя
	if err := CheckConsumers(c, Fields); err != nil {
		t.Fatalf("таблица потребителей: %v", err)
	}

	// ровно 5 мировых полей, не переспрашиваемых при переигровке
	wantFrozen := []string{"description", "social_order", "economy_mods", "magic", "censorship"}
	if len(c.WorldFields.Frozen) != 5 || !sameSet(c.WorldFields.Frozen, wantFrozen) {
		t.Fatalf("мировые поля: %v, want ровно 5 %v", c.WorldFields.Frozen, wantFrozen)
	}

	// задаваемые заново при переигровке — синопсис и герой
	wantReasked := []string{"synopsis", "hero_birth"}
	if !sameSet(c.WorldFields.Reasked, wantReasked) {
		t.Fatalf("поля переигровки: %v, want %v", c.WorldFields.Reasked, wantReasked)
	}

	// каждый ключ анкеты имеет минимум одного потребителя
	for _, f := range Fields {
		if len(c.Fields[f]) == 0 {
			t.Fatalf("у ключа %q нет потребителей", f)
		}
	}

	// появление нового ключа анкеты без потребителя — падение
	extended := append(append([]string{}, Fields...), "novyj-kljuch")
	if err := CheckConsumers(c, extended); err == nil {
		t.Fatal("новый ключ анкеты без потребителя не уронил проверку")
	} else if !strings.Contains(err.Error(), "novyj-kljuch") {
		t.Fatalf("ошибка не указывает на новый ключ: %v", err)
	}

	// лишний ключ в таблице потребителей — падение
	extra := Consumers{
		Fields:      map[string][]string{"novyj-kljuch": {"якорь"}},
		WorldFields: c.WorldFields,
	}
	if err := CheckConsumers(extra, Fields); err == nil {
		t.Fatal("ключ в таблице потребителей вне анкеты не уронил проверку")
	}
}
