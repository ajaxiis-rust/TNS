package portgate

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/journal"
)

const procNetHeader = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode"

// defaultWhitelist — белый список из задачи: 127.0.0.1:8720, :5001, :5002.
func defaultWhitelist() Whitelist {
	return ParseWhitelist([]string{"127.0.0.1:8720", "127.0.0.1:5001", "127.0.0.1:5002"})
}

// localAddr кодирует адрес в формат /proc/net/tcp(6): HEXIP:HEXPORT.
func localAddr(ip string, port uint16) string {
	addr := netip.MustParseAddr(ip)
	if addr.Is4() {
		b := addr.As4()
		return fmt.Sprintf("%02X%02X%02X%02X:%04X", b[3], b[2], b[1], b[0], port)
	}
	b := addr.As16()
	var sb strings.Builder
	for i := 0; i < 16; i += 4 {
		fmt.Fprintf(&sb, "%02X%02X%02X%02X", b[i+3], b[i+2], b[i+1], b[i])
	}
	return fmt.Sprintf("%s:%04X", sb.String(), port)
}

// procNetLine собирает одну строку /proc/net/tcp(6) с нужным локальным адресом.
func procNetLine(local, state, inode string) string {
	return fmt.Sprintf("   0: %s 00000000:0000 %s 00000000:00000000 00:00000000 00000000     0        0 %s 1 0 0 0 0", local, state, inode)
}

func TestFixtureExternal(t *testing.T) {
	wl := defaultWhitelist()

	tcp := procNetHeader + "\n" +
		procNetLine(localAddr("127.0.0.1", 8720), "0A", "1001") + "\n" + // loopback, в списке
		procNetLine(localAddr("192.0.2.10", 80), "0A", "1002") + "\n" + // внешний адрес
		procNetLine(localAddr("127.0.0.1", 9999), "0A", "1003") + "\n" // loopback, порт вне списка
	tcp6 := procNetHeader + "\n"

	ours := map[uint64]struct{}{1001: {}, 1002: {}, 1003: {}}
	sockets := matchSockets(tcp, tcp6, ours)
	if len(sockets) != 3 {
		t.Fatalf("sockets = %d, want 3 (%v)", len(sockets), sockets)
	}

	res := Evaluate(wl, sockets)
	if res.Code != ExitDeny {
		t.Fatalf("code = %d, want ExitDeny (%d)", res.Code, ExitDeny)
	}
	if len(res.Denied) != 2 {
		t.Fatalf("denied = %d, want 2 (%v)", len(res.Denied), res.Denied)
	}
	wantDenied := map[string]bool{
		netip.MustParseAddrPort("192.0.2.10:80").String():  false,
		netip.MustParseAddrPort("127.0.0.1:9999").String(): false,
	}
	for _, s := range res.Denied {
		if _, ok := wantDenied[s.Addr.String()]; !ok {
			t.Fatalf("неожиданный отказ: %s", s.Addr)
		}
		wantDenied[s.Addr.String()] = true
	}
	for addr, found := range wantDenied {
		if !found {
			t.Fatalf("не отказан ожидаемый сокет: %s", addr)
		}
	}

	// Дефект пишется в журнал (event=portgate.deny) и в код возврата.
	dir := t.TempDir()
	logger, err := journal.New(filepath.Join(dir, "engine.log"))
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	code := run(wl, logger, func() ([]Socket, error) { return sockets, nil })
	if code != ExitDeny {
		t.Fatalf("run code = %d, want ExitDeny (%d)", code, ExitDeny)
	}
	data, err := os.ReadFile(filepath.Join(dir, "engine.log"))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if !strings.Contains(string(data), "portgate.deny") {
		t.Fatalf("в журнале нет строки portgate.deny:\n%s", data)
	}
}

func TestFixtureLoopback(t *testing.T) {
	wl := defaultWhitelist()

	tcp := procNetHeader + "\n" +
		procNetLine(localAddr("127.0.0.1", 8720), "0A", "1001") + "\n" +
		procNetLine(localAddr("127.0.0.1", 5001), "0A", "1002") + "\n" +
		procNetLine(localAddr("127.0.0.1", 5002), "0A", "1003") + "\n"
	tcp6 := procNetHeader + "\n"

	ours := map[uint64]struct{}{1001: {}, 1002: {}, 1003: {}}
	sockets := matchSockets(tcp, tcp6, ours)
	if len(sockets) != 3 {
		t.Fatalf("sockets = %d, want 3", len(sockets))
	}

	res := Evaluate(wl, sockets)
	if res.Code != 0 {
		t.Fatalf("code = %d, want 0 (старт разрешён); denied=%v", res.Code, res.Denied)
	}
	if len(res.Denied) != 0 {
		t.Fatalf("denied = %d, want 0", len(res.Denied))
	}
}

func TestV6Loopback(t *testing.T) {
	wl := defaultWhitelist()

	tcp := procNetHeader + "\n"
	tcp6 := procNetHeader + "\n" +
		procNetLine(localAddr("::1", 8720), "0A", "1001") + "\n" +
		procNetLine(localAddr("::1", 5002), "0A", "1002") + "\n"

	ours := map[uint64]struct{}{1001: {}, 1002: {}}
	sockets := matchSockets(tcp, tcp6, ours)
	if len(sockets) != 2 {
		t.Fatalf("sockets = %d, want 2 (%v)", len(sockets), sockets)
	}

	res := Evaluate(wl, sockets)
	if res.Code != 0 {
		t.Fatalf("code = %d, want 0 (::1 — loopback); denied=%v", res.Code, res.Denied)
	}
	for _, s := range sockets {
		if !s.Addr.Addr().IsLoopback() {
			t.Fatalf("%s не loopback", s.Addr)
		}
	}
}
