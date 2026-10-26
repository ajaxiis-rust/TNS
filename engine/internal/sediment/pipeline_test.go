package sediment_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/sediment"
	"tns65/internal/testkit"
)

// journalEvent — одна JSON-строка журнала станка.
type journalEvent struct {
	Event  string          `json:"event"`
	Fields []journal.Field `json:"fields"`
}

// journalRecords читает журнал и возвращает события в порядке записи.
func journalRecords(t *testing.T, path string) []journalEvent {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var out []journalEvent
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var rec journalEvent
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка журнала не JSON: %v\n%s", err, line)
		}
		out = append(out, rec)
	}
	return out
}

// eventNames — имена событий в порядке записи.
func eventNames(recs []journalEvent) []string {
	names := make([]string, len(recs))
	for i, r := range recs {
		names[i] = r.Event
	}
	return names
}

// hasEvent сообщает, есть ли событие event среди записей журнала.
func hasEvent(recs []journalEvent, event string) bool {
	for _, r := range recs {
		if r.Event == event {
			return true
		}
	}
	return false
}

// eventField возвращает значение поля key события event; не найдено — false.
func eventField(recs []journalEvent, event, key string) (string, bool) {
	for _, r := range recs {
		if r.Event != event {
			continue
		}
		for _, f := range r.Fields {
			if f.Key == key {
				return f.Value, true
			}
		}
	}
	return "", false
}

