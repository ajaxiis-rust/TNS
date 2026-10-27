package workshop

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"tns65/internal/db"
	"tns65/internal/sediment"
)

// EpochRole — роль слота ковки справочника эпохи (W-19.2).
const EpochRole = "epoch"

// epochKind — kind записи справочника в bank_pool: реалии эпохи — рутина
// повседневности мира (CHECK bank_pool.kind допускает routine|drama|fallback).
const epochKind = "routine"

// epochLang — язык справочника эпохи (заводской Цензор ковки — ru).
const epochLang = "ru"

// epochTag — тег записи справочника эпохи в bank_pool: по нему Летописец читает
// реалии из банка (W-19.5), а не из головы модели.
const epochTag = "epoch"

// epochOriginPrefix — префикс origin записи справочника: одна эпоха — одна запись.
const epochOriginPrefix = "epoch:"

// worldDescriptionKey — ключ world_meta с описанием мира (B-01, T-02-03).
const worldDescriptionKey = "description"

// ResearchSource — источник материала ковки справочника эпохи (W-19.2): по теме
// эпохи возвращает материал реалий. Локальная реализация — LocalSource; внешняя
// (разведка Паганеля) подключается в P5 (T-06-10), до неё внешний источник не
// используется.
type ResearchSource interface {
	Fetch(ctx context.Context, topic string) (string, error)
}

// LocalSource — локальная реализация ResearchSource (шаг 2): материал справочника
// собирается из описания мира и книг-источников без внешних вызовов.
type LocalSource struct {
	description string
	books       []string
}

// NewLocalSource собирает локальный источник из описания мира world_meta и
// книг-источников. Отказ чтения описания — ошибка: эпоха без описания не куётся.
func NewLocalSource(world *db.Handle, books []string) (*LocalSource, error) {
	desc, err := readWorldMetaString(world, worldDescriptionKey)
	if err != nil {
		return nil, err
	}
	return &LocalSource{description: desc, books: books}, nil
}

