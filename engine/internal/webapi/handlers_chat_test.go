package webapi

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tns65/internal/db"
)

// seedChatWorld создаёт базу мира alpha по раскладке <root>/worlds/<slug>/<slug>.db,
// кладёт в world_meta заголовок и режим походов, заводит состояние героя и
// подменяет worldDBPath, активный мир и память последней активности.
func seedChatWorld(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	worldDir := filepath.Join(root, "worlds", "alpha")
	if err := os.MkdirAll(worldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(worldDir, "alpha.db")

	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(raw, "world", "shared"); err != nil {
		_ = raw.Close()
		t.Fatal(err)
	}
	_ = raw.Close()

	forge, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatal(err)
	}
	for key, val := range map[string]string{"title": "alpha", "travel_mode": "вымышленная"} {
		rawVal, err := json.Marshal(val)
		if err != nil {
			_ = forge.Close()
			t.Fatal(err)
		}
		if _, err := forge.Exec("world_meta",
			`INSERT INTO world_meta ("key", value) VALUES (?, ?)`, key, string(rawVal)); err != nil {
			_ = forge.Close()
			t.Fatal(err)
		}
	}
	_ = forge.Close()

	core, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.Exec("character_state",
		`INSERT INTO character_state (character_id, place_id, mood, status) VALUES (0, 1, 'спокоен', 'в игре')`); err != nil {
		_ = core.Close()
		t.Fatal(err)
	}
	_ = core.Close()

	oldPath := worldDBPath
	worldDBPath = func(slug string) string {
		return filepath.Join(root, "worlds", slug, slug+".db")
	}
	oldLast := chatLastActive
	chatLastActive = map[string]time.Time{}
	t.Cleanup(func() {
		worldDBPath = oldPath
		chatLastActive = oldLast
		setActiveSlug("")
		chatHub = nil
		chatLogger = nil
	})
	setActiveSlug("alpha")
	return path
}

// chatRequest выполняет запрос к хендлеру чата напрямую.
func chatRequest(t *testing.T, h http.HandlerFunc, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

// chatChunk — порция сцены из WS-топика turn.stream.
type chatChunk struct {
	TurnID int64  `json:"turn_id"`
	Text   string `json:"text"`
}

// dialTestWS поднимает клиентское WS-подключение к серверу и подписывается на
// топики. Кадры клиента маскируются (writeClientText), кадры сервера не
// маскируются (readServerFrame).
func dialTestWS(t *testing.T, baseURL string, topics []string) (net.Conn, *bufio.Reader) {
	t.Helper()
	u, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	host := u.Host
	var dialer net.Dialer
	conn, err := dialer.DialContext(context.Background(), "tcp", host)
	if err != nil {
		t.Fatal(err)
	}

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)

	req := "GET /ws HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatal(err)
	}

	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("ws handshake: %q", status)
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimRight(line, "\r\n") == "" {
			break
		}
	}

	sub, err := json.Marshal(map[string]any{"type": "subscribe", "topics": topics})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeClientText(conn, string(sub)); err != nil {
		t.Fatal(err)
	}
	return conn, br
}

// collectTurnChunks читает порции топика turn.stream до тайм-аута.
func collectTurnChunks(t *testing.T, conn net.Conn, br *bufio.Reader) []chatChunk {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var out []chatChunk
	for {
		op, data, err := readServerFrame(br)
		if err != nil {
			break
		}
		if op != opText {
			continue
		}
		var msg WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.Topic != turnStreamTopic || msg.EventType != turnChunkEvent {
			continue
		}
		var c chatChunk
		if err := json.Unmarshal(msg.Payload, &c); err != nil {
			continue
		}
		out = append(out, c)
	}
	return out
}

