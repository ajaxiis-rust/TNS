package sediment

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
)

// DedupConfig — порог смыслового дедупа из config.yaml (секция sediment).
type DedupConfig struct {
	Threshold float64 // близость ≥ порога — дубль по смыслу, сцена отклоняется.
}

// DefaultDedupConfig возвращает порог, совпадающий со значением config.yaml.
func DefaultDedupConfig() DedupConfig {
	return DedupConfig{Threshold: 0.85}
}

// LoadDedupConfig читает секцию sediment из файла конфигурации.
func LoadDedupConfig(path string) (DedupConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DedupConfig{}, err
	}
	return parseSedimentSection(string(data))
}

// parseSedimentSection разбирает топ-ключ `sediment:` со скалярным полем
// dedup_threshold.
func parseSedimentSection(data string) (DedupConfig, error) {
	cfg := DefaultDedupConfig()

	inSediment := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inSediment = trimmed == "sediment:"
			continue
		}
		if !inSediment {
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
		if strings.TrimSpace(key) == "dedup_threshold" {
			cfg.Threshold = f
		}
	}
	if err := sc.Err(); err != nil {
		return DedupConfig{}, err
	}
	return cfg, nil
}

// IsDuplicate сообщает, дублирует ли vec смысл какой-либо существующей записи:
// линейный перебор всех векторов, близость не ниже порога — дубль (W-03.4).
// Первая запись без существующих векторов проходит всегда.
func IsDuplicate(vec []float32, existing [][]float32, threshold float64) bool {
	for _, other := range existing {
		if cosine(vec, other) >= threshold {
			return true
		}
	}
	return false
}

// cosine — косинусная близость двух векторов; расхождение размерности или
// нулевая норма защищены — дают 0.
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		na += ai * ai
		nb += bi * bi
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
