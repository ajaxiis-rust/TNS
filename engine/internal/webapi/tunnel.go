package webapi

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"

	"tns65/internal/journal"
	"tns65/internal/netgate"
)

// DefaultTunnelEdge и DefaultTunnelLocal — параметры исходящего туннеля по
// умолчанию (F-04.3): край Cloudflare и loopback-адрес веб-интерфейса.
const (
	DefaultTunnelEdge  = "region1.v2.argotunnel.com"
	DefaultTunnelLocal = "http://127.0.0.1:8720"
)

// telegramAPIHost — хост Telegram Bot API. Бот ходит наружу только исходящим
// соединением (long poll getUpdates / sendMessage), входящих сокетов нет (F-04.4).
const telegramAPIHost = "api.telegram.org"

// chatPath — маршрут loopback-сервера, в который бот пересылает реплику из
// Telegram; активный мир привязывает хэндлер чата (T-08-15).
const chatPath = "/api/turn"

// newsTopic — топик WS loopback-сервера, по которому бот забирает вести станка
// (исходящее подключение к /ws, входящих сокетов у бота нет).
const newsTopic = "station.news"

// События подключения каналов (Писатель T-08-03: события подключения — в
// журнал станка). Имена событий — константы, строковые литералы запрещены
// (T-01-03, шаг 2).
const (
	eventTunnelConnect   = "tunnel.connect"
	eventTelegramConnect = "telegram.connect"
)

// ErrExternalBind — в конфигурации туннеля задан внешний слушающий адрес.
// Туннель работает только на исходящих соединениях (F-04.3, F-04.4).
var ErrExternalBind = errors.New("webapi: tunnel has external bind address")

// ErrTelegramNotConfigured — бот не настроен: нет токена или chat_id.
var ErrTelegramNotConfigured = errors.New("webapi: telegram bot is not configured")

// ErrNoActiveWorld — в интерфейсе не выбран активный мир, поэтому реплику из
// Telegram некуда передать. Бот продолжает long poll и сообщает игроку, что
// сначала нужно выбрать мир в Атласе.
var ErrNoActiveWorld = errors.New("webapi: no active world for telegram chat")

// TunnelConfig — конфигурация исходящего туннеля cloudflared (секция tunnel).
// Edge — край, к которому станок подключается наружу; Local — loopback-адрес
// веб-интерфейса, который туннель пробрасывает. Поля слушающего адреса нет.
// TelegramToken и TelegramChatID — исходящие параметры Telegram-канала (F-04.4):
// бот шлёт getUpdates/sendMessage наружу, никаких входящих сокетов.
type TunnelConfig struct {
	Edge           string // край туннеля (hostname; без схемы).
	Local          string // локальный URL, который туннель пробрасывает.
	Bind           string // внешний слушающий адрес — запрещён, должен быть пустым.
	TelegramToken  string // токен Telegram-бота; пусто — канал не настроен.
	TelegramChatID string // chat_id, куда бот пересылает чат и вести станка.
}

// DefaultTunnelConfig возвращает параметры, совпадающие со значениями config.yaml.
func DefaultTunnelConfig() TunnelConfig {
	return TunnelConfig{
		Edge:  DefaultTunnelEdge,
		Local: DefaultTunnelLocal,
	}
}

// Validate проверяет, что туннель исходящий: внешнего слушающего адреса нет,
// а пробрасываемый адрес — только loopback (F-04.3, F-04.4).
func (c TunnelConfig) Validate() error {
	if strings.TrimSpace(c.Bind) != "" {
		return ErrExternalBind
	}
	if c.Local != "" {
		if host := localHost(c.Local); host != "" && !hostIsLoopback(host) {
			return ErrExternalBind
		}
	}
	return nil
}

// localHost выделяет имя хоста из пробрасываемого адреса: URL
// ("http://host:port") или "host:port". Пустая строка — адрес без хоста.
func localHost(local string) string {
	local = strings.TrimPrefix(strings.TrimPrefix(local, "http://"), "https://")
	host, _, err := net.SplitHostPort(local)
	if err == nil {
		return strings.Trim(host, "[]")
	}
	addr, err := netip.ParseAddr(local)
	if err != nil {
		return ""
	}
	return addr.String()
}

