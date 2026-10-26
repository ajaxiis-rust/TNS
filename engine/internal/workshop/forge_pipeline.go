package workshop

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"tns65/internal/db"
	"tns65/internal/sediment"
	"tns65/internal/version"
)

// ErrCensorReject — Цензор при варке не пропустил выход ковки; в банк запись не
// попадает (W-14.4, шаг 2).
var ErrCensorReject = errors.New("workshop: Цензор не пропустил запись банка")

// ErrRemeltNotUnchecked — переплавить можно только осевшую запись с
// trust_level=1 (шаг 4); уже проверенная запись Гефесту не нужна.
var ErrRemeltNotUnchecked = errors.New("workshop: переплавка только записи trust_level=1")

// factCheckTask — task строки forge_log о расхождении фактов (шаг 1).
const factCheckTask = "fact_check"

// remeltKind — kind слотной заготовки bank_pool при переплавке осевшей сцены.
const remeltKind = "drama"

// remeltLang — язык переплавленной заготовки.
const remeltLang = "ru"

// ForgeMaterial — разбираемый материал ковки (чанк книги): сырой текст выхода
// ковки и источники замен — факты и привязки биндера (шаг 1).
type ForgeMaterial struct {
	// Origin — ключ идемпотентности ковки: одна ковка — одна запись банка.
	Origin string
	// SourceSceneID — сцена-источник материала; 0 — источника нет.
	SourceSceneID int64
	// Kind — bank_pool.kind: routine|drama|fallback.
	Kind string
	Lang string
	Tags string
	// Text — сырой текст выхода ковки до делексификации.
	Text     string
	Facts    []sediment.Fact
	Bindings []sediment.Binding
}

// Delexify — делексификация выхода ковки (шаг 1): источник замен — только факты
// и привязки разбираемого материала; все вхождения значения заменяются
// слот-токеном набора T-04-01, значение без источника не заменяется. Возвращает
// очищенный текст и использованные слот-токены в порядке реестра.
func Delexify(sourceSceneID int64, text string, facts []sediment.Fact, bindings []sediment.Binding) (string, []string, error) {
	rec, err := sediment.Clean(sediment.Scene{
		ID:     sourceSceneID,
		Status: sediment.SceneStatusClosed,
		Prose:  text,
		Facts:  facts,
	}, bindings)
	if err != nil {
		return "", nil, fmt.Errorf("workshop: делексификация: %w", err)
	}
	return rec.CleanedText, rec.Slots, nil
}

// ForgeBankRecord — конвейер ковки записи банка (шаги 1–3): идемпотентность по
// origin, делексификация, проверка фактов, Цензор на выходе и сборка записи.
// Повторная выдача той же записи (тот же origin) Цензора заново не запускает.
func ForgeBankRecord(world *db.Handle, lit *db.Handle, m ForgeMaterial) (int64, error) {
	if m.Origin == "" {
		return 0, errors.New("workshop: ковка без origin")
	}

	// Шаг 3: банк чистится один раз при ковке — повторная выдача записи
	// цензуру заново не запускает.
	if id, ok, err := findBankByOrigin(lit, m.Origin); err != nil {
		return 0, err
	} else if ok {
		return id, nil
	}

	text, _, err := Delexify(m.SourceSceneID, m.Text, m.Facts, m.Bindings)
	if err != nil {
		return 0, err
	}

	if _, err := FactCheck(world, lit, m.Facts); err != nil {
		return 0, err
	}

	// Шаг 2: Цензор при варке — на выходе ковки, до записи в банк.
	if ok, hits := censorForge().Check(text); !ok {
		return 0, fmt.Errorf("%w: %s", ErrCensorReject, strings.Join(hits, ", "))
	}

	return writeBankRecord(lit, m, text)
}

