package turn

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/structural"
	"tns65/internal/testkit"
)

// compFixture — оперативная база мира и литературная база для тестов композиции.
type compFixture struct {
	worldCore  *db.Handle
	worldForge *db.Handle
	litReader  *db.Handle
	litForge   *db.Handle
}

func newCompositionFixture(t *testing.T) compFixture {
	t.Helper()
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(alpha).db")
	litPath := filepath.Join(dir, "literary.db")

	w, err := db.OpenWriter(worldPath)
	if err != nil {
		t.Fatalf("OpenWriter(world): %v", err)
	}
	if err := db.Migrate(w, "world", "shared"); err != nil {
		_ = w.Close()
		t.Fatalf("Migrate(world): %v", err)
	}
	_ = w.Close()

	worldCore, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world core): %v", err)
	}
	worldForge, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		_ = worldCore.Close()
		t.Fatalf("OpenWriterAs(world forge): %v", err)
	}
	t.Cleanup(func() {
		_ = worldCore.Close()
		_ = worldForge.Close()
	})

	lw, err := db.OpenWriter(litPath)
	if err != nil {
		t.Fatalf("OpenWriter(literary): %v", err)
	}
	if err := db.Migrate(lw, "literary"); err != nil {
		_ = lw.Close()
		t.Fatalf("Migrate(literary): %v", err)
	}
	_ = lw.Close()

	litForge, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary forge): %v", err)
	}
	litReader, err := db.OpenReaderAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		_ = litForge.Close()
		t.Fatalf("OpenReaderAs(literary): %v", err)
	}
	t.Cleanup(func() {
		_ = litForge.Close()
		_ = litReader.Close()
	})

	return compFixture{worldCore: worldCore, worldForge: worldForge, litReader: litReader, litForge: litForge}
}

// insertDrama кладёт в bank_pool запись kind='drama' с заданной композицией.
func (f compFixture) insertDrama(t *testing.T, family, secondary, technique, moral string, inversion bool, trust int) {
	t.Helper()
	inv := 0
	if inversion {
		inv = 1
	}
	var sec, tech, mor any
	if secondary != "" {
		sec = secondary
	}
	if technique != "" {
		tech = technique
	}
	if moral != "" {
		mor = moral
	}
	if _, err := f.litForge.Exec("bank_pool",
		`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at,
		 family_primary, pattern_secondary, technique, moral_stance, inversion_flag)
		 VALUES ('drama', 'текст сцены', 'ru', '', ?, 'forge', '2026-01-01T00:00:00Z', ?, ?, ?, ?, ?)`,
		trust, family, sec, tech, mor, inv); err != nil {
		t.Fatalf("insert drama: %v", err)
	}
}

