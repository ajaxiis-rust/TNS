// План ковки по релевантности пути (T-21-12, W-02): в паузах куётся только
// релевантное текущему пути игрока. Днём куётся вечер, в ночном окне — утро и
// завтрашний день; суточная норма — 2–5 заготовок; дальняя заготовка ставится
// только при свободной очереди, ненужная снимается до исполнения.
package workshop

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"tns65/internal/db"
)

// ForgeTarget — цель ковки заготовки относительно пути игрока (W-02.2).
type ForgeTarget string

const (
	// TargetEvening — вечер текущего дня: дневная цель ковки.
	TargetEvening ForgeTarget = "evening"
	// TargetMorning — утро следующего дня: ближняя ночная цель ковки.
	TargetMorning ForgeTarget = "morning"
	// TargetTomorrow — завтрашний день: дальняя ночная цель ковки.
	TargetTomorrow ForgeTarget = "tomorrow"
)

// Суточная потребность в заготовках (W-02.3): 2–5 штук в сутки. Дальняя
// заготовка идёт сверх минимального набора и только при свободной очереди.
const (
	DailyBlanksMin = 2
	DailyBlanksMax = 5
)

// ForgeBlank — payload заказа-заготовки (W-02): цель ковки и тема из плана
// ковки. Заказ без цели планом не управляется — им распоряжается вид-владелец.
type ForgeBlank struct {
	Target ForgeTarget `json:"target"`
	Theme  string      `json:"theme"`
}

// ForgePlanEntry — строка плана ковки из заказа forge_plan (W-02.2): цель,
// тема и пики Пилы из гороскопа.
type ForgePlanEntry struct {
	Target ForgeTarget `json:"target"`
	Theme  string      `json:"theme"`
	Peaks  []string    `json:"peaks"`
}

// ForgePlanOrder — payload заказа forge_plan: расписание ковки книги по целям.
// Заказ рождения целей не несёт — ковка идёт по ступени паузы.
type ForgePlanOrder struct {
	Targets []ForgePlanEntry `json:"targets"`
}

// ForgePlan — план ковки на текущую паузу (W-02): цели окна ступени, остаток
// суточной нормы и занятость очереди.
type ForgePlan struct {
	Night   bool
	Targets []ForgeTarget
	Made    int
	Left    int
	Busy    bool
}

// PlanForge собирает план ковки (W-02.2–W-02.4): ступень паузы задаёт цели
// (ночное окно — ночь), made — заготовки, скованные за сутки, queued — число
// других незакрытых заказов очереди. Днём цель одна — вечер; в ночном окне утро
// покрывает возврат к игре, завтрашний день — дальняя заготовка.
func PlanForge(pause PauseStep, made, queued int) ForgePlan {
	p := ForgePlan{Night: pause == PauseNight, Made: made, Busy: queued > 0}
	if p.Night {
		p.Targets = []ForgeTarget{TargetMorning, TargetTomorrow}
	} else {
		p.Targets = []ForgeTarget{TargetEvening}
	}
	if p.Left = DailyBlanksMax - made; p.Left < 0 {
		p.Left = 0
	}
	return p
}

// Planned — входит ли цель в окно текущей ступени.
func (p ForgePlan) Planned(t ForgeTarget) bool {
	for _, x := range p.Targets {
		if x == t {
			return true
		}
	}
	return false
}

// Admit — допустима ли заготовка этой цели сейчас (W-02.3, W-02.4): сверх
// суточной нормы не куётся ничего, дальняя заготовка — только при свободной
// очереди и после закрытия минимального набора суток.
func (p ForgePlan) Admit(t ForgeTarget) bool {
	if !p.Planned(t) || p.Left == 0 {
		return false
	}
	if t == TargetTomorrow {
		return !p.Busy && p.Made >= DailyBlanksMin
	}
	return true
}

// restrict сужает цели окна объявленным планом ковки книги: заказ forge_plan с
// темами и пиками из гороскопа решает, что из окна куётся (W-02.2). Пустой
// список объявленных целей план не сужает.
func (p ForgePlan) restrict(declared []ForgeTarget) ForgePlan {
	if len(declared) == 0 {
		return p
	}
	var kept []ForgeTarget
	for _, t := range p.Targets {
		for _, d := range declared {
			if d == t {
				kept = append(kept, t)
				break
			}
		}
	}
	p.Targets = kept
	return p
}

