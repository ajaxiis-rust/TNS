package birth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/library"
	"tns65/internal/models"
	"tns65/internal/structural"
	"tns65/internal/workshop"
)

// Коды выхода подкоманды world birth (шаги 1 и 5).
const (
	ExitInvalid   = 2 // невалидная анкета: перечень причин, файлов не создано.
	ExitDuplicate = 6 // слаг уже существует: существующий мир не тронут.
)

// ErrDuplicateSlug — повторное рождение с существующим слагом.
var ErrDuplicateSlug = errors.New("birth: мир с таким слагом уже существует")

// ExitError — отказ рождения с кодом выхода команды.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }

func (e *ExitError) Unwrap() error { return e.Err }

// Config — параметры рождения мира.
type Config struct {
	WorldsRoot     string          // корень каталога миров; пусто — db.WorldsRoot.
	PacksRoot      string          // корень каталога пакетов; пусто — "packs".
	Logger         *journal.Logger // журнал станка; может быть nil.
	FailStep       string          // тестовый крючок: имя шага, на котором упасть.
	ProseCalls     *int            // тестовый счётчик вызовов слота прозы (гвардия B-04.1).
	PackCalls      *int            // тестовый счётчик вызовов генерации пачки (T-02-09).
	Isekai         bool            // переигровка в том же мире (T-02-12).
	BaseDBPath     string          // путь к замороженной базе языка; пусто — db.BaseRoot/base.<lang>.db.
	LibraryDir     string          // каталог полки книг; пусто — <WorldsRoot>/library.
	Embed          models.Embedder // эмбеддер векторов смысла; nil — клиент :5002.
	EmbedURL       string          // адрес эмбеддера для клиента по умолчанию; пусто — 127.0.0.1:5002.
	MatchThreshold float64         // порог косинусной близости якоря; 0 — 0.5.
	GazetteerPath  string          // путь к газетиру seed/gazetteer.jsonl; пусто — "seed/gazetteer.jsonl".
	GeoRadiusDays  float64         // радиус стартового региона в днях пути; 0 — 3.
	DayNormKm      float64         // норма дня пути в км; 0 — 25 (T-06-02).
}

// Result — созданный мир.
type Result struct {
	Slug       string
	Dir        string
	WorldDB    string // оперативная база <slug>.db.
	LiteraryDB string // литературная база literary.db.
}

// Состояния рождения (шаг 4): creating → ready или откат.
const (
	stateCreating = "creating"
	stateReady    = "ready"
)

const stateFileName = "state"

// registryKey — ключ world_meta с отпечатком канонического реестра семей
// (T-10-13, шаг 3): мир фиксирует, на каком наборе 14 он рождён.
const registryKey = "family_registry_sha256"

// defaultEmbedURL — адрес эмбеддера векторов смысла :5002 (T-02-07, «Создаётся»
// models/embed.go); подменяется полем Config.Embed в тестах.
const defaultEmbedURL = "http://127.0.0.1:5002"

// defaultGazetteerPath — путь к газетиру сида относительно рабочего каталога
// ядра (engine/): seed/gazetteer.jsonl (T-02-16, шаг 2).
const defaultGazetteerPath = "seed/gazetteer.jsonl"

