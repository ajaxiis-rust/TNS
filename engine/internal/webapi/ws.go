package webapi

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

// wsGUID — константа рукопожатия WebSocket (RFC 6455).
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// maxFrameSize — верхняя граница длины кадра (защита от безразмерных кадров).
const maxFrameSize = 1 << 20

// Опкоды кадров WebSocket.
const (
	opText   = 0x1
	opBinary = 0x2
	opClose  = 0x8
	opPing   = 0x9
	opPong   = 0xA
)

// ErrWSHandshake — отказ рукопожатия WebSocket.
var ErrWSHandshake = errors.New("webapi: websocket handshake failed")

// WSMessage — сообщение рассылки: топик экрана, тип события и payload
// (T-08-02, шаг 5). Формат зафиксирован в ROUTES.md.
type WSMessage struct {
	Topic     string          `json:"topic"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
}

// wsSubscribe — клиентская подписка на топики экранов.
type wsSubscribe struct {
	Type   string   `json:"type"`
	Topics []string `json:"topics"`
}

// Hub — WS-хаб: подключения, подписки на топики экранов и рассылка.
type Hub struct {
	mu      sync.Mutex
	clients map[*client]struct{}
}

// client — одно WebSocket-подключение.
type client struct {
	hub    *Hub
	conn   net.Conn
	br     *bufio.Reader
	mu     sync.Mutex
	topics map[string]struct{}
}

// NewHub создаёт пустой WS-хаб.
func NewHub() *Hub {
	return &Hub{clients: map[*client]struct{}{}}
}

// Close закрывает все подключения хаба.
func (h *Hub) Close() {
	h.mu.Lock()
	clients := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()
	for _, c := range clients {
		c.close()
	}
}

// Broadcast рассылает сообщение клиентам, подписавшимся на топик сообщения.
func (h *Hub) Broadcast(msg WSMessage) {
	h.mu.Lock()
	clients := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()
	for _, c := range clients {
		c.send(msg)
	}
}

func (h *Hub) add(c *client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// serveHTTP — обработчик GET /ws: рукопожатие и чтение подписок.
func (h *Hub) serveHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgradeWS(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "websocket upgrade failed")
		return
	}
	c := &client{
		hub:    h,
		conn:   conn,
		br:     bufio.NewReader(conn),
		topics: map[string]struct{}{},
	}
	h.add(c)
	go c.readLoop()
}

// upgradeWS выполняет серверное рукопожатие WebSocket (RFC 6455) и возвращает
// hijack-нутое соединение.
func upgradeWS(w http.ResponseWriter, r *http.Request) (net.Conn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, ErrWSHandshake
	}
	if !headerHasToken(r.Header.Get("Connection"), "upgrade") {
		return nil, ErrWSHandshake
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, ErrWSHandshake
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, ErrWSHandshake
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + wsAccept(key) + "\r\n\r\n"
	if _, err := rw.WriteString(resp); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if sw, ok := w.(*statusWriter); ok {
		sw.status = http.StatusSwitchingProtocols
	}
	return conn, nil
}

func wsAccept(key string) string {
	sum := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func headerHasToken(header, token string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

// readLoop читает кадры клиента до закрытия; подписка приходит текстовым
// фреймом {"type":"subscribe","topics":[...]}.
func (c *client) readLoop() {
	defer func() {
		c.hub.remove(c)
		_ = c.conn.Close()
	}()
	for {
		op, data, err := c.readFrame()
		if err != nil {
			return
		}
		switch op {
		case opClose:
			_ = c.writeFrame(opClose, nil)
			return
		case opPing:
			_ = c.writeFrame(opPong, data)
		case opText:
			var sub wsSubscribe
			if err := json.Unmarshal(data, &sub); err == nil && sub.Type == "subscribe" {
				c.setTopics(sub.Topics)
			}
		case opBinary:
			// бинарные кадры интерфейсом не используются
		}
	}
}

func (c *client) setTopics(topics []string) {
	c.mu.Lock()
	c.topics = make(map[string]struct{}, len(topics))
	for _, t := range topics {
		c.topics[t] = struct{}{}
	}
	c.mu.Unlock()
}

// send шлёт сообщение, если клиент подписан на его топик.
func (c *client) send(msg WSMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.topics[msg.Topic]; !ok {
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	_ = c.writeFrameLocked(opText, data)
}

func (c *client) close() {
	_ = c.conn.Close()
}

// readFrame читает один кадр клиента (кадры клиента обязаны быть замаскированы).
func (c *client) readFrame() (byte, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(c.br, header[:]); err != nil {
		return 0, nil, err
	}
	op := header[0] & 0x0F
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > maxFrameSize {
		return 0, nil, errors.New("webapi: websocket frame too large")
	}
	if !masked {
		return 0, nil, errors.New("webapi: unmasked client frame")
	}
	var mask [4]byte
	if _, err := io.ReadFull(c.br, mask[:]); err != nil {
		return 0, nil, err
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(c.br, data); err != nil {
		return 0, nil, err
	}
	for i := range data {
		data[i] ^= mask[i%4]
	}
	return op, data, nil
}

// writeFrame пишет один кадр сервера (сервер кадры не маскирует).
func (c *client) writeFrame(op byte, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeFrameLocked(op, payload)
}

func (c *client) writeFrameLocked(op byte, payload []byte) error {
	n := len(payload)
	var header []byte
	switch {
	case n < 126:
		header = []byte{0x80 | op, byte(n)}
	case n <= 0xFFFF:
		header = make([]byte, 4)
		header[0] = 0x80 | op
		header[1] = 126
		binary.BigEndian.PutUint16(header[2:], uint16(n))
	default:
		header = make([]byte, 10)
		header[0] = 0x80 | op
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:], uint64(n))
	}
	if _, err := c.conn.Write(header); err != nil {
		return err
	}
	if n > 0 {
		if _, err := c.conn.Write(payload); err != nil {
			return err
		}
	}
	return nil
}
