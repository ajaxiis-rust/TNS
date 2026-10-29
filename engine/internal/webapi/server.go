package webapi

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"tns65/internal/journal"
)

// DefaultHost и DefaultPort — дефолт строго loopback (T-08-02, шаг 1).
const (
	DefaultHost = "127.0.0.1"
	DefaultPort = 8720
)

// allowedHosts — фиксированная таблица допустимых значений host: только loopback.
var allowedHosts = map[string]struct{}{
	"127.0.0.1": {},
	"localhost": {},
}

// ErrHostNotAllowed — host вне фиксированной таблицы loopback (T-08-02, шаг 1).
var ErrHostNotAllowed = errors.New("webapi: host not allowed")

// Config — настройки веб-сервера: host, port и журнал доступа.
type Config struct {
	Host   string
	Port   int
	Logger *journal.Logger
}

// withDefaults заполняет пустые host/port значениями по умолчанию (loopback).
func (c Config) withDefaults() Config {
	if c.Host == "" {
		c.Host = DefaultHost
	}
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	return c
}

// Listen создаёт слушающий сокет строго на loopback: host обязан быть в
// фиксированной таблице (127.0.0.1 или localhost), иначе — ошибка старта.
func Listen(host string, port int) (net.Listener, error) {
	if _, ok := allowedHosts[host]; !ok {
		return nil, fmt.Errorf("%w: %q (allowed: 127.0.0.1, localhost)", ErrHostNotAllowed, host)
	}
	return net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
}

// Server — веб-сервер на loopback: статика из embed и JSON/WS-эндпоинты.
type Server struct {
	cfg     Config
	hub     *Hub
	httpSrv *http.Server
	ln      net.Listener
}

// New собирает сервер: роутер и WS-хаб с журналом доступа.
func New(cfg Config) *Server {
	cfg = cfg.withDefaults()
	atlasLogger = cfg.Logger
	mcpLogger = cfg.Logger
	coeffLogger = cfg.Logger
	chatLogger = cfg.Logger
	hub := NewHub()
	router := newRouter(hub)
	bindMonitorJournal(cfg.Logger)
	return &Server{
		cfg:     cfg,
		hub:     hub,
		httpSrv: &http.Server{Handler: accessLog(router, cfg.Logger)},
	}
}

// NewHandler возвращает HTTP-обработчик без слушателя (для тестов).
func NewHandler(cfg Config) http.Handler {
	cfg = cfg.withDefaults()
	atlasLogger = cfg.Logger
	mcpLogger = cfg.Logger
	coeffLogger = cfg.Logger
	chatLogger = cfg.Logger
	hub := NewHub()
	router := newRouter(hub)
	bindMonitorJournal(cfg.Logger)
	return accessLog(router, cfg.Logger)
}

// Hub возвращает WS-хаб сервера: подписка на топики экранов и рассылка.
func (s *Server) Hub() *Hub { return s.hub }

// Listen поднимает слушающий сокет конфигурации.
func (s *Server) Listen() error {
	ln, err := Listen(s.cfg.Host, s.cfg.Port)
	if err != nil {
		return err
	}
	s.ln = ln
	return nil
}

// Serve обслуживает запросы до Close.
func (s *Server) Serve() error {
	if s.ln == nil {
		return errors.New("webapi: server is not listening")
	}
	return s.httpSrv.Serve(s.ln)
}

// Close закрывает слушатель, WS-хаб и http.Server.
func (s *Server) Close() error {
	s.hub.Close()
	if s.ln != nil {
		_ = s.ln.Close()
	}
	if s.httpSrv != nil {
		return s.httpSrv.Close()
	}
	return nil
}

// Addr возвращает фактический адрес слушателя (пусто, если не поднят).
func (s *Server) Addr() string {
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// accessLog — middleware: каждый HTTP-запрос пишет строку доступа в журнал
// станка (event=web.access, T-08-02, шаг 6).
func accessLog(next http.Handler, logger *journal.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		if logger != nil {
			_ = logger.Event("webapi", journal.EventWebAccess,
				journal.Field{Key: "method", Value: r.Method},
				journal.Field{Key: "path", Value: r.URL.Path},
				journal.Field{Key: "status", Value: strconv.Itoa(status)})
		}
	})
}

// statusWriter запоминает код ответа для строки доступа.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Hijack пробрасывает к исходному ResponseWriter — нужно для WebSocket.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("webapi: response writer does not support hijacking")
	}
	return hj.Hijack()
}

// LoadConfig читает секцию web из config.yaml; при отсутствии секции —
// дефолт 127.0.0.1:8720.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg, err := parseWeb(string(data))
	if err != nil {
		return Config{}, err
	}
	return cfg.withDefaults(), nil
}

// parseWeb разбирает топ-ключ `web:` с полями host и port.
func parseWeb(data string) (Config, error) {
	cfg := Config{}
	inWeb := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inWeb = line == "web:"
			continue
		}
		if !inWeb {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if i := strings.IndexByte(val, '#'); i >= 0 {
			val = strings.TrimSpace(val[:i])
		}
		switch key {
		case "host":
			cfg.Host = val
		case "port":
			n, err := strconv.Atoi(val)
			if err != nil {
				return Config{}, fmt.Errorf("webapi: bad web.port %q", val)
			}
			cfg.Port = n
		}
	}
	if err := sc.Err(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
