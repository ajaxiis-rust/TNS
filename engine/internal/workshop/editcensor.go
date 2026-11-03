// Цензор правок Редактора (T-05-15): выравнивание слов 1:1, несущие слова,
// разметка MD и фактический контроль по world(<slug>).db. Фильтр — чистая
// функция над парой строк и хэндлом мира, в базу не пишет.
package workshop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"tns65/internal/db"
	"tns65/internal/lemmarpc"
)

// Lemmatizer — минимальный контракт клиента лемматизации (T-01-09) для Цензора
// правок: разбор слова на токены с леммой и частью речи. В проде —
// internal/lemmarpc.Client, в тестах подменяется через SetLemmatizer.
type Lemmatizer interface {
	Lemmatize(text string) ([]lemmarpc.Token, error)
}

// editLemMu/curLemmatizer — подменяемый лемматизатор Цензора правок (тестовая
// точка входа); nil — заводской резидент пакета ru.
var (
	editLemMu     sync.RWMutex
	curLemmatizer Lemmatizer
)

// SetLemmatizer подменяет лемматизатора Цензора правок; nil — заводской.
func SetLemmatizer(l Lemmatizer) {
	editLemMu.Lock()
	defer editLemMu.Unlock()
	curLemmatizer = l
}

// editLemmatizer возвращает текущего лемматизатора; nil заменяется заводским.
func editLemmatizer() Lemmatizer {
	editLemMu.RLock()
	defer editLemMu.RUnlock()
	if curLemmatizer != nil {
		return curLemmatizer
	}
	return defaultEditLemmatizer()
}

// defaultEditLemOnce — резидент пакета ru собирается один раз на процесс.
var defaultEditLemOnce = sync.OnceValue(func() Lemmatizer {
	root := editEngineRoot()
	if root == "" {
		return nil
	}
	binary := editPackBinary(root)
	if binary == "" {
		return nil
	}
	return lemmarpc.NewClient(binary, nil)
})

func defaultEditLemmatizer() Lemmatizer {
	return defaultEditLemOnce()
}

// editEngineRoot — корень экземпляра: репозиторий по go.mod, развёрнутая папка
// без go.mod — рабочий каталог (base/, packs/ лежат рядом с бинарником).
func editEngineRoot() string {
	if dir, err := os.Getwd(); err == nil {
		if root := walkUp(dir); root != "" {
			return root
		}
		return dir
	}
	return ""
}

