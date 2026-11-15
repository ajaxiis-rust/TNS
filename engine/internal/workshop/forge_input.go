package workshop

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	textenc "golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	textunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"tns65/internal/journal"
)

// Format — формат входа ковки (W-14.3, шаг 1): txt и fb2 принимаются, прочие
// форматы конвертируются до ковки, неподдержанный — отказ с причиной.
type Format string

const (
	FormatTXT Format = "txt"
	FormatFB2 Format = "fb2"
)

// ErrUnsupportedFormat — неподдержанный формат входа ковки.
var ErrUnsupportedFormat = errors.New("workshop: неподдержанный формат входа ковки")

// Chapter — заголовок главы из метаданных книги.
type Chapter struct {
	Title string `json:"title"`
}

// Chunk — кусок тела книги, нарезанный чанкером по границам глав (шаг 3).
type Chunk struct {
	Chapter string `json:"chapter"`
	Text    string `json:"text"`
}

// BookMeta — метаданные книги, извлечённые из входа (шаг 2).
type BookMeta struct {
	Title    string    `json:"title"`
	Author   string    `json:"author,omitempty"`
	Chapters []Chapter `json:"chapters,omitempty"`
}

// ParsedBook — результат разбора входа: метаданные книги и очередь чанков
// (шаг 5). Записи в базу на этом шаге нет.
type ParsedBook struct {
	Meta   BookMeta `json:"meta"`
	Chunks []Chunk  `json:"chunks"`
}

// MaxChunkRunes — потолок длины чанка ковки (P11, T-12-11): глава длиннее
// режется на куски, чтобы промпт слота compose и вектор чанка не раздувались.
// Порядок величины — сцены по 2–4 тыс. токенов из W-15.
const MaxChunkRunes = 4000

// ParseBookInput разбирает файл-вход ковки по расширению имени (шаг 1) и
// ограничивает длину чанков (лимит чанка).
func ParseBookInput(path string) (*ParsedBook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("workshop: читать вход ковки %s: %w", path, err)
	}
	p, err := parseBook(path, data)
	if err != nil {
		return nil, err
	}
	p.Chunks = limitChunks(p.Chunks)
	return p, nil
}

// limitChunks режет чанки длиннее MaxChunkRunes на куски, сохраняя заголовок
// главы у каждого куска (P11, T-12-11).
func limitChunks(chunks []Chunk) []Chunk {
	out := make([]Chunk, 0, len(chunks))
	for _, c := range chunks {
		runes := []rune(c.Text)
		if len(runes) <= MaxChunkRunes {
			out = append(out, c)
			continue
		}
		for start := 0; start < len(runes); start += MaxChunkRunes {
			end := start + MaxChunkRunes
			if end > len(runes) {
				end = len(runes)
			}
			out = append(out, Chunk{
				Chapter: c.Chapter,
				Text:    strings.TrimSpace(string(runes[start:end])),
			})
		}
	}
	return out
}

// parseBook разбирает вход по имени файла: txt — напрямую в чанкер, fb2 —
// вставка перед чанкером (титул и главы в метаданные, тело — в чанкер).
func parseBook(name string, data []byte) (*ParsedBook, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt":
		return parseTXT(data), nil
	case ".fb2":
		return parseFB2(data)
	default:
		return nil, unsupportedFormat(name)
	}
}

func unsupportedFormat(name string) error {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		ext = "(без расширения)"
	}
	return fmt.Errorf("%w: %s (принимаются только txt и fb2)", ErrUnsupportedFormat, ext)
}

// parseTXT подаёт тело в чанкер напрямую (шаг 3); метаданных у txt нет.
func parseTXT(data []byte) *ParsedBook {
	return &ParsedBook{Chunks: chunkByChapters(string(data))}
}

// chapterHeadingRe — граница главы в тексте: «Глава N», «Глава N. Заголовок»,
// «Chapter N», необязательно с Markdown-решёткой.
var chapterHeadingRe = regexp.MustCompile(`^\s*(?:#{1,6}\s*)?(?:[Гг]лава|[Cc]hapter)(?:\s+\S.*)?$`)

