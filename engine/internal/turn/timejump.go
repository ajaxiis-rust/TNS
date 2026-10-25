package turn

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"tns65/internal/db"
	"tns65/internal/workshop"
)

// Минуты в сутках — граница мгновенного скачка и заказа в мастерскую (T-03-12,
// шаг 3): меньше — мгновенно, от суток — пересчёт в паузе.
const MinutesPerDay int64 = 1440

// NightThresholdMin — единственная константа ночи (T-03-12, шаг 4): молчание
// дольше 8 часов означает ночь. Её же читают T-03-14 и T-05-01.
const NightThresholdMin int64 = 480

// jumpOrderKind — вид заказа мастерской для пересчёта мира после скачка длиннее
// суток. Отдельного вида в реестре CHECK нет (T-02-11), пересчёт ставится видом
// analysis: мир пересчитывается в паузе по векторам судьбы (T-03-13).
const jumpOrderKind = workshop.KindAnalysis

// Событие скачка и его источник в оперативной базе мира (events).
const (
	jumpEventKind   = "time_jump"
	jumpEventSource = "core"
)

// jumpPathLine — краткая фактическая строка пути прыжка (LT-10.7): рутина Челнока
// «долго и без событий». Прыжок не оставляется пустым.
const jumpPathLine = "Долго и без событий."

// TimeSpan — декларация времени в реплике (T-03-12, шаг 2): цитата из реплики и
// длительность в целых минутах.
type TimeSpan struct {
	RawQuote    string
	DurationMin int64
}

// timeConfig — формат detectors_time.json (T-01-08, шаг 6).
type timeConfig struct {
	Units        map[string]string `json:"units"`
	Numerals     map[string]int64  `json:"numerals"`
	Prepositions []string          `json:"prepositions"`
}

// unitMinutes — минуты единицы времени по имени вида из словаря.
func unitMinutes(kind string) int64 {
	switch kind {
	case "hours":
		return 60
	case "days":
		return MinutesPerDay
	default:
		return 0
	}
}

// TimeDetector — детектор времени языкового пакета (detectors_time.json) без LLM:
// словарные формы единиц времени, числительные и предлоги «через»/«спустя».
// Словоформы единиц совпадают по согласному скелету, чтобы «дня»/«день» и
// «час»/«часа» узнавались без лемматизатора.
type TimeDetector struct {
	units        map[string]int64 // согласный скелет единицы → минуты.
	numerals     map[string]int64
	prepositions map[string]struct{}
}

// LoadTimeDetector читает detectors_time.json из каталога языкового пакета dir.
func LoadTimeDetector(dir string) (*TimeDetector, error) {
	b, err := os.ReadFile(filepath.Join(dir, "detectors_time.json"))
	if err != nil {
		return nil, err
	}
	var cfg timeConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	return newTimeDetector(cfg), nil
}

func newTimeDetector(cfg timeConfig) *TimeDetector {
	d := &TimeDetector{
		units:        make(map[string]int64, len(cfg.Units)),
		numerals:     cfg.Numerals,
		prepositions: make(map[string]struct{}, len(cfg.Prepositions)),
	}
	for word, kind := range cfg.Units {
		d.units[consonantSkeleton(word)] = unitMinutes(kind)
	}
	for _, p := range cfg.Prepositions {
		d.prepositions[p] = struct{}{}
	}
	if d.numerals == nil {
		d.numerals = map[string]int64{}
	}
	return d
}

// Detect находит декларацию времени в реплике: предлог, необязательное
// числительное и единицу времени. Без декларации — false.
func (d *TimeDetector) Detect(text string) (TimeSpan, bool) {
	toks := timeWords(text)
	for i := 0; i < len(toks); i++ {
		if _, ok := d.prepositions[toks[i].norm]; !ok {
			continue
		}
		if span, ok := d.spanAt(toks, i); ok {
			return span, true
		}
	}
	return TimeSpan{}, false
}

// spanAt разбирает декларацию, начиная с предлога в позиции i: предлог, затем
// числительное (необязательно) и единица времени.
func (d *TimeDetector) spanAt(toks []timeToken, i int) (TimeSpan, bool) {
	j := i + 1
	if j >= len(toks) {
		return TimeSpan{}, false
	}
	num := int64(1)
	if n, ok := d.numerals[toks[j].norm]; ok {
		num = n
		j++
		if j >= len(toks) {
			return TimeSpan{}, false
		}
	}
	minutes := d.units[consonantSkeleton(toks[j].norm)]
	if minutes == 0 {
		return TimeSpan{}, false
	}
	end := j + 1
	return TimeSpan{RawQuote: joinTimeWords(toks[i:end]), DurationMin: num * minutes}, true
}

