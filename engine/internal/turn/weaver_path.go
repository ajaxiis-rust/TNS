package turn

import (
	"context"
	"strings"
	"time"
)

// Путь Ткача (T-03-08, LT-01): генерация прозы слотом модели со стримингом чанков
// и порогом тишины. Мир считает код, модель проговаривает — промпт собирается из
// фактов сцены (уже закрытых биндером, без токенов-заменителей) и инструкций движка.

// pathWeaver — путь Ткача в конвейере: проза генерируется слотом модели.
const pathWeaver = "weaver"

// SilenceThreshold — порог тишины Ткача (LT-14.1, T-03-08, шаг 3): «молчание» —
// ни одного нового чанка за это время от последнего чанка (или от старта, если
// чанков не было).
const SilenceThreshold = 60 * time.Second

// ProseSlot — слот прозы для пути Ткача: стриминг чанков построчно. Реализация в
// проде — models.Weaver, в тестах подменяется.
type ProseSlot interface {
	Stream(ctx context.Context, prompt string) (<-chan string, error)
}

// Clock — источник времени порога тишины: канал срабатывает по пересечении
// дедлайна на инжектируемых часах (testkit.Clock удовлетворяет контракту).
type Clock interface {
	After(d time.Duration) <-chan time.Time
}

// wallClock — реальные часы по умолчанию (когда тест не подставил testkit.Clock).
type wallClock struct{}

func (wallClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// streamWeaver стримит чанки слота и применяет порог тишины: select по чанку и
// clock.After(SilenceThreshold). При молчании или отказе слота возвращает запасную
// запись fallback — ход не прерван; degraded помечает деградацию слота (молчание,
// отказ или пустой ответ).
func streamWeaver(ctx context.Context, slot ProseSlot, prompt string, clock Clock, fallback string) (string, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if slot == nil {
		return fallback, false
	}
	ch, err := slot.Stream(ctx, prompt)
	if err != nil {
		return fallback, true
	}
	var b strings.Builder
	silence := clock.After(SilenceThreshold)
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				if b.Len() == 0 {
					return fallback, true
				}
				return b.String(), false
			}
			b.WriteString(chunk)
			// Порог отсчитывается от момента последнего чанка.
			silence = clock.After(SilenceThreshold)
		case <-silence:
			return fallback, true
		case <-ctx.Done():
			return fallback, true
		}
	}
}
