package birth

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"tns65/internal/db"
	"tns65/internal/library"
	"tns65/internal/models"
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

	// world_meta содержит 15 ключей анкеты, якорь сценарной рамки, четыре
	// ключа материализации якоря (T-10-06) и отпечаток реестра семей
	// (T-10-13) — всего 21.
	var n int
	if err := worldDB.QueryRow(`SELECT COUNT(*) FROM world_meta`).Scan(&n); err != nil {
		t.Fatalf("world_meta count: %v", err)
	}
	if n != 21 {
		t.Fatalf("ключей world_meta: %d, want 21", n)
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
	want = append(want, anchorKey, anchorNameKey, anchorSummaryKey, anchorVersesKey, anchorFamiliesKey, registryKey)
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

	_, err := Birth(a, Config{WorldsRoot: dir, BaseDBPath: testBaseDB(t, dir), FailStep: "pack"})
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

// TestBirthBaseRootDefault — пустой BaseDBPath берёт базу языка из каталога
// поставки db.BaseRoot (base/base.<lang>.db), а не из world_db/ (T-10-02).
func TestBirthBaseRootDefault(t *testing.T) {
	worldsRoot := t.TempDir()
	baseDir := t.TempDir()

	// Фикстура поставки: base.ru.db под db.BaseRoot (в проде — base/base.ru.db).
	vec := make([]float32, models.EmbedDim)
	vec[0] = 1
	writeAnchorBaseDB(t, filepath.Join(baseDir, "base.ru.db"), [][]float32{vec})

	old := db.BaseRoot
	db.BaseRoot = baseDir
	t.Cleanup(func() { db.BaseRoot = old })

	res, err := Birth(validAncketa(), Config{WorldsRoot: worldsRoot, Embed: testkit.NewFakeEmbed()})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}

	// Якорь прочитан из базы под db.BaseRoot.
	if ids := anchorIDs(t, res.WorldDB); len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("якорей в world_meta: %v, want [1]", ids)
	}

	// Под WorldsRoot каталог base/ не создаётся.
	if _, err := os.Stat(filepath.Join(worldsRoot, "base")); !os.IsNotExist(err) {
		t.Fatalf("каталог base/ создан под WorldsRoot: %v", err)
	}
}

// TestValidateLang — язык анкеты по установленным базам (TUN-2, T-10-03):
// пустой язык и не установленный язык дают код 2 с полем lang до создания
// каталога мира; установленный язык проходит до шага pack.
func TestValidateLang(t *testing.T) {
	worldsRoot := t.TempDir()
	baseDir := t.TempDir()
	writeLangBaseDB(t, baseDir, "en")
	old := db.BaseRoot
	db.BaseRoot = baseDir
	t.Cleanup(func() { db.BaseRoot = old })

	a := validAncketa()
	a.Lang = ""
	assertLangRejected(t, worldsRoot, a, "язык не указан")

	a = validAncketa()
	a.Lang = "klingon"
	assertLangRejected(t, worldsRoot, a, "нет базы base.klingon.db")

	a = validAncketa()
	a.Lang = "en"
	var packCalls int
	if _, err := Birth(a, Config{WorldsRoot: worldsRoot, Embed: testkit.NewFakeEmbed(), PackCalls: &packCalls}); err != nil {
		t.Fatalf("Birth с установленным языком: %v", err)
	}
	if packCalls != 1 {
		t.Fatalf("вызовов генерации пачки: %d, want 1", packCalls)
	}
}

func assertLangRejected(t *testing.T, worldsRoot string, a *Ancketa, wantReason string) {
	t.Helper()
	_, err := Birth(a, Config{WorldsRoot: worldsRoot})
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("ожидался *ExitError, получен %T: %v", err, err)
	}
	if ee.Code != ExitInvalid {
		t.Fatalf("код отказа: %d, want %d", ee.Code, ExitInvalid)
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("ожидался *ValidationError, получен %T: %v", err, err)
	}
	if len(ve.Fields) != 1 || ve.Fields[0].Field != "lang" {
		t.Fatalf("перечень полей: %+v, want ровно одно поле lang", ve.Fields)
	}
	if !strings.Contains(ve.Fields[0].Reason, wantReason) {
		t.Fatalf("причина: %q, want содержит %q", ve.Fields[0].Reason, wantReason)
	}
	worldDir := filepath.Join(worldsRoot, "worlds", a.Slug)
	if _, statErr := os.Stat(worldDir); !os.IsNotExist(statErr) {
		t.Fatalf("каталог мира создан при невалидном языке: %v", statErr)
	}
}

