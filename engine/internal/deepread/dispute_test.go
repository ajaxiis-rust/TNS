package deepread_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"tns65/internal/db"
	"tns65/internal/deepread"
	"tns65/internal/journal"
	"tns65/internal/testkit"
)

// TestDisputeBudget — гейт спорных вопросов слота (W-24, приёмка T-14-10):
// бюджет вопросов на книгу, батчи, проверка цитаты подстрокой окна, повторы и
// судьба спора без ответа.
func TestDisputeBudget(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Fatal("корень движка не найден")
	}

	t.Run("конфиг", func(t *testing.T) {
		cfg, err := deepread.LoadDisputeConfig(filepath.Join(root, "config.yaml"))
		if err != nil {
			t.Fatalf("LoadDisputeConfig: %v", err)
		}
		if cfg.Budget != 20 || cfg.Batch != 5 {
			t.Errorf("бюджет/батч = %d/%d, want 20/5", cfg.Budget, cfg.Batch)
		}

		path := filepath.Join(t.TempDir(), "config.yaml")
		data := "deepread:\n  thresholds:\n    confident: 0.6\n  slot_budget: 3\n  slot_batch: 2\n"
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		got, err := deepread.LoadDisputeConfig(path)
		if err != nil {
			t.Fatalf("LoadDisputeConfig(custom): %v", err)
		}
		if got.Budget != 3 || got.Batch != 2 {
			t.Errorf("бюджет/батч = %d/%d, want 3/2", got.Budget, got.Batch)
		}

		bad := filepath.Join(t.TempDir(), "bad.yaml")
		if err := os.WriteFile(bad, []byte("deepread:\n  slot_batch: 0\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := deepread.LoadDisputeConfig(bad); err == nil {
			t.Error("slot_batch 0 должен вернуть ошибку")
		}
	})

	t.Run("бюджет, батч и применение", func(t *testing.T) {
		lit, bookID := newTestLiterary(t)
		texts := []string{
			"Иван вошёл в высокий терем и запер дверь.",
			"Марья долго смотрела на заснеженный лес.",
			"Снейп угрожал ученикам и грозил расправой.",
			"Профессор учил детей и объяснял правило.",
			"Вечером Иван покинул терем.",
		}
		chunks := insertDisputeChunks(t, lit, bookID, texts)
		windows := make(map[int64]string, len(chunks))
		for i, id := range chunks {
			windows[id] = texts[i]
		}
		senior := insertDisputeCharacter(t, lit, bookID, "Снейп", chunks[2])
		junior := insertDisputeCharacter(t, lit, bookID, "Профессор", chunks[3])

		// Пять спорных мест: две полосы uncertain тегов, пограничное слияние,
		// граница сцен и спорный отрезок роли.
		queue, err := deepread.BuildDisputes(deepread.DisputeInput{
			BookID:  bookID,
			Windows: windows,
			TagKeys: []string{"architecture", "nature", "emotion"},
			Tags: []deepread.ChunkTags{
				{ChunkID: chunks[0], Tags: []deepread.ChunkTag{{Key: "architecture", Score: 0.95, Band: deepread.BandUncertain}}},
				{ChunkID: chunks[1], Tags: []deepread.ChunkTag{{Key: "nature", Score: 0.5, Band: deepread.BandUncertain}}},
			},
			Merges: []deepread.DisputedAlias{{
				SeniorID: senior, JuniorID: junior, SeniorName: "Снейп", JuniorName: "Профессор", Cosine: 0.9,
			}},
			Boundaries: []deepread.DisputedBoundary{{
				FromChunk: chunks[1], ToChunk: chunks[2], Signals: []string{"place"},
			}},
			Roles: []deepread.DisputedRole{{
				CharacterID: senior, FromChunk: chunks[3], ToChunk: chunks[3],
				Signals: []string{"угрожал", "учил"}, Window: "Профессор учил детей.",
			}},
		})
		if err != nil {
			t.Fatalf("BuildDisputes: %v", err)
		}
		if len(queue) != 5 {
			t.Fatalf("спорных мест %d, want 5", len(queue))
		}
		wantOrder := []string{
			"category:" + itoa(chunks[0]) + ":architecture",
			"merge:" + itoa(senior) + ":" + itoa(junior),
			"role:" + itoa(senior) + ":" + itoa(chunks[3]) + ":" + itoa(chunks[3]),
			"category:" + itoa(chunks[1]) + ":nature",
			"boundary:" + itoa(chunks[1]) + ":" + itoa(chunks[2]),
		}
		for i, key := range wantOrder {
			if queue[i].Key != key {
				t.Errorf("очередь[%d] = %q, want %q", i, queue[i].Key, key)
			}
		}

		slot := testkit.NewFakeSlot()
		slot.SetResponse(deepread.DisputeSlotRole,
			`[{"value":"architecture","quote":"высокий терем","confidence":0.9},`+
				`{"value":"same","quote":"Профессор","confidence":0.8},`+
				`{"value":"наставник","quote":"учил детей","confidence":0.7}]`)
		logger, counter := newDisputeJournal(t)

		outcomes, err := deepread.ResolveDisputes(queue, deepread.DisputeConfig{Budget: 3, Batch: 5}, slot, logger)
		if err != nil {
			t.Fatalf("ResolveDisputes: %v", err)
		}
		if got := slot.Calls(); got != 1 {
			t.Errorf("вызовов слота %d, want 1: три вопроса уходят одним батчем", got)
		}
		if got := counter.get(); got != 1 {
			t.Errorf("событий deepread.slot %d, want 1", got)
		}

		states := make(map[string]deepread.DisputeState, len(outcomes))
		for _, out := range outcomes {
			states[out.Question.Key] = out.State
		}
		for _, key := range wantOrder[:3] {
			if states[key] != deepread.DisputeAnswered {
				t.Errorf("спор %q: состояние %q, want answered", key, states[key])
			}
		}
		// Споры сверх бюджета остаются uncertain без запроса (шаг 2).
		for _, key := range wantOrder[3:] {
			if states[key] != deepread.DisputeUncertain {
				t.Errorf("спор %q сверх бюджета: состояние %q, want uncertain", key, states[key])
			}
		}

		for _, out := range outcomes {
			if err := deepread.ApplyDecision(lit, out); err != nil {
				t.Fatalf("ApplyDecision(%q): %v", out.Question.Key, err)
			}
		}
		if n := countRoleRows(t, lit, senior); n != 1 {
			t.Errorf("строк book_character_role %d, want 1", n)
		}
		role, confidence, evidence := readRoleRow(t, lit, senior)
		if role != "наставник" || confidence != 0.7 || evidence != "учил детей" {
			t.Errorf("роль/доверие/основание = %q/%v/%q, want наставник/0.7/«учил детей»",
				role, confidence, evidence)
		}
		if aliases := aliasesOf(t, lit, senior); !hasString(aliases, "Профессор") {
			t.Errorf("алиасы старшей карточки = %v, want «Профессор»", aliases)
		}
		if status := characterStatus(t, lit, junior); status != "merged" {
			t.Errorf("статус младшей карточки = %q, want merged", status)
		}

		category := findOutcome(t, outcomes, wantOrder[0])
		tag, ok := category.AppliedTag()
		if !ok || tag.Key != "architecture" || tag.Band != deepread.BandConfident || len(tag.Anchors) != 1 {
			t.Errorf("тег решения категории = %+v (ok=%v), want architecture/confident с якорем", tag, ok)
		}
	})

	t.Run("цитата не подстрока отклонена", func(t *testing.T) {
		lit, bookID := newTestLiterary(t)
		text := "Иван и Ваня вошли в терем."
		chunks := insertDisputeChunks(t, lit, bookID, []string{text})
		senior := insertDisputeCharacter(t, lit, bookID, "Иван", chunks[0])
		junior := insertDisputeCharacter(t, lit, bookID, "Ваня", chunks[0])

		queue, err := deepread.BuildDisputes(deepread.DisputeInput{
			BookID:  bookID,
			Windows: map[int64]string{chunks[0]: text},
			Merges: []deepread.DisputedAlias{{
				SeniorID: senior, JuniorID: junior, SeniorName: "Иван", JuniorName: "Ваня", Cosine: 0.88,
			}},
		})
		if err != nil {
			t.Fatalf("BuildDisputes: %v", err)
		}
		if len(queue) != 1 {
			t.Fatalf("спорных мест %d, want 1", len(queue))
		}

		slot := testkit.NewFakeSlot()
		slot.SetResponse(deepread.DisputeSlotRole, `{"value":"same","quote":"цитата не из окна","confidence":0.9}`)
		logger, counter := newDisputeJournal(t)

		outcomes, err := deepread.ResolveDisputes(queue, deepread.DisputeConfig{Budget: 5, Batch: 5}, slot, logger)
		if err != nil {
			t.Fatalf("ResolveDisputes: %v", err)
		}
		if len(outcomes) != 1 || outcomes[0].State != deepread.DisputeFailed {
			t.Fatalf("состояние спора = %+v, want failed", outcomes)
		}
		// Провал проверки — повтор не более двух раз: всего три вызова и события.
		if got := slot.Calls(); got != 3 {
			t.Errorf("вызовов слота %d, want 3", got)
		}
		if got := counter.get(); got != 3 {
			t.Errorf("событий deepread.slot %d, want 3", got)
		}

		if err := deepread.ApplyDecision(lit, outcomes[0]); err != nil {
			t.Fatalf("ApplyDecision: %v", err)
		}
		if aliases := aliasesOf(t, lit, senior); len(aliases) != 0 {
			t.Errorf("алиасы после отказа = %v, want пусто", aliases)
		}
		if status := characterStatus(t, lit, junior); status != "candidate" {
			t.Errorf("статус младшей карточки = %q, want candidate", status)
		}
	})

	t.Run("пустой ответ оставляет спор пустым", func(t *testing.T) {
		lit, bookID := newTestLiterary(t)
		text := "Иван вошёл в высокий терем."
		chunks := insertDisputeChunks(t, lit, bookID, []string{text})

		queue, err := deepread.BuildDisputes(deepread.DisputeInput{
			BookID:  bookID,
			Windows: map[int64]string{chunks[0]: text},
			TagKeys: []string{"architecture", "nature"},
			Tags: []deepread.ChunkTags{
				{ChunkID: chunks[0], Tags: []deepread.ChunkTag{{Key: "architecture", Score: 0.4, Band: deepread.BandUncertain}}},
			},
		})
		if err != nil {
			t.Fatalf("BuildDisputes: %v", err)
		}

		slot := testkit.NewFakeSlot()
		slot.SetResponse(deepread.DisputeSlotRole, `[{"value":"","quote":"","confidence":0}]`)
		logger, counter := newDisputeJournal(t)

		outcomes, err := deepread.ResolveDisputes(queue, deepread.DisputeConfig{Budget: 5, Batch: 5}, slot, logger)
		if err != nil {
			t.Fatalf("ResolveDisputes: %v", err)
		}
		if len(outcomes) != 1 || outcomes[0].State != deepread.DisputeEmpty {
			t.Fatalf("состояние спора = %+v, want empty", outcomes)
		}
		if got := slot.Calls(); got != 1 {
			t.Errorf("вызовов слота %d, want 1", got)
		}
		if got := counter.get(); got != 1 {
			t.Errorf("событий deepread.slot %d, want 1", got)
		}
		if err := deepread.ApplyDecision(lit, outcomes[0]); err != nil {
			t.Fatalf("ApplyDecision: %v", err)
		}
		if _, ok := outcomes[0].AppliedTag(); ok {
			t.Error("пустой ответ не должен давать тег чанка")
		}
		if n := countChunkTags(t, lit); n != 0 {
			t.Errorf("строк book_chunk_tag %d, want 0", n)
		}
	})

	t.Run("без слота разбор завершается", func(t *testing.T) {
		lit, bookID := newTestLiterary(t)
		text := "Иван вошёл в терем."
		chunks := insertDisputeChunks(t, lit, bookID, []string{text})
		senior := insertDisputeCharacter(t, lit, bookID, "Иван", chunks[0])
		junior := insertDisputeCharacter(t, lit, bookID, "Ваня", chunks[0])

		queue, err := deepread.BuildDisputes(deepread.DisputeInput{
			BookID:  bookID,
			Windows: map[int64]string{chunks[0]: text},
			Merges: []deepread.DisputedAlias{{
				SeniorID: senior, JuniorID: junior, SeniorName: "Иван", JuniorName: "Ваня", Cosine: 0.86,
			}},
		})
		if err != nil {
			t.Fatalf("BuildDisputes: %v", err)
		}
		outcomes, err := deepread.ResolveDisputes(queue, deepread.DisputeConfig{Budget: 5, Batch: 5}, nil, nil)
		if err != nil {
			t.Fatalf("ResolveDisputes без слота: %v", err)
		}
		if len(outcomes) != 1 || outcomes[0].State != deepread.DisputeUncertain {
			t.Errorf("состояние спора без слота = %+v, want uncertain", outcomes)
		}
	})

	t.Run("решение границы склеивает сцены", func(t *testing.T) {
		newSet := func() *deepread.BookScenes {
			return &deepread.BookScenes{
				BookID: 1,
				Places: []deepread.Place{{Key: "palace", Name: "Терем", FirstChunk: 1, LastChunk: 4}},
				Scenes: []deepread.Scene{
					{
						Ord: 0, FromChunk: 1, ToChunk: 2, PlaceKey: "palace",
						Participants: []string{"Иван"}, Tags: []string{deepread.KeyArchitecture},
						Emotion: map[string]float64{"страх": 0.5},
					},
					{
						Ord: 1, FromChunk: 3, ToChunk: 4, PlaceKey: "palace",
						Participants: []string{"Марья"}, Tags: []string{deepread.KeyNature},
						Emotion: map[string]float64{"страх": 0.5},
					},
				},
			}
		}
		decision := func(value string) []deepread.DisputeOutcome {
			return []deepread.DisputeOutcome{{
				Question: deepread.DisputeQuestion{Kind: deepread.DisputeBoundary, FromChunk: 2, ToChunk: 3},
				State:    deepread.DisputeAnswered,
				Value:    value,
			}}
		}

		joined := newSet()
		if err := deepread.ApplyBoundaries(joined, decision(deepread.AnswerBoundaryJoin)); err != nil {
			t.Fatalf("ApplyBoundaries(join): %v", err)
		}
		if len(joined.Scenes) != 1 {
			t.Fatalf("сцен после склейки %d, want 1", len(joined.Scenes))
		}
		scene := joined.Scenes[0]
		if scene.Ord != 0 || scene.FromChunk != 1 || scene.ToChunk != 4 {
			t.Errorf("склеенная сцена = ord %d, чанки %d..%d, want 0, 1..4",
				scene.Ord, scene.FromChunk, scene.ToChunk)
		}
		if !hasString(scene.Participants, "Иван") || !hasString(scene.Participants, "Марья") {
			t.Errorf("участники склеенной сцены = %v, want Иван и Марья", scene.Participants)
		}
		if !hasString(scene.Tags, deepread.KeyArchitecture) || !hasString(scene.Tags, deepread.KeyNature) {
			t.Errorf("теги склеенной сцены = %v, want architecture и nature", scene.Tags)
		}
		if got := scene.Emotion["страх"]; math.Abs(got-0.5) > 1e-9 {
			t.Errorf("доля эмоции = %v, want 0.5", got)
		}

		split := newSet()
		if err := deepread.ApplyBoundaries(split, decision(deepread.AnswerBoundarySplit)); err != nil {
			t.Fatalf("ApplyBoundaries(split): %v", err)
		}
		if len(split.Scenes) != 2 {
			t.Errorf("сцен после разреза %d, want 2", len(split.Scenes))
		}
	})
}

