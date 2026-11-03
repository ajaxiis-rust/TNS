package cast_test

import (
	"context"
	"errors"
	"testing"

	"tns65/internal/cast"
)

// roleForms — словоформы фикстуры ролей: глаголы-маркеры ролей с леммой и
// частью речи VERB, отрицание «не» — PRCL; имена — собственные.
func roleForms() map[string]fakeLemma {
	return map[string]fakeLemma{
		"Снейп":   {lemma: "снейп", pos: "NOUN", tags: []string{"Surn"}},
		"Гарри":   {lemma: "гарри", pos: "NOUN", tags: []string{"Name"}},
		"угрожал": {lemma: "угрожать", pos: "VERB"},
		"убил":    {lemma: "убить", pos: "VERB"},
		"спас":    {lemma: "спасти", pos: "VERB"},
		"учил":    {lemma: "учить", pos: "VERB"},
		"не":      {lemma: "не", pos: "PRCL"},
		"вошёл":   {lemma: "войти", pos: "VERB"},
		"сказал":  {lemma: "сказать", pos: "VERB"},
	}
}

func roleOpts() cast.RoleOptions {
	return cast.RoleOptions{Lemmatizer: fakeLemmatizer{forms: roleForms()}}
}

// sneaky — кандидат-фикстура: Снейп, лемма «снейп».
func sneaky() cast.Candidate {
	return cast.Candidate{Name: "Снейп", Lemma: "снейп"}
}

// TestRolesBySegments — «Проверка» T-12-24: синтетика «антагонист 1–2, союзник
// 3»; без слота карта ролей строится, запрос по куску даёт правильную роль.
func TestRolesBySegments(t *testing.T) {
	chunks := []cast.Chunk{
		{Ord: 1, Chapter: "Глава 1", Text: "Снейп угрожал Гарри."},
		{Ord: 2, Chapter: "Глава 2", Text: "Снейп убил сторожа."},
		{Ord: 3, Chapter: "Глава 3", Text: "Снейп спас Гарри."},
	}

	rs, err := cast.ChunkRoles(chunks, sneaky(), cast.DefaultRoleDict(), roleOpts())
	if err != nil {
		t.Fatalf("ChunkRoles: %v", err)
	}
	if len(rs) != 3 {
		t.Fatalf("ChunkRoles = %d кусков, want 3", len(rs))
	}
	if rs[0].Role != cast.RoleAntagonist || rs[1].Role != cast.RoleAntagonist {
		t.Errorf("куски 1–2 = %q/%q, want %q", rs[0].Role, rs[1].Role, cast.RoleAntagonist)
	}
	if rs[2].Role != cast.RoleAlly {
		t.Errorf("кусок 3 = %q, want %q", rs[2].Role, cast.RoleAlly)
	}

	segs := cast.MergeRoles(rs)
	if len(segs) != 2 {
		t.Fatalf("MergeRoles = %d отрезков, want 2: %+v", len(segs), segs)
	}
	if segs[0].Role != string(cast.RoleAntagonist) || segs[0].FromChunk != 1 || segs[0].ToChunk != 2 {
		t.Errorf("отрезок 0 = %+v, want антагонист 1–2", segs[0])
	}
	if segs[1].Role != string(cast.RoleAlly) || segs[1].FromChunk != 3 || segs[1].ToChunk != 3 {
		t.Errorf("отрезок 1 = %+v, want союзник 3–3", segs[1])
	}

	// Запрос по куску: правильная роль на 1–3, пустая вне отрезков.
	if got := cast.RoleAt(segs, 1); got != cast.RoleAntagonist {
		t.Errorf("RoleAt(1) = %q, want %q", got, cast.RoleAntagonist)
	}
	if got := cast.RoleAt(segs, 2); got != cast.RoleAntagonist {
		t.Errorf("RoleAt(2) = %q, want %q", got, cast.RoleAntagonist)
	}
	if got := cast.RoleAt(segs, 3); got != cast.RoleAlly {
		t.Errorf("RoleAt(3) = %q, want %q", got, cast.RoleAlly)
	}
	if got := cast.RoleAt(segs, 4); got != "" {
		t.Errorf("RoleAt(4) = %q, want пусто", got)
	}
}

