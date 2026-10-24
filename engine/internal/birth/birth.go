package birth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tns65/internal/db"
	"tns65/internal/journal"
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
	WorldsRoot string          // корень каталога миров; пусто — db.WorldsRoot.
	Logger     *journal.Logger // журнал станка; может быть nil.
	FailStep   string          // тестовый крючок: имя шага, на котором упасть.
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

// Birth рождает мир по анкете (B-03.1): каталог worlds/<slug>/ с оперативной
// базой и литературной, 14 ключей анкеты в world_meta. Падение любого шага
// удаляет частично созданный каталог; повторная попытка начинается с чистого
// состояния.
func Birth(a *Ancketa, cfg Config) (*Result, error) {
	root := cfg.WorldsRoot
	if root == "" {
		root = db.WorldsRoot
	}

	// Шаг 1: валидация до создания каталога.
	if err := Validate(a); err != nil {
		return nil, &ExitError{Code: ExitInvalid, Err: err}
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

	// Шаг 2: каталог мира и метка состояния creating.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("birth: создать каталог мира: %w", err)
	}
	if err := writeState(dir, stateCreating); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("birth: метка состояния: %w", err)
	}

	b := &builder{
		a:          a,
		dir:        dir,
		worldDB:    filepath.Join(dir, a.Slug+".db"),
		literaryDB: filepath.Join(dir, "literary.db"),
		logger:     cfg.Logger,
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
	a          *Ancketa
	dir        string
	worldDB    string
	literaryDB string
	logger     *journal.Logger
}

type birthStep struct {
	name string
	fn   func(b *builder) error
}

// steps — цепочка рождения. Швы для следующих задач: pack — T-02-04, якорь —
// T-02-07, заказы — T-02-11, сид — T-02-13, география — T-02-16.
func (b *builder) steps() []birthStep {
	return []birthStep{
		{name: "world_db", fn: (*builder).createWorldDB},
		{name: "literary_db", fn: (*builder).createLiteraryDB},
		{name: "world_meta", fn: (*builder).writeWorldMeta},
		{name: "pack", fn: (*builder).pack},
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
	return nil
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

// writeWorldMeta — 14 ключей анкеты в world_meta хэндлом forge (B-03.5, карта
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
	return nil
}

// pack — пачка из 96 карточек (T-02-04): 16 психотипов × 6 сословий в characters
// оперативной базы хэндлом core (карта писателей T-01-07). Падение здесь
// откатывает рождение (шаг 4).
func (b *builder) pack() error {
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

// ancketaValues — пары «ключ — JSON-значение» для 14 полей анкеты в порядке Fields.
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
