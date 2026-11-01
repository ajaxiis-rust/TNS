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
		botRunning.Store(false)
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

// ForwardChat пересылает реплику из Telegram в чат loopback-сервера
// (Telegram → loopback, локальный порт 8720).
func (b *TelegramBot) ForwardChat(ctx context.Context, text string) error {
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(b.localBase, "/")+chatPath, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.local.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("webapi: forward chat: status %d", resp.StatusCode)
	}
	return nil
}

// telegramUpdate — одно обновление long poll getUpdates (Telegram Bot API).
type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

// telegramMessage — входящее сообщение бота.
type telegramMessage struct {
	Text string `json:"text"`
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
// Возвращает следующий offset для продолжения long poll.
func (b *TelegramBot) PollOnce(ctx context.Context, offset int64) (int64, error) {
	updates, err := b.GetUpdates(ctx, offset)
	if err != nil {
		return offset, err
	}
	for _, u := range updates {
		if u.Message != nil && u.Message.Text != "" {
			if err := b.ForwardChat(ctx, u.Message.Text); err != nil {
				return offset, err
			}
		}
		if u.UpdateID >= offset {
			offset = u.UpdateID + 1
		}
	}
	return offset, nil
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
	if b.cfg.TelegramToken == "" || b.cfg.TelegramChatID == "" {
		return ErrTelegramNotConfigured
	}
	u := b.telegramBase + "/bot/" + b.cfg.TelegramToken + "/sendMessage"
	form := url.Values{}
	form.Set("chat_id", b.cfg.TelegramChatID)
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
		return fmt.Errorf("webapi: telegram sendMessage: status %d", resp.StatusCode)
	}
	return nil
}
