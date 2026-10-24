// Команда tns65-lemmaru — резидент-лемматизатор русского пакета (T-01-10).
// Читает словарь OpenCorpora при старте и обслуживает протокол stdio/JSON
// из internal/lemmarpc (T-01-09): запрос построчно, ответ одной JSON-строкой.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlexMaxy/gomorphy"

	"tns65/internal/lemmarpc"
)

func main() {
	dictFlag := flag.String("dict", "", "каталог словаря OpenCorpora (по умолчанию packs/ru/lemmatizer)")
	flag.Parse()

	dictPath := resolveDict(*dictFlag)
	if dictPath == "" {
		fmt.Fprintln(os.Stderr, "tns65-lemmaru: словарь не найден (задайте -dict или TNS65_LEMMARU_DICT)")
		os.Exit(1)
	}

	morph, err := gomorphy.GetMorphInstance(dictPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tns65-lemmaru: загрузка словаря %s: %v\n", dictPath, err)
		os.Exit(1)
	}

	if err := serve(os.Stdin, os.Stdout, morph); err != nil {
		fmt.Fprintf(os.Stderr, "tns65-lemmaru: %v\n", err)
		os.Exit(1)
	}
}

// serve читает запросы построчно из in и пишет ответы построчно в out.
func serve(in io.Reader, out io.Writer, morph *gomorphy.MorphAnalyzer) error {
	r := bufio.NewReader(in)
	w := bufio.NewWriter(out)
	defer w.Flush()

	for {
		req, err := lemmarpc.ReadRequest(r)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := lemmarpc.WriteResponse(w, req.ID, lemmatize(morph, req.Text)); err != nil {
			return err
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
}

// lemmatize разбирает text на слова и возвращает по одному токену на слово —
// лучший (первый) разбор gomorphy. «не» приходит отдельным словом и леммой (D-13.3).
func lemmatize(morph *gomorphy.MorphAnalyzer, text string) []lemmarpc.Token {
	var tokens []lemmarpc.Token
	for _, word := range strings.Fields(text) {
		parses := morph.Parse(word)
		if len(parses) == 0 {
			continue
		}
		p := parses[0]
		tokens = append(tokens, lemmarpc.Token{
			Form:  word,
			Lemma: p.NormalForm,
			Pos:   p.Tag.POS,
			Tags:  splitTags(p.Tag.RawTagsString),
		})
	}
	return tokens
}

// splitTags раскладывает сырую строку тегов OpenCorpora вида
// "NOUN,anim,femn sing,nomn" на плоский список граммем.
func splitTags(raw string) []string {
	var out []string
	for _, part := range strings.Fields(raw) {
		for _, g := range strings.Split(part, ",") {
			if g != "" {
				out = append(out, g)
			}
		}
	}
	return out
}

// resolveDict возвращает путь к каталогу словаря: флаг -dict, затем переменная
// TNS65_LEMMARU_DICT, затем packs/ru/lemmatizer от корня engine/ (поиском go.mod).
func resolveDict(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv("TNS65_LEMMARU_DICT"); env != "" {
		return env
	}
	if root := findEngineRoot(); root != "" {
		return filepath.Join(root, "packs", "ru", "lemmatizer")
	}
	return ""
}

// findEngineRoot поднимается от рабочего каталога (затем от каталога бинарника)
// до каталога с go.mod и возвращает его; пустая строка — корень не найден.
func findEngineRoot() string {
	if dir, err := os.Getwd(); err == nil {
		if root := walkUp(dir); root != "" {
			return root
		}
	}
	if exe, err := os.Executable(); err == nil {
		if root := walkUp(filepath.Dir(exe)); root != "" {
			return root
		}
	}
	return ""
}

// walkUp поднимается от dir к родителям до каталога с go.mod.
func walkUp(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
