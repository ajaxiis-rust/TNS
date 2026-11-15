package deepread

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tns65/internal/db"
	"tns65/internal/models"
	"tns65/internal/version"
)

// DictVersionLen — длина dict_version: первые 12 символов sha256 манифеста
// пака (§3 плана волны 14).
const DictVersionLen = 12

// KindDeepread — kind строк forge_log о разборе книги (писатель forge).
const KindDeepread = "deepread"

// ReasonUnchangedKey — причина пропуска разбора: ключ пересчёта не изменился
// (D-20.3, T-14-11 шаг 2).
const ReasonUnchangedKey = "неизменный ключ"

// statusDone — статус завершённого прогона в forge_log.
const statusDone = "done"

// Key — ключ пересчёта разбора (D-20.3): хеш текста книги, версия словарей
// разбора и модель эмбеддинга. Неизменный ключ разбор не пересчитывает.
type Key struct {
	TextHash    string `json:"text_hash"`
	DictVersion string `json:"dict_version"`
	EmbedModel  string `json:"embed_model"`
}

// Valid — все три элемента ключа заполнены.
func (k Key) Valid() bool {
	return k.TextHash != "" && k.DictVersion != "" && k.EmbedModel != ""
}

// Equal сравнивает ключи поэлементно.
func (k Key) Equal(other Key) bool {
	return k == other
}

// String — ключ для сообщений: элементы через «/».
func (k Key) String() string {
	return k.TextHash + "/" + k.DictVersion + "/" + k.EmbedModel
}

// DictVersion — версия словарей разбора: первые DictVersionLen символов
// sha256 файла manifest.json каталога пака (§3 плана волны 14).
func DictVersion(packDir string) (string, error) {
	f, err := os.Open(filepath.Join(packDir, "manifest.json"))
	if err != nil {
		return "", fmt.Errorf("deepread: версия словарей: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("deepread: версия словарей: %w", err)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	return sum[:DictVersionLen], nil
}

// TextHash — sha256 текста книги в hex, тот же вид, что book_chunk.text_hash.
func TextHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// LoadEmbedModel читает имя модели слота embed из config.yaml (llm.embed_model);
// пустое значение — models.DefaultEmbedModel (bge-m3), как у клиента эмбеддингов.
func LoadEmbedModel(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return parseEmbedModel(string(data))
}

// parseEmbedModel достаёт embed_model из секции llm; прочие ключи не читаются.
func parseEmbedModel(data string) (string, error) {
	model := ""
	inLLM := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inLLM = trimmed == "llm:"
			continue
		}
		if !inLLM || indent != 2 {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok || strings.TrimSpace(key) != "embed_model" {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		model = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	if model == "" {
		return models.DefaultEmbedModel, nil
	}
	return model, nil
}

// NewKey собирает ключ пересчёта разбора книги из каталога пака, файла
// конфигурации и текста книги (§3, D-20.3).
func NewKey(packDir, configPath, bookText string) (Key, error) {
	dictVersion, err := DictVersion(packDir)
	if err != nil {
		return Key{}, err
	}
	embedModel, err := LoadEmbedModel(configPath)
	if err != nil {
		return Key{}, fmt.Errorf("deepread: модель эмбеддинга: %w", err)
	}
	return Key{TextHash: TextHash(bookText), DictVersion: dictVersion, EmbedModel: embedModel}, nil
}

// RunRecord — запись forge_log о завершённом прогоне разбора: ключ пересчёта и
// число чанков; дата — колонки started_at/finished_at строки. Колонок под ключ
// в forge_log нет, поэтому запись хранится JSON-объектом в task.
type RunRecord struct {
	Key    Key `json:"key"`
	Chunks int `json:"chunks"`
}

// RunOutcome — итог прогона разбора: пропуск по неизменному ключу либо число
// разобранных чанков.
type RunOutcome struct {
	Skipped bool
	Reason  string
	Chunks  int
}

// FindRun ищет в журнале ковки последнюю строку kind='deepread' со статусом
// done и тем же ключом (T-14-11, шаг 2). Строки без ключа (сводки) пропускаются.
func FindRun(lit *db.Handle, key Key) (RunRecord, bool, error) {
	if lit == nil {
		return RunRecord{}, false, ErrNoStore
	}
	if !key.Valid() {
		return RunRecord{}, false, fmt.Errorf("deepread: ключ разбора неполон: %s", key)
	}

	rows, err := lit.Query(
		`SELECT task FROM forge_log WHERE kind = ? AND status = ? ORDER BY id DESC`,
		KindDeepread, statusDone)
	if err != nil {
		return RunRecord{}, false, fmt.Errorf("deepread: журнал разбора: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var task string
		if err := rows.Scan(&task); err != nil {
			return RunRecord{}, false, fmt.Errorf("deepread: журнал разбора: %w", err)
		}
		var rec RunRecord
		if err := json.Unmarshal([]byte(task), &rec); err != nil {
			continue
		}
		if rec.Key.Equal(key) {
			return rec, true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return RunRecord{}, false, fmt.Errorf("deepread: журнал разбора: %w", err)
	}
	return RunRecord{}, false, nil
}

// WriteRunLog пишет строку forge_log kind='deepread' о завершённом прогоне:
// ключ пересчёта и число чанков (T-14-11, шаг 3). Пустой now — текущее UTC.
func WriteRunLog(lit *db.Handle, key Key, chunks int, now string) error {
	if lit == nil {
		return ErrNoStore
	}
	if !key.Valid() {
		return fmt.Errorf("deepread: ключ разбора неполон: %s", key)
	}
	if chunks < 0 {
		return fmt.Errorf("deepread: число чанков разбора %d", chunks)
	}
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	task, err := json.Marshal(RunRecord{Key: key, Chunks: chunks})
	if err != nil {
		return fmt.Errorf("deepread: запись журнала разбора: %w", err)
	}

	return lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		_, err := tx.Exec("forge_log",
			`INSERT INTO forge_log (kind, task, status, started_at, finished_at, log_path, package_version) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			KindDeepread, string(task), statusDone, now, now, "", version.Version)
		return err
	})
}

// RunOnce — гейт идемпотентности разбора (D-20.3, T-14-11): прогон с ключом,
// уже отмеченным в журнале ковки статусом done, пропускается с причиной
// «неизменный ключ»; иначе выполняется run и по завершении пишется строка
// журнала с ключом и числом чанков. Смена любого элемента ключа — новый прогон.
func RunOnce(lit *db.Handle, key Key, run func() (int, error)) (RunOutcome, error) {
	if run == nil {
		return RunOutcome{}, fmt.Errorf("deepread: прогон разбора без функции")
	}
	rec, done, err := FindRun(lit, key)
	if err != nil {
		return RunOutcome{}, err
	}
	if done {
		return RunOutcome{Skipped: true, Reason: ReasonUnchangedKey, Chunks: rec.Chunks}, nil
	}

	chunks, err := run()
	if err != nil {
		return RunOutcome{}, err
	}
	if err := WriteRunLog(lit, key, chunks, ""); err != nil {
		return RunOutcome{}, err
	}
	return RunOutcome{Chunks: chunks}, nil
}
