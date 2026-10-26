package workshop

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"

	"tns65/internal/censor"
	"tns65/internal/db"
	"tns65/internal/models"
	"tns65/internal/version"
)

// HoroscopeRole — роль слота ковки гороскопа (tns65 swarm forge-horoscope).
const HoroscopeRole = "horoscope"

// HoroscopeKind — kind записей forge_log о ковке гороскопа.
const HoroscopeKind = "horoscope"

// horoscopeLang — язык гороскопа для заводского Цензора и векторов веток.
const horoscopeLang = "ru"

// horoscopeEmbedModel — имя модели эмбеддингов в таблице vectors (D-17.2).
const horoscopeEmbedModel = "bge-m3"

// horoscopeOwnerTable — owner_table векторов веток гороскопа.
const horoscopeOwnerTable = "horoscope_issue"

// HoroscopePayload — payload заказа horoscope (шаг 1): сцена-источник спроса,
// карта игрока Пифии и прогноз Пилы.
type HoroscopePayload struct {
	SceneID  int64  `json:"scene_id"`
	Pythia   string `json:"pythia"`
	Forecast string `json:"forecast"`
}

// HoroscopeOutput — строгий JSON ковки гороскопа (шаг 2): ветки ответа и прогноз
// Пилы (W-13.2).
type HoroscopeOutput struct {
	Branches []HoroscopeBranch `json:"branches"`
	Forecast string            `json:"forecast"`
}

// HoroscopeBranch — ветка ответа гороскопа: id neg|pos|aside, kind
// bank_ref|weaver_prompt, ссылка/тема и целевая реплика-предикат.
type HoroscopeBranch struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Ref       string `json:"ref"`
	Theme     string `json:"theme"`
	ProbeText string `json:"probe_text"`
}

// Censor — словарный фильтр пакета (T-04-11); в проде censor.Check, в тестах
// подменяется через SetCensor.
type Censor interface {
	Check(text string) (bool, []string)
}

// censorAdapter — заводской Цензор ковки: единый фильтр пакета по словарю ru.
type censorAdapter struct{}

func (censorAdapter) Check(text string) (bool, []string) {
	return censor.Check(text, horoscopeLang)
}

var (
	censorMu  sync.RWMutex
	curCensor Censor

	embedMu  sync.RWMutex
	curEmbed models.Embedder
)

// SetCensor подменяет Цензора ковки (тестовая точка входа T-05-04); nil — заводской.
func SetCensor(c Censor) {
	censorMu.Lock()
	defer censorMu.Unlock()
	curCensor = c
}

// SetEmbedder подменяет эмбеддера ковки (тестовая точка входа); nil — заводской.
func SetEmbedder(e models.Embedder) {
	embedMu.Lock()
	defer embedMu.Unlock()
	curEmbed = e
}

// censorForge возвращает текущего Цензора ковки; nil заменяется заводским.
func censorForge() Censor {
	censorMu.RLock()
	defer censorMu.RUnlock()
	if curCensor != nil {
		return curCensor
	}
	return censorAdapter{}
}

// embedderForge возвращает текущего эмбеддера ковки; nil — эмбеддер не подключён.
func embedderForge() models.Embedder {
	embedMu.RLock()
	defer embedMu.RUnlock()
	return curEmbed
}

