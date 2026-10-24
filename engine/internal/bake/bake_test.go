package bake

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"tns65/internal/db"
)

// defaultVerses — нейтральные стихи сида: одна строка — один стих.
const defaultVerses = "Юношу предают близкие и продают в чужую землю.\n" +
	"В беде он остаётся верным и обретает доверие господина.\n" +
	"Несправедливое обвинение приводит его в темницу.\n" +
	"Толкование сна возносит его к власти и примирению с семьёй.\n" +
	"После долгой разлуки герой пускается в обратный путь.\n" +
	"Препятствия и испытания проверяют его решимость.\n" +
	"Дома его узнают и возвращают ему прежнее место.\n"

func TestBakeFreezes(t *testing.T) {
	// Шаг 1: пустой сид (нет нарративов) — отказ с причиной.
	t.Run("no narratives", func(t *testing.T) {
		seed := t.TempDir()
		if err := os.WriteFile(filepath.Join(seed, "verses.txt"), []byte("стих\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(seed, "narratives"), 0o755); err != nil {
			t.Fatal(err)
		}
		err := Bake(Config{DBPath: filepath.Join(t.TempDir(), "base.db"), SeedDir: seed, EmbedURL: "http://127.0.0.1:1"})
		if !errors.Is(err, ErrNoNarratives) {
			t.Fatalf("err = %v, want ErrNoNarratives", err)
		}
	})

	// Шаг 1: у нарратива ровно одно резюме дуги.
	t.Run("two arc summaries", func(t *testing.T) {
		seed := writeSeedDir(t, "стих\n", map[string]string{
			"a.txt": "name: А\narc_summary: один\narc_summary: два\nverses: 1\n",
		})
		err := Bake(Config{DBPath: filepath.Join(t.TempDir(), "base.db"), SeedDir: seed, EmbedURL: "http://127.0.0.1:1"})
		if err == nil || !strings.Contains(err.Error(), "arc_summary") {
			t.Fatalf("err = %v, want arc_summary error", err)
		}
	})

	srv := newFakeEmbedder()
	defer srv.Close()

	seed := defaultSeedDir(t)
	outDir := t.TempDir()
	dbPath := filepath.Join(outDir, "base.db")

	if err := Bake(Config{DBPath: dbPath, SeedDir: seed, EmbedURL: srv.URL}); err != nil {
		t.Fatalf("Bake: %v", err)
	}

	// Заморозка: рядом лежит base.db.sha256, сумма совпадает с файлом.
	sum, err := os.ReadFile(dbPath + ".sha256")
	if err != nil {
		t.Fatalf("read base.db.sha256: %v", err)
	}
	if got := strings.TrimSpace(string(sum)); got != sha256Hex(t, dbPath) {
		t.Fatalf("base.db.sha256 = %q, want %q", got, sha256Hex(t, dbPath))
	}

	// Чтение замороженной базы: у каждого нарратива ровно один вектор dim=1024.
	r, err := db.OpenReader(dbPath)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer r.Close()

	var nNarr int
	if err := r.QueryRow(`SELECT COUNT(*) FROM narratives`).Scan(&nNarr); err != nil {
		t.Fatal(err)
	}
	if nNarr != 2 {
		t.Fatalf("narratives = %d, want 2", nNarr)
	}

	var nVec int
	if err := r.QueryRow(`SELECT COUNT(*) FROM vectors WHERE owner_table = 'narratives'`).Scan(&nVec); err != nil {
		t.Fatal(err)
	}
	if nVec != nNarr {
		t.Fatalf("vectors = %d, want ровно по одному на нарратив (%d)", nVec, nNarr)
	}

	rows, err := r.Query(`SELECT owner_id, dim, length(vec) FROM vectors WHERE owner_table = 'narratives' ORDER BY owner_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[int64]bool{}
	for rows.Next() {
		var ownerID int64
		var dim, blen int
		if err := rows.Scan(&ownerID, &dim, &blen); err != nil {
			t.Fatal(err)
		}
		if seen[ownerID] {
			t.Fatalf("нарратив %d имеет больше одного вектора", ownerID)
		}
		seen[ownerID] = true
		if dim != embeddingDim {
			t.Errorf("vector owner_id=%d dim = %d, want %d", ownerID, dim, embeddingDim)
		}
		if blen != embeddingDim*4 {
			t.Errorf("vector owner_id=%d blob len = %d, want %d", ownerID, blen, embeddingDim*4)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	// После заморозки запись в narratives через хэндл чтения даёт ошибку.
	_, err = r.Exec(`INSERT INTO narratives (name, arc_summary, lang, created_at) VALUES ('x', 'y', 'ru', '2026-01-01T00:00:00Z')`)
	if err == nil {
		t.Fatal("INSERT narratives на read-only хэндле не дал ошибку")
	}
	if !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("INSERT narratives error = %q, want containing %q", err.Error(), "readonly")
	}
}

func TestBakeIdempotent(t *testing.T) {
	bin := buildTns65Bake(t)

	srv := newFakeEmbedder()
	defer srv.Close()

	seed := defaultSeedDir(t)
	dbPath := filepath.Join(t.TempDir(), "base.db")
	args := []string{"-db", dbPath, "-seed", seed, "-embed-url", srv.URL}

	if out, err := exec.Command(bin, args...).CombinedOutput(); err != nil {
		t.Fatalf("первый запуск: %v\n%s", err, out)
	}

	out, err := exec.Command(bin, args...).CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("повторный запуск: err = %v, want ExitError с кодом 5", err)
	}
	if code := ee.ExitCode(); code != 5 {
		t.Fatalf("повторный запуск: код = %d, want 5\n%s", code, out)
	}
	if !strings.Contains(string(out), "сид уже запечён") {
		t.Fatalf("повторный запуск: вывод = %q, want содержит «сид уже запечён»", out)
	}
}

// newFakeEmbedder — подставной эмбеддер на httptest.Server: отвечает вектором
// из 1024 чисел; живой :5002 тесту не нужен (T-01-16, шаг 2).
func newFakeEmbedder() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		vec := make([]float64, embeddingDim)
		for i := range vec {
			vec[i] = float64(i%7) / 7.0
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": vec})
	}))
}

// writeSeedDir пишет каталог сида: verses.txt и файлы narratives/.
func writeSeedDir(t *testing.T, verses string, narratives map[string]string) string {
	t.Helper()
	seed := t.TempDir()
	if err := os.WriteFile(filepath.Join(seed, "verses.txt"), []byte(verses), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(seed, "narratives"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range narratives {
		if err := os.WriteFile(filepath.Join(seed, "narratives", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return seed
}

func defaultSeedDir(t *testing.T) string {
	t.Helper()
	return writeSeedDir(t, defaultVerses, map[string]string{
		"ascent.txt":     "name: Восхождение из рабства\narc_summary: Герой проходит путь от раба до второго человека в государстве.\nverses: 1 2 3 4\n",
		"homecoming.txt": "name: Возвращение домой\narc_summary: Скиталец после долгих лет возвращается домой и возвращает себе утраченное.\nverses: 5 6 7\n",
	})
}

func sha256Hex(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// buildTns65Bake собирает настоящий бинарник cmd/tns65-bake: проверяется путь
// кода 5 из main.go, а не мок Bake.
func buildTns65Bake(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "tns65-bake")
	cmd := exec.Command("go", "build", "-o", bin, "tns65/cmd/tns65-bake")
	cmd.Dir = engineRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build cmd/tns65-bake: %v\n%s", err, out)
	}
	return bin
}

func engineRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}").Output()
	if err != nil {
		t.Fatalf("go list -m: %v", err)
	}
	return strings.TrimSpace(string(out))
}
