//go:build windows

package portgate

import (
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func init() {
	platformCollect = collectWindows
}

// collectWindows собирает слушающие сокеты процессов ядра и его детей через `netstat -ano`.
func collectWindows() ([]Socket, error) {
	ours := ourPIDs()

	out, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
	if err != nil {
		return nil, err
	}

	var sockets []Socket
	for _, line := range strings.Split(string(out), "\r\n") {
		f := strings.Fields(line)
		// TCP <local> <remote> <state> <pid>.
		if len(f) < 5 || f[0] != "TCP" || f[3] != "LISTENING" {
			continue
		}
		pid, err := strconv.ParseUint(f[4], 10, 32)
		if err != nil {
			continue
		}
		if _, ok := ours[uint32(pid)]; !ok {
			continue
		}
		ap, err := netip.ParseAddrPort(f[1])
		if err != nil {
			continue
		}
		sockets = append(sockets, Socket{Addr: ap})
	}
	return sockets, nil
}

// ourPIDs — множество PID ядра и его детей (по дереву процессов Toolhelp).
func ourPIDs() map[uint32]struct{} {
	root := uint32(os.Getpid())
	ours := map[uint32]struct{}{root: {}}

	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ours
	}
	defer windows.CloseHandle(snapshot)

	parents := map[uint32]uint32{}
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err = windows.Process32First(snapshot, &pe); err != nil {
		return ours
	}
	for {
		parents[pe.ProcessID] = pe.ParentProcessID
		if err = windows.Process32Next(snapshot, &pe); err != nil {
			break
		}
	}

	for pid := range parents {
		if descendantOfWin(pid, root, parents) {
			ours[pid] = struct{}{}
		}
	}
	return ours
}

// descendantOfWin сообщает, является ли pid потомком root (по цепочке PPID).
func descendantOfWin(pid, root uint32, parents map[uint32]uint32) bool {
	for i := 0; i < 64; i++ {
		if pid == root {
			return true
		}
		ppid, ok := parents[pid]
		if !ok || ppid == 0 || ppid == pid {
			return false
		}
		pid = ppid
	}
	return false
}
