package webapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"tns65/internal/book"
	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/sediment"
	"tns65/internal/turn"
)

// Чат (T-08-15): реплика уходит в конвейер T-03-01, ответ мира доставляется
// порциями по WS-топику turn.stream, панель мир/герой/система читается из
// GET /api/state, сигналы сцены пишутся в scene_signals. Ход коммитится
// конвейером (писатель core, T-03-05); webapi сам пишет только sessions через
// book.EnsureSession и scene_signals (точка вызова T-04-06).

const (
	chatTurnPath     = "/api/turn"
	chatStatePath    = "/api/state"
	chatSignalPrefix = "/api/scene/"
	turnStreamTopic  = "turn.stream"
	turnChunkEvent   = "chunk"
)

// chatSummaryText — сводка «пока тебя не было» (T-03-14): игрок видит её при
// возврате до первой реплики. Полную механику паузы ведёт стадия возврата
// конвейера; здесь — только порог ночи для показа сводки в чате.
const chatSummaryText = "Пока тебя не было, мир изменился."

// chatHub — WS-хаб для рассылки порций хода; подключается при сборке роутера
// в routes.go.
var chatHub *Hub

// chatLogger — журнал станка для хода; подключается при сборке сервера.
var chatLogger *journal.Logger

// chatLastActive — момент последней реплики игрока по миру (проводка
// последней активности сервером хода, T-08-15). Ключ — слаг мира.
var (
	chatLastActiveMu sync.Mutex
	chatLastActive   = map[string]time.Time{}
)

// chatTurnRequest — тело POST /api/turn (шаг 1).
type chatTurnRequest struct {
	World string `json:"world"`
	Text  string `json:"text"`
}

// chatSignalRequest — тело POST /api/scene/{id}/signal (шаг 6).
type chatSignalRequest struct {
	Kind string `json:"kind"`
}

// chatHeroState — состояние героя панели чата из character_state.
type chatHeroState struct {
	PlaceID int64  `json:"place_id,omitempty"`
	Mood    string `json:"mood,omitempty"`
	Status  string `json:"status,omitempty"`
}

// chatSystemState — состояние системы панели чата.
type chatSystemState struct {
	Mode      string `json:"mode"`
	SessionID int64  `json:"session_id,omitempty"`
}

// chatTurnItem — строка журнала хода для переподключения (обрыв WS не теряет
// ход: переподключение получает текущее состояние из GET /api/state).
type chatTurnItem struct {
	TurnID     int64  `json:"turn_id"`
	SceneID    int64  `json:"scene_id,omitempty"`
	PlayerText string `json:"player_text"`
	ReplyText  string `json:"reply_text"`
}

// chatStateResponse — тело GET /api/state (шаг 3).
type chatStateResponse struct {
	World         string            `json:"world"`
	WorldState    map[string]string `json:"world_state"`
	Hero          chatHeroState     `json:"hero"`
	System        chatSystemState   `json:"system"`
	ReturnSummary string            `json:"return_summary,omitempty"`
	ReturnNews    []string          `json:"return_news"`
	Turns         []chatTurnItem    `json:"turns"`
}

// handleChatTurn — POST /api/turn: подача реплики в конвейер, ответ мира
// доставляется порциями по WS-топику turn.stream.
func handleChatTurn(w http.ResponseWriter, r *http.Request) {
	var req chatTurnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad turn body")
		return
	}
	slug := strings.TrimSpace(req.World)
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "text required")
		return
	}
	if !chatWorldReady(slug) {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}

	h, err := openChatWriter(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "world open failed")
		return
	}
	defer h.Close()

	sessionID, err := book.EnsureSession(h, nowUTCString())
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "session failed")
		return
	}

	lang := chatWorldLang(h)
	baseDBPath := ""
	if lang != "" {
		baseDBPath = filepath.Join(db.BaseRoot, "base."+lang+".db")
	}

	lit := openChatLiterary(slug)
	if lit != nil {
		defer lit.Close()
	}

	out, err := (&turn.Pipeline{
		Logger:     chatLogger,
		DB:         h,
		LitDB:      lit,
		DriftDB:    h,
		BaseDBPath: baseDBPath,
		Lang:       lang,
		Clock:      chatClock,
		LastActive: chatLastActiveAt(slug),
	}).Run(r.Context(), turn.TurnInput{
		SessionID:  sessionID,
		PlayerText: text,
		At:         time.Now(),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "turn failed")
		return
	}

	turnID := lastTurnID(h, sessionID)
	for _, chunk := range chatChunks(out.ReplyText) {
		broadcastTurnChunk(turnID, chunk)
	}
	chatTouch(slug)

	writeJSON(w, http.StatusOK, map[string]int64{"turn_id": turnID})
}

