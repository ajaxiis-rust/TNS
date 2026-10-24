package birth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/langpack"
)

// seedBankLine — строка сид-банка packs/<lang>/versions/<version>/bank_*.jsonl
// (T-02-13, шаг 1): банк рутины, драмы и страховочный fallback для Ткача.
type seedBankLine struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
	Tags string `json:"tags"`
	Lang string `json:"lang"`
}

// seedTemplateLine — строка словарного шаблона packs/<lang>/lit_templates.jsonl
// (T-02-13, шаг 1): поля те же, что у таблицы lit_template.
type seedTemplateLine struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
	Tags string `json:"tags"`
	Lang string `json:"lang"`
}

// seedKind — kind записи forge_log о наполнении сида (T-02-13, шаг 3).
const seedKind = "seed"

// seedOrigin — origin записей bank_pool, пришедших из сида пакета (D-04.3).
const seedOrigin = "seed"

// seedLiteraryBase — шаг цепочки рождения: наполнение литературной базы сидом
// выбранного языкового пакета (T-02-13).
func (b *builder) seedLiteraryBase() error {
	return seedLiteraryBase(b.literaryDB, filepath.Join(b.packsRoot, b.a.Lang), b.logger)
}

// seedLiteraryBase читает сид-банки пакета packDir и словарные шаблоны и пишет
// их в литературную базу literaryDB хэндлом forge: bank_pool и lit_template с
// trust_level=2, в forge_log отметка kind='seed' с путём к пакету и версией.
// Повторное наполнение того же мира пропускается (запись seed уже есть).
func seedLiteraryBase(literaryDB, packDir string, logger *journal.Logger) error {
	h, err := db.OpenWriterAs(literaryDB, db.DBLiterary, db.WriterForge, logger)
	if err != nil {
		return err
	}
	defer h.Close()

	seeded, err := hasSeedRecord(h)
	if err != nil {
		return err
	}
	if seeded {
		return nil
	}

	// Пакет отсутствует — наполнять нечем (юнит-тесты рождения не заводят пакет).
	if _, err := os.Stat(packDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	version, err := currentPackVersion(packDir)
	if err != nil {
		return err
	}
	lines, err := readSeedBanks(packDir, version)
	if err != nil {
		return err
	}
	templates, err := readTemplates(packDir)
	if err != nil {
		return err
	}

	// Пакет без сид-банков и шаблонов: наполнять нечего, рождение не ломаем.
	if len(lines) == 0 && len(templates) == 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	return h.Tx(db.WriterForge, func(tx *db.Tx) error {
		for _, l := range lines {
			if _, err := tx.Exec("bank_pool",
				`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at) VALUES (?, ?, ?, ?, 2, ?, ?)`,
				l.Kind, l.Text, l.Lang, l.Tags, seedOrigin, now); err != nil {
				return err
			}
		}
		for _, t := range templates {
			if _, err := tx.Exec("lit_template",
				`INSERT INTO lit_template (kind, text, lang, tags) VALUES (?, ?, ?, ?)`,
				t.Kind, t.Text, t.Lang, t.Tags); err != nil {
				return err
			}
		}
		_, err := tx.Exec("forge_log",
			`INSERT INTO forge_log (kind, task, status, started_at, finished_at, log_path, package_version) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			seedKind, seedKind, "done", now, now, packDir, version)
		return err
	})
}

// hasSeedRecord — запись наполнения сида уже есть в forge_log.
func hasSeedRecord(h *db.Handle) (bool, error) {
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM forge_log WHERE kind = ?`, seedKind).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// currentPackVersion — текущая версия пакета: указатель current, иначе version
// из manifest.json (T-01-08, шаг 3).
func currentPackVersion(packDir string) (string, error) {
	if raw, err := os.ReadFile(filepath.Join(packDir, "current")); err == nil {
		if v := strings.TrimSpace(string(raw)); v != "" {
			return v, nil
		}
	}
	m, err := langpack.LoadManifest(filepath.Join(packDir, "manifest.json"))
	if err != nil {
		return "", fmt.Errorf("birth: манифест пакета %s: %w", packDir, err)
	}
	if m.Version == "" {
		return "", fmt.Errorf("birth: пакет %s без версии", packDir)
	}
	return m.Version, nil
}

// readSeedBanks читает все файлы versions/<version>/bank_*.jsonl пакета.
func readSeedBanks(packDir, version string) ([]seedBankLine, error) {
	paths, err := filepath.Glob(filepath.Join(packDir, "versions", version, "bank_*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	var out []seedBankLine
	for _, p := range paths {
		lines, err := readJSONLines[seedBankLine](p)
		if err != nil {
			return nil, err
		}
		out = append(out, lines...)
	}
	return out, nil
}

// readTemplates читает словарные шаблоны lit_templates.jsonl; файла нет — пусто.
func readTemplates(packDir string) ([]seedTemplateLine, error) {
	lines, err := readJSONLines[seedTemplateLine](filepath.Join(packDir, "lit_templates.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return lines, nil
}

// readJSONLines читает файл построчно как JSON-записи; пустые строки пропущены.
func readJSONLines[T any](path string) ([]T, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []T
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var v T
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			return nil, fmt.Errorf("birth: %s: %w", path, err)
		}
		out = append(out, v)
	}
	return out, nil
}
