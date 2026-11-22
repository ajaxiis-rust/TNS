//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"tns65/internal/book"
	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/lemmarpc"
	"tns65/internal/sediment"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

// int12CloseReplica — явная реплика закрытия главы (сон): лексикон намерений
// даёт намерение «сон» по лемме «спать» (INT-10.2, CB-03.4).
const int12CloseReplica = "Я ложусь в постель и хочу лечь спать."

// int12SceneProse — проза сцены-кандидата: имя героя вырезается очисткой в слот
// {NAME} (W-03.2).
const int12SceneProse = "Иван стоит у ворот и ждёт."

// int12PythiaOutput — строгий JSON ночного прогона Пифии (W-12): модель
// возвращает свой психотип, но в карту он не попадает (W-12.4).
const int12PythiaOutput = `{"mood":"спокойное","interests":["море","дом"],` +
	`"habits":["прогулки"],"avoid":["толпа"],"psychotype":"INTP"}`

// int12StyleText — стилизованная заготовка ночного пополнения банка (W-21.4).
const int12StyleText = "Ночь лежит на черепичных крышах, и город дышит ровно."

// int12Slot — слот ночного прогона: отвечает по роли и запоминает порядок ролей.
type int12Slot struct {
	roles []string
}

func (s *int12Slot) Generate(_ context.Context, role, _ string) (string, error) {
	s.roles = append(s.roles, role)
	switch role {
	case "autocritic":
		return `{"verdict":"clean"}`, nil
	case "pythia":
		return int12PythiaOutput, nil
	case "style":
		return int12StyleText, nil
	default:
		return "Ночная заготовка готова.", nil
	}
}

// int12Censor — словарный фильтр, пропускающий любой текст (Цензор T-04-11 в
// тестах подменяется).
type int12Censor struct{}

func (int12Censor) Check(string) (bool, []string) { return true, nil }

// int12Lemmatizer — подставной лемматизатор намерения закрытия главы: слова
// сводятся к нижнему регистру, пунктуация отбрасывается.
type int12Lemmatizer struct{}

func (int12Lemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error) {
	var out []lemmarpc.Token
	for _, w := range strings.Fields(text) {
		lemma := strings.Map(func(r rune) rune {
			if unicode.IsPunct(r) {
				return -1
			}
			return unicode.ToLower(r)
		}, w)
		if lemma == "" {
			continue
		}
		out = append(out, lemmarpc.Token{Form: w, Lemma: lemma})
	}
	return out, nil
}

// int12SedimentOrder — заказ оседания: id строки очереди и turn_id сцены.
type int12SedimentOrder struct {
	ID     int64
	TurnID int64
}

