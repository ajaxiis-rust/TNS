package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
)

func TestNoWorldWriteRoutes(t *testing.T) {
	specs := routeSpecs(NewHub())
	writeMethods := map[string]bool{
		http.MethodPost:   true,
		http.MethodPut:    true,
		http.MethodDelete: true,
	}

	// Замороженные поля анкеты не имеют маршрутов записи.
	for _, field := range frozenWorldFields {
		if got := writeRouteForWorldField(field); got != "" {
			t.Fatalf("frozen field %q has write route %q", field, got)
		}
	}

	// Настраиваемые параметры пишутся только /api/world/tunables.
	for _, field := range tunableFields {
		if got := writeRouteForWorldField(field); got != worldTunablesPath {
			t.Fatalf("tunable %q write route = %q, want %q", field, got, worldTunablesPath)
		}
	}

	hasRead := false
	hasTunables := false
	for _, s := range specs {
		if s.method == http.MethodGet && s.path == worldFieldsPath {
			hasRead = true
		}
		if s.method == http.MethodPut && s.path == worldTunablesPath {
			hasTunables = true
		}
		if !writeMethods[s.method] {
			continue
		}
		if s.path == worldTunablesPath {
			continue
		}
		if s.path == worldFieldsPath || strings.HasPrefix(s.path, worldFieldsPath+"/") {
			t.Fatalf("write route %s %s mutates world fields", s.method, s.path)
		}
	}
	if !hasRead {
		t.Fatal("no GET /api/world read route")
	}
	if !hasTunables {
		t.Fatal("no PUT /api/world/tunables write route")
	}
}

func TestTunablesRoute(t *testing.T) {
	dbPath := newWorldDB(t)
	oldPath := worldDBPath
	worldDBPath = func(string) string { return dbPath }
	defer func() { worldDBPath = oldPath }()

	h := NewHandler(Config{})

	body := `{"world":"test","tunables":{"globalLuck":0.5,"dayNormKm":30}}`
	req := httptest.NewRequest(http.MethodPut, worldTunablesPath, strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("PUT /api/world/tunables: status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	got := readWorldMetaValues(t, dbPath)
	if got["globalLuck"] != "0.5" {
		t.Fatalf("world_meta globalLuck = %q, want 0.5", got["globalLuck"])
	}
	if got["dayNormKm"] != "30" {
		t.Fatalf("world_meta dayNormKm = %q, want 30", got["dayNormKm"])
	}

	// world_meta пишет только forge: хэндл core отклоняется картой писателей.
	core, err := db.OpenWriterAs(dbPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	err = writeWorldTunables(core, map[string]json.RawMessage{"globalLuck": json.RawMessage("0.6")})
	if !errors.Is(err, db.ErrForeignWriter) {
		t.Fatalf("writeWorldTunables(core): err = %v, want ErrForeignWriter", err)
	}

	// Замороженное поле не является настраиваемым параметром и отвергается.
	bad := `{"world":"test","tunables":{"description":"x"}}`
	req = httptest.NewRequest(http.MethodPut, worldTunablesPath, strings.NewReader(bad))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("PUT /api/world/tunables with frozen field: status = %d, want 400", rr.Code)
	}
}

func newWorldDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "world.db")
	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(raw, "world", "shared"); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func readWorldMetaValues(t *testing.T, path string) map[string]string {
	t.Helper()
	d, err := db.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	rows, err := d.Query(`SELECT "key", value FROM world_meta`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