// itoa — id в строку ключа спора.
func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

// findOutcome ищет итог по ключу спора.
func findOutcome(t *testing.T, outcomes []deepread.DisputeOutcome, key string) deepread.DisputeOutcome {
	t.Helper()
	for _, out := range outcomes {
		if out.Question.Key == key {
			return out
		}
	}
	t.Fatalf("итог спора %q не найден", key)
	return deepread.DisputeOutcome{}
}

// hasString проверяет строку в списке.
func hasString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// disputeEvents считает события deepread.slot через наблюдателя журнала.
type disputeEvents struct {
	n int
}

// count — наблюдатель журнала: считает вызовы слота спорных вопросов.
func (e *disputeEvents) count(event string) {
	if event == deepread.EventDeepreadSlot {
		e.n++
	}
}

// get — число событий deepread.slot.
func (e *disputeEvents) get() int {
	return e.n
}

// newDisputeJournal открывает журнал во временном каталоге с наблюдателем.
func newDisputeJournal(t *testing.T) (*journal.Logger, *disputeEvents) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	counter := &disputeEvents{}
	logger.OnEvent(counter.count)
	return logger, counter
}

// insertDisputeChunks вставляет чанки книги по порядку текстов.
func insertDisputeChunks(t *testing.T, lit *db.Handle, bookID int64, texts []string) []int64 {
	t.Helper()
	ids := make([]int64, len(texts))
	for i := range texts {
		res, err := lit.Exec("book_chunk",
			`INSERT INTO book_chunk (book_id, ord, chapter, text_hash) VALUES (?, ?, '1', ?)`,
			bookID, i, "hash-"+itoa(int64(i)))
		if err != nil {
			t.Fatalf("INSERT book_chunk %d: %v", i, err)
		}
		if ids[i], err = res.LastInsertId(); err != nil {
			t.Fatalf("book_chunk LastInsertId: %v", err)
		}
	}
	return ids
}

