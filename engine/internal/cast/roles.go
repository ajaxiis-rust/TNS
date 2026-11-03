// Роли действующих лиц по отрезкам кусков (P12 §5.9 п.5). Роль — единственное,
// что требует суждения: Снейп «антагонист» куски 1–6, «союзник» куски 7–17.
// Слот опционален и нужен только для спорных случаев; без слота работают
// словарные маркеры ролей («угрожал», «убил», «учил», «спас»), при отсутствии
// сигнала роль остаётся пустой.
package cast

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Role — каноническое имя роли из закрытого списка (P12 §5.9 п.5).
type Role string

// Закрытый список ролей: выдумывать нельзя (P12 §5.9 п.5). «фон» — роль второго
// плана, отличная от пустой роли (нет сигнала).
const (
	RoleProtagonist Role = "протагонист"
	RoleAntagonist  Role = "антагонист"
	RoleMentor      Role = "наставник"
	RoleAlly        Role = "союзник"
	RoleRival       Role = "соперник"
	RoleTrickster   Role = "трикстер"
	RoleVictim      Role = "жертва"
	RoleAuthority   Role = "власть"
	RoleBackground  Role = "фон"
)

// closedRoles — закрытый список ролей в каноническом порядке.
var closedRoles = []Role{
	RoleProtagonist,
	RoleAntagonist,
	RoleMentor,
	RoleAlly,
	RoleRival,
	RoleTrickster,
	RoleVictim,
	RoleAuthority,
	RoleBackground,
}

// roleMarkers — словарные маркеры ролей (P12 §5.9 п.5): леммы глаголов, ведущие
// к роли. Протагонист и фон маркеров не имеют: протагониста выделяет частотность
// упоминаний (промоушен, T-12-25), фон — отсутствие сигнала, как пустая роль.
var roleMarkers = map[Role][]string{
	RoleAntagonist: {"угрожать", "убить", "напасть", "предать", "погубить", "ненавидеть"},
	RoleMentor:     {"учить", "наставлять", "объяснять", "тренировать", "обучать"},
	RoleAlly:       {"спасти", "помочь", "защитить", "выручить", "поддержать"},
	RoleRival:      {"соперничать", "спорить", "дразнить", "завидовать", "соревноваться"},
	RoleTrickster:  {"обмануть", "разыграть", "насмехаться", "подшутить", "хитрить"},
	RoleVictim:     {"пострадать", "погибнуть", "плакать", "испугаться", "бояться"},
	RoleAuthority:  {"приказать", "повелеть", "править", "управлять", "возглавить"},
}

// RoleList возвращает копию закрытого списка ролей в каноническом порядке.
func RoleList() []Role {
	return append([]Role(nil), closedRoles...)
}

// ValidRole сообщает, входит ли s в закрытый список ролей. Пустая строка — не
// роль, а нормальное отсутствие сигнала.
func ValidRole(s string) bool {
	for _, r := range closedRoles {
		if string(r) == s {
			return true
		}
	}
	return false
}

// RoleDict — словарь ролей: закрытый список и леммы-маркеры (данные, не код).
// Индекс «лемма → роль» строится при первом использовании.
type RoleDict struct {
	Roles   []Role
	Markers map[Role][]string

	byLemma map[string]Role
}

// DefaultRoleDict возвращает встроенный русский словарь ролей.
func DefaultRoleDict() *RoleDict {
	markers := make(map[Role][]string, len(roleMarkers))
	for r, lemmas := range roleMarkers {
		markers[r] = append([]string(nil), lemmas...)
	}
	return &RoleDict{Roles: append([]Role(nil), closedRoles...), Markers: markers}
}

// buildMarkers строит индекс «лемма → роль»; леммы в нижнем регистре, пустые
// записи пропускаются.
func (d *RoleDict) buildMarkers() {
	d.byLemma = map[string]Role{}
	for role, lemmas := range d.Markers {
		for _, l := range lemmas {
			key := strings.ToLower(strings.TrimSpace(l))
			if key == "" {
				continue
			}
			d.byLemma[key] = role
		}
	}
}

// RoleResolver — опциональный разбор спорных случаев слотом (P12 §5.9 п.5):
// словарные маркеры дали ничью между ролями tied, слот решает по имени героя и
// тексту куска. Отказ/пустой выход — маркеры остаются.
type RoleResolver interface {
	ResolveRole(ctx context.Context, name, chunkText string, tied []Role) (Role, error)
}

