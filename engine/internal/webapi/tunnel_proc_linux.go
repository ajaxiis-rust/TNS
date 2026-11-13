//go:build linux

package webapi

import (
	"os"
	"strconv"
	"strings"
)

// tunnelProcessRunning ищет живой cloudflared в таблице процессов. Туннель
// запускается отдельной командой (scripts/tunnel.sh), ядро его не порождает,
// поэтому признак запуска — запись /proc/<pid>/comm.
func tunnelProcessRunning() bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile("/proc/" + e.Name() + "/comm")
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(comm)) == "cloudflared" {
			return true
		}
	}
	return false
}
