package netgate

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/journal"
)

func readJournalLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func TestDeny(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	cfg := filepath.Join(dir, "config.yaml")
	body := "netgate:\n  allowed_hosts:\n    - allowed.example\n"
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := Load(cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	SetLogger(logger)

	client := Client("outside.example")
	resp, err := client.Get("https://outside.example/")
	if resp != nil {
		resp.Body.Close()
		t.Fatalf("resp = %v, want nil", resp)
	}
	if !errors.Is(err, ErrDeniedHost) {
		t.Fatalf("err = %v, want ErrDeniedHost", err)
	}

	found := false
	for _, line := range readJournalLines(t, logPath) {
		if strings.Contains(line, "netgate.deny") && strings.Contains(line, "outside.example") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("в журнале нет строки netgate.deny с хостом outside.example:\n%s",
			strings.Join(readJournalLines(t, logPath), "\n"))
	}
}

func TestLocalHook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "http://")
	client := Local(addr)

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestNoRedirect(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "journal.log")
	logger, err := journal.New(logPath)
	if err != nil {
		t.Fatalf("journal.New: %v", err)
	}

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://outside.example/", http.StatusFound)
	}))
	defer target.Close()

	hs := newHostSet()
	hs.add("127.0.0.1/32")
	g := &gate{allowed: hs, logger: logger}

	client := g.client(target.Listener.Addr().String(), "test")
	resp, err := client.Get(target.URL)
	if resp != nil {
		resp.Body.Close()
	}
	if !errors.Is(err, ErrDeniedHost) {
		t.Fatalf("err = %v, want ErrDeniedHost (redirect not followed)", err)
	}

	found := false
	for _, line := range readJournalLines(t, logPath) {
		if strings.Contains(line, "netgate.deny") && strings.Contains(line, "outside.example") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("в журнале нет строки netgate.deny с хостом outside.example:\n%s",
			strings.Join(readJournalLines(t, logPath), "\n"))
	}
}

func TestOnlyGateDials(t *testing.T) {
	root := engineRoot(t)
	netgateDir := filepath.Join(root, "internal", "netgate") + string(filepath.Separator)

	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "bin" || d.Name() == "state" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasPrefix(path, netgateDir) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		for _, call := range []string{"http.Get(", "http.Post(", "net.Dial("} {
			if strings.Contains(content, call) {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, fmt.Sprintf("%s: %s", rel, call))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("прямые вызовы вне netgate:\n%s", strings.Join(offenders, "\n"))
	}
}

// engineRoot возвращает корень модуля engine (каталог с go.mod).
func engineRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}").Output()
	if err != nil {
		t.Fatalf("go list -m: %v", err)
	}
	return strings.TrimSpace(string(out))
}
