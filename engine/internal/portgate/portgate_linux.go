//go:build linux

package portgate

import (
	"bufio"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func init() {
	platformCollect = collectLinux
}

// collectLinux собирает слушающие сокеты процессов ядра и его детей: читает
// /proc/net/tcp и /proc/net/tcp6, оставляет записи со state LISTEN (0A), чей
// inode принадлежит ядру или его детям (привязка через /proc/<pid>/fd).
func collectLinux() ([]Socket, error) {
	ours := ourInodes()

	tcp, err := os.ReadFile("/proc/net/tcp")
	if err != nil {
		return nil, err
	}
	tcp6, err := os.ReadFile("/proc/net/tcp6")
	if err != nil {
		return nil, err
	}

	return matchSockets(string(tcp), string(tcp6), ours), nil
}

// matchSockets объединяет записи /proc/net/tcp и /proc/net/tcp6, оставляя
// слушающие сокеты, чей inode в ours.
func matchSockets(tcp, tcp6 string, ours map[uint64]struct{}) []Socket {
	var out []Socket
	for _, addr := range parseProcNet(tcp, ours) {
		out = append(out, Socket{Addr: addr})
	}
	for _, addr := range parseProcNet(tcp6, ours) {
		out = append(out, Socket{Addr: addr})
	}
	return out
}

// parseProcNet разбирает содержимое /proc/net/tcp или /proc/net/tcp6: для строк
// со state LISTEN (0A), чей inode есть в ours, возвращает локальный адрес.
func parseProcNet(data string, ours map[uint64]struct{}) []netip.AddrPort {
	var out []netip.AddrPort
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "sl") {
			continue
		}
		f := strings.Fields(line)
		// f[0]=слот, f[1]=local_address, f[2]=rem_address, f[3]=state, f[9]=inode.
		if len(f) < 10 || f[3] != "0A" {
			continue
		}
		inode, err := strconv.ParseUint(f[9], 10, 64)
		if err != nil {
			continue
		}
		if _, ok := ours[inode]; !ok {
			continue
		}
		addr, err := parseLocalAddr(f[1])
		if err != nil {
			continue
		}
		out = append(out, addr)
	}
	return out
}

// parseLocalAddr разбирает "local_address" из /proc/net/tcp: HEXIP:HEXPORT.
func parseLocalAddr(field string) (netip.AddrPort, error) {
	i := strings.LastIndexByte(field, ':')
	if i < 0 {
		return netip.AddrPort{}, errors.New("portgate: bad local_address")
	}
	port, err := strconv.ParseUint(field[i+1:], 16, 16)
	if err != nil {
		return netip.AddrPort{}, err
	}
	raw, err := decodeHexIP(field[:i])
	if err != nil {
		return netip.AddrPort{}, err
	}

	var addr netip.Addr
	switch len(raw) {
	case 4:
		var b4 [4]byte
		copy(b4[:], raw)
		addr = netip.AddrFrom4(b4)
	case 16:
		var b16 [16]byte
		copy(b16[:], raw)
		addr = netip.AddrFrom16(b16)
	default:
		return netip.AddrPort{}, errors.New("portgate: bad ip length")
	}
	return netip.AddrPortFrom(addr, uint16(port)), nil
}

// decodeHexIP переводит hex-строку IP из /proc/net/tcp в байты в сетевом порядке.
// IPv4: 8 hex-цифр, байты в обратном порядке (хост little-endian). IPv6: 32
// hex-цифры, четыре 32-битных слова, каждое в обратном порядке байтов.
func decodeHexIP(s string) ([]byte, error) {
	n := len(s)
	if n != 8 && n != 32 {
		return nil, errors.New("portgate: bad ip hex length")
	}
	b := make([]byte, n/2)
	for j := 0; j < n; j += 2 {
		v, err := strconv.ParseUint(s[j:j+2], 16, 8)
		if err != nil {
			return nil, err
		}
		b[j/2] = byte(v)
	}
	if n == 8 {
		for i, j := 0, 3; i < j; i, j = i+1, j-1 {
			b[i], b[j] = b[j], b[i]
		}
		return b, nil
	}
	for j := 0; j < 16; j += 4 {
		b[j], b[j+3] = b[j+3], b[j]
		b[j+1], b[j+2] = b[j+2], b[j+1]
	}
	return b, nil
}

// ourInodes — множество inode сокетов, принадлежащих ядру и его детям.
func ourInodes() map[uint64]struct{} {
	pid := os.Getpid()
	ours := map[uint64]struct{}{}
	pids := map[int]struct{}{pid: {}}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return ours
	}
	for _, de := range entries {
		if !de.IsDir() {
			continue
		}
		cpid, err := strconv.Atoi(de.Name())
		if err != nil || cpid == pid {
			continue
		}
		if descendantOf(cpid, pid) {
			pids[cpid] = struct{}{}
		}
	}

	for p := range pids {
		collectFDInodes(p, ours)
	}
	return ours
}

// descendantOf сообщает, является ли pid потомком root (по цепочке PPID).
func descendantOf(pid, root int) bool {
	for i := 0; i < 64; i++ {
		if pid == root {
			return true
		}
		ppid := parentOf(pid)
		if ppid <= 1 {
			return false
		}
		pid = ppid
	}
	return false
}

// parentOf читает PPID из /proc/<pid>/stat (поле после закрывающей скобки).
func parentOf(pid int) int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	s := string(data)
	i := strings.LastIndexByte(s, ')')
	if i < 0 || i+2 >= len(s) {
		return 0
	}
	rest := strings.Fields(s[i+2:])
	if len(rest) < 2 {
		return 0
	}
	ppid, err := strconv.Atoi(rest[1])
	if err != nil {
		return 0
	}
	return ppid
}

// collectFDInodes читает симлинки /proc/<pid>/fd/* → socket:[inode].
func collectFDInodes(pid int, ours map[uint64]struct{}) {
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return
	}
	for _, de := range entries {
		link, err := os.Readlink(filepath.Join(fdDir, de.Name()))
		if err != nil || !strings.HasPrefix(link, "socket:[") {
			continue
		}
		s := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
		inode, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			continue
		}
		ours[inode] = struct{}{}
	}
}
