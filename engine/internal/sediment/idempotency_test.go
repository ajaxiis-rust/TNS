package sediment_test

import (
	"context"
	"testing"

	"tns65/internal/db"
	"tns65/internal/sediment"
	"tns65/internal/testkit"
)

// TestIdempotent — повторный прогон одной сцены не создаёт вторую запись
// (T-04-10, «Проверка»): одна строка, второй прогон без вызовов слота и
// эмбеддера, в журнале sediment.dup_skip.
func TestIdempotent(t *testing.T) {
	logger, logPath := newTestLogger(t)
	h := openLiterary(t, t.TempDir(), db.WriterSediment, logger)

	slot := testkit.NewFakeSlot()
	slot.SetResponse("autocritic", `{"verdict":"clean"}`)
	embed := testkit.NewFakeEmbed()
	cen := &fakeCensor{ok: true}

	p := sediment.NewPipeline(embed, slot, cen, logger)

	scene := sediment.Scene{
		ID:     42,
		Status: sediment.SceneStatusClosed,
		Prose:  "Иван купил меч.",
	}
	bindings := []sediment.Binding{{Token: "[NPCname]", Value: "Иван"}}

	first, err := p.Run(context.Background(), h, scene, bindings)
	if err != nil {
		t.Fatalf("первый прогон: %v", err)
	}

	second, err := p.Run(context.Background(), h, scene, bindings)
	if err != nil {
		t.Fatalf("второй прогон: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("второй прогон вернул id=%d, want %d (вернуть существующую запись)", second.ID, first.ID)
	}

	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM sediment_scene`).Scan(&n); err != nil {
		t.Fatalf("COUNT(sediment_scene): %v", err)
	}
	if n != 1 {
		t.Errorf("строк sediment_scene = %d, want 1", n)
	}

	if slot.Calls() != 1 {
		t.Errorf("вызовов слота = %d, want 1 (второй прогон не зовёт модель)", slot.Calls())
	}
	if embed.Calls() != 1 {
		t.Errorf("вызовов эмбеддера = %d, want 1 (второй прогон не векторизует)", embed.Calls())
	}

	recs := journalRecords(t, logPath)
	if !hasEvent(recs, sediment.EventDupSkip) {
		t.Errorf("в журнале нет события %s", sediment.EventDupSkip)
	}
}