// handleChatState — GET /api/state: состояние мира, героя и системы панели чата,
// сводка при возврате и журнал ходов для переподключения.
func handleChatState(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("world"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}
	if !chatWorldReady(slug) {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}

	h, err := db.OpenReaderAs(worldDBPath(slug), db.DBWorld, db.WriterCore, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "world open failed")
		return
	}
	defer h.Close()

	writeJSON(w, http.StatusOK, chatStateResponse{
		World:         slug,
		WorldState:    readChatWorldState(h),
		Hero:          readChatHero(h),
		System:        readChatSystem(h),
		ReturnSummary: chatReturnSummary(slug),
		ReturnNews:    []string{},
		Turns:         readChatTurns(h),
	})
}

// handleChatSignal — POST /api/scene/{id}/signal: сигнал игрока о сцене
// (лайк/дизлайк) пишется в scene_signals хэндлом core (T-04-06, шаг 1).
func handleChatSignal(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, chatSignalPrefix)
	idStr, _, ok := strings.Cut(rest, "/signal")
	if !ok {
		writeError(w, http.StatusBadRequest, codeBadRequest, "signal route required")
		return
	}
	sceneID, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
	if err != nil || sceneID <= 0 {
		writeError(w, http.StatusBadRequest, codeBadRequest, "scene id required")
		return
	}

	var req chatSignalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "bad signal body")
		return
	}
	if req.Kind != string(sediment.SignalLike) && req.Kind != string(sediment.SignalDislike) {
		writeError(w, http.StatusBadRequest, codeBadRequest, "kind must be like or dislike")
		return
	}

	slug := strings.TrimSpace(getActiveSlug())
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "no active world")
		return
	}
	if !chatWorldReady(slug) {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}

	h, err := openChatWriter(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "world open failed")
		return
	}
	defer h.Close()

	if _, err := h.Exec("scene_signals",
		`INSERT INTO scene_signals (scene_id, kind, created_at) VALUES (?, ?, ?)`,
		sceneID, req.Kind, nowUTCString()); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "signal write failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// openChatWriter открывает оперативную базу мира хэндлом core: сессию и сигналы
// пишет webapi, сам ход — конвейер (тот же писатель).
func openChatWriter(slug string) (*db.Handle, error) {
	return db.OpenWriterAs(worldDBPath(slug), db.DBWorld, db.WriterCore, chatLogger)
}

// openChatLiterary открывает литературную базу мира для выбора композиции
// (T-10-11). Базы нет — nil: композиция не выбирается, ход идёт прежним путём
// (деградация без отказа).
func openChatLiterary(slug string) *db.Handle {
	if _, err := os.Stat(litDBPath(slug)); err != nil {
		return nil
	}
	lit, err := db.OpenReaderAs(litDBPath(slug), db.DBLiterary, db.WriterForge, chatLogger)
	if err != nil {
		return nil
	}
	return lit
}

// chatWorldLang читает язык мира из world_meta (замороженное поле lang, TUN-5):
// значение — JSON-строка. Пусто — пул имён не подключается.
func chatWorldLang(h *db.Handle) string {
	var raw string
	if err := h.QueryRow(`SELECT value FROM world_meta WHERE "key" = 'lang'`).Scan(&raw); err != nil {
		return ""
	}
	var lang string
	if err := json.Unmarshal([]byte(raw), &lang); err != nil {
		return ""
	}
	return lang
}

// chatWorldReady сообщает, существует ли база мира по слагу.
func chatWorldReady(slug string) bool {
	_, err := os.Stat(worldDBPath(slug))
	return err == nil
}