// TestTurnEndpoint — реплика уходит в конвейер и коммитится строкой turns;
// слеш-команда идёт тем же конвейером.
func TestTurnEndpoint(t *testing.T) {
	path := seedChatWorld(t)

	rr := chatRequest(t, handleChatTurn, http.MethodPost, chatTurnPath, `{"world":"alpha","text":"Я иду по дороге."}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/turn: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var res struct {
		TurnID int64 `json:"turn_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if res.TurnID <= 0 {
		t.Fatalf("turn_id = %d, want > 0", res.TurnID)
	}

	reader, err := db.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	var n int
	if err := reader.QueryRow(`SELECT COUNT(*) FROM turns`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("строк в turns = %d, want 1", n)
	}
	var reply string
	if err := reader.QueryRow(`SELECT reply_text FROM turns WHERE id = ?`, res.TurnID).Scan(&reply); err != nil {
		t.Fatal(err)
	}
	if reply == "" {
		t.Fatal("reply_text пуст")
	}

	rr = chatRequest(t, handleChatTurn, http.MethodPost, chatTurnPath, `{"world":"alpha","text":"/статус"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/turn (команда): status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var cmdRes struct {
		TurnID int64 `json:"turn_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &cmdRes); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if cmdRes.TurnID <= 0 {
		t.Fatalf("turn_id команды = %d, want > 0", cmdRes.TurnID)
	}
	if err := reader.QueryRow(`SELECT COUNT(*) FROM turns`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("строк в turns после команды = %d, want 2", n)
	}

	if rr := chatRequest(t, handleChatTurn, http.MethodPost, chatTurnPath, `{}`); rr.Code != http.StatusBadRequest {
		t.Fatalf("пустое тело: status = %d, want 400", rr.Code)
	}
	if rr := chatRequest(t, handleChatTurn, http.MethodPost, chatTurnPath, `{"world":"alpha"}`); rr.Code != http.StatusBadRequest {
		t.Fatalf("без текста: status = %d, want 400", rr.Code)
	}
	if rr := chatRequest(t, handleChatTurn, http.MethodPost, chatTurnPath, `{"world":"nope","text":"х"}`); rr.Code != http.StatusNotFound {
		t.Fatalf("несуществующий мир: status = %d, want 404", rr.Code)
	}
}

