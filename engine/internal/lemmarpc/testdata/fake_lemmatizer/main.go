// Command fake_lemmatizer — тестовый лемматизатор по протоколу lemmarpc
// (T-01-09, шаг 3): отвечает на stdio/JSON, умеет имитировать задержку.
//
// Переменные окружения:
//
//	LEMMARPC_FAKE_DELAY    — задержка каждого ответа (Go duration, например "300ms").
//	LEMMARPC_FAKE_PID_LOG  — файл, в который при старте дописывается PID процесса.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"tns65/internal/lemmarpc"
)

func main() {
	logPID()

	delay := time.Duration(0)
	if s := os.Getenv("LEMMARPC_FAKE_DELAY"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			delay = d
		}
	}

	in := bufio.NewReader(os.Stdin)
	for {
		req, err := lemmarpc.ReadRequest(in)
		if err != nil {
			return
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		if err := lemmarpc.WriteResponse(os.Stdout, req.ID, lemmatize(req.Text)); err != nil {
			return
		}
	}
}

// lemmatize — упрощённый разбор: «не» отдельной леммой «не» (D-13.3),
// «хочу» → «хотеть»; прочие слова — леммой-формой.
func lemmatize(text string) []lemmarpc.Token {
	words := strings.Fields(text)
	tokens := make([]lemmarpc.Token, 0, len(words))
	for _, w := range words {
		t := lemmarpc.Token{Form: w, Lemma: w, Pos: "NOUN", Tags: []string{}}
		switch w {
		case "не":
			t.Lemma = "не"
			t.Pos = "PART"
		case "хочу", "хотеть":
			t.Lemma = "хотеть"
			t.Pos = "VERB"
		}
		tokens = append(tokens, t)
	}
	return tokens
}

// logPID дописывает PID процесса в файл, если путь задан.
func logPID() {
	path := os.Getenv("LEMMARPC_FAKE_PID_LOG")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	_ = f.Close()
}
