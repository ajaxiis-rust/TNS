//go:build !windows

package supervisor

import (
	"os"
	"syscall"
)

// lockFile — flock(2): эксклюзивный неблокирующий замок (T-01-14, шаг 1).
func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// unlockFile — flock(2): снятие замка при штатной остановке.
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
