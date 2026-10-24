// Package turn — конвейер реплики живого хода: из реплики игрока рождается ответ
// мира, ход доставляется всегда (T-03-01, LT-01, LT-15).
package turn

import (
	"context"
	"strconv"
	"time"

	"tns65/internal/db"
	"tns65/internal/journal"
)

// TurnInput — вход хода (T-03-01, шаг 1): реплика игрока с идентификатором сессии
// и моментом приёма.
type TurnInput struct {
	SessionID  int64
	PlayerText string
	At         time.Time
}

// TurnOutput — результат хода (T-03-01, шаг 1): ответ игроку, путь, признак
// деградации и латентность.
type TurnOutput struct {
	ReplyText string
	Path      string
	Degraded  bool
	LatencyMS int64
}

// Pipeline — скелет конвейера реплики: упорядоченный реестр стадий и журнал.
// В оперативную базу мира пишет только коммит-стадия после подтверждения
// (T-03-05); до неё след хода — события журнала.
type Pipeline struct {
	Logger *journal.Logger // журнал станка; nil — события не пишутся.
	DB     *db.Handle      // писатель core оперативной базы мира; nil — коммит пропускается.
}

// Run прогоняет реплику по реестру стадий и возвращает результат. Каждая стадия
// отмечается событием turn.stage; после стадий коммит пишет строку в turns.
func (p *Pipeline) Run(ctx context.Context, in TurnInput) (TurnOutput, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	started := in.At
	if started.IsZero() {
		started = time.Now()
	}

	st := &TurnState{Input: in, StartedAt: started}
	if cmd, ok := parseCommand(in.PlayerText); ok {
		st.Command = &cmd
	}

	for _, stage := range p.stages() {
		p.logStage(stage.Name(), st)
		if err := stage.Run(ctx, st); err != nil {
			return TurnOutput{}, err
		}
	}

	return TurnOutput{
		ReplyText: st.Reply,
		Path:      st.Path,
		Degraded:  st.Degraded,
		LatencyMS: st.LatencyMS,
	}, nil
}

// logStage — событие turn.stage в журнал станка до прогона стадии (шаг 3).
func (p *Pipeline) logStage(name string, st *TurnState) {
	if p.Logger == nil {
		return
	}
	_ = p.Logger.Event("turn", journal.EventTurnStage,
		journal.Field{Key: "stage", Value: name},
		journal.Field{Key: "session_id", Value: strconv.FormatInt(st.Input.SessionID, 10)})
}
