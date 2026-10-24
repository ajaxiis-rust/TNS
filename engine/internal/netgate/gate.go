package netgate

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"tns65/internal/journal"
)

// ErrDeniedHost — целевой хост вне белого списка; соединение не устанавливается.
var ErrDeniedHost = errors.New("netgate: denied host")

// dialTimeout — время установки исходящего соединения. Отказы по белому списку
// происходят до соединения и этим тайм-аутом не ограничены.
const dialTimeout = 30 * time.Second

// hostSet — белый список: точные имена хостов и диапазоны host/cidr.
type hostSet struct {
	names map[string]struct{}
	cidrs []netip.Prefix
}

func newHostSet() hostSet {
	return hostSet{names: map[string]struct{}{}}
}

// add вносит одну запись конфигурации: `host` — точное имя (без регистра),
// `host/cidr` — диапазон адресов.
func (s *hostSet) add(entry string) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return
	}
	if strings.Contains(entry, "/") {
		if p, err := netip.ParsePrefix(entry); err == nil {
			s.cidrs = append(s.cidrs, p)
		}
		return
	}
	s.names[strings.ToLower(strings.TrimSuffix(entry, "."))] = struct{}{}
}

// allows проверяет hostname по белому списку. Проверка — по hostname, DNS-ответ
// не сверяется; IP сверяется только с диапазонами host/cidr.
func (s hostSet) allows(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if _, ok := s.names[host]; ok {
		return true
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		for _, p := range s.cidrs {
			if p.Contains(addr) {
				return true
			}
		}
	}
	return false
}

// gate — состояние гейта: белый список хостов и журнал отказов.
type gate struct {
	mu      sync.RWMutex
	allowed hostSet
	logger  *journal.Logger
}

var defaultGate = &gate{allowed: newHostSet()}

// Load читает белый список из файла конфигурации (секция netgate.allowed_hosts)
// и заполняет гейт по умолчанию.
func Load(path string) error {
	hs, err := readAllowedHosts(path)
	if err != nil {
		return err
	}
	defaultGate.mu.Lock()
	defaultGate.allowed = hs
	defaultGate.mu.Unlock()
	return nil
}

// SetLogger задаёт журнал станка для записей netgate.deny.
func SetLogger(l *journal.Logger) {
	defaultGate.mu.Lock()
	defaultGate.logger = l
	defaultGate.mu.Unlock()
}

// Client — HTTP-клиент наружу. Единственный путь наружного HTTP: DialContext
// сверяет целевой хост с белым списком до установки соединения, редирект за
// пределы списка не выполняется. host — целевой хост (hostname или host:port).
func Client(host string) *http.Client {
	return defaultGate.client(host, callerName(2))
}

// Local — HTTP-клиент loopback (:5001, :5002, httptest в тестах): белым списком
// не проверяется — локальные адреса списком не занимаются.
func Local(addr string) *http.Client {
	return defaultGate.local(addr)
}

func (g *gate) client(host string, caller string) *http.Client {
	g.mu.RLock()
	allowed := g.allowed
	logger := g.logger
	g.mu.RUnlock()

	bound := hostnameOf(host)

	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		h := hostnameOf(addr)
		if bound != "" && h != bound {
			denyHost(logger, h, caller)
			return nil, ErrDeniedHost
		}
		if !allowed.allows(h) {
			denyHost(logger, h, caller)
			return nil, ErrDeniedHost
		}
		d := &net.Dialer{Timeout: dialTimeout}
		return d.DialContext(ctx, network, addr)
	}

	redirect := func(req *http.Request, _ []*http.Request) error {
		h := req.URL.Hostname()
		if !allowed.allows(h) {
			denyHost(logger, h, caller)
			return ErrDeniedHost
		}
		return nil
	}

	return &http.Client{
		Transport:     &http.Transport{DialContext: dial},
		CheckRedirect: redirect,
	}
}

func (g *gate) local(addr string) *http.Client {
	dial := func(ctx context.Context, network, _ string) (net.Conn, error) {
		d := &net.Dialer{Timeout: dialTimeout}
		return d.DialContext(ctx, network, addr)
	}
	return &http.Client{
		Transport: &http.Transport{DialContext: dial},
	}
}

// denyHost пишет отказ в журнал: event=netgate.deny с хостом и вызывающим.
func denyHost(logger *journal.Logger, host, caller string) {
	if logger == nil {
		return
	}
	_ = logger.Event("netgate", journal.EventNetgateDeny,
		journal.Field{Key: "host", Value: host},
		journal.Field{Key: "caller", Value: caller})
}

// hostnameOf выделяет имя хоста из адреса "host:port" (IPv6 без скобок).
func hostnameOf(addr string) string {
	h, _, err := net.SplitHostPort(addr)
	if err != nil {
		h = addr
	}
	return strings.ToLower(strings.TrimSuffix(h, "."))
}

// callerName возвращает имя функции, вызвавшей Client (через skip кадров).
func callerName(skip int) string {
	pc, _, _, ok := runtime.Caller(skip)
	if !ok {
		return "unknown"
	}
	if fn := runtime.FuncForPC(pc); fn != nil {
		return fn.Name()
	}
	return "unknown"
}

// readAllowedHosts разбирает секцию netgate.allowed_hosts из YAML-файла
// конфигурации: топ-ключ `netgate:`, внутри — список `allowed_hosts:`.
func readAllowedHosts(path string) (hostSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return hostSet{}, err
	}

	hs := newHostSet()
	inNetgate := false
	inAllowedHosts := false

	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inNetgate = line == "netgate:"
			inAllowedHosts = false
			continue
		}
		if !inNetgate {
			continue
		}
		if line == "allowed_hosts:" && !inAllowedHosts {
			inAllowedHosts = true
			continue
		}
		if !inAllowedHosts {
			continue
		}
		if !strings.HasPrefix(line, "-") {
			inAllowedHosts = false
			continue
		}
		item := strings.TrimSpace(strings.TrimPrefix(line, "-"))
		if i := strings.IndexByte(item, '#'); i >= 0 {
			item = strings.TrimSpace(item[:i])
		}
		hs.add(item)
	}
	if err := sc.Err(); err != nil {
		return hostSet{}, err
	}
	return hs, nil
}
