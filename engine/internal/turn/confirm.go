package turn

import (
	"bufio"
	"context"
	"math"
	"os"
	"strconv"
	"strings"
)

// Подтверждение смысла реплики двумя голосами (T-03-04): словарь и вектор.
// Первый голос — доля лемм гипотезы, найденных в реплике; второй — косинус
// вектора целой реплики к вектору гипотезы. Контроль — по целой реплике,
// анализ — по предложениям (леммы сегментов). При расхождении словарь главнее
// вектора; в дерзком режиме при согласии с запасом рутина коммитится сразу.

// ConfirmConfig — пороги подтверждения из config.yaml (секция confirm).
type ConfirmConfig struct {
	DictThreshold   float64 // dict_match ≥ — словарь считает смысл подтверждённым.
	VectorThreshold float64 // cos ≥ — вектор согласен (при расхождении главнее словарь).
	MarginDict      float64 // запас словаря для дерзкого режима.
	MarginVector    float64 // запас вектора для дерзкого режима.
}

// DefaultConfirmConfig возвращает пороги, совпадающие со значениями config.yaml.
func DefaultConfirmConfig() ConfirmConfig {
	return ConfirmConfig{
		DictThreshold:   0.6,
		VectorThreshold: 0.75,
		MarginDict:      0.7,
		MarginVector:    0.85,
	}
}

// LoadConfirmConfig читает секцию confirm из файла конфигурации.
func LoadConfirmConfig(path string) (ConfirmConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfirmConfig{}, err
	}
	return parseConfirmSection(string(data))
}

// parseConfirmSection разбирает топ-ключ `confirm:` со скалярными полями
// dict_threshold, vector_threshold, margin_dict, margin_vector.
func parseConfirmSection(data string) (ConfirmConfig, error) {
	cfg := DefaultConfirmConfig()

	inConfirm := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inConfirm = trimmed == "confirm:"
			continue
		}
		if !inConfirm {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(key) {
		case "dict_threshold":
			cfg.DictThreshold = f
		case "vector_threshold":
			cfg.VectorThreshold = f
		case "margin_dict":
			cfg.MarginDict = f
		case "margin_vector":
			cfg.MarginVector = f
		}
	}
	if err := sc.Err(); err != nil {
		return ConfirmConfig{}, err
	}
	return cfg, nil
}

// Решения подтверждения (T-03-04, шаг 2).
const (
	DecisionConfirmed = "confirmed" // подтверждено (при расхождении — словарь главнее).
	DecisionReask     = "reask"     // не подтверждено — переспрос нейтральной записью.
	DecisionMargin    = "margin"    // согласие с запасом — коммит сразу (дерзкий режим).
)

// Confirmation — результат двухголосия: решение и его показатели. Живёт в памяти
// хода; записи в базы нет (T-03-04, «Писатель: нет записи»).
type Confirmation struct {
	Decision     string
	DictMatch    float64
	Cosine       float64
	Disagreement bool // dict_match ≥ порога, но cos < порога: решено по словарю.
}

// Confirmed — смысл признан подтверждённым (подтверждено или согласие с запасом).
func (c Confirmation) Confirmed() bool {
	return c.Decision == DecisionConfirmed || c.Decision == DecisionMargin
}

// confirm вычисляет два голоса и применяет таблицу решений (шаги 1–2).
// Пороги приходят из cfg (config.yaml, «Проверка»); при их отсутствии — дефолты.
func confirm(ctx context.Context, st *TurnState, cfg ConfirmConfig) Confirmation {
	if cfg.DictThreshold <= 0 {
		cfg.DictThreshold = 0.6
	}
	if cfg.VectorThreshold <= 0 {
		cfg.VectorThreshold = 0.75
	}
	if cfg.MarginDict <= 0 {
		cfg.MarginDict = 0.7
	}
	if cfg.MarginVector <= 0 {
		cfg.MarginVector = 0.85
	}

	replicaLemmas := collectLemmas(st.Cut)
	dictMatch := dictMatch(st.HypothesisLemmas, replicaLemmas)

	// Второй голос — вектор: косинус вектора целой реплики к вектору гипотезы.
	var cos float64
	if st.Embedder != nil && len(st.HypothesisLemmas) > 0 {
		if hv, err := st.Embedder.Embed(ctx, hypothesisText(st.HypothesisLemmas)); err == nil {
			cos = cosine(st.Cut.Whole, hv)
		}
	}

	out := Confirmation{
		DictMatch: dictMatch,
		Cosine:    cos,
	}

	// Таблица решений с числами (шаг 2).
	switch {
	case dictMatch >= cfg.MarginDict && cos >= cfg.MarginVector:
		// Запас: в дерзком режиме рутина коммитится сразу (LT-04.4).
		out.Decision = DecisionMargin
	case dictMatch >= cfg.DictThreshold:
		// Подтверждено; при cos ниже порога — расхождение, словарь главнее.
		out.Decision = DecisionConfirmed
		out.Disagreement = cos < cfg.VectorThreshold
	default:
		out.Decision = DecisionReask
	}
	return out
}

// dictMatch — доля лемм гипотезы, найденных в леммах реплики:
// |леммы гипотезы ∩ леммы реплики| / |леммы гипотезы| (шаг 1). Пустая гипотеза
// даёт 0 — подтверждать нечего.
func dictMatch(hypothesis, replica []string) float64 {
	if len(hypothesis) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(replica))
	for _, l := range replica {
		seen[l] = struct{}{}
	}
	matched := 0
	for _, l := range hypothesis {
		if _, ok := seen[l]; ok {
			matched++
		}
	}
	return float64(matched) / float64(len(hypothesis))
}

// hypothesisText — текст гипотезы для векторизации: её леммы одной строкой.
func hypothesisText(lemmas []string) string {
	return strings.Join(lemmas, " ")
}

// cosine — косинусная близость двух векторов; размерности и нулевая норма
// защищены: расхождение размерности или пустой вектор дают 0.
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
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
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
