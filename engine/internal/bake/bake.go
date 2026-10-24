// Package bake запекает сид (D-04): внеигровой инструмент, который один раз
// готовит замороженную base.db с базовыми нарративами и их векторами смысла.
package bake

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tns65/internal/db"
	"tns65/internal/netgate"
)

// ErrAlreadyBaked — сид уже запечён: повторный запуск запрещён (D-03.2).
var ErrAlreadyBaked = errors.New("bake: seed already baked")

// ErrNoNarratives — пустой сид: без нарративов якорь невозможен (B-10).
var ErrNoNarratives = errors.New("bake: seed has no narratives")

// embeddingDim — размерность вектора смысла (D-16.2).
const embeddingDim = 1024

const (
	seedLang = "ru"     // язык сида.
	model    = "bge-m3" // эмбеддинг-модель текущей реализации (D-16.1).
)

// Config — параметры запекания.
type Config struct {
	DBPath   string // путь к base.db.
	SeedDir  string // каталог сида: narratives/ и verses.txt.
	EmbedURL string // адрес эмбеддера (--embed-url).
}

// narrative — один нарратив сида: имя, резюме дуги и его стихи.
type narrative struct {
	name       string
	arcSummary string
	verses     []string
}

// Bake запекает сид один раз в cfg.DBPath. Повторный вызов на уже существующей
// базе возвращает ErrAlreadyBaked (код 5 в cmd/tns65-bake).
func Bake(cfg Config) error {
	if cfg.DBPath == "" {
		return errors.New("bake: empty DB path")
	}
	if _, err := os.Stat(cfg.DBPath); err == nil {
		return ErrAlreadyBaked
	}

	narratives, err := loadSeed(cfg.SeedDir)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return fmt.Errorf("bake: mkdir %s: %w", filepath.Dir(cfg.DBPath), err)
	}

	// Схему ставит мигратор db (base + общий shared/0002_vectors): он пишет
	// schema_migrations и PRAGMA user_version. Данные ниже пишутся через хэндл
	// с писателем bake (карта писателей T-01-07).
	raw, err := db.OpenWriter(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("bake: open writer: %w", err)
	}
	if err := db.Migrate(raw, "base", "shared"); err != nil {
		_ = raw.Close()
		return fmt.Errorf("bake: migrate: %w", err)
	}
	if err := raw.Close(); err != nil {
		return fmt.Errorf("bake: close writer: %w", err)
	}

	if err := writeSeed(cfg, narratives); err != nil {
		return err
	}

	return freeze(cfg.DBPath)
}