// EnsureHoroscopeOrder ставит заказ Пифии (шаг 1): по сцене, карте игрока и
// прогнозу Пилы ставится заказ ковки гороскопа (тип horoscope) в очередь хэндлом
// core. Повторная постановка того же заказа не создаёт дубль. Возвращает true,
// если заказ стоит в очереди после вызова.
func EnsureHoroscopeOrder(h *db.Handle, payload HoroscopePayload) (bool, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM workshop_queue WHERE kind = ? AND status = 'queued' AND payload_json = ?`,
		string(KindHoroscope), string(raw)).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	if err := Enqueue(h, KindHoroscope, StepHeavy, DefaultProvider, payload); err != nil {
		return false, err
	}
	return true, nil
}

// HoroscopePrompt строит промпт ковки гороскопа: сцена, карта игрока и прогноз
// Пилы (шаг 1).
func HoroscopePrompt(p HoroscopePayload) string {
	raw, _ := json.Marshal(map[string]any{
		"task":     HoroscopeRole,
		"scene_id": p.SceneID,
		"pythia":   p.Pythia,
		"forecast": p.Forecast,
	})
	return string(raw)
}

// RunHoroscopeForge — ковка гороскопа Гефестом по заказу Пифии (шаги 2–4):
// модель (слот) варит строгий JSON; каждый текст ветки проходит Цензора до записи
// (W-13.9); probe_text векторизуется эмбеддером (dim=1024) и пишется рядом для
// сверки на сцене (T-05-05). Гороскоп пишется в horoscope_issue хэндлом forge,
// сводка — в forge_log хэндлом forge, векторы веток — в vectors мира хэндлом core
// (owner_table='horoscope_issue'; shared-векторы к литературной базе не
// подключены, D-19.2). Возвращает id строки horoscope_issue.
func RunHoroscopeForge(world *db.Handle, literary *db.Handle, payload HoroscopePayload) (int64, error) {
	slot := slotForProvider(DefaultProvider)
	if slot == nil {
		return 0, errors.New("workshop: слот ковки гороскопа не подключён")
	}
	out, err := slot.Generate(context.Background(), HoroscopeRole, HoroscopePrompt(payload))
	if err != nil {
		return 0, err
	}

	var ho HoroscopeOutput
	if err := json.Unmarshal([]byte(out), &ho); err != nil {
		return 0, fmt.Errorf("workshop: выход ковки гороскопа не JSON: %w", err)
	}
	if err := validateHoroscopeOutput(ho); err != nil {
		return 0, err
	}

	// Цензура без исключений (W-13.9): тексты веток до записи.
	cen := censorForge()
	for _, b := range ho.Branches {
		for _, text := range []string{b.Ref, b.Theme, b.ProbeText} {
			if ok, hits := cen.Check(text); !ok {
				return 0, fmt.Errorf("workshop: Цензор не пропустил ветку гороскопа: %s", strings.Join(hits, ", "))
			}
		}
	}

	emb := embedderForge()
	if emb == nil {
		return 0, errors.New("workshop: эмбеддер не подключён")
	}
	type branchVector struct {
		probe string
		vec   []float32
	}
	vecs := make([]branchVector, 0, len(ho.Branches))
	for _, b := range ho.Branches {
		v, err := emb.Embed(context.Background(), b.ProbeText)
		if err != nil {
			return 0, fmt.Errorf("workshop: эмбеддинг ветки гороскопа: %w", err)
		}
		if len(v) != models.EmbedDim {
			return 0, fmt.Errorf("workshop: вектор ветки dim=%d, want %d", len(v), models.EmbedDim)
		}
		vecs = append(vecs, branchVector{probe: b.ProbeText, vec: v})
	}

	issueJSON, err := json.Marshal(ho)
	if err != nil {
		return 0, err
	}
	now := nowUTC()

	var issueID int64
	err = literary.Tx(db.WriterForge, func(tx *db.Tx) error {
		res, err := tx.Exec("horoscope_issue",
			`INSERT INTO horoscope_issue (scene_id, issue_json, created_at) VALUES (?, ?, ?)`,
			payload.SceneID, string(issueJSON), now)
		if err != nil {
			return err
		}
		issueID, err = res.LastInsertId()
		if err != nil {
			return err
		}
		_, err = tx.Exec("forge_log",
			`INSERT INTO forge_log (kind, task, status, started_at, finished_at, log_path, package_version) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			HoroscopeKind, HoroscopeRole, "done", now, now, "", version.Version)
		return err
	})
	if err != nil {
		return 0, err
	}

	err = world.Tx(db.WriterCore, func(tx *db.Tx) error {
		for _, bv := range vecs {
			if _, err := tx.Exec("vectors",
				`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				horoscopeOwnerTable, issueID, horoscopeEmbedModel, models.EmbedDim, horoscopeLang, bv.probe, horoscopeVecBlob(bv.vec), now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	return issueID, nil
}

// validateHoroscopeOutput проверяет строгий JSON ковки гороскопа: прогноз Пилы и
// ветки на месте, у ветки допустимые id/kind и непустая целевая реплика.
func validateHoroscopeOutput(ho HoroscopeOutput) error {
	if ho.Forecast != "echo" && ho.Forecast != "contrapunt" {
		return fmt.Errorf("workshop: прогноз Пилы вне echo|contrapunt: %q", ho.Forecast)
	}
	if len(ho.Branches) == 0 {
		return errors.New("workshop: гороскоп без веток")
	}
	for _, b := range ho.Branches {
		if b.ID != "neg" && b.ID != "pos" && b.ID != "aside" {
			return fmt.Errorf("workshop: id ветки вне neg|pos|aside: %q", b.ID)
		}
		if b.Kind != "bank_ref" && b.Kind != "weaver_prompt" {
			return fmt.Errorf("workshop: kind ветки вне bank_ref|weaver_prompt: %q", b.Kind)
		}
		if b.ProbeText == "" {
			return errors.New("workshop: ветка без probe_text")
		}
	}
	return nil
}

// horoscopeVecBlob кодирует вектор ветки в BLOB Float32 little-endian (D-17.2).
func horoscopeVecBlob(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, f := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}
