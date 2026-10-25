package selector

import (
	"slices"
	"time"

	"tns65/internal/db"
)

// Причины границы сцены — по трём признакам (шаг 5).
const (
	ReasonPlace        = "place"
	ReasonTime         = "time"
	ReasonParticipants = "participants"
)

// SceneSign — три признака границы сцены (шаг 5): место, время и участники.
// Смена любого из них фиксирует границу; смена места — это намерение
// «перемещение» из лексикона намерений (глаголы движения) или сдвиг положения
// кодом (T-03-15), смена времени — детектор времени (T-03-12), смена участников —
// новый деятель или глагол смены (глагол состояния границу не даёт).
type SceneSign struct {
	Place        int64   // место: place_id (сдвиг положения кодом).
	Move         string  // цитата намерения «перемещение»; пусто — без намерения.
	Time         int64   // метка времени мира (минута с начала мира, T-03-12).
	Participants []int64 // деятели сцены: ближний круг + названные игроком.
}

// Boundary — итог проверки границы: сменился ли хотя бы один признак и какой.
type Boundary struct {
	Changed bool
	Reason  string
}

// BoundaryDetector — датчик границы сцены: сравнивает признаки с предыдущим
// ходом; смена любого из трёх фиксирует границу (шаг 5). Живёт отдельно от
// глубины Пилы — это датчик, а не переменная нагнетания.
type BoundaryDetector struct {
	prev SceneSign
	has  bool
}

// NewBoundaryDetector возвращает датчик без предыдущего снимка.
func NewBoundaryDetector() *BoundaryDetector {
	return &BoundaryDetector{}
}

// Check возвращает, сменился ли признак, и запоминает снимок для следующего
// хода. Первый ход — не граница: сравнивать не с чем.
func (d *BoundaryDetector) Check(s SceneSign) Boundary {
	if !d.has {
		d.has = true
		d.prev = s
		return Boundary{}
	}
	out := Boundary{}
	switch {
	case s.Place != d.prev.Place || s.Move != d.prev.Move:
		out = Boundary{Changed: true, Reason: ReasonPlace}
	case s.Time != d.prev.Time:
		out = Boundary{Changed: true, Reason: ReasonTime}
	case !sameParticipants(s.Participants, d.prev.Participants):
		out = Boundary{Changed: true, Reason: ReasonParticipants}
	}
	d.prev = s
	return out
}

// sameParticipants сравнивает наборы деятелей без учёта порядка.
func sameParticipants(a, b []int64) bool {
	ca := slices.Clone(a)
	cb := slices.Clone(b)
	slices.Sort(ca)
	slices.Sort(cb)
	return slices.Equal(ca, cb)
}

// SceneArc — арка сцены: отдельный сигнал той же пилообразной формы, что и Пила.
// Обнуляется на границе сцены и растёт заново; хранится в scene_arc и не
// смешивается с saw_state (шаг 6).
type SceneArc struct {
	sceneID int64
	value   float64
	db      *db.Handle // писатель core оперативной базы; nil — только в памяти.
}

// NewSceneArc создаёт арку сцены sceneID; db опционален.
func NewSceneArc(sceneID int64, h *db.Handle) *SceneArc {
	return &SceneArc{sceneID: sceneID, db: h}
}

// Grow наращивает арку на rise, не выходя из [0,1], и пишет scene_arc.
func (a *SceneArc) Grow(rise float64) error {
	a.value = clamp01(a.value + rise)
	return a.persist()
}

// Reset обнуляет арку на границе сцены и пишет scene_arc.
func (a *SceneArc) Reset() error {
	a.value = 0
	return a.persist()
}

// Value — текущее значение арки сцены.
func (a *SceneArc) Value() float64 { return a.value }

// persist пишет значение арки в scene_arc одной строкой (scene_id, value,
// updated_at); без писателя — ничего.
func (a *SceneArc) persist() error {
	if a.db == nil {
		return nil
	}
	_, err := a.db.Exec("scene_arc",
		`INSERT INTO scene_arc (scene_id, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(scene_id) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		a.sceneID, a.value, time.Now().UTC().Format(time.RFC3339))
	return err
}
