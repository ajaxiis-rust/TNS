// Package censor — единый словарный фильтр пакета (T-04-11): лемматизирует
// текст и сверяет леммы со словарём запрета из языкового пакета.
package censor

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"tns65/internal/lemmarpc"
)

// Lemmatizer — минимальный контракт клиента лемматизации (T-01-09): разбор
// текста на токены с леммой. В проде — internal/lemmarpc.Client, в тестах
// подменяется.
type Lemmatizer interface {
	Lemmatize(text string) ([]lemmarpc.Token, error)
}

// stoplistFile — имя словарного файла пакета: по лемме в строке (T-01-08, шаг 6).
const stoplistFile = "stoplist.txt"

// Checker — словарный фильтр: лемматизатор и множество запретных лемм.
type Checker struct {
	lemmatizer Lemmatizer
	stoplist   map[string]struct{}
}

// New собирает фильтр из лемматизатора и запретных лемм stoplist.
func New(lemmatizer Lemmatizer, stoplist []string) *Checker {
	set := make(map[string]struct{}, len(stoplist))
	for _, s := range stoplist {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		set[strings.ToLower(s)] = struct{}{}
	}
	return &Checker{lemmatizer: lemmatizer, stoplist: set}
}

// Check лемматизирует text и сверяет леммы токенов со словарём. ok=false —
// найдена запретная лемма; hits — найденные запретные леммы без повторов.
// Отказ лемматизатора — fail-closed: (false, nil).
func (c *Checker) Check(text string) (bool, []string) {
	tokens, err := c.lemmatizer.Lemmatize(text)
	if err != nil {
		return false, nil
	}
	var hits []string
	seen := make(map[string]struct{})
	for _, tok := range tokens {
		lemma := strings.ToLower(tok.Lemma)
		if _, bad := c.stoplist[lemma]; !bad {
			continue
		}
		if _, dup := seen[lemma]; dup {
			continue
		}
		seen[lemma] = struct{}{}
		hits = append(hits, lemma)
	}
	return len(hits) == 0, hits
}

// Check — единый словарный фильтр пакета (T-04-11): лемматизирует text клиентом
// T-01-09 и сверяет леммы со stoplist.txt пакета lang. Совпадение по лемме
// ловит и словоформу. Словарь читается из packs/<lang>/stoplist.txt, не из базы.
func Check(text string, lang string) (bool, []string) {
	root := findEngineRoot()
	if root == "" {
		return false, nil
	}
	packDir := filepath.Join(root, "packs", lang)
	stoplist, err := LoadStoplist(packDir)
	if err != nil {
		return false, nil
	}
	binary := packLemmatizerBinary(root, packDir)
	if binary == "" {
		return false, nil
	}
	client := lemmarpc.NewClient(binary, nil)
	defer client.Close()
	return New(client, stoplist).Check(text)
}

// LoadStoplist читает запретные леммы из stoplist.txt пакета (по лемме в строке,
// T-01-08, шаг 6); пустые строки пропускаются.
func LoadStoplist(packDir string) ([]string, error) {
	f, err := os.Open(filepath.Join(packDir, stoplistFile))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out, sc.Err()
}

// packLemmatizerBinary возвращает путь бинарника лемматизатора из manifest.json
// пакета (lemmatizer.binary); пустая строка — бинарник не задан.
func packLemmatizerBinary(root, packDir string) string {
	raw, err := os.ReadFile(filepath.Join(packDir, "manifest.json"))
	if err != nil {
		return ""
	}
	var m struct {
		Lemmatizer struct {
			Binary string `json:"binary"`
		} `json:"lemmatizer"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || m.Lemmatizer.Binary == "" {
		return ""
	}
	name := m.Lemmatizer.Binary
	if filepath.IsAbs(name) || strings.ContainsRune(name, filepath.Separator) {
		return name
	}
	return filepath.Join(root, "bin", name)
}

// findEngineRoot поднимается от рабочего каталога до каталога с go.mod.
func findEngineRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
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
