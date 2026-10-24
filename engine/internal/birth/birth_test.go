package birth

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
)

func TestBirthCreatesWorld(t *testing.T) {
	dir := t.TempDir()
	res, err := Birth(validAncketa(), Config{WorldsRoot: dir, BaseDBPath: testBaseDB(t, dir), Embed: testkit.NewFakeEmbed()})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}
	if res == nil {
		t.Fatal("Birth: результат пуст")
	}

	// Каталог существует.
	if _, err := os.Stat(res.Dir); err != nil {
		t.Fatalf("каталог мира не создан: %v", err)
	}

	// Обе базы открываются.
	worldDB := openReader(t, res.WorldDB)
	litDB := openReader(t, res.LiteraryDB)

	// world_meta содержит 14 ключей анкеты плюс якорь сценарной рамки (15).
	var n int
	if err := worldDB.QueryRow(`SELECT COUNT(*) FROM world_meta`).Scan(&n); err != nil {
		t.Fatalf("world_meta count: %v", err)
	}
	if n != 15 {
		t.Fatalf("ключей world_meta: %d, want 15", n)
	}

	var keys []string
	rows, err := worldDB.Query(`SELECT "key" FROM world_meta`)
	if err != nil {
		t.Fatalf("world_meta keys: %v", err)
	}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatalf("world_meta scan: %v", err)
		}
		keys = append(keys, k)
	}
	rows.Close()

	want := append([]string{}, Fields...)
	want = append(want, anchorKey)
	sort.Strings(want)
	sort.Strings(keys)
	if !slices.Equal(keys, want) {
		t.Fatalf("ключи world_meta: %v, want %v", keys, want)
	}

	// Литературная база открывается по схеме литературной.
	if _, err := litDB.Query(`SELECT COUNT(*) FROM bank_pool`); err != nil {
		t.Fatalf("литературная база не открывается по схеме: %v", err)
	}

	// Состояние — ready (шаг 4).
	if st, err := readState(res.Dir); err != nil || st != stateReady {
		t.Fatalf("состояние мира: %q (%v), want %q", st, err, stateReady)
	}
}

func TestBirthRollback(t *testing.T) {
	dir := t.TempDir()
	a := validAncketa()

	_, err := Birth(a, Config{WorldsRoot: dir, FailStep: "pack"})
	if err == nil {
		t.Fatal("рождение, прерванное на шаге пачки, не вернуло ошибку")
	}

	worldDir := filepath.Join(dir, "worlds", a.Slug)
	if _, statErr := os.Stat(worldDir); !os.IsNotExist(statErr) {
		t.Fatalf("частично созданный каталог остался: %v", statErr)
	}
}

func TestBirthDuplicateSlug(t *testing.T) {
	dir := t.TempDir()
	a := validAncketa()

	res, err := Birth(a, Config{WorldsRoot: dir, BaseDBPath: testBaseDB(t, dir), Embed: testkit.NewFakeEmbed()})
	if err != nil {
		t.Fatalf("первое рождение: %v", err)
	}

	beforeWorld, err := os.ReadFile(res.WorldDB)
	if err != nil {
		t.Fatalf("чтение оперативной базы до повтора: %v", err)
	}
	beforeLit, err := os.ReadFile(res.LiteraryDB)
	if err != nil {
		t.Fatalf("чтение литературной базы до повтора: %v", err)
	}

	_, err = Birth(a, Config{WorldsRoot: dir})
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("повторное рождение: ожидался *ExitError, получен %T: %v", err, err)
	}
	if ee.Code != ExitDuplicate {
		t.Fatalf("код повторного рождения: %d, want %d", ee.Code, ExitDuplicate)
	}

	// Существующий мир не тронут: обе базы побайтово не изменились.
	afterWorld, err := os.ReadFile(res.WorldDB)
	if err != nil {
		t.Fatalf("чтение оперативной базы после повтора: %v", err)
	}
	afterLit, err := os.ReadFile(res.LiteraryDB)
	if err != nil {
		t.Fatalf("чтение литературной базы после повтора: %v", err)
	}
	if !bytes.Equal(beforeWorld, afterWorld) {
		t.Fatal("оперативная база изменилась при повторном рождении")
	}
	if !bytes.Equal(beforeLit, afterLit) {
		t.Fatal("литературная база изменилась при повторном рождении")
	}
}

func TestEmptySynopsis(t *testing.T) {
	dir := t.TempDir()
	a := validAncketa()
	a.Synopsis = ""

	var packCalls int
	_, err := Birth(a, Config{WorldsRoot: dir, PackCalls: &packCalls})

	// Код отказа, а не рождение мира.
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("ожидался *ExitError, получен %T: %v", err, err)
	}
	if ee.Code != ExitInvalid {
		t.Fatalf("код отказа: %d, want %d", ee.Code, ExitInvalid)
	}

	// Подсказка «заполните синопсис».
	if !strings.Contains(err.Error(), "заполните синопсис") {
		t.Fatalf("отказ не содержит подсказку «заполните синопсис»: %v", err)
	}

	// Каталог мира не создан.
	worldDir := filepath.Join(dir, "worlds", a.Slug)
	if _, statErr := os.Stat(worldDir); !os.IsNotExist(statErr) {
		t.Fatalf("каталог мира создан при пустом синопсисе: %v", statErr)
	}

	// Карточек 0: генерация пачки не вызывалась — валидация единственная точка
	// входа, до неё ни один шаг рождения не стартует.
	if packCalls != 0 {
		t.Fatalf("вызовов генерации пачки: %d, want 0", packCalls)
	}
}

func TestNoProseAtBirth(t *testing.T) {
	dir := t.TempDir()
	var proseCalls int
	res, err := Birth(validAncketa(), Config{WorldsRoot: dir, ProseCalls: &proseCalls, BaseDBPath: testBaseDB(t, dir), Embed: testkit.NewFakeEmbed()})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}

	// Шаг 2: цепочка рождения не вызывала слот прозы.
	if proseCalls != 0 {
		t.Fatalf("вызовов слота прозы: %d, want 0", proseCalls)
	}

	// Шаг 1: три таблицы живого хода пусты.
	worldDB := openReader(t, res.WorldDB)
	for _, table := range []string{"scenes", "turns", "book_text"} {
		var n int
		if err := worldDB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			t.Fatalf("%s count: %v", table, err)
		}
		if n != 0 {
			t.Fatalf("%s: %d строк, want 0", table, n)
		}
	}
}

func TestNoProseAtBirthRejectsSeededProse(t *testing.T) {
	dir := t.TempDir()
	res, err := Birth(validAncketa(), Config{WorldsRoot: dir, BaseDBPath: testBaseDB(t, dir), Embed: testkit.NewFakeEmbed()})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}

	// Готовая сцена, записанная мимо цепочки, обязана быть замечена гвардией.
	h, err := db.OpenWriterAs(res.WorldDB, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	if _, err := h.Exec("scenes",
		`INSERT INTO scenes (session_id, day, status, prose) VALUES (0, 1, 'open', 'готовый текст')`); err != nil {
		t.Fatalf("INSERT scenes: %v", err)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := checkNoProseAtBirth(res.WorldDB, nil); err == nil {
		t.Fatal("гвардия не заметила готовую прозу в scenes")
	}
}

func openReader(t *testing.T, path string) *sql.DB {
	t.Helper()
	d, err := db.OpenReader(path)
	if err != nil {
		t.Fatalf("OpenReader(%s): %v", path, err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}
