package webapi

import (
	"context"
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
	"tns65/internal/models"
	"tns65/internal/sediment"
	"tns65/internal/textlimit"
	"tns65/internal/turn"
	"tns65/internal/workshop"
)

// Чат (T-08-15): реплика уходит в конвейер T-03-01, ответ мира доставляется
// порциями по WS-топику turn.stream, панель мир/герой/система читается из
// GET /api/state, сигналы сцены пишутся в scene_signals. Ход коммитится
// конвейером (писатель core, T-03-05); webapi сам пишет только sessions через
// book.EnsureSession, запускает автозакрытие главы book.AutoClose (CB-03.5) и
// scene_signals (точка вызова T-04-06).

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

// chatWeaver — слот Ткача пути генерации прозы (T-03-08); nil — слот не
// подключён, ход уходит на страховку. Подменяется e2e-хуком SetE2ESlot.
var chatWeaver turn.ProseSlot

// SetChatWeaver подключает слот Ткача к чатовому конвейеру. Пустой слот и уже
// подключённый слот не перезаписываются: в e2e-сборке подставной слот ставит
// хук --fake-slot до старта ядра, и релизная проводка его не затирает.
func SetChatWeaver(s turn.ProseSlot) {
	if s == nil || chatWeaver != nil {
		return
	}
	chatWeaver = s
}

// chatLemmatizer — лемматизатор резки реплики (T-03-02); nil — лемм нет.
// Подменяется e2e-хуком SetE2ELemmatizer.
var chatLemmatizer turn.Lemmatizer

// chatEmbedder — эмбеддер векторов смысла реплики (T-20-02); nil — резка без
// векторов, векторный голос подтверждения не звучит.
var chatEmbedder models.Embedder

// SetChatEmbedder подключает эмбеддер к чатовому конвейеру: резка векторизует
// реплику, сегменты и ранние слова, подтверждение получает векторный голос. nil
// отключает векторный путь — ход идёт без падения.
func SetChatEmbedder(emb models.Embedder) {
	chatEmbedder = emb
}

// chatLastActive — момент последней реплики игрока по миру (проводка
// последней активности сервером хода, T-08-15). Ключ — слаг мира.
var (
	chatLastActiveMu sync.Mutex
	chatLastActive   = map[string]time.Time{}
)

// inputLimits — лимиты вводимых текстов из секции limits config.yaml (IMP-7);
// грузятся при старте сервера.
var inputLimits = textlimit.DefaultLimits()

// chatTurnRequest — тело POST /api/turn (шаг 1).
type chatTurnRequest struct {
	World string `json:"world"`
	Text  string `json:"text"`
}

// chatSignalRequest — тело POST /api/scene/{id}/signal (шаг 6).
type chatSignalRequest struct {
	Kind string `json:"kind"`
}

// chatHeroState — состояние героя панели чата из character_state и слоя
// состояния character_layers (W2).
type chatHeroState struct {
	PlaceID     int64  `json:"place_id,omitempty"`
	Mood        string `json:"mood,omitempty"`
	Status      string `json:"status,omitempty"`
	Health      int    `json:"health,omitempty"`
	HealthKnown bool   `json:"health_known"`
	Place       string `json:"place,omitempty"`
	PlaceKind   string `json:"place_kind,omitempty"`
}

