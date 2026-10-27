package paganel

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"tns65/internal/db"
	"tns65/internal/geo"
)

// TravelMode — режим походов мира (W-07.1).
type TravelMode string

// Режимы походов задаются полем анкеты travel_mode явно (реальная/вымышленная/
// смешанная); из жанров режим не выводится (W-07.1, T-06-03, шаг 1).
const (
	TravelReal    TravelMode = "реальная"
	TravelFiction TravelMode = "вымышленная"
	TravelMixed   TravelMode = "смешанная"
)

// Valid — режим походов из списка реальная|вымышленная|смешанная.
func (m TravelMode) Valid() bool {
	switch m {
	case TravelReal, TravelFiction, TravelMixed:
		return true
	}
	return false
}

// travelModeKey — ключ world_meta с полем анкеты travel_mode (T-02-03, шаг 3).
const travelModeKey = "travel_mode"

// ReadTravelMode читает режим походов из world_meta (T-06-03, шаг 1, W-07.1):
// значение хранится JSON-строкой (T-02-03, шаг 3); чтение смотрит только на ключ
// travel_mode — из жанров режим не выводится. Отсутствующий ключ или значение
// вне списка — отказ с причиной.
func ReadTravelMode(h *db.Handle) (TravelMode, error) {
	if h == nil {
		return "", fmt.Errorf("paganel: world_meta handle не задан")
	}
	var raw string
	err := h.QueryRow(`SELECT value FROM world_meta WHERE "key" = ?`, travelModeKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("paganel: world_meta.%s отсутствует", travelModeKey)
	}
	if err != nil {
		return "", fmt.Errorf("paganel: читать world_meta.%s: %w", travelModeKey, err)
	}
	var mode string
	if err := json.Unmarshal([]byte(raw), &mode); err != nil {
		return "", fmt.Errorf("paganel: разбор world_meta.%s: %w", travelModeKey, err)
	}
	m := TravelMode(mode)
	if !m.Valid() {
		return "", fmt.Errorf("paganel: режим походов %q вне списка реальная|вымышленная|смешанная", mode)
	}
	return m, nil
}

// namedTravel — разобранное слово игрока о пути: время в днях или расстояние в км.
type namedTravel struct {
	days       float64
	distanceKm float64
	isTime     bool
	isDistance bool
}

// FictionDistance калибрует дистанцию до упора в выдуманном мире из слов игрока
// (T-06-03, шаг 2, W-07.2): названное время («две недели пути») переводится в
// километры через норму дня (дней × dayNormKm), названное расстояние («300 км»)
// берётся как есть. Выдуманному миру внешние данные не нужны — наружу не выходим
// (W-07.5).
func FictionDistance(phrase string, dayNormKm float64) (float64, error) {
	n, err := parseNamedTravel(phrase)
	if err != nil {
		return 0, err
	}
	switch {
	case n.isDistance:
		return n.distanceKm, nil
	case n.isTime:
		return n.days * dayNormKm, nil
	default:
		return 0, fmt.Errorf("paganel: фраза %q не называет время или расстояние пути", phrase)
	}
}

// ruNumeral — русские слова чисел 1–10 (для «две недели пути» и подобного).
var ruNumeral = map[string]float64{
	"один": 1, "одна": 1, "одно": 1,
	"два": 2, "две": 2,
	"три":    3,
	"четыре": 4,
	"пять":   5,
	"шесть":  6,
	"семь":   7,
	"восемь": 8,
	"девять": 9,
	"десять": 10,
}

// timeUnits — слова единиц времени и число дней в единице.
var timeUnits = map[string]float64{
	"день": 1, "дня": 1, "дней": 1, "сутки": 1, "суток": 1,
	"неделя": 7, "недели": 7, "недель": 7, "неделю": 7,
	"месяц": 30, "месяца": 30, "месяцев": 30,
	"год": 365, "года": 365, "лет": 365,
}

// distanceUnits — слова единиц расстояния.
var distanceUnits = map[string]float64{
	"км": 1, "километр": 1, "километра": 1, "километров": 1,
}

// parseNamedTravel разбирает фразу о пути: число (цифрами или русским словом)
// плюс единица времени/расстояния (W-07.2). Не разобрано — отказ с причиной.
func parseNamedTravel(phrase string) (namedTravel, error) {
	words := strings.Fields(strings.ToLower(phrase))
	if len(words) == 0 {
		return namedTravel{}, fmt.Errorf("paganel: пустая фраза о пути")
	}
	for i, w := range words {
		n, ok := numberWord(w)
		if !ok {
			continue
		}
		for _, unit := range words[i+1:] {
			if days, ok := timeUnits[unit]; ok {
				return namedTravel{days: n * days, isTime: true}, nil
			}
			if factor, ok := distanceUnits[unit]; ok {
				return namedTravel{distanceKm: n * factor, isDistance: true}, nil
			}
		}
	}
	return namedTravel{}, fmt.Errorf("paganel: фраза %q не разобрана как время или расстояние пути", phrase)
}

// numberWord — число словами или цифрами.
func numberWord(w string) (float64, bool) {
	if n, err := strconv.ParseFloat(w, 64); err == nil {
		return n, true
	}
	n, ok := ruNumeral[w]
	return n, ok
}

// RealDays — дни пути в реальном мире по координатам газетира (T-06-03, шаг 3,
// W-07.3): реальные координаты первичны, наружу выходить можно. Расстояние —
// геодезия, дни = расстояние / норма дня (трение 1.0 — оценка без пути, T-05-13).
func RealDays(from, to Place, dayNormKm float64) float64 {
	return geo.Distance(geo.Place{X: from.X, Y: from.Y}, geo.Place{X: to.X, Y: to.Y}) / dayNormKm
}

// MixedPlace — место смешанного мира для калибровки: реальное место или книжная
// точка (Pinned). Книжные точки не сдвигаются смещением реального холста (W-07.4).
type MixedPlace struct {
	Place
	Pinned bool
}

// ShiftMixedCanvas сдвигает реальный холст смешанного мира на (dx, dy) в градусах
// (T-06-03, шаг 4, W-07.4): реальные (не книжные) места сдвигаются, книжные точки
// (Pinned) не двигаются. Обратное встраивание запрещено — книжную точку нельзя
// сдвинуть. Возвращает копию мест; исходный срез не меняется.
func ShiftMixedCanvas(places []MixedPlace, dx, dy float64) []MixedPlace {
	out := make([]MixedPlace, len(places))
	copy(out, places)
	for i := range out {
		if !out[i].Pinned {
			out[i].X += dx
			out[i].Y += dy
		}
	}
	return out
}
