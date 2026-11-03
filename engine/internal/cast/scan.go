package cast

import (
	"bufio"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"tns65/internal/lemmarpc"
)

// Lemmatizer — минимальный контракт лемматизатора (T-01-09), как в censor и
// turn: текст → токены с леммой. В проде — lemmarpc.Client, в тестах подменяется.
type Lemmatizer interface {
	Lemmatize(text string) ([]lemmarpc.Token, error)
}

// Chunk — кусок книги для скана: номер куска, глава и текст (P12 §5.9).
type Chunk struct {
	Ord     int64
	Chapter string
	Text    string
}

// Candidate — кандидат в действующие лица карты книги (P12 §5.9, шаг 1):
// имя, слитое по лемме, с картой упоминаний first/last.
type Candidate struct {
	// Name — каноническая форма для записи (лемма с заглавной).
	Name string
	// Lemma — лемма, по которой сливались словоформы.
	Lemma string
	// Aliases — встреченные словоформы (без повторов, в порядке появления).
	Aliases []string
	// FirstChunk — ord первого упоминания; 0 — упоминаний в тексте нет.
	FirstChunk int64
	// LastChunk — ord последнего упоминания.
	LastChunk int64
	// Mentions — общее число упоминаний.
	Mentions int64
	// ByChunk — карта упоминаний: ord куска → счётчик.
	ByChunk map[int64]int64
	// Speech — число упоминаний с атрибуцией реплики рядом (глагол речи).
	Speech int64
	// Imported — имя пришло из готового списка, а не из текста.
	Imported bool
}

// ScanOptions — параметры скана кандидатов.
type ScanOptions struct {
	// Lemmatizer — клиент лемматизации; nil — скан без лемм, по заглавным.
	Lemmatizer Lemmatizer
	// Profile — профиль языка со стоп-листами и маркерами речи.
	Profile LangProfile
	// Names — готовый список имён, импортируемый до скана (необязательно).
	Names []string
	// SpeechWindow — окно токенов вокруг кандидата для атрибуции реплики;
	// 0 — дефолт defaultSpeechWindow.
	SpeechWindow int
}

// defaultSpeechWindow — окно атрибуции реплики по умолчанию (±N токенов).
const defaultSpeechWindow = 3

// acc — накопитель кандидата по лемме во время скана.
type acc struct {
	cand Candidate
	seen map[string]struct{}
}

// Scan находит кандидатов в действующие лица в кусках книги (quick, без слота).
// Скан по заглавным и по леммам: кандидат — слово с заглавной буквы либо
// собственное имя по разбору лемматизатора; словоформы сливаются по лемме.
// Стоп-листы отсеивают служебные слова, топонимы и нарицательные; рядом с
// глаголом речи упоминание помечается атрибуцией. Готовый список имён
// импортируется и подтверждает найденных кандидатов.
func Scan(chunks []Chunk, opts ScanOptions) ([]Candidate, error) {
	set := opts.Profile.build()
	if opts.SpeechWindow <= 0 {
		opts.SpeechWindow = defaultSpeechWindow
	}

	byLemma := map[string]*acc{}
	var order []string
	index := map[string]int{}

	touch := func(lemma string) {
		if _, ok := index[lemma]; !ok {
			index[lemma] = len(order)
			order = append(order, lemma)
		}
	}
	newAcc := func(lemma, name string) *acc {
		return &acc{
			cand: Candidate{Name: name, Lemma: lemma, ByChunk: map[int64]int64{}},
			seen: map[string]struct{}{},
		}
	}
	ensure := func(lemma string) *acc {
		if a, ok := byLemma[lemma]; ok {
			return a
		}
		a := newAcc(lemma, titleWord(lemma))
		byLemma[lemma] = a
		return a
	}
	addAlias := func(a *acc, form string) {
		if form == "" {
			return
		}
		if _, dup := a.seen[form]; dup {
			return
		}
		a.seen[form] = struct{}{}
		a.cand.Aliases = append(a.cand.Aliases, form)
	}

	for _, ch := range chunks {
		tokens, err := scanTokens(opts.Lemmatizer, ch.Text)
		if err != nil {
			return nil, err
		}
		for i, tok := range tokens {
			lemma, ok := candidateLemma(tok, set)
			if !ok {
				continue
			}
			touch(lemma)
			a := ensure(lemma)
			a.cand.Mentions++
			a.cand.ByChunk[ch.Ord]++
			if a.cand.FirstChunk == 0 || ch.Ord < a.cand.FirstChunk {
				a.cand.FirstChunk = ch.Ord
			}
			if ch.Ord > a.cand.LastChunk {
				a.cand.LastChunk = ch.Ord
			}
			addAlias(a, cleanForm(tok.Form))
			if speechNear(tokens, i, opts.SpeechWindow, set.speech) {
				a.cand.Speech++
			}
		}
	}

	// Готовый список имён файлом: добавляет пропущенных кандидатов и
	// подтверждает найденных (известный фандом быстрее автопоиска).
	for _, name := range opts.Names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if a, ok := byLemma[key]; ok {
			a.cand.Imported = true
			continue
		}
		touch(key)
		a := newAcc(key, name)
		byLemma[key] = a
		a.cand.Imported = true
		addAlias(a, name)
	}

	out := make([]Candidate, 0, len(order))
	for _, lemma := range order {
		out = append(out, byLemma[lemma].cand)
	}
	return out, nil
}