// Command возвращает аргументы cloudflared для исходящего туннеля: станок сам
// подключается к краю и пробрасывает loopback-адрес веб-интерфейса (F-04.3).
func (c TunnelConfig) Command() []string {
	edge := c.Edge
	if edge == "" {
		edge = DefaultTunnelEdge
	}
	local := c.Local
	if local == "" {
		local = DefaultTunnelLocal
	}
	return []string{"cloudflared", "tunnel", "--edge", edge, "--url", local}
}

// hostIsLoopback сообщает, является ли адрес loopback (127.0.0.0/8 или ::1).
func hostIsLoopback(host string) bool {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return addr.IsLoopback()
}

// LoadTunnelConfig читает секцию tunnel из файла конфигурации.
func LoadTunnelConfig(path string) (TunnelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TunnelConfig{}, err
	}
	return parseTunnel(string(data))
}

// LoadBotConfig читает секцию tunnel и подмешивает токен Telegram из
// settings.toml (ключ telegram), если в config.yaml токен не задан: токен в UI
// (W2) пишется в settings.toml, config.yaml хранит прежний fallback.
func LoadBotConfig(tunnelPath, settingsPath string) (TunnelConfig, error) {
	cfg, err := LoadTunnelConfig(tunnelPath)
	if err != nil {
		return TunnelConfig{}, err
	}
	if cfg.TelegramToken == "" {
		cfg.TelegramToken = readSettingsKeys(settingsPath)["telegram"]
	}
	return cfg, nil
}

// unquoteScalar снимает одну пару кавычек ("…" или '…') со скаляра после
// обрезки инлайн-комментария: пустые кавычки становятся пустой строкой,
// значение без кавычек не меняется.
func unquoteScalar(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// parseTunnel разбирает топ-ключ `tunnel:` с полями edge, local, bind,
// telegram_token и telegram_chat_id. Наличие ключа bind (внешний слушающий
// адрес) возвращает ErrExternalBind — туннель исходящий, слушающих адресов у
// него нет (F-04.3). Токен и chat_id — исходящие параметры (F-04.4).
func parseTunnel(data string) (TunnelConfig, error) {
	cfg := DefaultTunnelConfig()

	inTunnel := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inTunnel = trimmed == "tunnel:"
			continue
		}
		if !inTunnel {
			continue
		}

		key, val, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = unquoteScalar(strings.TrimSpace(strings.SplitN(strings.TrimSpace(val), "#", 2)[0]))

		switch key {
		case "edge":
			cfg.Edge = val
		case "local":
			cfg.Local = val
		case "telegram_token":
			cfg.TelegramToken = val
		case "telegram_chat_id":
			cfg.TelegramChatID = val
		case "bind":
			return TunnelConfig{}, ErrExternalBind
		}
	}
	if err := sc.Err(); err != nil {
		return TunnelConfig{}, err
	}
	return cfg, nil
}

// LogTunnelConnect пишет событие подключения туннеля в журнал станка
// (Писатель T-08-03). Поля — только исходящие параметры: край и loopback-адрес.
func LogTunnelConnect(logger *journal.Logger, cfg TunnelConfig) {
	if logger == nil {
		return
	}
	_ = logger.Event("tunnel", eventTunnelConnect,
		journal.Field{Key: "edge", Value: cfg.Edge},
		journal.Field{Key: "local", Value: cfg.Local})
}

// LogTelegramConnect пишет событие подключения Telegram-канала в журнал станка.
// Режим подписки — long poll (исходящее соединение, без входящих сокетов).
func LogTelegramConnect(logger *journal.Logger, cfg TunnelConfig) {
	if logger == nil {
		return
	}
	_ = logger.Event("telegram", eventTelegramConnect,
		journal.Field{Key: "local", Value: cfg.Local},
		journal.Field{Key: "mode", Value: "long_poll"})
}

// TelegramBot — исходящий Telegram-канал, решающий NAT без входящих сокетов
// (F-04.4, T-08-03 шаг 2): подписка на сообщения — long poll getUpdates,
// пересылка реплик чата — в loopback-сервер, пересылка вестей станка — в
// Telegram. Связь с loopback-сервером — только через локальный порт 8720
// (шаг 3); наружу уходит только исходящее.
type TelegramBot struct {
	cfg          TunnelConfig
	logger       *journal.Logger
	telegramBase string
	localBase    string
	telegram     *http.Client
	local        *http.Client
	dial         func(ctx context.Context, network, addr string) (net.Conn, error)
}