// setAnchorFamilies пишет anchor_families в world_meta хэндлом forge.
func (f compFixture) setAnchorFamilies(t *testing.T, fams []string) {
	t.Helper()
	raw, err := json.Marshal(fams)
	if err != nil {
		t.Fatalf("marshal anchor_families: %v", err)
	}
	if _, err := f.worldForge.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES ('anchor_families', ?)`, string(raw)); err != nil {
		t.Fatalf("insert anchor_families: %v", err)
	}
}

// seedBookUsage кладёт book-счётчик дрейфа семьи хэндлом core.
func (f compFixture) seedBookUsage(t *testing.T, family string, count int) {
	t.Helper()
	if _, err := f.worldCore.Exec("family_usage",
		`INSERT INTO family_usage (scope, family, count, updated_at) VALUES ('book', ?, ?, '2026-01-01T00:00:00Z')`,
		family, count); err != nil {
		t.Fatalf("seed book usage: %v", err)
	}
}

// newCompositionPipeline собирает конвейер над фикстурой с журналом во временном
// файле и возвращает путь журнала.
func newCompositionPipeline(t *testing.T, f compFixture) (*Pipeline, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	return &Pipeline{Logger: logger, DB: f.worldCore, LitDB: f.litReader, DriftDB: f.worldCore}, logPath
}

// compositionEvent — строка события turn.composition в журнале.
type compositionEvent struct {
	Event  string          `json:"event"`
	Fields []journal.Field `json:"fields"`
}

func readCompositionEvents(t *testing.T, logPath string) []compositionEvent {
	t.Helper()
	var out []compositionEvent
	for _, line := range readJournalLines(t, logPath) {
		var rec compositionEvent
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка журнала не JSON: %v\n%s", err, line)
		}
		if rec.Event == journal.EventTurnComposition {
			out = append(out, rec)
		}
	}
	return out
}

func compositionField(ev compositionEvent, key string) string {
	for _, f := range ev.Fields {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}

func assertUsage(t *testing.T, h *db.Handle, scope, family string, want int) {
	t.Helper()
	usage, err := structural.Usage(h, scope)
	if err != nil {
		t.Fatalf("Usage(%s): %v", scope, err)
	}
	if got := usage[family]; got != want {
		t.Fatalf("family_usage %s %s = %d, want %d", scope, family, got, want)
	}
}

func engineRootForTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("engine root (go.mod) not found")
		}
		dir = parent
	}
}

func loadTestFamilies(t *testing.T) []structural.Family {
	t.Helper()
	fams, err := structural.LoadFamilies(structural.FamiliesPath(engineRootForTest(t)))
	if err != nil {
		t.Fatalf("LoadFamilies: %v", err)
	}
	return fams
}

func newCompositionStageForTest(f compFixture, fams []structural.Family, clock ClockSource) *compositionStage {
	return &compositionStage{lit: f.litReader, world: f.worldCore, fams: fams, clock: clock}
}

// runawayClock — часы, убегающие вперёд на step при каждом чтении времени: имитируют
// выборку, не укладывающуюся в бюджет.
type runawayClock struct {
	mu   sync.Mutex
	now  time.Time
	step time.Duration
}

func (c *runawayClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(c.step)
	return c.now
}

func (c *runawayClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

func TestCompositionStage(t *testing.T) {
	t.Run("а: лучшая по формуле, событие и счётчики", func(t *testing.T) {
		f := newCompositionFixture(t)
		f.insertDrama(t, "quest_journey", "rescue", "procedural_pressure", "redemptive", false, 2)
		f.insertDrama(t, "rescue", "", "", "", false, 1)
		f.insertDrama(t, "loyalty", "", "", "", false, 0)
		f.setAnchorFamilies(t, []string{"quest_journey"})

		p, logPath := newCompositionPipeline(t, f)
		out, err := p.Run(context.Background(), TurnInput{SessionID: 1, PlayerText: "Я иду по дороге.", At: time.Now()})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if out.ReplyText == "" {
			t.Fatalf("reply_text пуст")
		}

		evs := readCompositionEvents(t, logPath)
		if len(evs) != 1 {
			t.Fatalf("событий turn.composition = %d, want 1", len(evs))
		}
		if got := compositionField(evs[0], "family"); got != "quest_journey" {
			t.Fatalf("family = %q, want quest_journey", got)
		}
		if got := compositionField(evs[0], "seq"); got != "1" {
			t.Fatalf("seq = %q, want 1", got)
		}
		score, err := strconv.ParseFloat(compositionField(evs[0], "score"), 64)
		if err != nil {
			t.Fatalf("score не число: %v", err)
		}
		if score <= 0.4 {
			t.Fatalf("score = %.4f, want > 0.4 (золотая формула)", score)
		}

		assertUsage(t, f.worldCore, structural.ScopeChapter, "quest_journey", 1)
		assertUsage(t, f.worldCore, structural.ScopeBook, "quest_journey", 1)
	})

	t.Run("б: 4-й ход выбирает другую семью", func(t *testing.T) {
		f := newCompositionFixture(t)
		f.insertDrama(t, "quest_journey", "rescue", "procedural_pressure", "redemptive", false, 2)
		f.insertDrama(t, "rescue", "", "", "", false, 0)
		f.setAnchorFamilies(t, []string{"quest_journey"})

		p, logPath := newCompositionPipeline(t, f)
		for i := 0; i < 4; i++ {
			if _, err := p.Run(context.Background(), TurnInput{SessionID: 1, PlayerText: "Я иду по дороге.", At: time.Now()}); err != nil {
				t.Fatalf("Run %d: %v", i+1, err)
			}
		}

		evs := readCompositionEvents(t, logPath)
		if len(evs) != 4 {
			t.Fatalf("событий turn.composition = %d, want 4", len(evs))
		}
		for i := 0; i < 3; i++ {
			if got := compositionField(evs[i], "family"); got != "quest_journey" {
				t.Fatalf("ход %d: family = %q, want quest_journey", i+1, got)
			}
		}
		if got := compositionField(evs[3], "family"); got != "rescue" {
			t.Fatalf("ход 4: family = %q, want rescue (дрейф)", got)
		}

		assertUsage(t, f.worldCore, structural.ScopeChapter, "quest_journey", 3)
		assertUsage(t, f.worldCore, structural.ScopeChapter, "rescue", 1)
	})

	t.Run("в: seq%5 — принудительный инвертированный", func(t *testing.T) {
		f := newCompositionFixture(t)
		f.insertDrama(t, "quest_journey", "rescue", "procedural_pressure", "redemptive", false, 2)
		f.insertDrama(t, "rescue", "", "sudden_reversal", "", true, 0)
		f.setAnchorFamilies(t, []string{"quest_journey"})
		f.seedBookUsage(t, "loyalty", 4) // 4 сделанных выбора → следующий №5.

		p, logPath := newCompositionPipeline(t, f)
		if _, err := p.Run(context.Background(), TurnInput{SessionID: 1, PlayerText: "Я иду.", At: time.Now()}); err != nil {
			t.Fatalf("Run: %v", err)
		}

		evs := readCompositionEvents(t, logPath)
		if len(evs) != 1 {
			t.Fatalf("событий turn.composition = %d, want 1", len(evs))
		}
		if got := compositionField(evs[0], "seq"); got != "5" {
			t.Fatalf("seq = %q, want 5", got)
		}
		if got := compositionField(evs[0], "family"); got != "rescue" {
			t.Fatalf("family = %q, want rescue (принудительный инвертированный)", got)
		}
		if got := compositionField(evs[0], "inversion"); got != "true" {
			t.Fatalf("inversion = %q, want true", got)
		}
	})

	t.Run("г: пул без композиции — деградация", func(t *testing.T) {
		f := newCompositionFixture(t)
		// Легаси family-only драма без family_primary: кандидата композиции нет.
		if _, err := f.litForge.Exec("bank_pool",
			`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at)
			 VALUES ('drama', 'старая сцена', 'ru', '', 0, 'forge', '2026-01-01T00:00:00Z')`); err != nil {
			t.Fatalf("insert legacy drama: %v", err)
		}

		p, logPath := newCompositionPipeline(t, f)
		out, err := p.Run(context.Background(), TurnInput{SessionID: 1, PlayerText: "Я иду.", At: time.Now()})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if out.ReplyText == "" {
			t.Fatalf("reply_text пуст (ход должен доставиться)")
		}

		if evs := readCompositionEvents(t, logPath); len(evs) != 0 {
			t.Fatalf("событий turn.composition = %d, want 0", len(evs))
		}
		var n int
		if err := f.worldCore.QueryRow(`SELECT COUNT(*) FROM family_usage`).Scan(&n); err != nil {
			t.Fatalf("COUNT(family_usage): %v", err)
		}
		if n != 0 {
			t.Fatalf("строк family_usage = %d, want 0 (счётчики не тронуты)", n)
		}
	})

	t.Run("д: бюджет ≤ 100 мс на часах testkit", func(t *testing.T) {
		f := newCompositionFixture(t)
		f.insertDrama(t, "quest_journey", "", "", "", false, 2)
		fams := loadTestFamilies(t)

		// Статичные часы: выборка укладывается в бюджет.
		static := testkit.NewClock()
		st := &TurnState{}
		if err := newCompositionStageForTest(f, fams, static).Run(context.Background(), st); err != nil {
			t.Fatalf("Run (static clock): %v", err)
		}
		if st.Composition.FamilyPrimary != "quest_journey" {
			t.Fatalf("Composition.FamilyPrimary = %q, want quest_journey", st.Composition.FamilyPrimary)
		}

		// Часы, убегающие на 200 мс за выборку: бюджет превышен — деградация.
		runaway := &runawayClock{step: 200 * time.Millisecond}
		st2 := &TurnState{}
		if err := newCompositionStageForTest(f, fams, runaway).Run(context.Background(), st2); err != nil {
			t.Fatalf("Run (runaway clock): %v", err)
		}
		if st2.Composition.FamilyPrimary != "" {
			t.Fatalf("Composition.FamilyPrimary = %q, want пусто (бюджет превышен)", st2.Composition.FamilyPrimary)
		}
	})

	t.Run("сцена тегируется слагом семьи, руководство Ткача", func(t *testing.T) {
		st := &TurnState{
			Prepath:     Prepath{Intent: Intent{Name: "речь"}, Mood: "тревога"},
			Composition: structural.Composition{FamilyPrimary: "quest_journey", Technique: "sudden_reversal", MoralStance: "ironic", Inversion: true},
		}

		tags := sceneTags(st)
		if !containsString(tags, "quest_journey") {
			t.Fatalf("sceneTags = %v, want слаг quest_journey", tags)
		}

		got := compositionGuide(st.Composition)
		want := "приём: sudden_reversal; позиция: ironic; слом ожидания: да"
		if got != want {
			t.Fatalf("compositionGuide = %q, want %q", got, want)
		}
		if compositionGuide(structural.Composition{}) != "" {
			t.Fatalf("compositionGuide нулевой композиции = %q, want пусто", compositionGuide(structural.Composition{}))
		}
	})
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