// TestINT12 — INT-12. Ночь и мастерская (T-17-05, атом INT-10): глава, закрытая
// явной репликой, после молчания больше 8 часов даёт ступень «ночь»; в ночном
// окне очередь исполняет заготовки и оседание (очистка, дедуп, авто-критик),
// банк пополняется, а ночной прогон Пифии пишет карту игрока (психотип, привычки).
func TestINT12(t *testing.T) {
	bin := buildBinary(t, []string{"e2e"})
	dir := worldDir(t)
	linkConfig(t, dir)
	linkPacks(t, dir)

	// Подставной слот Ткача: живой ход получает прозу без внешней модели.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(int12SceneProse))
	}))
	defer srv.Close()

	slug := "noch-i-masterskaya"
	ancketa := writeAncketa(t, t.TempDir(), slug,
		"Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.")
	stdout, stderr, code := runCmd(t, dir, bin, "world", "birth", "--ancketa", ancketa, "--fake-embed", "--fake-slot")
	if code != 0 {
		t.Fatalf("world birth exit=%d, want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	worldDB := worldDBPath(dir, slug)
	literaryDB := filepath.Join(dir, "world_db", "worlds", slug, "literary.db")

	// Шаг 1: мир с открытой главой; глава закрывается явной репликой (сон).
	birthDB, err := db.OpenWriterAs(worldDB, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("открыть базу мира после рождения: %v", err)
	}
	if _, err := book.OpenChapter(birthDB, 1, time.Now().UTC().Format(time.RFC3339)); err != nil {
		birthDB.Close()
		t.Fatalf("OpenChapter: %v", err)
	}
	if err := birthDB.Close(); err != nil {
		t.Fatalf("закрыть базу мира после рождения: %v", err)
	}

	c := startCore(t, dir, bin, "--fake-clock", "--fake-slot", srv.URL)
	c.postTurn(t, slug, "Я иду, еду, плыву и бегу.")
	c.postTurn(t, slug, int12CloseReplica)
	c.stop()

	h, err := db.OpenWriterAs(worldDB, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("открыть базу мира: %v", err)
	}
	defer h.Close()
	litSed, err := db.OpenWriterAs(literaryDB, db.DBLiterary, db.WriterSediment, nil)
	if err != nil {
		t.Fatalf("открыть литературную базу (sediment): %v", err)
	}
	defer litSed.Close()
	litForge, err := db.OpenWriterAs(literaryDB, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("открыть литературную базу (forge): %v", err)
	}
	defer litForge.Close()

	report := map[string]any{}

	lex, err := book.LoadLexicon(filepath.Join(engineRoot(t), "packs", "ru"))
	if err != nil {
		t.Fatalf("LoadLexicon: %v", err)
	}
	intent, ok, err := book.DetectClose(int12Lemmatizer{}, lex, int12CloseReplica)
	if err != nil || !ok {
		t.Fatalf("реплика %q не распознана как закрытие главы: intent=%q err=%v", int12CloseReplica, intent, err)
	}
	report["close_intent"] = intent

	ch := int12OpenChapter(t, h)
	if err := ch.Close(h, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("закрыть главу: %v", err)
	}

	// Сцена-кандидат оседания: закрытая сцена с привязкой биндера, чтобы очистка
	// вырезала имя в слот {NAME}. Живой ход оставляет сцены open (дефект), статус
	// закрытой сцены в e2e-сценарии ставится напрямую.
	orders := int12SedimentOrders(t, h)
	if len(orders) == 0 {
		t.Fatalf("очередь: заказов оседания нет (turns=%d, scenes=%d, queue=%d)",
			int12Count(t, h, `SELECT COUNT(*) FROM turns`),
			int12Count(t, h, `SELECT COUNT(*) FROM scenes`),
			int12Count(t, h, `SELECT COUNT(*) FROM workshop_queue`))
	}
	turnID := orders[0].TurnID
	var sceneID int64
	if err := h.QueryRow(`SELECT id FROM scenes WHERE turn_id = ? ORDER BY id DESC LIMIT 1`, turnID).Scan(&sceneID); err != nil {
		t.Fatalf("сцена хода %d: %v", turnID, err)
	}
	if _, err := h.Exec("scenes", `UPDATE scenes SET prose = ?, status = 'closed' WHERE id = ?`, int12SceneProse, sceneID); err != nil {
		t.Fatalf("UPDATE scenes: %v", err)
	}
	if _, err := h.Exec("scenes", `UPDATE scenes SET status = 'closed' WHERE status = 'open'`); err != nil {
		t.Fatalf("закрыть сцены: %v", err)
	}
	var charID int64
	if err := h.QueryRow(`SELECT id FROM characters ORDER BY id LIMIT 1`).Scan(&charID); err != nil {
		t.Fatalf("персонаж для привязки: %v", err)
	}
	if _, err := h.Exec("characters", `UPDATE characters SET name = ? WHERE id = ?`, "Иван", charID); err != nil {
		t.Fatalf("UPDATE characters: %v", err)
	}
	if _, err := h.Exec("token_bindings",
		`INSERT INTO token_bindings (scene_id, token, character_id, updated_at) VALUES (?, ?, ?, ?)`,
		sceneID, "[NPCname]", charID, "2026-11-01T22:00:00Z"); err != nil {
		t.Fatalf("INSERT token_bindings: %v", err)
	}

	// Повтор того же кандидата — заказ-дубль: идемпотентность оседания помечает
	// повтор в журнале sediment.dup_skip и не пишет вторую строку sediment_scene
	// (T-04-10).
	var payload string
	if err := h.QueryRow(`SELECT payload_json FROM workshop_queue WHERE id = ?`, orders[0].ID).Scan(&payload); err != nil {
		t.Fatalf("payload заказа оседания: %v", err)
	}
	if _, err := h.Exec("workshop_queue",
		`INSERT INTO workshop_queue (kind, payload_json, priority, status, step, resumable, attempts, created_at) VALUES ('sediment', ?, 50, 'queued', 'light', 0, 0, ?)`,
		payload, "2026-11-01T22:00:01Z"); err != nil {
		t.Fatalf("INSERT дубля оседания: %v", err)
	}

	// Заготовка следующего дня — тяжёлый ночной заказ (W-01.7, CB-01.2).
	if err := book.EnqueueWakeForge(h, ch.ID); err != nil {
		t.Fatalf("EnqueueWakeForge: %v", err)
	}

	nightLog := filepath.Join(t.TempDir(), "night.log")
	logger, err := journal.New(nightLog)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	ctx := context.Background()
	clock := testkit.NewClock()
	slot := &int12Slot{}
	sched := workshop.NewScheduler(h, clock, slot, logger)
	sched.SetChapterClosed(true)

	// Шаг 1: до паузы и на лёгкой ступени заказы оседания не исполняются.
	report["step_start"] = string(sched.Step())
	clock.Advance(10 * time.Minute)
	report["step_light"] = string(sched.Step())
	if got := sched.Step(); got != workshop.PauseLight {
		t.Fatalf("ступень на 10 минутах = %q, want %q", got, workshop.PauseLight)
	}
	if err := sched.Run(ctx); err != nil {
		t.Fatalf("Run(light): %v", err)
	}
	report["sediment_light_queued"] = int12Count(t, h,
		`SELECT COUNT(*) FROM workshop_queue WHERE kind = 'sediment' AND status = 'queued'`)
	if report["sediment_light_queued"] != len(orders)+1 {
		t.Fatalf("на лёгкой ступени заказов оседания queued = %v, want %d",
			report["sediment_light_queued"], len(orders)+1)
	}

	// Молчание больше 8 часов при закрытой главе — ступень «ночь» (INT-10.1).
	clock.Advance(9 * time.Hour)
	report["step_night"] = string(sched.Step())
	if got := sched.Step(); got != workshop.PauseNight {
		t.Fatalf("ступень после 9 часов молчания = %q, want %q", got, workshop.PauseNight)
	}

	pipeline := sediment.NewPipeline(testkit.NewFakeEmbed(), slot, int12Censor{}, logger)
	sched.SetSedimentRunner(workshop.NewSedimentRunner(litSed, pipeline))
	if err := sched.Run(ctx); err != nil {
		t.Fatalf("Run(night): %v", err)
	}

	// Шаг 2: ночные работы очереди исполнены — тяжёлые заказы дошли до слота,
	// оседание прошло очистку, дедуп и авто-критик; повтор сцены помечен в
	// журнале sediment.dup_skip и второй строки не создал.
	report["night_roles"] = strings.Join(slot.roles, ",")
	if !strings.Contains(report["night_roles"].(string), "wakeup") {
		t.Fatalf("ночная заготовка не исполнена, роли слота = %v", slot.roles)
	}
	if got := int12Count(t, h, `SELECT COUNT(*) FROM workshop_queue WHERE kind = 'sediment' AND status = 'done'`); got != len(orders)+1 {
		t.Fatalf("заказов оседания done = %d, want %d", got, len(orders)+1)
	}
	if got := int12Count(t, h, `SELECT COUNT(*) FROM workshop_queue WHERE kind = 'sediment' AND status = 'queued'`); got != 0 {
		t.Fatalf("заказов оседания queued после ночи = %d, want 0", got)
	}

	var cleaned, verdict string
	var trust int
	if err := litSed.QueryRow(`SELECT cleaned_text, verdict, trust_level FROM sediment_scene WHERE source_scene_id = ?`, sceneID).
		Scan(&cleaned, &verdict, &trust); err != nil {
		t.Fatalf("SELECT sediment_scene: %v", err)
	}
	report["cleaned_text"] = cleaned
	report["verdict"] = verdict
	if cleaned != "{NAME} стоит у ворот и ждёт." {
		t.Fatalf("очистка не вырезала имя: cleaned_text = %q", cleaned)
	}
	if verdict != sediment.VerdictClean || trust != sediment.TrustLevelUnchecked {
		t.Fatalf("sediment_scene verdict/trust = %q/%d, want %q/%d", verdict, trust, sediment.VerdictClean, sediment.TrustLevelUnchecked)
	}
	rows := int12Count(t, litSed, `SELECT COUNT(*) FROM sediment_scene`)
	report["sediment_rows"] = rows
	if rows != len(orders) {
		t.Fatalf("строк sediment_scene = %d, want %d (дубль не пишется)", rows, len(orders))
	}

	events := int12JournalRecords(t, nightLog)
	report["clean_events"] = int12EventCount(events, sediment.EventClean)
	report["dedup_events"] = int12EventCount(events, sediment.EventDedup)
	report["autocritic_events"] = int12EventCount(events, sediment.EventAutocriticVerdict)
	if report["clean_events"].(int) == 0 || report["dedup_events"].(int) == 0 || report["autocritic_events"].(int) == 0 {
		t.Fatalf("в журнале нет ступеней оседания: clean=%v dedup=%v autocritic=%v",
			report["clean_events"], report["dedup_events"], report["autocritic_events"])
	}
	outcomes := int12SedimentOutcomes(events)
	report["outcomes"] = strings.Join(outcomes, ",")
	// Повтор уже осевшей сцены идемпотентен: второй заказ закрывается без второй
	// строки, журнал помечает повтор событием sediment.dup_skip (T-04-10).
	report["dup_skip_events"] = int12EventCount(events, sediment.EventDupSkip)
	if report["dup_skip_events"].(int) == 0 {
		t.Fatalf("в журнале нет события %s", sediment.EventDupSkip)
	}
	if len(outcomes) != len(orders)+1 || !int12AllStrings(outcomes, string(workshop.SedimentWrite)) {
		t.Fatalf("исходы оседания = %v, want %d записей write", outcomes, len(orders)+1)
	}

	// Шаг 3: банк пополнен ночной ковкой, Пифия записала карту игрока.
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	bankBefore := int12Count(t, litForge, `SELECT COUNT(*) FROM bank_pool`)
	if _, err := workshop.StylizeClassic(ctx, litForge, "Черновик ночной заготовки."); err != nil {
		t.Fatalf("StylizeClassic: %v", err)
	}
	bankAfter := int12Count(t, litForge, `SELECT COUNT(*) FROM bank_pool`)
	report["bank_delta"] = bankAfter - bankBefore
	if bankAfter-bankBefore != 1 {
		t.Fatalf("пополнение банка = %d строк, want 1", bankAfter-bankBefore)
	}
	if got := int12Count(t, litForge, `SELECT COUNT(*) FROM bank_pool WHERE tags = 'style' AND text = ?`, int12StyleText); got != 1 {
		t.Fatalf("записей стилизации в банке = %d, want 1", got)
	}

	// Карточка героя — characters id 0: пачка рождения кладёт только NPC (дефект),
	// в e2e-сценарии карточка героя сеется с психотипом пачки.
	if _, err := h.Exec("characters",
		`INSERT INTO characters (id, name, psychotype, estate, card_json) VALUES (0, '', 'ENTJ', '', '{}')`); err != nil {
		t.Fatalf("INSERT characters (герой): %v", err)
	}
	const pythiaDay = 2
	if err := workshop.RunPythia(h, litForge, pythiaDay); err != nil {
		t.Fatalf("RunPythia: %v", err)
	}
	atlasRows := int12Count(t, h, `SELECT COUNT(*) FROM player_atlas WHERE day = ? AND source = 'pythia'`, pythiaDay)
	report["atlas_rows"] = atlasRows
	if atlasRows == 0 {
		t.Fatal("player_atlas не пополнен ночным прогоном Пифии")
	}
	habit := int12Count(t, h, `SELECT COUNT(*) FROM player_atlas WHERE day = ? AND theme = 'habit' AND habits = 'прогулки'`, pythiaDay)
	report["atlas_habits"] = habit
	if habit != 1 {
		t.Fatalf("строк привычек в player_atlas = %d, want 1", habit)
	}
	withPsychotype := int12Count(t, h, `SELECT COUNT(*) FROM player_atlas WHERE day = ? AND psychotype = 'ENTJ'`, pythiaDay)
	report["atlas_psychotype"] = withPsychotype
	if withPsychotype != atlasRows {
		t.Fatalf("строк с психотипом пачки = %d, want %d", withPsychotype, atlasRows)
	}
	if got := int12Count(t, h, `SELECT COUNT(*) FROM player_atlas WHERE day = ? AND psychotype = 'INTP'`, pythiaDay); got != 0 {
		t.Fatalf("психотип модели %q попал в карту: %d строк", "INTP", got)
	}
	report["pythia_log"] = int12Count(t, litForge, `SELECT COUNT(*) FROM forge_log WHERE kind = 'pythia' AND status = 'done'`)
	if report["pythia_log"] != 1 {
		t.Fatalf("записей forge_log pythia/done = %v, want 1", report["pythia_log"])
	}

	writeReport(t, "int12", report)
}

// int12OpenChapter читает открытую главу и её сессию: книга связи глав-сессий не
// хранит, её держит ядро (T-07-01).
func int12OpenChapter(t *testing.T, h *db.Handle) book.Chapter {
	t.Helper()
	var ch book.Chapter
	var status, kind string
	if err := h.QueryRow(
		`SELECT id, day, title, status, kind FROM book_chapters WHERE status = 'open' ORDER BY id DESC LIMIT 1`).
		Scan(&ch.ID, &ch.Day, &ch.Title, &status, &kind); err != nil {
		t.Fatalf("открытая глава: %v", err)
	}
	ch.Status = book.ChapterStatus(status)
	ch.Kind = book.ChapterKind(kind)
	if err := h.QueryRow(`SELECT id FROM sessions WHERE ended_at IS NULL ORDER BY id DESC LIMIT 1`).Scan(&ch.SessionID); err != nil {
		t.Fatalf("открытая сессия главы: %v", err)
	}
	return ch
}

// int12SedimentOrders возвращает queued-заказы оседания с turn_id сцены.
func int12SedimentOrders(t *testing.T, h *db.Handle) []int12SedimentOrder {
	t.Helper()
	rows, err := h.Query(`SELECT id, payload_json FROM workshop_queue WHERE kind = 'sediment' AND status = 'queued' ORDER BY id`)
	if err != nil {
		t.Fatalf("SELECT workshop_queue: %v", err)
	}
	defer rows.Close()
	var out []int12SedimentOrder
	for rows.Next() {
		var o int12SedimentOrder
		var raw string
		if err := rows.Scan(&o.ID, &raw); err != nil {
			t.Fatalf("scan workshop_queue: %v", err)
		}
		var p workshop.SedimentOrderPayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatalf("payload оседания: %v", err)
		}
		o.TurnID = p.TurnID
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return out
}

// int12Count считает строки запросом COUNT(*).
func int12Count(t *testing.T, h *db.Handle, query string, args ...any) int {
	t.Helper()
	var n int
	if err := h.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("COUNT: %v", err)
	}
	return n
}

