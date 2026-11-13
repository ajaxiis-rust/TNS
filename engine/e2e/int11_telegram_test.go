//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestINT11 — INT-11. Telegram-канал (F-04.4, W2/W3): канал настраивается через
// настройки, бот ходит в локальный фейк Bot API, чужие chat_id игнорируются,
// реплика из своего чата доходит до мира, ответ мира уходит в Telegram с inline
// лайк/дизлайк, команда /world отвечает текущим миром. Живой Telegram не нужен.
func TestINT11(t *testing.T) {
	bin := buildBinary(t, []string{"e2e"})
	dir := worldDir(t)
	copyConfig(t, dir)
	linkPacks(t, dir)

	const (
		slug  = "telegram-kanal"
		token = "e2e-telegram-token"
		chat  = int64(42)
		other = int64(999)
	)

	// Подставной слот Ткача: штатная проза с маркером, как в INT-3.
	slotSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/completion" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		for _, line := range deepSceneProse() {
			fmt.Fprintln(w, line)
		}
	}))
	defer slotSrv.Close()

	fakeTg, tgSrv := newFakeTelegram(t, token)

	ancketa := writeAncketa(t, t.TempDir(), slug,
		"Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.")
	if stdout, stderr, code := runCmd(t, dir, bin, "world", "birth", "--ancketa", ancketa, "--fake-embed", "--fake-slot"); code != 0 {
		t.Fatalf("world birth exit=%d, want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	c := startCore(t, dir, bin, "--fake-slot", slotSrv.URL, "--fake-telegram", tgSrv.URL)
	worldDB := worldDBPath(dir, slug)
	report := map[string]any{}

	// Шаг 1: активный мир выбирается через Атлас — общий с ботом in-process слаг.
	var active struct {
		Active string `json:"active"`
	}
	if code := coreJSON(t, c, http.MethodPost, "/api/atlas/worlds/active",
		map[string]string{"world": slug}, &active); code != http.StatusOK || active.Active != slug {
		t.Fatalf("active world: status=%d active=%q, want 200/%q", code, active.Active, slug)
	}

	// Шаг 2: канал настраивается через PUT /api/settings/channel; бот стартует и
	// начинает long poll фейка — живой Telegram не нужен.
	var ch struct {
		Status  string `json:"status"`
		Warning string `json:"warning"`
	}
	if code := coreJSON(t, c, http.MethodPut, "/api/settings/channel", map[string]string{
		"edge":             "region1.v2.argotunnel.com",
		"local":            "http://127.0.0.1:8720",
		"telegram_chat_id": "42",
		"telegram_token":   token,
	}, &ch); code != http.StatusOK || ch.Status != "ok" {
		t.Fatalf("PUT channel: status=%d body=%+v, want 200/ok", code, ch)
	}
	report["channel_warning"] = ch.Warning
	waitForCondition(t, "бот не начал long poll", 5*time.Second, func() bool {
		return fakeTg.pollCount() > 0
	})
	report["channel_started"] = true

	// Шаг 3: чужой chat_id игнорируется — ни хода, ни сообщения в Telegram.
	beforeTurns := turnsCount(t, worldDB)
	beforeSent := fakeTg.sentCount()
	polls := fakeTg.pollCount()
	fakeTg.pushMessage(other, "Иду, еду, плыву, бегу.")
	waitForCondition(t, "чужое сообщение не прошло опрос", 5*time.Second, func() bool {
		return fakeTg.pollCount() > polls
	})
	time.Sleep(150 * time.Millisecond)
	report["foreign_chat_ignored"] = fakeTg.sentCount() == beforeSent && turnsCount(t, worldDB) == beforeTurns
	if !report["foreign_chat_ignored"].(bool) {
		t.Fatal("чужой chat_id не игнорируется: появился ход или сообщение")
	}

	// Шаг 4: реплика из своего чата доходит до мира, ответ мира уходит в Telegram
	// с inline-клавиатурой лайк/дизлайк.
	fakeTg.pushMessage(chat, "Иду, еду, плыву, бегу.")
	waitForCondition(t, "ход из Telegram не закоммичен", 10*time.Second, func() bool {
		return turnsCount(t, worldDB) == beforeTurns+1
	})
	reply := lastTurns(t, worldDB, 1)[0].ReplyText
	report["turn_committed"] = strings.TrimSpace(reply) != ""
	if !report["turn_committed"].(bool) {
		t.Fatal("ответ мира пуст")
	}
	if sent := fakeTg.waitText(t, "тропе", 10*time.Second); sent.Text == "" {
		t.Fatal("ответ мира не доставлен в Telegram")
	}
	report["reply_delivered"] = true
	if !fakeTg.hasKeyboardSignal() {
		t.Fatal("в Telegram нет inline-клавиатуры лайк/дизлайк")
	}
	report["inline_keyboard"] = true

	// Шаг 5: команда /world отвечает текущим миром.
	fakeTg.pushMessage(chat, "/world")
	cmd := fakeTg.waitText(t, "Текущий мир:", 10*time.Second)
	report["world_command"] = strings.Contains(cmd.Text, slug)
	if !report["world_command"].(bool) {
		t.Fatalf("/world ответ = %q, want текущий мир %q", cmd.Text, slug)
	}

	// Шаг 6: inline-лайк уходит сигналом сцены в мир, игрок получает «принято».
	sceneID := lastSceneID(t, worldDB)
	if sceneID == 0 {
		t.Fatal("у хода нет сцены: сигнал лайка некуда отправить")
	}
	fakeTg.pushCallback(chat, "cb-like", fmt.Sprintf("signal:%d:like", sceneID))
	answer := fakeTg.waitAnswer(t, "cb-like", 10*time.Second)
	report["like_answered"] = strings.TrimSpace(answer) == "принято"
	if !report["like_answered"].(bool) {
		t.Fatalf("answerCallbackQuery = %q, want «принято»", answer)
	}
	report["like_signal"] = sceneSignals(t, worldDB, sceneID, "like") == 1
	if !report["like_signal"].(bool) {
		t.Fatal("сигнал лайка не записан в scene_signals")
	}

	// Шаг 7: callback из чужого чата игнорируется — ответа и сигнала нет.
	polls = fakeTg.pollCount()
	fakeTg.pushCallback(other, "cb-foreign", fmt.Sprintf("signal:%d:dislike", sceneID))
	waitForCondition(t, "чужой callback не прошёл опрос", 5*time.Second, func() bool {
		return fakeTg.pollCount() > polls
	})
	time.Sleep(150 * time.Millisecond)
	report["foreign_callback_ignored"] = !fakeTg.hasAnswer("cb-foreign") &&
		sceneSignals(t, worldDB, sceneID, "dislike") == 0
	if !report["foreign_callback_ignored"].(bool) {
		t.Fatal("callback чужого чата не игнорируется")
	}

	writeReport(t, "int11", report)
}

// copyConfig кладёт настоящую копию engine/config.yaml во временный каталог:
// настройка канала пишет tunnel-секцию, поэтому симлинк на репозиторий нельзя.
func copyConfig(t *testing.T, dir string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(engineRoot(t), "config.yaml"))
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), raw, 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
}