// LoadNames читает готовый список имён файлом: по имени в строке, пустые строки
// и комментарии (#) пропускаются. Имена возвращаются в порядке файла.
func LoadNames(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// scanTokens разбирает текст на токены лемматизатором; nil — деградация без
// лемм: токены по словам с леммой-формой в нижнем регистре (скан по заглавным).
func scanTokens(l Lemmatizer, text string) ([]lemmarpc.Token, error) {
	if l != nil {
		return l.Lemmatize(text)
	}
	words := strings.Fields(text)
	toks := make([]lemmarpc.Token, 0, len(words))
	for _, w := range words {
		toks = append(toks, lemmarpc.Token{Form: w, Lemma: strings.ToLower(w), Pos: "NOUN"})
	}
	return toks, nil
}

// candidateLemma возвращает ключ-лемму токена и признак кандидата. Отсеиваются
// служебные слова, топонимы и нарицательные (по лемме); кандидат — слово с
// заглавной буквы либо собственное имя по тегам лемматизатора.
func candidateLemma(tok lemmarpc.Token, set profileSet) (string, bool) {
	lemma := strings.ToLower(cleanForm(tok.Lemma))
	if lemma == "" {
		lemma = strings.ToLower(cleanForm(tok.Form))
	}
	if _, bad := set.function[lemma]; bad {
		return "", false
	}
	if _, bad := set.toponym[lemma]; bad {
		return "", false
	}
	if _, bad := set.common[lemma]; bad {
		return "", false
	}
	if isCapitalized(cleanForm(tok.Form)) || hasNameTag(tok.Tags) {
		return lemma, true
	}
	return "", false
}

// speechNear сообщает, есть ли глагол речи в окне window токенов вокруг i.
func speechNear(tokens []lemmarpc.Token, i, window int, speech map[string]struct{}) bool {
	lo := i - window
	if lo < 0 {
		lo = 0
	}
	hi := i + window
	if hi >= len(tokens) {
		hi = len(tokens) - 1
	}
	for j := lo; j <= hi; j++ {
		if j == i {
			continue
		}
		if _, ok := speech[strings.ToLower(cleanForm(tokens[j].Lemma))]; ok {
			return true
		}
	}
	return false
}

// hasNameTag сообщает, помечено ли слово лемматизатором как имя собственное
// (те же теги OpenCorpora, что в editcensor.hasNameTag).
func hasNameTag(tags []string) bool {
	for _, t := range tags {
		switch t {
		case "Name", "Surn", "Patr", "Geox", "Orgn", "Init":
			return true
		}
	}
	return false
}

// isCapitalized сообщает, начинается ли слово с заглавной буквы.
func isCapitalized(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsUpper(r)
}

// cleanForm снимает с формы ведущую и хвостовую пунктуацию, чтобы алиасы не
// хранили «Снейп,».
func cleanForm(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// titleWord приводит лемму к канонической форме: первая буква заглавная,
// остальные строчные.
func titleWord(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + strings.ToLower(s[size:])
}
