package workshop_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/llmparams"
	"tns65/internal/models"
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

// paramsSlot — слот с параметрами генерации (П1): помнит params каждого вызова,
// ответ берёт из встроенного счётчика ladderSlot.
type paramsSlot struct {
	ladderSlot
	params []models.GenerateParams
}

func (s *paramsSlot) GenerateParams(ctx context.Context, role, prompt string, params models.GenerateParams) (string, error) {
	s.params = append(s.params, params)
	return s.Generate(ctx, role, prompt)
}

// TestGenerateWithRetryForwardsParams — параметры из GenerateOptions доходят до
// слота, умеющего их принимать (П2); слот без GenerateParams работает как раньше.
func TestGenerateWithRetryForwardsParams(t *testing.T) {
	dir := t.TempDir()
	litPath := filepath.Join(dir, "literary.db")
	newDB(t, litPath, "literary")
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer lit.Close()

	temp, maxTokens := 0.45, 700
	slot := &paramsSlot{ladderSlot: ladderSlot{answers: []string{"готовая проза."}}}
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	out, err := workshop.GenerateWithRetry(context.Background(), lit, workshop.GenerateOptions{
		Role:     "test-params",
		Lang:     "ru",
		Provider: workshop.DefaultProvider,
		Params:   models.GenerateParams{Temperature: &temp, MaxTokens: &maxTokens},
	}, func(_ int, _ string) string {
		return `{"task":"test-params"}`
	})
	if err != nil {
		t.Fatalf("GenerateWithRetry: %v", err)
	}
	if out != "готовая проза." {
		t.Fatalf("выход = %q, want %q", out, "готовая проза.")
	}
	if len(slot.params) != 1 {
		t.Fatalf("вызовов с параметрами = %d, want 1", len(slot.params))
	}
	got := slot.params[0]
	if got.Temperature == nil || *got.Temperature != temp || got.MaxTokens == nil || *got.MaxTokens != maxTokens {
		t.Fatalf("params слота = %+v, ожидались temperature=%v max_tokens=%d", got, temp, maxTokens)
	}
}

// TestGenerateWithRetryParamsFromConfig — П3: params без явного GenerateOptions
// собираются из llm: config.yaml и состава роли (world_meta) и доходят до слота;
// температура роли главнее станковой.
func TestGenerateWithRetryParamsFromConfig(t *testing.T) {
	dir := t.TempDir()
	litPath := filepath.Join(dir, "literary.db")
	newDB(t, litPath, "literary")
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer lit.Close()

	llmparams.SetConfig(llmparams.ParseSettings("llm:\n" +
		"  gen_max_tokens: 300\n  gen_temperature: 0.25\n  gen_top_p: 0.8\n" +
		"  gen_top_k: 32\n  gen_reasoning: 0\n"))
	llmparams.SetRoleConfigs(map[string]llmparams.RoleConfig{"config-params": {Temperature: 0.65}})
	t.Cleanup(func() {
		llmparams.SetConfig(llmparams.Config{})
		llmparams.SetRoleConfigs(nil)
	})

	slot := &paramsSlot{ladderSlot: ladderSlot{answers: []string{"готовая проза."}}}
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	if _, err := workshop.GenerateWithRetry(context.Background(), lit, workshop.GenerateOptions{
		Role:     "config-params",
		Lang:     "ru",
		Provider: workshop.DefaultProvider,
	}, func(_ int, _ string) string { return `{"task":"config-params"}` }); err != nil {
		t.Fatalf("GenerateWithRetry: %v", err)
	}
	if len(slot.params) != 1 {
		t.Fatalf("вызовов с параметрами = %d, want 1", len(slot.params))
	}
	got := slot.params[0]
	if got.Temperature == nil || *got.Temperature != 0.65 {
		t.Fatalf("temperature = %v, want 0.65 (роль)", got.Temperature)
	}
	if got.MaxTokens == nil || *got.MaxTokens != 300 ||
		got.TopP == nil || *got.TopP != 0.8 ||
		got.TopK == nil || *got.TopK != 32 ||
		got.Reasoning == nil || *got.Reasoning != 0 {
		t.Fatalf("params из config.yaml = %+v, ожидались max_tokens=300 top_p=0.8 top_k=32 reasoning=0", got)
	}
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

// newCastLit открывает литературную базу для теста спорного случая роли.
func newCastLit(t *testing.T) *db.Handle {
	t.Helper()
	dir := t.TempDir()
	litPath := filepath.Join(dir, "literary.db")
	newDB(t, litPath, "literary")
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	return lit
}

// TestResolveCastRole — спорный случай роли (P12 §5.9 п.5, T-12-24): слот
// отвечает одной ролью из спорного списка; промпт несёт героя и роли.
func TestResolveCastRole(t *testing.T) {
	lit := newCastLit(t)
	defer lit.Close()

	slot := &ladderSlot{answers: []string{"союзник"}}
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	role, err := workshop.ResolveCastRole(context.Background(), lit, "Снейп",
		"Снейп угрожал и спас Гарри.", []string{"антагонист", "союзник"})
	if err != nil {
		t.Fatalf("ResolveCastRole: %v", err)
	}
	if role != "союзник" {
		t.Fatalf("роль = %q, want «союзник»", role)
	}
	if slot.calls != 1 {
		t.Fatalf("вызовов слота = %d, want 1", slot.calls)
	}
	if !strings.Contains(slot.prompts[0], "Снейп") || !strings.Contains(slot.prompts[0], "антагонист") {
		t.Fatalf("промпт без героя/ролей: %q", slot.prompts[0])
	}
}

// TestResolveCastRoleRefusal — отказ слота оставляет маркеры (пустая роль).
func TestResolveCastRoleRefusal(t *testing.T) {
	lit := newCastLit(t)
	defer lit.Close()

	slot := &ladderSlot{answers: []string{"не могу помочь с этим запросом."}}
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	role, err := workshop.ResolveCastRole(context.Background(), lit, "Снейп",
		"Снейп угрожал и спас Гарри.", []string{"антагонист", "союзник"})
	if err != nil {
		t.Fatalf("ResolveCastRole(отказ): %v", err)
	}
	if role != "" {
		t.Fatalf("при отказе роль = %q, want пусто (маркеры остаются)", role)
	}
}

// TestResolveCastRoleNoSlot — без слота спорный случай не зовёт модель.
func TestResolveCastRoleNoSlot(t *testing.T) {
	lit := newCastLit(t)
	defer lit.Close()

	workshop.SetSlot(nil)
	role, err := workshop.ResolveCastRole(context.Background(), lit, "Снейп",
		"Снейп угрожал и спас Гарри.", []string{"антагонист", "союзник"})
	if err != nil {
		t.Fatalf("ResolveCastRole(без слота): %v", err)
	}
	if role != "" {
		t.Fatalf("без слота роль = %q, want пусто", role)
	}
}
