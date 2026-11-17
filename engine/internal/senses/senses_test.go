package senses_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/senses"
)

// resolveCase — предложение смысла: омоним и слова-триггеры его контекста.
// Лемматизация живёт вне пакета, поэтому тест подаёт леммы напрямую.
type resolveCase struct {
	lemma    string
	sense    string
	triggers []string
}

// resolveCases — по одному предложению на каждый смысл всех 12 лемм волны.
var resolveCases = []resolveCase{
	{"замок", "строение", []string{"ров", "решётка", "рыцарь", "король", "принцесса", "стража"}},
	{"замок", "запирающее устройство", []string{"скважина", "засов", "защёлка", "отпереть", "связка", "слесарь"}},
	{"ключ", "родник", []string{"журчать", "напиться", "студёный", "жажда", "напоить", "жадно"}},
	{"ключ", "отпирающее устройство", []string{"скважина", "отпереть", "связка", "дверца", "слесарь"}},
	{"коса", "причёска", []string{"заплести", "лента", "заколка", "плести", "распустить", "девичий"}},
	{"коса", "сельскохозяйственное орудие", []string{"косить", "сенокос", "покос", "точить", "грабли", "лезвие"}},
	{"коса", "песчаная отмель", []string{"мель", "песчаный", "остров", "обсохнуть", "фарватер"}},
	{"кисть", "рука", []string{"пожать", "сжать", "браслет", "маникюр", "рукопожатие"}},
	{"кисть", "инструмент для рисования", []string{"краска", "мазок", "холст", "палитра", "акварель", "маляр"}},
	{"кисть", "гроздь ягод", []string{"виноград", "ягода", "гроздь", "рябина", "смородина"}},
	{"ножка", "опора мебели", []string{"шататься", "подпилить", "столяр", "скрипеть", "пошатнуться"}},
	{"ножка", "часть гриба", []string{"гриб", "боровик", "шляпка", "подберёзовик", "корзинка"}},
	{"ножка", "детская нога", []string{"топать", "босой", "пелёнка", "колготки", "коляска"}},
	{"пол", "настил", []string{"паркет", "половица", "подметать", "линолеум", "плинтус"}},
	{"пол", "биологический признак", []string{"мужчина", "женщина", "самка", "самец", "род"}},
	{"батарея", "отопление", []string{"тепло", "труба", "обогреватель", "радиатор", "чугунный", "горячий"}},
	{"батарея", "артиллерийская", []string{"пушка", "пальба", "канонир", "запал", "лафет"}},
	{"батарея", "электрическая", []string{"фонарик", "заряд", "аккумулятор", "клемма", "разрядиться"}},
	{"ласка", "животное", []string{"зверёк", "горностай", "куница", "норка", "пушистый", "хищный"}},
	{"ласка", "нежность", []string{"утешать", "прикосновение", "забота", "трепетный", "голубить", "ворковать"}},
	{"свет", "освещение", []string{"лампа", "лампочка", "фонарь", "луч", "яркий", "люстра"}},
	{"свет", "высшее общество", []string{"салон", "сплетня", "аристократ", "манеры", "бал", "этикет"}},
	{"свет", "мир, вселенная", []string{"вселенная", "мироздание", "сотворить", "бытие", "создатель"}},
	{"звезда", "небесное тело", []string{"созвездие", "телескоп", "мерцать", "галактика", "астроном", "небосклон"}},
	{"звезда", "знаменитость", []string{"эстрада", "поклонник", "слава", "интервью", "фанат", "эфир"}},
	{"звезда", "награда", []string{"награда", "медаль", "орден", "герой", "лавры", "вручить"}},
	{"гребень", "расчёска", []string{"расчесать", "пробор", "заколка", "прядь", "спутаться"}},
	{"гребень", "горный", []string{"перевал", "отрог", "седловина", "кряж", "альпинист"}},
	{"гребень", "петушиный", []string{"петух", "курица", "курятник", "петушок", "хохолок"}},
	{"кран", "водопроводный", []string{"вода", "труба", "смеситель", "сантехник", "протекать"}},
	{"кран", "подъёмный", []string{"стрела", "стройка", "груз", "башенный", "монтажник", "поднимать"}},
}