// writeSeed пишет нарративы, их векторы и стихи через хэндл писателя bake.
func writeSeed(cfg Config, narratives []narrative) error {
	h, err := db.OpenWriterAs(cfg.DBPath, db.DBBase, db.WriterBake, nil)
	if err != nil {
		return fmt.Errorf("bake: open handle: %w", err)
	}
	defer h.Close()

	client, err := embedderClient(cfg.EmbedURL)
	if err != nil {
		return fmt.Errorf("bake: embed url: %w", err)
	}

	created := time.Now().UTC().Format(time.RFC3339)
	for _, n := range narratives {
		res, err := h.Exec("narratives",
			`INSERT INTO narratives (name, arc_summary, lang, created_at) VALUES (?, ?, ?, ?)`,
			n.name, n.arcSummary, seedLang, created)
		if err != nil {
			return fmt.Errorf("bake: insert narrative %q: %w", n.name, err)
		}
		nid, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("bake: narrative %q id: %w", n.name, err)
		}

		vec, err := embed(client, cfg.EmbedURL, n.arcSummary)
		if err != nil {
			return fmt.Errorf("bake: embed %q: %w", n.name, err)
		}
		if _, err := h.Exec("vectors",
			`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			"narratives", nid, model, embeddingDim, seedLang, n.arcSummary, vec, created); err != nil {
			return fmt.Errorf("bake: insert vector %q: %w", n.name, err)
		}

		for i, v := range n.verses {
			if _, err := h.Exec("verses",
				`INSERT INTO verses (narrative_id, text, lang, ord) VALUES (?, ?, ?, ?)`,
				nid, v, seedLang, i+1); err != nil {
				return fmt.Errorf("bake: insert verse %q: %w", n.name, err)
			}
		}
	}
	return nil
}

// loadSeed читает verses.txt и файлы narratives/ и собирает список нарративов.
func loadSeed(seedDir string) ([]narrative, error) {
	verses, err := readVerses(filepath.Join(seedDir, "verses.txt"))
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(filepath.Join(seedDir, "narratives"))
	if err != nil {
		return nil, fmt.Errorf("bake: read narratives dir: %w", err)
	}
	var narratives []narrative
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		n, err := parseNarrative(filepath.Join(seedDir, "narratives", e.Name()), verses)
		if err != nil {
			return nil, err
		}
		narratives = append(narratives, n)
	}
	if len(narratives) == 0 {
		return nil, ErrNoNarratives
	}
	return narratives, nil
}

// readVerses читает стихи сида: одна строка — один стих, пустые строки пропущены.
func readVerses(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bake: read verses.txt: %w", err)
	}
	var verses []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			verses = append(verses, line)
		}
	}
	return verses, nil
}

// parseNarrative разбирает файл нарратива: ровно одно name и ровно одно
// arc_summary, список стихов — ссылки на строки verses.txt (с 1).
func parseNarrative(path string, verses []string) (narrative, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return narrative{}, fmt.Errorf("bake: read narrative %s: %w", path, err)
	}
	var n narrative
	nameCount, arcCount := 0, 0
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "name:"):
			nameCount++
			n.name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		case strings.HasPrefix(line, "arc_summary:"):
			arcCount++
			n.arcSummary = strings.TrimSpace(strings.TrimPrefix(line, "arc_summary:"))
		case strings.HasPrefix(line, "verses:"):
			for _, r := range strings.Fields(strings.TrimPrefix(line, "verses:")) {
				idx, err := strconv.Atoi(r)
				if err != nil {
					return narrative{}, fmt.Errorf("bake: %s: bad verse ref %q", path, r)
				}
				if idx < 1 || idx > len(verses) {
					return narrative{}, fmt.Errorf("bake: %s: verse ref %d out of range 1..%d", path, idx, len(verses))
				}
				n.verses = append(n.verses, verses[idx-1])
			}
		}
	}
	if nameCount != 1 {
		return narrative{}, fmt.Errorf("bake: %s: exactly one name required, got %d", path, nameCount)
	}
	if arcCount != 1 {
		return narrative{}, fmt.Errorf("bake: %s: exactly one arc_summary required, got %d", path, arcCount)
	}
	return n, nil
}

// embedderClient — HTTP-клиент к эмбеддеру через loopback-хук сетевого гейта
// (:5002 и httptest в тестах; наружный трафик гейтом не разрешён).
func embedderClient(embedURL string) (*http.Client, error) {
	u, err := url.Parse(embedURL)
	if err != nil {
		return nil, err
	}
	return netgate.Local(u.Host), nil
}

type embedRequest struct {
	Input string `json:"input"`
}

type embedResponse struct {
	Embedding []float64 `json:"embedding"`
}

// embed вычисляет вектор смысла текста: POST к эмбеддеру, ответ — 1024 числа.
// Вектор строится по резюме дуги, а не как среднее векторов частей (D-02.3).
func embed(client *http.Client, embedURL, text string) ([]byte, error) {
	body, err := json.Marshal(embedRequest{Input: text})
	if err != nil {
		return nil, err
	}
	resp, err := client.Post(embedURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedder status %d", resp.StatusCode)
	}
	var out embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Embedding) != embeddingDim {
		return nil, fmt.Errorf("embedder returned dim %d, want %d", len(out.Embedding), embeddingDim)
	}
	buf := make([]byte, len(out.Embedding)*4)
	for i, f := range out.Embedding {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(float32(f)))
	}
	return buf, nil
}

// freeze записывает рядом с базой её sha256: заморозка — далее база открывается
// только на чтение (query_only), сумма сверяет целостность артефакта.
func freeze(dbPath string) error {
	sum, err := sha256File(dbPath)
	if err != nil {
		return err
	}
	return os.WriteFile(dbPath+".sha256", []byte(hex.EncodeToString(sum)+"\n"), 0o644)
}

func sha256File(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("bake: open %s for checksum: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, fmt.Errorf("bake: checksum %s: %w", path, err)
	}
	return h.Sum(nil), nil
}