// Birth рождает мир по анкете (B-03.1): каталог worlds/<slug>/ с оперативной
// базой и литературной, 15 ключей анкеты и якорь сценарной рамки в world_meta.
// Падение любого шага удаляет частично созданный каталог; повторная попытка
// начинается с чистого состояния.
func Birth(a *Ancketa, cfg Config) (*Result, error) {
	root := cfg.WorldsRoot
	if root == "" {
		root = db.WorldsRoot
	}
	packsRoot := cfg.PacksRoot
	if packsRoot == "" {
		packsRoot = "packs"
	}
	libraryDir := cfg.LibraryDir
	if libraryDir == "" {
		libraryDir = filepath.Join(root, "library")
	}

	// Шаг 1: валидация до создания каталога — единственная точка входа рождения
	// (T-02-09): без неё не стартует ни один шаг цепочки — ни базы, ни пачка, ни
	// якорь.
	if err := Validate(a); err != nil {
		return nil, &ExitError{Code: ExitInvalid, Err: withSynopsisHint(a, err)}
	}

	// Переигровка isekai (T-02-12): мир уже существует — каталог и обе базы не
	// пересоздаются, открываются новая сессия и книга в том же мире.
	if cfg.Isekai {
		return replayBirth(a, root, cfg)
	}

	dir := filepath.Join(root, "worlds", a.Slug)

	// Шаг 5: слаг уже существует — отказ кодом 6; существующий мир не трогаем.
	if _, err := os.Stat(dir); err == nil {
		if st, _ := readState(dir); st == stateReady {
			return nil, &ExitError{Code: ExitDuplicate, Err: fmt.Errorf("%w: %q", ErrDuplicateSlug, a.Slug)}
		}
		// Остаток прерванного рождения (состояние creating или без файла) —
		// чистим и начинаем с чистого состояния (шаг 4).
		if err := os.RemoveAll(dir); err != nil {
			return nil, fmt.Errorf("birth: убрать остаток каталога %s: %w", dir, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("birth: проверить каталог мира: %w", err)
	}

	// Язык анкеты — по установленным базам (TUN-2, T-10-03): отказ кодом 2 с
	// перечнем полей до создания каталога мира, тот же контракт, что у Validate.
	if fe := validateLangForConfig(a, cfg); fe != nil {
		return nil, &ExitError{Code: ExitInvalid, Err: &ValidationError{Fields: []FieldError{*fe}}}
	}

	// Книга-источник фанфика резолвится по id полки до создания каталога (P11
	// §5.5): id не найден — отказ кодом 2 с полем source_book.
	sourceBook, fe := resolveSourceBook(a, libraryDir)
	if fe != nil {
		return nil, &ExitError{Code: ExitInvalid, Err: &ValidationError{Fields: []FieldError{*fe}}}
	}

	// Шаг 2: каталог мира и метка состояния creating.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("birth: создать каталог мира: %w", err)
	}
	if err := writeState(dir, stateCreating); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("birth: метка состояния: %w", err)
	}

	baseDBPath := cfg.BaseDBPath
	if baseDBPath == "" {
		// Базы языка — по одной на язык (base.<lang>.db) в каталоге поставки
		// db.BaseRoot рядом с бинарниками движка (D-01, D-03, T-10-02): мир
		// рождается на языке анкеты, якорь читает векторы базы этого языка.
		// Каталог world_db/ базами поставки не занимается.
		baseDBPath = filepath.Join(db.BaseRoot, "base."+a.Lang+".db")
	}
	embedder := cfg.Embed
	if embedder == nil {
		embedURL := cfg.EmbedURL
		if embedURL == "" {
			embedURL = defaultEmbedURL
		}
		c, err := models.NewClient(embedURL, cfg.Logger)
		if err != nil {
			return nil, err
		}
		embedder = c
	}

	gazetteerPath := cfg.GazetteerPath
	if gazetteerPath == "" {
		gazetteerPath = defaultGazetteerPath
	}
	geoRadiusDays := cfg.GeoRadiusDays
	if geoRadiusDays <= 0 {
		geoRadiusDays = defaultGeoRadiusDays
	}
	dayNormKm := cfg.DayNormKm
	if dayNormKm <= 0 {
		dayNormKm = defaultDayNormKm
	}

	b := &builder{
		a:              a,
		dir:            dir,
		worldDB:        filepath.Join(dir, a.Slug+".db"),
		literaryDB:     filepath.Join(dir, "literary.db"),
		logger:         cfg.Logger,
		packCalls:      cfg.PackCalls,
		packsRoot:      packsRoot,
		baseDBPath:     baseDBPath,
		sourceBook:     sourceBook,
		embedder:       embedder,
		matchThreshold: cfg.MatchThreshold,
		gazetteerPath:  gazetteerPath,
		geoRadiusDays:  geoRadiusDays,
		dayNormKm:      dayNormKm,
	}

	if err := b.run(cfg); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}

	return &Result{
		Slug:       a.Slug,
		Dir:        dir,
		WorldDB:    b.worldDB,
		LiteraryDB: b.literaryDB,
	}, nil
}

// builder — состояние одного рождения.
type builder struct {
	a              *Ancketa
	dir            string
	worldDB        string
	literaryDB     string
	logger         *journal.Logger
	packCalls      *int
	packsRoot      string
	baseDBPath     string
	sourceBook     library.Book
	embedder       models.Embedder
	matchThreshold float64
	gazetteerPath  string
	geoRadiusDays  float64
	dayNormKm      float64
}

type birthStep struct {
	name string
	fn   func(b *builder) error
}

// steps — цепочка рождения. География (T-02-16) идёт после якоря, до готовности:
// падение любого шага откатывает рождение (T-02-03, шаг 4).
func (b *builder) steps() []birthStep {
	return []birthStep{
		{name: "world_db", fn: (*builder).createWorldDB},
		{name: "literary_db", fn: (*builder).createLiteraryDB},
		{name: "world_meta", fn: (*builder).writeWorldMeta},
		{name: "pack", fn: (*builder).pack},
		{name: "orders", fn: (*builder).enqueueOrders},
		{name: "seed", fn: (*builder).seedLiteraryBase},
		{name: "anchor", fn: (*builder).anchor},
		{name: "anchor_material", fn: (*builder).materializeAnchor},
		{name: "geo", fn: (*builder).geoSeed},
		{name: "ready", fn: (*builder).markReady},
	}
}