// packsDir — каталог языкового пакета: тесты идут из каталога пакета.
func packsDir() string {
	return filepath.Join("..", "..", "packs", "ru")
}

// loadSenses загружает словарь смыслов пака через загрузчик.
func loadSenses(t *testing.T) *senses.Dictionary {
	t.Helper()
	d, err := senses.Load(filepath.Join(packsDir(), "senses.json"))
	if err != nil {
		t.Fatalf("Load(senses.json): %v", err)
	}
	return d
}

// readSensesFile читает senses.json пака как данные: разметка теста сверяется с
// файлом, а не с загрузчиком.
func readSensesFile(t *testing.T) []senses.Sense {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(packsDir(), "senses.json"))
	if err != nil {
		t.Fatalf("ReadFile(senses.json): %v", err)
	}
	var f struct {
		Version int            `json:"version"`
		Senses  []senses.Sense `json:"senses"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("senses.json: %v", err)
	}
	if f.Version != 1 {
		t.Fatalf("версия схемы %d, ожидалась 1", f.Version)
	}
	return f.Senses
}

// sentenceTokens собирает токены предложения одного абзаца: омоним первым
// токеном, за ним слова его триггеров.
func sentenceTokens(lemma string, triggers []string) []senses.Token {
	tokens := make([]senses.Token, 0, len(triggers)+1)
	tokens = append(tokens, senses.Token{Form: lemma, Lemma: lemma, POS: "NOUN", Line: 1})
	for i, tr := range triggers {
		tokens = append(tokens, senses.Token{Form: tr, Lemma: tr, POS: "NOUN", Line: 1, Index: i + 1})
	}
	return tokens
}

// TestResolveSenses — по одному предложению на каждый смысл всех 12 лемм
// (шаг 5): токены омонима и его триггеров дают верно решённый смысл, набор
// случаев совпадает со словарём пака; окно, абзац и однократность триггера
// держат шаг 4.
func TestResolveSenses(t *testing.T) {
	d := loadSenses(t)
	fileSenses := readSensesFile(t)

	cases := make(map[string]resolveCase, len(resolveCases))
	for _, c := range resolveCases {
		key := c.lemma + "/" + c.sense
		if _, dup := cases[key]; dup {
			t.Fatalf("случай %s повторяется", key)
		}
		cases[key] = c
	}
	if len(cases) != len(fileSenses) {
		t.Fatalf("случаев %d, смыслов в senses.json %d", len(cases), len(fileSenses))
	}

	for _, s := range fileSenses {
		key := s.Lemma + "/" + s.Sense
		c, ok := cases[key]
		if !ok {
			t.Fatalf("нет предложения для смысла %s", key)
		}
		own := make(map[string]struct{}, len(s.Triggers))
		for _, tr := range s.Triggers {
			own[tr.Lemma] = struct{}{}
		}
		for _, tr := range c.triggers {
			if _, ok := own[tr]; !ok {
				t.Fatalf("%s: триггер %q не из словаря смысла", key, tr)
			}
		}

		decisions := d.Resolve(sentenceTokens(c.lemma, c.triggers))
		if len(decisions) != 1 {
			t.Fatalf("%s: решений %d, ожидалось 1", key, len(decisions))
		}
		got := decisions[0]
		if got.Index != 0 {
			t.Fatalf("%s: Index %d, ожидался 0", key, got.Index)
		}
		if !got.Decided {
			t.Fatalf("%s: смысл не решён (score %.2f, margin %.2f, evidence %v)",
				key, got.Score, got.Margin, got.Evidence)
		}
		if got.Sense != s.Sense {
			t.Fatalf("%s: решён смысл %q, ожидался %q (score %.2f, margin %.2f, evidence %v)",
				key, got.Sense, s.Sense, got.Score, got.Margin, got.Evidence)
		}
		if got.Category != s.Category {
			t.Fatalf("%s: категория %q, ожидалась %q", key, got.Category, s.Category)
		}
		if len(got.Evidence) != len(c.triggers) {
			t.Fatalf("%s: evidence %v, ожидались все триггеры окна %v", key, got.Evidence, c.triggers)
		}
	}

	// withFiller — омоним «пол» и n токенов-заполнителей того же абзаца.
	withFiller := func(n int) []senses.Token {
		tokens := make([]senses.Token, 0, n+2)
		tokens = append(tokens, senses.Token{Form: "пол", Lemma: "пол", POS: "NOUN", Line: 1})
		for i := 0; i < n; i++ {
			tokens = append(tokens, senses.Token{Form: "и", Lemma: "и", POS: "CONJ", Line: 1, Index: i + 1})
		}
		return tokens
	}

	t.Run("окно ±12", func(t *testing.T) {
		edge := append(withFiller(11), senses.Token{Form: "паркет", Lemma: "паркет", POS: "NOUN", Line: 1})
		decisions := d.Resolve(edge)
		if len(decisions) != 1 || !decisions[0].Decided || decisions[0].Sense != "настил" {
			t.Fatalf("триггер на границе окна не учтён: %+v", decisions)
		}

		far := append(withFiller(12), senses.Token{Form: "паркет", Lemma: "паркет", POS: "NOUN", Line: 1})
		decisions = d.Resolve(far)
		if len(decisions) != 1 || decisions[0].Decided {
			t.Fatalf("триггер за границей окна учтён: %+v", decisions)
		}
	})

	t.Run("чужой абзац", func(t *testing.T) {
		tokens := []senses.Token{
			{Form: "пол", Lemma: "пол", POS: "NOUN", Line: 1},
			{Form: "паркет", Lemma: "паркет", POS: "NOUN", Line: 2},
		}
		decisions := d.Resolve(tokens)
		if len(decisions) != 1 || decisions[0].Decided {
			t.Fatalf("триггер чужого абзаца учтён: %+v", decisions)
		}
	})

	t.Run("триггер один раз", func(t *testing.T) {
		decisions := d.Resolve(sentenceTokens("пол", []string{"паркет", "паркет", "паркет"}))
		if len(decisions) != 1 || !decisions[0].Decided {
			t.Fatalf("ожидался решённый смысл: %+v", decisions)
		}
		if math.Abs(decisions[0].Score-1.2) > 1e-9 {
			t.Fatalf("score = %g, ожидался вес одного триггера 1.2", decisions[0].Score)
		}
	})
}

// TestResolveAmbiguous — без триггеров, со слабым триггером, с равным счётом
// смыслов и на токенах вне словаря смысл не решается (шаг 4).
func TestResolveAmbiguous(t *testing.T) {
	d := loadSenses(t)

	t.Run("без триггеров", func(t *testing.T) {
		tokens := []senses.Token{
			{Form: "пол", Lemma: "пол", POS: "NOUN", Line: 1},
			{Form: "был", Lemma: "быть", POS: "VERB", Line: 1, Index: 1},
			{Form: "холодным", Lemma: "холодный", POS: "ADJF", Line: 1, Index: 2},
		}
		decisions := d.Resolve(tokens)
		if len(decisions) != 1 {
			t.Fatalf("решений %d, ожидалось 1", len(decisions))
		}
		got := decisions[0]
		if got.Decided || got.Sense != "" || got.Category != "" || got.Score != 0 || len(got.Evidence) != 0 {
			t.Fatalf("ожидался нерешённый смысл, получено %+v", got)
		}
	})

	t.Run("порог веса", func(t *testing.T) {
		// Единственный триггер «слесарь» весит 0.6 — ниже MinScore.
		decisions := d.Resolve(sentenceTokens("ключ", []string{"слесарь"}))
		if len(decisions) != 1 || decisions[0].Decided {
			t.Fatalf("слабый триггер решил смысл: %+v", decisions)
		}
		if math.Abs(decisions[0].Score-0.6) > 1e-9 {
			t.Fatalf("score = %g, ожидалось 0.6", decisions[0].Score)
		}
	})

	t.Run("отрыв", func(t *testing.T) {
		// «паркет» (1.2) против «мужчина» (1.0): отрыв 0.2 ниже MinMargin.
		decisions := d.Resolve(sentenceTokens("пол", []string{"паркет", "мужчина"}))
		if len(decisions) != 1 || decisions[0].Decided {
			t.Fatalf("малый отрыв решил смысл: %+v", decisions)
		}
		if math.Abs(decisions[0].Margin-0.2) > 1e-9 {
			t.Fatalf("margin = %g, ожидался 0.2", decisions[0].Margin)
		}
	})

	t.Run("не омоним", func(t *testing.T) {
		tokens := []senses.Token{{Form: "стол", Lemma: "стол", POS: "NOUN", Line: 1}}
		if decisions := d.Resolve(tokens); len(decisions) != 0 {
			t.Fatalf("лемма вне словаря смыслов дала решения: %+v", decisions)
		}
	})

	t.Run("другая часть речи", func(t *testing.T) {
		tokens := []senses.Token{{Form: "коса", Lemma: "коса", POS: "VERB", Line: 1}}
		if decisions := d.Resolve(tokens); len(decisions) != 0 {
			t.Fatalf("часть речи вне словаря смыслов дала решения: %+v", decisions)
		}
	})
}

// threeTriggers — три триггера валидного смысла.
const threeTriggers = `{"lemma":"ров","weight":1.0},{"lemma":"рыцарь","weight":0.9},{"lemma":"стража","weight":0.8}`

// oneSense собирает файл senses.json из одного смысла с заданными триггерами.
func oneSense(lemma, name, category, triggers string) string {
	return `{"version":1,"senses":[{"lemma":"` + lemma + `","pos":"NOUN","sense":"` + name +
		`","category":"` + category + `","triggers":[` + triggers + `]}]}`
}

// writeSensesFile кладёт тело senses.json во временный каталог.
func writeSensesFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "senses.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// TestLoadSenses — загрузчик отвергает битый файл, повтор ключа
// (lemma, pos, sense), смысл с менее чем тремя триггерами, вес вне 0.5–1.5 и
// категорию вне канона.
func TestLoadSenses(t *testing.T) {
	t.Run("валидный", func(t *testing.T) {
		path := writeSensesFile(t, oneSense("замок", "строение", "architecture", threeTriggers))
		if _, err := senses.Load(path); err != nil {
			t.Fatalf("Load: %v", err)
		}
	})

	t.Run("разбор", func(t *testing.T) {
		if _, err := senses.Load(writeSensesFile(t, `{"version":1,"senses":[`)); err == nil {
			t.Fatal("битый JSON разобран без ошибки")
		}
	})

	t.Run("версия", func(t *testing.T) {
		if _, err := senses.Load(writeSensesFile(t, `{"version":2,"senses":[]}`)); err == nil {
			t.Fatal("версия схемы 2 принята без ошибки")
		}
	})

	t.Run("пусто", func(t *testing.T) {
		if _, err := senses.Load(writeSensesFile(t, `{"version":1,"senses":[]}`)); err == nil {
			t.Fatal("пустой словарь принят без ошибки")
		}
	})

	t.Run("дубликат ключа", func(t *testing.T) {
		sense := `{"lemma":"замок","pos":"NOUN","sense":"строение","category":"architecture","triggers":[` +
			threeTriggers + `]}`
		body := `{"version":1,"senses":[` + sense + `,` + sense + `]}`
		if _, err := senses.Load(writeSensesFile(t, body)); err == nil {
			t.Fatal("повтор (lemma, pos, sense) принят без ошибки")
		}
	})

	t.Run("мало триггеров", func(t *testing.T) {
		path := writeSensesFile(t, oneSense("замок", "строение", "architecture",
			`{"lemma":"ров","weight":1.0},{"lemma":"рыцарь","weight":0.9}`))
		if _, err := senses.Load(path); err == nil {
			t.Fatal("смысл с двумя триггерами принят без ошибки")
		}
	})

	t.Run("вес", func(t *testing.T) {
		path := writeSensesFile(t, oneSense("замок", "строение", "architecture",
			`{"lemma":"ров","weight":2.0},{"lemma":"рыцарь","weight":0.9},{"lemma":"стража","weight":0.8}`))
		if _, err := senses.Load(path); err == nil {
			t.Fatal("вес 2.0 принят без ошибки")
		}
	})

	t.Run("категория", func(t *testing.T) {
		path := writeSensesFile(t, oneSense("замок", "строение", "dwelling", threeTriggers))
		if _, err := senses.Load(path); err == nil {
			t.Fatal("категория вне канона принята без ошибки")
		}
	})
}

// TestSensesFile — реальный packs/ru/senses.json (шаг 2): 12 лемм-омонимов, у
// каждой не меньше двух смыслов, у каждого смысла не меньше трёх триггеров с
// весом 0.5–1.5, категория из канона, а триггеры не стоят в словаре категории
// своего смысла и не повторяются у смыслов одной леммы.
func TestSensesFile(t *testing.T) {
	fileSenses := readSensesFile(t)
	wantLemmas := []string{
		"замок", "ключ", "коса", "кисть", "ножка", "пол",
		"батарея", "ласка", "свет", "звезда", "гребень", "кран",
	}

	byLemma := make(map[string][]senses.Sense, len(wantLemmas))
	seen := make(map[string]struct{}, len(fileSenses))
	for _, s := range fileSenses {
		key := s.Lemma + "/" + s.POS + "/" + s.Sense
		if _, dup := seen[key]; dup {
			t.Fatalf("смысл %s повторяется", key)
		}
		seen[key] = struct{}{}
		byLemma[s.Lemma] = append(byLemma[s.Lemma], s)
	}
	if len(byLemma) != len(wantLemmas) {
		t.Fatalf("лемм %d, ожидалось %d", len(byLemma), len(wantLemmas))
	}

	allowed := map[string]bool{"": true}
	for _, c := range senses.Categories {
		allowed[c] = true
	}
	dicts := make(map[string]map[string]struct{}, len(senses.Categories))

	for _, lemma := range wantLemmas {
		group := byLemma[lemma]
		if len(group) < 2 {
			t.Fatalf("лемма %s: смыслов %d, ожидалось не меньше 2", lemma, len(group))
		}
		shared := make(map[string]string, len(group)*6)
		for _, s := range group {
			key := s.Lemma + "/" + s.Sense
			if !allowed[s.Category] {
				t.Fatalf("%s: категория %q вне канона", key, s.Category)
			}
			if len(s.Triggers) < 3 {
				t.Fatalf("%s: триггеров %d, ожидалось не меньше 3", key, len(s.Triggers))
			}
			dict := categoryLemmas(t, dicts, s.Category)
			for _, tr := range s.Triggers {
				if tr.Weight < 0.5 || tr.Weight > 1.5 {
					t.Fatalf("%s: триггер %q: вес %g вне 0.5–1.5", key, tr.Lemma, tr.Weight)
				}
				if _, ok := dict[tr.Lemma]; ok {
					t.Fatalf("%s: триггер %q стоит в словаре категории %q", key, tr.Lemma, s.Category)
				}
				if other, dup := shared[tr.Lemma]; dup {
					t.Fatalf("лемма %s: триггер %q общий у смыслов %q и %q",
						lemma, tr.Lemma, other, s.Sense)
				}
				shared[tr.Lemma] = s.Sense
			}
		}
	}
}

// categoryLemmas читает словарь категории пака и возвращает его леммы; у смысла
// вне категорий словаря нет.
func categoryLemmas(t *testing.T, cache map[string]map[string]struct{}, category string) map[string]struct{} {
	t.Helper()
	if category == "" {
		return nil
	}
	if lemmas, ok := cache[category]; ok {
		return lemmas
	}
	raw, err := os.ReadFile(filepath.Join(packsDir(), category+".json"))
	if err != nil {
		t.Fatalf("ReadFile(%s.json): %v", category, err)
	}
	var f struct {
		Lemmas map[string]int `json:"lemmas"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("%s.json: %v", category, err)
	}
	lemmas := make(map[string]struct{}, len(f.Lemmas))
	for lemma := range f.Lemmas {
		lemmas[strings.ToLower(lemma)] = struct{}{}
	}
	cache[category] = lemmas
	return lemmas
}
