package turn

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"tns65/internal/db"
	"tns65/internal/journal"
)

// newTestPipeline создаёт оперативную базу мира и журнал во временном каталоге
// и возвращает конвейер с открытым писателем core.
func newTestPipeline(t *testing.T) (*Pipeline, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "world(alpha).db")

	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(raw, "world", "shared"); err != nil {
		_ = raw.Close()
		t.Fatalf("Migrate: %v", err)
	}
	_ = raw.Close()

	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, logger)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	return &Pipeline{Logger: logger, DB: h}, logPath
}

func TestPipelineDelivers(t *testing.T) {
	p, logPath := newTestPipeline(t)

	out, err := p.Run(context.Background(), TurnInput{
		SessionID:  1,
		PlayerText: "Я иду по дороге к городу.",
		At:         time.Now(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.ReplyText == "" {
		t.Fatalf("reply_text пуст")
	}
	if out.Path == "" {
		t.Fatalf("path не заполнен: %+v", out)
	}

	// Ровно одна строка в turns с заполненным path.
	rows, err := p.DB.Query(`SELECT player_text, reply_text, path FROM turns`)
	if err != nil {
		t.Fatalf("turns query: %v", err)
	}
	var turns []struct{ player, reply, path string }
	for rows.Next() {
		var tr struct{ player, reply, path string }
		if err := rows.Scan(&tr.player, &tr.reply, &tr.path); err != nil {
			t.Fatalf("turns scan: %v", err)
		}
		turns = append(turns, tr)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("turns rows: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("строк в turns = %d, ожидалось 1", len(turns))
	}
	if turns[0].path == "" {
		t.Fatalf("path в turns пуст")
	}
	if turns[0].reply != out.ReplyText {
		t.Fatalf("reply_text в turns = %q, ожидалось %q", turns[0].reply, out.ReplyText)
	}

	// В журнале по событию turn.stage на каждую стадию, в порядке реестра.
	want := stageNames(p)
	got := stageEvents(t, logPath)
	if !slices.Equal(got, want) {
		t.Fatalf("turn.stage = %v, ожидалось %v", got, want)
	}
}

func TestSlashCommands(t *testing.T) {
	p, logPath := newTestPipeline(t)

	commands := []string{"/статус", "/осмотр", "/квесты", "/помощь"}
	for _, cmd := range commands {
		out, err := p.Run(context.Background(), TurnInput{SessionID: 2, PlayerText: cmd, At: time.Now()})
		if err != nil {
			t.Fatalf("Run(%s): %v", cmd, err)
		}
		if out.ReplyText == "" {
			t.Fatalf("команда %s: reply_text пуст", cmd)
		}
		if out.Path == "" {
			t.Fatalf("команда %s: path пуст", cmd)
		}
	}

	// Слеш-команда не отключает прозаический путь: проза после команды проходит.
	out, err := p.Run(context.Background(), TurnInput{SessionID: 2, PlayerText: "Здравствуй, путник.", At: time.Now()})
	if err != nil {
		t.Fatalf("Run(проза после команды): %v", err)
	}
	if out.ReplyText == "" {
		t.Fatalf("проза после команды: reply_text пуст")
	}

	// Ровно одна строка на ход: четыре команды + одна проза.
	var n int
	if err := p.DB.QueryRow(`SELECT COUNT(*) FROM turns`).Scan(&n); err != nil {
		t.Fatalf("COUNT(turns): %v", err)
	}
	if n != len(commands)+1 {
		t.Fatalf("строк в turns = %d, ожидалось %d", n, len(commands)+1)
	}

	// В журнале по событию turn.stage на каждую стадию каждого хода.
	want := (len(commands) + 1) * len(p.stages())
	if got := len(stageEvents(t, logPath)); got != want {
		t.Fatalf("событий turn.stage = %d, ожидалось %d", got, want)
	}
}

// stageNames — имена стадий реестра в порядке их прохождения.
func stageNames(p *Pipeline) []string {
	var names []string
	for _, s := range p.stages() {
		names = append(names, s.Name())
	}
	return names
}

// stageEvents — имена стадий из событий turn.stage журнала, в порядке записи.
func stageEvents(t *testing.T, logPath string) []string {
	t.Helper()
	var names []string
	for _, line := range readJournalLines(t, logPath) {
		var rec struct {
			Event  string          `json:"event"`
			Fields []journal.Field `json:"fields"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка журнала не JSON: %v\n%s", err, line)
		}
		if rec.Event != journal.EventTurnStage {
			continue
		}
		for _, f := range rec.Fields {
			if f.Key == "stage" {
				names = append(names, f.Value)
			}
		}
	}
	return names
}

func readJournalLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan journal: %v", err)
	}
	return lines
}
