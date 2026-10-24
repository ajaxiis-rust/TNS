// Package portgate — порт-гейт при старте — слушающие сокеты ядра сверяются с белым списком портов (T-01-12).
package portgate

import (
	"bufio"
	"errors"
	"net/netip"
	"os"
	"strings"

	"tns65/internal/journal"
)

// ExitDeny — код возврата ядра при отказе старта из-за чужого слушающего сокета (T-01-12, шаг 2).
const ExitDeny = 3

// ErrDenied — слушающий сокет не на loopback или порт вне белого списка.
var ErrDenied = errors.New("portgate: denied listening socket")

// Whitelist — белый список портов ядра. Сокет допускается, если адрес loopback
// (127.0.0.0/8 или ::1) и порт есть в списке.
type Whitelist struct {
	ports map[uint16]struct{}
}

// ParseWhitelist собирает белый список из записей "host:port" секции
// ports.whitelist. Хост обязан быть loopback: не-loopback запись в список не
// попадает — чужой адрес никогда не разрешается.
func ParseWhitelist(entries []string) Whitelist {
	w := Whitelist{ports: make(map[uint16]struct{})}
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		ap, err := netip.ParseAddrPort(e)
		if err != nil || !ap.Addr().IsLoopback() {
			continue
		}
		w.ports[ap.Port()] = struct{}{}
	}
	return w
}

// Allows сообщает, допустим ли адрес: loopback и порт в белом списке.
func (w Whitelist) Allows(addr netip.AddrPort) bool {
	if !addr.Addr().IsLoopback() {
		return false
	}
	_, ok := w.ports[addr.Port()]
	return ok
}

// Socket — слушающий сокет процесса ядра или его ребёнка.
type Socket struct {
	Addr netip.AddrPort
}

// Result — решение гейта.
type Result struct {
	Code   int      // 0 — старт разрешён, ExitDeny — отказ.
	Denied []Socket // сокеты-нарушители.
}

// Evaluate сверяет сокеты с белым списком без доступа к /proc — чистая функция,
// на ней построены тесты на фикстурах.
func Evaluate(wl Whitelist, sockets []Socket) Result {
	res := Result{}
	for _, s := range sockets {
		if !wl.Allows(s.Addr) {
			res.Denied = append(res.Denied, s)
		}
	}
	if len(res.Denied) > 0 {
		res.Code = ExitDeny
	}
	return res
}

// platformCollect — сборщик слушающих сокетов текущей платформы. Подменяется
// в portgate_linux.go и portgate_windows.go; на иных платформах старт запрещён
// (закрыт по отказу).
var platformCollect = func() ([]Socket, error) {
	return nil, ErrDenied
}

// Run собирает слушающие сокеты платформы, сверяет с белым списком, пишет
// отказы в журнал и возвращает код выхода (0 — старт, ExitDeny — отказ).
func Run(wl Whitelist, logger *journal.Logger) int {
	return run(wl, logger, platformCollect)
}

// run — проверка с инжектируемым сборщиком сокетов (тесты подают фикстуры).
func run(wl Whitelist, logger *journal.Logger, collectFn func() ([]Socket, error)) int {
	sockets, err := collectFn()
	if err != nil {
		// Гейт не смог прочитать сокеты — старт запрещён (закрыт по отказу).
		return ExitDeny
	}
	res := Evaluate(wl, sockets)
	for _, s := range res.Denied {
		if logger != nil {
			_ = logger.Event("portgate", journal.EventPortgateDeny,
				journal.Field{Key: "addr", Value: s.Addr.String()})
		}
	}
	return res.Code
}

// LoadWhitelist читает секцию ports.whitelist из файла конфигурации.
func LoadWhitelist(path string) (Whitelist, error) {
	entries, err := readWhitelist(path)
	if err != nil {
		return Whitelist{}, err
	}
	return ParseWhitelist(entries), nil
}

// readWhitelist разбирает топ-ключ `ports:`, внутри — список `whitelist:`.
func readWhitelist(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var out []string
	inPorts := false
	inWhitelist := false

	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inPorts = line == "ports:"
			inWhitelist = false
			continue
		}
		if !inPorts {
			continue
		}
		if line == "whitelist:" && !inWhitelist {
			inWhitelist = true
			continue
		}
		if !inWhitelist {
			continue
		}
		if !strings.HasPrefix(line, "-") {
			inWhitelist = false
			continue
		}
		item := strings.TrimSpace(strings.TrimPrefix(line, "-"))
		if i := strings.IndexByte(item, '#'); i >= 0 {
			item = strings.TrimSpace(item[:i])
		}
		out = append(out, item)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
