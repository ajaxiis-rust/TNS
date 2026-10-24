package supervisor

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrAlreadyRunning — ядро уже запущено: замок занят другим процессом. main.go
// отображает его в код возврата 4 (T-01-14, шаг 1).
var ErrAlreadyRunning = errors.New("supervisor: core already running")

// Lock — замок одного экземпляра ядра на файле state/tns65.lock. Блокировка
// держится на открытом файловом описании: при аварийном выходе ОС снимает её
// сама, поэтому остаток файла новому старту не мешает (шаг 2).
type Lock struct {
	file *os.File
}

// Acquire создаёт каталог пути и берёт эксклюзивный неблокирующий замок.
// Второй вызов по тому же пути возвращает ErrAlreadyRunning.
func Acquire(path string) (*Lock, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		return nil, ErrAlreadyRunning
	}
	return &Lock{file: f}, nil
}

// Release снимает замок и закрывает файл. Идемпотентен; nil-приёмник — бездействие.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := unlockFile(l.file)
	_ = l.file.Close()
	l.file = nil
	return err
}
