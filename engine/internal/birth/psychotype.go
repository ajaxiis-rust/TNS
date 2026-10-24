package birth

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed data/metrics.json data/archetypes.json
var psychotypeFS embed.FS

// Axis — ось психотипа: два полюса. Левый полюс даёт нормированное значение
// +1.0, правый — −1.0 (контракт вычисления профиля, B-06.2).
type Axis struct {
	Name  string `json:"name"`
	Left  string `json:"left"`
	Right string `json:"right"`
}

// Axes — четыре оси психотипа в каноническом порядке кода (B-06.1).
var Axes = []Axis{
	{Name: "E/I", Left: "E", Right: "I"},
	{Name: "N/S", Left: "N", Right: "S"},
	{Name: "T/F", Left: "T", Right: "F"},
	{Name: "J/P", Left: "J", Right: "P"},
}

// Code — код психотипа: по одной букве на ось в порядке E/I, N/S, T/F, J/P
// (например, "ENTJ"). Пространство из 16 комбинаций, а не готовый перечень (B-06.3).
type Code [4]byte

func (c Code) String() string { return string(c[:]) }

// ParseCode разбирает строку вида "ENTJ" в код: ровно четыре буквы, каждая —
// один из полюсов соответствующей оси.
func ParseCode(s string) (Code, error) {
	var c Code
	if len(s) != len(Axes) {
		return c, fmt.Errorf("birth: код психотипа %q: нужно ровно %d букв", s, len(Axes))
	}
	for i, ax := range Axes {
		b := s[i]
		switch b {
		case ax.Left[0], ax.Right[0]:
			c[i] = b
		default:
			return c, fmt.Errorf("birth: код психотипа %q: на позиции %d ожидался полюс оси %s (%s/%s)",
				s, i+1, ax.Name, ax.Left, ax.Right)
		}
	}
	return c, nil
}

// AllCodes — пространство кодов: декартово произведение полюсов четырёх осей,
// ровно 16 комбинаций. Вычисляется из осей, а не хранится перечнем (B-06.3).
func AllCodes() []Code {
	out := []Code{{}}
	for i, ax := range Axes {
		var next []Code
		for _, c := range out {
			left, right := c, c
			left[i] = ax.Left[0]
			right[i] = ax.Right[0]
			next = append(next, left, right)
		}
		out = next
	}
	return out
}

func axisIndex(name string) (int, bool) {
	for i, ax := range Axes {
		if ax.Name == name {
			return i, true
		}
	}
	return 0, false
}

// axisValue — нормированное значение оси для выбранного полюса: левый +1.0,
// правый −1.0. Значения детерминированны (B-06.2).
func axisValue(ax Axis, pole byte) (float64, error) {
	switch pole {
	case ax.Left[0]:
		return 1.0, nil
	case ax.Right[0]:
		return -1.0, nil
	default:
		return 0, fmt.Errorf("birth: полюс %q не принадлежит оси %s", pole, ax.Name)
	}
}

// Metric — метрика профиля: имя, ось и вес (контракт data/metrics.json, B-06.2).
type Metric struct {
	Name   string  `json:"metric"`
	Axis   string  `json:"axis"`
	Weight float64 `json:"weight"`
}

// LoadMetrics возвращает 14 метрик профиля из data/metrics.json в порядке файла.
func LoadMetrics() ([]Metric, error) {
	b, err := psychotypeFS.ReadFile("data/metrics.json")
	if err != nil {
		return nil, err
	}
	var metrics []Metric
	if err := json.Unmarshal(b, &metrics); err != nil {
		return nil, err
	}
	return metrics, nil
}

// LoadArchetypes возвращает 12 нарративных архетипов Юнга (B-06.4) — отдельный
// этаж для лиц мира, не сводится к 16 кодам психотипа.
func LoadArchetypes() ([]string, error) {
	b, err := psychotypeFS.ReadFile("data/archetypes.json")
	if err != nil {
		return nil, err
	}
	var archs []string
	if err := json.Unmarshal(b, &archs); err != nil {
		return nil, err
	}
	return archs, nil
}

// Profile — профиль психотипа: имя метрики → значение (B-06.2).
type Profile map[string]float64

// ProfileFromCode вычисляет профиль по коду психотипа: для каждой метрики
// значение = нормированное значение её оси × вес. Чистая детерминированная
// функция — без LLM и без math/rand (B-06.2).
func ProfileFromCode(code Code, metrics []Metric) (Profile, error) {
	p := make(Profile, len(metrics))
	for _, m := range metrics {
		idx, ok := axisIndex(m.Axis)
		if !ok {
			return nil, fmt.Errorf("birth: метрика %q ссылается на неизвестную ось %q", m.Name, m.Axis)
		}
		v, err := axisValue(Axes[idx], code[idx])
		if err != nil {
			return nil, err
		}
		p[m.Name] = v * m.Weight
	}
	return p, nil
}
