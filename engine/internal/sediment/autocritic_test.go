package sediment_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/journal"
	"tns65/internal/sediment"
)

// fakeAutocriticSlot — слот с заготовками ответов по порядку вызовов; считает
// вызовы и запоминает роль и промпт последнего вызова.
type fakeAutocriticSlot struct {
	calls      int
	responses  []string
	lastRole   string
	lastPrompt string
}

func (s *fakeAutocriticSlot) Generate(_ context.Context, role, prompt string) (string, error) {
	s.lastRole = role
	s.lastPrompt = prompt
	s.calls++
	if s.calls <= len(s.responses) {
		return s.responses[s.calls-1], nil
	}
	return "", errors.New("заготовок больше нет")
}

// newTestLogger открывает журнал во временном каталоге.
func newTestLogger(t *testing.T) (*journal.Logger, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "journal.log")
	logger, err := journal.New(path)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	return logger, path
}

// journalEvents возвращает множество имён событий журнала.
func journalEvents(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	events := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var rec struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка журнала не JSON: %v\n%s", err, line)
		}
		events[rec.Event] = true
	}
	return events
}

func TestAutocritic(t *testing.T) {
	t.Run("невырезаемое помечено needs_review, не удаляя", func(t *testing.T) {
		marked := sediment.MarkNeedsReview("{NAME} вошёл. Он сказал, что тот человек ушёл.")

		if !strings.Contains(marked, "<needs_review>Он</needs_review>") {
			t.Errorf("местоимение не помечено: %q", marked)
		}
		if !strings.Contains(marked, "<needs_review>тот человек</needs_review>") {
			t.Errorf("пересказ не помечен: %q", marked)
		}
		// Фрагменты не удалены: исходные слова стоят внутри пометки.
		for _, span := range []string{"Он", "тот человек"} {
			if !strings.Contains(marked, span) {
				t.Errorf("невырезаемое удалено: нет %q в %q", span, marked)
			}
		}
		// Небуквенная граница: «он» внутри «слон» не помечается.
		if strings.Contains(sediment.MarkNeedsReview("слон"), "<needs_review>") {
			t.Errorf("фрагмент внутри слова помечен: %q", sediment.MarkNeedsReview("слон"))
		}
	})

	t.Run("вердикт issues пишется в Verdict и журнал", func(t *testing.T) {
		slot := &fakeAutocriticSlot{responses: []string{`{"verdict":"issues","issues":["неоднозначно"]}`}}
		logger, logPath := newTestLogger(t)

		in := sediment.SedimentRecord{
			SourceSceneID: 7,
			CleanedText:   "{NAME} вошёл. Он сказал.",
		}
		out := sediment.Autocritic(context.Background(), slot, logger, in)

		if out.Verdict != sediment.VerdictIssues {
			t.Fatalf("Verdict = %q, want %q", out.Verdict, sediment.VerdictIssues)
		}
		if out.CleanedText != in.CleanedText {
			t.Fatalf("CleanedText изменён: %q != %q", out.CleanedText, in.CleanedText)
		}
		if slot.lastRole != "autocritic" {
			t.Errorf("роль слота = %q, want %q", slot.lastRole, "autocritic")
		}
		if !strings.Contains(slot.lastPrompt, "<needs_review>Он</needs_review>") {
			t.Errorf("в промпт не попала очищенная форма с пометками: %q", slot.lastPrompt)
		}

		events := journalEvents(t, logPath)
		if !events["sediment.autocritic.verdict"] {
			t.Errorf("в журнале нет события sediment.autocritic.verdict")
		}
	})

	t.Run("мусорный ответ после двух повторов даёт degraded", func(t *testing.T) {
		slot := &fakeAutocriticSlot{responses: []string{"мусор", "мусор", "мусор"}}
		logger, logPath := newTestLogger(t)

		in := sediment.SedimentRecord{
			SourceSceneID: 8,
			CleanedText:   "{NAME} вошёл.",
		}
		out := sediment.Autocritic(context.Background(), slot, logger, in)

		if out.Verdict != sediment.VerdictDegraded {
			t.Fatalf("Verdict = %q, want %q", out.Verdict, sediment.VerdictDegraded)
		}
		if slot.calls != 3 {
			t.Errorf("вызовов слота = %d, want 3 (один плюс два повтора)", slot.calls)
		}
		if out.CleanedText != in.CleanedText {
			t.Fatalf("CleanedText изменён: %q != %q", out.CleanedText, in.CleanedText)
		}

		events := journalEvents(t, logPath)
		if !events["sediment.autocritic.degraded"] {
			t.Errorf("в журнале нет события sediment.autocritic.degraded")
		}
		if events["sediment.autocritic.verdict"] {
			t.Errorf("при деградации не должно быть события sediment.autocritic.verdict")
		}
	})

	t.Run("модель не может изменить cleaned_text через ответ", func(t *testing.T) {
		garbage := `{"verdict":"clean","issues":[],"cleaned_text":"ПОДМЕНА"}`
		slot := &fakeAutocriticSlot{responses: []string{garbage, garbage, garbage}}
		logger, _ := newTestLogger(t)

		in := sediment.SedimentRecord{
			SourceSceneID: 9,
			CleanedText:   "{NAME} вошёл.",
		}
		out := sediment.Autocritic(context.Background(), slot, logger, in)

		// Лишнее поле cleaned_text — неизвестное поле строгого JSON: ответ
		// считается мусорным, после повторов вердикт degraded, текст не тронут.
		if out.Verdict != sediment.VerdictDegraded {
			t.Fatalf("Verdict = %q, want %q", out.Verdict, sediment.VerdictDegraded)
		}
		if out.CleanedText != in.CleanedText {
			t.Fatalf("CleanedText изменён моделью: %q != %q", out.CleanedText, in.CleanedText)
		}
	})
}
