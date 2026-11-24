package turn

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestFinaleMenu — открытый финал (T-19-10, «Проверка»): после завершающей
// стадии мир в состоянии меню; каждый из трёх путей инициируется; эпилог
// связывается с пиками.
func TestFinaleMenu(t *testing.T) {
	t.Run("после завершающей стадии мир в состоянии меню", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		seedHeroHealth(t, p, 2)

		death := &deathStage{db: p.DB}
		finale := &finaleStage{db: p.DB}

		// Ход на пороге смерти выдаёт сцену прощания; меню ещё закрыто.
		st := &TurnState{Input: TurnInput{SessionID: 1, PlayerText: "Прими печать.", At: time.Now()}}
		if err := death.Run(context.Background(), st); err != nil {
			t.Fatalf("death: %v", err)
		}
		if err := finale.Run(context.Background(), st); err != nil {
			t.Fatalf("finale: %v", err)
		}
		if !st.Farewell || st.Finale.Open {
			t.Fatalf("ход прощания: farewell=%v, меню=%v; want true, false", st.Farewell, st.Finale.Open)
		}
		if st.Reply != farewellReply {
			t.Fatalf("reply = %q, want %q", st.Reply, farewellReply)
		}

		// Следующий ход: сцена прощания завершена — мир спрашивает меню.
		st = finaleMenuTurn(t, death, finale)
		if st.Farewell {
			t.Fatal("сцена прощания выдана повторно")
		}
		if !st.Finale.Open {
			t.Fatal("мир не в состоянии меню")
		}
		if st.Reply != finaleMenuReply {
			t.Fatalf("reply = %q, want меню финала %q", st.Reply, finaleMenuReply)
		}

		// Спрашивает однажды и ждёт: пока выбора нет, меню остаётся.
		if st = finaleMenuTurn(t, death, finale); !st.Finale.Open {
			t.Fatal("меню пропало без выбора")
		}
	})

	t.Run("принять финал пишет эпилог и связывает пики", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		seedFinaleOpen(t, p)

		const text = "И жили они долго и счастливо."
		const peak = "Главный перелом — сцена с печатью в таверне."
		menu, err := ChooseFinale(p.DB, FinaleAccept, text, peak, "2026-11-24T10:00:00Z")
		if err != nil {
			t.Fatalf("ChooseFinale: %v", err)
		}
		if menu.Choice != FinaleAccept || menu.Open || !menu.Farewell {
			t.Fatalf("меню = %+v, want выбор accept при закрытом меню", menu)
		}

		var kind string
		if err := p.DB.QueryRow(`SELECT kind FROM book_chapters WHERE id = ?`, menu.ChapterID).Scan(&kind); err != nil {
			t.Fatalf("SELECT book_chapters: %v", err)
		}
		if kind != "epilogue" {
			t.Fatalf("вид главы = %q, want epilogue", kind)
		}

		// Эпилог связан с пиками: метка ссылается на главу эпилога (CB-13.2).
		var chapterID int64
		var peakKind, note string
		if err := p.DB.QueryRow(
			`SELECT chapter_id, kind, note FROM book_peaks WHERE chapter_id = ?`, menu.ChapterID).
			Scan(&chapterID, &peakKind, &note); err != nil {
			t.Fatalf("SELECT book_peaks: %v", err)
		}
		if chapterID != menu.ChapterID || peakKind != "chapter" || note != peak {
			t.Fatalf("пик = (%d, %q, %q), want (%d, chapter, %q)",
				chapterID, peakKind, note, menu.ChapterID, peak)
		}

		// Финал отвечен: стадия финала больше не спрашивает.
		st := &TurnState{
			Input: TurnInput{SessionID: 1, PlayerText: "Дальше", At: time.Now()},
			Reply: "ответ мира",
		}
		if err := (&finaleStage{db: p.DB}).Run(context.Background(), st); err != nil {
			t.Fatalf("finale: %v", err)
		}
		if st.Finale.Open || st.Reply != "ответ мира" {
			t.Fatalf("после выбора мир спрашивает снова: open=%v reply=%q", st.Finale.Open, st.Reply)
		}
		// Выбор делается один раз.
		if _, err := ChooseFinale(p.DB, FinaleReplay, "", "", ""); err == nil {
			t.Fatal("повторный выбор финала принят")
		}
	})

	t.Run("переигровка оставляет прошлого героя в истории мира", func(t *testing.T) {
		f := newCompositionFixture(t)
		p := &Pipeline{DB: f.worldCore}
		seedFinaleOpen(t, p)
		// hero_birth пишет рождение мира хэндлом forge.
		if _, err := f.worldForge.Exec("world_meta",
			`INSERT INTO world_meta ("key", value) VALUES ('hero_birth', ?)`, `"Иосиф"`); err != nil {
			t.Fatalf("insert hero_birth: %v", err)
		}

		menu, err := ChooseFinale(p.DB, FinaleReplay, "", "", "")
		if err != nil {
			t.Fatalf("ChooseFinale: %v", err)
		}
		if menu.Choice != FinaleReplay || menu.Open {
			t.Fatalf("меню = %+v, want выбор replay", menu)
		}

		var kind, payload, source string
		if err := p.DB.QueryRow(`SELECT kind, payload_json, source FROM events`).
			Scan(&kind, &payload, &source); err != nil {
			t.Fatalf("SELECT events: %v", err)
		}
		if kind != finalePastHeroKind || source != finaleSource {
			t.Fatalf("событие = (%q, %q), want (%q, %q)", kind, source, finalePastHeroKind, finaleSource)
		}
		var got struct {
			Reason       string `json:"reason"`
			PreviousHero string `json:"previous_hero"`
		}
		if err := json.Unmarshal([]byte(payload), &got); err != nil {
			t.Fatalf("разбор payload: %v", err)
		}
		if got.Reason != finalePastHeroReason || got.PreviousHero != "Иосиф" {
			t.Fatalf("payload = %+v, want причину смерти героя «Иосиф»", got)
		}
		// Эпилог не пишется: книга остаётся прежней.
		assertNoEpilogue(t, p)
	})

	t.Run("новый мир ждёт анкету без эпилога и следа героя", func(t *testing.T) {
		p, _ := newTestPipeline(t)
		seedFinaleOpen(t, p)

		menu, err := ChooseFinale(p.DB, FinaleNewWorld, "", "", "")
		if err != nil {
			t.Fatalf("ChooseFinale: %v", err)
		}
		if menu.Choice != FinaleNewWorld || menu.Open {
			t.Fatalf("меню = %+v, want выбор new_world", menu)
		}
		assertNoEpilogue(t, p)

		var events int
		if err := p.DB.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&events); err != nil {
			t.Fatalf("COUNT events: %v", err)
		}
		if events != 0 {
			t.Fatalf("след прошлого героя при новом мире: событий %d, want 0", events)
		}

		// Путь героя закрыт: мир больше не спрашивает.
		st := &TurnState{
			Input: TurnInput{SessionID: 1, PlayerText: "Анкета", At: time.Now()},
			Reply: "ответ мира",
		}
		if err := (&finaleStage{db: p.DB}).Run(context.Background(), st); err != nil {
			t.Fatalf("finale: %v", err)
		}
		if st.Finale.Open {
			t.Fatal("новый мир снова спрашивает финал")
		}
	})
}

