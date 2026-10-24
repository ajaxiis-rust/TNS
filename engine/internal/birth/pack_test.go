package birth

import (
	"encoding/json"
	"testing"
)

// forbiddenMysteryKeys — ключи тайны, которых в карточке быть не должно (B-05.4).
var forbiddenMysteryKeys = []string{"mystery", "secret", "тайна", "тайный"}

func TestPack96(t *testing.T) {
	a := validAncketa()

	// Рождение пишет пачку в characters оперативной базы (шаг 5).
	dir := t.TempDir()
	res, err := Birth(a, Config{WorldsRoot: dir})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}
	worldDB := openReader(t, res.WorldDB)

	var n int
	if err := worldDB.QueryRow(`SELECT COUNT(*) FROM characters`).Scan(&n); err != nil {
		t.Fatalf("characters count: %v", err)
	}
	if n != 96 {
		t.Fatalf("карточек в characters: %d, want 96", n)
	}

	// Канонические наборы.
	estates, err := LoadEstates()
	if err != nil {
		t.Fatalf("LoadEstates: %v", err)
	}
	if len(estates) != 6 {
		t.Fatalf("сословий: %d, want 6", len(estates))
	}
	estateWeightMap := map[string]float64{}
	estateSet := map[string]bool{}
	for _, e := range estates {
		estateSet[e.Estate] = true
		estateWeightMap[e.Estate] = estateWeight(e, a.SocialOrder)
	}

	codes := AllCodes()
	codeSet := map[string]bool{}
	for _, c := range codes {
		codeSet[c.String()] = true
	}

	archs, err := LoadArchetypes()
	if err != nil {
		t.Fatalf("LoadArchetypes: %v", err)
	}
	archSet := map[string]bool{}
	for _, v := range archs {
		archSet[v] = true
	}

	voices, err := LoadVoices()
	if err != nil {
		t.Fatalf("LoadVoices: %v", err)
	}
	voiceSet := map[string]bool{}
	for _, v := range voices {
		voiceSet[v] = true
	}

	appearances, err := LoadAppearances()
	if err != nil {
		t.Fatalf("LoadAppearances: %v", err)
	}
	appearanceSet := map[string]bool{}
	for _, v := range appearances {
		appearanceSet[v] = true
	}

	// Полное произведение 16 кодов × 6 сословий — без пропусков и дублей.
	expectedPairs := map[string]bool{}
	for _, c := range codes {
		for _, e := range estates {
			expectedPairs[c.String()+"|"+e.Estate] = true
		}
	}

	rows, err := worldDB.Query(`SELECT psychotype, estate, card_json FROM characters`)
	if err != nil {
		t.Fatalf("characters query: %v", err)
	}
	defer rows.Close()

	pairs := map[string]bool{}
	i := 0
	for rows.Next() {
		i++
		var psychotype, estate, raw string
		if err := rows.Scan(&psychotype, &estate, &raw); err != nil {
			t.Fatalf("карточка %d: %v", i, err)
		}

		key := psychotype + "|" + estate
		if pairs[key] {
			t.Fatalf("дубль пары «психотип — сословие»: %s", key)
		}
		pairs[key] = true
		if !codeSet[psychotype] {
			t.Fatalf("карточка %d: психотип %q вне 16 кодов", i, psychotype)
		}
		if !estateSet[estate] {
			t.Fatalf("карточка %d: сословие %q вне шести", i, estate)
		}

		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatalf("карточка %d: card_json не JSON: %v", i, err)
		}
		for _, k := range forbiddenMysteryKeys {
			if _, ok := m[k]; ok {
				t.Fatalf("карточка %d несёт поле тайны %q (B-05.4)", i, k)
			}
		}
		if !archSet[jsonStr(t, m["archetype"], "archetype", i)] {
			t.Fatalf("карточка %d: архетип %q вне двенадцати", i, jsonStr(t, m["archetype"], "archetype", i))
		}
		if !voiceSet[jsonStr(t, m["voice"], "voice", i)] {
			t.Fatalf("карточка %d: голос %q вне таблицы", i, jsonStr(t, m["voice"], "voice", i))
		}
		if !appearanceSet[jsonStr(t, m["appearance"], "appearance", i)] {
			t.Fatalf("карточка %d: внешность %q вне таблицы", i, jsonStr(t, m["appearance"], "appearance", i))
		}

		var weight float64
		if err := json.Unmarshal(m["weight"], &weight); err != nil {
			t.Fatalf("карточка %d: weight не число: %v", i, err)
		}
		if want := estateWeightMap[estate]; weight != want {
			t.Fatalf("карточка %d: вес %v, want %v (строй %q)", i, weight, want, a.SocialOrder)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("characters rows: %v", err)
	}
	if len(pairs) != 96 {
		t.Fatalf("пар «психотип — сословие»: %d, want 96", len(pairs))
	}
	for k := range expectedPairs {
		if !pairs[k] {
			t.Fatalf("пара %q отсутствует в пачке", k)
		}
	}
}

func TestPackDeterministic(t *testing.T) {
	a := validAncketa()

	first, err := Pack(a)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	second, err := Pack(a)
	if err != nil {
		t.Fatalf("Pack (повтор): %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("число карточек при повторе: %d, want %d", len(second), len(first))
	}
	for i := range first {
		if first[i].Voice != second[i].Voice || first[i].Appearance != second[i].Appearance {
			t.Fatalf("карточка %d: голос/внешность изменились при повторной генерации: %s/%s vs %s/%s",
				i, first[i].Voice, first[i].Appearance, second[i].Voice, second[i].Appearance)
		}
	}
}

// jsonStr читает строковое поле JSON из карточки.
func jsonStr(t *testing.T, raw json.RawMessage, field string, i int) string {
	t.Helper()
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("карточка %d: поле %s не строка: %v", i, field, err)
	}
	return s
}
