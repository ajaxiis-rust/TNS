package turn

import (
	"context"
	"strconv"
	"strings"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/sediment"
)

// Обработка сигналов игрока о последней сцене (W-04.3/4, DOCS/03-LIVE-TURN.md
// §2.8): первый дизлайк подряд — мягкая регенерация сцены новым промптом (на
// рутине — свежий выбор банка без вызова слота), второй — откат к сырому ходу
// игрока. Лайк и отсутствие сигналов эскалации не дают.

// feedbackEvent — событие журнала об эскалации дизлайка подряд.
const feedbackEvent = "turn.feedback"

// feedbackTag — тег свежего выбора банка при мягкой регенерации: ключ окна
// повторяемости меняется, Челнок выбирает запись заново (0 вызовов слота).
const feedbackTag = "перевыбор"

// feedbackScenesLimit — сколько последних сцен сессии читает стадия: серия
// дизлайков после двух всё равно откат, глубже читать нечего.
const feedbackScenesLimit = 8

// feedbackReason — причина отказа для промпта регенерации. В TNS35 причина была
// одна и захардкожена; Пила и настроение уточнят её отдельной задачей.
const feedbackReason = "слишком агрессивно"

// regenerationDirective — директива Ткачу при мягкой регенерации: причина отказа
// и требование не повторять отклонённый ход.
const regenerationDirective = "Это переписывание отклонённой сцены: причина отказа — %s. Напиши другой ход — мягче по тону, другими словами и действиями, не повторяя прежней сцены."

// FeedbackPlan — план хода по сигналам о последних сценах (W-04.3/4).
type FeedbackPlan struct {
	Level    int    // sediment.Escalation*: 0 — эскалации нет.
	Streak   int    // число дизлайков подряд.
	SceneID  int64  // последняя отклонённая сцена; 0 — сигналов нет.
	Reason   string // причина отказа для промпта регенерации.
	Rollback string // сырой ход игрока, к которому откатывает второй дизлайк.
}

// RollbackReady — откат возможен: второй дизлайк подряд и сырой ход известен.
func (p FeedbackPlan) RollbackReady() bool {
	return p.Level >= sediment.EscalationRollback && strings.TrimSpace(p.Rollback) != ""
}

// Regenerating — сцена переписывается новым промптом: первый дизлайк подряд или
// второй без известного сырого хода (деградация без отказа).
func (p FeedbackPlan) Regenerating() bool {
	if p.RollbackReady() {
		return false
	}
	return p.Level >= sediment.EscalationSoft
}

// regenerationInstruction — директива регенерации с причиной отказа; пусто, если
// план регенерации не требует.
func (p FeedbackPlan) regenerationInstruction() string {
	if !p.Regenerating() {
		return ""
	}
	return strings.Replace(regenerationDirective, "%s", p.Reason, 1)
}

// feedbackStage — стадия обработки сигналов игрока: читает последние сцены
// сессии и их сигналы, определяет серию дизлайков и кладёт план в состояние
// хода; исполняет план стадия исполнения, событие эскалации пишется здесь.
type feedbackStage struct {
	db     *db.Handle
	logger *journal.Logger
}

func (*feedbackStage) Name() string { return "feedback" }

func (s *feedbackStage) Run(_ context.Context, st *TurnState) error {
	if s.db == nil {
		return nil
	}
	plan, err := readFeedback(s.db, st.Input.SessionID)
	if err != nil {
		return err
	}
	st.Feedback = plan
	if plan.Level == sediment.EscalationNone || s.logger == nil {
		return nil
	}
	_ = s.logger.Event("turn", feedbackEvent,
		journal.Field{Key: "level", Value: strconv.Itoa(plan.Level)},
		journal.Field{Key: "streak", Value: strconv.Itoa(plan.Streak)},
		journal.Field{Key: "scene_id", Value: strconv.FormatInt(plan.SceneID, 10)})
	return nil
}

// feedbackScenesSQL — последние сцены сессии с последним сигналом и сырым ходом
// игрока, породившим сцену: откат возвращает именно этот ход.
const feedbackScenesSQL = `
SELECT s.id,
       COALESCE((SELECT ss.kind FROM scene_signals ss WHERE ss.scene_id = s.id
                 ORDER BY ss.created_at DESC, ss.rowid DESC LIMIT 1), ''),
       COALESCE((SELECT t.player_text FROM turns t WHERE t.id = s.turn_id), '')
FROM scenes s
WHERE s.session_id = ?
ORDER BY s.id DESC
LIMIT ?`

// readFeedback читает сигналы последних сцен сессии от свежей к ранней и сводит
// их в план: серия дизлайков подряд задаёт уровень эскалации, последняя
// отклонённая сцена — сцену регенерации и сырой ход для отката.
func readFeedback(h *db.Handle, sessionID int64) (FeedbackPlan, error) {
	rows, err := h.Query(feedbackScenesSQL, sessionID, feedbackScenesLimit)
	if err != nil {
		return FeedbackPlan{}, err
	}
	defer rows.Close()

	type sceneSignal struct {
		id   int64
		kind sediment.SignalKind
		raw  string
	}
	var scenes []sceneSignal
	for rows.Next() {
		var sc sceneSignal
		var kind string
		if err := rows.Scan(&sc.id, &kind, &sc.raw); err != nil {
			return FeedbackPlan{}, err
		}
		sc.kind = sediment.SignalKind(kind)
		scenes = append(scenes, sc)
	}
	if err := rows.Err(); err != nil {
		return FeedbackPlan{}, err
	}
	if len(scenes) == 0 {
		return FeedbackPlan{}, nil
	}

	kinds := make([]sediment.SignalKind, 0, len(scenes))
	for _, sc := range scenes {
		kinds = append(kinds, sc.kind)
	}
	plan := FeedbackPlan{
		Streak: sediment.DislikeStreak(kinds),
	}
	plan.Level = sediment.Escalation(plan.Streak)
	if plan.Level == sediment.EscalationNone {
		return plan, nil
	}
	// Эскалация есть — свежая сцена и есть отклонённая: по ней регенерация, её
	// сырой ход — точка отката.
	plan.SceneID = scenes[0].id
	plan.Rollback = scenes[0].raw
	plan.Reason = feedbackReason
	return plan, nil
}