func (b *builder) run(cfg Config) error {
	for _, s := range b.steps() {
		if cfg.FailStep == s.name {
			return fmt.Errorf("birth: шаг %q упал (инъекция теста)", s.name)
		}
		if err := s.fn(b); err != nil {
			return fmt.Errorf("birth: шаг %q: %w", s.name, err)
		}
	}
	// Гвардия B-04.1: рождение не создаёт готовой прозы и не зовёт слот прозы.
	return checkNoProseAtBirth(b.worldDB, cfg.ProseCalls)
}

// createWorldDB — оперативная база <slug>.db по схеме world + shared vectors.
func (b *builder) createWorldDB() error {
	return createDB(b.worldDB, "world", "shared")
}

// createLiteraryDB — литературная база literary.db по схеме literary
// (shared-векторы к литературной не подключены, D-19.2).
func (b *builder) createLiteraryDB() error {
	return createDB(b.literaryDB, "literary")
}

// createDB ставит схему мигратором и закрывает служебный хэндл записи.
func createDB(path string, dirs ...string) error {
	raw, err := db.OpenWriter(path)
	if err != nil {
		return err
	}
	defer raw.Close()
	if err := db.Migrate(raw, dirs...); err != nil {
		return err
	}
	return nil
}

// writeWorldMeta — 15 ключей анкеты в world_meta хэндлом forge (B-03.5, карта
// писателей T-01-07). После рождения world_meta пишет только forge: запись из
// интерфейса (писатель core) отклоняется картой писателей (D-07, B-03.8).
func (b *builder) writeWorldMeta() error {
	h, err := db.OpenWriterAs(b.worldDB, db.DBWorld, db.WriterForge, b.logger)
	if err != nil {
		return err
	}
	defer h.Close()

	values, err := ancketaValues(b.a)
	if err != nil {
		return err
	}
	for _, k := range Fields {
		if _, err := h.Exec("world_meta",
			`INSERT INTO world_meta ("key", value) VALUES (?, ?)`, k, values[k]); err != nil {
			return err
		}
	}
	// Отпечаток реестра семей (T-10-13, шаг 3) — тем же хэндлом forge, что и
	// прочие ключи world_meta при рождении. Реестр не найден — ключ не пишется
	// (рождение мира от отпечатка не зависит).
	if sum := registrySHA256(); sum != "" {
		if _, err := h.Exec("world_meta",
			`INSERT INTO world_meta ("key", value) VALUES (?, ?)`, registryKey, sum); err != nil {
			return err
		}
	}
	// Привязка фанфика (P11 §5.5): книга полки регистрируется в реестре
	// literary.db, а в world_meta пишутся source_book_id и source_book_name.
	return b.writeSourceBookBinding(h)
}

// writeSourceBookBinding пишет привязку книги-источника фанфика тем же хэндлом
// forge, что и остальные ключи world_meta. Без книги-источника (жанр не фанфик)
// ничего не делает.
func (b *builder) writeSourceBookBinding(world *db.Handle) error {
	if b.sourceBook.ID == "" {
		return nil
	}
	lit, err := db.OpenWriterAs(b.literaryDB, db.DBLiterary, db.WriterForge, b.logger)
	if err != nil {
		return err
	}
	defer lit.Close()

	id, err := workshop.InsertBook(lit, workshop.Book{
		ShelfID: b.sourceBook.ID,
		Name:    b.sourceBook.Name,
		Lang:    b.a.Lang,
		Status:  library.StatusReady,
	})
	if err != nil {
		return err
	}
	if _, err := world.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES (?, ?)`, "source_book_id", strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	_, err = world.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES (?, ?)`, "source_book_name", b.sourceBook.Name)
	return err
}

// resolveSourceBook резолвит книгу-источник фанфика по id полки (P11 §5.5).
// Пустое значение — книги нет, не ошибка; id не найден в манифесте полки —
// ошибка поля source_book. Возвращает запись полки для привязки при рождении.
func resolveSourceBook(a *Ancketa, libraryDir string) (library.Book, *FieldError) {
	if a.SourceBook == "" {
		return library.Book{}, nil
	}
	books, err := library.Load(libraryDir)
	if err != nil {
		return library.Book{}, &FieldError{Field: "source_book", Reason: "полка книг недоступна: " + err.Error()}
	}
	for _, bk := range books {
		if bk.ID == a.SourceBook {
			return bk, nil
		}
	}
	return library.Book{}, &FieldError{Field: "source_book", Reason: "книга полки не найдена: " + a.SourceBook}
}