// chatSystemState — состояние системы панели чата.
type chatSystemState struct {
	Mode      string `json:"mode"`
	SessionID int64  `json:"session_id,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
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
	// Потолок реплики (IMP-7): оценка с поправкой на язык; превышение — отказ
	// с подсказкой, ход не начинается и конвейер не запускается.
	if err := textlimit.Check(textlimit.KindPlayerMessage, text, inputLimits.PlayerMessage); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, err.Error())
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

	// Автозакрытие главы (CB-03.5): открытая глава старше 20 часов закрывается
	// до прихода игрока, иначе она осталась бы открытой вечно. Заметки о жизни
	// мира собираются кодом: слот мастерской на этом шаге не подключён. Сбой
	// автозакрытия ход не валит.
	now := nowUTCString()
	if _, _, err := book.AutoClose(h, now, nil); err != nil && chatLogger != nil {
		_ = chatLogger.Event("book", "book.autoclose",
			journal.Field{Key: "error", Value: err.Error()})
	}

	sessionID, err := book.EnsureSession(h, now)
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
		Weaver:     chatWeaver,
		Lemmatizer: chatLemmatizer,
		Embedder:   chatEmbedder,
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
	// T-21-08: ранние векторы набора сверяются с гороскопом сцены (W-13.7);
	// совпавшая bank_ref-ветка отдаётся готовой записью банка, промах ставит
	// ветку кандидатом в оседание (W-13.6). Сбой сверки ход не валит.
	if branch, err := sceneHoroscopeReply(r.Context(), h, lit, turnID, sessionID, text); err != nil {
		if chatLogger != nil {
			_ = chatLogger.Event("workshop", "workshop.horoscope_match",
				journal.Field{Key: "error", Value: err.Error()})
		}
	} else {
		for _, chunk := range chatChunks(branch) {
			broadcastTurnChunk(turnID, chunk)
		}
	}
	chatTouch(slug)
	// T-20-04: реплика возвращает мастерскую с паузы — молчание обнуляется.
	chatMarkActive()
	// T-21-07: гороскоп живёт до смены темы сцены (W-13.1) — при смене ставится
	// заказ Пифии; сбой постановки ход не валит.
	if err := ensureHoroscopeTopic(h, sessionID); err != nil && chatLogger != nil {
		_ = chatLogger.Event("workshop", "workshop.horoscope_order",
			journal.Field{Key: "error", Value: err.Error()})
	}

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
		ReturnNews:    readChatReturnNews(h),
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

// readChatReturnNews читает вести последнего возврата из журнала событий мира
// (T-03-14, шаг 2): стадия digest пишет одно событие news со списком текстов,
// поэтому последнее такое событие — вести текущего возврата. Пустой срез —
// вестей не было; ответ всегда отдаёт массив, а не null.
func readChatReturnNews(h *db.Handle) []string {
	var raw string
	if err := h.QueryRow(`SELECT payload_json FROM events WHERE kind = ? ORDER BY id DESC LIMIT 1`,
		turn.NewsEventKind).Scan(&raw); err != nil {
		return []string{}
	}
	var payload struct {
		Texts []string `json:"texts"`
		Text  string   `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return []string{}
	}
	if len(payload.Texts) > 0 {
		return payload.Texts
	}
	if payload.Text != "" {
		return []string{payload.Text}
	}
	return []string{}
}

// chatLastActiveAt возвращает момент последней реплики игрока по миру.
func chatLastActiveAt(slug string) time.Time {
	chatLastActiveMu.Lock()
	last := chatLastActive[slug]
	chatLastActiveMu.Unlock()
	return last
}

// ChatLastActive возвращает момент последней реплики игрока по миру (T-21-05):
// воркер мастерской инициализирует от него паузу при открытии мира. Нулевое
// время — реплик в этом процессе не было.
func ChatLastActive(slug string) time.Time {
	return chatLastActiveAt(slug)
}

// chatTouch фиксирует момент последней реплики игрока по миру.
func chatTouch(slug string) {
	chatLastActiveMu.Lock()
	chatLastActive[slug] = chatClock.Now()
	chatLastActiveMu.Unlock()
}

// WorkshopScheduler — планировщик мастерской, которому чат сообщает активность
// игрока (T-20-04): реплика обнуляет молчание и возвращает ступень паузы.
type WorkshopScheduler interface {
	MarkActive()
}

// workshopSched — планировщик мастерской ядра; nil — воркер не подключён.
var (
	workshopSchedMu sync.Mutex
	workshopSched   WorkshopScheduler
)

// SetWorkshopScheduler подключает планировщик мастерской ядра (T-20-04); nil —
// планировщика нет, активность хода только пишется в чат.
func SetWorkshopScheduler(s WorkshopScheduler) {
	workshopSchedMu.Lock()
	defer workshopSchedMu.Unlock()
	workshopSched = s
}

// chatMarkActive сообщает планировщику мастерской о реплике игрока (T-20-04).
func chatMarkActive() {
	workshopSchedMu.Lock()
	s := workshopSched
	workshopSchedMu.Unlock()
	if s != nil {
		s.MarkActive()
	}
}

// chatHoroscopeForecast — прогноз Пилы заказа гороскопа: эхо по умолчанию
// (W-13.2); контрапункт ставит планировщик по назревающему пику.
const chatHoroscopeForecast = "echo"

