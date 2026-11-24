package langpack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Pool — базовый пул сида: имя файла результата и строки исходного пула.
type Pool struct {
	Name string
	Text []string
}

// TranslateFunc — слот перевода (модель): исходный текст → текст на языке пакета.
// Слот принадлежит станку и вызывается только сборкой пакета.
type TranslateFunc func(ctx context.Context, text string) (string, error)

// BuildOptions — зависимости сборки: слот перевода и журнал сборки.
// Нулевое значение — сборка без перевода (passthrough), деградация без отказа.
type BuildOptions struct {
	Translate TranslateFunc
	Log       io.Writer
}

// cyrillicLangs — языки пакетов с кириллической письменностью.
var cyrillicLangs = map[string]bool{"be": true, "bg": true, "mk": true, "ru": true, "sr": true, "uk": true}

// translateSeed — единственный шаг перевода станка: переводит строку сида на
// язык пакета одним голосом (D-15.1). Живёт только в сборке пакета — конвейер
// живого хода его не вызывает (см. TestBuildHasTranslate).
// Строка, уже написанная на языке пакета, слоту не отдаётся. Слот недоступен
// или отказал — деградация без отказа: строка остаётся как есть, в журнал
// сборки идёт пометка.
func translateSeed(ctx context.Context, s, lang string, opts BuildOptions) string {
	if s == "" || inLanguage(s, lang) {
		return s
	}
	if opts.Translate == nil {
		buildLog(opts.Log, "слот перевода недоступен: строка оставлена как есть")
		return s
	}
	out, err := opts.Translate(ctx, s)
	if err != nil {
		buildLog(opts.Log, "слот перевода отказал (%v): строка оставлена как есть", err)
		return s
	}
	if strings.TrimSpace(out) == "" {
		buildLog(opts.Log, "слот перевода вернул пустую строку: строка оставлена как есть")
		return s
	}
	return out
}

// inLanguage сообщает, что строка уже на языке пакета: буквы его письменности
// составляют не меньше половины всех букв строки. Такую строку слот не получает.
func inLanguage(s, lang string) bool {
	total, native := 0, 0
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		total++
		if writtenIn(r, lang) {
			native++
		}
	}
	return total > 0 && native*2 >= total
}

// writtenIn — буква принадлежит письменности языка пакета.
func writtenIn(r rune, lang string) bool {
	if cyrillicLangs[lang] {
		return unicode.Is(unicode.Cyrillic, r)
	}
	return unicode.Is(unicode.Latin, r)
}

// buildLog пишет строку в журнал сборки; nil-журнал означает молчание.
func buildLog(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "langpack build: "+format+"\n", args...)
}

// Build переводит базовые пулы сида одним проходом одним голосом и кладёт
// результат в каталог версии вместе с манифестом, обновляя указатель current.
func Build(ctx context.Context, packDir, version string, pools []Pool, opts BuildOptions) error {
	if err := validateVersion(version); err != nil {
		return err
	}
	base, err := LoadManifest(filepath.Join(packDir, "manifest.json"))
	if err != nil {
		return err
	}

	versionDir := filepath.Join(packDir, "versions", version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return err
	}

	files := make(map[string]string, len(pools))
	for _, p := range pools {
		out := make([]string, len(p.Text))
		for i, line := range p.Text {
			out[i] = translateSeed(ctx, line, base.Lang, opts)
		}
		data := strings.Join(out, "\n")
		if data != "" {
			data += "\n"
		}
		name := filepath.Base(p.Name)
		path := filepath.Join(versionDir, name)
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			return err
		}
		sum, err := sha256File(path)
		if err != nil {
			return err
		}
		files[name] = sum
	}

	vm := Manifest{
		ID:         base.ID,
		Name:       base.Name,
		Lang:       base.Lang,
		Version:    version,
		Lemmatizer: base.Lemmatizer,
		Files:      files,
	}
	b, err := json.MarshalIndent(vm, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(versionDir, "manifest.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	return writeCurrent(packDir, version)
}