// finaleMenuTurn прогоняет завершающие стадии хода после сцены прощания.
func finaleMenuTurn(t *testing.T, death *deathStage, finale *finaleStage) *TurnState {
	t.Helper()
	st := &TurnState{Input: TurnInput{SessionID: 1, PlayerText: "Я слушаю.", At: time.Now()}}
	if err := death.Run(context.Background(), st); err != nil {
		t.Fatalf("death: %v", err)
	}
	if err := finale.Run(context.Background(), st); err != nil {
		t.Fatalf("finale: %v", err)
	}
	return st
}

// seedFinaleOpen сеет состояние «сцена прощания выдана»: финал ждёт выбора.
func seedFinaleOpen(t *testing.T, p *Pipeline) {
	t.Helper()
	if err := saveFinale(p.DB, FinaleMenu{Farewell: true}); err != nil {
		t.Fatalf("saveFinale: %v", err)
	}
}

// assertNoEpilogue — эпилога книги нет.
func assertNoEpilogue(t *testing.T, p *Pipeline) {
	t.Helper()
	var n int
	if err := p.DB.QueryRow(`SELECT COUNT(*) FROM book_chapters WHERE kind = 'epilogue'`).Scan(&n); err != nil {
		t.Fatalf("COUNT epilogue: %v", err)
	}
	if n != 0 {
		t.Fatalf("глав-эпилогов = %d, want 0", n)
	}
}