// RoleOptions — параметры выбора ролей.
type RoleOptions struct {
	// Lemmatizer — клиент лемматизации; nil — деградация по формам без лемм.
	Lemmatizer Lemmatizer
	// SentenceWindow — окно предложений вокруг упоминания (±N); 0 — дефолт.
	SentenceWindow int
	// NegWindow — окно токенов отрицания «не» перед маркером; 0 — дефолт.
	NegWindow int
	// MinHits — порог маркеров на кусок; 0 — один маркер уже сигнал.
	MinHits int
	// Resolver — разбор спорных случаев слотом; nil — слот не подключён, спор
	// решается по маркерам детерминированно.
	Resolver RoleResolver
	// Context — контекст вызова резолвера; nil — context.Background.
	Context context.Context
}

// RoleEvidence — основание выбора роли: сырая форма маркера и цитата.
type RoleEvidence struct {
	Role  Role
	Word  string
	Quote string
}

// ChunkRole — роль героя в одном куске; пустая Role — норма (нет сигнала).
type ChunkRole struct {
	Ord        int64
	Role       Role
	Confidence float64
	Evidence   []RoleEvidence
}

// ChunkRoles выбирает роль героя в каждом куске по словарным маркерам, без
// слота (P12 §5.9 п.5). Вокруг каждого упоминания берётся окно ±SentenceWindow
// предложений, леммы-глаголы сверяются со словарём ролей; отрицание «не» в окне
// NegWindow токенов гасит маркер. Ничья между ролями — спорный случай: решает
// Resolver, при отказе остаётся первый по алфавиту маркер. Результат
// детерминирован.
func ChunkRoles(chunks []Chunk, c Candidate, dict *RoleDict, opts RoleOptions) ([]ChunkRole, error) {
	if dict == nil {
		return nil, fmt.Errorf("cast: словарь ролей не задан")
	}
	if dict.byLemma == nil {
		dict.buildMarkers()
	}
	if opts.SentenceWindow <= 0 {
		opts.SentenceWindow = defaultSentenceWindow
	}
	if opts.NegWindow <= 0 {
		opts.NegWindow = defaultNegWindow
	}
	if opts.MinHits <= 0 {
		opts.MinHits = 1
	}
	if opts.Context == nil {
		opts.Context = context.Background()
	}

	lemma := strings.ToLower(strings.TrimSpace(c.Lemma))
	if lemma == "" {
		lemma = strings.ToLower(strings.TrimSpace(c.Name))
	}

	out := make([]ChunkRole, 0, len(chunks))
	for _, ch := range chunks {
		cr, err := chunkRole(ch, c.Name, lemma, dict, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, cr)
	}
	return out, nil
}

// chunkRole выбирает роль героя в одном куске.
func chunkRole(ch Chunk, name, lemma string, dict *RoleDict, opts RoleOptions) (ChunkRole, error) {
	var sents []descSentence
	for _, s := range splitSentences(ch.Text) {
		toks, err := scanTokens(opts.Lemmatizer, s)
		if err != nil {
			return ChunkRole{}, err
		}
		sents = append(sents, descSentence{text: s, tokens: toks})
	}

	inWindow := make([]bool, len(sents))
	anyMention := false
	for i, s := range sents {
		if mentions(s.tokens, lemma) {
			anyMention = true
			lo := i - opts.SentenceWindow
			if lo < 0 {
				lo = 0
			}
			hi := i + opts.SentenceWindow
			if hi >= len(sents) {
				hi = len(sents) - 1
			}
			for j := lo; j <= hi; j++ {
				inWindow[j] = true
			}
		}
	}

	cr := ChunkRole{Ord: ch.Ord}
	if !anyMention {
		return cr, nil
	}

	counts := map[Role]int{}
	var evidence []RoleEvidence
	for i, s := range sents {
		if !inWindow[i] {
			continue
		}
		for j, tok := range s.tokens {
			role, ok := dict.byLemma[tokenLemma(tok)]
			if !ok {
				continue
			}
			if !markerPOS(tok.Pos, true) {
				continue
			}
			if negatedBefore(s.tokens, j, opts.NegWindow) {
				continue
			}
			counts[role]++
			evidence = append(evidence, RoleEvidence{
				Role:  role,
				Word:  cleanForm(tok.Form),
				Quote: strings.TrimSpace(s.text),
			})
		}
	}

	cr.Role, cr.Confidence = pickRole(counts, name, ch.Text, opts)
	cr.Evidence = sortRoleEvidence(evidence)
	return cr, nil
}