// chunkByChapters режет тело на куски по границам глав (шаг 3): строка начала
// главы становится заголовком куска, текст до следующей границы — его телом.
func chunkByChapters(body string) []Chunk {
	lines := strings.Split(body, "\n")
	var chunks []Chunk
	var cur *Chunk

	flush := func() {
		if cur == nil {
			return
		}
		cur.Text = strings.TrimSpace(cur.Text)
		chunks = append(chunks, *cur)
		cur = nil
	}

	for _, line := range lines {
		if chapterHeadingRe.MatchString(line) {
			flush()
			cur = &Chunk{Chapter: strings.TrimSpace(line)}
			continue
		}
		if cur == nil {
			if strings.TrimSpace(line) == "" {
				continue
			}
			cur = &Chunk{}
		}
		cur.Text += line + "\n"
	}
	flush()
	return chunks
}

// Кодировки входа ковки (IMP-8): импорт перекодирует сырьё в utf-8, чтобы fb2
// с чужой декларацией и однобайтовые txt читались без потерь. Уже utf-8 вход
// не переписывается.
const (
	encUTF8 = "utf-8"

	methodNone      = "без перекодировки"
	methodBOM       = "BOM"
	methodDecl      = "декларация"
	methodHeuristic = "эвристика"
)

// ErrUnknownCharset — кодировку входа определить не удалось; текст ошибки
// перечисляет кандидатов и их скоры для карточки источника.
var ErrUnknownCharset = errors.New("workshop: кодировка входа не определена")

// Transcode — итог перекодировки входа в utf-8: исходная кодировка, способ её
// определения (декларация/BOM/эвристика) и скор эвристики.
type Transcode struct {
	Source string
	Method string
	Score  float64
}

// singleByteCandidate — кандидат эвристики txt/md: кириллическая однобайтовая
// кодировка, которой пробуют декодировать вход.
type singleByteCandidate struct {
	name  string
	table *charmap.Charmap
}

var singleByteCandidates = []singleByteCandidate{
	{"windows-1251", charmap.Windows1251},
	{"KOI8-R", charmap.KOI8R},
	{"CP866", charmap.CodePage866},
	{"ISO-8859-5", charmap.ISO8859_5},
}

// utf16BOMLE и utf16BOMBE — метки порядка байтов utf-16.
var (
	utf16BOMLE = []byte{0xFF, 0xFE}
	utf16BOMBE = []byte{0xFE, 0xFF}
	utf8BOM    = []byte{0xEF, 0xBB, 0xBF}
)

// heuristicMinScore — порог правдоподобия эвристики: ниже него кандидата нет и
// карточка уходит в ошибку со списком скоров.
const heuristicMinScore = 0.55

// xmlEncodingRe — метка кодировки в XML-декларации (§1 IMP-8).
var xmlEncodingRe = regexp.MustCompile(`(?i)<\?xml[^>]*?encoding\s*=\s*["']([A-Za-z0-9._-]+)["']`)

// xmlEncodingAttrRe — атрибут encoding внутри XML-декларации.
var xmlEncodingAttrRe = regexp.MustCompile(`(?i)encoding\s*=\s*["'][A-Za-z0-9._-]+["']`)

// NormalizeBookFile перекодирует файл-вход ковки в utf-8 на месте (IMP-8):
// fb2 — по декларации XML или BOM, txt/md — по BOM или эвристике среди
// кириллических однобайтовых кодировок. Ошибка — причина для карточки.
func NormalizeBookFile(path string) (Transcode, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Transcode{}, fmt.Errorf("workshop: читать вход ковки %s: %w", path, err)
	}
	out, tc, err := toUTF8(raw)
	if err != nil {
		return Transcode{}, err
	}
	if tc.Method == methodNone {
		return tc, nil
	}
	if err := writeBookUTF8(path, out); err != nil {
		return Transcode{}, err
	}
	return tc, nil
}

