package turn

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tns65/internal/journal"
	"tns65/internal/sediment"
)

// seedScene заводит ход игрока и его сцену с последним сигналом kind; пустой
// kind — сигнала нет.
func seedScene(t *testing.T, p *Pipeline, sessionID, turnID, sceneID int64, playerText, kind, createdAt string) {
	t.Helper()
	if _, err := p.DB.Exec("turns",
		`INSERT INTO turns (id, session_id, player_text, reply_text, path, selector_decision, saw_recommendation, trust_level, latency_ms, degraded)
		 VALUES (?, ?, ?, 'Ответ мира.', 'bank', '4 repeatable', 0, 1, 0, 0)`,
		turnID, sessionID, playerText); err != nil {
		t.Fatalf("INSERT turns: %v", err)
	}
	if _, err := p.DB.Exec("scenes",
		`INSERT INTO scenes (id, session_id, day, status, prose, turn_id) VALUES (?, ?, 1, 'closed', 'Проза сцены.', ?)`,
		sceneID, sessionID, turnID); err != nil {
		t.Fatalf("INSERT scenes: %v", err)
	}
	if kind == "" {
		return
	}
	if _, err := p.DB.Exec("scene_signals",
		`INSERT INTO scene_signals (scene_id, kind, created_at) VALUES (?, ?, ?)`,
		sceneID, kind, createdAt); err != nil {
		t.Fatalf("INSERT scene_signals: %v", err)
	}
}

// feedbackState — подтверждённый ход с готовым промптом сцены: на нём стадия
// эскалации и стадия исполнения проверяются без остальных стадий конвейера.
func feedbackState(sessionID int64, playerText string) *TurnState {
	return &TurnState{
		Input:   TurnInput{SessionID: sessionID, PlayerText: playerText, At: time.Now()},
		Path:    pathWeaver,
		Confirm: Confirmation{Decision: DecisionConfirmed},
		Prepath: Prepath{Prompt: "Иван стоит у ворот и ждёт."},
	}
}

// feedbackLevels возвращает уровни эскалации из событий turn.feedback по порядку.
func feedbackLevels(t *testing.T, logPath string) []string {
	t.Helper()
	var levels []string
	for _, line := range readJournalLines(t, logPath) {
		var rec struct {
			Event  string          `json:"event"`
			Fields []journal.Field `json:"fields"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка журнала не JSON: %v\n%s", err, line)
		}
		if rec.Event != feedbackEvent {
			continue
		}
		for _, f := range rec.Fields {
			if f.Key == "level" {
				levels = append(levels, f.Value)
			}
		}
	}
	return levels
}

func TestFeedbackEscalation(t *testing.T) {
	t.Run("два дизлайка подряд — регенерация новым промптом и откат", func(t *testing.T) {
		p, logPath := newTestPipeline(t)
		seedScene(t, p, 7, 101, 1, "Я вхожу в город.", "dislike", "2026-01-01T00:00:00Z")

		// Первый дизлайк подряд: мягкая регенерация сцены новым промптом.
		st := feedbackState(7, "Я иду дальше.")
		if err := (&feedbackStage{db: p.DB, logger: p.Logger}).Run(context.Background(), st); err != nil {
			t.Fatalf("feedback: %v", err)
		}
		if st.Feedback.Level != sediment.EscalationSoft || st.Feedback.Streak != 1 {
			t.Fatalf("план регенерации = %+v, want level=%d streak=1", st.Feedback, sediment.EscalationSoft)
		}
		ch := make(chan string, 1)
		ch <- "Он отступает в тень."
		close(ch)
		slot := &fakeProseSlot{ch: ch}
		if err := (&executeStage{weaver: slot}).Run(context.Background(), st); err != nil {
			t.Fatalf("execute: %v", err)
		}
		prompt := slot.lastPrompt()
		if !strings.Contains(prompt, "переписывание отклонённой сцены") || !strings.Contains(prompt, feedbackReason) {
			t.Fatalf("промпт регенерации не собран: %q", prompt)
		}
		if !strings.Contains(st.Reply, "Иван стоит у ворот") {
			t.Fatalf("ответ регенерации = %q, want подкладку сцены", st.Reply)
		}

		// Второй дизлайк подряд: сцена отклонена снова — откат к её сырому ходу.
		seedScene(t, p, 7, 102, 2, "Я жду ответа.", "dislike", "2026-01-01T00:01:00Z")
		st2 := feedbackState(7, "Я иду дальше.")
		if err := (&feedbackStage{db: p.DB, logger: p.Logger}).Run(context.Background(), st2); err != nil {
			t.Fatalf("feedback (2): %v", err)
		}
		if st2.Feedback.Level != sediment.EscalationRollback || st2.Feedback.Streak != 2 {
			t.Fatalf("план отката = %+v, want level=%d streak=2", st2.Feedback, sediment.EscalationRollback)
		}
		if st2.Feedback.Rollback != "Я жду ответа." {
			t.Fatalf("откат = %q, want сырой ход игрока", st2.Feedback.Rollback)
		}
		ch2 := make(chan string, 1)
		ch2 <- "Новый ход."
		close(ch2)
		slot2 := &fakeProseSlot{ch: ch2}
		if err := (&executeStage{weaver: slot2}).Run(context.Background(), st2); err != nil {
			t.Fatalf("execute (2): %v", err)
		}
		if st2.Reply != "Я жду ответа." {
			t.Fatalf("ответ отката = %q, want сырой ход игрока", st2.Reply)
		}
		if len(slot2.prompts) != 0 {
			t.Fatalf("при откате слот вызван %d раз, want 0", len(slot2.prompts))
		}

		if levels := feedbackLevels(t, logPath); len(levels) != 2 || levels[0] != "1" || levels[1] != "2" {
			t.Fatalf("события turn.feedback: %v, want [1 2]", levels)
		}
	})

	t.Run("лайк обрывает серию дизлайков", func(t *testing.T) {
		p, logPath := newTestPipeline(t)
		seedScene(t, p, 8, 201, 1, "Я вхожу в город.", "dislike", "2026-01-01T00:00:00Z")
		seedScene(t, p, 8, 202, 2, "Я жду ответа.", "like", "2026-01-01T00:01:00Z")

		st := feedbackState(8, "Я иду дальше.")
		if err := (&feedbackStage{db: p.DB, logger: p.Logger}).Run(context.Background(), st); err != nil {
			t.Fatalf("feedback: %v", err)
		}
		if st.Feedback.Level != sediment.EscalationNone {
			t.Fatalf("план = %+v, want эскалации нет", st.Feedback)
		}
		if levels := feedbackLevels(t, logPath); len(levels) != 0 {
			t.Fatalf("события turn.feedback без эскалации: %v", levels)
		}
	})

	t.Run("нет сигналов — эскалации нет", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		seedScene(t, p, 9, 301, 1, "Я вхожу в город.", "", "")

		st := feedbackState(9, "Я иду дальше.")
		if err := (&feedbackStage{db: p.DB, logger: p.Logger}).Run(context.Background(), st); err != nil {
			t.Fatalf("feedback: %v", err)
		}
		if st.Feedback.Level != sediment.EscalationNone || st.Feedback.SceneID != 0 {
			t.Fatalf("план = %+v, want эскалации нет", st.Feedback)
		}
	})
}
