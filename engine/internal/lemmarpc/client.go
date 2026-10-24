package lemmarpc

import (
	"bufio"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"

	"tns65/internal/journal"
)

// ErrTimeout — ответ лемматизатора не пришёл за тайм-аут (T-01-09, шаг 2).
var ErrTimeout = errors.New("lemmarpc: timeout waiting for response")

// responseTimeout — тайм-аут ответа протокола (шаг 2).
const responseTimeout = 200 * time.Millisecond

// maxConsecutiveTimeouts — после стольких подряд тайм-аутов клиент перезапускает
// процесс лемматизатора (шаг 2).
const maxConsecutiveTimeouts = 3

// Client — клиент лемматизатора по протоколу stdio/JSON. Запускает резидент по
// пути из манифеста пакета и не зависит от реализации резидента (шаги 2–3).
type Client struct {
	binary string
	logger *journal.Logger

	mu       sync.Mutex
	cur      *session
	timeouts int
	nextID   int
}

// session — один запущенный процесс лемматизатора и его каналы.
type session struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	reader  *bufio.Reader
	pending map[int]chan responseResult
}

type responseResult struct {
	resp *Response
	err  error
}

// NewClient создаёт клиент для бинарника binary. logger может быть nil.
func NewClient(binary string, logger *journal.Logger) *Client {
	return &Client{binary: binary, logger: logger}
}

// Lemmatize разбирает text и возвращает токены (лемма, часть речи, метки).
// Тайм-аут ответа — 200 мс; после трёх подряд тайм-аутов процесс перезапускается;
// ошибки пишутся в журнал событием lemma.fail (шаг 2).
func (c *Client) Lemmatize(text string) ([]Token, error) {
	c.mu.Lock()
	if c.cur == nil {
		if err := c.startLocked(); err != nil {
			c.failLocked("start", err)
			c.mu.Unlock()
			return nil, err
		}
	}

	s := c.cur
	id := c.nextID
	c.nextID++
	ch := make(chan responseResult, 1)
	s.pending[id] = ch
	if err := WriteRequest(s.stdin, id, text); err != nil {
		delete(s.pending, id)
		c.failLocked("write", err)
		c.stopLocked()
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()

	select {
	case r := <-ch:
		c.mu.Lock()
		c.timeouts = 0
		c.mu.Unlock()
		if r.err != nil {
			c.handleReadError(r.err)
			return nil, r.err
		}
		return r.resp.Tokens, nil
	case <-time.After(responseTimeout):
		c.mu.Lock()
		delete(s.pending, id)
		c.timeouts++
		restart := c.timeouts >= maxConsecutiveTimeouts
		c.failLocked("timeout", ErrTimeout)
		if restart {
			c.timeouts = 0
			c.stopLocked()
		}
		c.mu.Unlock()
		return nil, ErrTimeout
	}
}

// Close останавливает процесс лемматизатора.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
	return nil
}

// startLocked запускает процесс и читатель ответов. Вызывается под c.mu.
func (c *Client) startLocked() error {
	cmd := exec.Command(c.binary)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	s := &session{
		cmd:     cmd,
		stdin:   stdin,
		reader:  bufio.NewReader(stdout),
		pending: map[int]chan responseResult{},
	}
	c.cur = s
	go c.pump(s)
	return nil
}

// stopLocked останавливает текущий процесс и забывает сессию. Вызывается под c.mu.
func (c *Client) stopLocked() {
	if c.cur == nil {
		return
	}
	if c.cur.cmd.Process != nil {
		_ = c.cur.cmd.Process.Kill()
		_ = c.cur.cmd.Wait()
	}
	c.cur = nil
}

// handleReadError пишет ошибку чтения в журнал и останавливает процесс.
func (c *Client) handleReadError(err error) {
	c.mu.Lock()
	c.failLocked("read", err)
	c.stopLocked()
	c.mu.Unlock()
}

// failLocked пишет событие lemma.fail. Вызывается под c.mu.
func (c *Client) failLocked(reason string, err error) {
	if c.logger == nil {
		return
	}
	_ = c.logger.Event("lemma", journal.EventLemmaFail,
		journal.Field{Key: "reason", Value: reason},
		journal.Field{Key: "error", Value: err.Error()})
}

// pump — единственный читатель stdout сессии: читает ответы в порядке строк и
// раздаёт их ожидающим запросам по id, чтобы тайм-ауты не рассинхронизировали
// поток. Ответ без ожидающего запроса отбрасывается.
func (c *Client) pump(s *session) {
	for {
		resp, err := ReadResponse(s.reader)
		if err != nil {
			c.mu.Lock()
			for id, ch := range s.pending {
				delete(s.pending, id)
				ch <- responseResult{err: err}
			}
			c.mu.Unlock()
			return
		}

		c.mu.Lock()
		ch, ok := s.pending[resp.ID]
		if ok {
			delete(s.pending, resp.ID)
		}
		c.mu.Unlock()
		if ok {
			ch <- responseResult{resp: resp}
		}
	}
}
