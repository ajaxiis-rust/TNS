package senses

// vectors.go — слой эмбеддингов смыслов (T-15-03): контексты смыслов пака,
// центроиды смыслов и слияние триггерного и векторного счёта. Центроиды
// считаются лениво и живут в памяти прогона, кэш в базу не заводится.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"tns65/internal/models"
)

// Границы числа контекстов смысла (шаг 1).
const (
	minContexts = 5
	maxContexts = 10
)

// DefaultEmbedWeight — вес векторного счёта смысла по умолчанию
// (deepread.senses_embed_weight, шаг 3); 0 — только триггерный счёт (T-15-02).
const DefaultEmbedWeight = 3.0

// SenseContexts — контексты одного смысла (шаг 1): одно-два предложения с
// триггерами смысла, без имён собственных.
type SenseContexts struct {
	Lemma    string   `json:"lemma"`
	POS      string   `json:"pos"`
	Sense    string   `json:"sense"`
	Contexts []string `json:"contexts"`
}

// contextsFile — схема senses_contexts.json (шаг 1).
type contextsFile struct {
	Version  int             `json:"version"`
	Contexts []SenseContexts `json:"contexts"`
}

// Contexts — контексты смыслов пака по ключу (лемма, часть речи, смысл).
type Contexts struct {
	bySense map[string][]string
}

// LoadContexts читает и валидирует senses_contexts.json пака (шаг 1): файл не
// разобрался, версия схемы не 1, смысла нет в словаре, контекстов меньше пяти
// или больше десяти, контекст пуст, повторяется или содержит имя собственное,
// контекстов смысла словаря нет.
func LoadContexts(path string, dict *Dictionary) (*Contexts, error) {
	if dict == nil {
		return nil, fmt.Errorf("senses: контексты смыслов без словаря")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	var f contextsFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("senses: %s: %w", name, err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("senses: %s: версия схемы %d, ожидалась 1", name, f.Version)
	}
	if len(f.Contexts) == 0 {
		return nil, fmt.Errorf("senses: %s: контексты смыслов пусты", name)
	}

	c := &Contexts{bySense: make(map[string][]string, len(f.Contexts))}
	for i, sc := range f.Contexts {
		lemma := normalizeLemma(sc.Lemma)
		pos := strings.ToUpper(strings.TrimSpace(sc.POS))
		sense := strings.TrimSpace(sc.Sense)
		if lemma == "" || pos == "" || sense == "" {
			return nil, fmt.Errorf("senses: %s: контексты %d: пустые лемма, часть речи или смысл", name, i)
		}
		if !dict.hasSense(lemma, pos, sense) {
			return nil, fmt.Errorf("senses: %s: %s/%s/%s: смысла нет в словаре", name, lemma, pos, sense)
		}
		key := senseKeyOf(lemma, pos, sense)
		if _, dup := c.bySense[key]; dup {
			return nil, fmt.Errorf("senses: %s: контексты смысла %s/%s/%s повторяются",
				name, lemma, pos, sense)
		}
		if len(sc.Contexts) < minContexts || len(sc.Contexts) > maxContexts {
			return nil, fmt.Errorf("senses: %s: %s/%s/%s: контекстов %d, нужно %d..%d",
				name, lemma, pos, sense, len(sc.Contexts), minContexts, maxContexts)
		}
		seen := make(map[string]struct{}, len(sc.Contexts))
		contexts := make([]string, 0, len(sc.Contexts))
		for _, text := range sc.Contexts {
			text = strings.Join(strings.Fields(text), " ")
			if text == "" {
				return nil, fmt.Errorf("senses: %s: %s/%s/%s: пустой контекст", name, lemma, pos, sense)
			}
			if _, dup := seen[text]; dup {
				return nil, fmt.Errorf("senses: %s: %s/%s/%s: контекст %q повторяется",
					name, lemma, pos, sense, text)
			}
			if hasProperNoun(text) {
				return nil, fmt.Errorf("senses: %s: %s/%s/%s: имя собственное в контексте %q",
					name, lemma, pos, sense, text)
			}
			seen[text] = struct{}{}
			contexts = append(contexts, text)
		}
		c.bySense[key] = contexts
	}

	for _, entries := range dict.groups {
		for _, s := range entries {
			if _, ok := c.bySense[senseKeyOf(s.Lemma, s.POS, s.Sense)]; !ok {
				return nil, fmt.Errorf("senses: %s: у смысла %s/%s нет контекстов",
					name, s.Lemma, s.Sense)
			}
		}
	}
	return c, nil
}

// hasSense сообщает, что смысл есть в группе (лемма, часть речи) словаря.
func (d *Dictionary) hasSense(lemma, pos, sense string) bool {
	for _, s := range d.groups[groupKey(lemma, pos)] {
		if s.Sense == sense {
			return true
		}
	}
	return false
}

// senseKeyOf — ключ смысла: группа (лемма, часть речи) и имя смысла.
func senseKeyOf(lemma, pos, sense string) string {
	return groupKey(lemma, pos) + "\x00" + sense
}

// hasProperNoun сообщает, что в тексте есть слово с заглавной буквы не в начале
// предложения: имена собственные в контекстах смыслов запрещены (шаг 1).
func hasProperNoun(text string) bool {
	var prev rune
	for _, word := range strings.Fields(text) {
		runes := []rune(word)
		if unicode.IsUpper(runes[0]) && prev != 0 && !strings.ContainsRune(".!?…", prev) {
			return true
		}
		prev = runes[len(runes)-1]
	}
	return false
}

// VectorLayer — слой эмбеддингов прогона (шаг 2–3): эмбеддер, вес
// deepread.senses_embed_weight и центроиды смыслов. Центроид смысла считается
// лениво при первом его омониме и переиспользуется всеми чанками книги; ошибка
// эмбеддера отключает слой до конца прогона, счёт остаётся триггерным.
type VectorLayer struct {
	dict      *Dictionary
	contexts  *Contexts
	embed     models.Embedder
	weight    float64
	centroids map[string][]float32
	down      bool
}

// NewVectorLayer собирает слой векторов прогона; nil — нет словаря, контекстов
// или эмбеддера либо вес не положителен: счёт остаётся триггерным (T-15-02).
func NewVectorLayer(dict *Dictionary, contexts *Contexts, embed models.Embedder, weight float64) *VectorLayer {
	if dict == nil || contexts == nil || embed == nil || math.IsNaN(weight) || weight <= 0 {
		return nil
	}
	return &VectorLayer{
		dict:      dict,
		contexts:  contexts,
		embed:     embed,
		weight:    weight,
		centroids: make(map[string][]float32),
	}
}

// Weight — вес векторного счёта слоя.
func (l *VectorLayer) Weight() float64 {
	if l == nil {
		return 0
	}
	return l.weight
}

// ResolveWithVectors — Resolve со смысловым вектором (шаг 3): итоговый счёт
// смысла — trigger_score + weight × cosine(окно, центроид). Недоступный
// эмбеддер оставляет только триггерный счёт, прогон не падает; nil-слой —
// поведение T-15-02.
func (d *Dictionary) ResolveWithVectors(tokens []Token, text string, layer *VectorLayer) []Decision {
	if layer == nil {
		return d.Resolve(tokens)
	}
	return d.resolve(tokens, text, layer)
}

// scores возвращает векторный вклад смыслов группы: вес × близость окна к
// центроиду смысла. nil — окно не собралось, векторы недоступны или слой
// отключён: счёт остаётся триггерным.
func (l *VectorLayer) scores(entries []Sense, windowText string) []float64 {
	if l == nil || l.down || strings.TrimSpace(windowText) == "" {
		return nil
	}
	vec := l.embedText(windowText)
	if len(vec) == 0 {
		return nil
	}
	out := make([]float64, len(entries))
	for i, s := range entries {
		centroid := l.centroidOf(s)
		if len(centroid) == 0 {
			continue
		}
		out[i] = l.weight * cosine(vec, centroid)
	}
	return out
}

// centroidOf — центроид смысла (шаг 2): среднее нормализованных векторов его
// контекстов, посчитанное один раз на прогон.
func (l *VectorLayer) centroidOf(s Sense) []float32 {
	key := senseKeyOf(s.Lemma, s.POS, s.Sense)
	if centroid, ok := l.centroids[key]; ok {
		return centroid
	}
	contexts := l.contexts.bySense[key]
	if len(contexts) == 0 {
		return nil
	}
	var sum []float64
	for _, text := range contexts {
		vec := l.embedText(text)
		if len(vec) == 0 {
			return nil
		}
		if sum == nil {
			sum = make([]float64, len(vec))
		}
		if len(vec) != len(sum) {
			l.fail()
			return nil
		}
		for i, x := range vec {
			sum[i] += float64(x)
		}
	}
	centroid := normalize(sum)
	if len(centroid) == 0 {
		return nil
	}
	l.centroids[key] = centroid
	return centroid
}

// embedText возвращает нормализованный вектор текста; ошибка эмбеддера
// отключает слой до конца прогона.
func (l *VectorLayer) embedText(text string) []float32 {
	if l.down {
		return nil
	}
	raw, err := l.embed.Embed(context.Background(), text)
	if err != nil || len(raw) == 0 {
		l.fail()
		return nil
	}
	vec := make([]float64, len(raw))
	for i, x := range raw {
		vec[i] = float64(x)
	}
	out := normalize(vec)
	if len(out) == 0 {
		l.fail()
	}
	return out
}

// fail отключает слой: дальше счёт остаётся триггерным.
func (l *VectorLayer) fail() {
	l.down = true
}

// normalize приводит вектор к единичной L2-норме; нулевой вектор — nil.
func normalize(v []float64) []float32 {
	var sumSq float64
	for _, x := range v {
		sumSq += x * x
	}
	norm := math.Sqrt(sumSq)
	if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
		return nil
	}
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(x / norm)
	}
	return out
}

