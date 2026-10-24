package testkit

import (
	"context"
	"hash/fnv"
	"math"
	"sync"
)

// EmbedDim — размерность векторов слота эмбеддингов (MD-03.2).
const EmbedDim = 1024

// FakeEmbed — фейковый эмбеддер: детерминированный вектор EmbedDim измерений из
// хеша текста (L2-нормированный), счётчик вызовов; без сети.
type FakeEmbed struct {
	mu    sync.Mutex
	calls int
}

// NewFakeEmbed возвращает эмбеддер с нулевым счётчиком.
func NewFakeEmbed() *FakeEmbed {
	return &FakeEmbed{}
}

// Embed возвращает детерминированный L2-нормированный вектор из хеша текста.
func (e *FakeEmbed) Embed(_ context.Context, text string) ([]float32, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	return embedVector(text), nil
}

// Calls — число вызовов Embed.
func (e *FakeEmbed) Calls() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func embedVector(text string) []float32 {
	h := fnv.New64a()
	h.Write([]byte(text))
	seed := h.Sum64()

	vec := make([]float32, EmbedDim)
	var sumSq float64
	for i := range vec {
		seed = nextRand(seed)
		// Старшие 53 бита дают ровно представимое [0,1) значение.
		v := float32(float64(seed>>11)/(1<<53)*2 - 1)
		vec[i] = v
		sumSq += float64(v) * float64(v)
	}
	norm := float32(math.Sqrt(sumSq))
	if norm == 0 {
		vec[0] = 1
		return vec
	}
	for i := range vec {
		vec[i] /= norm
	}
	return vec
}

// nextRand — splitmix64: детерминированный шаг ГСЧ от seed.
func nextRand(state uint64) uint64 {
	state += 0x9E3779B97F4A7C15
	z := state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}