// walkUp поднимается от dir к родителям до каталога с go.mod.
func walkUp(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// editPackBinary возвращает путь бинарника лемматизатора из manifest.json
// пакета ru (lemmatizer.binary). Путь в манифесте считается относительно
// каталога пакета; пустая строка — бинарник не задан.
func editPackBinary(root string) string {
	packDir := filepath.Join(root, "packs", "ru")
	raw, err := os.ReadFile(filepath.Join(packDir, "manifest.json"))
	if err != nil {
		return ""
	}
	var m struct {
		Lemmatizer struct {
			Binary string `json:"binary"`
		} `json:"lemmatizer"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || m.Lemmatizer.Binary == "" {
		return ""
	}
	name := m.Lemmatizer.Binary
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(packDir, filepath.FromSlash(name))
}

// Validate проверяет правку before → after (W-15.6, CB-06.4–9). ok=true —
// правка допустима; ok=false — reason, почему правка бракуется. Правка «по
// этому» → «поэтому» (слияние одной пары соседей) допустима; вставка, удаление
// и перестановка слов запрещены. reason на ok=true при правке с пометкой
// «возможно намеренное» — не пустая строка.
func Validate(world *db.Handle, before, after string) (bool, string) {
	if before == after {
		return true, ""
	}

	bw, bmd := scan(before)
	aw, amd := scan(after)
	if !equalStrings(bmd, amd) {
		return false, "разметка MD изменена"
	}

	lem := editLemmatizer()
	b := analyzeWords(bw, lem)
	a := analyzeWords(aw, lem)

	ok, reason, changed := align(b, a)
	if !ok {
		return false, reason
	}
	if ok, reason := checkFacts(world, b, a); !ok {
		return false, reason
	}
	ok, reason = checkLexicon(world, changed)
	if !ok {
		return false, reason
	}
	return true, reason
}

// wordKind — класс несущего слова: обычное, имя, число, отрицание.
type wordKind uint8

const (
	kindPlain wordKind = iota
	kindName
	kindNumber
	kindNegation
)

// analyzedWord — слово текста с нормализованной формой, классом и признаком
// наличия в словаре лемматизатора.
type analyzedWord struct {
	form  string
	fold  string
	kind  wordKind
	known bool
}

// analyzeWords лемматизирует каждое слово и определяет его класс.
func analyzeWords(words []string, lem Lemmatizer) []analyzedWord {
	out := make([]analyzedWord, len(words))
	for i, w := range words {
		out[i] = analyzeWord(w, lem)
	}
	return out
}

func analyzeWord(form string, lem Lemmatizer) analyzedWord {
	w := analyzedWord{form: form, fold: foldWord(form), kind: kindPlain}

	if allDigits(form) {
		w.kind = kindNumber
		return w
	}

	var tok lemmarpc.Token
	if lem != nil {
		toks, err := lem.Lemmatize(form)
		if err == nil && len(toks) > 0 {
			tok = toks[0]
		}
	}
	w.known = tok.Pos != "" && tok.Pos != "UNKN"

	if strings.EqualFold(tok.Lemma, "не") || strings.EqualFold(form, "не") {
		w.kind = kindNegation
		return w
	}
	if tok.Pos == "NUMR" {
		w.kind = kindNumber
		return w
	}
	if hasUpperFirst(form) && (tok.Pos == "NOUN" || hasNameTag(tok.Tags)) {
		w.kind = kindName
	}
	return w
}

// hasNameTag сообщает, помечено ли слово лемматизатором как имя собственное.
func hasNameTag(tags []string) bool {
	for _, t := range tags {
		switch t {
		case "Name", "Surn", "Patr", "Geox", "Orgn", "Init":
			return true
		}
	}
	return false
}

// allDigits сообщает, состоит ли слово только из цифр.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// hasUpperFirst сообщает, начинается ли слово с заглавной буквы.
func hasUpperFirst(s string) bool {
	for _, r := range s {
		return unicode.IsUpper(r)
	}
	return false
}

// foldWord — нормализация слова для сравнения: нижний регистр, без дефисов и
// комбинируемых знаков ударения (слитно/раздельно/через дефис и ударные гласные
// не считаются изменением состава слова).
func foldWord(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if r == '-' {
			continue
		}
		if r >= 0x0300 && r <= 0x036f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// editPair — пара «было → стало» для проверки кандидатов в лексикон.
type editPair struct {
	from, to analyzedWord
}

// align выравнивает последовательности слов 1:1 (с одним допустимым слиянием
// n:1 или разделением 1:n пары соседей) и возвращает изменённые пары.
func align(b, a []analyzedWord) (ok bool, reason string, changed []editPair) {
	n, m := len(b), len(a)
	if n == m {
		var out []editPair
		for i := 0; i < n; i++ {
			if !compatible(b[i], a[i]) {
				return false, pairReason(b[i], a[i]), nil
			}
			if b[i].fold != a[i].fold {
				out = append(out, editPair{from: b[i], to: a[i]})
			}
		}
		return true, "", out
	}

	if n > m {
		k := n - m + 1 // число слов before, слившихся в одно слово after
		for i := 0; i+k <= n; i++ {
			if prefixOK(b[:i], a[:i]) && mergeOK(b[i:i+k], a[i]) && suffixOK(b[i+k:], a[i+1:]) {
				return true, "", nil
			}
		}
		return false, "количество или порядок слов изменены", nil
	}

	k := m - n + 1 // число слов after, на которое разделилось одно слово before
	for i := 0; i < n; i++ {
		if prefixOK(b[:i], a[:i]) && splitOK(b[i], a[i:i+k]) && suffixOK(b[i+1:], a[i+k:]) {
			return true, "", nil
		}
	}
	return false, "количество или порядок слов изменены", nil
}

// prefixOK проверяет попарную совместимость равных по длине префиксов.
func prefixOK(b, a []analyzedWord) bool {
	if len(b) != len(a) {
		return false
	}
	for i := range b {
		if !compatible(b[i], a[i]) {
			return false
		}
	}
	return true
}

// suffixOK — как prefixOK для хвостов.
func suffixOK(b, a []analyzedWord) bool { return prefixOK(b, a) }

// mergeOK сообщает, что несколько соседних слов before слились в одно слово
// after (слитно или через дефис) без участия несущих слов.
func mergeOK(parts []analyzedWord, single analyzedWord) bool {
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if p.kind != kindPlain {
			return false
		}
	}
	if single.kind != kindPlain {
		return false
	}
	return joinFold(parts) == single.fold
}

// splitOK — разделение одного слова на несколько (1:n), симметрично mergeOK.
func splitOK(single analyzedWord, parts []analyzedWord) bool {
	return mergeOK(parts, single)
}

func joinFold(parts []analyzedWord) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.fold)
	}
	return b.String()
}

// compatible сообщает, что слово before и слово after — одно слово: несущие
// слова не меняются (имя допускает только явную опечатку в пределах двух букв),
// обычные — равны или отличаются опечаткой.
func compatible(x, y analyzedWord) bool {
	switch {
	case x.kind == kindNegation || y.kind == kindNegation:
		return x.kind == kindNegation && y.kind == kindNegation && x.fold == y.fold
	case x.kind == kindNumber || y.kind == kindNumber:
		return x.kind == kindNumber && y.kind == kindNumber && x.fold == y.fold
	case x.kind == kindName || y.kind == kindName:
		return x.kind == kindName && y.kind == kindName &&
			(x.fold == y.fold || levenshtein(x.fold, y.fold) <= 2)
	default:
		return x.fold == y.fold || levenshtein(x.fold, y.fold) <= 2
	}
}

func pairReason(x, y analyzedWord) string {
	switch {
	case x.kind == kindNegation || y.kind == kindNegation:
		return "несущее отрицание изменено"
	case x.kind == kindNumber || y.kind == kindNumber:
		return "несущее число изменено"
	case x.kind == kindName || y.kind == kindName:
		return "несущее имя изменено"
	default:
		return "замена или перестановка слова"
	}
}

// checkFacts — фактический контроль (CB-06.8): имя собственное, появившееся в
// результате правки, должно существовать в world(<slug>).db (characters/places).
func checkFacts(world *db.Handle, b, a []analyzedWord) (bool, string) {
	if world == nil {
		return true, ""
	}
	beforeNames := nameSet(b)
	afterNames := nameSet(a)
	for fold, form := range afterNames {
		if _, same := beforeNames[fold]; same {
			continue
		}
		if !worldHasEntity(world, fold) {
			return false, "фактическое расхождение: " + form
		}
	}
	return true, ""
}

func nameSet(words []analyzedWord) map[string]string {
	m := map[string]string{}
	for _, w := range words {
		if w.kind != kindName {
			continue
		}
		if _, ok := m[w.fold]; !ok {
			m[w.fold] = w.form
		}
	}
	return m
}

// worldHasEntity сообщает, есть ли имя в characters или places мира.
func worldHasEntity(world *db.Handle, fold string) bool {
	for _, table := range []string{"characters", "places"} {
		rows, err := world.Query("SELECT name FROM " + table)
		if err != nil {
			continue
		}
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				continue
			}
			if foldWord(n) == fold {
				rows.Close()
				return true
			}
		}
		rows.Close()
	}
	return false
}

// checkLexicon — правило повтора (CB-06.9): слово, одинаково написанное трижды
// и более в обработанных главах (регистр нечувствителен), — кандидат в лексикон
// мира и не правится; при словарном соседе в пределах двух букв правится с
// пометкой «возможно намеренное».
func checkLexicon(world *db.Handle, pairs []editPair) (bool, string) {
	if world == nil || len(pairs) == 0 {
		return true, ""
	}
	counts, err := processedWordCounts(world)
	if err != nil {
		return true, ""
	}

	var mark string
	for _, p := range pairs {
		if counts[p.from.fold] < 3 {
			continue
		}
		if dictionaryNeighbor(p) {
			if mark == "" {
				mark = "возможно намеренное: " + p.from.form
			}
			continue
		}
		return false, "кандидат в лексикон мира: " + p.from.form
	}
	return true, mark
}

// dictionaryNeighbor сообщает, что исправленная форма — словарный сосед исходной
// в пределах двух букв (вероятная системная опечатка, а не намеренное слово).
func dictionaryNeighbor(p editPair) bool {
	if levenshtein(p.from.fold, p.to.fold) > 2 {
		return false
	}
	return p.to.known
}

// processedWordCounts считает вхождения слов в обработанных главах (published=1).
func processedWordCounts(world *db.Handle) (map[string]int, error) {
	counts := map[string]int{}
	rows, err := world.Query(`SELECT text FROM book_text WHERE published = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		words, _ := scan(text)
		for _, w := range words {
			counts[foldWord(w)]++
		}
	}
	return counts, rows.Err()
}

// mdMarkupChars — токены разметки MD (заголовки, выделения, код, цитаты,
// таблицы), которые сравниваются дословно.
const mdMarkupChars = "#*_`>~=|"

// scan разбирает текст на слова и токены разметки MD. Пунктуация прозы
// отбрасывается — она свободна (CB-06.7).
func scan(s string) (words []string, md []string) {
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		r := runes[i]
		if unicode.IsSpace(r) {
			i++
			continue
		}
		if strings.ContainsRune(mdMarkupChars, r) {
			j := i
			for j < len(runes) && strings.ContainsRune(mdMarkupChars, runes[j]) {
				j++
			}
			md = append(md, string(runes[i:j]))
			i = j
			continue
		}
		if isWordRune(r) {
			j := i
			for j < len(runes) {
				c := runes[j]
				if isWordRune(c) {
					j++
					continue
				}
				if (c == '-' || c == '\'') && j > i && j+1 < len(runes) &&
					isWordRune(runes[j-1]) && isWordRune(runes[j+1]) {
					j++
					continue
				}
				break
			}
			words = append(words, string(runes[i:j]))
			i = j
			continue
		}
		i++
	}
	return words, md
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// levenshtein — расстояние Левенштейна между строками по рунам.
func levenshtein(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}

	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := 0; j <= len(br); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 0
			if ar[i-1] != br[j-1] {
				cost = 1
			}
			cur[j] = minInt(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

func minInt(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
