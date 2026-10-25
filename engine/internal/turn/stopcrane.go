package turn

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"tns65/internal/db"
	"tns65/internal/models"
)

// Стоп-кран (T-03-05, LT-07): до подтверждения смысла предварительный путь только
// читает и готовит, записи нет; неподтверждённая ветка в мир не пишется. Коммит
// хода — одной транзакцией db.Tx(core, …) после полной подтверждённой реплики.

// heroCharacterID — идентификатор героя в character_state (0 — как в рождении,
// T-02-16).
const heroCharacterID = 0

// Значения, которые пишет коммит хода в оперативную базу мира.
const (
	sceneStatusOpen = "open"
	eventKindTurn   = "turn_committed"
	eventSourceCore = "core"
	embedModel      = "bge-m3"
	embedLang       = "ru"
)

// commitTurn фиксирует ход одной транзакцией: turns, scenes, events, vectors,
// token_bindings, character_state — либо все, либо ни одной (T-03-05, шаг 1).
// Неподтверждённый смысл записей не оставляет; слеш-команда и подтверждённый ход
// коммитятся.
// failTable — тестовый крючок: имя таблицы, после записи которой вернуть ошибку
// для проверки отката; пусто в проде.
func commitTurn(st *TurnState, h *db.Handle, failTable string) error {
	if h == nil || !shouldCommit(st) {
		return nil
	}
	return h.Tx(db.WriterCore, func(tx *db.Tx) error {
		turnID, err := writeTurn(tx, st)
		if err != nil {
			return err
		}
		if failTable == "turns" {
			return fmt.Errorf("turn: инъекция отказа на таблице turns")
		}

		sceneID, err := writeScene(tx, st, turnID)
		if err != nil {
			return err
		}
		if failTable == "scenes" {
			return fmt.Errorf("turn: инъекция отказа на таблице scenes")
		}

		if err := writeTurnEvent(tx, turnID); err != nil {
			return err
		}
		if failTable == "events" {
			return fmt.Errorf("turn: инъекция отказа на таблице events")
		}

		if err := writeVectors(tx, st, turnID); err != nil {
			return err
		}
		if failTable == "vectors" {
			return fmt.Errorf("turn: инъекция отказа на таблице vectors")
		}

		if err := writeBindings(tx, st, sceneID); err != nil {
			return err
		}
		if failTable == "token_bindings" {
			return fmt.Errorf("turn: инъекция отказа на таблице token_bindings")
		}

		if err := writeHeroState(tx, st); err != nil {
			return err
		}
		if failTable == "character_state" {
			return fmt.Errorf("turn: инъекция отказа на таблице character_state")
		}
		return nil
	})
}

// shouldCommit — стоп-кран: слеш-команда и подтверждённый смысл коммитятся;
// неподтверждённая ветка в мир не пишется (LT-07.3). Лексикон не загружен —
// подтверждать нечего, ход с fallback-ответом коммитится.
func shouldCommit(st *TurnState) bool {
	if st.Command != nil {
		return true
	}
	if !st.Prepath.LexiconLoaded {
		return true
	}
	return st.Confirm.Confirmed()
}

// writeTurn пишет строку хода и возвращает её идентификатор базы.
func writeTurn(tx *db.Tx, st *TurnState) (int64, error) {
	degraded := 0
	if st.Degraded {
		degraded = 1
	}
	res, err := tx.Exec("turns",
		`INSERT INTO turns (session_id, player_text, reply_text, path, saw_recommendation, selector_decision, trust_level, latency_ms, degraded) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		st.Input.SessionID, st.Input.PlayerText, st.Reply, st.Path, st.SawRecommendation, st.SelectorDecision, trustLevel(st.Confirm), st.LatencyMS, degraded)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// trustLevel — доверие подтверждения хода: подтверждено — 1, иначе 0.
func trustLevel(c Confirmation) int {
	if c.Confirmed() {
		return 1
	}
	return 0
}

// writeScene пишет сцену хода и возвращает её идентификатор базы — его получают
// привязки токенов token_bindings.
func writeScene(tx *db.Tx, st *TurnState, turnID int64) (int64, error) {
	res, err := tx.Exec("scenes",
		`INSERT INTO scenes (session_id, day, place_id, status, prose, turn_id) VALUES (?, 0, ?, ?, ?, ?)`,
		st.Input.SessionID, st.Prepath.Frame.HeroPlaceID, sceneStatusOpen, st.Reply, turnID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// writeTurnEvent пишет событие коммита хода в историю мира (events).
func writeTurnEvent(tx *db.Tx, turnID int64) error {
	payload, err := json.Marshal(map[string]int64{"turn_id": turnID})
	if err != nil {
		return err
	}
	_, err = tx.Exec("events",
		`INSERT INTO events (day, kind, payload_json, source) VALUES (0, ?, ?, ?)`,
		eventKindTurn, string(payload), eventSourceCore)
	return err
}

// writeVectors пишет векторы хода в vectors: вектор целой реплики и по вектору на
// сегмент; ранние векторы слов коммит не снимает (T-03-02, шаг 3).
func writeVectors(tx *db.Tx, st *TurnState, turnID int64) error {
	created := time.Now().UTC().Format(time.RFC3339)
	if st.Cut.Whole != nil {
		if err := insertVector(tx, turnID, st.Input.PlayerText, st.Cut.Whole, created); err != nil {
			return err
		}
	}
	for _, seg := range st.Cut.Segments {
		if seg.Vector == nil {
			continue
		}
		if err := insertVector(tx, turnID, seg.Text, seg.Vector, created); err != nil {
			return err
		}
	}
	return nil
}

func insertVector(tx *db.Tx, turnID int64, source string, vec []float32, created string) error {
	if len(vec) != models.EmbedDim {
		return fmt.Errorf("turn: вектор хода dim=%d, want %d", len(vec), models.EmbedDim)
	}
	_, err := tx.Exec("vectors",
		`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES ('turns', ?, ?, ?, ?, ?, ?, ?)`,
		turnID, embedModel, models.EmbedDim, embedLang, source, vecBlob(vec), created)
	return err
}

// vecBlob кодирует вектор Float32 little-endian (BLOB, T-03-02, шаг 2).
func vecBlob(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, f := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// writeBindings пишет привязки токенов биндера на коммите хода (T-03-03, шаг 2):
// до подтверждения они жили в памяти, scene_id получают только сейчас.
func writeBindings(tx *db.Tx, st *TurnState, sceneID int64) error {
	updated := time.Now().UTC().Format(time.RFC3339)
	for _, b := range st.Bindings {
		if _, err := tx.Exec("token_bindings",
			`INSERT INTO token_bindings (scene_id, chapter_id, token, character_id, updated_at) VALUES (?, ?, ?, ?, ?)`,
			sceneID, b.ChapterID, b.Token, b.CharacterID, updated); err != nil {
			return err
		}
	}
	return nil
}

// writeHeroState обновляет состояние героя: место из рамки сцены и настроение
// реплики; строку героя заводит, если её ещё нет.
func writeHeroState(tx *db.Tx, st *TurnState) error {
	_, err := tx.Exec("character_state",
		`INSERT INTO character_state (character_id, place_id, mood) VALUES (?, ?, ?)
		 ON CONFLICT(character_id) DO UPDATE SET place_id = excluded.place_id, mood = excluded.mood`,
		heroCharacterID, st.Prepath.Frame.HeroPlaceID, st.Prepath.Mood)
	return err
}
