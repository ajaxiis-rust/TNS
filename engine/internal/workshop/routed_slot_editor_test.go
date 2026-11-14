package workshop_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"tns65/internal/book"
	"tns65/internal/db"
	"tns65/internal/providercfg"
	"tns65/internal/providerroute"
	"tns65/internal/workshop"
)

// editorRouteSlot собирает слот мастерской по двум маршрутам: sources — провайдер
// sourcesURL, editor — editorURL; роли литературной обработки главы идут маршрутом
// editor, прочие — маршрутом sources.
func editorRouteSlot(t *testing.T, sourcesURL, editorURL string) *workshop.RoutedWorkshopSlot {
	t.Helper()
	dir := t.TempDir()
	providersPath := filepath.Join(dir, "providers.toml")
	providers := fmt.Sprintf(`
[[providers]]
name = "sources"
protocol = "openai"
base_url = %q
default_model = "sources-1"

[[providers]]
name = "editor"
protocol = "openai"
base_url = %q
default_model = "editor-1"
`, sourcesURL, editorURL)
	if err := os.WriteFile(providersPath, []byte(providers), 0o644); err != nil {
		t.Fatalf("WriteFile(providers.toml): %v", err)
	}
	settingsPath := filepath.Join(dir, "settings.toml")
	settings := "sources = \"sk-sources\"\neditor = \"sk-editor\"\n" +
		"route_sources = \"sources\"\nroute_editor = \"editor\"\n"
	if err := os.WriteFile(settingsPath, []byte(settings), 0o644); err != nil {
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
	return workshop.NewRoutedWorkshopSlot(router.Slot(providerroute.TaskSources)).
		RouteRoles(router.Slot(providerroute.TaskEditor), workshop.EditorRoles()...)
}

// roleProviderServer — подставной провайдер маршрута: считает вызовы и отдаёт
// одним SSE-чанком текст, выбранный по телу запроса.
func roleProviderServer(calls *atomic.Int32, reply func(body string) string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", reply(string(body)))
	}))
}

// insertEditorProcessedText кладёт обработанную Редактором версию главы — вход
// дайджеста (edited_by='editor').
func insertEditorProcessedText(t *testing.T, h *db.Handle, chapterID int64, text string) {
	t.Helper()
	if _, err := h.Exec("book_text",
		`INSERT INTO book_text (chapter_id, version, text, edited_by, published) VALUES (?, 2, ?, 'editor', 0)`,
		chapterID, text); err != nil {
		t.Fatalf("INSERT book_text(edited_by=editor): %v", err)
	}
}

// TestRoutedWorkshopSlotEditorRoles — Э4: роли editor, editor_digest и style идут
// маршрутом editor, прочие роли остаются на маршруте sources; промпт роли уходит
// без изменений, слот, отданный в SetSlot, подменой ролей не меняется.
func TestRoutedWorkshopSlotEditorRoles(t *testing.T) {
	sources := &fakeRouteSlot{chunks: []string{"источники"}}
	editor := &fakeRouteSlot{chunks: []string{"редактор"}}
	slot := workshop.NewRoutedWorkshopSlot(sources).RouteRoles(editor, workshop.EditorRoles()...)

	roles := []string{workshop.EditorRole, workshop.EditorDigestRole, workshop.StyleRole}
	for _, role := range roles {
		prompt := `{"task":"` + role + `","text":"промпт роли"}`
		got, err := slot.Generate(context.Background(), role, prompt)
		if err != nil {
			t.Fatalf("Generate(%s): %v", role, err)
		}
		if got != "редактор" {
			t.Fatalf("роль %s обслужил не маршрут editor: ответ = %q", role, got)
		}
		if n := len(editor.prompts); n == 0 || editor.prompts[n-1] != prompt {
			t.Fatalf("промпт роли %s в маршруте editor = %q, ожидался без изменений %q",
				role, editor.prompts[len(editor.prompts)-1], prompt)
		}
	}
	if len(sources.prompts) != 0 {
		t.Fatalf("маршрут sources вызван ролями литературной обработки: %d запросов", len(sources.prompts))
	}

	// Роль вне литературной обработки главы остаётся на маршруте sources.
	got, err := slot.Generate(context.Background(), workshop.ComposeRole, "промпт куска")
	if err != nil {
		t.Fatalf("Generate(%s): %v", workshop.ComposeRole, err)
	}
	if got != "источники" {
		t.Fatalf("роль %s ушла не на маршрут sources: ответ = %q", workshop.ComposeRole, got)
	}
	if len(editor.prompts) != len(roles) {
		t.Fatalf("запросов в маршрут editor = %d, ожидалось %d", len(editor.prompts), len(roles))
	}
}

