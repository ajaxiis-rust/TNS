package webapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"tns65/internal/netgate"
	web "tns65/web"
)

func newTestServer() *httptest.Server {
	return httptest.NewServer(NewHandler(Config{}))
}

// testGet выполняет GET к loopback-серверу через netgate.Local — единственный
// разрешённый путь HTTP в коде вне netgate (T-01-11).
func testGet(t *testing.T, baseURL, path string) *http.Response {
	t.Helper()
	addr := strings.TrimPrefix(baseURL, "http://")
	resp, err := netgate.Local(addr).Get(baseURL + path)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestServeStatic(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	wantIndex, err := web.FS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	res := testGet(t, srv.URL, "/")
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /: status = %d, want 200", res.StatusCode)
	}
	if string(body) != string(wantIndex) {
		t.Fatal("GET /: body not served from embed")
	}

	wantJS, err := web.FS.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	res = testGet(t, srv.URL, "/static/app.js")
	body, err = io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /static/app.js: status = %d, want 200", res.StatusCode)
	}
	if string(body) != string(wantJS) {
		t.Fatal("GET /static/app.js: body not served from embed")
	}
}

func TestHealth(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	res := testGet(t, srv.URL, "/api/health")
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/health: status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("GET /api/health: Content-Type = %q, want application/json", ct)
	}
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("GET /api/health: bad JSON: %v", err)
	}
	if got["status"] != "ok" {
		t.Fatalf("GET /api/health: status = %q, want ok", got["status"])
	}

	// Единый контракт ошибок: 404 с телом {"error":{"code":…,"message":…}}.
	res = testGet(t, srv.URL, "/api/nope")
	body, err = io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/nope: status = %d, want 404", res.StatusCode)
	}
	var eb errorBody
	if err := json.Unmarshal(body, &eb); err != nil {
		t.Fatalf("GET /api/nope: bad error JSON: %v", err)
	}
	if eb.Error.Code != codeNotFound {
		t.Fatalf("GET /api/nope: error code = %q, want %q", eb.Error.Code, codeNotFound)
	}
}

func TestLoopbackOnly(t *testing.T) {
	if _, err := Listen("0.0.0.0", 0); err == nil {
		t.Fatal("Listen(0.0.0.0) accepted, want rejection")
	}
	if _, err := Listen("example.com", 0); err == nil {
		t.Fatal("Listen(example.com) accepted, want rejection")
	}
	ln, err := Listen("127.0.0.1", 0)
	if err != nil {
		t.Fatalf("Listen(127.0.0.1): %v", err)
	}
	_ = ln.Close()
}

func TestRouteTable(t *testing.T) {
	data, err := os.ReadFile("ROUTES.md")
	if err != nil {
		t.Fatal(err)
	}
	want := parseRouteTable(string(data))
	got := routeLines(NewHub())
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("route table mismatch:\n got %v\nwant %v", got, want)
	}
}

// parseRouteTable извлекает строки "METHOD PATH" из fenced-блока ```routes в
// ROUTES.md.
func parseRouteTable(md string) []string {
	const marker = "```routes"
	start := strings.Index(md, marker)
	if start < 0 {
		return nil
	}
	start += len(marker)
	end := strings.Index(md[start:], "```")
	if end < 0 {
		return nil
	}
	block := md[start : start+end]
	lines := strings.Split(strings.TrimSpace(block), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
