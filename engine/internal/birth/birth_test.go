package birth

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"tns65/internal/db"
)

func TestBirthCreatesWorld(t *testing.T) {
	dir := t.TempDir()
	res, err := Birth(validAncketa(), Config{WorldsRoot: dir})
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

	// world_meta содержит ровно 14 ключей анкеты.
	var n int
	if err := worldDB.QueryRow(`SELECT COUNT(*) FROM world_meta`).Scan(&n); err != nil {
		t.Fatalf("world_meta count: %v", err)
	}
	if n != 14 {
		t.Fatalf("ключей world_meta: %d, want 14", n)
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

	res, err := Birth(a, Config{WorldsRoot: dir})
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

func openReader(t *testing.T, path string) *sql.DB {
	t.Helper()
	d, err := db.OpenReader(path)
	if err != nil {
		t.Fatalf("OpenReader(%s): %v", path, err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}