// queuedBlank — незакрытая заготовка очереди: заказ queued с целью в payload.
type queuedBlank struct {
	id     int64
	target ForgeTarget
}

// ApplyForgePlan приводит очередь к плану ковки (T-21-12): заготовки с целью вне
// окна, сверх суточной нормы и дальние при занятой очереди снимаются до
// исполнения — status=cancelled (W-02.5). Заказы без цели в payload планом не
// управляются. Возвращает число снятых заготовок.
func ApplyForgePlan(h *db.Handle, pause PauseStep, now time.Time) (int, error) {
	queued, blanks, err := queueBlanks(h)
	if err != nil {
		return 0, err
	}
	made, err := blanksMadeOn(h, now)
	if err != nil {
		return 0, err
	}
	declared, err := planTargetsFromOrder(h)
	if err != nil {
		return 0, err
	}
	// Занятость очереди — снимок до прохода: все незакрытые заказы, кроме самой
	// заготовки. Снятые в этом проходе заготовки очередь для дальних целей не
	// освобождают — иначе исход зависел бы от порядка строк.
	others := queued - 1
	if others < 0 {
		others = 0
	}
	var cancelled int
	for _, b := range blanks {
		plan := PlanForge(pause, made, others).restrict(declared)
		if plan.Admit(b.target) {
			continue
		}
		if err := Cancel(h, b.id); err != nil {
			return cancelled, err
		}
		cancelled++
	}
	return cancelled, nil
}

// queueBlanks читает незакрытые заказы очереди (queued и running): общее число
// и заготовки с целью в payload — только их исполнение решает план ковки.
func queueBlanks(h *db.Handle) (int, []queuedBlank, error) {
	rows, err := h.Query(`SELECT id, status, payload_json FROM workshop_queue WHERE status IN (?, ?)`,
		string(StatusQueued), string(StatusRunning))
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	var (
		total  int
		blanks []queuedBlank
	)
	for rows.Next() {
		var (
			id     int64
			status string
			raw    string
		)
		if err := rows.Scan(&id, &status, &raw); err != nil {
			return 0, nil, err
		}
		total++
		if status != string(StatusQueued) {
			continue
		}
		var b ForgeBlank
		if err := json.Unmarshal([]byte(raw), &b); err != nil {
			return 0, nil, fmt.Errorf("workshop: payload заказа %d не JSON: %w", id, err)
		}
		if b.Target == "" {
			continue
		}
		blanks = append(blanks, queuedBlank{id: id, target: b.Target})
	}
	return total, blanks, rows.Err()
}

// blanksMadeOn — сколько заготовок сковано за сутки now (W-02.3): заказы-
// заготовки, поставленные в этот день (UTC), в любом статусе — снятая заготовка
// суточную норму не возвращает.
func blanksMadeOn(h *db.Handle, now time.Time) (int, error) {
	rows, err := h.Query(`SELECT payload_json FROM workshop_queue WHERE created_at LIKE ?`,
		now.UTC().Format("2006-01-02")+"%")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var made int
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return 0, err
		}
		var b ForgeBlank
		if err := json.Unmarshal([]byte(raw), &b); err != nil {
			return 0, fmt.Errorf("workshop: payload заготовки не JSON: %w", err)
		}
		if b.Target != "" {
			made++
		}
	}
	return made, rows.Err()
}

// planTargetsFromOrder читает цели из payload самого свежего заказа forge_plan
// (T-02-11): темы и пики из гороскопа задают, что куётся в окне (W-02). Заказа
// нет или он без целей — окно ступени не сужается.
func planTargetsFromOrder(h *db.Handle) ([]ForgeTarget, error) {
	var raw string
	err := h.QueryRow(`SELECT payload_json FROM workshop_queue WHERE kind = ? ORDER BY id DESC LIMIT 1`,
		string(KindForgePlan)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var p ForgePlanOrder
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("workshop: payload заказа forge_plan: %w", err)
	}
	var out []ForgeTarget
	for _, e := range p.Targets {
		if e.Target != "" {
			out = append(out, e.Target)
		}
	}
	return out, nil
}