// Fetch возвращает материал эпохи: тему, описание мира и книги-источники.
func (s *LocalSource) Fetch(_ context.Context, topic string) (string, error) {
	var b strings.Builder
	if topic = strings.TrimSpace(topic); topic != "" {
		b.WriteString(topic)
		b.WriteString("\n")
	}
	if desc := strings.TrimSpace(s.description); desc != "" {
		b.WriteString(desc)
		b.WriteString("\n")
	}
	for _, book := range s.books {
		if book = strings.TrimSpace(book); book != "" {
			b.WriteString(book)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String()), nil
}

// EpochFromWorld читает описание мира из world_meta и выводит из него эпоху (W-19.1).
func EpochFromWorld(world *db.Handle) (string, error) {
	desc, err := readWorldMetaString(world, worldDescriptionKey)
	if err != nil {
		return "", err
	}
	return DeriveEpoch(desc), nil
}

// DeriveEpoch — детерминированный вывод эпохи из описания мира (W-19.1): первое
// предложение описания — краткая характеристика эпохи. Модель в выводе не участвует.
func DeriveEpoch(description string) string {
	desc := strings.TrimSpace(description)
	if i := strings.IndexAny(desc, ".!?…"); i >= 0 {
		return strings.TrimSpace(desc[:i])
	}
	return desc
}

// ForgeEpoch — ковка справочника эпохи (шаги 1–5): эпоха выводится из описания
// мира, материал собирается через ResearchSource, модель кует справочник, он
// проходит авто-критик и Цензора, запись пишется в bank_pool хэндлом forge с
// уровнем доверия банка-ресурса. source == nil — локальная реализация (шаг 2).
// Повторная ковка той же эпохи не создаёт дубль.
func ForgeEpoch(ctx context.Context, world *db.Handle, lit *db.Handle, source ResearchSource) (int64, error) {
	epoch, err := EpochFromWorld(world)
	if err != nil {
		return 0, err
	}
	if epoch == "" {
		return 0, errors.New("workshop: эпоха не выведена из описания мира")
	}

	if id, ok, err := findEpochRecord(lit, epoch); err != nil {
		return 0, err
	} else if ok {
		return id, nil
	}

	if source == nil {
		source, err = NewLocalSource(world, nil)
		if err != nil {
			return 0, err
		}
	}
	material, err := source.Fetch(ctx, epoch)
	if err != nil {
		return 0, err
	}

	slot := slotForProvider(DefaultProvider)
	if slot == nil {
		return 0, errors.New("workshop: слот ковки справочника эпохи не подключён")
	}
	reference, err := slot.Generate(ctx, EpochRole, EpochPrompt(epoch, material))
	if err != nil {
		return 0, err
	}

	// Авто-критик (W-19.3): справочник проходит ревью, текст моделью не меняется.
	rec := sediment.Autocritic(ctx, slot, nil, sediment.SedimentRecord{CleanedText: reference})
	if rec.Verdict == sediment.VerdictDegraded {
		return 0, errors.New("workshop: авто-критик не провёл ревью справочника эпохи")
	}

	// Цензор (W-19.3): как любая запись банка — до записи.
	if ok, hits := censorForge().Check(reference); !ok {
		return 0, fmt.Errorf("%w: %s", ErrCensorReject, strings.Join(hits, ", "))
	}

	return writeEpochRecord(lit, epoch, reference)
}

// EpochPrompt строит промпт ковки справочника эпохи: тема эпохи и материал источника.
func EpochPrompt(epoch, material string) string {
	raw, _ := json.Marshal(map[string]any{
		"task":     EpochRole,
		"epoch":    epoch,
		"material": material,
	})
	return string(raw)
}

// writeEpochRecord пишет справочник эпохи в bank_pool хэндлом forge (шаг 5):
// trust_level как у банка-ресурса (проверенный, поднятый ковкой).
func writeEpochRecord(lit *db.Handle, epoch, text string) (int64, error) {
	var id int64
	err := lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		res, err := tx.Exec("bank_pool",
			`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			epochKind, text, epochLang, epochTag, sediment.TrustLevelVerified, epochOrigin(epoch), nowUTC())
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// findEpochRecord — уже собранная запись справочника эпохи; false — записи нет.
func findEpochRecord(lit *db.Handle, epoch string) (int64, bool, error) {
	var id int64
	err := lit.QueryRow(`SELECT id FROM bank_pool WHERE origin = ?`, epochOrigin(epoch)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// EpochReference — запись справочника эпохи из банка, вход для Летописца (W-19.5).
type EpochReference struct {
	ID         int64
	Epoch      string
	Text       string
	TrustLevel int
}

// ReadEpochReference читает реалии эпохи из bank_pool по тегу справочника (W-19.5):
// Летописец берёт реалии из банка, а не из головы модели.
func ReadEpochReference(lit *db.Handle) ([]EpochReference, error) {
	rows, err := lit.Query(`SELECT id, origin, text, trust_level FROM bank_pool WHERE tags = ? ORDER BY id`, epochTag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EpochReference
	for rows.Next() {
		var e EpochReference
		var origin string
		if err := rows.Scan(&e.ID, &origin, &e.Text, &e.TrustLevel); err != nil {
			return nil, err
		}
		e.Epoch = strings.TrimPrefix(origin, epochOriginPrefix)
		out = append(out, e)
	}
	return out, rows.Err()
}

// epochOrigin — origin записи справочника эпохи.
func epochOrigin(epoch string) string {
	return epochOriginPrefix + epoch
}

// readWorldMetaString читает JSON-строковое значение world_meta по ключу
// (T-02-03, шаг 3: значения world_meta хранятся JSON-строками).
func readWorldMetaString(world *db.Handle, key string) (string, error) {
	if world == nil {
		return "", errors.New("workshop: нет хэндла мира для чтения world_meta")
	}
	var raw string
	err := world.QueryRow(`SELECT value FROM world_meta WHERE "key" = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("workshop: world_meta.%s не найден", key)
	}
	if err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return "", fmt.Errorf("workshop: разбор world_meta.%s: %w", key, err)
	}
	return s, nil
}