// NewTelegramBot собирает бота на исходящих клиентах: наружу — через единый
// сетевой гейт (netgate.Client, F-05), к loopback-серверу — напрямую
// (netgate.Local, F-04.2).
func NewTelegramBot(cfg TunnelConfig, logger *journal.Logger) *TelegramBot {
	return &TelegramBot{
		cfg:          cfg,
		logger:       logger,
		telegramBase: "https://" + telegramAPIHost,
		localBase:    cfg.Local,
		telegram:     netgate.Client(telegramAPIHost),
		local:        netgate.Local(localAddr(cfg.Local)),
		dial:         (&net.Dialer{}).DialContext,
	}
}

// localAddr выделяет "host:port" из loopback-URL для netgate.Local.
func localAddr(local string) string {
	if u, err := url.Parse(local); err == nil && u.Host != "" {
		return u.Host
	}
	return strings.TrimPrefix(strings.TrimPrefix(local, "http://"), "https://")
}

// newTelegramBot — фабрика бота. Переменная, чтобы тест подменял Telegram- и
// loopback-клиентов на httptest-фейки без живой сети.
var newTelegramBot = NewTelegramBot

// TelegramRestart — хук перезапуска Telegram-канала после изменения настроек
// канала. Выставляется main после первого StartTelegram; в тестах остаётся nil,
// и хэндлер PUT /api/settings/channel его не вызывает.
var TelegramRestart func(context.Context) error

// StopTelegram — отмена текущего канала: гасит контекст, переданный в
// StartTelegram. Выставляется main вместе с TelegramRestart.
var StopTelegram func()

// StartTelegram запускает исходящий Telegram-канал (F-04.4): long poll реплик
// (Poll) и подписку на вести станка (SubscribeNews) в отдельных горутинах на
// производном отменяемом контексте. Возвращает ErrTelegramNotConfigured, когда
// токен или chat_id не заданы. Событие telegram.connect пишется один раз на
// старт канала; флаг botRunning (экран Настроек, W2) поднимается на старте и
// снимается, когда ctx отменён.
func StartTelegram(ctx context.Context, cfg TunnelConfig, logger *journal.Logger) error {
	if strings.TrimSpace(cfg.TelegramToken) == "" || strings.TrimSpace(cfg.TelegramChatID) == "" {
		return ErrTelegramNotConfigured
	}

	bot := newTelegramBot(cfg, logger)
	runCtx, cancel := context.WithCancel(ctx)
	gen := botGeneration.Add(1)
	botRunning.Store(true)
	LogTelegramConnect(logger, cfg)

	go func() {
		_ = bot.Poll(runCtx)
	}()
	go func() {
		_ = bot.SubscribeNews(runCtx, func(msg WSMessage) error {
			var p struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				return nil
			}
			return bot.SendNews(runCtx, p.Text)
		})
	}()

	go func() {
		<-ctx.Done()
		cancel()
		// Флаг снимает только текущее поколение канала: при перезапуске старый
		// бот не должен перекрывать botRunning у нового.
		if botGeneration.Load() == gen {
			botRunning.Store(false)
		}
	}()

	return nil
}

// SendChat пересылает сообщение чата из игры в Telegram (loopback → Telegram).
func (b *TelegramBot) SendChat(ctx context.Context, text string) error {
	return b.sendTelegram(ctx, text)
}

// SendNews пересылает весть станка в Telegram (loopback → Telegram).
func (b *TelegramBot) SendNews(ctx context.Context, text string) error {
	return b.sendTelegram(ctx, text)
}

// chatTurnResponse — тело ответа POST /api/turn: номер закоммиченного хода.
type chatTurnResponse struct {
	TurnID int64 `json:"turn_id"`
}

