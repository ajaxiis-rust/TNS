package workshop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"tns65/internal/db"
	"tns65/internal/structural"
	"tns65/internal/version"
)

// ComposeRole — роль слота извлечения композиции при ковке (T-10-12, шаг 3):
// локальный слот генерации :5001, тот же пресет DefaultProvider.
const ComposeRole = "compose"

// composeTask — task строк forge_log шага compose: мусор/тайм-аут (fallback)
// и расхождение семьи слота с кодом (family_mismatch).
const composeTask = "compose"

// composeMaxRetries — число повторов извлечения после первой неудачной попытки
// (мусор/тайм-аут), не больше двух (T-10-12, шаг 3).
const composeMaxRetries = 2

// ComposeOutput — строгий JSON слота compose (T-10-12, шаг 3). Поле
// family_primary сверяется с классификацией кода (шаг 4): при расхождении
// берётся кодовое значение, событие — строка forge_log.
type ComposeOutput struct {
	FamilyPrimary    string `json:"family_primary"`
	PatternSecondary string `json:"pattern_secondary"`
	Technique        string `json:"technique"`
	MoralStance      string `json:"moral_stance"`
	Inversion        bool   `json:"inversion"`
}

// ClassifyFamily — классификация семьи кодом без LLM (T-10-12, шаг 1): поиск
// ключевых слов по границам слова, победитель по числу попаданий; ничья или ноль
// — пустая строка (деградация family-only, тайного фолбэка нет).
func ClassifyFamily(text string, fams []structural.Family) string {
	lower := strings.ToLower(text)
	best := ""
	bestHits := 0
	for _, f := range fams {
		hits := 0
		for _, kw := range f.Keywords {
			if keywordPresent(lower, strings.ToLower(kw)) {
				hits++
			}
		}
		if hits > bestHits {
			best = f.Name
			bestHits = hits
		}
	}
	return best
}

// keywordPresent сообщает, есть ли ключевое слово (возможно, многословное) в
// тексте по границам слова: слева и справа от вхождения — не буква/цифра.
func keywordPresent(text, kw string) bool {
	for i := 0; ; {
		idx := strings.Index(text[i:], kw)
		if idx < 0 {
			return false
		}
		start := i + idx
		end := start + len(kw)
		if (start == 0 || !wordChar(text[start-1])) && (end == len(text) || !wordChar(text[end])) {
			return true
		}
		i = start + 1
	}
}

func wordChar(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_'
}

// Compose извлекает композицию слотом (T-10-12, шаги 3–4): промпт чанка →
// строгий JSON ComposeOutput с валидацией по реестру и закрытым спискам.
// Мусор/тайм-аут — ≤2 повтора, затем нулевая композиция (family-only) и строка
// forge_log; нарушение валидации — ошибка (отказ ковки, шаг 5). codeFamily —
// классификация кода шага 1: семью задаёт код, слот извлекает остальное.
func Compose(ctx context.Context, lit *db.Handle, text string, fams []structural.Family, codeFamily string) (structural.Composition, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	base := structural.Composition{FamilyPrimary: codeFamily}

	slot := slotForProvider(DefaultProvider)
	if slot == nil {
		return base, nil
	}

	for attempt := 0; attempt <= composeMaxRetries; attempt++ {
		out, err := slot.Generate(ctx, ComposeRole, composePrompt(text))
		if err != nil {
			continue // тайм-аут/отказ слота.
		}
		var po ComposeOutput
		if err := json.Unmarshal([]byte(out), &po); err != nil {
			continue // мусор.
		}
		if po.FamilyPrimary != "" && po.FamilyPrimary != "none" && po.FamilyPrimary != codeFamily {
			// Расхождение: код главнее (шаг 4).
			_ = writeComposeLog(lit, "family_mismatch")
		}
		c := structural.Composition{
			FamilyPrimary:    codeFamily,
			PatternSecondary: po.PatternSecondary,
			Technique:        po.Technique,
			MoralStance:      po.MoralStance,
			Inversion:        po.Inversion,
		}
		if err := validateComposition(c, fams); err != nil {
			return structural.Composition{}, err
		}
		return c, nil
	}

	// Мусор/тайм-аут: запись без композиции (family-only), ход игрока не затронут.
	_ = writeComposeLog(lit, "fallback")
	return base, nil
}

// ForgeComposition — ковка чанка с композицией (T-10-12): классификация семьи
// кодом, извлечение остального слотом compose, валидация и запись в банк.
// Идемпотентность по origin проверяется до вызова слота (шаг 6): повторная ковка
// поля не перезаписывает и слота не зовёт.
func ForgeComposition(ctx context.Context, world, lit *db.Handle, m ForgeMaterial, fams []structural.Family) (int64, error) {
	if m.Origin == "" {
		return 0, errors.New("workshop: ковка без origin")
	}
	if id, ok, err := findBankByOrigin(lit, m.Origin); err != nil {
		return 0, err
	} else if ok {
		return id, nil
	}

	comp, err := Compose(ctx, lit, m.Text, fams, ClassifyFamily(m.Text, fams))
	if err != nil {
		return 0, err
	}
	m.Composition = comp
	return ForgeBankRecord(world, lit, m)
}

// validateComposition проверяет композицию перед записью (T-10-12, шаг 5):
// семья и secondary — из реестра, secondary ≠ primary, техника и позиция — из
// закрытых списков. Пустая семья — family-only, разрешена.
func validateComposition(c structural.Composition, fams []structural.Family) error {
	if c.FamilyPrimary != "" && !familyInRegistry(c.FamilyPrimary, fams) {
		return fmt.Errorf("workshop: семья %q вне реестра", c.FamilyPrimary)
	}
	if c.PatternSecondary != "" {
		if c.PatternSecondary == c.FamilyPrimary {
			return fmt.Errorf("workshop: pattern_secondary %q совпадает с family_primary", c.PatternSecondary)
		}
		if !familyInRegistry(c.PatternSecondary, fams) {
			return fmt.Errorf("workshop: pattern_secondary %q вне реестра", c.PatternSecondary)
		}
	}
	if c.Technique != "" && !structural.ValidTechnique(c.Technique) {
		return fmt.Errorf("workshop: техника %q вне списка", c.Technique)
	}
	if c.MoralStance != "" && !structural.ValidMoralStance(c.MoralStance) {
		return fmt.Errorf("workshop: моральная позиция %q вне списка", c.MoralStance)
	}
	return nil
}

// familyInRegistry — вхождение слага семьи в реестр.
func familyInRegistry(name string, fams []structural.Family) bool {
	for _, f := range fams {
		if f.Name == name {
			return true
		}
	}
	return false
}

// composePrompt строит промпт слота compose: чанк книги, строгий JSON на выходе.
func composePrompt(text string) string {
	raw, _ := json.Marshal(map[string]any{
		"task":  ComposeRole,
		"chunk": text,
	})
	return string(raw)
}

// writeComposeLog помечает отказ/расхождение шага compose строкой forge_log
// (писатель forge).
func writeComposeLog(lit *db.Handle, status string) error {
	now := nowUTC()
	return lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		_, err := tx.Exec("forge_log",
			`INSERT INTO forge_log (kind, task, status, started_at, finished_at, log_path, package_version) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ComposeRole, composeTask, status, now, now, "", version.Version)
		return err
	})
}
