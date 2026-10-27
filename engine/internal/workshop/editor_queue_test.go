package workshop_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// roleCountingSlot — слот поверх FakeSlot, считающий вызовы по ролям. Число
// вызовов EditorRole и есть «счётчик вызовов Редактора»: одна глава — один запуск.
type roleCountingSlot struct {
	inner *testkit.FakeSlot
	mu    sync.Mutex
	roles map[string]int
}

func (s *roleCountingSlot) Generate(ctx context.Context, role, prompt string) (string, error) {
	s.mu.Lock()
	s.roles[role]++
	s.mu.Unlock()
	return s.inner.Generate(ctx, role, prompt)
}

func (s *roleCountingSlot) roleCalls(role string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.roles[role]
}

// TestEditorQueue — «Проверка» T-05-09: после закрытия главы в очереди появилась
// задача edit на главу; разбор очереди до исполнения выполнен без вызова модели
// (счётчик 0); после исполнения счётчик вызовов Редактора 1 и задача в done.
func TestEditorQueue(t *testing.T) {
	workshop.SetLemmatizer(fakeEditLemmatizer{forms: editorLexicon()})
	defer workshop.SetLemmatizer(nil)

	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	newDB(t, worldPath, "world", "shared")
	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()

	// Закрытая глава с собранной версией текста.
	insertEditorChapter(t, world, 1)
	insertEditorText(t, world, 1, 1, "по этому городу")

	// Шаг 1: по закрытию главы в очереди появилась задача edit на главу.
	if _, err := workshop.EnqueueEditorTask(world, 1); err != nil {
		t.Fatalf("EnqueueEditorTask: %v", err)
	}

	var kind, status, rawPayload string
	if err := world.QueryRow(`SELECT kind, status, payload_json FROM workshop_queue ORDER BY id`).Scan(&kind, &status, &rawPayload); err != nil {
		t.Fatalf("SELECT workshop_queue: %v", err)
	}
	if kind != string(workshop.KindEdit) {
		t.Fatalf("kind = %q, want %q", kind, workshop.KindEdit)
	}
	if status != string(workshop.StatusQueued) {
		t.Fatalf("status после постановки = %q, want queued", status)
	}
	var payload workshop.EditorQueuePayload
	if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
		t.Fatalf("payload заказа: %v", err)
	}
	if payload.ChapterID != 1 {
		t.Fatalf("payload.ChapterID = %d, want 1", payload.ChapterID)
	}

	slot := testkit.NewFakeSlot()
	counter := &roleCountingSlot{inner: slot, roles: map[string]int{}}

	// Шаг 2: разбор очереди до исполнения — dequeue и перевод в running без
	// вызова модели.
	task, err := workshop.DequeueEditorTask(world)
	if err != nil {
		t.Fatalf("DequeueEditorTask: %v", err)
	}
	if task == nil {
		t.Fatal("DequeueEditorTask вернул nil при стоящей задаче")
	}
	if task.ChapterID != 1 {
		t.Fatalf("ChapterID = %d, want 1", task.ChapterID)
	}
	if got := slot.Calls(); got != 0 {
		t.Fatalf("вызовов слота после разбора очереди = %d, want 0", got)
	}
	if err := world.QueryRow(`SELECT status FROM workshop_queue WHERE id = ?`, task.ID).Scan(&status); err != nil {
		t.Fatalf("SELECT status: %v", err)
	}
	if status != string(workshop.StatusRunning) {
		t.Fatalf("status после разбора = %q, want running", status)
	}

	// Шаг 3: разобранная задача запускает Редактора пакетно по главе.
	slot.SetResponse(workshop.EditorRole, `{"edits":[{"before":"по этому","after":"поэтому"}]}`)
	slot.SetResponse(workshop.EditorDigestRole, `{"summary":"кратко."}`)

	if _, err := workshop.RunEditorTask(context.Background(), world, counter, task); err != nil {
		t.Fatalf("RunEditorTask: %v", err)
	}

	if got := counter.roleCalls(workshop.EditorRole); got != 1 {
		t.Fatalf("вызовов Редактора = %d, want 1", got)
	}
	if err := world.QueryRow(`SELECT status FROM workshop_queue WHERE id = ?`, task.ID).Scan(&status); err != nil {
		t.Fatalf("SELECT status: %v", err)
	}
	if status != string(workshop.StatusDone) {
		t.Fatalf("status после исполнения = %q, want done", status)
	}
}
