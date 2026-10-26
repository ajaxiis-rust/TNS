package sediment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"tns65/internal/journal"
)

// Slot — слот генерации мастерской: авто-критик вызывает его для ревью
// очищенной формы (T-04-03, шаг 2). В тестах подменяется testkit.FakeSlot.
type Slot interface {
	Generate(ctx context.Context, role, prompt string) (string, error)
}

// Вердикты авто-критика (строгий набор, W-03.3): clean и issues возвращает
// модель, degraded ставит код после исчерпания повторов.
const (
	VerdictClean    = "clean"
	VerdictIssues   = "issues"
	VerdictDegraded = "degraded"
)

// Роль слота мастерской для авто-критика (T-04-03, шаг 2).
const autocriticRole = "autocritic"

// Источник событий журнала оседания.
const autocriticSource = "sediment"

// События журнала авто-критика (T-04-03, шаги 2–3).
const (
	eventAutocriticVerdict  = "sediment.autocritic.verdict"
	eventAutocriticDegraded = "sediment.autocritic.degraded"
)

// maxAutocriticRetries — мусорный ответ повторяется не более двух раз
// (00-INDEX «Сквозные гейты»: отказы модельных шагов).
const maxAutocriticRetries = 2

// Пометка невырезаемого: флаг needs_review вокруг фрагмента, не удаляя его.
const (
	needsReviewOpen  = "<needs_review>"
	needsReviewClose = "</needs_review>"
)

// noncutTerms — невырезаемое для очистки (W-03.3): местоимения и пересказ.
// Перечень закрыт, пополняется только правкой этого списка. Многословные
// фразы идут раньше одиночных слов: «тот человек» помечается целиком, а не
// как отдельное «тот».
var noncutTerms = []string{
	// Пересказ — демонстратив с категорийным словом.
	"тот человек", "этот человек", "тот мужчина", "этот мужчина",
	"та женщина", "эта женщина",
	"то место", "это место",
	"та вещь", "эта вещь",
	"та цена", "эта цена",
	"те люди", "эти люди",

	// Местоимения.
	"собой", "собою",
	"него", "неё", "нее", "нему", "ними",
	"этот", "себя", "себе",
	"ней", "нём", "нем", "ним", "них",
	"его", "её", "ее", "ему", "им", "ими", "их",
	"эта", "эти", "тот", "это",
	"она", "оно", "они",
	"ей", "он", "та", "то", "те",
}

// autocriticVerdict — строгий JSON-контракт выхода авто-критика (шаг 2).
type autocriticVerdict struct {
	Verdict string   `json:"verdict"`
	Issues  []string `json:"issues"`
}

// MarkNeedsReview помечает невырезаемое (местоимения, пересказ) флагом
// needs_review, не удаляя сам фрагмент. Возвращает очищенную форму с пометками —
// вход авто-критика.
func MarkNeedsReview(text string) string {
	runes := []rune(text)
	var b strings.Builder
	i := 0
	for i < len(runes) {
		if term, n := matchNoncut(runes, i); n > 0 {
			b.WriteString(needsReviewOpen)
			b.WriteString(term)
			b.WriteString(needsReviewClose)
			i += n
			continue
		}
		b.WriteRune(runes[i])
		i++
	}
	return b.String()
}

// matchNoncut ищет первый подходящий термин невырезаемого в runes начиная с
// позиции i; возвращает исходный текст термина и его длину в рунах (0 — нет).
func matchNoncut(runes []rune, i int) (string, int) {
	for _, term := range noncutTerms {
		tr := []rune(term)
		if i+len(tr) > len(runes) {
			continue
		}
		if !strings.EqualFold(string(runes[i:i+len(tr)]), term) {
			continue
		}
		// Границы: слева и справа от термина не должно быть буквы.
		if i > 0 && unicode.IsLetter(runes[i-1]) {
			continue
		}
		if i+len(tr) < len(runes) && unicode.IsLetter(runes[i+len(tr)]) {
			continue
		}
		return string(runes[i : i+len(tr)]), len(tr)
	}
	return "", 0
}

// Autocritic помечает невырезаемое, вызывает авто-критика через слот мастерской
// и записывает вердикт в поле Verdict записи (T-04-03). Модель не имеет права
// менять CleanedText: на выходе он равен входному — строгий парсер отвергает
// любые лишние поля ответа, а код не пишет в CleanedText. Мусорный ответ
// повторяется не более двух раз, затем Verdict=degraded — конвейер не ломается.
func Autocritic(ctx context.Context, slot Slot, logger *journal.Logger, rec SedimentRecord) SedimentRecord {
	marked := MarkNeedsReview(rec.CleanedText)
	prompt := buildAutocriticPrompt(marked)

	if slot == nil {
		rec.Verdict = VerdictDegraded
		if logger != nil {
			_ = logger.Event(autocriticSource, eventAutocriticDegraded,
				journal.Field{Key: "err", Value: "slot is nil"})
		}
		return rec
	}

	verdict, issues, err := askAutocritic(ctx, slot, prompt)
	if err != nil {
		rec.Verdict = VerdictDegraded
		if logger != nil {
			_ = logger.Event(autocriticSource, eventAutocriticDegraded,
				journal.Field{Key: "err", Value: err.Error()})
		}
		return rec
	}

	rec.Verdict = verdict
	if logger != nil {
		fields := []journal.Field{{Key: "verdict", Value: verdict}}
		if len(issues) > 0 {
			fields = append(fields, journal.Field{Key: "issues", Value: strings.Join(issues, ";")})
		}
		_ = logger.Event(autocriticSource, eventAutocriticVerdict, fields...)
	}
	return rec
}

// buildAutocriticPrompt собирает промпт авто-критика из очищенной формы с пометками.
func buildAutocriticPrompt(marked string) string {
	return "Очищенная сцена с пометками невырезаемого. Фрагменты в <needs_review> не вырезать. " +
		`Ответь строго одним JSON: {"verdict":"clean"|"issues","issues":["…"]}.` + "\n\n" + marked
}

// askAutocritic вызывает слот до maxAutocriticRetries повторов при мусорном
// ответе или отказе слота и возвращает разобранный вердикт.
func askAutocritic(ctx context.Context, slot Slot, prompt string) (string, []string, error) {
	var lastErr error
	for attempt := 0; attempt <= maxAutocriticRetries; attempt++ {
		raw, err := slot.Generate(ctx, autocriticRole, prompt)
		if err != nil {
			lastErr = err
			continue
		}
		verdict, issues, err := parseVerdict(raw)
		if err != nil {
			lastErr = err
			continue
		}
		return verdict, issues, nil
	}
	return "", nil, lastErr
}

// parseVerdict разбирает строгий JSON-ответ авто-критика: неизвестные поля
// (включая попытку модели вернуть свой cleaned_text) и лишний хвост отвергаются.
func parseVerdict(raw string) (string, []string, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()

	var out autocriticVerdict
	if err := dec.Decode(&out); err != nil {
		return "", nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return "", nil, errors.New("sediment: лишние данные после JSON-вердикта")
	}
	switch out.Verdict {
	case VerdictClean, VerdictIssues:
		return out.Verdict, out.Issues, nil
	default:
		return "", nil, fmt.Errorf("sediment: неизвестный вердикт %q", out.Verdict)
	}
}
