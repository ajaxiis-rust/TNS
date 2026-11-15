package deepread

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"tns65/internal/lemmarpc"
)

// ErrNoResident — пакет не задаёт бинарник резидента лемматизатора (W-23.6):
// лемматизированный текст без резидента не воспроизводится, прогон не начинается.
var ErrNoResident = errors.New("deepread: пакет не задаёт бинарник лемматизатора")

// Lemma — токен разбора книги: форма, лемма, часть речи и позиция токена.
// Line — номер строки текста с 1, Index — позиция токена в строке с 0.
// Parses — все разборы токена (лемма и часть речи, лучший первым), леммы в
// нижнем регистре; Lemma и POS дублируют первый разбор (IMP-10).
type Lemma struct {
	Form   string
	Lemma  string
	POS    string
	Parses []lemmarpc.Parse
	Line   int
	Index  int
}

// parses возвращает все разборы токена; токен без Parses даёт единственный
// разбор из Lemma и POS — так сопоставление со словарём работает и на
// резиденте без мультиразбора (IMP-10).
func (l Lemma) parses() []lemmarpc.Parse {
	if len(l.Parses) > 0 {
		return l.Parses
	}
	return []lemmarpc.Parse{{Lemma: l.Lemma, Pos: l.POS}}
}

// Run — резидент лемматизатора прогона книги (W-23.6): один процесс на прогон
// разбора, леммы живут в памяти прогона и в базу не пишутся.
type Run struct {
	mu     sync.Mutex
	client *lemmarpc.Client
	err    error
}

// OpenRun открывает прогон лемм на резиденте пакета packDir. Бинарник берётся
// из manifest.json пакета (lemmatizer.binary, путь относительно packDir);
// без бинарника возвращается ErrNoResident. Процесс поднимается при первом
// запросе и закрывается Close по завершении прогона.
func OpenRun(packDir string) (*Run, error) {
	binary := lemmatizerBinary(packDir)
	if binary == "" {
		return nil, ErrNoResident
	}
	return &Run{client: lemmarpc.NewClient(binary, nil)}, nil
}

// Lemmatize лемматизирует text построчно: на каждую непустую строку один
// запрос к резиденту, токены сохраняют номер строки и позицию в строке.
// Лемма приводится к нижнему регистру — в таком виде её сверяют со словарём
// категорий. Сбой резидента делает прогон недействительным: ошибка липнет к
// Run, следующие вызовы и Close возвращают её же.
func (r *Run) Lemmatize(text string) ([]Lemma, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	lemmas, err := r.lemmatizeText(text)
	if err != nil {
		r.err = err
		return nil, err
	}
	return lemmas, nil
}

// ChunkLemmas лемматизирует чанки книги тем же резидентом прогона: процесс
// один на книгу, а не на чанк. Сбой резидента обрывает прогон — возвращается
// nil, причина доступна через Err и Close.
func (r *Run) ChunkLemmas(chunks []string) [][]Lemma {
	out := make([][]Lemma, len(chunks))
	for i, chunk := range chunks {
		lemmas, err := r.Lemmatize(chunk)
		if err != nil {
			return nil
		}
		out[i] = lemmas
	}
	return out
}

// Err — первая ошибка прогона; nil, пока резидент отвечает.
func (r *Run) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// Close останавливает резидент и возвращает ошибку прогона, если она была.
func (r *Run) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	err := r.client.Close()
	if r.err != nil {
		return r.err
	}
	return err
}

// lemmatizeText — построчный прогон текста через резидент. Номера строк
// считаются по исходному тексту, пустые строки пропускаются без сдвига
// нумерации.
func (r *Run) lemmatizeText(text string) ([]Lemma, error) {
	lines := strings.Split(text, "\n")
	out := make([]Lemma, 0, len(lines))
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		tokens, err := r.client.Lemmatize(line)
		if err != nil {
			return nil, fmt.Errorf("deepread: лемматизация строки %d: %w", i+1, err)
		}
		for j, tok := range tokens {
			out = append(out, Lemma{
				Form:   tok.Form,
				Lemma:  strings.ToLower(tok.Lemma),
				POS:    tok.Pos,
				Parses: lowerParses(tok.AllParses()),
				Line:   i + 1,
				Index:  j,
			})
		}
	}
	return out, nil
}

// lowerParses приводит леммы разборов к нижнему регистру — в таком виде
// словарь категорий сверяет пару (лемма, часть речи) (IMP-10).
func lowerParses(parses []lemmarpc.Parse) []lemmarpc.Parse {
	out := make([]lemmarpc.Parse, len(parses))
	for i, p := range parses {
		out[i] = lemmarpc.Parse{Lemma: strings.ToLower(p.Lemma), Pos: p.Pos}
	}
	return out
}

// lemmatizerBinary возвращает путь бинарника лемматизатора из manifest.json
// пакета (lemmatizer.binary). Путь в манифесте считается относительно каталога
// пакета. Пустая строка — бинарник не задан или манифест не читается.
func lemmatizerBinary(packDir string) string {
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
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(packDir, filepath.FromSlash(name))
}
