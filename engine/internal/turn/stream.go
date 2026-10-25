package turn

import (
	"context"
	"strings"

	"tns65/internal/db"
	"tns65/internal/workshop"
)

// Стриминговая подкладка (T-03-10, LT-06): сцена доставляется в чат порциями по
// мере готовности — мгновенные заготовки подкладываются под живую генерацию Ткача.
// Ветки, не вошедшие в ход, не отбрасываются: сразу после закрытия сцены они
// становятся кандидатами в оседание (заказ sediment в workshop_queue), а в игровые
// таблицы не пишутся (LT-06.8, LT-06.9).

// portionKind — вид порции потока.
type portionKind string

const (
	portionUnderlay portionKind = "underlay" // мгновенная заготовка подкладки.
	portionWeaver   portionKind = "weaver"   // живой фрагмент генерации Ткача.
)

// Portion — порция сцены, доставляемая игроку по мере готовности (шаг 1).
type Portion struct {
	Kind     portionKind
	Text     string
	Degraded bool // только для portionWeaver: молчание/отказ слота — запасная запись.
}

// underlayHook — действие-крючок подкладки: мгновенная заготовка, которую игрок
// видит, пока Ткач генерирует живую речь.
const underlayHook = "В воздухе повисает ожидание — что-то должно произойти.\n"

// underlayPortions — мгновенные заготовки подкладки (шаг 1): описание персонажа
// из книги (первое предложение закрытого промпта сцены) и действие-крючок.
func underlayPortions(st *TurnState) []Portion {
	if st == nil || st.Prepath.Prompt == "" {
		return nil
	}
	desc := firstSentence(st.Prepath.Prompt)
	if desc == "" {
		return nil
	}
	return []Portion{
		{Kind: portionUnderlay, Text: desc + "\n"},
		{Kind: portionUnderlay, Text: underlayHook},
	}
}

// firstSentence — первое предложение текста; пусто, если знака конца нет.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, ".!?…"); i >= 0 {
		return s[:i+1]
	}
	return s
}

// streamScene — инкрементальная доставка сцены (шаг 1): подкладка выходит первой
// порцией до завершения генерации Ткача, следом — живой поток с порогом тишины
// (streamWeaver). Канал закрывается по завершении потока.
func streamScene(ctx context.Context, st *TurnState, slot ProseSlot, prompt string, clock Clock) <-chan Portion {
	ch := make(chan Portion)
	go func() {
		defer close(ch)
		for _, p := range underlayPortions(st) {
			select {
			case ch <- p:
			case <-ctx.Done():
				return
			}
		}
		reply, degraded := streamWeaver(ctx, slot, prompt, clock, fallbackReply)
		select {
		case ch <- Portion{Kind: portionWeaver, Text: reply, Degraded: degraded}:
		case <-ctx.Done():
		}
	}()
	return ch
}

// Виды веток ответа (шаг 2): отрицательная — готовая запись из БД (банк),
// положительная — живой ответ модели (Ткач).
const (
	BranchBank   = "bank"
	BranchWeaver = "weaver"
)

// SedimentCandidate — ссылка на ветку, не вошедшую в ход (шаг 3). До оседания P3
// кандидат существует только строкой workshop_queue и в игровые таблицы не
// пишется (LT-06.8, LT-06.9).
type SedimentCandidate struct {
	TurnID    int64  `json:"turn_id"`
	SessionID int64  `json:"session_id"`
	Kind      string `json:"kind"`
	Text      string `json:"text,omitempty"`
}

// queueSedimentCandidates ставит ветки, не вошедшие в ход, заказом sediment в
// workshop_queue сразу после закрытия сцены (шаг 3). Приоритет 50 задаёт
// workshop.KindPriority; записей в игровые таблицы функция не делает.
func queueSedimentCandidates(h *db.Handle, candidates []SedimentCandidate) error {
	for _, c := range candidates {
		if err := workshop.Enqueue(h, workshop.KindSediment, workshop.StepLight, workshop.DefaultProvider, c); err != nil {
			return err
		}
	}
	return nil
}