// cosine — косинус угла между векторами; нулевая длина или разная размерность
// дают 0.
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	cos := dot / (math.Sqrt(na) * math.Sqrt(nb))
	if cos > 1 {
		return 1
	}
	if cos < -1 {
		return -1
	}
	return cos
}

// WindowText собирает текст окна омонима tokens[at] (шаг 4) из текста чанка:
// слова строки абзаца (Line с 1) в ±WindowRadius от позиции токена (Index с 0),
// без самого омонима — как в триггерном окне. Пусто — окно не собралось.
func WindowText(text string, tokens []Token, at int) string {
	if text == "" || at < 0 || at >= len(tokens) {
		return ""
	}
	lines := strings.Split(text, "\n")
	line := tokens[at].Line - 1
	if line < 0 || line >= len(lines) {
		return ""
	}
	words := strings.Fields(lines[line])
	idx := tokens[at].Index
	if idx < 0 || idx >= len(words) {
		return ""
	}
	lo := idx - WindowRadius
	if lo < 0 {
		lo = 0
	}
	hi := idx + WindowRadius
	if hi > len(words)-1 {
		hi = len(words) - 1
	}
	parts := make([]string, 0, hi-lo)
	parts = append(parts, words[lo:idx]...)
	parts = append(parts, words[idx+1:hi+1]...)
	return strings.Join(parts, " ")
}