// int12JournalRecord — строка журнала станка: событие и поля.
type int12JournalRecord struct {
	Event  string          `json:"event"`
	Fields []journal.Field `json:"fields"`
}

// int12JournalRecords читает журнал ночного прогона.
func int12JournalRecords(t *testing.T, path string) []int12JournalRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var out []int12JournalRecord
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var rec int12JournalRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка журнала не JSON: %v\n%s", err, line)
		}
		out = append(out, rec)
	}
	return out
}

// int12EventCount считает события журнала с заданным именем.
func int12EventCount(recs []int12JournalRecord, event string) int {
	n := 0
	for _, r := range recs {
		if r.Event == event {
			n++
		}
	}
	return n
}

// int12SedimentOutcomes собирает исходы заказов оседания из журнала.
func int12SedimentOutcomes(recs []int12JournalRecord) []string {
	var out []string
	for _, r := range recs {
		if r.Event != "workshop.sediment" {
			continue
		}
		for _, f := range r.Fields {
			if f.Key == "outcome" {
				out = append(out, f.Value)
			}
		}
	}
	return out
}

// int12AllStrings сообщает, все ли значения равны want.
func int12AllStrings(values []string, want string) bool {
	for _, v := range values {
		if v != want {
			return false
		}
	}
	return true
}
