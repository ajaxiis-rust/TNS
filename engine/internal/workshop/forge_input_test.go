package workshop_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	textenc "golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	textunicode "golang.org/x/text/encoding/unicode"

	"tns65/internal/journal"
	"tns65/internal/workshop"
)

func TestForgeInput(t *testing.T) {
	// fb2: титул и главы — в метаданных, тело — в чанках, без отдельного
	// парсера тела.
	fb2, err := workshop.ParseBookInput(filepath.Join("testdata", "sample.fb2"))
	if err != nil {
		t.Fatalf("ParseBookInput(fb2): %v", err)
	}
	if fb2.Meta.Title != "Сумрачный город" {
		t.Errorf("Meta.Title = %q, want %q", fb2.Meta.Title, "Сумрачный город")
	}
	if fb2.Meta.Author != "Иван Петров" {
		t.Errorf("Meta.Author = %q, want %q", fb2.Meta.Author, "Иван Петров")
	}
	if len(fb2.Meta.Chapters) != 2 {
		t.Fatalf("глав в метаданных fb2: %d, want 2", len(fb2.Meta.Chapters))
	}
	wantChapters := []string{"Глава 1. Встреча", "Глава 2. Погоня"}
	for i, want := range wantChapters {
		if fb2.Meta.Chapters[i].Title != want {
			t.Errorf("Meta.Chapters[%d].Title = %q, want %q", i, fb2.Meta.Chapters[i].Title, want)
		}
	}
	if len(fb2.Chunks) != 2 {
		t.Fatalf("чанков fb2: %d, want 2", len(fb2.Chunks))
	}
	if !strings.Contains(fb2.Chunks[0].Text, "Первый абзац первой главы.") {
		t.Errorf("чанк 0 не содержит тело первой главы: %q", fb2.Chunks[0].Text)
	}
	if !strings.Contains(fb2.Chunks[1].Text, "Первый абзац второй главы.") {
		t.Errorf("чанк 1 не содержит тело второй главы: %q", fb2.Chunks[1].Text)
	}

	// txt: тело подаётся в чанкер напрямую и режется по границам глав.
	txt, err := workshop.ParseBookInput(filepath.Join("testdata", "sample.txt"))
	if err != nil {
		t.Fatalf("ParseBookInput(txt): %v", err)
	}
	if len(txt.Chunks) != 2 {
		t.Fatalf("чанков txt: %d, want 2", len(txt.Chunks))
	}
	if txt.Chunks[0].Chapter != "Глава 1. Начало" {
		t.Errorf("Chunks[0].Chapter = %q, want %q", txt.Chunks[0].Chapter, "Глава 1. Начало")
	}
	if !strings.Contains(txt.Chunks[1].Text, "Текст второй главы.") {
		t.Errorf("чанк 1 не содержит тело второй главы: %q", txt.Chunks[1].Text)
	}

	// Незнакомый формат — отказ с причиной.
	unknown := filepath.Join(t.TempDir(), "book.epub")
	if err := os.WriteFile(unknown, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err = workshop.ParseBookInput(unknown)
	if !errors.Is(err, workshop.ErrUnsupportedFormat) {
		t.Fatalf("ошибка незнакомого формата = %v, want ErrUnsupportedFormat", err)
	}
	if !strings.Contains(err.Error(), ".epub") || !strings.Contains(err.Error(), "txt и fb2") {
		t.Fatalf("отказ не содержит причину: %v", err)
	}
}

// charsetSample — русский текст фикстур кодировок (IMP-8): две главы, чтобы
// после перекодировки чанкер видел ту же разметку.
const charsetSample = `Глава 1. Встреча
Она пришла в дом на краю города, когда стемнело, и долго стояла у окна.
Ветер гнал по улице пыль, но в комнате было тихо и тепло.

Глава 2. Погоня
Он не стал ждать: взял фонарь, вышел на двор и побежал к реке.
Там, за поворотом, кто-то шёл по берегу и оставлял следы на песке.
`

// fb2Sample1251 — минимальная fb2-фикстура: две главы, декларация windows-1251.
const fb2Sample1251 = `<?xml version="1.0" encoding="windows-1251"?>
<FictionBook>
<description><title-info><book-title>Сумрачный город</book-title>
<author><first-name>Иван</first-name><last-name>Петров</last-name></author>
</title-info></description>
<body>
<section><title><p>Глава 1. Встреча</p></title><p>Первый абзац первой главы.</p></section>
<section><title><p>Глава 2. Погоня</p></title><p>Первый абзац второй главы.</p></section>
</body>
</FictionBook>
`

// encodeCharset перекодирует текст в кодировку фикстуры.
func encodeCharset(t *testing.T, enc textenc.Encoding, s string) string {
	t.Helper()
	out, err := enc.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return string(out)
}

// writeBook кладёт фикстуру во временный файл и возвращает путь.
func writeBook(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// TestBookCharsetNormalize — IMP-8: txt в windows-1251, KOI8-R, CP866, валидный
// utf-8 и utf-16LE с BOM, fb2 в windows-1251 — NormalizeBookFile перекодирует
// вход в utf-8 и называет исходную кодировку и метод.
func TestBookCharsetNormalize(t *testing.T) {
	txtCases := []struct {
		name     string
		content  string
		encoding string
		method   string
	}{
		{"w1251.txt", encodeCharset(t, charmap.Windows1251, charsetSample), "windows-1251", "эвристика"},
		{"koi8.txt", encodeCharset(t, charmap.KOI8R, charsetSample), "KOI8-R", "эвристика"},
		{"cp866.txt", encodeCharset(t, charmap.CodePage866, charsetSample), "CP866", "эвристика"},
		{"utf8.txt", charsetSample, "utf-8", "без перекодировки"},
		{"utf16.txt", encodeCharset(t, textunicode.UTF16(textunicode.LittleEndian, textunicode.UseBOM), charsetSample), "utf-16le", "BOM"},
	}
	for _, tc := range txtCases {
		path := writeBook(t, tc.name, tc.content)
		got, err := workshop.NormalizeBookFile(path)
		if err != nil {
			t.Fatalf("%s: NormalizeBookFile: %v", tc.name, err)
		}
		if got.Source != tc.encoding || got.Method != tc.method {
			t.Fatalf("%s: transcode = %+v, want %s/%s", tc.name, got, tc.encoding, tc.method)
		}
		if tc.method == "эвристика" && got.Score < 0.55 {
			t.Fatalf("%s: скор эвристики = %v, want >= 0.55", tc.name, got.Score)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: ReadFile: %v", tc.name, err)
		}
		if !utf8.Valid(raw) {
			t.Fatalf("%s: файл после перекодировки не utf-8", tc.name)
		}
		if string(raw) != charsetSample {
			t.Fatalf("%s: текст после перекодировки = %q, want %q", tc.name, raw, charsetSample)
		}
	}

	// fb2: декларация windows-1251 → utf-8, декларация переписана.
	path := writeBook(t, "book.fb2", encodeCharset(t, charmap.Windows1251, fb2Sample1251))
	got, err := workshop.NormalizeBookFile(path)
	if err != nil {
		t.Fatalf("fb2: NormalizeBookFile: %v", err)
	}
	if got.Source != "windows-1251" || got.Method != "декларация" {
		t.Fatalf("fb2: transcode = %+v, want windows-1251/декларация", got)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fb2: ReadFile: %v", err)
	}
	if !utf8.Valid(raw) {
		t.Fatal("fb2: файл после перекодировки не utf-8")
	}
	if !strings.Contains(string(raw), `encoding="utf-8"`) || strings.Contains(string(raw), "windows-1251") {
		t.Fatalf("fb2: декларация не переписана на utf-8: %q", raw)
	}
	parsed, err := workshop.ParseBookInput(path)
	if err != nil {
		t.Fatalf("fb2: ParseBookInput: %v", err)
	}
	if len(parsed.Meta.Chapters) != 2 || len(parsed.Chunks) != 2 {
		t.Fatalf("fb2: глав %d, чанков %d, want 2/2", len(parsed.Meta.Chapters), len(parsed.Chunks))
	}

	// Кодировку выбрать нельзя — ошибка со списком кандидатов и скоров.
	bad := writeBook(t, "bad.txt", "\x80\x81\x82\x83")
	_, err = workshop.NormalizeBookFile(bad)
	if !errors.Is(err, workshop.ErrUnknownCharset) {
		t.Fatalf("ошибка эвристики = %v, want ErrUnknownCharset", err)
	}
	for _, name := range []string{"windows-1251", "KOI8-R", "CP866", "ISO-8859-5"} {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("ошибка эвристики не называет кандидата %s: %v", name, err)
		}
	}
}

// TestParseFB2CharsetReader — IMP-8: parseFB2 читает не-перекодированные
// 1251-байты через CharsetReader, главы и титул на месте.
func TestParseFB2CharsetReader(t *testing.T) {
	path := writeBook(t, "legacy.fb2", encodeCharset(t, charmap.Windows1251, fb2Sample1251))
	parsed, err := workshop.ParseBookInput(path)
	if err != nil {
		t.Fatalf("ParseBookInput(1251 fb2): %v", err)
	}
	if parsed.Meta.Title != "Сумрачный город" {
		t.Fatalf("Meta.Title = %q, want Сумрачный город", parsed.Meta.Title)
	}
	if len(parsed.Meta.Chapters) != 2 {
		t.Fatalf("глав = %d, want 2", len(parsed.Meta.Chapters))
	}
	if !strings.Contains(parsed.Chunks[0].Text, "Первый абзац первой главы.") {
		t.Fatalf("тело первой главы не прочитано: %q", parsed.Chunks[0].Text)
	}
}

// TestParseFB2DeclarationLie — IMP-8: fb2 заявляет utf-8, байты не utf-8 —
// в журнал станка уходит предупреждение, разбор идёт дальше как раньше.
func TestParseFB2DeclarationLie(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	workshop.SetLogger(logger)
	defer workshop.SetLogger(nil)

	lie := strings.Replace(fb2Sample1251, `encoding="windows-1251"`, `encoding="utf-8"`, 1)
	path := writeBook(t, "lie.fb2", encodeCharset(t, charmap.Windows1251, lie))
	_, _ = workshop.ParseBookInput(path)

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(journal): %v", err)
	}
	if !strings.Contains(string(raw), journal.EventSourceWarning) {
		t.Fatalf("в журнале нет предупреждения %s: %s", journal.EventSourceWarning, raw)
	}
}

