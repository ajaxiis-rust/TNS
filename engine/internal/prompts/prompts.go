// Тексты промптов движка (П4 плана GEN-PARAMS-PROMPTS): внешний файл языкового
// пакета packs/<lang>/prompts.json, правка роли из world_meta
// role.<роль>.prompts и дефолт из Go. Файл читается при старте пакета, поэтому
// правка промпта доезжает до запроса модели без пересборки движка; дефолт в Go
// остаётся на случай отсутствия файла или ключа.
package prompts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"tns65/internal/llmparams"
)

// FileName — имя файла промптов в каталоге языкового пакета.
const FileName = "prompts.json"

// Снимок файла пакета: ключ → шаблон. Пустая таблица — дефолты Go.
var (
	mu    sync.RWMutex
	texts = map[string]string{}
)

// Load читает prompts.json из каталога пакета dir. Отсутствие файла — не
// ошибка: остаются дефолты Go; битый JSON — ошибка, видимая на старте.
func Load(dir string) error {
	path := filepath.Join(dir, FileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			Set(nil)
			return nil
		}
		return fmt.Errorf("prompts: %s: %w", path, err)
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("prompts: %s: %w", path, err)
	}
	Set(m)
	return nil
}

// Set заменяет таблицу текстов пакета; nil снимает её (остаются дефолты Go).
func Set(m map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	if m == nil {
		texts = map[string]string{}
		return
	}
	texts = m
}

// Text возвращает шаблон промпта по ключу: файл пакета → дефолт Go. Пустой
// текст в файле считается отсутствующим ключом.
func Text(key, def string) string {
	mu.RLock()
	t, ok := texts[key]
	mu.RUnlock()
	if ok && strings.TrimSpace(t) != "" {
		return t
	}
	return def
}

// ForRole возвращает текст промпта роли: правка world_meta
// role.<роль>.prompts главнее файла пакета и дефолта Go.
func ForRole(role, key, def string) string {
	tmpl := Text(key, def)
	override := llmparams.Prompts(role)
	if override == "" {
		return tmpl
	}
	return applyOverride(tmpl, override)
}

// applyOverride вставляет правку роли в промпт: JSON-объект правки заменяет
// шаблон целиком, короткая строка дополняет промпт полем extra (JSON-обёртка)
// или отдельным абзацем (текстовый промпт).
func applyOverride(tmpl, override string) string {
	override = strings.TrimSpace(override)
	if strings.HasPrefix(override, "{") && json.Valid([]byte(override)) {
		return override
	}
	if strings.HasPrefix(strings.TrimSpace(tmpl), "{") {
		var m map[string]any
		if err := json.Unmarshal([]byte(tmpl), &m); err == nil {
			m["extra"] = override
			if raw, err := json.Marshal(m); err == nil {
				return string(raw)
			}
		}
	}
	if strings.TrimSpace(tmpl) == "" {
		return override
	}
	return strings.TrimSpace(tmpl) + "\n\n" + override
}

// Render подставляет значения {имя} в промпт роли (ForRole). JSON-шаблон
// рендерится объектом: значение-заготовка подставляется как JSON-значение,
// поэтому форма промпта (JSON-обёртка) сохраняется.
func Render(role, key, def string, vars map[string]any) string {
	return Substitute(ForRole(role, key, def), vars)
}

// Substitute подставляет значения {имя} в шаблон: JSON-объект — по значениям
// полей, текстовый шаблон — строковой подстановкой.
func Substitute(tmpl string, vars map[string]any) string {
	if len(vars) == 0 {
		return tmpl
	}
	trimmed := strings.TrimSpace(tmpl)
	if strings.HasPrefix(trimmed, "{") {
		var v any
		if err := json.Unmarshal([]byte(trimmed), &v); err == nil {
			if _, ok := v.(map[string]any); ok {
				if raw, err := json.Marshal(substituteJSON(v, vars)); err == nil {
					return string(raw)
				}
			}
		}
	}
	return substituteText(tmpl, vars)
}

// substituteJSON заменяет заготовки в значениях JSON-шаблона: строка целиком
// вида "{имя}" — подстановка значения как есть (массив, число, объект),
// вложенная заготовка — подстановка скаляра внутрь строки.
func substituteJSON(v any, vars map[string]any) any {
	switch x := v.(type) {
	case string:
		if name, ok := placeholderName(x); ok {
			if val, ok := vars[name]; ok {
				return val
			}
		}
		out := x
		for k, val := range vars {
			out = strings.ReplaceAll(out, "{"+k+"}", jsonScalar(val))
		}
		return out
	case map[string]any:
		for k := range x {
			x[k] = substituteJSON(x[k], vars)
		}
		return x
	case []any:
		for i := range x {
			x[i] = substituteJSON(x[i], vars)
		}
		return x
	default:
		return v
	}
}

// substituteText подставляет значения в текстовый шаблон одной проходкой:
// значение-список склеивается через ", ".
func substituteText(tmpl string, vars map[string]any) string {
	pairs := make([]string, 0, len(vars)*2)
	for k, val := range vars {
		pairs = append(pairs, "{"+k+"}", textScalar(val))
	}
	return strings.NewReplacer(pairs...).Replace(tmpl)
}

// placeholderName — имя заготовки, если строка целиком "{имя}".
func placeholderName(s string) (string, bool) {
	if len(s) < 3 || s[0] != '{' || s[len(s)-1] != '}' {
		return "", false
	}
	name := s[1 : len(s)-1]
	if strings.ContainsAny(name, "{} \t\n") {
		return "", false
	}
	return name, true
}

// jsonScalar — скаляр для подстановки внутрь строки JSON: кавычки снимаются,
// спецсимволы экранируются средствами encoding/json.
func jsonScalar(val any) string {
	raw, err := json.Marshal(val)
	if err != nil {
		return textScalar(val)
	}
	s := string(raw)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// textScalar — значение для текстовой подстановки.
func textScalar(val any) string {
	switch x := val.(type) {
	case string:
		return x
	case []string:
		return strings.Join(x, ", ")
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(val)
	}
}
