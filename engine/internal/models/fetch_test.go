package models

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testBody — детерминированное «тело модели» заданного размера.
func testBody(size int) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = byte(i*31 + 7)
	}
	return b
}

func shaHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// fetchServer — httptest-подобие CDN HF: отдаёт тело по Range, умеет не
// поддерживать Range и обрывать соединение на середине ответа.
type fetchServer struct {
	body        []byte
	ignoreRange bool
	dropAfter   int64 // байт до обрыва; 0 — без обрыва
	drops       atomic.Int64

	mu     sync.Mutex
	ranges []string
}

func (s *fetchServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hdr := r.Header.Get("Range")
	s.mu.Lock()
	s.ranges = append(s.ranges, hdr)
	s.mu.Unlock()

	start, end, ok := parseRangeHeader(hdr, int64(len(s.body)))
	if s.ignoreRange || !ok {
		w.Header().Set("Content-Length", strconv.Itoa(len(s.body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(s.body)
		return
	}
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(s.body)))
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	w.WriteHeader(http.StatusPartialContent)
	chunk := s.body[start : end+1]
	// Проба (bytes=0-0) не расходует обрыв: он нужен на отрезке.
	if s.dropAfter > 0 && hdr != "bytes=0-0" && s.drops.Add(-1) >= 0 {
		if int64(len(chunk)) > s.dropAfter {
			chunk = chunk[:s.dropAfter]
		}
		_, _ = w.Write(chunk)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		panic(http.ErrAbortHandler)
	}
	_, _ = w.Write(chunk)
}

func (s *fetchServer) rangeHeaders() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ranges...)
}

// parseRangeHeader разбирает "bytes=start-end" для тела size.
func parseRangeHeader(h string, size int64) (start, end int64, ok bool) {
	if !strings.HasPrefix(h, "bytes=") {
		return 0, 0, false
	}
	spec := strings.TrimPrefix(h, "bytes=")
	dash := strings.Index(spec, "-")
	if dash < 0 {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(spec[:dash]), 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	end = size - 1
	if s := strings.TrimSpace(spec[dash+1:]); s != "" {
		end, err = strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, 0, false
		}
	}
	if end >= size {
		end = size - 1
	}
	if end < start {
		return 0, 0, false
	}
	return start, end, true
}

