package turn

import (
	"context"
	"fmt"
	"strings"

	"tns65/internal/censor"
	"tns65/internal/workshop"
)

// Отложенные вести двумя формами подачи (IMP-5): весть возврата может прийти
// текстовой записью (как раньше) или сценой с диалогом НПС. Сцена пишется штатным
// путём в scenes, весть остаётся в событиях журнала как раньше; выбор формы —
// детерминированный, вызов модели нужен только сценовой форме.

// newsSceneRole — роль слота мастерской для сцены вести: Редактор — существующая
// роль прозы сцен.
const newsSceneRole = workshop.EditorRole

// defaultNewsSceneMinBatch — меньше вестей за возврат — сцены нет: одиночная
// весть не тянет сцену, она подаётся текстом.
const defaultNewsSceneMinBatch = 2

// newsSceneLang — язык детектора отказа пакета.
const newsSceneLang = "ru"

// newsScenePromptFmt — промпт сцены вести: действующее лицо приносит весть,
// прямая речь с тире в начале строки.
const newsScenePromptFmt = `Напиши короткую сцену мира (6–10 строк): действующее лицо — %s — приносит герою весть.
Весть: %s
Форма: проза с прямой речью, каждая реплика с тире в начале строки. Без заголовков и пояснений, только текст сцены.`

// newsSceneNPCFallback — действующее лицо, если труппа не дала смертного.
const newsSceneNPCFallback = "случайный житель мира"

// NewsSceneConfig — настройка сценовой подачи вестей (IMP-5). Нулевое значение —
// сцена выключена: вести подаются текстом.
type NewsSceneConfig struct {
	Enabled  bool // false — все вести текстом.
	MinBatch int  // меньше вестей за возврат — сцены нет; 0 — дефолт.
}

// DefaultNewsSceneConfig возвращает дефолт: сцена включена, от двух вестей.
func DefaultNewsSceneConfig() NewsSceneConfig {
	return NewsSceneConfig{Enabled: true, MinBatch: defaultNewsSceneMinBatch}
}

// NewsScene — весть, поданная сценой с диалогом НПС (IMP-5).
type NewsScene struct {
	NewsIndex int   // номер вести в пачке возврата; -1 — сцены нет.
	SceneID   int64 // строка scenes; 0 — сцены нет.
	Prose     string
}

// NewsSceneSlot — слот мастерской для сцены вести; nil — слот не подключён.
type NewsSceneSlot interface {
	Generate(ctx context.Context, role, prompt string) (string, error)
}

// sceneNewsIndex выбирает весть для сценовой подачи: сценой идёт последняя весть
// пачки — самый дальний круг, но только когда вестей больше одной. Выбор
// детерминирован: тот же список даёт тот же номер; -1 — сцены нет.
func sceneNewsIndex(news []string, cfg NewsSceneConfig) int {
	if !cfg.Enabled || len(news) == 0 {
		return -1
	}
	min := cfg.MinBatch
	if min <= 0 {
		min = defaultNewsSceneMinBatch
	}
	if len(news) < min {
		return -1
	}
	return len(news) - 1
}

// newsScenePrompt собирает промпт сцены: лицо из труппы приносит весть.
func newsScenePrompt(news string, roster *Roster) string {
	npc := newsSceneNPC(roster)
	if npc == "" {
		npc = newsSceneNPCFallback
	}
	return fmt.Sprintf(newsScenePromptFmt, npc, news)
}

// newsSceneNPC возвращает имя первого смертного труппы — действующего лица сцены.
func newsSceneNPC(r *Roster) string {
	if r == nil {
		return ""
	}
	for _, npc := range r.NPCs {
		if npc.ID == heroCharacterID || npc.Deity || npc.Name == "" {
			continue
		}
		return npc.Name
	}
	return ""
}

// sceneProseUsable проверяет выход слота: непустой текст с прямой речью и не
// отказ модели; иначе сцена — брак, весть уходит текстом.
func sceneProseUsable(prose string) bool {
	if prose == "" || !hasDirectSpeech(prose) {
		return false
	}
	// IsRefusal: true — ответ признан отказом модели, такая проза не сцена.
	refusal, _ := censor.IsRefusal(prose, newsSceneLang)
	return !refusal
}

// hasDirectSpeech сообщает, есть ли в тексте реплика с тире в начале строки.
func hasDirectSpeech(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "—") || strings.HasPrefix(line, "–") || strings.HasPrefix(line, "-") {
			return true
		}
	}
	return false
}
