package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LevelInfo — единственный уровень событий этой задачи.
const LevelInfo = "info"

// Field — пара ключ-значение в массиве fields JSON-события.
type Field struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Logger — журнал станка: один писатель (мьютекс), O_APPEND, ротация по размеру.
type Logger struct {
	mu       sync.Mutex
	f        *os.File
	path     string
	maxBytes int64
	maxFiles int
}

// New создаёт каталоги пути (os.MkdirAll) и открывает файл на добавление.
func New(path string) (*Logger, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Logger{
		f:        f,
		path:     path,
		maxBytes: 50 << 20,
		maxFiles: 5,
	}, nil
}

// eventRecord — структура одной JSON-строки события.
type eventRecord struct {
	TS     string  `json:"ts"`
	Level  string  `json:"level"`
	Source string  `json:"source"`
	Event  string  `json:"event"`
	Fields []Field `json:"fields"`
}

// Event пишет одно событие одной JSON-строкой, завершённой '\n'.
func (l *Logger) Event(source, event string, fields ...Field) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	rec := eventRecord{
		TS:     time.Now().Format(time.RFC3339),
		Level:  LevelInfo,
		Source: source,
		Event:  event,
		Fields: fields,
	}
	if rec.Fields == nil {
		rec.Fields = []Field{}
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	line := append(b, '\n')

	if l.f != nil {
		if info, err := l.f.Stat(); err == nil && info.Size()+int64(len(line)) > l.maxBytes {
			if err := l.rotateLocked(); err != nil {
				return err
			}
		}
	}

	_, err = l.f.Write(line)
	return err
}

// rotateLocked закрывает текущий файл, сдвигает архивы и открывает новый.
// Вызывается под мьютексом.
func (l *Logger) rotateLocked() error {
	if l.f != nil {
		if err := l.f.Close(); err != nil {
			return err
		}
		l.f = nil
	}

	// Старейший архив удаляется.
	_ = os.Remove(fmt.Sprintf("%s.%d", l.path, l.maxFiles))

	// Сдвиг: <path>.4→.5, … , <path>.1→.2 (отсутствующие игнорируются).
	for i := l.maxFiles - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", l.path, i), fmt.Sprintf("%s.%d", l.path, i+1))
	}

	// Текущий становится архивом <path>.1.
	if err := os.Rename(l.path, fmt.Sprintf("%s.1", l.path)); err != nil {
		return err
	}

	// Новый текущий файл.
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	l.f = f
	return nil
}
