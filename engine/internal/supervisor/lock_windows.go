//go:build windows

package supervisor

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFile — LockFileEx: эксклюзивный неблокирующий замок на весь файл
// (T-01-14, шаг 1). Отказ сразу — без ожидания другого процесса.
func lockFile(f *os.File) error {
	var ol windows.Overlapped
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &ol)
}

// unlockFile — UnlockFileEx: снятие замка при штатной остановке.
func unlockFile(f *os.File) error {
	var ol windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ol)
}
