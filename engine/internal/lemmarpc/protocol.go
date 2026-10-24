package lemmarpc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Token — один токен лемматизатора: форма, лемма, часть речи и метки (D-13.2).
type Token struct {
	Form  string   `json:"form"`
	Lemma string   `json:"lemma"`
	Pos   string   `json:"pos"`
	Tags  []string `json:"tags"`
}

// Request — запрос протокола stdio/JSON (T-01-09, шаг 1).
type Request struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

// Response — ответ протокола stdio/JSON.
type Response struct {
	ID     int     `json:"id"`
	Tokens []Token `json:"tokens"`
}

// WriteRequest сериализует запрос одной JSON-строкой с завершающим '\n'.
func WriteRequest(w io.Writer, id int, text string) error {
	return writeLine(w, Request{ID: id, Text: text})
}

// ReadRequest читает один запрос построчно из r.
func ReadRequest(r *bufio.Reader) (*Request, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return nil, fmt.Errorf("lemmarpc: bad request: %w", err)
	}
	return &req, nil
}

// WriteResponse сериализует ответ одной JSON-строкой с завершающим '\n'.
func WriteResponse(w io.Writer, id int, tokens []Token) error {
	return writeLine(w, Response{ID: id, Tokens: tokens})
}

// ReadResponse читает один ответ построчно из r.
func ReadResponse(r *bufio.Reader) (*Response, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("lemmarpc: bad response: %w", err)
	}
	return &resp, nil
}

// writeLine сериализует v одной JSON-строкой с завершающим '\n'.
func writeLine(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// readLine читает одну строку, отбрасывая завершающие '\n' и '\r'.
func readLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	line = line[:len(line)-1]
	if n := len(line); n > 0 && line[n-1] == '\r' {
		line = line[:n-1]
	}
	return line, nil
}