// writeBookUTF8 перезаписывает вход ковки атомарно: временный файл рядом и
// rename, чтобы падение не оставило половину книги.
func writeBookUTF8(path string, data []byte) error {
	tmp := path + ".utf8.tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("workshop: писать utf-8 вход ковки %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("workshop: заменить вход ковки %s: %w", path, err)
	}
	return nil
}

// toUTF8 определяет кодировку входа и возвращает байты utf-8 вместе с итогом
// перекодировки. метод без перекодировки — вход уже utf-8.
func toUTF8(raw []byte) ([]byte, Transcode, error) {
	switch {
	case bytes.HasPrefix(raw, utf8BOM):
		return withDeclUTF8(raw[len(utf8BOM):]), Transcode{Source: encUTF8, Method: methodBOM}, nil
	case bytes.HasPrefix(raw, utf16BOMLE):
		out, err := decodeWith(textunicode.UTF16(textunicode.LittleEndian, textunicode.IgnoreBOM), raw[len(utf16BOMLE):])
		if err != nil {
			return nil, Transcode{}, err
		}
		return withDeclUTF8(out), Transcode{Source: "utf-16le", Method: methodBOM}, nil
	case bytes.HasPrefix(raw, utf16BOMBE):
		out, err := decodeWith(textunicode.UTF16(textunicode.BigEndian, textunicode.IgnoreBOM), raw[len(utf16BOMBE):])
		if err != nil {
			return nil, Transcode{}, err
		}
		return withDeclUTF8(out), Transcode{Source: "utf-16be", Method: methodBOM}, nil
	}

	if label, ok := declaredEncoding(raw); ok {
		low := strings.ToLower(strings.TrimSpace(label))
		switch {
		case low == encUTF8 || low == "utf8":
			// Верные байты не трогаем; ложь декларации ловит parseFB2.
			if utf8.Valid(raw) {
				return raw, Transcode{Source: encUTF8, Method: methodNone}, nil
			}
		case strings.HasPrefix(low, "utf-16") || low == "utf16":
			out, err := decodeWith(utf16ByName(low), raw)
			if err != nil {
				return nil, Transcode{}, fmt.Errorf("workshop: вход ковки: кодировка %q: %w", label, err)
			}
			return withDeclUTF8(out), Transcode{Source: low, Method: methodDecl}, nil
		default:
			table, known := charmapByLabel(low)
			if !known {
				return nil, Transcode{}, fmt.Errorf("workshop: вход ковки: кодировка %q не поддержана", label)
			}
			out, err := decodeWith(table, raw)
			if err != nil {
				return nil, Transcode{}, fmt.Errorf("workshop: вход ковки: кодировка %q: %w", label, err)
			}
			return withDeclUTF8(out), Transcode{Source: canonicalCharsetName(low), Method: methodDecl}, nil
		}
	}

	if utf8.Valid(raw) {
		return raw, Transcode{Source: encUTF8, Method: methodNone}, nil
	}
	out, tc, err := heuristicDecode(raw)
	if err != nil {
		return nil, Transcode{}, err
	}
	return withDeclUTF8(out), tc, nil
}

// heuristicDecode выбирает кодировку txt/md по скору правдоподобия текста:
// кандидаты — кириллические однобайтовые кодировки. Не выбрался никто —
// ошибка со списком кандидатов и скоров (§1 IMP-8).
func heuristicDecode(raw []byte) ([]byte, Transcode, error) {
	type scored struct {
		name  string
		out   []byte
		score float64
	}
	results := make([]scored, 0, len(singleByteCandidates))
	for _, c := range singleByteCandidates {
		out, err := decodeWith(c.table, raw)
		if err != nil {
			continue
		}
		results = append(results, scored{name: c.name, out: out, score: scoreText(string(out))})
	}
	if len(results) == 0 {
		return nil, Transcode{}, fmt.Errorf("%w: кандидатов нет", ErrUnknownCharset)
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].score > results[j].score })
	if results[0].score < heuristicMinScore {
		parts := make([]string, 0, len(results))
		for _, r := range results {
			parts = append(parts, fmt.Sprintf("%s %.2f", r.name, r.score))
		}
		return nil, Transcode{}, fmt.Errorf("%w: %s", ErrUnknownCharset, strings.Join(parts, ", "))
	}
	best := results[0]
	return best.out, Transcode{Source: best.name, Method: methodHeuristic, Score: best.score}, nil
}