// fakeTelegram — локальный фейк Telegram Bot API: очередь обновлений и записи
// исходящих sendMessage/answerCallbackQuery.
type fakeTelegram struct {
	token string

	mu      sync.Mutex
	nextID  int64
	queue   []any
	polls   int64
	sent    []fakeTelegramMessage
	answers []fakeTelegramAnswer
}

// fakeTelegramMessage — исходящий sendMessage фейка.
type fakeTelegramMessage struct {
	ChatID   string
	Text     string
	Keyboard string
}

// fakeTelegramAnswer — исходящий answerCallbackQuery фейка.
type fakeTelegramAnswer struct {
	CallbackID string
	Text       string
}

func newFakeTelegram(t *testing.T, token string) (*fakeTelegram, *httptest.Server) {
	t.Helper()
	f := &fakeTelegram{token: token}
	srv := httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeTelegram) handle(w http.ResponseWriter, r *http.Request) {
	switch strings.TrimPrefix(r.URL.Path, "/bot/"+f.token) {
	case "/getUpdates":
		f.mu.Lock()
		updates := f.queue
		f.queue = nil
		f.polls++
		f.mu.Unlock()
		// Небольшая пауза вместо long poll: бот не крутит запросы вхолостую.
		time.Sleep(10 * time.Millisecond)
		writeFakeTelegram(w, map[string]any{"ok": true, "result": updates})
	case "/sendMessage":
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.sent = append(f.sent, fakeTelegramMessage{
			ChatID: r.Form.Get("chat_id"), Text: r.Form.Get("text"), Keyboard: r.Form.Get("reply_markup"),
		})
		f.mu.Unlock()
		writeFakeTelegram(w, map[string]any{"ok": true, "result": map[string]any{}})
	case "/answerCallbackQuery":
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.answers = append(f.answers, fakeTelegramAnswer{
			CallbackID: r.Form.Get("callback_query_id"), Text: r.Form.Get("text"),
		})
		f.mu.Unlock()
		writeFakeTelegram(w, map[string]any{"ok": true, "result": true})
	default:
		http.NotFound(w, r)
	}
}