// startFetch поднимает сервер и качает тело в TempDir с короткой паузой ретраев.
func startFetch(t *testing.T, fs *fetchServer, workers int, progress func(DownloadProgress)) (DownloadResult, string, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(fs)
	t.Cleanup(srv.Close)
	dest := filepath.Join(t.TempDir(), "bge-m3-Q4_K_M.gguf")
	res, err := DownloadFile(context.Background(), DownloadSpec{
		URL:        srv.URL + "/model.gguf",
		Dest:       dest,
		Workers:    workers,
		Client:     srv.Client(),
		Progress:   progress,
		RetryPause: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	return res, dest, srv
}

// Проверка T-13-01: сервер с Range — параллельные отрезки, слияние в файл,
// sha256 и удаление частичных файлов.
func TestFetchRangedParallel(t *testing.T) {
	body := testBody(300 << 10)
	fs := &fetchServer{body: body}
	var last DownloadProgress
	calls := 0
	res, dest, _ := startFetch(t, fs, 4, func(p DownloadProgress) {
		calls++
		last = p
	})

	if res.Workers != 4 {
		t.Fatalf("воркеров: %d, ждали 4", res.Workers)
	}
	if res.Size != int64(len(body)) {
		t.Fatalf("размер: %d, ждали %d", res.Size, len(body))
	}
	if res.SHA256 != shaHex(body) {
		t.Fatalf("sha256: %s, ждали %s", res.SHA256, shaHex(body))
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("чтение файла: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("содержимое файла не совпало: %d байт", len(got))
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Fatalf("каталог отрезков не удалён: %v", err)
	}
	if calls == 0 || last.Done != int64(len(body)) || last.Total != int64(len(body)) {
		t.Fatalf("прогресс: вызовов %d, последний %+v", calls, last)
	}
	// Проба Range + по запросу на каждый из четырёх отрезков.
	headers := fs.rangeHeaders()
	if len(headers) != 5 {
		t.Fatalf("запросов с Range: %d (%v), ждали 5", len(headers), headers)
	}
	for _, h := range headers {
		if !strings.HasPrefix(h, "bytes=") {
			t.Fatalf("запрос без Range: %q", h)
		}
	}
}

// Проверка T-13-01: сервер без поддержки Range — один поток, файл целиком.
func TestFetchNoRange(t *testing.T) {
	body := testBody(200 << 10)
	fs := &fetchServer{body: body, ignoreRange: true}
	res, dest, _ := startFetch(t, fs, 0, nil)

	if res.Workers != 1 {
		t.Fatalf("воркеров: %d, ждали 1", res.Workers)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("чтение файла: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("содержимое файла не совпало: %d байт", len(got))
	}
	if res.SHA256 != shaHex(body) {
		t.Fatalf("sha256: %s, ждали %s", res.SHA256, shaHex(body))
	}
	plain := 0
	for _, h := range fs.rangeHeaders() {
		if h == "" {
			plain++
		}
	}
	if plain == 0 {
		t.Fatal("не было запроса без Range: сервер без Range должен качаться одним потоком")
	}
}

// Проверка T-13-01: докачка продолжается с размера частичного файла отрезка.
func TestFetchResumeFromPart(t *testing.T) {
	body := testBody(200 << 10)
	fs := &fetchServer{body: body}
	segs := splitSegments(int64(len(body)), 2)
	if len(segs) != 2 {
		t.Fatalf("отрезков: %d, ждали 2", len(segs))
	}
	destDir := t.TempDir()
	dest := filepath.Join(destDir, "model.gguf")
	partDir := dest + ".part"
	if err := os.MkdirAll(partDir, 0o755); err != nil {
		t.Fatalf("каталог отрезков: %v", err)
	}
	done := int64(1000)
	seg := segs[1]
	if err := os.WriteFile(filepath.Join(partDir, strconv.Itoa(seg.index)), body[seg.start:seg.start+done], 0o644); err != nil {
		t.Fatalf("частичный файл: %v", err)
	}

	srv := httptest.NewServer(fs)
	defer srv.Close()
	res, err := DownloadFile(context.Background(), DownloadSpec{
		URL:        srv.URL + "/model.gguf",
		Dest:       dest,
		Workers:    2,
		Client:     srv.Client(),
		RetryPause: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	if res.Retries != 0 {
		t.Fatalf("ретраев без обрыва: %d", res.Retries)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("чтение файла: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatal("содержимое файла после докачки не совпало")
	}
	// Докачка обязана запросить отрезок с позиции размера части.
	want := fmt.Sprintf("bytes=%d-%d", seg.start+done, seg.end)
	found := false
	for _, h := range fs.rangeHeaders() {
		if h == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("нет запроса %q среди %v", want, fs.rangeHeaders())
	}
}

// Проверка T-13-01: обрыв соединения на середине отрезка — повтор с места
// обрыва, файл собирается целиком.
func TestFetchDropMidSegment(t *testing.T) {
	body := testBody(200 << 10)
	fs := &fetchServer{body: body, dropAfter: 1000}
	fs.drops.Store(1)
	res, dest, _ := startFetch(t, fs, 2, nil)

	if res.Retries < 1 {
		t.Fatalf("ретраев после обрыва: %d", res.Retries)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("чтение файла: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("содержимое файла после обрыва не совпало: %d байт", len(got))
	}
	if res.SHA256 != shaHex(body) {
		t.Fatalf("sha256: %s, ждали %s", res.SHA256, shaHex(body))
	}
}

// Проверка T-13-01: ретраи исчерпаны — ошибка, а не зависание.
func TestFetchRetriesExhausted(t *testing.T) {
	fs := &fetchServer{body: testBody(100 << 10), dropAfter: 100}
	fs.drops.Store(100)
	srv := httptest.NewServer(fs)
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "model.gguf")

	_, err := DownloadFile(context.Background(), DownloadSpec{
		URL:        srv.URL + "/model.gguf",
		Dest:       dest,
		Workers:    1,
		Client:     srv.Client(),
		RetryPause: time.Millisecond,
	})
	if err == nil {
		t.Fatal("ждали ошибку после исчерпания ретраев")
	}
	// Проба + 3 попытки единственного отрезка.
	if n := len(fs.rangeHeaders()); n != 4 {
		t.Fatalf("запросов: %d, ждали 4: %v", n, fs.rangeHeaders())
	}
}