// TestRolesEmptyRole — пустая роль норма: нет маркера — нет отрезка.
func TestRolesEmptyRole(t *testing.T) {
	chunks := []cast.Chunk{
		{Ord: 1, Chapter: "Глава 1", Text: "Снейп вошёл в зал. Гарри сказал слово."},
	}

	rs, err := cast.ChunkRoles(chunks, sneaky(), cast.DefaultRoleDict(), roleOpts())
	if err != nil {
		t.Fatalf("ChunkRoles: %v", err)
	}
	if rs[0].Role != "" {
		t.Errorf("роль без маркера = %q, want пусто", rs[0].Role)
	}
	if segs := cast.MergeRoles(rs); len(segs) != 0 {
		t.Errorf("MergeRoles без ролей = %+v, want 0 отрезков", segs)
	}
}

// TestRolesNegation — отрицание «не» перед маркером гасит роль.
func TestRolesNegation(t *testing.T) {
	chunks := []cast.Chunk{
		{Ord: 1, Chapter: "Глава 1", Text: "Снейп не угрожал Гарри."},
	}

	rs, err := cast.ChunkRoles(chunks, sneaky(), cast.DefaultRoleDict(), roleOpts())
	if err != nil {
		t.Fatalf("ChunkRoles: %v", err)
	}
	if rs[0].Role != "" {
		t.Errorf("«не угрожал» = %q, want пусто", rs[0].Role)
	}
}

// TestRolesDisputedResolver — спорный случай (ничья маркеров) решает слот; отказ
// слота оставляет маркеры.
func TestRolesDisputedResolver(t *testing.T) {
	// «угрожал» и «спас» в одном куске — ничья антагонист/союзник.
	chunks := []cast.Chunk{
		{Ord: 1, Chapter: "Глава 1", Text: "Снейп угрожал и спас Гарри."},
	}

	// Без слота: первый по алфавиту маркер — антагонист.
	rs, err := cast.ChunkRoles(chunks, sneaky(), cast.DefaultRoleDict(), roleOpts())
	if err != nil {
		t.Fatalf("ChunkRoles: %v", err)
	}
	if rs[0].Role != cast.RoleAntagonist {
		t.Errorf("без слота спор = %q, want %q (первый по алфавиту)", rs[0].Role, cast.RoleAntagonist)
	}

	// Слот решает в пользу союзника.
	rs, err = cast.ChunkRoles(chunks, sneaky(), cast.DefaultRoleDict(), cast.RoleOptions{
		Lemmatizer: fakeLemmatizer{forms: roleForms()},
		Resolver:   pickResolver{role: cast.RoleAlly},
	})
	if err != nil {
		t.Fatalf("ChunkRoles(resolver): %v", err)
	}
	if rs[0].Role != cast.RoleAlly {
		t.Errorf("со слотом спор = %q, want %q", rs[0].Role, cast.RoleAlly)
	}

	// Отказ слота: маркеры остаются (антагонист).
	rs, err = cast.ChunkRoles(chunks, sneaky(), cast.DefaultRoleDict(), cast.RoleOptions{
		Lemmatizer: fakeLemmatizer{forms: roleForms()},
		Resolver:   pickResolver{err: errors.New("слот отказал")},
	})
	if err != nil {
		t.Fatalf("ChunkRoles(отказ): %v", err)
	}
	if rs[0].Role != cast.RoleAntagonist {
		t.Errorf("при отказе слота = %q, want маркеры %q", rs[0].Role, cast.RoleAntagonist)
	}
}

// pickResolver — подставной резолвер: возвращает заданную роль или ошибку.
type pickResolver struct {
	role cast.Role
	err  error
}

func (p pickResolver) ResolveRole(_ context.Context, _, _ string, _ []cast.Role) (cast.Role, error) {
	return p.role, p.err
}

// TestRoleListValid — закрытый список: все роли допустимы, выдуманное имя — нет.
func TestRoleListValid(t *testing.T) {
	for _, r := range cast.RoleList() {
		if !cast.ValidRole(string(r)) {
			t.Errorf("роль %q из списка не признана допустимой", r)
		}
	}
	if cast.ValidRole("бог") {
		t.Errorf("выдуманная роль «бог» признана допустимой")
	}
	if cast.ValidRole("") {
		t.Errorf("пустая строка признана ролью (это отсутствие сигнала)")
	}
}