// TestChunkBoundaries — T-19-13: глава длиннее потолка режется по границе
// абзаца, предложения или пробела (слово не рвётся), маркеры глав расширены,
// текст без глав режется по абзацам с пустым заголовком.
func TestChunkBoundaries(t *testing.T) {
	// Маркеры глав: каждый открывает новый кусок, заголовок — сама строка.
	markers := []string{
		"Глава 1. Начало", "Chapter 2", "Пролог", "Эпилог",
		"Часть I. Первая", "Часть 2", "I. Первая", "IV. Четвёртая",
		"1. Пункт", "===", "***",
	}
	for _, marker := range markers {
		marker := marker
		t.Run("маркер "+marker, func(t *testing.T) {
			body := marker + "\nТекст первой сцены.\n\nГлава 99. Конец\nТекст второй сцены.\n"
			chunks := parseChunks(t, body)
			if len(chunks) != 2 {
				t.Fatalf("чанков = %d, want 2: %+v", len(chunks), chunks)
			}
			if chunks[0].Chapter != marker {
				t.Fatalf("Chapter куска 0 = %q, want %q", chunks[0].Chapter, marker)
			}
			if chunks[0].Text != "Текст первой сцены." {
				t.Fatalf("текст куска 0 = %q, want %q", chunks[0].Text, "Текст первой сцены.")
			}
			if chunks[1].Chapter != "Глава 99. Конец" {
				t.Fatalf("Chapter куска 1 = %q, want %q", chunks[1].Chapter, "Глава 99. Конец")
			}
			if !strings.Contains(chunks[1].Text, "Текст второй сцены.") {
				t.Fatalf("кусок 1 не содержит тело второй главы: %q", chunks[1].Text)
			}
		})
	}

	// Длинная глава из абзацев: куски не длиннее потолка и кончаются на
	// границе абзаца, слова не рвутся и не теряются.
	para := strings.Repeat("Корабль шёл вдоль берега, и команда молчала. ", 40)
	bodyText := strings.Join([]string{para, para, para}, "\n\n")
	chunks := parseChunks(t, "Глава 1. Длинная\n"+bodyText+"\n")
	if len(chunks) < 2 {
		t.Fatalf("длинная глава не разрезана: %d чанк(ов)", len(chunks))
	}
	for i, c := range chunks {
		if c.Chapter != "Глава 1. Длинная" {
			t.Fatalf("чанк %d: Chapter = %q, want %q", i, c.Chapter, "Глава 1. Длинная")
		}
		if got := utf8.RuneCountInString(c.Text); got > workshop.MaxChunkRunes {
			t.Fatalf("чанк %d длиной %d рун, want ≤ %d", i, got, workshop.MaxChunkRunes)
		}
		if i < len(chunks)-1 && utf8.RuneCountInString(c.Text) <= workshop.MaxChunkRunes/2 {
			t.Fatalf("чанк %d слишком короткий: %d рун", i, utf8.RuneCountInString(c.Text))
		}
		if !strings.HasSuffix(c.Text, ".") {
			t.Fatalf("чанк %d не кончается границей абзаца: %q", i, chunkTail(c.Text, 40))
		}
	}
	if got, want := joinWords(chunks), strings.Join(strings.Fields(bodyText), " "); got != want {
		t.Fatalf("слова чанков не совпадают с текстом главы: разрыв или потеря слова")
	}

	// Длинный абзац без переводов строк: граница — конец предложения.
	sentence := "Он смотрел на воду и думал о том, что путь ещё долог. "
	sentText := strings.Repeat(sentence, 100)
	sentChunks := parseChunks(t, "Глава 2. Сплошная\n"+sentText)
	if len(sentChunks) < 2 {
		t.Fatalf("длинный абзац не разрезан: %d чанк(ов)", len(sentChunks))
	}
	for i, c := range sentChunks {
		if got := utf8.RuneCountInString(c.Text); got > workshop.MaxChunkRunes {
			t.Fatalf("чанк %d длиной %d рун, want ≤ %d", i, got, workshop.MaxChunkRunes)
		}
		if !strings.HasSuffix(c.Text, ".") && i < len(sentChunks)-1 {
			t.Fatalf("чанк %d не кончается концом предложения: %q", i, chunkTail(c.Text, 40))
		}
	}
	if got, want := joinWords(sentChunks), strings.Join(strings.Fields(sentText), " "); got != want {
		t.Fatalf("слова чанков не совпадают с текстом абзаца")
	}

	// Абзац без терминаторов и переводов строк: граница — пробел, слово целое.
	wordy := strings.Repeat("слово ", 900)
	wordChunks := parseChunks(t, "Глава 3. Слова\n"+wordy)
	if len(wordChunks) < 2 {
		t.Fatalf("абзац без терминаторов не разрезан: %d чанк(ов)", len(wordChunks))
	}
	for i, c := range wordChunks {
		if got := utf8.RuneCountInString(c.Text); got > workshop.MaxChunkRunes {
			t.Fatalf("чанк %d длиной %d рун, want ≤ %d", i, got, workshop.MaxChunkRunes)
		}
		if !strings.HasSuffix(c.Text, "слово") {
			t.Fatalf("чанк %d разорвал слово: %q", i, chunkTail(c.Text, 40))
		}
	}
	if got, want := joinWords(wordChunks), strings.Join(strings.Fields(wordy), " "); got != want {
		t.Fatalf("слова чанков не совпадают с текстом абзаца")
	}

	// Текст без распознанных глав: режется по абзацам, заголовок пуст,
	// порядок кусков сохранён.
	plain := strings.Join([]string{
		"начало текста " + strings.Repeat("слово ", 400),
		"середина текста " + strings.Repeat("слово ", 400),
		"конец текста " + strings.Repeat("слово ", 400),
	}, "\n\n")
	plainChunks := parseChunks(t, plain)
	if len(plainChunks) < 2 {
		t.Fatalf("текст без глав не разрезан: %d чанк(ов)", len(plainChunks))
	}
	for i, c := range plainChunks {
		if c.Chapter != "" {
			t.Fatalf("чанк %d: Chapter = %q, want пусто", i, c.Chapter)
		}
		if got := utf8.RuneCountInString(c.Text); got > workshop.MaxChunkRunes {
			t.Fatalf("чанк %d длиной %d рун, want ≤ %d", i, got, workshop.MaxChunkRunes)
		}
	}
	if !strings.HasPrefix(plainChunks[0].Text, "начало текста") {
		t.Fatalf("первый кусок = %q, want начало текста", chunkTail(plainChunks[0].Text, 40))
	}
	if last := plainChunks[len(plainChunks)-1].Text; !strings.Contains(last, "конец текста") {
		t.Fatalf("последний кусок не содержит конец текста: %q", chunkTail(last, 40))
	}
	if got, want := joinWords(plainChunks), strings.Join(strings.Fields(plain), " "); got != want {
		t.Fatalf("слова чанков не совпадают с текстом без глав")
	}
}

// parseChunks разбирает книгу из строки и возвращает очередь чанков.
func parseChunks(t *testing.T, body string) []workshop.Chunk {
	t.Helper()
	parsed, err := workshop.ParseBookInput(writeBook(t, "chunks.txt", body))
	if err != nil {
		t.Fatalf("ParseBookInput: %v", err)
	}
	return parsed.Chunks
}

// joinWords склеивает слова чанков через пробел — сверка без потерь.
func joinWords(chunks []workshop.Chunk) string {
	words := make([]string, 0, len(chunks))
	for _, c := range chunks {
		words = append(words, strings.Fields(c.Text)...)
	}
	return strings.Join(words, " ")
}

// chunkTail — последние n рун текста для сообщений об ошибке.
func chunkTail(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[len(r)-n:]
	}
	return string(r)
}