// pickRole сводит счётчики маркеров куска к одной роли. Нет сигнала — пустая
// роль; единственный лидер — его роль; ничья — спорный случай резолвера, при
// отказе остаётся первый по алфавиту маркер.
func pickRole(counts map[Role]int, name, chunkText string, opts RoleOptions) (Role, float64) {
	total := 0
	for _, n := range counts {
		total += n
	}
	if total == 0 {
		return "", 0
	}

	bestN := 0
	var tied []Role
	for r, n := range counts {
		if n > bestN {
			bestN = n
			tied = []Role{r}
		} else if n == bestN {
			tied = append(tied, r)
		}
	}
	if bestN < opts.MinHits {
		return "", 0
	}
	sortRoles(tied)

	conf := float64(bestN) / float64(total)
	if len(tied) == 1 {
		return tied[0], conf
	}

	// Спорный случай: слот решает, отказ — маркеры остаются (первый по алфавиту).
	if opts.Resolver != nil {
		if role, err := opts.Resolver.ResolveRole(opts.Context, name, chunkText, tied); err == nil && ValidRole(string(role)) {
			return role, conf
		}
	}
	return tied[0], conf
}

// sortRoles упорядочивает роли по имени для детерминизма.
func sortRoles(rs []Role) {
	sort.Slice(rs, func(i, j int) bool { return rs[i] < rs[j] })
}

// sortRoleEvidence упорядочивает основания для детерминированного результата.
func sortRoleEvidence(ev []RoleEvidence) []RoleEvidence {
	sort.Slice(ev, func(i, j int) bool {
		if ev[i].Role != ev[j].Role {
			return ev[i].Role < ev[j].Role
		}
		if ev[i].Word != ev[j].Word {
			return ev[i].Word < ev[j].Word
		}
		return ev[i].Quote < ev[j].Quote
	})
	return ev
}

// MergeRoles сливает подряд идущие куски с одинаковой непустой ролью в отрезки
// BookRole (P12 §5.9 п.5). Пустая роль разрывает отрезок; пустые куски в отрезок
// не пишутся. Confidence усредняется по кускам, Evidence склеивается маркерами.
func MergeRoles(rs []ChunkRole) []BookRole {
	var out []BookRole
	var count []int
	for _, r := range rs {
		if r.Role == "" {
			continue
		}
		if n := len(out); n > 0 {
			last := &out[n-1]
			if last.Role == string(r.Role) && last.ToChunk+1 == r.Ord {
				last.ToChunk = r.Ord
				last.Confidence += r.Confidence
				count[n-1]++
				last.Evidence = mergeEvidence(last.Evidence, r.Evidence)
				continue
			}
		}
		out = append(out, BookRole{
			FromChunk:  r.Ord,
			ToChunk:    r.Ord,
			Role:       string(r.Role),
			Confidence: r.Confidence,
			Evidence:   joinEvidence(r.Evidence),
		})
		count = append(count, 1)
	}
	for i := range out {
		out[i].Confidence /= float64(count[i])
	}
	return out
}

// RoleAt возвращает роль отрезка, покрывающего ord; пустая — если отрезка нет.
func RoleAt(segs []BookRole, ord int64) Role {
	for _, s := range segs {
		if ord >= s.FromChunk && ord <= s.ToChunk {
			return Role(s.Role)
		}
	}
	return ""
}

// mergeEvidence дописывает слова-маркеры к склеенному основанию без повторов.
func mergeEvidence(joined string, ev []RoleEvidence) string {
	words := make([]string, 0, len(ev))
	seen := map[string]struct{}{}
	if joined != "" {
		for _, w := range strings.Split(joined, "; ") {
			if w != "" {
				seen[w] = struct{}{}
				words = append(words, w)
			}
		}
	}
	for _, e := range ev {
		if e.Word == "" {
			continue
		}
		if _, dup := seen[e.Word]; dup {
			continue
		}
		seen[e.Word] = struct{}{}
		words = append(words, e.Word)
	}
	return strings.Join(words, "; ")
}

// joinEvidence склеивает слова-маркеры основания в одну строку без повторов.
func joinEvidence(ev []RoleEvidence) string {
	return mergeEvidence("", ev)
}