// ForwardChat пересылает реплику из Telegram в чат loopback-сервера
// (Telegram → loopback, локальный порт 8720) и возвращает номер хода.
// Пустой ответ без turn_id — не ошибка: вызывающий сообщит игроку честно.
func (b *TelegramBot) ForwardChat(ctx context.Context, text string) (int64, error) {
	slug := getActiveSlug()
	if slug == "" {
		return 0, ErrNoActiveWorld
	}
	body, err := json.Marshal(map[string]string{"world": slug, "text": text})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(b.localBase, "/")+chatPath, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.local.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("webapi: forward chat: status %d", resp.StatusCode)
	}
	var out chatTurnResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	return out.TurnID, nil
}

// telegramUpdate — одно обновление long poll getUpdates (Telegram Bot API).
// Update может нести сообщение (message) или нажатие inline-кнопки
// (callback_query) — оба обрабатываются, пока чат свой.
type telegramUpdate struct {
	UpdateID      int64                  `json:"update_id"`
	Message       *telegramMessage       `json:"message"`
	CallbackQuery *telegramCallbackQuery `json:"callback_query"`
}

// telegramCallbackQuery — нажатие inline-кнопки (W3). ID — идентификатор
// callback-запроса для answerCallbackQuery, Data — callback_data кнопки.
type telegramCallbackQuery struct {
	ID      string           `json:"id"`
	Data    string           `json:"data"`
	Message *telegramMessage `json:"message"`
}

// telegramMessage — входящее сообщение бота.
type telegramMessage struct {
	Text string        `json:"text"`
	Chat *telegramChat `json:"chat"`
}

// telegramChat — чат сообщения Telegram Bot API. Бот отвечает только своему
// chat_id (W2): чужой чат игнорируется, но offset всё равно двигается.
type telegramChat struct {
	ID int64 `json:"id"`
}