// TestChatState — панель мир/герой/система и сводка при возврате до первой
// реплики.
func TestChatState(t *testing.T) {
	seedChatWorld(t)

	// Отсутствие превысило порог ночи — при возврате есть сводка.
	chatLastActive["alpha"] = time.Now().Add(-500 * time.Minute)

	rr := chatRequest(t, handleChatState, http.MethodGet, chatStatePath+"?world=alpha", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/state: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out chatStateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if out.World != "alpha" {
		t.Fatalf("world = %q, want alpha", out.World)
	}
	if out.WorldState["title"] != "alpha" {
		t.Fatalf("world_state.title = %q, want alpha", out.WorldState["title"])
	}
	if out.Hero.PlaceID != 1 || out.Hero.Mood != "спокоен" {
		t.Fatalf("hero = %+v, want place_id 1 и mood спокоен", out.Hero)
	}
	if out.System.Mode != "normal" {
		t.Fatalf("system.mode = %q, want normal", out.System.Mode)
	}
	if out.ReturnSummary == "" {
		t.Fatal("return_summary пуст при возврате")
	}

	// После первой реплики сводка гаснет, ход попадает в журнал с id сцены.
	rr = chatRequest(t, handleChatTurn, http.MethodPost, chatTurnPath, `{"world":"alpha","text":"Я вернулся."}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/turn: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	rr = chatRequest(t, handleChatState, http.MethodGet, chatStatePath+"?world=alpha", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/state после хода: status = %d, want 200", rr.Code)
	}
	var after chatStateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &after); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if after.ReturnSummary != "" {
		t.Fatalf("return_summary = %q после первой реплики, want пусто", after.ReturnSummary)
	}
	if len(after.Turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(after.Turns))
	}
	if after.Turns[0].SceneID <= 0 {
		t.Fatalf("scene_id = %d, want > 0 для сигналов", after.Turns[0].SceneID)
	}
}

// TestSceneSignal — сигнал сцены пишется в scene_signals.
func TestSceneSignal(t *testing.T) {
	seedChatWorld(t)

	rr := chatRequest(t, handleChatSignal, http.MethodPost, "/api/scene/7/signal", `{"kind":"like"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/scene/7/signal (like): status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	rr = chatRequest(t, handleChatSignal, http.MethodPost, "/api/scene/8/signal", `{"kind":"dislike"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/scene/8/signal (dislike): status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	reader, err := db.OpenReader(worldDBPath("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	var n int
	if err := reader.QueryRow(`SELECT COUNT(*) FROM scene_signals`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("строк scene_signals = %d, want 2", n)
	}
	var kind string
	if err := reader.QueryRow(`SELECT kind FROM scene_signals WHERE scene_id = 7`).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind != "like" {
		t.Fatalf("kind сцены 7 = %q, want like", kind)
	}

	if rr := chatRequest(t, handleChatSignal, http.MethodPost, "/api/scene/7/signal", `{"kind":"neutral"}`); rr.Code != http.StatusBadRequest {
		t.Fatalf("kind=neutral: status = %d, want 400", rr.Code)
	}
	if rr := chatRequest(t, handleChatSignal, http.MethodPost, "/api/scene/x/signal", `{"kind":"like"}`); rr.Code != http.StatusBadRequest {
		t.Fatalf("неверный id: status = %d, want 400", rr.Code)
	}
}

// TestTurnStream — ответ мира доставляется порциями по WS turn.stream; обрыв WS
// не теряет ход.
func TestTurnStream(t *testing.T) {
	path := seedChatWorld(t)

	h := NewHandler(Config{})
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, br := dialTestWS(t, srv.URL, []string{turnStreamTopic})
	defer conn.Close()

	// Ждём, пока сервер обработает подписку на turn.stream.
	deadline := time.Now().Add(2 * time.Second)
	for !wsSubscribed(chatHub, turnStreamTopic) {
		if time.Now().After(deadline) {
			t.Fatal("клиент не подписался на turn.stream")
		}
		time.Sleep(2 * time.Millisecond)
	}

	resp, err := srv.Client().Post(srv.URL+chatTurnPath, "application/json",
		strings.NewReader(`{"world":"alpha","text":"Он идёт. Ветер стих. Дорога пуста."}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/turn: status = %d, want 200; body=%s", resp.StatusCode, body)
	}
	var turnRes struct {
		TurnID int64 `json:"turn_id"`
	}
	if err := json.Unmarshal(body, &turnRes); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if turnRes.TurnID <= 0 {
		t.Fatalf("turn_id = %d, want > 0", turnRes.TurnID)
	}

	chunks := collectTurnChunks(t, conn, br)
	if len(chunks) == 0 {
		t.Fatal("нет порций по WS turn.stream")
	}
	var joined string
	for _, c := range chunks {
		if c.TurnID != turnRes.TurnID {
			t.Errorf("chunk turn_id = %d, want %d", c.TurnID, turnRes.TurnID)
		}
		joined += c.Text
	}

	reader, err := db.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	var reply string
	if err := reader.QueryRow(`SELECT reply_text FROM turns WHERE id = ?`, turnRes.TurnID).Scan(&reply); err != nil {
		_ = reader.Close()
		t.Fatal(err)
	}
	_ = reader.Close()
	if joined != reply {
		t.Fatalf("порции по WS = %q, want reply %q", joined, reply)
	}

	// Обрыв WS не теряет ход: переподключение получает текущее состояние.
	_ = conn.Close()
	rr := chatRequest(t, handleChatState, http.MethodGet, chatStatePath+"?world=alpha", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/state после обрыва: status = %d, want 200", rr.Code)
	}
	var st chatStateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &st); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(st.Turns) != 1 || st.Turns[0].TurnID != turnRes.TurnID {
		t.Fatalf("turns = %+v, want ход %d", st.Turns, turnRes.TurnID)
	}
}