func writeFakeTelegram(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// pushMessage ставит входящее сообщение в очередь getUpdates.
func (f *fakeTelegram) pushMessage(chatID int64, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	f.queue = append(f.queue, map[string]any{
		"update_id": f.nextID,
		"message": map[string]any{
			"text": text,
			"chat": map[string]any{"id": chatID},
		},
	})
}

// pushCallback ставит в очередь нажатие inline-кнопки.
func (f *fakeTelegram) pushCallback(chatID int64, callbackID, data string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	f.queue = append(f.queue, map[string]any{
		"update_id": f.nextID,
		"callback_query": map[string]any{
			"id":   callbackID,
			"data": data,
			"message": map[string]any{
				"chat": map[string]any{"id": chatID},
			},
		},
	})
}

func (f *fakeTelegram) pollCount() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.polls
}

func (f *fakeTelegram) sentCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

// waitText ждёт исходящее сообщение, содержащее substr, и возвращает его.
func (f *fakeTelegram) waitText(t *testing.T, substr string, timeout time.Duration) fakeTelegramMessage {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		for _, m := range f.sent {
			if strings.Contains(m.Text, substr) {
				f.mu.Unlock()
				return m
			}
		}
		f.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("в Telegram нет сообщения с %q", substr)
	return fakeTelegramMessage{}
}

// hasKeyboardSignal сообщает, что в исходящих есть inline-клавиатура сигналов.
func (f *fakeTelegram) hasKeyboardSignal() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.sent {
		if strings.Contains(m.Keyboard, "signal:") {
			return true
		}
	}
	return false
}

// waitAnswer ждёт answerCallbackQuery по id и возвращает текст.
func (f *fakeTelegram) waitAnswer(t *testing.T, callbackID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		for _, a := range f.answers {
			if a.CallbackID == callbackID {
				f.mu.Unlock()
				return a.Text
			}
		}
		f.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("нет answerCallbackQuery для %q", callbackID)
	return ""
}

func (f *fakeTelegram) hasAnswer(callbackID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.answers {
		if a.CallbackID == callbackID {
			return true
		}
	}
	return false
}

// waitForCondition ждёт выполнения условия или падает по тайм-ауту.
func waitForCondition(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("тайм-аут: %s", what)
}

// turnsCount — число закоммиченных ходов мира.
func turnsCount(t *testing.T, worldDB string) int {
	t.Helper()
	h := openWorldDB(t, worldDB)
	defer h.Close()
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM turns`).Scan(&n); err != nil {
		t.Fatalf("COUNT turns: %v", err)
	}
	return n
}

// lastSceneID — id последней сцены мира (0 — сцен нет).
func lastSceneID(t *testing.T, worldDB string) int64 {
	t.Helper()
	h := openWorldDB(t, worldDB)
	defer h.Close()
	var id int64
	if err := h.QueryRow(`SELECT id FROM scenes ORDER BY id DESC LIMIT 1`).Scan(&id); err != nil {
		return 0
	}
	return id
}

// sceneSignals — число сигналов сцены заданного вида.
func sceneSignals(t *testing.T, worldDB string, sceneID int64, kind string) int {
	t.Helper()
	h := openWorldDB(t, worldDB)
	defer h.Close()
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM scene_signals WHERE scene_id = ? AND kind = ?`,
		sceneID, kind).Scan(&n); err != nil {
		t.Fatalf("COUNT scene_signals: %v", err)
	}
	return n
}
