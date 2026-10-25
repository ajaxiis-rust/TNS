// Package selector — выбор пути хода: Пила оценивает глубину момента и советует
// путь, Селектор решает (LT-05, LT-09). Здесь же живёт датчик границы сцены и
// отдельный сигнал арки сцены. Пила предлагает, Селектор решает.
package selector

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"

	"tns65/internal/db"
	"tns65/internal/journal"
)

// Имена метрик психотипа, из которых Пила вычисляет наклон нагнетания (шаг 2).
const (
	metricExtraversion = "extraversion"
	metricRationality  = "rationality"
)

// Profile — юнгианский профиль (14 метрик) из карточки NPC. Пила берёт его
// готовым и не пересчитывает: профилировщик не вызывается (LT-09.2).
type Profile map[string]float64

// K — коэффициент наклона нагнетания: среднее метрик экстраверсии и рациональности,
// нормированное в [0,1]. Значения профиля лежат в [−1,1]; отсутствующие метрики
// считаются нулём.
func K(p Profile) float64 {
	if len(p) == 0 {
		return 0
	}
	mean := (p[metricExtraversion] + p[metricRationality]) / 2
	return clamp01((mean + 1) / 2)
}

// SawConfig — параметры сигнала Пилы из config.yaml (секция saw, шаг 2).
type SawConfig struct {
	RiseMin float64 // saw.rise_min: наклон при K=0.
	RiseMax float64 // saw.rise_max: наклон при K=1.
	Peak    float64 // saw.peak: порог пика и разрядки.
	Fall    float64 // saw.fall: множитель разрядки s := s·fall.
	Flat    float64 // saw.flat: ниже — ровный участок (банк).
}

// DefaultSawConfig возвращает параметры, совпадающие со значениями config.yaml.
func DefaultSawConfig() SawConfig {
	return SawConfig{RiseMin: 0.03, RiseMax: 0.08, Peak: 0.8, Fall: 0.35, Flat: 0.3}
}

// LoadSawConfig читает секцию saw из файла конфигурации.
func LoadSawConfig(path string) (SawConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SawConfig{}, err
	}
	return parseSawSection(string(data))
}

// parseSawSection разбирает топ-ключ `saw:` со скалярными полями rise_min,
// rise_max, peak, fall, flat.
func parseSawSection(data string) (SawConfig, error) {
	cfg := DefaultSawConfig()

	inSaw := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inSaw = trimmed == "saw:"
			continue
		}
		if !inSaw {
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
		case "rise_min":
			cfg.RiseMin = f
		case "rise_max":
			cfg.RiseMax = f
		case "peak":
			cfg.Peak = f
		case "fall":
			cfg.Fall = f
		case "flat":
			cfg.Flat = f
		}
	}
	if err := sc.Err(); err != nil {
		return SawConfig{}, err
	}
	return cfg, nil
}

// MoodLink — связь настроения реплики с психотипом (LT-09.9): эхо или контрапункт.
// Тип задаёт психотип; Пила получает его готовым и применяет множитель наклона.
type MoodLink int

const (
	MoodNeutral      MoodLink = iota // настроение не задано — множитель 1.
	MoodEcho                         // совпадение настроения — rise × 1.25.
	MoodCounterpoint                 // контрапункт — rise × 0.75.
)

// Recommendation — рекомендация Пилы на ход. Статус Пилы — советник, не судья:
// окончательное решение пути принимает Селектор (T-03-09).
type Recommendation string

const (
	RecommendWeaver Recommendation = "weaver" // s ≥ peak — Ткач.
	RecommendBank   Recommendation = "bank"   // s < flat — Челнок.
	RecommendNone   Recommendation = ""       // между — без рекомендации.
)

// Saw — Пила: пилообразная переменная нагнетания, ведущаяся по нитям отношений.
// Отдельная переменная на каждую нить; общая — сумма с насыщением при 1 (шаг 1).
type Saw struct {
	cfg     SawConfig
	k       float64
	charID  int64
	threads map[int64]float64
	db      *db.Handle      // писатель core оперативной базы; nil — только в памяти.
	logger  *journal.Logger // журнал станка; nil — события не пишутся.
}

// NewSaw создаёт Пилу над профилем profile для героя charID. db и logger
// опциональны: без них Пила живёт только в памяти.
func NewSaw(profile Profile, cfg SawConfig, charID int64, h *db.Handle, logger *journal.Logger) *Saw {
	if cfg.RiseMin <= 0 {
		cfg.RiseMin = 0.03
	}
	if cfg.RiseMax <= 0 {
		cfg.RiseMax = 0.08
	}
	if cfg.Peak <= 0 {
		cfg.Peak = 0.8
	}
	if cfg.Fall <= 0 {
		cfg.Fall = 0.35
	}
	if cfg.Flat <= 0 {
		cfg.Flat = 0.3
	}
	return &Saw{
		cfg:     cfg,
		k:       K(profile),
		charID:  charID,
		threads: map[int64]float64{},
		db:      h,
		logger:  logger,
	}
}

// Step — один ход на нити отношения threadID: нагнетание s += rise (с эхом или
// контрапунктом по link), разрядка при достижении пика и рекомендация пути.
// Записывает новое значение нити в saw_state, если Пиле передан писатель.
func (s *Saw) Step(threadID int64, link MoodLink) (Recommendation, error) {
	rise := s.cfg.RiseMin + (s.cfg.RiseMax-s.cfg.RiseMin)*s.k
	switch link {
	case MoodEcho:
		rise *= 1.25
	case MoodCounterpoint:
		rise *= 0.75
	}

	v := clamp01(s.threads[threadID] + rise)
	s.threads[threadID] = v
	rec := s.recommend()

	// Разрядка резкая (шаг 2): при достижении пика s := s · fall.
	if v >= s.cfg.Peak {
		s.threads[threadID] = v * s.cfg.Fall
	}
	return rec, s.persist(threadID)
}

// ThreadValue — переменная Пилы нити отношения threadID (шаг 1).
func (s *Saw) ThreadValue(threadID int64) float64 {
	return s.threads[threadID]
}

// Total — суммарное значение Пилы с насыщением при 1 (шаг 1).
func (s *Saw) Total() float64 {
	var sum float64
	for _, v := range s.threads {
		sum += v
	}
	return clamp01(sum)
}

// PhaseBreak — сбой фазы событием мира извне (шаг 3): s := s·0.5 на всех нитях
// и событие saw.phase_break в журнале.
func (s *Saw) PhaseBreak() {
	for id, v := range s.threads {
		s.threads[id] = v * 0.5
		_ = s.persist(id)
	}
	if s.logger != nil {
		_ = s.logger.Event("selector", journal.EventSawPhaseBreak)
	}
}

// recommend — рекомендация по суммарному значению (шаг 3): s ≥ peak — Ткач,
// s < flat — банк, между — без рекомендации.
func (s *Saw) recommend() Recommendation {
	switch total := s.Total(); {
	case total >= s.cfg.Peak:
		return RecommendWeaver
	case total < s.cfg.Flat:
		return RecommendBank
	default:
		return RecommendNone
	}
}

// persist пишет значение нити в saw_state одной строкой (character_id, thread_id,
// value, updated_at); без писателя — ничего.
func (s *Saw) persist(threadID int64) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec("saw_state",
		`INSERT INTO saw_state (character_id, thread_id, value, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(character_id, thread_id) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		s.charID, threadID, s.threads[threadID], time.Now().UTC().Format(time.RFC3339))
	return err
}

// clamp01 ограничивает значение отрезком [0,1].
func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}
