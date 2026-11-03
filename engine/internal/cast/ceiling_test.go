package cast_test

import (
	"testing"

	"tns65/internal/cast"
)

// maxTestCharacters — потолок карты (T-12-27, §8.1): карта строится по трём
// героям; полная книга с десятками персонажей в тесте — ошибка, а не медленный
// успех.
const maxTestCharacters = 3

// capTestCharacters режет кандидатов до потолка maxTestCharacters и пишет в лог,
// если скан вернул больше героев.
func capTestCharacters(t *testing.T, cands []cast.Candidate) []cast.Candidate {
	t.Helper()
	if len(cands) > maxTestCharacters {
		t.Logf("скан вернул %d героев, режу до потолка %d", len(cands), maxTestCharacters)
		cands = cands[:maxTestCharacters]
	}
	return cands
}

// ceilingForms — словоформы фикстуры потолка: имена и маркеры описаний/ролей.
func ceilingForms() map[string]fakeLemma {
	return map[string]fakeLemma{
		"Мара":     {lemma: "мара", pos: "NOUN"},
		"Ефрем":    {lemma: "ефрем", pos: "NOUN"},
		"Огар":     {lemma: "огар", pos: "NOUN"},
		"высокий":  {lemma: "высокий", pos: "ADJF"},
		"высокой":  {lemma: "высокий", pos: "ADJF"},
		"седой":    {lemma: "седой", pos: "ADJF"},
		"тучный":   {lemma: "тучный", pos: "ADJF"},
		"жестокий": {lemma: "жестокий", pos: "ADJF"},
		"угрожал":  {lemma: "угрожать", pos: "VERB"},
		"вошла":    {lemma: "войти", pos: "VERB"},
		"был":      {lemma: "быть", pos: "VERB"},
		"была":     {lemma: "быть", pos: "VERB"},
		"снова":    {lemma: "снова", pos: "ADVB"},
		"и":        {lemma: "и", pos: "CONJ"},
		"Она":      {lemma: "она", pos: "NPRO"},
	}
}

// TestCastCeilings — «Проверка» T-12-27 (потолок героев): пять кандидатов, карта
// строится по трём; описания и роли считаются только для трёх героев.
func TestCastCeilings(t *testing.T) {
	cands := []cast.Candidate{
		{Name: "Мара", Lemma: "мара"},
		{Name: "Ефрем", Lemma: "ефрем"},
		{Name: "Огар", Lemma: "огар"},
		{Name: "Тим", Lemma: "тим"},
		{Name: "Ника", Lemma: "ника"},
	}
	capped := capTestCharacters(t, cands)
	if len(capped) != maxTestCharacters {
		t.Fatalf("после потолка %d героев, want %d", len(capped), maxTestCharacters)
	}

	dict := loadRuDict(t)
	chunks := []cast.Chunk{
		{Ord: 1, Chapter: "Глава 1", Text: "Мара вошла. Она была высокий, седой. Ефрем был жестокий. Огар был тучный."},
		{Ord: 2, Chapter: "Глава 2", Text: "Ефрем угрожал. Мара была седой и высокой. Огар был тучный снова."},
	}

	got, err := cast.Describe(chunks, capped, dict, cast.DescribeOptions{
		Lemmatizer: fakeLemmatizer{forms: ceilingForms()},
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if len(got) != maxTestCharacters {
		t.Fatalf("описано %d героев, want %d", len(got), maxTestCharacters)
	}

	for _, c := range capped {
		if _, err := cast.ChunkRoles(chunks, c, cast.DefaultRoleDict(), cast.RoleOptions{
			Lemmatizer: fakeLemmatizer{forms: ceilingForms()},
		}); err != nil {
			t.Fatalf("ChunkRoles(%s): %v", c.Name, err)
		}
	}
}
