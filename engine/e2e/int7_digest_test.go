//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"tns65/internal/book"
	"tns65/internal/db"
	"tns65/internal/workshop"
)

// int7Digest — дайджест ровно из шести предложений по сплиттеру `. ! ? …`
// (T-07-06).
const int7Digest = "Первое предложение. Второе предложение. Третье предложение. Четвёртое предложение. Пятое предложение. Шестое предложение."

// int7EditorSlot — слот Редактора: текст не правится (пустой список правок),
// дайджест — шесть предложений; порядок вызовов ролей запоминается, чтобы
// проверить «текст → дайджест» и один вызов дайджеста на главу.
type int7EditorSlot struct {
	roles []string
}

func (s *int7EditorSlot) Generate(_ context.Context, role, _ string) (string, error) {
	s.roles = append(s.roles, role)
	switch role {
	case workshop.EditorRole:
		return `{"edits":[]}`, nil
	case workshop.EditorDigestRole:
		return `{"summary":` + strconv.Quote(int7Digest) + `}`, nil
	default:
		return "", fmt.Errorf("e2e: неожиданная роль Редактора %q", role)
	}
}

// digestCalls — число вызовов слота Редактора для дайджеста (роль editor_digest).
func (s *int7EditorSlot) digestCalls() int {
	n := 0
	for _, r := range s.roles {
		if r == workshop.EditorDigestRole {
			n++
		}
	}
	return n
}

// textBeforeDigest — все вызовы правки текста идут до вызова дайджеста.
func (s *int7EditorSlot) textBeforeDigest() bool {
	seenDigest := false
	seenText := false
	for _, r := range s.roles {
		if r == workshop.EditorDigestRole {
			seenDigest = true
			continue
		}
		if r == workshop.EditorRole {
			if seenDigest {
				return false
			}
			seenText = true
		}
	}
	return seenText && seenDigest
}

