package journal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrNoWriter — Pipe получил nil-писатель.
var ErrNoWriter = errors.New("journal: no writer")

// Pipe возвращает писатель для stdout/stderr ребёнка. Он буферизует ввод,
// режет по строкам ('\n'; завершающий '\r' отбрасывается) и каждую строку
// сериализует как событие журнала той же схемы, что Event: source = "child:"+name,
// event = EventChildOutput, fields = [{key:"text", value:<строка>}].
// Невалидные UTF-8 байты кодируются литералами \xNN и не теряются.
func Pipe(name string, w io.Writer) (io.Writer, error) {
	if w == nil {
		return nil, ErrNoWriter
	}
	return &pipeWriter{name: name, w: w}, nil
}

type pipeWriter struct {
	name string
	w    io.Writer
	buf  []byte
}

func (p *pipeWriter) Write(data []byte) (int, error) {
	p.buf = append(p.buf, data...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			break
		}
		line := p.buf[:i]
		p.buf = p.buf[i+1:]
		if err := p.emit(line); err != nil {
			return len(data), err
		}
	}
	return len(data), nil
}

// Close сбрасывает неполную последнюю строку (без завершающего '\n') в w.
func (p *pipeWriter) Close() error {
	if len(p.buf) == 0 {
		return nil
	}
	return p.emit(p.buf)
}

func (p *pipeWriter) emit(line []byte) error {
	if n := len(line); n > 0 && line[n-1] == '\r' {
		line = line[:n-1]
	}
	rec := eventRecord{
		TS:     time.Now().Format(time.RFC3339),
		Level:  LevelInfo,
		Source: "child:" + p.name,
		Event:  EventChildOutput,
		Fields: []Field{{Key: "text", Value: encodeBytes(line)}},
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = p.w.Write(b)
	return err
}

// encodeBytes переводит невалидные UTF-8 байты в литералы \xNN,
// сохраняя валидные последовательности как есть (байты восстановимы из записи).
func encodeBytes(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&sb, "\\x%02x", b[0])
			b = b[1:]
			continue
		}
		sb.Write(b[:size])
		b = b[size:]
	}
	return sb.String()
}

// Write реализует io.Writer для готовых строк журнала (один вызов — одна строка
// с завершающим '\n'): добавляет байты в файл под тем же мьютексом и с той же
// ротацией, что Event. Это единственная композиция Pipe → журнал без второго
// писателя в обход мьютекса (в production: cmd.Stdout = Pipe(name, logger)).
func (l *Logger) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.f != nil {
		if info, err := l.f.Stat(); err == nil && info.Size()+int64(len(p)) > l.maxBytes {
			if err := l.rotateLocked(); err != nil {
				return 0, err
			}
		}
	}

	if _, err := l.f.Write(p); err != nil {
		return 0, err
	}
	return len(p), nil
}
