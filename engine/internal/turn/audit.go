package turn

import (
	"context"
	"strconv"

	"tns65/internal/journal"
)

// Журнал хода и наблюдаемость (T-03-16, F-09.1): на каждый ход пишется строка
// аудита — рекомендация Пилы, решение Селектора, путь, доверие, латентность; при
// деградации слота — событие turn.degraded с причиной, при срабатывании страховки
// из банка — отметка turn.fallback. События идут только константами events.go,
// поля хода в turns пишет коммит (T-03-05), новых таблиц аудит не вводит.

// auditStage — стадия журнала хода: пишет события аудита в журнал станка после
// коммита, когда латентность и признак деградации уже известны.
type auditStage struct {
	logger *journal.Logger
}

func (*auditStage) Name() string { return "audit" }

func (s *auditStage) Run(_ context.Context, st *TurnState) error {
	if s.logger == nil {
		return nil
	}
	_ = s.logger.Event("turn", journal.EventTurnSawRecommendation,
		journal.Field{Key: "value", Value: strconv.Itoa(st.SawRecommendation)})
	_ = s.logger.Event("turn", journal.EventTurnSelectorDecision,
		journal.Field{Key: "value", Value: st.SelectorDecision})
	_ = s.logger.Event("turn", journal.EventTurnPath,
		journal.Field{Key: "value", Value: st.Path})
	_ = s.logger.Event("turn", journal.EventTurnTrust,
		journal.Field{Key: "value", Value: strconv.Itoa(trustLevel(st.Confirm))})
	_ = s.logger.Event("turn", journal.EventTurnLatency,
		journal.Field{Key: "value", Value: strconv.FormatInt(st.LatencyMS, 10)})
	if st.Degraded {
		_ = s.logger.Event("turn", journal.EventTurnDegraded,
			journal.Field{Key: "reason", Value: st.DegradeReason})
	}
	if st.Reply == fallbackReply {
		_ = s.logger.Event("turn", journal.EventTurnFallback)
	}
	return nil
}