// lastTurnID возвращает последний закоммиченный ход сессии.
func lastTurnID(h *db.Handle, sessionID int64) int64 {
	var id int64
	if err := h.QueryRow(`SELECT id FROM turns WHERE session_id = ? ORDER BY id DESC LIMIT 1`, sessionID).Scan(&id); err != nil {
		return 0
	}
	return id
}

// chatReturnSummary возвращает сводку при возврате, если отсутствие превысило
// порог ночи (T-03-14, LT-13.1); пусто — сводки нет.
func chatReturnSummary(slug string) string {
	chatLastActiveMu.Lock()
	last, ok := chatLastActive[slug]
	chatLastActiveMu.Unlock()
	if !ok || last.IsZero() {
		return ""
	}
	if chatClock.Now().Sub(last) < time.Duration(turn.NightThresholdMin)*time.Minute {
		return ""
	}
	return chatSummaryText
}

// chatLastActiveAt возвращает момент последней реплики игрока по миру.
func chatLastActiveAt(slug string) time.Time {
	chatLastActiveMu.Lock()
	last := chatLastActive[slug]
	chatLastActiveMu.Unlock()
	return last
}

// chatTouch фиксирует момент последней реплики игрока по миру.
func chatTouch(slug string) {
	chatLastActiveMu.Lock()
	chatLastActive[slug] = chatClock.Now()
	chatLastActiveMu.Unlock()
}

// broadcastTurnChunk рассылает порцию сцены подписчикам топика turn.stream.
func broadcastTurnChunk(turnID int64, text string) {
	if chatHub == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{"turn_id": turnID, "text": text})
	if err != nil {
		return
	}
	chatHub.Broadcast(WSMessage{Topic: turnStreamTopic, EventType: turnChunkEvent, Payload: payload})
}

// chatChunks режет ответ на порции по границам предложений — инкрементальная
// доставка сцены в чат по мере готовности (T-03-10, LT-06).
func chatChunks(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var out []string
	var b strings.Builder
	for _, r := range text {
		b.WriteRune(r)
		if strings.ContainsRune(".!?…\n", r) {
			out = append(out, b.String())
			b.Reset()
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// readChatWorldState читает замороженные поля анкеты мира из world_meta
// (значения — JSON-строки) для панели «мир».
func readChatWorldState(h *db.Handle) map[string]string {
	out := map[string]string{}
	rows, err := h.Query(`SELECT "key", value FROM world_meta`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		if !isFrozenWorldField(key) {
			continue
		}
		var s string
		if err := json.Unmarshal([]byte(value), &s); err != nil {
			continue
		}
		out[key] = s
	}
	return out
}

// readChatHero читает состояние героя из character_state (character_id=0).
func readChatHero(h *db.Handle) chatHeroState {
	var out chatHeroState
	var placeID sql.NullInt64
	var mood, status sql.NullString
	if err := h.QueryRow(`SELECT place_id, mood, status FROM character_state WHERE character_id = 0`).
		Scan(&placeID, &mood, &status); err != nil {
		return out
	}
	out.PlaceID = placeID.Int64
	out.Mood = mood.String
	out.Status = status.String
	return out
}

// readChatSystem читает состояние системы: режим и открытую сессию прихода.
func readChatSystem(h *db.Handle) chatSystemState {
	out := chatSystemState{Mode: "normal"}
	var id int64
	if err := h.QueryRow(`SELECT id FROM sessions WHERE ended_at IS NULL ORDER BY id DESC LIMIT 1`).Scan(&id); err == nil {
		out.SessionID = id
	}
	return out
}

// readChatTurns читает последние ходы мира в хронологическом порядке — текущее
// состояние чата для переподключения. Сцена хода нужна для сигналов лайк/дизлайк.
func readChatTurns(h *db.Handle) []chatTurnItem {
	rows, err := h.Query(`SELECT t.id, COALESCE(s.id, 0), t.player_text, t.reply_text
		FROM turns t LEFT JOIN scenes s ON s.turn_id = t.id
		ORDER BY t.id DESC LIMIT 20`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []chatTurnItem
	for rows.Next() {
		var item chatTurnItem
		if err := rows.Scan(&item.TurnID, &item.SceneID, &item.PlayerText, &item.ReplyText); err != nil {
			continue
		}
		out = append(out, item)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func nowUTCString() string {
	return time.Now().UTC().Format(time.RFC3339)
}