// TestEditChapterThroughEditorRoute — критерий Э4: правка главы идёт выбранным
// провайдером маршрута editor, маршрут sources не вызывается; промпты прежние,
// Цензор правок по-прежнему проверяет 1:1 и бракует правку вне орфографии.
func TestEditChapterThroughEditorRoute(t *testing.T) {
	allowLoopbackGate(t)
	workshop.SetLemmatizer(fakeEditLemmatizer{forms: editorLexicon()})
	defer workshop.SetLemmatizer(nil)

	var sourcesCalls, editorCalls atomic.Int32
	sources := roleProviderServer(&sourcesCalls, func(string) string { return composeOK })
	defer sources.Close()
	editor := roleProviderServer(&editorCalls, func(body string) string {
		if strings.Contains(body, "scene") {
			return `{"edits":[{"before":"по этому","after":"поэтому"},{"before":"ёлку.","after":"ёлку…"},{"before":"Иван","after":"Гастон"}]}`
		}
		return `{"summary":"Иван гулял по городу и встретил ёлку."}`
	})
	defer editor.Close()

	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	newDB(t, worldPath, "world", "shared")
	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()

	insertEditorChapter(t, world, 1)
	insertEditorText(t, world, 1, 1, "Иван шёл по этому городу и увидел ёлку.")

	slot := editorRouteSlot(t, sources.URL+"/v1", editor.URL+"/v1")
	res, err := workshop.EditChapter(context.Background(), world, slot, 1)
	if err != nil {
		t.Fatalf("EditChapter: %v", err)
	}

	// Правка и краткое содержание — два запроса выбранному провайдеру маршрута
	// editor; маршрут sources не участвует.
	if got := editorCalls.Load(); got != 2 {
		t.Fatalf("вызовов провайдера editor = %d, ожидалось 2 (правки и краткое содержание)", got)
	}
	if got := sourcesCalls.Load(); got != 0 {
		t.Fatalf("вызовов провайдера sources = %d, ожидалось 0", got)
	}
	if len(res.Edits) != 2 {
		t.Fatalf("применённых правок: %d, want 2", len(res.Edits))
	}
	const want = "Иван шёл поэтому городу и увидел ёлку…"
	if got := editorText(t, world, 1); got != want {
		t.Fatalf("текст после редактуры = %q, want %q", got, want)
	}
	if strings.Contains(editorText(t, world, 1), "Гастон") {
		t.Fatal("правка имени применена, want отбракована Цензором правок")
	}
}

// TestBuildDigestThroughEditorRoute — критерий Э4: дайджест главы идёт выбранным
// провайдером маршрута editor, маршрут sources не вызывается.
func TestBuildDigestThroughEditorRoute(t *testing.T) {
	allowLoopbackGate(t)

	const digest = "Иван гулял по городу. Он встретил ёлку. День был ясный. Город спал. Вечер пришёл тихо."

	var sourcesCalls, editorCalls atomic.Int32
	sources := roleProviderServer(&sourcesCalls, func(string) string { return composeOK })
	defer sources.Close()
	editor := roleProviderServer(&editorCalls, func(string) string { return digest })
	defer editor.Close()

	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world(a).db")
	newDB(t, worldPath, "world", "shared")
	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	defer world.Close()

	insertEditorChapter(t, world, 1)
	insertEditorProcessedText(t, world, 1, "Иван шёл поэтому городу и увидел ёлку…")

	slot := editorRouteSlot(t, sources.URL+"/v1", editor.URL+"/v1")
	if err := book.BuildDigest(context.Background(), world, slot, 1); err != nil {
		t.Fatalf("BuildDigest: %v", err)
	}

	if got := editorCalls.Load(); got != 1 {
		t.Fatalf("вызовов провайдера editor = %d, ожидался 1 на главу", got)
	}
	if got := sourcesCalls.Load(); got != 0 {
		t.Fatalf("вызовов провайдера sources = %d, ожидалось 0", got)
	}
	if got := editorDigest(t, world, 1); got != digest {
		t.Fatalf("дайджест = %q, want %q", got, digest)
	}
}
