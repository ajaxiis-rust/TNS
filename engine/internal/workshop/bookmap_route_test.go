package workshop_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"tns65/internal/db"
	"tns65/internal/deepread"
	"tns65/internal/providercfg"
	"tns65/internal/providerroute"
	"tns65/internal/workshop"
)

// bookMapAnswer — ответ картографа: текст карты книги-источника.
const bookMapAnswer = "Москва — столица; путь на восток."

// bookMapProvider — подставной провайдер маршрута карты: считает вызовы и
// запоминает последнее тело запроса.
type bookMapProvider struct {
	calls atomic.Int32

	mu   sync.Mutex
	body string
}

func (p *bookMapProvider) requestBody() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.body
}

// newBookMapProvider поднимает OpenAI-совместимый httptest-провайдер: status 0 —
// 200 с одним SSE-чанком, иначе HTTP-ошибка.
func newBookMapProvider(t *testing.T, text string, status int) (*httptest.Server, *bookMapProvider) {
	t.Helper()
	p := &bookMapProvider{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.calls.Add(1)
		raw, _ := io.ReadAll(r.Body)
		p.mu.Lock()
		p.body = string(raw)
		p.mu.Unlock()
		if status != 0 {
			http.Error(w, "refused", status)
			return
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", text)
	}))
	t.Cleanup(srv.Close)
	return srv, p
}

// bookMapRoleSlot — обёртка слота картографа: помнит роль, которую выбрал
// проход, и число запросов.
type bookMapRoleSlot struct {
	inner deepread.MapSlot

	role    string
	queries int
}

func (s *bookMapRoleSlot) Generate(ctx context.Context, role, prompt string) (string, error) {
	s.role = role
	s.queries++
	return s.inner.Generate(ctx, role, prompt)
}