// scoreText — правдоподобие русского текста после декодирования (0..1):
// близость частот букв к русским, доля кириллицы и частые биграммы/слова.
// Замена-символы и латинские буквы скор снижают.
func scoreText(s string) float64 {
	lower := strings.ToLower(s)
	var letters, cyr, bad int
	freq := make(map[rune]int, len(russianLetterFreq))
	for _, r := range lower {
		switch {
		case r == utf8.RuneError:
			bad++
		case unicode.IsLetter(r):
			letters++
			if unicode.Is(unicode.Cyrillic, r) {
				cyr++
				freq[r]++
			}
		}
	}
	if letters == 0 {
		return 0
	}
	score := 0.5*freqSimilarity(freq, cyr) +
		0.2*float64(cyr)/float64(letters) +
		0.3*bigramRate(lower) -
		0.5*float64(bad)/float64(letters+bad)
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

// russianLetterFreq — частоты букв русского текста (доля среди букв): по ним
// эвристика отличает верную кодировку от неверной.
var russianLetterFreq = map[rune]float64{
	'о': 0.1097, 'е': 0.0845, 'а': 0.0801, 'и': 0.0735, 'н': 0.0670,
	'т': 0.0626, 'с': 0.0547, 'р': 0.0473, 'в': 0.0454, 'л': 0.0440,
	'к': 0.0349, 'м': 0.0321, 'д': 0.0298, 'п': 0.0281, 'у': 0.0262,
	'я': 0.0201, 'ы': 0.0190, 'ь': 0.0174, 'г': 0.0170, 'з': 0.0165,
	'б': 0.0159, 'ч': 0.0144, 'й': 0.0121, 'х': 0.0097, 'ж': 0.0094,
	'ш': 0.0073, 'ю': 0.0064, 'ц': 0.0048, 'щ': 0.0036, 'э': 0.0032,
	'ф': 0.0026, 'ъ': 0.0004, 'ё': 0.0004,
}

// russianCommonBigrams — частые русские биграммы эвристики.
var russianCommonBigrams = []string{
	"ст", "но", "то", "на", "ен", "ни", "по", "ко", "ра", "во",
	"пр", "ос", "го", "ер", "ло", "ли", "не", "та", "ть", "ел",
	"ал", "от", "ро", "ор", "ка", "ол", "ан", "ит", "те", "ла",
	"ат", "ов", "ом", "их", "ий", "ый", "ая", "ое", "ые", "ся",
}

// russianCommonWords — частые русские слова (с пробелами по краям) эвристики.
var russianCommonWords = []string{
	" и ", " в ", " не ", " на ", " что ", " как ", " это ", " по ",
	" но ", " из ", " от ", " так ", " же ", " все ", " она ", " его ",
	" был ", " для ", " они ", " когда ", " если ", " чтобы ", " у ", " с ",
}

// freqSimilarity — косинусная близость частот букв текста к russianLetterFreq.
func freqSimilarity(freq map[rune]int, total int) float64 {
	if total == 0 {
		return 0
	}
	var dot, normA, normB float64
	for r, want := range russianLetterFreq {
		got := float64(freq[r]) / float64(total)
		dot += got * want
		normA += got * got
		normB += want * want
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// bigramRate — доля частых русских биграмм (и вес частых слов) среди всех
// биграмм текста: верная кодировка даёт высокую долю, неверная — низкую.
func bigramRate(lower string) float64 {
	runes := []rune(lower)
	total := 0
	for i := 0; i+1 < len(runes); i++ {
		if unicode.IsLetter(runes[i]) && unicode.IsLetter(runes[i+1]) {
			total++
		}
	}
	if total == 0 {
		return 0
	}
	hits := 0
	for _, bg := range russianCommonBigrams {
		hits += strings.Count(lower, bg)
	}
	words := 0
	for _, w := range russianCommonWords {
		words += strings.Count(lower, w)
	}
	rate := (float64(hits) + 10*float64(words)) / float64(total)
	if rate > 1 {
		rate = 1
	}
	return rate
}

// declaredEncoding читает метку кодировки из XML-декларации (первые 1 КиБ).
func declaredEncoding(raw []byte) (string, bool) {
	head := raw
	if len(head) > 1024 {
		head = head[:1024]
	}
	m := xmlEncodingRe.FindSubmatch(head)
	if m == nil {
		return "", false
	}
	return string(m[1]), true
}

// withDeclUTF8 заменяет метку кодировки в XML-декларации на utf-8, иначе
// парсер снова упадёт на чужой декларации. Декларации нет — байты как есть.
func withDeclUTF8(doc []byte) []byte {
	limit := len(doc)
	if limit > 1024 {
		limit = 1024
	}
	loc := bytes.Index(doc[:limit], []byte("<?xml"))
	if loc < 0 {
		return doc
	}
	end := bytes.Index(doc[loc:limit], []byte("?>"))
	if end < 0 {
		return doc
	}
	span := doc[loc : loc+end]
	replaced := xmlEncodingAttrRe.ReplaceAll(span, []byte(`encoding="utf-8"`))
	if bytes.Equal(replaced, span) {
		return doc
	}
	out := make([]byte, 0, len(doc))
	out = append(out, doc[:loc]...)
	out = append(out, replaced...)
	out = append(out, doc[loc+end:]...)
	return out
}

// decodeWith перекодирует байты в utf-8 заданной кодировкой.
func decodeWith(enc textenc.Encoding, raw []byte) ([]byte, error) {
	out, _, err := transform.Bytes(enc.NewDecoder(), raw)
	if err != nil {
		return nil, fmt.Errorf("workshop: декодировать вход в utf-8: %w", err)
	}
	return out, nil
}

// charmapByLabel — таблица кодировки по метке декларации или MIME-имени.
func charmapByLabel(label string) (*charmap.Charmap, bool) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "windows-1251", "cp1251", "win-1251", "x-cp1251":
		return charmap.Windows1251, true
	case "koi8-r", "koi8r", "cp20866":
		return charmap.KOI8R, true
	case "cp866", "ibm866", "866":
		return charmap.CodePage866, true
	case "iso-8859-5", "iso8859-5", "cyrillic":
		return charmap.ISO8859_5, true
	case "windows-1252", "cp1252":
		return charmap.Windows1252, true
	}
	return nil, false
}

// canonicalCharsetName — каноническое имя кодировки для карточки и журнала.
func canonicalCharsetName(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "windows-1251", "cp1251", "win-1251", "x-cp1251":
		return "windows-1251"
	case "koi8-r", "koi8r", "cp20866":
		return "KOI8-R"
	case "cp866", "ibm866", "866":
		return "CP866"
	case "iso-8859-5", "iso8859-5", "cyrillic":
		return "ISO-8859-5"
	case "windows-1252", "cp1252":
		return "windows-1252"
	}
	return strings.ToLower(strings.TrimSpace(label))
}

