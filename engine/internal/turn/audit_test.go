package turn

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"tns65/internal/journal"
	"tns65/internal/selector"
)

// confirmedState — подтверждённый ход с заполненными сигналами Селектора и Пилы:
// на нём коммит пишет строку turns, а аудит — события хода в журнал.
func confirmedState() *TurnState {
	return &TurnState{
		Input:             TurnInput{SessionID: 1, PlayerText: "идти ехать плыть бежать", At: time.Now()},
		Reply:             "Ответ мира.",
		Path:              "bank",
		SawRecommendation: selector.RecommendationCode(selector.RecommendBank),
		SelectorDecision:  "4 repeatable",
		Confirm:           Confirmation{Decision: DecisionConfirmed},
		Prepath:           Prepath{LexiconLoaded: true, Frame: SceneFrame{HeroPlaceID: 0}},
	}
}

// runAudit прогоняет коммит и аудит над состоянием хода: коммит пишет строку
// turns, аудит — события хода в журнал.
func runAudit(t *testing.T, p *Pipeline, st *TurnState) {
	t.Helper()
	if err := (&commitStage{db: p.DB}).Run(context.Background(), st); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := (&auditStage{logger: p.Logger}).Run(context.Background(), st); err != nil {
		t.Fatalf("audit: %v", err)
	}
}

// eventNames возвращает множество имён событий журнала.
func eventNames(t *testing.T, logPath string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, line := range readJournalLines(t, logPath) {
		var rec struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка журнала не JSON: %v\n%s", err, line)
		}
		out[rec.Event] = true
	}
	return out
}

// fieldValue возвращает значение поля key первого события event; пусто — нет.
func fieldValue(t *testing.T, logPath, event, key string) string {
	t.Helper()
	for _, line := range readJournalLines(t, logPath) {
		var rec struct {
			Event  string          `json:"event"`
			Fields []journal.Field `json:"fields"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка журнала не JSON: %v\n%s", err, line)
		}
		if rec.Event != event {
			continue
		}
		for _, f := range rec.Fields {
			if f.Key == key {
				return f.Value
			}
		}
	}
	return ""
}

func TestAudit(t *testing.T) {
	t.Run("события аудита и поля turns за ход", func(t *testing.T) {
		p, logPath := newTestPipeline(t)
		runAudit(t, p, confirmedState())

		events := eventNames(t, logPath)
		for _, e := range []string{
			journal.EventTurnSawRecommendation,
			journal.EventTurnSelectorDecision,
			journal.EventTurnPath,
			journal.EventTurnTrust,
			journal.EventTurnLatency,
		} {
			if !events[e] {
				t.Fatalf("в журнале нет события %s", e)
			}
		}

		var sawRec, trust, latency, degraded int
		var selDec, path string
		if err := p.DB.QueryRow(`SELECT saw_recommendation, selector_decision, path, trust_level, latency_ms, degraded FROM turns`).
			Scan(&sawRec, &selDec, &path, &trust, &latency, &degraded); err != nil {
			t.Fatalf("turns query: %v", err)
		}
		if sawRec != selector.RecommendationCode(selector.RecommendBank) {
			t.Fatalf("turns.saw_recommendation = %d, want %d", sawRec, selector.RecommendationCode(selector.RecommendBank))
		}
		if selDec == "" || path == "" {
			t.Fatalf("поля turns не заполнены: selector_decision=%q path=%q", selDec, path)
		}
		if trust != 1 {
			t.Fatalf("turns.trust_level = %d, want 1", trust)
		}
		if latency < 0 {
			t.Fatalf("turns.latency_ms = %d, want ≥ 0", latency)
		}
		if degraded != 0 {
			t.Fatalf("turns.degraded = %d, want 0", degraded)
		}
	})

	t.Run("деградация слота и страховка из банка", func(t *testing.T) {
		p, logPath := newTestPipeline(t)
		st := confirmedState()
		st.Degraded = true
		st.DegradeReason = "slot"
		st.Reply = fallbackReply
		runAudit(t, p, st)

		events := eventNames(t, logPath)
		if !events[journal.EventTurnDegraded] {
			t.Fatalf("при деградации в журнале нет события %s", journal.EventTurnDegraded)
		}
		if reason := fieldValue(t, logPath, journal.EventTurnDegraded, "reason"); reason == "" {
			t.Fatalf("событие %s без причины", journal.EventTurnDegraded)
		}
		if !events[journal.EventTurnFallback] {
			t.Fatalf("при страховке в журнале нет события %s", journal.EventTurnFallback)
		}

		var degraded int
		if err := p.DB.QueryRow(`SELECT degraded FROM turns`).Scan(&degraded); err != nil {
			t.Fatalf("turns query: %v", err)
		}
		if degraded != 1 {
			t.Fatalf("turns.degraded = %d, want 1", degraded)
		}
	})

	t.Run("страховка из банка без деградации", func(t *testing.T) {
		p, logPath := newTestPipeline(t)
		st := confirmedState()
		st.Reply = fallbackReply
		runAudit(t, p, st)

		events := eventNames(t, logPath)
		if !events[journal.EventTurnFallback] {
			t.Fatalf("при страховке в журнале нет события %s", journal.EventTurnFallback)
		}
		if events[journal.EventTurnDegraded] {
			t.Fatalf("без деградации в журнале не должно быть события %s", journal.EventTurnDegraded)
		}
	})

	t.Run("события идут константами events.go", func(t *testing.T) {
		// Значения констант совпадают с именами событий журнала: аудит не пишет
		// строковые литералы имён, а ссылается на journal/events.go (T-01-03).
		want := map[string]string{
			journal.EventTurnSawRecommendation: "turn.saw_recommendation",
			journal.EventTurnSelectorDecision:  "turn.selector_decision",
			journal.EventTurnPath:              "turn.path",
			journal.EventTurnTrust:             "turn.trust",
			journal.EventTurnLatency:           "turn.latency",
			journal.EventTurnDegraded:          "turn.degraded",
			journal.EventTurnFallback:          "turn.fallback",
		}
		for k, v := range want {
			if k != v {
				t.Fatalf("константа события %q != значение %q", k, v)
			}
		}
	})
}
