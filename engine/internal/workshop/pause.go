package workshop

import (
	"sync"
	"time"
)

// PauseStep — ступень паузы (T-05-01, шаг 2): нет работы, лёгкая, средняя,
// тяжёлая и ночное окно. Ступень хранится в памяти, переходы ступеней пишутся в
// журнал станка (Писатель: нет записи).
type PauseStep string

const (
	PauseNone   PauseStep = "none"
	PauseLight  PauseStep = "light"
	PauseMedium PauseStep = "medium"
	PauseHeavy  PauseStep = "heavy"
	PauseNight  PauseStep = "night"
)

// Границы ступеней (T-05-01, шаг 2): [0,10 мин) — нет работы, [10 мин,1 ч] —
// лёгкая, (1 ч,3 ч] — средняя, (3 ч,8 ч] — тяжёлая, дольше 8 ч — ночное окно.
const (
	lightStepAfter  = 10 * time.Minute
	mediumStepAfter = time.Hour
	heavyStepAfter  = 3 * time.Hour
	nightStepAfter  = 8 * time.Hour
)

// PauseStepFor вычисляет ступень паузы по длительности молчания и состоянию
// главы (шаг 2). Границы: ровно 10 мин — лёгкая, ровно 1 ч — лёгкая, ровно 3 ч —
// средняя, ровно 8 ч — тяжёлая; дольше 8 ч при закрытой главе — ночь, при
// открытой — тяжёлая.
func PauseStepFor(silence time.Duration, chapterClosed bool) PauseStep {
	switch {
	case silence < lightStepAfter:
		return PauseNone
	case silence <= mediumStepAfter:
		return PauseLight
	case silence <= heavyStepAfter:
		return PauseMedium
	case silence <= nightStepAfter:
		return PauseHeavy
	default:
		if chapterClosed {
			return PauseNight
		}
		return PauseHeavy
	}
}

// PauseClock — источник времени паузы (T-05-01, шаг 1): текущий момент.
// testkit.Clock удовлетворяет контракту; в проде — системные часы.
type PauseClock interface {
	Now() time.Time
}

// realPauseClock — системные часы по умолчанию.
type realPauseClock struct{}

func (realPauseClock) Now() time.Time { return time.Now() }

// PauseTimer — таймер паузы: измеряет молчание игрока на сервере (шаг 1). Время
// приходит из PauseClock; реального ожидания в тестах нет.
type PauseTimer struct {
	mu         sync.Mutex
	clock      PauseClock
	lastActive time.Time
}

// NewPauseTimer заводит таймер на нулевом молчании; nil — системные часы.
func NewPauseTimer(c PauseClock) *PauseTimer {
	t := &PauseTimer{clock: c}
	if t.clock == nil {
		t.clock = realPauseClock{}
	}
	t.lastActive = t.clock.Now()
	return t
}

// MarkActive фиксирует активность игрока — молчание обнуляется.
func (t *PauseTimer) MarkActive() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastActive = t.clock.Now()
}

// Silence — длительность молчания с последней активности.
func (t *PauseTimer) Silence() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.clock.Now().Sub(t.lastActive)
}

// Step — ступень паузы для текущего молчания и состояния главы (шаг 2).
func (t *PauseTimer) Step(chapterClosed bool) PauseStep {
	return PauseStepFor(t.Silence(), chapterClosed)
}

// StepAllowed — вес заказа (light/heavy), допустимый на ступени паузы (шаг 3).
// Лёгкая и средняя ступени допускают только подготовку (StepLight); тяжёлая и
// ночь — полную ковку (StepLight и StepHeavy). Средняя допускает ту же
// подготовку, что и лёгкая: тяжёлые заказы открываются на тяжёлой ступени
// (T-05-02 ставит доукомплектацию только на тяжёлой).
func StepAllowed(orderStep Step, pause PauseStep) bool {
	switch pause {
	case PauseNone:
		return false
	case PauseLight, PauseMedium:
		return orderStep == StepLight
	case PauseHeavy, PauseNight:
		return orderStep == StepLight || orderStep == StepHeavy
	}
	return false
}

// KindHasProse — признак «проза» вида заказа (шаг 4): генерирует ли вид прозу.
// На лёгкой ступени такой вид к запуску не допускается.
func KindHasProse(k Kind) bool {
	return k != KindForgePlan
}

// CanRun — итоговый гейт запуска заказа: вес и признак прозы на ступени паузы
// (шаги 3–4).
func CanRun(k Kind, orderStep Step, pause PauseStep) bool {
	if !StepAllowed(orderStep, pause) {
		return false
	}
	if pause == PauseLight && KindHasProse(k) {
		return false
	}
	return true
}
