package workshop_test

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"tns65/internal/db"
	"tns65/internal/testkit"
	"tns65/internal/workshop"
)

func TestHoroscopeMatch(t *testing.T) {
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	litPath := filepath.Join(dir, "literary.db")

	newDB(t, worldPath, "world", "shared")
	newDB(t, litPath, "literary")

	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	defer lit.Close()

	sceneID := insertScene(t, world, "open")
	insertHoroscopeFixture(t, world, lit, sceneID)

	slot := testkit.NewFakeSlot()
	workshop.SetSlot(slot)
	defer workshop.SetSlot(nil)

	cfg := workshop.DefaultHoroscopeMatchConfig()

	before := dbSnapshot(t, world) + dbSnapshot(t, lit)

	// Реплика в прогнозе (косинус ≥ 0.75) отдаёт готовую ветку мгновенно,
	// без вызова слота генерации.
	m, err := workshop.MatchHoroscope(world, lit, sceneID, [][]float32{earlyVector(0.75)}, cfg)
	if err != nil {
		t.Fatalf("MatchHoroscope (прогноз): %v", err)
	}
	if m == nil {
		t.Fatal("реплика в прогнозе не отдала готовую ветку")
	}
	if m.Branch.ID != "pos" {
		t.Fatalf("ветка = %q, want pos (положительная — промпт Ткача)", m.Branch.ID)
	}
	if m.Branch.Kind != "weaver_prompt" {
		t.Fatalf("kind ветки = %q, want weaver_prompt", m.Branch.Kind)
	}
	if slot.Calls() != 0 {
		t.Fatalf("слот вызван %d раз, want 0", slot.Calls())
	}

	// Реплика с косинусом 0.7 ветку не выдаёт (промах не форсируется).
	m, err = workshop.MatchHoroscope(world, lit, sceneID, [][]float32{earlyVector(0.7)}, cfg)
	if err != nil {
		t.Fatalf("MatchHoroscope (промах): %v", err)
	}
	if m != nil {
		t.Fatalf("реплика с косинусом 0.7 выдала ветку: %+v", m)
	}

	// Матчер — только механика: ни одна таблица обеих баз не изменилась.
	after := dbSnapshot(t, world) + dbSnapshot(t, lit)
	if after != before {
		t.Fatal("матчер изменил таблицы world(a).db или literary.db")
	}
}

// insertHoroscopeFixture пишет гороскоп и векторы веток прямо в базы, чтобы
// задать точные косинусы: положительная ветка — ось 0, отрицательная — ось 1.
func insertHoroscopeFixture(t *testing.T, world, lit *db.Handle, sceneID int64) int64 {
	t.Helper()
	issue := workshop.HoroscopeOutput{
		Branches: []workshop.HoroscopeBranch{
			{ID: "neg", Kind: "bank_ref", Ref: "bank://12", Theme: "опасность", ProbeText: "герой отступает"},
			{ID: "pos", Kind: "weaver_prompt", Ref: "продолжить тему", Theme: "надежда", ProbeText: "герой идёт вперёд"},
		},
		Forecast: "echo",
	}
	raw, err := json.Marshal(issue)
	if err != nil {
		t.Fatalf("marshal issue: %v", err)
	}
	res, err := lit.Exec("horoscope_issue",
		`INSERT INTO horoscope_issue (scene_id, issue_json, created_at) VALUES (?, ?, ?)`,
		sceneID, string(raw), "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("INSERT horoscope_issue: %v", err)
	}
	issueID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("horoscope_issue id: %v", err)
	}

	vecByProbe := map[string][]float32{
		"герой отступает":   axisVector(1024, 1),
		"герой идёт вперёд": axisVector(1024, 0),
	}
	for _, b := range issue.Branches {
		if _, err := world.Exec("vectors",
			`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			"horoscope_issue", issueID, "bge-m3", 1024, "ru", b.ProbeText, vecBlob(vecByProbe[b.ProbeText]), "2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("INSERT vectors: %v", err)
		}
	}
	return issueID
}

// earlyVector строит нормированный вектор [t, sqrt(1-t²), 0…]: его косинус
// с осью 0 равен t, с осью 1 равен sqrt(1-t²).
func earlyVector(t float64) []float32 {
	v := make([]float32, 1024)
	v[0] = float32(t)
	v[1] = float32(math.Sqrt(1 - t*t))
	return v
}

func axisVector(dim, axis int) []float32 {
	v := make([]float32, dim)
	v[axis] = 1
	return v
}

func vecBlob(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, f := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// dbSnapshot снимает детерминированный отпечаток всех таблиц базы (имя и все
// строки), чтобы проверить, что матчер ничего не записал.
func dbSnapshot(t *testing.T, h *db.Handle) string {
	t.Helper()
	rows, err := h.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatalf("scan table name: %v", err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("rows tables: %v", err)
	}

	hsh := sha256.New()
	for _, name := range tables {
		fmt.Fprintf(hsh, "table:%s\n", name)
		r, err := h.Query(`SELECT * FROM "` + name + `" ORDER BY rowid`)
		if err != nil {
			t.Fatalf("select %s: %v", name, err)
		}
		cols, err := r.Columns()
		if err != nil {
			r.Close()
			t.Fatalf("columns %s: %v", name, err)
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		for r.Next() {
			if err := r.Scan(ptrs...); err != nil {
				r.Close()
				t.Fatalf("scan row %s: %v", name, err)
			}
			for _, v := range vals {
				fmt.Fprintf(hsh, "%v|", v)
			}
			fmt.Fprintln(hsh)
		}
		if err := r.Err(); err != nil {
			r.Close()
			t.Fatalf("rows %s: %v", name, err)
		}
		r.Close()
	}
	return hex.EncodeToString(hsh.Sum(nil))
}