// TestBirthSourceBookBinding — рождение фанфика с книгой-источником: world_meta
// получает source_book_id и source_book_name, а реестр literary.db — строку
// книги с полочным id (P11 §5.5, T-12-13).
func TestBirthSourceBookBinding(t *testing.T) {
	dir := t.TempDir()

	// Полка с книгой-источником.
	shelf := filepath.Join(dir, "library")
	book := library.Book{
		ID:        "garry-potter",
		Name:      "Гарри Поттер",
		Format:    "txt",
		Lang:      "ru",
		Status:    library.StatusImported,
		CreatedAt: "2026-01-01T00:00:00Z",
	}
	if err := library.Save(shelf, []library.Book{book}); err != nil {
		t.Fatalf("Save полки: %v", err)
	}

	a := validAncketa()
	a.Genres = []string{"фанфик"}
	a.SourceBook = "garry-potter"

	res, err := Birth(a, Config{WorldsRoot: dir, BaseDBPath: testBaseDB(t, dir), Embed: testkit.NewFakeEmbed()})
	if err != nil {
		t.Fatalf("Birth с книгой-источником: %v", err)
	}

	worldDB := openReader(t, res.WorldDB)
	var idRaw, name string
	if err := worldDB.QueryRow(`SELECT value FROM world_meta WHERE "key" = 'source_book_id'`).Scan(&idRaw); err != nil {
		t.Fatalf("source_book_id в world_meta: %v", err)
	}
	bookID, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil {
		t.Fatalf("source_book_id не число: %q", idRaw)
	}
	if err := worldDB.QueryRow(`SELECT value FROM world_meta WHERE "key" = 'source_book_name'`).Scan(&name); err != nil {
		t.Fatalf("source_book_name в world_meta: %v", err)
	}
	if name != "Гарри Поттер" {
		t.Fatalf("source_book_name = %q, want %q", name, "Гарри Поттер")
	}

	// Реестр литературной базы содержит книгу с полочным id и совпадающим id.
	litDB := openReader(t, res.LiteraryDB)
	var shelfID, bookName string
	if err := litDB.QueryRow(`SELECT shelf_id, name FROM book WHERE id = ?`, bookID).Scan(&shelfID, &bookName); err != nil {
		t.Fatalf("книга в реестре по id %d: %v", bookID, err)
	}
	if shelfID != "garry-potter" || bookName != "Гарри Поттер" {
		t.Fatalf("реестр: shelf_id=%q name=%q, want garry-potter/Гарри Поттер", shelfID, bookName)
	}
}

// TestBirthSourceBookRequired — фанфик без книги-источника (или с id, которого
// нет на полке) отвергается ошибкой поля source_book (P11 §5.5, T-12-13).
func TestBirthSourceBookRequired(t *testing.T) {
	dir := t.TempDir()

	// Фанфик с пустой книгой-источником.
	a := validAncketa()
	a.Genres = []string{"фанфик"}
	a.SourceBook = ""
	_, err := Birth(a, Config{WorldsRoot: dir, BaseDBPath: testBaseDB(t, dir), Embed: testkit.NewFakeEmbed()})
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != ExitInvalid {
		t.Fatalf("фанфик без книги: код отказа %v, want ExitInvalid; err=%v", ee, err)
	}
	if !hasFieldError(err, "source_book") {
		t.Fatalf("фанфик без книги: ожидалось поле source_book, получено %v", err)
	}

	// Фанфик с id, которого нет на полке.
	a = validAncketa()
	a.Genres = []string{"фанфик"}
	a.SourceBook = "net-takogo"
	_, err = Birth(a, Config{WorldsRoot: dir, BaseDBPath: testBaseDB(t, dir), Embed: testkit.NewFakeEmbed()})
	ee = nil
	if !errors.As(err, &ee) || ee.Code != ExitInvalid {
		t.Fatalf("фанфик с отсутствующей книгой: код отказа %v, want ExitInvalid; err=%v", ee, err)
	}
	if !hasFieldError(err, "source_book") {
		t.Fatalf("фанфик с отсутствующей книгой: ожидалось поле source_book, получено %v", err)
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