// FactCheck — проверка фактов (шаг 1): сверка имён и мест материала с миром —
// имя по characters, место по places. Каждое расхождение помечается строкой
// forge_log; расхождение ковку не останавливает. Возвращает список расхождений.
func FactCheck(world *db.Handle, lit *db.Handle, facts []sediment.Fact) ([]string, error) {
	var mismatches []string
	for _, f := range facts {
		var known bool
		var err error
		switch f.Token {
		case sediment.TokenName:
			known, err = worldHasName(world, f.Value)
		case sediment.TokenPlace:
			known, err = worldHasPlace(world, f.Value)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		if !known {
			mismatches = append(mismatches, f.Value)
		}
	}
	if len(mismatches) > 0 {
		if err := writeFactMismatchLog(lit, mismatches); err != nil {
			return nil, err
		}
	}
	return mismatches, nil
}

// RemeltScene переплавляет осевшую запись sediment_scene с trust_level=1 в
// слотную заготовку bank_pool (шаг 4). Гефест поднимает trust_level до 2
// (проверенного) — единственный путь наверх шкалы. Повторная переплавка той же
// сцены не создаёт дубль.
func RemeltScene(lit *db.Handle, sceneID int64) (int64, error) {
	var cleaned string
	var trust int
	var sourceSceneID int64
	err := lit.QueryRow(
		`SELECT cleaned_text, trust_level, source_scene_id FROM sediment_scene WHERE id = ?`,
		sceneID,
	).Scan(&cleaned, &trust, &sourceSceneID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("workshop: sediment_scene %d не найдена", sceneID)
	}
	if err != nil {
		return 0, err
	}
	if trust != sediment.TrustLevelUnchecked {
		return 0, fmt.Errorf("%w: trust_level=%d", ErrRemeltNotUnchecked, trust)
	}

	origin := fmt.Sprintf("sediment:%d", sceneID)
	if id, ok, err := findBankByOrigin(lit, origin); err != nil {
		return 0, err
	} else if ok {
		return id, nil
	}

	var id int64
	err = lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		var source any
		if sourceSceneID != 0 {
			source = sourceSceneID
		}
		res, err := tx.Exec("bank_pool",
			`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, source_scene_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			remeltKind, cleaned, remeltLang, "", sediment.TrustLevelVerified, origin, source, nowUTC())
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

// writeBankRecord собирает и пишет запись банка хэндлом forge (шаг 1, сборка).
// Выкованная запись — проверенная (trust_level=2), конкретных имён в тексте нет.
func writeBankRecord(lit *db.Handle, m ForgeMaterial, text string) (int64, error) {
	var id int64
	err := lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		var source any
		if m.SourceSceneID != 0 {
			source = m.SourceSceneID
		}
		res, err := tx.Exec("bank_pool",
			`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, source_scene_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			m.Kind, text, m.Lang, m.Tags, sediment.TrustLevelVerified, m.Origin, source, nowUTC())
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

// findBankByOrigin — уже собранная запись того же origin; false — записи нет.
func findBankByOrigin(lit *db.Handle, origin string) (int64, bool, error) {
	var id int64
	err := lit.QueryRow(`SELECT id FROM bank_pool WHERE origin = ?`, origin).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// worldHasName сообщает, есть ли имя в characters мира.
func worldHasName(world *db.Handle, name string) (bool, error) {
	var n int
	if err := world.QueryRow(`SELECT COUNT(*) FROM characters WHERE name = ?`, name).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// worldHasPlace сообщает, есть ли место в places мира.
func worldHasPlace(world *db.Handle, name string) (bool, error) {
	var n int
	if err := world.QueryRow(`SELECT COUNT(*) FROM places WHERE name = ?`, name).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// writeFactMismatchLog помечает расхождения фактов строками forge_log (шаг 1).
func writeFactMismatchLog(lit *db.Handle, mismatches []string) error {
	now := nowUTC()
	return lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		for range mismatches {
			if _, err := tx.Exec("forge_log",
				`INSERT INTO forge_log (kind, task, status, started_at, finished_at, log_path, package_version) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				ForgeBookKind, factCheckTask, "mismatch", now, now, "", version.Version); err != nil {
				return err
			}
		}
		return nil
	})
}
