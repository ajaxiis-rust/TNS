package workshop_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/sediment"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// TestStyle — проза в ключе игрока (T-05-14, «Проверка»): в промпте генерации
// присутствует блок примеров стиля игрока (не более 8 реплик, ≤ 2000 токенов);
// в живом ходе стилизация под классика не вызвана (счётчик 0); вывод не
// содержит блока морали ни по одному маркеру.
func TestStyle(t *testing.T) {
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	litPath := filepath.Join(dir, "literary.db")

	newDB(t, worldPath, "world", "shared")
	newDB(t, litPath, "literary")

	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer lit.Close()

	// Журнал ходов: 10 реплик игрока. Селектор детерминированный и берёт
	// последние — не более style.sample_count.
	for i := 0; i < 10; i++ {
		insertTurn(t, world, 1, fmt.Sprintf("Реплика игрока номер %d.", i), "ответ мира")
	}

	slot := testkit.NewFakeSlot()
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	cfg := workshop.DefaultStyleConfig()
	if cfg.SampleCount != 8 || cfg.SampleTokenLimit != 2000 {
		t.Fatalf("DefaultStyleConfig = %+v, want sample_count=8, sample_token_limit=2000", cfg)
	}

	t.Run("config style section", func(t *testing.T) {
		cfgPath := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(cfgPath, []byte("style:\n  sample_count: 5\n  sample_token_limit: 900\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		loaded, err := workshop.LoadStyleConfig(cfgPath)
		if err != nil {
			t.Fatalf("LoadStyleConfig: %v", err)
		}
		if loaded.SampleCount != 5 || loaded.SampleTokenLimit != 900 {
			t.Fatalf("LoadStyleConfig = %+v, want sample_count=5, sample_token_limit=900", loaded)
		}
	})

	samples, err := workshop.CollectSamples(world, cfg)
	if err != nil {
		t.Fatalf("CollectSamples: %v", err)
	}
	if len(samples) > cfg.SampleCount {
		t.Fatalf("образцов = %d, want ≤ %d", len(samples), cfg.SampleCount)
	}
	if len(samples) != 8 {
		t.Fatalf("образцов из 10 реплик = %d, want 8 (последние)", len(samples))
	}
	var total int
	for _, s := range samples {
		total += s.Tokens
	}
	if total > cfg.SampleTokenLimit {
		t.Fatalf("токенов образцов = %d, want ≤ %d", total, cfg.SampleTokenLimit)
	}

	// Промпт генерации несёт блок примеров стиля игрока.
	prompt := workshop.StylePrompt("базовый промпт", samples)
	if !strings.Contains(prompt, workshop.StyleBlockHeader) {
		t.Fatalf("промпт без заголовка блока стиля: %q", prompt)
	}
	for _, s := range samples {
		if !strings.Contains(prompt, s.Text) {
			t.Fatalf("промпт без образца %q: %q", s.Text, prompt)
		}
	}

	// В живом ходе стилизация под классика не вызывается — слот молчит.
	if slot.Calls() != 0 {
		t.Fatalf("вызовов слота в живом ходе = %d, want 0", slot.Calls())
	}

	// Токенный лимит: последняя реплика длиной 1500 слов, следующая ещё 1500 —
	// суммарно 3000 > 2000, селектор останавливается на одной свежей.
	limitDir := t.TempDir()
	limitPath := filepath.Join(limitDir, "world(a).db")
	newDB(t, limitPath, "world", "shared")
	limitWorld, err := db.OpenWriterAs(limitPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(limit world): %v", err)
	}
	defer limitWorld.Close()
	insertTurn(t, limitWorld, 1, strings.Repeat("слово ", 1500), "ответ")
	insertTurn(t, limitWorld, 1, strings.Repeat("другое ", 1500), "ответ")

	limitSamples, err := workshop.CollectSamples(limitWorld, cfg)
	if err != nil {
		t.Fatalf("CollectSamples(limit): %v", err)
	}
	limitTotal := 0
	for _, s := range limitSamples {
		limitTotal += s.Tokens
	}
	if limitTotal > cfg.SampleTokenLimit {
		t.Fatalf("токенов образцов при лимите = %d, want ≤ %d", limitTotal, cfg.SampleTokenLimit)
	}
	if len(limitSamples) != 1 {
		t.Fatalf("образцов при лимите = %d, want 1 (свежая реплика)", len(limitSamples))
	}

	// Вывод движка не содержит блока морали ни по одному маркеру.
	if workshop.HasMoralBlock("Герой шёл по дороге, и ветер нёс пыль.") {
		t.Fatal("чистая проза распознана как блок морали")
	}
	for _, marker := range workshop.MoralMarkers {
		if !workshop.HasMoralBlock(marker + ", герой понял главное.") {
			t.Fatalf("маркер %q в конце ответа не распознан как блок морали", marker)
		}
	}

	// Фоновая стилизация под классика слот вызывает и пишет заготовку в bank_pool.
	slot.SetResponse(workshop.StyleRole, "Стилизованная проза под классика.")
	id, err := workshop.StylizeClassic(context.Background(), lit, "черновик главы")
	if err != nil {
		t.Fatalf("StylizeClassic: %v", err)
	}
	if slot.Calls() != 1 {
		t.Fatalf("вызовов слота стилизации = %d, want 1", slot.Calls())
	}
	if id == 0 {
		t.Fatal("StylizeClassic вернул id=0")
	}
	if got := styleTrust(t, lit, id); got != sediment.TrustLevelVerified {
		t.Fatalf("trust_level заготовки = %d, want %d", got, sediment.TrustLevelVerified)
	}
}

func insertTurn(t *testing.T, h *db.Handle, sessionID int64, player, reply string) {
	t.Helper()
	if _, err := h.Exec("turns",
		`INSERT INTO turns (session_id, player_text, reply_text, saw_recommendation, trust_level, latency_ms, degraded) VALUES (?, ?, ?, 0, 2, 0, 0)`,
		sessionID, player, reply); err != nil {
		t.Fatalf("INSERT turns: %v", err)
	}
}

func styleTrust(t *testing.T, lit *db.Handle, id int64) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT trust_level FROM bank_pool WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("SELECT trust_level: %v", err)
	}
	return n
}
