package sediment

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"tns65/internal/db"
)

// Шкала доверия литературной базы (T-01-05, комментарий DDL): 0 снятый,
// 1 непроверенный (оседание), 2 проверенный (сид и подъём Гефестом).
const (
	TrustLevelSettled   = 0
	TrustLevelUnchecked = 1
	TrustLevelVerified  = 2
)

// ErrCensorReject — Цензор не пропустил cleaned_text, строка не создаётся.
var ErrCensorReject = errors.New("sediment: цензор не пропустил запись")

// ErrTrustEscalation — писатель оседания не может выставить trust_level=2:
// до проверенного поднимает Гефест при переплавке (T-05-07).
var ErrTrustEscalation = errors.New("sediment: trust_level=2 выставляет только Гефест")

// Censor — словарный фильтр пакета (T-04-11); в проде censor.Checker, в тестах
// подменяется.
type Censor interface {
	Check(text string) (bool, []string)
}

// Write записывает осевшую сцену в литературную базу (T-04-05, шаги 2–4). Перед
// записью прогоняет cleaned_text через Цензор: непрошедшее — отказ, строка не
// создаётся. Писатель оседания пишет trust_level=1; trust_level=2 он выставить не
// может (ErrTrustEscalation). Строка связывается с vector_id и source_scene_id;
// vectorID==0 — ссылки на вектор нет (shared-векторы к литературной базе не
// подключены, D-19.2).
func Write(h *db.Handle, cen Censor, rec SedimentRecord, vectorID int64, now string) (int64, error) {
	if cen == nil {
		return 0, ErrCensorReject
	}
	ok, hits := cen.Check(rec.CleanedText)
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrCensorReject, strings.Join(hits, ", "))
	}
	if rec.TrustLevel == TrustLevelVerified {
		return 0, ErrTrustEscalation
	}

	var vid any
	if vectorID > 0 {
		vid = vectorID
	}

	res, err := h.Exec("sediment_scene",
		`INSERT INTO sediment_scene (source_scene_id, cleaned_text, vector_id, verdict, trust_level, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		rec.SourceSceneID, rec.CleanedText, vid, rec.Verdict, TrustLevelUnchecked, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// findExisting возвращает уже осевшую строку sediment_scene по source_scene_id
// (T-04-10, шаг 2). Повторная сцена не переписывается: конвейер возвращает
// существующую запись без вызова слота, эмбеддера и записи — дешёвый повтор.
func findExisting(h *db.Handle, sourceSceneID int64) (WrittenScene, bool, error) {
	var out WrittenScene
	var vectorID sql.NullInt64
	err := h.QueryRow(
		`SELECT id, source_scene_id, vector_id, verdict, trust_level FROM sediment_scene WHERE source_scene_id = ?`,
		sourceSceneID,
	).Scan(&out.ID, &out.SourceSceneID, &vectorID, &out.Verdict, &out.TrustLevel)
	if errors.Is(err, sql.ErrNoRows) {
		return WrittenScene{}, false, nil
	}
	if err != nil {
		return WrittenScene{}, false, err
	}
	out.VectorID = vectorID.Int64
	return out, true, nil
}
