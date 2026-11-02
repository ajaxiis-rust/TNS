// Общий слой повторов генерации (T-12-08): GenerateWithRetry пробует слот по
// лестнице из четырёх ступеней (п.6.2, шаг 4), пишет строку forge_log на каждый
// модельный отказ и умеет двухпроходный режим ролей прозы «факты → проза»
// (шаг 5).
package workshop

import (
	"context"
	"encoding/json"
	"errors"

	"tns65/internal/censor"
	"tns65/internal/db"
	"tns65/internal/version"
)

// ErrForgeDegraded — все ступени лестницы повторов отказали (п.6.2, шаг 4):
// ковка деградирует, ход не отменяется.
var ErrForgeDegraded = errors.New("workshop: все ступени генерации отказали")

// refusalKind / refusalTask — kind и task строк forge_log о модельном отказе
// (п.6.2, шаг 6).
const (
	refusalKind = "censor"
	refusalTask = "refusal"
)

// Ключи промпта ступеней лестницы (п.6.2, шаг 4): ступень 1 — художественная
// рамка и рейтинг в системном промпте, ступени 2–3 — трудная часть уходит за
// кадр.
const (
	frameKey     = "frame"
	ratingKey    = "rating"
	offscreenKey = "offscreen"
)

// artisticFrame — художественная рамка ступени 1.
const artisticFrame = "художественная проза"

// GenerateOptions — параметры общего слоя повторов генерации.
type GenerateOptions struct {
	// Role — роль слота; она же пишется в строку forge_log отказа.
	Role string
	// Lang — язык детектора отказа (packs/<lang>/refusals.txt).
	Lang string
	// Provider — пресет слота первых двух ступеней; третья — локальный.
	Provider Provider
	// TwoPass — двухпроходный режим ролей прозы: сначала сухие факты, затем
	// проза (п.6.2, шаг 5).
	TwoPass bool
}

// PromptFunc — строитель промпта прохода: pass 1 — сухие факты «что происходит»,
// pass 2 — проза. facts — текст прохода фактов (пуст в проходе фактов и для
// однопроходных ролей).
type PromptFunc func(pass int, facts string) string

// GenerateWithRetry пробует слот по лестнице из четырёх ступеней (п.6.2, шаг 4)
// и пишет строку forge_log на каждый модельный отказ. Двухпроходный режим
// сначала куёт факты, затем прозу; отказ прозы оставляет факты (шаг 5).
func GenerateWithRetry(ctx context.Context, lit *db.Handle, opts GenerateOptions, prompt PromptFunc) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Lang == "" {
		opts.Lang = "ru"
	}

	if !opts.TwoPass {
		return ladder(ctx, lit, opts, prompt(2, ""))
	}

	facts, err := ladder(ctx, lit, opts, prompt(1, ""))
	if err != nil {
		return "", err
	}
	prose, err := ladder(ctx, lit, opts, prompt(2, facts))
	if err != nil {
		return facts, nil // отказ прозы — факты выживают (шаг 5).
	}
	return prose, nil
}

// ladder — лестница повторов одного прохода (п.6.2, шаг 4): три генеративные
// ступени и деградация четвёртой.
func ladder(ctx context.Context, lit *db.Handle, opts GenerateOptions, base string) (string, error) {
	for step := 1; step <= 3; step++ {
		provider := opts.Provider
		if step == 3 {
			provider = ProviderLocal
		}
		slot := slotForProvider(provider)
		if slot == nil {
			return "", errors.New("workshop: слот генерации не подключён")
		}
		out, err := slot.Generate(ctx, opts.Role, ladderPrompt(base, step))
		if err != nil {
			return "", err
		}
		if ok, _ := censor.IsRefusal(out, opts.Lang); !ok {
			return out, nil
		}
		if err := writeRefusalLog(lit, opts.Role); err != nil {
			return "", err
		}
	}
	// Шаг 4 — деградация: сцена без выкованного фрагмента, отказ в журнал.
	if err := writeRefusalLog(lit, opts.Role); err != nil {
		return "", err
	}
	return "", ErrForgeDegraded
}

// ladderPrompt строит промпт ступени лестницы: ступень 1 добавляет художественную
// рамку и рейтинг, ступени 2–3 уводят трудную часть за кадр.
func ladderPrompt(base string, step int) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(base), &m); err != nil || m == nil {
		m = map[string]any{"prompt": base}
	}
	switch step {
	case 1:
		m[frameKey] = artisticFrame
		if r := ActiveRating(); r != "" {
			m[ratingKey] = r
		}
	case 2, 3:
		m[offscreenKey] = true
	}
	raw, _ := json.Marshal(m)
	return string(raw)
}

// writeRefusalLog помечает модельный отказ строкой forge_log (п.6.2, шаг 6,
// писатель forge). Роль отказавшего слота пишется в status — колонки провайдера
// и роли в forge_log нет.
func writeRefusalLog(lit *db.Handle, role string) error {
	now := nowUTC()
	return lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		_, err := tx.Exec("forge_log",
			`INSERT INTO forge_log (kind, task, status, started_at, finished_at, log_path, package_version) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			refusalKind, refusalTask, role, now, now, "", version.Version)
		return err
	})
}