// bookMapSlot собирает слот картографа на маршруте sources_map: провайдеры —
// два адреса httptest, settings — тело settings.toml с ключами и маршрутами.
func bookMapSlot(t *testing.T, firstURL, secondURL, settingsBody string) *workshop.RoutedWorkshopSlot {
	t.Helper()
	dir := t.TempDir()
	providersPath := filepath.Join(dir, "providers.toml")
	providers := fmt.Sprintf(`
[[providers]]
name = "first"
protocol = "openai"
base_url = %q
default_model = "first-1"

[[providers]]
name = "second"
protocol = "openai"
base_url = %q
default_model = "second-1"
`, firstURL, secondURL)
	if err := os.WriteFile(providersPath, []byte(providers), 0o644); err != nil {
		t.Fatalf("WriteFile(providers.toml): %v", err)
	}
	settingsPath := filepath.Join(dir, "settings.toml")
	if err := os.WriteFile(settingsPath, []byte(settingsBody), 0o644); err != nil {
		t.Fatalf("WriteFile(settings.toml): %v", err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("slots:\n  default: local\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(config.yaml): %v", err)
	}

	cfg, err := providerroute.LoadSettings(configPath, settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	router := providerroute.NewRouter(cfg, providercfg.Load(providersPath), nil)
	return workshop.NewRoutedWorkshopSlot(router.Slot(providerroute.TaskSourcesMap))
}

// newBookMapLiterary создаёт литературную базу с книгой, местом, сценой и
// прозой: проза в промпт карты попадать не должна.
func newBookMapLiterary(t *testing.T) (*db.Handle, int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "literary.db")
	newDB(t, path, "literary")
	lit, err := db.OpenWriterAs(path, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	t.Cleanup(func() { _ = lit.Close() })

	res, err := lit.Exec("book",
		`INSERT INTO book (shelf_id, name, lang, genres, rating, chapters, status, created_at) VALUES ('p13', 'Карта источника', 'ru', '', '', 3, 'ready', '2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("INSERT book: %v", err)
	}
	bookID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("book LastInsertId: %v", err)
	}
	place, err := lit.Exec("book_place",
		`INSERT INTO book_place (book_id, name, aliases, kind, first_chunk, last_chunk, mentions) VALUES (?, 'Москва', '[]', 'city', 1, 8, 4)`, bookID)
	if err != nil {
		t.Fatalf("INSERT book_place: %v", err)
	}
	placeID, err := place.LastInsertId()
	if err != nil {
		t.Fatalf("book_place LastInsertId: %v", err)
	}
	if _, err := lit.Exec("book_scene",
		`INSERT INTO book_scene (book_id, ord, from_chunk, to_chunk, place_id, time_marker, participants, tags, emotion) VALUES (?, 1, 1, 8, ?, 'Спустя три дня', '["Иван"]', '[]', '{}')`,
		bookID, placeID); err != nil {
		t.Fatalf("INSERT book_scene: %v", err)
	}
	if _, err := lit.Exec("book_chunk",
		`INSERT INTO book_chunk (book_id, ord, chapter, text_hash) VALUES (?, 1, 'Глава тайная', 'hash-1')`, bookID); err != nil {
		t.Fatalf("INSERT book_chunk: %v", err)
	}
	return lit, bookID
}

// TestBookMapThroughSourcesMapRoute — критерий §12.6: карта строится по
// выписанным местам и временам (httptest-провайдер) без повторного чтения
// прозы, роль выбирается режимом походов, провайдер — маршрутом sources_map.
func TestBookMapThroughSourcesMapRoute(t *testing.T) {
	allowLoopbackGate(t)
	srv, provider := newBookMapProvider(t, bookMapAnswer, 0)
	slot := bookMapSlot(t, srv.URL+"/v1", srv.URL+"/v1",
		"first = \"sk-first\"\nsecond = \"sk-second\"\nroute_sources_map = \"first,second\"\n")
	rec := &bookMapRoleSlot{inner: slot}

	lit, bookID := newBookMapLiterary(t)
	bm, err := deepread.BuildBookMap(context.Background(), lit, deepread.MapInput{
		BookID: bookID,
		Mode:   deepread.MapModeMixed,
		Slot:   rec,
		Now:    "2026-11-14T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("BuildBookMap: %v", err)
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("вызовов провайдера = %d, want 1", provider.calls.Load())
	}
	if rec.role != deepread.MapNavigatorRole {
		t.Errorf("роль картографа = %q, want %q", rec.role, deepread.MapNavigatorRole)
	}
	if bm.Text != bookMapAnswer || len(bm.Places) != 1 || len(bm.Scenes) != 1 {
		t.Errorf("карта = %+v", bm)
	}
	body := provider.requestBody()
	for _, want := range []string{"book-map", "Москва", "Спустя три дня"} {
		if !strings.Contains(body, want) {
			t.Errorf("запрос провайдеру без %q: %s", want, body)
		}
	}
	if strings.Contains(body, "Глава тайная") {
		t.Errorf("промпт карты содержит прозу: %s", body)
	}
	saved, ok, err := deepread.LoadBookMap(lit, bookID)
	if err != nil || !ok {
		t.Fatalf("LoadBookMap = (%v, %v, %v), want карту в литературной базе", saved, ok, err)
	}
	if saved.Text != bookMapAnswer {
		t.Errorf("сохранённый текст карты = %q", saved.Text)
	}
}

// TestBookMapFallsBackToSourcesRoute — пустой sources_map падает на sources:
// провайдер маршрута источников отвечает за карту.
func TestBookMapFallsBackToSourcesRoute(t *testing.T) {
	allowLoopbackGate(t)
	srv, provider := newBookMapProvider(t, bookMapAnswer, 0)
	slot := bookMapSlot(t, srv.URL+"/v1", srv.URL+"/v1",
		"first = \"sk-first\"\nsecond = \"sk-second\"\nroute_sources = \"first,second\"\n")
	rec := &bookMapRoleSlot{inner: slot}

	lit, bookID := newBookMapLiterary(t)
	if _, err := deepread.BuildBookMap(context.Background(), lit, deepread.MapInput{
		BookID: bookID,
		Mode:   deepread.MapModeReal,
		Slot:   rec,
	}); err != nil {
		t.Fatalf("BuildBookMap: %v", err)
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("вызовов провайдера = %d, want 1: маршрут падает на sources", provider.calls.Load())
	}
	if rec.role != deepread.MapChroniclerRole {
		t.Errorf("роль картографа = %q, want %q", rec.role, deepread.MapChroniclerRole)
	}
}

// TestBookMapWithoutKeys — ключей нет: проход деградирует, провайдер не
// вызывается, карта не пишется, разбор книги не страдает.
func TestBookMapWithoutKeys(t *testing.T) {
	srv, provider := newBookMapProvider(t, bookMapAnswer, 0)
	slot := bookMapSlot(t, srv.URL+"/v1", srv.URL+"/v1",
		"route_sources_map = \"first,second\"\n")

	lit, bookID := newBookMapLiterary(t)
	_, err := deepread.BuildBookMap(context.Background(), lit, deepread.MapInput{
		BookID: bookID,
		Mode:   deepread.MapModeFiction,
		Slot:   slot,
	})
	if !errors.Is(err, providerroute.ErrNoProvider) {
		t.Fatalf("ошибка = %v, want ErrNoProvider", err)
	}
	if provider.calls.Load() != 0 {
		t.Errorf("провайдер без ключа вызван %d раз", provider.calls.Load())
	}
	if bm, ok, err := deepread.LoadBookMap(lit, bookID); err != nil || ok {
		t.Errorf("карта записана при отказе: (%+v, %v, %v)", bm, ok, err)
	}
}