// utf16ByName — декодер utf-16 по метке: без BOM utf-16 читается как BE
// (RFC 2781), с суффиксом le/be порядок задан меткой.
func utf16ByName(label string) textenc.Encoding {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "utf-16le", "utf16le":
		return textunicode.UTF16(textunicode.LittleEndian, textunicode.IgnoreBOM)
	case "utf-16be", "utf16be":
		return textunicode.UTF16(textunicode.BigEndian, textunicode.IgnoreBOM)
	default:
		return textunicode.UTF16(textunicode.BigEndian, textunicode.UseBOM)
	}
}

// charsetReader — подстраховка parseFB2 (IMP-8): старые не-перекодированные
// fb2 с чужой декларацией читаются через таблицы кириллических кодировок.
func charsetReader(label string, input io.Reader) (io.Reader, error) {
	low := strings.ToLower(strings.TrimSpace(label))
	if low == encUTF8 || low == "utf8" {
		return input, nil
	}
	if strings.HasPrefix(low, "utf-16") || low == "utf16" {
		return transform.NewReader(input, utf16ByName(low).NewDecoder()), nil
	}
	if table, ok := charmapByLabel(low); ok {
		return transform.NewReader(input, table.NewDecoder()), nil
	}
	return nil, fmt.Errorf("workshop: fb2: кодировка %q не поддержана", label)
}