// registrySHA256 — отпечаток канонического реестра семей base/archetypes.json.
// Ошибка загрузки — пустая строка: ключ не пишется, рождение не падает.
func registrySHA256() string {
	root := findEngineRoot()
	if root == "" {
		return ""
	}
	fams, err := structural.LoadFamilies(structural.FamiliesPath(root))
	if err != nil {
		return ""
	}
	return structural.RegistrySHA256(fams)
}

// findEngineRoot поднимается от рабочего каталога до каталога с go.mod — корень
// движка, от которого считается base/archetypes.json.
func findEngineRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
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

// pack — пачка из 96 карточек (T-02-04): 16 психотипов × 6 сословий в characters
// оперативной базы хэндлом core (карта писателей T-01-07). Падение здесь
// откатывает рождение (шаг 4).
func (b *builder) pack() error {
	if b.packCalls != nil {
		*b.packCalls++
	}
	cards, err := Pack(b.a)
	if err != nil {
		return err
	}

	h, err := db.OpenWriterAs(b.worldDB, db.DBWorld, db.WriterCore, b.logger)
	if err != nil {
		return err
	}
	defer h.Close()

	// Имя NPC — не часть пачки (B-05.3): поле characters.name пусто, имя
	// назначается в живом ходе.
	return h.Tx(db.WriterCore, func(tx *db.Tx) error {
		for _, c := range cards {
			raw, err := json.Marshal(c)
			if err != nil {
				return err
			}
			if _, err := tx.Exec("characters",
				`INSERT INTO characters (name, psychotype, estate, card_json) VALUES (?, ?, ?, ?)`,
				"", c.Psychotype, c.Estate, string(raw)); err != nil {
				return err
			}
		}
		return nil
	})
}

// markReady — финальное состояние ready (шаг 4).
func (b *builder) markReady() error {
	return writeState(b.dir, stateReady)
}

// enqueueOrders — постановка двух заказов мастерской при рождении (T-02-11,
// шаг 2): analysis и forge_plan хэндлом core. До исполнителя очереди (P4) заказы
// живут со статусом queued.
func (b *builder) enqueueOrders() error {
	h, err := db.OpenWriterAs(b.worldDB, db.DBWorld, db.WriterCore, b.logger)
	if err != nil {
		return err
	}
	defer h.Close()
	return workshop.EnqueueBirthOrders(h)
}

// checkNoProseAtBirth — гвардия B-04.1: при рождении не создаётся ни одной
// готовой сцены, реплики или главы живого хода. Три таблицы оперативной базы
// пусты, а слот прозы цепочкой рождения не вызывался (счётчик 0).
func checkNoProseAtBirth(worldDB string, proseCalls *int) error {
	if proseCalls != nil && *proseCalls != 0 {
		return fmt.Errorf("birth: вызовов слота прозы %d, ожидалось 0", *proseCalls)
	}
	d, err := db.OpenReader(worldDB)
	if err != nil {
		return fmt.Errorf("birth: открыть базу для гвардии прозы: %w", err)
	}
	defer d.Close()
	for _, table := range []string{"scenes", "turns", "book_text"} {
		var n int
		if err := d.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			return fmt.Errorf("birth: проверить %s: %w", table, err)
		}
		if n != 0 {
			return fmt.Errorf("birth: %s непуста: %d строк", table, n)
		}
	}
	return nil
}

// ancketaValues — пары «ключ — JSON-значение» для 15 полей анкеты в порядке Fields.
func ancketaValues(a *Ancketa) (map[string]string, error) {
	raw, err := json.Marshal(a)
	if err != nil {
		return nil, fmt.Errorf("birth: маршалинг анкеты: %w", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("birth: разбор анкеты: %w", err)
	}
	out := make(map[string]string, len(Fields))
	for _, k := range Fields {
		v, ok := m[k]
		if !ok {
			// source_book необязателен и помечен omitempty: при пустом
			// значении его нет в JSON — в world_meta пишем пустую строку.
			if k == "source_book" {
				out[k] = ""
				continue
			}
			return nil, fmt.Errorf("birth: ключ %q отсутствует в анкете", k)
		}
		out[k] = string(v)
	}
	return out, nil
}

func writeState(dir, state string) error {
	return os.WriteFile(filepath.Join(dir, stateFileName), []byte(state+"\n"), 0o644)
}

func readState(dir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, stateFileName))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

// withSynopsisHint добавляет к отказу подсказку «заполните синопсис», когда
// синопсис пуст (T-02-09): пользователь должен понять, как исправить анкету, а
// не только что поле пусто.
func withSynopsisHint(a *Ancketa, err error) error {
	if strings.TrimSpace(a.Synopsis) == "" {
		return fmt.Errorf("%w — заполните синопсис", err)
	}
	return err
}
