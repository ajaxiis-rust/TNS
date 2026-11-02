package workshop_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/workshop"
)

// ladderSlot — слот-шпион: по очереди возвращает заготовленные ответы и
// запоминает промпты каждого вызова.
type ladderSlot struct {
	calls   int
	prompts []string
	answers []string
}

func (s *ladderSlot) Generate(_ context.Context, _ string, prompt string) (string, error) {
	i := s.calls
	if i >= len(s.answers) {
		i = len(s.answers) - 1
	}
	s.calls++
	s.prompts = append(s.prompts, prompt)
	return s.answers[i], nil
}

// TestGenerateWithRetryLadder — лестница повторов (T-12-08, «Проверка»):
// фейковый слот «отказ → успех»; первая ступень несёт художественную рамку и
// рейтинг, вторая уводит трудную часть за кадр; каждый отказ пишет forge_log.
func TestGenerateWithRetryLadder(t *testing.T) {
	dir := t.TempDir()
	litPath := filepath.Join(dir, "literary.db")
	newDB(t, litPath, "literary")
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer lit.Close()

	slot := &ladderSlot{answers: []string{
		"не могу помочь с этим запросом.",
		"готовая проза.",
	}}
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	workshop.SetRating("16+")
	defer workshop.SetRating("")

	out, err := workshop.GenerateWithRetry(context.Background(), lit, workshop.GenerateOptions{
		Role:     "test-role",
		Lang:     "ru",
		Provider: workshop.DefaultProvider,
	}, func(_ int, _ string) string {
		return `{"task":"test-role","scene":"герой идёт по дороге"}`
	})
	if err != nil {
		t.Fatalf("GenerateWithRetry: %v", err)
	}
	if out != "готовая проза." {
		t.Fatalf("выход = %q, want %q", out, "готовая проза.")
	}
	if slot.calls != 2 {
		t.Fatalf("вызовов слота = %d, want 2 (отказ → успех)", slot.calls)
	}
	if !strings.Contains(slot.prompts[0], "художественная проза") {
		t.Fatalf("промпт ступени 1 без художественной рамки: %q", slot.prompts[0])
	}
	if !strings.Contains(slot.prompts[0], "16+") {
		t.Fatalf("промпт ступени 1 без рейтинга: %q", slot.prompts[0])
	}
	if !strings.Contains(slot.prompts[1], "offscreen") {
		t.Fatalf("промпт ступени 2 без закадрового пересказа: %q", slot.prompts[1])
	}
	if got := countRefusalLog(t, lit, "test-role"); got != 1 {
		t.Fatalf("строк forge_log об отказе = %d, want 1", got)
	}
}

// TestGenerateWithRetryTwoPass — двухпроходный режим ролей прозы (T-12-08):
// сначала сухие факты, затем проза; отказ прозы оставляет факты.
func TestGenerateWithRetryTwoPass(t *testing.T) {
	dir := t.TempDir()
	litPath := filepath.Join(dir, "literary.db")
	newDB(t, litPath, "literary")
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer lit.Close()

	const facts = "сухие факты: герой идёт по дороге."
	slot := &ladderSlot{answers: []string{
		facts,
		"не могу помочь с этим запросом.",
		"не могу помочь с этим запросом.",
		"не могу помочь с этим запросом.",
	}}
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	out, err := workshop.GenerateWithRetry(context.Background(), lit, workshop.GenerateOptions{
		Role:     "prose-role",
		Lang:     "ru",
		Provider: workshop.DefaultProvider,
		TwoPass:  true,
	}, func(pass int, factText string) string {
		if pass == 1 {
			return `{"task":"prose-role","pass":"facts"}`
		}
		return `{"task":"prose-role","pass":"prose","facts":"` + factText + `"}`
	})
	if err != nil {
		t.Fatalf("GenerateWithRetry two-pass: %v", err)
	}
	if out != facts {
		t.Fatalf("выход = %q, want факты (проза отказала)", out)
	}
	if slot.calls != 4 {
		t.Fatalf("вызовов слота = %d, want 4 (факты + 3 ступени прозы)", slot.calls)
	}
}

func countRefusalLog(t *testing.T, lit *db.Handle, status string) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM forge_log WHERE kind = 'censor' AND task = 'refusal' AND status = ?`, status).Scan(&n); err != nil {
		t.Fatalf("COUNT forge_log refusal: %v", err)
	}
	return n
}