// consonantSkeleton — согласный скелет слова: гласные и мягкий/твёрдый знак
// отбрасываются. «день» и «дня» дают «дн», «час» и «часа» — «чс».
func consonantSkeleton(s string) string {
	var b strings.Builder
	for _, r := range s {
		if !strings.ContainsRune("аеёиоуыэюяьъ", r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// timeToken — слово реплики: исходная форма и нормированная (строчными).
type timeToken struct {
	orig string
	norm string
}

// timeWords разбивает текст на слова, отбрасывая знаки препинания; нормированная
// форма — строчными буквами.
func timeWords(s string) []timeToken {
	var out []timeToken
	start := -1
	for i, r := range s {
		if unicode.IsLetter(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			orig := s[start:i]
			out = append(out, timeToken{orig: orig, norm: strings.ToLower(orig)})
			start = -1
		}
	}
	if start >= 0 {
		orig := s[start:]
		out = append(out, timeToken{orig: orig, norm: strings.ToLower(orig)})
	}
	return out
}

func joinTimeWords(toks []timeToken) string {
	parts := make([]string, len(toks))
	for i, t := range toks {
		parts[i] = t.orig
	}
	return strings.Join(parts, " ")
}

// TimeJump — состояние скачка времени в ходе (T-03-12): декларация, порядок
// пересчёта и строка пути. Пустой Span — декларации не было, часы мира просто
// идут 1:1.
type TimeJump struct {
	Span     TimeSpan
	PathLine string // краткая фактическая строка пути в ответе.
	Ordered  bool   // скачок от суток: пересчёт ушёл заказом в мастерскую.
}

// jumpEvent — событие скачка в events (шаг 3, до суток): скачок и просчёт
// ближнего круга.
type jumpEvent struct {
	DurationMin        int64  `json:"duration_min"`
	Quote              string `json:"quote"`
	InnerCircleRecount bool   `json:"inner_circle_recount"`
}

// jumpOrder — заказ пересчёта мира после скачка от суток (шаг 3).
type jumpOrder struct {
	DurationMin int64  `json:"duration_min"`
	Quote       string `json:"quote"`
	Kind        string `json:"kind"`
}

// timejumpStage — детектор времени на входе (T-03-12, «Правится» stages.go):
// читает реплику игрока, находит декларацию времени и применяет скачок. Ставится
// после исполнения пути, чтобы строка прыжка легла в уже собранный ответ.
type timejumpStage struct {
	db       *db.Handle
	detector *TimeDetector
}

func (*timejumpStage) Name() string { return "timejump" }

func (s *timejumpStage) Run(_ context.Context, st *TurnState) error {
	if st.Command != nil {
		return nil // слеш-команда времени не декларирует.
	}
	det := s.detector
	if det == nil {
		det = loadDefaultTimeDetector()
	}
	if det == nil {
		return nil // пакета нет — декларацию не распознать, ход не прерван.
	}
	span, ok := det.Detect(st.Input.PlayerText)
	if !ok {
		return nil
	}
	st.TimeJump.Span = span

	var err error
	if span.DurationMin >= MinutesPerDay {
		st.TimeJump.Ordered = true
		err = s.orderRecompute(span)
	} else {
		err = s.applyInstantJump(span)
	}
	if err != nil {
		return err
	}
	st.TimeJump.PathLine = jumpPathLine
	st.Reply = jumpPathLine + "\n" + st.Reply
	return nil
}

// applyInstantJump — скачок до суток (шаг 3): ближний круг NPC пересчитывается
// скачком; событие events фиксирует скачок и просчёт ближнего круга. Дальний круг
// идёт лениво в мастерской по векторам судьбы — их заводит T-03-13.
func (s *timejumpStage) applyInstantJump(span TimeSpan) error {
	if s.db == nil {
		return nil
	}
	payload, err := json.Marshal(jumpEvent{
		DurationMin:        span.DurationMin,
		Quote:              span.RawQuote,
		InnerCircleRecount: true,
	})
	if err != nil {
		return err
	}
	_, err = s.db.Exec("events",
		`INSERT INTO events (day, kind, payload_json, source) VALUES (?, ?, ?, ?)`,
		0, jumpEventKind, string(payload), jumpEventSource)
	return err
}

// orderRecompute — скачок от суток (шаг 3): заказ в мастерскую, мир
// пересчитывается в паузе; мгновенного события скачка нет.
func (s *timejumpStage) orderRecompute(span TimeSpan) error {
	if s.db == nil {
		return nil
	}
	return workshop.Enqueue(s.db, jumpOrderKind, workshop.StepHeavy, workshop.DefaultProvider, jumpOrder{
		DurationMin: span.DurationMin,
		Quote:       span.RawQuote,
		Kind:        jumpEventKind,
	})
}

// loadDefaultTimeDetector загружает детектор времени из packs/ru; при отсутствии
// пакета детектора нет.
func loadDefaultTimeDetector() *TimeDetector {
	d, err := LoadTimeDetector("packs/ru")
	if err != nil {
		return nil
	}
	return d
}

// TimeConfig — параметры времени из config.yaml (секция time).
type TimeConfig struct {
	NightThresholdMin int64 // night_threshold_min: молчание дольше — ночь.
}

// DefaultTimeConfig возвращает единственную константу ночи (480 минут = 8 часов).
func DefaultTimeConfig() TimeConfig {
	return TimeConfig{NightThresholdMin: NightThresholdMin}
}

// LoadTimeConfig читает секцию time из config.yaml.
func LoadTimeConfig(path string) (TimeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TimeConfig{}, err
	}
	return parseTimeSection(string(data))
}

// parseTimeSection разбирает топ-ключ `time:` со скалярным полем
// night_threshold_min.
func parseTimeSection(data string) (TimeConfig, error) {
	cfg := DefaultTimeConfig()

	inTime := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inTime = trimmed == "time:"
			continue
		}
		if !inTime {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			continue
		}
		if strings.TrimSpace(key) == "night_threshold_min" {
			cfg.NightThresholdMin = n
		}
	}
	if err := sc.Err(); err != nil {
		return TimeConfig{}, err
	}
	return cfg, nil
}