// GetUpdates выполняет один исходящий запрос getUpdates (long poll). Подписка
// на сообщения — только исходящими запросами, webhook-входящих сокетов нет
// (F-04.4).
func (b *TelegramBot) GetUpdates(ctx context.Context, offset int64) ([]telegramUpdate, error) {
	u := b.telegramBase + "/bot/" + b.cfg.TelegramToken + "/getUpdates"
	if offset > 0 {
		u += "?offset=" + strconv.FormatInt(offset, 10)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.telegram.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var res struct {
		OK     bool             `json:"ok"`
		Result []telegramUpdate `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	if !res.OK {
		return nil, fmt.Errorf("webapi: telegram getUpdates: ok=false")
	}
	return res.Result, nil
}

// PollOnce забирает одну пачку обновлений и пересылает реплики в чат loopback.
// Возвращает следующий offset для продолжения long poll. Ошибка передачи одной
// реплики (loopback или Telegram) не роняет цикл — канал продолжает опрос.
func (b *TelegramBot) PollOnce(ctx context.Context, offset int64) (int64, error) {
	updates, err := b.GetUpdates(ctx, offset)
	if err != nil {
		return offset, err
	}
	for _, u := range updates {
		switch {
		case u.CallbackQuery != nil:
			if b.fromOwnCallback(u.CallbackQuery) {
				b.relayCallback(ctx, u.CallbackQuery)
			}
		case u.Message != nil:
			if b.fromOwnChat(u.Message) {
				b.relayChat(ctx, u.Message.Text)
			}
		}
		if u.UpdateID >= offset {
			offset = u.UpdateID + 1
		}
	}
	return offset, nil
}

// fromOwnChatID сообщает, что чат — настроенный chat_id бота. Общий страж для
// сообщений и callback-запросов (W2/W3): чужой чат игнорируется молча.
func (b *TelegramBot) fromOwnChatID(chat *telegramChat) bool {
	if chat == nil {
		return false
	}
	return strconv.FormatInt(chat.ID, 10) == strings.TrimSpace(b.cfg.TelegramChatID)
}

// fromOwnChat сообщает, что сообщение пришло из настроенного chat_id бота и не
// пусто. Чужой чат и сообщение без чата игнорируются молча (W2): кто бы ни нашёл
// бота, движок он не дёргает.
func (b *TelegramBot) fromOwnChat(m *telegramMessage) bool {
	if m == nil || strings.TrimSpace(m.Text) == "" {
		return false
	}
	return b.fromOwnChatID(m.Chat)
}

// fromOwnCallback сообщает, что callback-запрос пришёл из настроенного chat_id
// бота (W3). Чужой чат игнорируется молча — тот же страж, что и для сообщений.
func (b *TelegramBot) fromOwnCallback(q *telegramCallbackQuery) bool {
	if q == nil || q.Message == nil {
		return false
	}
	return b.fromOwnChatID(q.Message.Chat)
}

// signalDataPrefix и сигналы сцены в callback_data inline-кнопок.
const (
	signalDataPrefix = "signal:"
	signalLike       = "like"
	signalDislike    = "dislike"
)

// parseSignalData разбирает callback_data "signal:<scene_id>:like|dislike".
// Неизвестные данные возвращают ok=false — бот молча их игнорирует (W3).
func parseSignalData(data string) (sceneID int64, kind string, ok bool) {
	rest := strings.TrimPrefix(data, signalDataPrefix)
	if rest == data {
		return 0, "", false
	}
	idStr, kind, ok := strings.Cut(rest, ":")
	if !ok {
		return 0, "", false
	}
	sceneID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || sceneID <= 0 {
		return 0, "", false
	}
	if kind != signalLike && kind != signalDislike {
		return 0, "", false
	}
	return sceneID, kind, true
}

// relayCallback обрабатывает нажатие inline-кнопки лайк/дизлайк (W3): сигнал
// уходит в loopback POST /api/scene/{id}/signal, а игроку — короткий ответ
// answerCallbackQuery («принято»/«ошибка»). Клавиатура остаётся, новое сообщение
// не отправляется — мир не отвечает второй раз и ничего не дублируется.
func (b *TelegramBot) relayCallback(ctx context.Context, q *telegramCallbackQuery) {
	sceneID, kind, ok := parseSignalData(q.Data)
	if !ok {
		return
	}
	if err := b.loopbackJSON(ctx, http.MethodPost,
		chatSignalPrefix+strconv.FormatInt(sceneID, 10)+"/signal",
		chatSignalRequest{Kind: kind}, nil); err != nil {
		_ = b.answerCallback(ctx, q.ID, "ошибка")
		return
	}
	_ = b.answerCallback(ctx, q.ID, "принято")
}

// answerCallback — одно исходящее answerCallbackQuery (Telegram Bot API): текст
// всплывает как короткое уведомление, не создавая нового сообщения в чате.
func (b *TelegramBot) answerCallback(ctx context.Context, callbackID, text string) error {
	if b.cfg.TelegramToken == "" {
		return ErrTelegramNotConfigured
	}
	u := b.telegramBase + "/bot/" + b.cfg.TelegramToken + "/answerCallbackQuery"
	form := url.Values{}
	form.Set("callback_query_id", callbackID)
	form.Set("text", text)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := b.telegram.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("webapi: telegram answerCallbackQuery: status %d", resp.StatusCode)
	}
	return nil
}

// relayChat передаёт одну реплику в мир и возвращает ответ мира в Telegram.
// Ошибки loopback и Telegram не роняют long poll: канал продолжает опрос, а
// игроку при сбое приходит короткое честное сообщение вместо тишины.
func (b *TelegramBot) relayChat(ctx context.Context, text string) {
	if strings.HasPrefix(strings.TrimSpace(text), "/") {
		b.handleCommand(ctx, text)
		return
	}

	turnID, err := b.ForwardChat(ctx, text)
	switch {
	case errors.Is(err, ErrNoActiveWorld):
		_ = b.SendChat(ctx, "Сначала выберите мир в Атласе.")
		return
	case err != nil:
		_ = b.SendChat(ctx, "Не получилось передать реплику миру, попробуйте ещё раз.")
		return
	case turnID == 0:
		_ = b.SendChat(ctx, "Мир принял реплику, но не вернул номер хода.")
		return
	}

	item, found, err := b.turnReply(ctx, turnID)
	if err != nil || !found || strings.TrimSpace(item.ReplyText) == "" {
		return
	}
	_ = b.sendReply(ctx, item.ReplyText, item.SceneID)
}

// Poll — цикл исходящего long poll: подписка на Telegram без входящих сокетов.
// Событие подключения пишет StartTelegram (один раз на старт канала).
func (b *TelegramBot) Poll(ctx context.Context) error {
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		next, err := b.PollOnce(ctx, offset)
		if err != nil {
			return err
		}
		offset = next
	}
}

// SubscribeNews подключается к loopback-серверу исходящим WebSocket и
// подписывается на топик вестей станка; каждую весть передаёт в onNews.
// Входящих сокетов у бота нет — соединение к /ws исходящее (F-04.4, шаг 3).
func (b *TelegramBot) SubscribeNews(ctx context.Context, onNews func(WSMessage) error) error {
	conn, br, err := b.dialWS(ctx, wsURL(b.localBase))
	if err != nil {
		return err
	}
	defer conn.Close()

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stop:
		}
	}()

	if err := writeClientText(conn, `{"type":"subscribe","topics":["`+newsTopic+`"]}`); err != nil {
		return err
	}

	for {
		op, data, err := readServerFrame(br)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		switch op {
		case opClose:
			return nil
		case opText:
			var msg WSMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			if msg.Topic != newsTopic {
				continue
			}
			if err := onNews(msg); err != nil {
				return err
			}
		}
	}
}

// wsURL строит адрес исходящего WebSocket loopback-сервера (/ws).
func wsURL(local string) string {
	u, err := url.Parse(local)
	if err != nil {
		return ""
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/ws"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// dialWS выполняет клиентское рукопожатие WebSocket и возвращает соединение с
// буферизованным читателем для чтения кадров.
func (b *TelegramBot) dialWS(ctx context.Context, wsURL string) (net.Conn, *bufio.Reader, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, nil, err
	}
	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "wss" {
			host = net.JoinHostPort(u.Hostname(), "443")
		} else {
			host = net.JoinHostPort(u.Hostname(), "80")
		}
	}

	conn, err := b.dial(ctx, "tcp", host)
	if err != nil {
		return nil, nil, err
	}
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)

	req := "GET " + u.RequestURI() + " HTTP/1.1\r\n" +
		"Host: " + u.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	if !strings.Contains(statusLine, "101") {
		_ = conn.Close()
		return nil, nil, ErrWSHandshake
	}

	accept := ""
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			return nil, nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if name, val, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(name), "Sec-WebSocket-Accept") {
			accept = strings.TrimSpace(val)
		}
	}
	if accept != wsAccept(key) {
		_ = conn.Close()
		return nil, nil, ErrWSHandshake
	}
	return conn, br, nil
}

// writeClientText пишет текстовый кадр клиента (замаскированный, RFC 6455).
func writeClientText(conn net.Conn, text string) error {
	return writeClientFrame(conn, opText, []byte(text))
}

func writeClientFrame(conn net.Conn, op byte, payload []byte) error {
	n := len(payload)
	var header []byte
	switch {
	case n < 126:
		header = []byte{0x80 | op, 0x80 | byte(n)}
	case n <= 0xFFFF:
		header = make([]byte, 4)
		header[0] = 0x80 | op
		header[1] = 0x80 | 126
		binary.BigEndian.PutUint16(header[2:], uint16(n))
	default:
		header = make([]byte, 10)
		header[0] = 0x80 | op
		header[1] = 0x80 | 127
		binary.BigEndian.PutUint64(header[2:], uint64(n))
	}
	if _, err := conn.Write(header); err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	mask := [4]byte{0x11, 0x22, 0x33, 0x44}
	masked := make([]byte, n)
	for i := range payload {
		masked[i] = payload[i] ^ mask[i%4]
	}
	if _, err := conn.Write(mask[:]); err != nil {
		return err
	}
	_, err := conn.Write(masked)
	return err
}

// readServerFrame читает один кадр сервера (сервер кадры не маскирует).
func readServerFrame(br *bufio.Reader) (byte, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(br, header[:]); err != nil {
		return 0, nil, err
	}
	op := header[0] & 0x0F
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(br, ext[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(br, ext[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > maxFrameSize {
		return 0, nil, errors.New("webapi: websocket frame too large")
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(br, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(br, data); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range data {
			data[i] ^= mask[i%4]
		}
	}
	return op, data, nil
}

// sendTelegram — одно исходящее сообщение в Telegram (sendMessage).
func (b *TelegramBot) sendTelegram(ctx context.Context, text string) error {
	return b.sendTelegramMessage(ctx, text, "")
}

// sendTelegramMessage — исходящее sendMessage с опциональной inline-клавиатурой
// (reply_markup). Текст уходит без parse_mode.
func (b *TelegramBot) sendTelegramMessage(ctx context.Context, text, keyboard string) error {
	if b.cfg.TelegramToken == "" || b.cfg.TelegramChatID == "" {
		return ErrTelegramNotConfigured
	}
	u := b.telegramBase + "/bot/" + b.cfg.TelegramToken + "/sendMessage"
	form := url.Values{}
	form.Set("chat_id", b.cfg.TelegramChatID)
	form.Set("text", text)
	if keyboard != "" {
		form.Set("reply_markup", keyboard)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := b.telegram.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("webapi: telegram sendMessage: status %d", resp.StatusCode)
	}
	return nil
}

// maxTelegramText — верхняя граница одного сообщения Telegram. Режем с запасом
// под лимит 4096, чтобы служебные поля не переполнили запрос.
const maxTelegramText = 4000

// turnReply читает GET /api/state и возвращает строку хода turnID. Если хода в
// журнале уже нет (устарел), found=false — ответ не отправляется.
func (b *TelegramBot) turnReply(ctx context.Context, turnID int64) (item chatTurnItem, found bool, err error) {
	slug := getActiveSlug()
	if slug == "" {
		return chatTurnItem{}, false, ErrNoActiveWorld
	}
	u := strings.TrimSuffix(b.localBase, "/") + chatStatePath + "?world=" + url.QueryEscape(slug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return chatTurnItem{}, false, err
	}
	resp, err := b.local.Do(req)
	if err != nil {
		return chatTurnItem{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return chatTurnItem{}, false, fmt.Errorf("webapi: chat state: status %d", resp.StatusCode)
	}
	var state chatStateResponse
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return chatTurnItem{}, false, err
	}
	for _, t := range state.Turns {
		if t.TurnID == turnID {
			return t, true, nil
		}
	}
	return chatTurnItem{}, false, nil
}

// sendReply отправляет ответ мира в Telegram: длинные ответы режутся на
// сообщения по границам абзацев, клавиатура сигналов сцены прикрепляется к
// последнему сообщению. Ошибку Telegram бот терпит (W3 добавит обработчик).
func (b *TelegramBot) sendReply(ctx context.Context, text string, sceneID int64) error {
	chunks := splitReply(text)
	if len(chunks) == 0 {
		return nil
	}
	for i, chunk := range chunks {
		var keyboard string
		if sceneID > 0 && i == len(chunks)-1 {
			keyboard = sceneKeyboard(sceneID)
		}
		if err := b.sendTelegramMessage(ctx, chunk, keyboard); err != nil {
			return err
		}
	}
	return nil
}

// sceneKeyboard строит inline-клавиатуру сигналов сцены (лайк/дизлайк).
// Обработчик callback_data приедет в W3; W1 лишь прикрепляет клавиатуру.
func sceneKeyboard(sceneID int64) string {
	mk, err := json.Marshal(map[string]any{
		"inline_keyboard": [][]map[string]string{
			{
				{"text": "👍", "callback_data": fmt.Sprintf("signal:%d:like", sceneID)},
				{"text": "👎", "callback_data": fmt.Sprintf("signal:%d:dislike", sceneID)},
			},
		},
	})
	if err != nil {
		return ""
	}
	return string(mk)
}

// splitReply режет текст на куски не длиннее maxTelegramText по границам
// абзацев (перевод строки), а внутри абзаца — по границе слова; слово не рвётся.
func splitReply(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len(text) <= maxTelegramText {
		return []string{text}
	}
	var out []string
	rest := text
	for len(rest) > maxTelegramText {
		cut := strings.LastIndex(rest[:maxTelegramText], "\n")
		if cut <= 0 {
			cut = strings.LastIndex(rest[:maxTelegramText], " ")
		}
		if cut <= 0 {
			cut = maxTelegramText
		}
		out = append(out, strings.TrimSpace(rest[:cut]))
		rest = strings.TrimSpace(rest[cut:])
	}
	if rest != "" {
		out = append(out, rest)
	}
	return out
}
