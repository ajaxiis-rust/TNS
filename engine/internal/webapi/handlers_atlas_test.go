package webapi

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/birth"
	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/models"
	"tns65/internal/testkit"
)

// setupAtlas подменяет корень миров, base.db и эмбеддер на временные фикстуры и
// возвращает корень. Эмбеддер — фейк testkit (T-08-05, шаг 5), без сети.
func setupAtlas(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	oldRoot, oldBase, oldEmbed, oldLogger := atlasRoot, atlasBaseDB, atlasEmbed, atlasLogger
	atlasRoot = dir
	atlasBaseDB = writeAtlasBaseDB(t, dir)
	atlasEmbed = testkit.NewFakeEmbed()
	atlasLogger = nil
	setActiveSlug("")
	t.Cleanup(func() {
		atlasRoot, atlasBaseDB, atlasEmbed, atlasLogger = oldRoot, oldBase, oldEmbed, oldLogger
		setActiveSlug("")
	})
	return dir
}

// writeAtlasBaseDB создаёт base.db с одним нарративом и вектором dim=1024 —
// рождение обязано найти якорь (T-02-07).
func writeAtlasBaseDB(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "base.db")
	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := db.Migrate(w, "base", "shared"); err != nil {
		t.Fatal(err)
	}

	vec := make([]float32, models.EmbedDim)
	vec[0] = 1
	res, err := w.Exec(`INSERT INTO narratives (name, arc_summary, lang, created_at) VALUES (?,?,?,?)`,
		"n1", "arc 1", "ru", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec(`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		"narratives", id, "bge-m3", models.EmbedDim, "ru", "arc 1", float32Blob(vec), "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	return path
}

func float32Blob(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func atlasValidAncketa(slug string) *birth.Ancketa {
	return &birth.Ancketa{
		Slug:        slug,
		Title:       "Тёмный мир",
		Description: "Мир, где свет угас навсегда.",
		Synopsis:    "Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.",
		Prologue:    "Всё началось с тишины.",
		Lang:        "ru",
		TravelMode:  "вымышленная",
		Genres:      []string{"фэнтези", "хоррор"},
		WorldRules:  []string{"магия: редкая и опасная"},
		SocialOrder: "феодализм",
		EconomyMods: []string{"бартер"},
		Magic:       "редкая",
		HeroBirth:   "простолюдин",
		Censorship:  "16+",
	}
}

func atlasAncketaJSON(slug string) string {
	return `{"slug":"` + slug + `","title":"Тёмный мир","description":"Мир, где свет угас навсегда.","synopsis":"Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.","prologue":"Всё началось с тишины.","lang":"ru","travel_mode":"вымышленная","genres":["фэнтези","хоррор"],"world_rules":["магия: редкая и опасная"],"social_order":"феодализм","economy_mods":["бартер"],"magic":"редкая","hero_birth":"простолюдин","censorship":"16+"}`
}

func seedWorld(t *testing.T, dir, slug, title string) {
	t.Helper()
	a := atlasValidAncketa(slug)
	a.Title = title
	if _, err := birth.Birth(a, birth.Config{
		WorldsRoot: dir,
		BaseDBPath: filepath.Join(dir, "base.db"),
		Embed:      testkit.NewFakeEmbed(),
	}); err != nil {
		t.Fatalf("Birth(%s): %v", slug, err)
	}
}

func atlasRequest(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func worldCharacters(t *testing.T, dir, slug string) int {
	t.Helper()
	d, err := db.OpenReader(filepath.Join(dir, "worlds", slug, slug+".db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM characters`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAtlasList(t *testing.T) {
	dir := setupAtlas(t)
	seedWorld(t, dir, "alpha-world", "Альфа-мир")
	seedWorld(t, dir, "beta-world", "Бета-мир")

	rr := atlasRequest(t, NewHandler(Config{}), http.MethodGet, atlasWorldsPath, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", atlasWorldsPath, rr.Code, rr.Body.String())
	}
	var out atlasListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET %s: bad JSON: %v", atlasWorldsPath, err)
	}
	if out.Active != "" {
		t.Fatalf("active = %q, want empty", out.Active)
	}
	if len(out.Worlds) != 2 {
		t.Fatalf("worlds = %d, want 2", len(out.Worlds))
	}
	if out.Worlds[0].Slug != "alpha-world" || out.Worlds[0].Title != "Альфа-мир" {
		t.Fatalf("world[0] = %+v", out.Worlds[0])
	}
	if out.Worlds[1].Slug != "beta-world" || out.Worlds[1].Title != "Бета-мир" {
		t.Fatalf("world[1] = %+v", out.Worlds[1])
	}
	if out.Worlds[0].Characters != 96 {
		t.Fatalf("world[0] counters = %+v, want 96 characters", out.Worlds[0])
	}
	if out.Worlds[0].Entities != out.Worlds[0].Characters+out.Worlds[0].Places+out.Worlds[0].Paths {
		t.Fatalf("world[0] entities = %d, want characters+places+paths", out.Worlds[0].Entities)
	}
	if out.Worlds[0].Sessions != 0 {
		t.Fatalf("world[0] sessions = %d, want 0", out.Worlds[0].Sessions)
	}
	if out.Worlds[0].Active {
		t.Fatal("world[0] active = true, want false")
	}
}

func TestAtlasBirth(t *testing.T) {
	dir := setupAtlas(t)

	// Все 14 ключей анкеты перечислены в ROUTES.md (T-08-05, шаг 2).
	md, err := os.ReadFile("ROUTES.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range birth.Fields {
		if len(key) == 0 || !strings.Contains(string(md), key) {
			t.Fatalf("ключ анкеты %q не перечислен в ROUTES.md", key)
		}
	}
	if len(birth.Fields) != 14 {
		t.Fatalf("ключей анкеты: %d, want 14", len(birth.Fields))
	}

	h := NewHandler(Config{})
	rr := atlasRequest(t, h, http.MethodPost, atlasBirthPath, atlasAncketaJSON("sumerechnyj-mir"))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST %s: status = %d, want 200; body=%s", atlasBirthPath, rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "worlds", "sumerechnyj-mir")); err != nil {
		t.Fatalf("каталог мира не создан: %v", err)
	}
	if got := worldCharacters(t, dir, "sumerechnyj-mir"); got != 96 {
		t.Fatalf("карточек после рождения: %d, want 96", got)
	}

	// Валидация анкеты: пустой синопсис отвергается до создания каталога.
	bad := strings.Replace(atlasAncketaJSON("bad-world"), `"synopsis":"Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё."`, `"synopsis":""`, 1)
	rr = atlasRequest(t, h, http.MethodPost, atlasBirthPath, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("POST с пустым синопсисом: status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "worlds", "bad-world")); !os.IsNotExist(err) {
		t.Fatalf("каталог мира создан при пустом синопсисе: %v", err)
	}
}

func TestAtlasSwitch(t *testing.T) {
	dir := setupAtlas(t)
	seedWorld(t, dir, "alpha-world", "Альфа-мир")

	h := NewHandler(Config{})
	rr := atlasRequest(t, h, http.MethodPost, atlasActivePath, `{"world":"alpha-world"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST %s: status = %d, want 200; body=%s", atlasActivePath, rr.Code, rr.Body.String())
	}

	rr = atlasRequest(t, h, http.MethodGet, atlasWorldsPath, "")
	var out atlasListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Active != "alpha-world" {
		t.Fatalf("active = %q, want alpha-world", out.Active)
	}
	if len(out.Worlds) != 1 || !out.Worlds[0].Active {
		t.Fatalf("worlds = %+v, want одна активная карточка", out.Worlds)
	}

	// Несуществующий мир переключить нельзя.
	rr = atlasRequest(t, h, http.MethodPost, atlasActivePath, `{"world":"nope"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("POST несуществующего мира: status = %d, want 400", rr.Code)
	}
}

func TestAtlasReplay(t *testing.T) {
	dir := setupAtlas(t)
	seedWorld(t, dir, "alpha-world", "Альфа-мир")

	// Переигровка: тот же слаг, новые синопсис и герой; пачка не пересоздаётся.
	body := strings.Replace(atlasAncketaJSON("alpha-world"),
		`"synopsis":"Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё."`,
		`"synopsis":"Новый герой приходит в забытый мир. Он меняет его судьбу. Финал иной."`, 1)
	body = strings.Replace(body, `"hero_birth":"простолюдин"`, `"hero_birth":"аристократ"`, 1)

	h := NewHandler(Config{})
	rr := atlasRequest(t, h, http.MethodPost, atlasReplayPath, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST %s: status = %d, want 200; body=%s", atlasReplayPath, rr.Code, rr.Body.String())
	}

	// Повторного создания баз нет: карточек по-прежнему 96, сессия одна.
	if got := worldCharacters(t, dir, "alpha-world"); got != 96 {
		t.Fatalf("карточек после переигровки: %d, want 96", got)
	}
	d, err := db.OpenReader(filepath.Join(dir, "worlds", "alpha-world", "alpha-world.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var sessions int
	if err := d.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 {
		t.Fatalf("сессий после переигровки: %d, want 1", sessions)
	}
	var pastHero int
	if err := d.QueryRow(`SELECT COUNT(*) FROM events WHERE kind = 'past_hero'`).Scan(&pastHero); err != nil {
		t.Fatal(err)
	}
	if pastHero == 0 {
		t.Fatal("событие past_hero не записано при переигровке")
	}

	// Переигровка мира, которого нет, — 404.
	rr = atlasRequest(t, h, http.MethodPost, atlasReplayPath, atlasAncketaJSON("nope"))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("POST переигровки несуществующего мира: status = %d, want 404", rr.Code)
	}
}

func TestAtlasDelete(t *testing.T) {
	dir := setupAtlas(t)
	seedWorld(t, dir, "alpha-world", "Альфа-мир")

	logPath := filepath.Join(dir, "station.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatal(err)
	}

	h := NewHandler(Config{Logger: logger})
	worldDir := filepath.Join(dir, "worlds", "alpha-world")

	// Без подтверждения каталог не удаляется.
	rr := atlasRequest(t, h, http.MethodDelete, atlasDeletePath+"?world=alpha-world", "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("DELETE без подтверждения: status = %d, want 400", rr.Code)
	}
	if _, err := os.Stat(worldDir); err != nil {
		t.Fatalf("мир удалён без подтверждения: %v", err)
	}

	// С подтверждением мир удаляется целиком и пишется событие world.deleted.
	rr = atlasRequest(t, h, http.MethodDelete, atlasDeletePath+"?world=alpha-world&confirm=alpha-world", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE с подтверждением: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(worldDir); !os.IsNotExist(err) {
		t.Fatalf("каталог мира не удалён: %v", err)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), `"world.deleted"`) {
		t.Fatalf("журнал не содержит world.deleted: %s", logData)
	}
}