// TestINT7 — INT-7. Дайджест главы (T-09-07): закрытая и обработанная глава
// получает дайджест 5–6 предложений; Редактор вызывается один раз на главу,
// после обработки текста; «дайджест» и «сводка» в словаре интерфейса разведены.
func TestINT7(t *testing.T) {
	bin := buildBinary(t, []string{"e2e"})
	dir := worldDir(t)

	slug := "daidzhest-glavy"
	ancketa := writeAncketa(t, t.TempDir(), slug,
		"Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.")
	stdout, stderr, code := runCmd(t, dir, bin, "world", "birth", "--ancketa", ancketa, "--fake-embed", "--fake-slot")
	if code != 0 {
		t.Fatalf("world birth exit=%d, want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	worldDB := worldDBPath(dir, slug)
	h, err := db.OpenWriterAs(worldDB, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("открыть базу мира: %v", err)
	}
	defer h.Close()

	report := map[string]any{}

	// Шаг 1: закрыть главу сном и дождаться обработки — разобрать очередь
	// редактуры и исполнить задачу edit на главу.
	ch := seedOpenChapter(t, h)
	if err := ch.Close(h, "2026-10-31T22:00:00Z"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := book.AssembleChapter(h, ch); err != nil {
		t.Fatalf("AssembleChapter: %v", err)
	}
	task, err := workshop.DequeueEditorTask(h)
	if err != nil {
		t.Fatalf("DequeueEditorTask: %v", err)
	}
	if task == nil {
		t.Fatal("задачи редактуры нет после закрытия главы")
	}

	slot := &int7EditorSlot{}
	if _, err := workshop.RunEditorTask(context.Background(), h, slot, task); err != nil {
		t.Fatalf("RunEditorTask: %v", err)
	}

	// Шаг 2: раздел «Краткое содержание» — дайджест из book_chapters.digest,
	// 5–6 предложений по сплиттеру `. ! ? …` (счёт точный, не диапазон).
	digest := chapterDigest(t, h, ch.ID)
	sentences := book.CountDigestSentences(digest)
	report["sentences"] = sentences
	if sentences != 5 && sentences != 6 {
		t.Fatalf("sentences=%d, want 5 или 6; digest=%q", sentences, digest)
	}

	// Шаг 3: один вызов слота Редактора на главу, порядок «текст → дайджест».
	report["editor_calls"] = slot.digestCalls()
	report["order_ok"] = slot.textBeforeDigest()
	if report["editor_calls"] != 1 {
		t.Fatalf("editor_calls=%d, want 1", report["editor_calls"])
	}
	if report["order_ok"] != true {
		t.Fatalf("order_ok=false: вызовы Редактора = %v", slot.roles)
	}

	// Шаг 4: сводка мастерской дайджестом не называется — термины в i18n.js
	// разведены, пересечений нет.
	report["terms_separated"] = i18nTermsSeparated(t)
	if report["terms_separated"] != true {
		t.Fatal("terms_separated=false: «дайджест» и «сводка» в i18n.js пересекаются")
	}

	writeReport(t, "int7", report)
}

// seedOpenChapter ставит открытую главу с сессией и журналом (ход и сцена),
// чтобы после закрытия и сборки у главы был текст для редактуры.
func seedOpenChapter(t *testing.T, h *db.Handle) book.Chapter {
	t.Helper()
	res, err := h.Exec("sessions",
		`INSERT INTO sessions (started_at, mode) VALUES ('2026-10-31T08:00:00Z', 'normal')`)
	if err != nil {
		t.Fatalf("INSERT sessions: %v", err)
	}
	sessionID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("sessions id: %v", err)
	}

	res, err = h.Exec("book_chapters",
		`INSERT INTO book_chapters (day, title, status, kind) VALUES (1, 'Глава 1', 'open', 'chapter')`)
	if err != nil {
		t.Fatalf("INSERT book_chapters: %v", err)
	}
	chapterID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("book_chapters id: %v", err)
	}

	if _, err := h.Exec("turns",
		`INSERT INTO turns (session_id, player_text, reply_text, saw_recommendation, trust_level, latency_ms, degraded) VALUES (?, ?, ?, 0, 1, 10, 0)`,
		sessionID, "Мы легли спать.", "Доброй ночи."); err != nil {
		t.Fatalf("INSERT turns: %v", err)
	}
	if _, err := h.Exec("scenes",
		`INSERT INTO scenes (session_id, day, status, prose) VALUES (?, 1, 'closed', 'Ночь опустилась на город.')`,
		sessionID); err != nil {
		t.Fatalf("INSERT scenes: %v", err)
	}

	return book.Chapter{ID: chapterID, Day: 1, Title: "Глава 1", Status: book.StatusOpen, Kind: book.KindChapter, SessionID: sessionID}
}

// chapterDigest — дайджест главы из book_chapters.digest.
func chapterDigest(t *testing.T, h *db.Handle, chapterID int64) string {
	t.Helper()
	var s string
	if err := h.QueryRow(`SELECT digest FROM book_chapters WHERE id = ?`, chapterID).Scan(&s); err != nil {
		t.Fatalf("SELECT book_chapters.digest: %v", err)
	}
	return s
}

// i18nPairRE — пара «ключ: значение» словаря i18n.js (ключи и значения в кавычках).
var i18nPairRE = regexp.MustCompile(`"([^"]+)"\s*:\s*"([^"]*)"`)

// i18nTermsSeparated — термины «дайджест» (игроку) и «сводка» (мастерской) в
// i18n.js разведены: значения ключей digest содержат «дайджест», значения ключей
// summary — «сводка», пересечений нет.
func i18nTermsSeparated(t *testing.T) bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(engineRoot(t), "web", "static", "i18n.js"))
	if err != nil {
		t.Fatalf("read i18n.js: %v", err)
	}
	matches := i18nPairRE.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatal("i18n.js: пар ключ→значение не найдено")
	}

	const digestTerm = "дайджест"
	const summaryTerm = "сводка"
	var digestValues, summaryValues []string
	for _, m := range matches {
		key := strings.ToLower(m[1])
		value := m[2]
		low := strings.ToLower(value)
		if strings.Contains(low, digestTerm) && strings.Contains(low, summaryTerm) {
			return false
		}
		if strings.Contains(key, "digest") {
			digestValues = append(digestValues, value)
		}
		if strings.Contains(key, "summary") {
			summaryValues = append(summaryValues, value)
		}
	}
	if len(digestValues) == 0 || len(summaryValues) == 0 {
		return false
	}
	for _, v := range digestValues {
		if !strings.Contains(strings.ToLower(v), digestTerm) || strings.Contains(strings.ToLower(v), summaryTerm) {
			return false
		}
	}
	for _, v := range summaryValues {
		if !strings.Contains(strings.ToLower(v), summaryTerm) || strings.Contains(strings.ToLower(v), digestTerm) {
			return false
		}
	}
	return true
}
