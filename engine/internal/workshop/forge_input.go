package workshop

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Format — формат входа ковки (W-14.3, шаг 1): txt и fb2 принимаются, прочие
// форматы конвертируются до ковки, неподдержанный — отказ с причиной.
type Format string

const (
	FormatTXT Format = "txt"
	FormatFB2 Format = "fb2"
)

// ErrUnsupportedFormat — неподдержанный формат входа ковки.
var ErrUnsupportedFormat = errors.New("workshop: неподдержанный формат входа ковки")

// Chapter — заголовок главы из метаданных книги.
type Chapter struct {
	Title string `json:"title"`
}

// Chunk — кусок тела книги, нарезанный чанкером по границам глав (шаг 3).
type Chunk struct {
	Chapter string `json:"chapter"`
	Text    string `json:"text"`
}

// BookMeta — метаданные книги, извлечённые из входа (шаг 2).
type BookMeta struct {
	Title    string    `json:"title"`
	Author   string    `json:"author,omitempty"`
	Chapters []Chapter `json:"chapters,omitempty"`
}

// ParsedBook — результат разбора входа: метаданные книги и очередь чанков
// (шаг 5). Записи в базу на этом шаге нет.
type ParsedBook struct {
	Meta   BookMeta `json:"meta"`
	Chunks []Chunk  `json:"chunks"`
}

// ParseBookInput разбирает файл-вход ковки по расширению имени (шаг 1).
func ParseBookInput(path string) (*ParsedBook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("workshop: читать вход ковки %s: %w", path, err)
	}
	return parseBook(path, data)
}

// parseBook разбирает вход по имени файла: txt — напрямую в чанкер, fb2 —
// вставка перед чанкером (титул и главы в метаданные, тело — в чанкер).
func parseBook(name string, data []byte) (*ParsedBook, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt":
		return parseTXT(data), nil
	case ".fb2":
		return parseFB2(data)
	default:
		return nil, unsupportedFormat(name)
	}
}

func unsupportedFormat(name string) error {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		ext = "(без расширения)"
	}
	return fmt.Errorf("%w: %s (принимаются только txt и fb2)", ErrUnsupportedFormat, ext)
}

// parseTXT подаёт тело в чанкер напрямую (шаг 3); метаданных у txt нет.
func parseTXT(data []byte) *ParsedBook {
	return &ParsedBook{Chunks: chunkByChapters(string(data))}
}

// chapterHeadingRe — граница главы в тексте: «Глава N», «Глава N. Заголовок»,
// «Chapter N», необязательно с Markdown-решёткой.
var chapterHeadingRe = regexp.MustCompile(`^\s*(?:#{1,6}\s*)?(?:[Гг]лава|[Cc]hapter)(?:\s+\S.*)?$`)

// chunkByChapters режет тело на куски по границам глав (шаг 3): строка начала
// главы становится заголовком куска, текст до следующей границы — его телом.
func chunkByChapters(body string) []Chunk {
	lines := strings.Split(body, "\n")
	var chunks []Chunk
	var cur *Chunk

	flush := func() {
		if cur == nil {
			return
		}
		cur.Text = strings.TrimSpace(cur.Text)
		chunks = append(chunks, *cur)
		cur = nil
	}

	for _, line := range lines {
		if chapterHeadingRe.MatchString(line) {
			flush()
			cur = &Chunk{Chapter: strings.TrimSpace(line)}
			continue
		}
		if cur == nil {
			if strings.TrimSpace(line) == "" {
				continue
			}
			cur = &Chunk{}
		}
		cur.Text += line + "\n"
	}
	flush()
	return chunks
}

// parseFB2 — вставка перед чанкером (шаг 2): из XML титул и главы уходят в
// метаданные, тело — в существующий чанкер. Отдельного парсера тела нет — один
// обход XML собирает и заголовки, и текст глав.
func parseFB2(data []byte) (*ParsedBook, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var (
		meta        BookMeta
		chapters    []Chapter
		authorParts []string
		bodyText    strings.Builder
	)

	var stack []string
	var buf []byte
	capture := ""

	flush := func() string {
		s := strings.TrimSpace(string(buf))
		buf = nil
		capture = ""
		return s
	}
	inElement := func(name string) bool {
		for _, e := range stack {
			if e == name {
				return true
			}
		}
		return false
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("workshop: fb2: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			local := t.Name.Local
			stack = append(stack, local)
			switch local {
			case "book-title":
				if capture == "" {
					capture = "book-title"
				}
			case "first-name", "middle-name", "last-name", "nickname":
				if capture == "" && inElement("author") {
					capture = "author-name"
				}
			case "title":
				if capture == "" && inElement("body") {
					capture = "chapter-title"
				}
			case "p":
				if capture == "" && inElement("body") {
					capture = "p"
				}
			}

		case xml.EndElement:
			local := t.Name.Local
			switch local {
			case "book-title":
				if capture == "book-title" {
					meta.Title = flush()
				}
			case "first-name", "middle-name", "last-name", "nickname":
				if capture == "author-name" {
					if part := flush(); part != "" {
						authorParts = append(authorParts, part)
					}
				}
			case "author":
				meta.Author = strings.Join(authorParts, " ")
				authorParts = nil
			case "title":
				if capture == "chapter-title" {
					if title := flush(); title != "" {
						chapters = append(chapters, Chapter{Title: title})
						bodyText.WriteString(title + "\n\n")
					}
				}
			case "p":
				if capture == "p" {
					if text := flush(); text != "" {
						bodyText.WriteString(text + "\n\n")
					}
				}
			}
			stack = stack[:len(stack)-1]

		case xml.CharData:
			if capture != "" {
				buf = append(buf, t...)
			}
		}
	}

	meta.Chapters = chapters
	return &ParsedBook{Meta: meta, Chunks: chunkByChapters(bodyText.String())}, nil
}