// openLiteraryWithVector открывает литературную базу с таблицей vectors и одним
// засеянным вектором существующей записи. DDL literary не содержит vectors
// (D-19.2), таблица создаётся только в тесте, чтобы конвейер прошёл ветку дубля.
func openLiteraryWithVector(t *testing.T, dir string, logger *journal.Logger, vec []float32) *db.Handle {
	t.Helper()
	path := filepath.Join(dir, "literary.db")

	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(raw, "literary"); err != nil {
		_ = raw.Close()
		t.Fatalf("Migrate(literary): %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE vectors (
		id INTEGER PRIMARY KEY,
		owner_table TEXT NOT NULL,
		owner_id INTEGER NOT NULL,
		model TEXT NOT NULL,
		dim INTEGER NOT NULL CHECK (dim = 1024),
		lang TEXT NOT NULL,
		source_text TEXT NOT NULL,
		vec BLOB NOT NULL,
		created_at TEXT NOT NULL
	)`); err != nil {
		_ = raw.Close()
		t.Fatalf("CREATE TABLE vectors: %v", err)
	}
	blob, err := sediment.VectorBlob(vec)
	if err != nil {
		_ = raw.Close()
		t.Fatalf("VectorBlob: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES ('sediment_scene', 1, 'fake', 1024, 'ru', '', ?, '2026-01-01T00:00:00Z')`, blob); err != nil {
		_ = raw.Close()
		t.Fatalf("INSERT vectors: %v", err)
	}
	_ = raw.Close()

	h, err := db.OpenWriterAs(path, db.DBLiterary, db.WriterSediment, logger)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// loadGoldenScene загружает входную сцену набора T-04-08 и эталон очистки.
func loadGoldenScene(t *testing.T) (sediment.Scene, []sediment.Binding, string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(goldenDir, "scene_in_*.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("scene_in_*.json: найдено %d файлов, want ровно 1", len(files))
	}
	in := loadJSON[goldenInput](t, files[0])
	scene := sediment.Scene{ID: in.ID, Status: in.Status, Prose: in.Prose, Facts: in.Facts}
	cleanPath := strings.Replace(files[0], "scene_in_", "scene_clean_", 1)
	wantClean := loadJSON[goldenClean](t, cleanPath)
	return scene, in.Bindings, wantClean.CleanedText
}

// newPipeline собирает конвейер с фейковым слотом и пропускающим Цензором.
func newPipeline(logger *journal.Logger) *sediment.Pipeline {
	return sediment.NewPipeline(
		testkit.NewFakeEmbed(),
		&fakeAutocriticSlot{responses: []string{`{"verdict":"clean"}`}},
		&fakeCensor{ok: true},
		logger,
	)
}

func TestGoldenPipeline(t *testing.T) {
	t.Run("успех: пять событий и write с trust_level=1", func(t *testing.T) {
		logger, logPath := newTestLogger(t)
		h := openLiterary(t, t.TempDir(), db.WriterSediment, logger)
		p := newPipeline(logger)
		scene, bindings, wantClean := loadGoldenScene(t)

		ws, err := p.Run(context.Background(), h, scene, bindings)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if ws.TrustLevel != sediment.TrustLevelUnchecked {
			t.Errorf("TrustLevel = %d, want %d", ws.TrustLevel, sediment.TrustLevelUnchecked)
		}

		recs := journalRecords(t, logPath)
		want := []string{
			sediment.EventIn,
			sediment.EventClean,
			sediment.EventAutocriticVerdict,
			sediment.EventDedup,
			sediment.EventWrite,
		}
		if got := eventNames(recs); !equalStrings(got, want) {
			t.Fatalf("цепочка событий не совпала:\n got: %v\nwant: %v", got, want)
		}
		if v, ok := eventField(recs, sediment.EventWrite, "trust_level"); !ok || v != "1" {
			t.Errorf("событие %s не несёт trust_level=1: value=%q ok=%v", sediment.EventWrite, v, ok)
		}

		var n int
		if err := h.QueryRow(`SELECT COUNT(*) FROM sediment_scene`).Scan(&n); err != nil {
			t.Fatalf("COUNT(sediment_scene): %v", err)
		}
		if n != 1 {
			t.Errorf("строк sediment_scene = %d, want 1", n)
		}
		var cleaned string
		if err := h.QueryRow(`SELECT cleaned_text FROM sediment_scene`).Scan(&cleaned); err != nil {
			t.Fatalf("SELECT cleaned_text: %v", err)
		}
		if cleaned != wantClean {
			t.Errorf("cleaned_text не совпал с эталоном набора:\n got: %q\nwant: %q", cleaned, wantClean)
		}
	})

	t.Run("дубль: dup_skip без write", func(t *testing.T) {
		logger, logPath := newTestLogger(t)
		embed := testkit.NewFakeEmbed()
		scene, bindings, wantClean := loadGoldenScene(t)

		vec, err := embed.Embed(context.Background(), wantClean)
		if err != nil {
			t.Fatalf("Embed: %v", err)
		}
		h := openLiteraryWithVector(t, t.TempDir(), logger, vec)
		p := sediment.NewPipeline(embed, &fakeAutocriticSlot{responses: []string{`{"verdict":"clean"}`}}, &fakeCensor{ok: true}, logger)

		if _, err := p.Run(context.Background(), h, scene, bindings); !errors.Is(err, sediment.ErrDuplicate) {
			t.Fatalf("Run(дубль) = %v, want ErrDuplicate", err)
		}

		recs := journalRecords(t, logPath)
		if !hasEvent(recs, sediment.EventDupSkip) {
			t.Errorf("в журнале нет события %s", sediment.EventDupSkip)
		}
		if hasEvent(recs, sediment.EventWrite) {
			t.Errorf("при дубле не должно быть события %s", sediment.EventWrite)
		}

		var n int
		if err := h.QueryRow(`SELECT COUNT(*) FROM sediment_scene`).Scan(&n); err != nil {
			t.Fatalf("COUNT(sediment_scene): %v", err)
		}
		if n != 0 {
			t.Errorf("строк sediment_scene = %d, want 0 (дубль не пишется)", n)
		}
	})

	t.Run("набор: число записей равно эталону (3)", func(t *testing.T) {
		logger, _ := newTestLogger(t)
		h := openLiterary(t, t.TempDir(), db.WriterSediment, logger)
		p := newPipeline(logger)
		scene, bindings, wantClean := loadGoldenScene(t)

		// Эталон набора T-04-09 — 3 записи sediment_scene (план, «Проверка»).
		const wantRecords = 3
		for i := 0; i < wantRecords; i++ {
			// Три разные сцены одного смысла: после T-04-10 повтор одной сцены
			// (source_scene_id) не создаёт вторую запись.
			scene.ID = int64(42 + i)
			if _, err := p.Run(context.Background(), h, scene, bindings); err != nil {
				t.Fatalf("Run(%d): %v", i, err)
			}
		}

		var n int
		if err := h.QueryRow(`SELECT COUNT(*) FROM sediment_scene`).Scan(&n); err != nil {
			t.Fatalf("COUNT(sediment_scene): %v", err)
		}
		if n != wantRecords {
			t.Errorf("строк sediment_scene = %d, want %d (эталон набора)", n, wantRecords)
		}

		rows, err := h.Query(`SELECT cleaned_text FROM sediment_scene`)
		if err != nil {
			t.Fatalf("SELECT cleaned_text: %v", err)
		}
		defer rows.Close()
		var i int
		for rows.Next() {
			var cleaned string
			if err := rows.Scan(&cleaned); err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if cleaned != wantClean {
				t.Errorf("запись %d: cleaned_text не совпал с эталоном набора", i)
			}
			i++
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows.Err: %v", err)
		}
	})
}