// insertDisputeCharacter вставляет карточку персонажа со статусом candidate.
func insertDisputeCharacter(t *testing.T, lit *db.Handle, bookID int64, name string, chunkID int64) int64 {
	t.Helper()
	res, err := lit.Exec("book_character",
		`INSERT INTO book_character (book_id, name, aliases, confidence, first_chunk, last_chunk, mentions, status, created_at) VALUES (?, ?, '[]', 0.5, ?, ?, 1, 'candidate', '2026-01-01T00:00:00Z')`,
		bookID, name, chunkID, chunkID)
	if err != nil {
		t.Fatalf("INSERT book_character %q: %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("book_character LastInsertId: %v", err)
	}
	return id
}

// countRoleRows — число строк book_character_role персонажа.
func countRoleRows(t *testing.T, lit *db.Handle, characterID int64) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM book_character_role WHERE character_id = ?`, characterID).Scan(&n); err != nil {
		t.Fatalf("COUNT book_character_role: %v", err)
	}
	return n
}

// readRoleRow читает единственную строку роли персонажа.
func readRoleRow(t *testing.T, lit *db.Handle, characterID int64) (string, float64, string) {
	t.Helper()
	var role, evidence string
	var confidence float64
	if err := lit.QueryRow(`SELECT role, confidence, evidence FROM book_character_role WHERE character_id = ? ORDER BY id DESC LIMIT 1`, characterID).
		Scan(&role, &confidence, &evidence); err != nil {
		t.Fatalf("SELECT book_character_role: %v", err)
	}
	return role, confidence, evidence
}

// aliasesOf читает алиасы карточки персонажа.
func aliasesOf(t *testing.T, lit *db.Handle, characterID int64) []string {
	t.Helper()
	var raw string
	if err := lit.QueryRow(`SELECT aliases FROM book_character WHERE id = ?`, characterID).Scan(&raw); err != nil {
		t.Fatalf("SELECT aliases: %v", err)
	}
	var aliases []string
	if err := json.Unmarshal([]byte(raw), &aliases); err != nil {
		t.Fatalf("алиасы персонажа %d не JSON: %v", characterID, err)
	}
	return aliases
}

// characterStatus читает статус карточки персонажа.
func characterStatus(t *testing.T, lit *db.Handle, characterID int64) string {
	t.Helper()
	var status string
	if err := lit.QueryRow(`SELECT status FROM book_character WHERE id = ?`, characterID).Scan(&status); err != nil {
		t.Fatalf("SELECT status: %v", err)
	}
	return status
}
