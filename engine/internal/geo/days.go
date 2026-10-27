package geo

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"tns65/internal/db"
)

// dayNormKey — ключ world_meta с нормой дня мира (T-05-13, шаг 2).
const dayNormKey = "dayNormKm"

// Норма дня мира: 20–30 км пешком, дефолт 25 (W-06.4, T-05-13, шаг 2).
const (
	MinDayNormKm     = 20.0
	MaxDayNormKm     = 30.0
	DefaultDayNormKm = 25.0
)

// Path — известный путь между местами из paths: длина в км и коэффициент
// трения по типу пути (тракт/река/море).
type Path struct {
	LengthKm float64
	Friction float64
}

// CheckDayNorm — диапазон нормы дня мира [20,30] (T-05-13, шаг 2).
func CheckDayNorm(v float64) error {
	if v < MinDayNormKm || v > MaxDayNormKm {
		return fmt.Errorf("geo: dayNormKm %v вне диапазона [%v, %v]", v, MinDayNormKm, MaxDayNormKm)
	}
	return nil
}

// Geo — менеджер гео-расчёта мира; норма дня мира зафиксирована при создании.
type Geo struct {
	dayNormKm float64
}

// NewGeo создаёт Geo с нормой дня мира; вне диапазона — отказ с причиной
// (T-05-13, шаг 2).
func NewGeo(dayNormKm float64) (*Geo, error) {
	if err := CheckDayNorm(dayNormKm); err != nil {
		return nil, err
	}
	return &Geo{dayNormKm: dayNormKm}, nil
}

// DayNormKm возвращает норму дня мира.
func (g *Geo) DayNormKm() float64 { return g.dayNormKm }

// Days — контракт Days(from, to, path) float64 (T-05-13, шаг 2): с известным
// путём — length_km / (dayNormKm · friction); пути нет — геодезическое
// расстояние с трением 1.0 (оценка). Результат — дробные дни, хранение
// запрещено, округление — только при показе игроку (CeilDays).
func (g *Geo) Days(from, to Place, path *Path) float64 {
	if path != nil {
		return path.LengthKm / (g.dayNormKm * path.Friction)
	}
	return Distance(from, to) / g.dayNormKm
}

// ReadDayNorm читает dayNormKm из world_meta; записи нет — DefaultDayNormKm;
// значение вне диапазона — отказ с причиной (T-05-13, шаг 2).
func ReadDayNorm(h *db.Handle) (float64, error) {
	if h == nil {
		return DefaultDayNormKm, nil
	}
	var raw string
	err := h.QueryRow(`SELECT value FROM world_meta WHERE "key" = ?`, dayNormKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultDayNormKm, nil
	}
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("geo: разбор world_meta.%s: %w", dayNormKey, err)
	}
	if err := CheckDayNorm(v); err != nil {
		return 0, err
	}
	return v, nil
}

// CeilDays — округление дней вверх только при показе игроку (T-05-13, шаг 2).
func CeilDays(days float64) float64 { return math.Ceil(days) }

// forbiddenPromptSubstrings — фиксированный список запретных подстрок сборки
// промпта модели (T-05-13, шаг 5, W-20.2): модель не вычисляет расстояния и дни.
var forbiddenPromptSubstrings = []string{
	"посчитай расстояние",
	"посчитай дни",
}

// CheckPrompt — промпт-гейт (T-05-13, шаг 5): отказ, если промпт содержит
// запретную подстроку «посчитай расстояние» или «посчитай дни».
func CheckPrompt(prompt string) error {
	for _, s := range forbiddenPromptSubstrings {
		if strings.Contains(prompt, s) {
			return fmt.Errorf("geo: промпт содержит запретную подстроку %q", s)
		}
	}
	return nil
}

// TravelPrompt — сборка промпта модели (T-05-13, шаг 4): координаты передаются
// как есть, расстояние и дни — готовыми числами от кода (W-20.1, W-20.2).
func TravelPrompt(from, to Place, distanceKm, days float64) string {
	return fmt.Sprintf(
		"Путь от места (%s, %s) до места (%s, %s): расстояние %s км, дни %s.",
		strconv.FormatFloat(from.X, 'f', -1, 64),
		strconv.FormatFloat(from.Y, 'f', -1, 64),
		strconv.FormatFloat(to.X, 'f', -1, 64),
		strconv.FormatFloat(to.Y, 'f', -1, 64),
		strconv.FormatFloat(distanceKm, 'f', -1, 64),
		strconv.FormatFloat(days, 'f', -1, 64),
	)
}