// warnDeclarationLie пишет в журнал станка предупреждение о fb2, где заявлен
// utf-8, а байты не utf-8 (IMP-8): разбор при этом идёт дальше.
func warnDeclarationLie(data []byte) {
	label, ok := declaredEncoding(data)
	if !ok {
		return
	}
	low := strings.ToLower(strings.TrimSpace(label))
	if low != encUTF8 && low != "utf8" {
		return
	}
	if utf8.Valid(data) {
		return
	}
	if l := loggerOf(); l != nil {
		_ = l.Event("workshop", journal.EventSourceWarning,
			journal.Field{Key: "reason", Value: "fb2: декларация utf-8, байты не utf-8"})
	}
}

// parseFB2 — вставка перед чанкером (шаг 2): из XML титул и главы уходят в
// метаданные, тело — в существующий чанкер. Отдельного парсера тела нет — один
// обход XML собирает и заголовки, и текст глав.
func parseFB2(data []byte) (*ParsedBook, error) {
	warnDeclarationLie(data)
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charsetReader
	var (
		meta        BookMeta
		chapters    []Chapter
		authorParts []string
		bodyText    strings.Builder
	)

	var stack []string
	var buf []byte
	capture := ""

	flush := func() string {
		s := strings.TrimSpace(string(buf))
		buf = nil
		capture = ""
		return s
	}
	inElement := func(name string) bool {
		for _, e := range stack {
			if e == name {
				return true
			}
		}
		return false
	}

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("workshop: fb2: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			local := t.Name.Local
			stack = append(stack, local)
			switch local {
			case "book-title":
				if capture == "" {
					capture = "book-title"
				}
			case "first-name", "middle-name", "last-name", "nickname":
				if capture == "" && inElement("author") {
					capture = "author-name"
				}
			case "title":
				if capture == "" && inElement("body") {
					capture = "chapter-title"
				}
			case "p":
				if capture == "" && inElement("body") {
					capture = "p"
				}
			}

		case xml.EndElement:
			local := t.Name.Local
			switch local {
			case "book-title":
				if capture == "book-title" {
					meta.Title = flush()
				}
			case "first-name", "middle-name", "last-name", "nickname":
				if capture == "author-name" {
					if part := flush(); part != "" {
						authorParts = append(authorParts, part)
					}
				}
			case "author":
				meta.Author = strings.Join(authorParts, " ")
				authorParts = nil
			case "title":
				if capture == "chapter-title" {
					if title := flush(); title != "" {
						chapters = append(chapters, Chapter{Title: title})
						bodyText.WriteString(title + "\n\n")
					}
				}
			case "p":
				if capture == "p" {
					if text := flush(); text != "" {
						bodyText.WriteString(text + "\n\n")
					}
				}
			}
			stack = stack[:len(stack)-1]

		case xml.CharData:
			if capture != "" {
				buf = append(buf, t...)
			}
		}
	}

	meta.Chapters = chapters
	return &ParsedBook{Meta: meta, Chunks: chunkByChapters(bodyText.String())}, nil
}
