#!/usr/bin/env bash
# Бенчмарк ковки книги (T-12-30, §8.1): варит книгу целиком или первые N кусков
# в локальных слотах :5001/:5002, пишет мир и отчёт во временный каталог и
# печатает время по шагам (чанкер, карта, ковка), число вызовов слота и токены.
# Это ручной бенчмарк, а не тест: в CI он не входит.
set -euo pipefail

# Корень движка (скрипт лежит в engine/scripts).
cd "$(dirname "$0")/.."
ENGINE_ROOT="$(pwd)"

usage() {
    cat <<EOF
usage: $0 <файл|каталог> [кусков]

Варит книгу целиком (или первые N кусков) локальными слотами :5001/:5002 и
печатает время по шагам, число вызовов слота и токены. Мир и отчёт пишутся во
временный каталог, который удаляется по завершении.

  файл      путь к книге .txt/.fb2; каталог — берётся самая маленькая .fb2 из него
             (имя можно задать переменной TNS65_TEST_BOOK)
  кусков    сколько кусков варить; пусто — все куски
EOF
    exit "${1:-0}"
}

# resolve_book превращает первый аргумент в путь к файлу книги: каталог — поиск
# самой маленькой .fb2, файл — как есть; TNS65_TEST_BOOK задаёт имя.
resolve_book() {
    local arg="$1" name best bestsize=-1
    if [ -f "$arg" ]; then
        printf '%s' "$arg"
        return 0
    fi
    if [ ! -d "$arg" ]; then
        echo "bench-book: нет такого файла или каталога: $arg" >&2
        return 2
    fi
    if [ -n "${TNS65_TEST_BOOK:-}" ]; then
        name="${TNS65_TEST_BOOK}"
        [ -f "$name" ] || name="$arg/$name"
        if [ -f "$name" ]; then
            printf '%s' "$name"
            return 0
        fi
        echo "bench-book: книга $TNS65_TEST_BOOK не найдена в $arg" >&2
        return 2
    fi
    for f in "$arg"/*.fb2; do
        [ -f "$f" ] || continue
        size="$(stat -c%s "$f")"
        if [ "$bestsize" -lt 0 ] || [ "$size" -lt "$bestsize" ]; then
            bestsize="$size"
            best="$f"
        fi
    done
    if [ -z "${best:-}" ]; then
        echo "bench-book: в каталоге $arg нет книг .fb2" >&2
        return 2
    fi
    printf '%s' "$best"
}

main() {
    case "${1:-}" in
        ""|-h|--help) usage 0 ;;
    esac

    local book chunks="${2:-0}"
    book="$(resolve_book "$1")" || exit $?

    case "$chunks" in
        ''|*[!0-9]*) echo "bench-book: кусков должно быть целым числом: $chunks" >&2; exit 2 ;;
    esac

    # Временный каталог внутри модуля: go run импортирует tns65/internal из
    # пакета с путём tns65/<каталог>, иначе internal-импорт запрещён. Глобальная
    # переменная, а не local: её читает trap EXIT после возврата из main.
    tmp="$(mktemp -d "$ENGINE_ROOT/.bench-book.XXXXXX")"
    trap 'rm -rf "$tmp"' EXIT

    local prog="$tmp/main.go"
    cat > "$prog" <<'GO_PROG'
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tns65/internal/cast"
	"tns65/internal/db"
	"tns65/internal/models"
	"tns65/internal/structural"
	"tns65/internal/supervisor"
	"tns65/internal/workshop"
)

const (
	genURL   = "http://127.0.0.1:5001"
	embedURL = "http://127.0.0.1:5002"
)

// passCensor пропускает любой текст: бенчмарк меряет механику ковки, а не
// словарь Цензора мира.
type passCensor struct{}

func (passCensor) Check(string) (bool, []string) { return true, nil }

// countingSlot оборачивает слот генерации и считает вызовы; inner nil — вызовы
// не выполняются (модель не поднята).
type countingSlot struct {
	inner workshop.Slot
	calls int
}

func (s *countingSlot) Generate(ctx context.Context, role, prompt string) (string, error) {
	s.calls++
	if s.inner == nil {
		return "", fmt.Errorf("bench-book: слот генерации не подключён")
	}
	return s.inner.Generate(ctx, role, prompt)
}

// countingEmbedder оборачивает эмбеддер и считает вызовы.
type countingEmbedder struct {
	inner models.Embedder
	calls int
}

func (e *countingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	e.calls++
	if e.inner == nil {
		return nil, fmt.Errorf("bench-book: эмбеддер не подключён")
	}
	return e.inner.Embed(ctx, text)
}

// localSlot — слот генерации поверх models.Weaver: workshop.Slot ждёт готовую
// строку, Weaver стримит чанки /completion — адаптер их склеивает.
type localSlot struct{ weaver *models.Weaver }

func (s localSlot) Generate(ctx context.Context, _ string, prompt string) (string, error) {
	ch, err := s.weaver.Stream(ctx, prompt)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for line := range ch {
		b.WriteString(line)
	}
	return b.String(), nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "bench-book:", err)
	os.Exit(1)
}

// newDB открывает и мигрирует базу до схемы каталогов dirs.
func newDB(path string, dirs ...string) {
	w, err := db.OpenWriter(path)
	if err != nil {
		fatal(err)
	}
	defer w.Close()
	if err := db.Migrate(w, dirs...); err != nil {
		fatal(err)
	}
}

// writeChunksTXT кладёт куски в txt для ковки ровно отмеренного числа кусков.
func writeChunksTXT(path string, chunks []workshop.Chunk) {
	var b []byte
	for _, c := range chunks {
		b = append(b, c.Chapter...)
		b = append(b, '\n')
		b = append(b, c.Text...)
		b = append(b, '\n', '\n')
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		fatal(err)
	}
}

// health проверяет /health локального резидента; down — false.
func health(port int) bool {
	return supervisor.CheckHealth(supervisor.Config{Port: port, HealthPath: "/health"}) == nil
}

func main() {
	bookPath := flag.String("book", "", "путь к книге txt/fb2")
	chunks := flag.Int("chunks", 0, "сколько кусков варить (0 — все)")
	outDir := flag.String("out", "", "каталог для мира и отчёта")
	engineRoot := flag.String("root", "", "корень движка (base/, packs/)")
	flag.Parse()

	if *bookPath == "" || *outDir == "" || *engineRoot == "" {
		fmt.Fprintln(os.Stderr, "bench-book: обязательны -book, -out, -root")
		os.Exit(2)
	}

	// Шаг 1 — чанкер.
	startChunk := time.Now()
	parsed, err := workshop.ParseBookInput(*bookPath)
	if err != nil {
		fatal(err)
	}
	chunkDur := time.Since(startChunk)
	if len(parsed.Chunks) == 0 {
		fmt.Fprintln(os.Stderr, "bench-book: книга без кусков")
		os.Exit(1)
	}
	total := len(parsed.Chunks)
	if *chunks > 0 && *chunks < total {
		parsed.Chunks = parsed.Chunks[:*chunks]
	}
	n := len(parsed.Chunks)

	// Шаг 2 — карта (скан, описания, роли; quick, без слота).
	castChunks := make([]cast.Chunk, 0, n)
	for i, c := range parsed.Chunks {
		castChunks = append(castChunks, cast.Chunk{Ord: int64(i + 1), Chapter: c.Chapter, Text: c.Text})
	}
	profile, ok := cast.Profile("ru")
	if !ok {
		fmt.Fprintln(os.Stderr, "bench-book: профиль ru не найден")
		os.Exit(1)
	}
	dict, err := cast.LoadDescriptions(filepath.Join(*engineRoot, "packs", "ru", "descriptions.json"))
	if err != nil {
		fatal(err)
	}

	startCard := time.Now()
	cands, err := cast.Scan(castChunks, cast.ScanOptions{Profile: profile})
	if err != nil {
		fatal(err)
	}
	if len(cands) > 0 {
		if _, err := cast.Describe(castChunks, cands, dict, cast.DescribeOptions{}); err != nil {
			fatal(err)
		}
	}
	for _, c := range cands {
		if _, err := cast.ChunkRoles(castChunks, c, cast.DefaultRoleDict(), cast.RoleOptions{}); err != nil {
			fatal(err)
		}
	}
	cardDur := time.Since(startCard)

	// Мир во временном каталоге.
	worldPath := filepath.Join(*outDir, "world.db")
	litPath := filepath.Join(*outDir, "literary.db")
	newDB(worldPath, "world", "shared")
	newDB(litPath, "literary")

	world, err := db.OpenWriterAs(worldPath, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		fatal(err)
	}
	defer world.Close()
	lit, err := db.OpenWriterAs(litPath, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		fatal(err)
	}
	defer lit.Close()

	genOK := health(5001)
	embedOK := health(5002)

	cs := &countingSlot{}
	ce := &countingEmbedder{}
	if genOK {
		weaver, err := models.NewWeaver(genURL)
		if err != nil {
			fatal(err)
		}
		cs.inner = localSlot{weaver: weaver}
	}
	if embedOK {
		emb, err := models.NewClient(embedURL, nil)
		if err != nil {
			fatal(err)
		}
		ce.inner = emb
	}

	workshop.SetCensor(passCensor{})
	defer workshop.SetCensor(nil)
	if genOK {
		workshop.SetSlot(cs)
		defer workshop.SetSlot(nil)
	}
	if embedOK {
		workshop.SetEmbedder(ce)
		defer workshop.SetEmbedder(nil)
	}

	fams, err := structural.LoadFamilies(filepath.Join(*engineRoot, "base", "archetypes.json"))
	if err != nil {
		fatal(err)
	}

	// Шаг 3 — ковка ровно n кусков.
	forgeInput := filepath.Join(*outDir, "book.txt")
	writeChunksTXT(forgeInput, parsed.Chunks)

	startForge := time.Now()
	res, err := workshop.RunForgeBook(context.Background(), world, lit, forgeInput, workshop.ForgeBookOptions{Families: fams})
	forgeDur := time.Since(startForge)
	if err != nil {
		fatal(err)
	}

	// Токены — детерминированная мера countTokens: число слов по пробелам.
	tokens := 0
	for _, c := range parsed.Chunks {
		tokens += len(strings.Fields(c.Text))
	}

	report := struct {
		File       string  `json:"file"`
		Chunks     int     `json:"chunks"`
		TotalChunks int    `json:"total_chunks"`
		Candidates int     `json:"candidates"`
		SlotCalls  int     `json:"slot_calls"`
		EmbedCalls int     `json:"embed_calls"`
		Tokens     int     `json:"tokens"`
		ChunkerMS  int64   `json:"chunker_ms"`
		CardMS     int64   `json:"card_ms"`
		ForgeMS    int64   `json:"forge_ms"`
		Progress   float64 `json:"progress"`
		BookID     int64   `json:"book_id"`
	}{
		File:        *bookPath,
		Chunks:      n,
		TotalChunks: total,
		Candidates:  len(cands),
		SlotCalls:   cs.calls,
		EmbedCalls:  ce.calls,
		Tokens:      tokens,
		ChunkerMS:   chunkDur.Milliseconds(),
		CardMS:      cardDur.Milliseconds(),
		ForgeMS:     forgeDur.Milliseconds(),
		Progress:    res.Progress,
		BookID:      res.BookID,
	}

	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "report.json"), raw, 0o644); err != nil {
		fatal(err)
	}

	fmt.Printf("bench-book: %s\n", *bookPath)
	fmt.Printf("куски: %d (из %d)\n", report.Chunks, report.TotalChunks)
	fmt.Printf("время:\n")
	fmt.Printf("  чанкер  %d мс\n", report.ChunkerMS)
	fmt.Printf("  карта   %d мс (героев: %d)\n", report.CardMS, report.Candidates)
	fmt.Printf("  ковка   %d мс\n", report.ForgeMS)
	if !genOK {
		fmt.Println("предупреждение: локальная модель :5001 не поднята — вызовов слота нет (см. scripts/test-local.sh)")
	}
	if !embedOK {
		fmt.Println("предупреждение: локальный эмбеддер :5002 не поднят — векторов нет (см. scripts/test-local.sh)")
	}
	fmt.Printf("вызовы слота: %d\n", report.SlotCalls)
	fmt.Printf("вызовы эмбеддера: %d\n", report.EmbedCalls)
	fmt.Printf("токены: %d\n", report.Tokens)
	fmt.Printf("прогресс: %.3f\n", report.Progress)
}
GO_PROG

    echo "bench-book: временный каталог $tmp"
    cd "$ENGINE_ROOT"
    go run "./$(basename "$tmp")" -book "$book" -chunks "$chunks" -out "$tmp" -root "$ENGINE_ROOT"
}

main "$@"
