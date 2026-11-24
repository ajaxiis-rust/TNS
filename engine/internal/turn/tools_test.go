package turn

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/journal"
)

// TestHeroTools — инструменты героя (T-19-09, «Проверка»): стартовый набор
// выводится из hero_birth до первого хода, событие мира меняет набор, сцена
// получает набор в факторах.
func TestHeroTools(t *testing.T) {
	t.Run("стартовый набор из hero_birth записан до хода", func(t *testing.T) {
		p, path := newToolsPipeline(t)
		seedHeroBirthMeta(t, path, "простолюдин")

		stage := &toolsStage{db: p.DB}
		if err := stage.Run(context.Background(), &TurnState{}); err != nil {
			t.Fatalf("tools: %v", err)
		}
		tools, ok, err := readHeroTools(p.DB)
		if err != nil {
			t.Fatalf("readHeroTools: %v", err)
		}
		if !ok {
			t.Fatal("слой tools не записан")
		}
		names := toolNames(tools.Tools)
		for _, want := range []string{"рабочий нож", "survival", "семья"} {
			if !names[want] {
				t.Fatalf("в стартовом наборе нет %q: %+v", want, tools.Tools)
			}
		}

		// Повторный ход не переписывает стартовый набор.
		if err := stage.Run(context.Background(), &TurnState{}); err != nil {
			t.Fatalf("tools повторно: %v", err)
		}
		again, _, err := readHeroTools(p.DB)
		if err != nil {
			t.Fatalf("readHeroTools повторно: %v", err)
		}
		if len(again.Tools) != len(tools.Tools) {
			t.Fatalf("стартовый набор переписан: %+v → %+v", tools.Tools, again.Tools)
		}
	})

	t.Run("событие мира меняет набор", func(t *testing.T) {
		p, path := newToolsPipeline(t)
		seedHeroBirthMeta(t, path, "аристократ")

		stage := &toolsStage{db: p.DB}
		if err := stage.Run(context.Background(), &TurnState{}); err != nil {
			t.Fatalf("tools: %v", err)
		}

		payload := `{"add":[{"kind":"item","name":"ключ от кабинета"}],` +
			`"remove":[{"kind":"item","name":"родовая печать"}]}`
		if _, err := p.DB.Exec("events",
			`INSERT INTO events (day, kind, payload_json, source) VALUES (0, ?, ?, ?)`,
			toolEventKind, payload, "world"); err != nil {
			t.Fatalf("событие мира: %v", err)
		}

		st := &TurnState{}
		if err := stage.Run(context.Background(), st); err != nil {
			t.Fatalf("tools после события: %v", err)
		}
		if names := toolNames(st.HeroTools.Tools); !names["ключ от кабинета"] {
			t.Fatalf("событие не добавило опору: %+v", st.HeroTools.Tools)
		} else if names["родовая печать"] {
			t.Fatalf("событие не сняло опору: %+v", st.HeroTools.Tools)
		}
		if st.HeroTools.LastEventID == 0 {
			t.Fatal("событие не отмечено номером LastEventID")
		}

		// Повторный ход то же событие не применяет.
		if err := stage.Run(context.Background(), &TurnState{}); err != nil {
			t.Fatalf("tools повторно: %v", err)
		}
		tools, _, err := readHeroTools(p.DB)
		if err != nil {
			t.Fatalf("readHeroTools: %v", err)
		}
		if names := toolNames(tools.Tools); !names["ключ от кабинета"] || names["родовая печать"] {
			t.Fatalf("событие применилось повторно: %+v", tools.Tools)
		}
	})

	t.Run("сцена получает набор в факторах", func(t *testing.T) {
		p, path := newToolsPipeline(t)
		seedHeroBirthMeta(t, path, "воин")

		if err := (&toolsStage{db: p.DB}).Run(context.Background(), &TurnState{}); err != nil {
			t.Fatalf("tools: %v", err)
		}
		frame := (&prepathStage{db: p.DB}).assembleFrame(&TurnState{})
		if len(frame.HeroTools) == 0 {
			t.Fatal("рамка сцены без инструментов героя")
		}
		if names := toolNames(frame.HeroTools); !names["меч"] {
			t.Fatalf("сцена не видит опору героя: %+v", frame.HeroTools)
		}
	})
}

// newToolsPipeline поднимает пустое ядро мира и отдаёт путь базы: тесту нужен
// второй хэндл (forge), чтобы записать hero_birth как рождение.
func newToolsPipeline(t *testing.T) (*Pipeline, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "world(alpha).db")
	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := db.Migrate(raw, "world", "shared"); err != nil {
		_ = raw.Close()
		t.Fatalf("Migrate: %v", err)
	}
	_ = raw.Close()

	logger, err := journal.New(filepath.Join(filepath.Dir(path), "journal.log"))
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}
	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, logger)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return &Pipeline{Logger: logger, DB: h}, path
}

// seedHeroBirthMeta пишет hero_birth в world_meta хэндлом forge — как рождение.
func seedHeroBirthMeta(t *testing.T, path, birth string) {
	t.Helper()
	forge, err := db.OpenWriterAs(path, db.DBWorld, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(forge): %v", err)
	}
	defer forge.Close()
	raw, err := json.Marshal(birth)
	if err != nil {
		t.Fatalf("marshal hero_birth: %v", err)
	}
	if _, err := forge.Exec("world_meta",
		`INSERT INTO world_meta ("key", value) VALUES ('hero_birth', ?)`, string(raw)); err != nil {
		t.Fatalf("world_meta.hero_birth: %v", err)
	}
}

// toolNames — имена опор для проверки набора.
func toolNames(tools []HeroTool) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, tool := range tools {
		out[tool.Name] = true
	}
	return out
}
