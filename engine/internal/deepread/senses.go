package deepread

// senses.go — смысловой слой salience (T-15-02): senses.json пака рядом со
// словарями категорий и счёт омонимов по решённому смыслу. Смыслы живут в
// памяти прогона, в базу ничего не пишется.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"tns65/internal/models"
	"tns65/internal/senses"
)

// sensesFileName — файл словаря смыслов в каталоге пака.
const sensesFileName = "senses.json"

// sensesContextsFileName — файл контекстов смыслов пака (T-15-03, шаг 1).
const sensesContextsFileName = "senses_contexts.json"

// defaultSensesMargin — порог отрыва смысла по умолчанию: константа T-15-01.
const defaultSensesMargin = senses.MinMargin

// senseEps — допуск сравнения порогов смысла (как floatEps в senses).
const senseEps = 1e-9

// SenseLayer — смысловой слой прогона: словарь смыслов пака и порог отрыва
// deepread.senses_margin (0 — константа T-15-01). Vector — слой векторов
// смыслов (T-20-03), nil — счёт остаётся триггерным.
type SenseLayer struct {
	Dict   *senses.Dictionary
	Margin float64
	Vector *senses.VectorLayer
}

// LoadSenses читает senses.json рядом со словарями категорий пака (шаг 1);
// файла нет — nil без ошибки, разбор работает как раньше.
func LoadSenses(packDir string) (*senses.Dictionary, error) {
	path := filepath.Join(packDir, sensesFileName)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return senses.Load(path)
}

// LoadSenseLayer собирает смысловой слой прогона из пака и конфига; senses.json
// нет — nil, теги считаются по T-14. Эмбеддер (T-20-03) и senses_contexts.json
// пака подключают к слою векторный счёт смыслов.
func LoadSenseLayer(packDir string, cfg SalienceConfig, embed ...models.Embedder) (*SenseLayer, error) {
	dict, err := LoadSenses(packDir)
	if err != nil {
		return nil, err
	}
	if dict == nil {
		return nil, nil
	}
	layer := &SenseLayer{Dict: dict, Margin: cfg.SensesMargin}
	if len(embed) == 0 || embed[0] == nil {
		return layer, nil
	}
	vectors, err := loadSenseVectors(packDir, dict, cfg.SensesEmbedWeight, embed[0])
	if err != nil {
		return nil, err
	}
	layer.Vector = vectors
	return layer, nil
}

// loadSenseVectors собирает senses.VectorLayer прогона (T-20-03): нужны словарь,
// senses_contexts.json пака и эмбеддер; файла нет или вес не положителен — nil,
// смыслы решаются только триггерами (T-15-02).
func loadSenseVectors(packDir string, dict *senses.Dictionary, weight float64, embed models.Embedder) (*senses.VectorLayer, error) {
	if dict == nil || embed == nil {
		return nil, nil
	}
	path := filepath.Join(packDir, sensesContextsFileName)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	contexts, err := senses.LoadContexts(path, dict)
	if err != nil {
		return nil, err
	}
	return senses.NewVectorLayer(dict, contexts, embed, weight), nil
}

// margin возвращает порог отрыва лучшего смысла от второго: значение конфига
// либо константа T-15-01 при 0. Решение senses несёт константу, поэтому порог
// конфига может только ужесточить отбор.
func (l *SenseLayer) margin() float64 {
	if l == nil || l.Margin == 0 {
		return senses.MinMargin
	}
	return l.Margin
}

// senseMark — смысл одного разбора токена: часть речи, признак решения и
// категория решённого смысла (пусто — смысл вне категорий разбора).
type senseMark struct {
	pos      string
	decided  bool
	category string
}

// senseKey — ключ разбора: лемма и часть речи.
type senseKey struct {
	lemma string
	pos   string
}

// senseMarks решает смыслы омонимов чанка (шаг 2): разборы токенов чанка
// подаются в senses.ResolveWithVectors, окно контекста — ±12 токенов в пределах
// строки; векторный слой слоя (T-20-03) добавляет к триггерному счёту близость
// окна к центроиду смысла. Решение принимается при отрыве от второго смысла не
// ниже порога слоя; незаданный смысл остаётся нерешённым. nil-слой — меток нет.
func senseMarks(chunk SalienceChunk, layer *SenseLayer) map[string][]senseMark {
	if layer == nil || layer.Dict == nil {
		return nil
	}
	tokens := make([]senses.Token, 0, len(chunk.Lemmas))
	keys := make([]senseKey, 0, len(chunk.Lemmas))
	for _, lemma := range chunk.Lemmas {
		for _, parse := range lemma.parses() {
			if parse.Lemma == "" {
				continue
			}
			tokens = append(tokens, senses.Token{
				Form:  lemma.Form,
				Lemma: parse.Lemma,
				POS:   parse.Pos,
				Line:  lemma.Line,
				Index: lemma.Index,
			})
			keys = append(keys, senseKey{lemma: parse.Lemma, pos: parse.Pos})
		}
	}
	decisions := layer.Dict.ResolveWithVectors(tokens, chunk.Text, layer.Vector)
	if len(decisions) == 0 {
		return nil
	}

	margin := layer.margin()
	out := make(map[string][]senseMark, len(decisions))
	for _, d := range decisions {
		k := keys[d.Index]
		mark := senseMark{pos: k.pos}
		if d.Decided && d.Margin+senseEps >= margin {
			mark.decided = true
			mark.category = d.Category
		}
		out[k.lemma] = append(out[k.lemma], mark)
	}
	return out
}

// senseAllows сообщает, даёт ли лемма словаря вклад в категорию key (шаг 2):
// смысл ни одного разбора не решён — вклад как прежде (консервативно); есть
// решение в key — вклад; все решения в других категориях — вклад 0.
func senseAllows(marks map[string][]senseMark, lemma, pos, key string) bool {
	entries := marks[lemma]
	if len(entries) == 0 {
		return true
	}
	matched, hit := false, false
	for _, m := range entries {
		if pos != "" && !strings.EqualFold(m.pos, pos) {
			continue
		}
		matched = true
		if !m.decided {
			return true
		}
		if m.category == key {
			hit = true
		}
	}
	if !matched {
		return true
	}
	return hit
}