// chatPythiaMap — карта игрока для payload гороскопа: последняя строка
// player_atlas (тема, настроение, интерес); пусто — карты ещё нет.
func chatPythiaMap(h *db.Handle) string {
	var theme, mood, interest string
	if err := h.QueryRow(`SELECT theme, mood, interest FROM player_atlas ORDER BY id DESC LIMIT 1`).
		Scan(&theme, &mood, &interest); err != nil {
		return ""
	}
	parts := make([]string, 0, 3)
	for _, s := range []string{theme, mood, interest} {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "; ")
}

// ensureHoroscopeTopic ставит заказ Пифии при смене темы сцены (T-21-07,
// W-13.1): гороскоп живёт до смены темы, повтор без смены заказ не дублирует.
func ensureHoroscopeTopic(h *db.Handle, sessionID int64) error {
	topic, err := workshop.CurrentSceneTopic(h, sessionID)
	if err != nil || topic.SceneID == 0 {
		return err
	}
	live, ok, err := workshop.LiveHoroscopeTopic(h)
	if err != nil {
		return err
	}
	if ok && live.PlaceID == topic.PlaceID {
		return nil
	}
	_, err = workshop.EnsureHoroscopeOrder(h, workshop.HoroscopePayload{
		SceneID:  topic.SceneID,
		Pythia:   chatPythiaMap(h),
		Forecast: chatHoroscopeForecast,
	})
	return err
}

// sceneHoroscopeReply сверяет ранние векторы реплики с гороскопом сцены хода
// (T-21-08, W-13.7): совпадение отдаёт текст готовой bank_ref-ветки, промах
// ставит ветку кандидатом в оседание (W-13.6). Пусто — ветки нет.
func sceneHoroscopeReply(ctx context.Context, h, lit *db.Handle, turnID, sessionID int64, text string) (string, error) {
	if lit == nil {
		return "", nil
	}
	sceneID := turnSceneID(h, turnID)
	if sceneID == 0 {
		return "", nil
	}
	return turn.MatchSceneHoroscope(ctx, h, lit, sceneID, text, chatEmbedder,
		turn.SedimentCandidate{TurnID: turnID, SessionID: sessionID})
}

// turnSceneID возвращает сцену хода; 0 — сцены нет (слеш-команда, откат).
func turnSceneID(h *db.Handle, turnID int64) int64 {
	var id int64
	if err := h.QueryRow(`SELECT id FROM scenes WHERE turn_id = ? ORDER BY id DESC LIMIT 1`, turnID).Scan(&id); err != nil {
		return 0
	}
	return id
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
	_ = rows.Err() // ошибка чтения гасится: панель «мир» показывает прочитанное.
	return out
}

// chatHeroHealthLayer — минимальный слой состояния героя для чтения здоровья
// (W2): здоровье лежит в character_layers character_id=0 layer='state',
// payload_json.health — целое число процентов.
type chatHeroHealthLayer struct {
	Health int `json:"health"`
}

// readChatHero читает состояние героя из character_state (character_id=0) и
// дополняет его здоровьем из character_layers и названием места из places —
// только чтение, ничего не пишется.
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

	var raw string
	if err := h.QueryRow(`SELECT payload_json FROM character_layers WHERE character_id = 0 AND layer = 'state'`).Scan(&raw); err == nil {
		var layer chatHeroHealthLayer
		if json.Unmarshal([]byte(raw), &layer) == nil {
			out.Health = layer.Health
			out.HealthKnown = true
		}
	}

	if out.PlaceID != 0 {
		var name, kind sql.NullString
		if err := h.QueryRow(`SELECT name, kind FROM places WHERE id = ?`, out.PlaceID).Scan(&name, &kind); err == nil {
			out.Place = name.String
			out.PlaceKind = kind.String
		}
	}
	return out
}

// readChatSystem читает состояние системы: режим и открытую сессию прихода.
func readChatSystem(h *db.Handle) chatSystemState {
	out := chatSystemState{Mode: "normal"}
	var id int64
	var startedAt sql.NullString
	if err := h.QueryRow(`SELECT id, started_at FROM sessions WHERE ended_at IS NULL ORDER BY id DESC LIMIT 1`).
		Scan(&id, &startedAt); err == nil {
		out.SessionID = id
		out.StartedAt = startedAt.String
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
	_ = rows.Err() // ошибка чтения гасится: чат показывает прочитанное.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func nowUTCString() string {
	return time.Now().UTC().Format(time.RFC3339)
}
