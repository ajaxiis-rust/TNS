package workshop_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/birth"
	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

func TestBirthOrders(t *testing.T) {
	dir := t.TempDir()

	res, err := birth.Birth(validAncketa(), birth.Config{WorldsRoot: dir, BaseDBPath: testBaseDB(t, dir), Embed: testkit.NewFakeEmbed()})
	if err != nil {
		t.Fatalf("Birth: %v", err)
	}

	worldDB := openReader(t, res.WorldDB)

	// После рождения в очереди ровно два заказа: analysis и forge_plan.
	type order struct {
		kind   string
		status string
	}
	var orders []order
	rows, err := worldDB.Query(`SELECT kind, status FROM workshop_queue ORDER BY kind`)
	if err != nil {
		t.Fatalf("workshop_queue query: %v", err)
	}
	for rows.Next() {
		var o order
		if err := rows.Scan(&o.kind, &o.status); err != nil {
			t.Fatalf("workshop_queue scan: %v", err)
		}
		orders = append(orders, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("workshop_queue rows: %v", err)
	}

	if len(orders) != 2 {
		t.Fatalf("заказов в очереди: %d, want 2", len(orders))
	}
	want := []order{{"analysis", "queued"}, {"forge_plan", "queued"}}
	for i := range want {
		if orders[i] != want[i] {
			t.Errorf("заказ %d = %+v, want %+v", i, orders[i], want[i])
		}
	}

	// При пресете local оба привязаны к локальному слоту (:5001).
	type binding struct {
		role     string
		modelRef string
		provider string
	}
	var bindings []binding
	brows, err := worldDB.Query(`SELECT role, model_ref, provider FROM model_bindings WHERE role IN ('analysis','forge_plan') ORDER BY role`)
	if err != nil {
		t.Fatalf("model_bindings query: %v", err)
	}
	for brows.Next() {
		var b binding
		if err := brows.Scan(&b.role, &b.modelRef, &b.provider); err != nil {
			t.Fatalf("model_bindings scan: %v", err)
		}
		bindings = append(bindings, b)
	}
	brows.Close()

	wantBindings := []binding{
		{"analysis", ":5001", "local"},
		{"forge_plan", ":5001", "local"},
	}
	if len(bindings) != len(wantBindings) {
		t.Fatalf("привязок к слоту: %d, want %d", len(bindings), len(wantBindings))
	}
	for i := range wantBindings {
		if bindings[i] != wantBindings[i] {
			t.Errorf("привязка %d = %+v, want %+v", i, bindings[i], wantBindings[i])
		}
	}
}

func TestQueueKinds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "world(a).db")

	w, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	defer w.Close()
	if err := db.Migrate(w, "world", "shared"); err != nil {
		t.Fatalf("Migrate(world, shared): %v", err)
	}

	// Запись через хэндл core: вид вне реестра отвергается CHECK таблицы.
	h, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs: %v", err)
	}
	defer h.Close()

	_, err = h.Exec("workshop_queue",
		`INSERT INTO workshop_queue (kind, payload_json, priority, status, step, resumable, attempts, created_at) VALUES ('poem', '{}', 50, 'queued', 'light', 0, 0, '2026-01-01T00:00:00Z')`)
	if err == nil {
		t.Fatal("вид заказа вне реестра не отвергнут CHECK")
	}
	if !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("ошибка не упоминает CHECK: %v", err)
	}

	// Всякий вид из реестра Kinds принимается CHECK-ом.
	for _, k := range workshop.Kinds {
		if _, err := h.Exec("workshop_queue",
			`INSERT INTO workshop_queue (kind, payload_json, priority, status, step, resumable, attempts, created_at) VALUES (?, '{}', 50, 'queued', 'light', 0, 0, '2026-01-01T00:00:00Z')`,
			string(k)); err != nil {
			t.Errorf("вид %q из реестра отвергнут CHECK: %v", k, err)
		}
	}
}

// testBaseDB — фикстурная base.db с одним нарративом и вектором dim=1024
// (T-02-07): рождение в юнит-тестах не зависит от запечённого сида T-01-16.
func testBaseDB(t *testing.T, dir string) string {
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
	res, err := w.Exec(`INSERT INTO narratives (name, arc_summary, lang, created_at) VALUES (?,?,?,?)`,
		"fixture", "arc", "ru", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec(`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		"narratives", id, "bge-m3", 1024, "ru", "arc", make([]byte, 1024*4), "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	return path
}

func validAncketa() *birth.Ancketa {
	return &birth.Ancketa{
		Slug:        "sumerechnyj-mir",
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

func openReader(t *testing.T, path string) *sql.DB {
	t.Helper()
	d, err := db.OpenReader(path)
	if err != nil {
		t.Fatalf("OpenReader(%s): %v", path, err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}
