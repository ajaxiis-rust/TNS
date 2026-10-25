package turn

import "context"

// Страховка хода (T-03-08): ход доставляется всегда. После исполнения пути пустой
// ответ (нештатный случай — отказ или молчание слота, пустой банк) заменяется
// нейтральной записью fallback (T-02-13), ход не прерывается.

// safetyStage — страховка: гарантирует непустой ответ игроку. Путь Ткача и Челнок
// уже подставляют fallback сами; сюда приходит пустой ответ только при нештатном
// ходе — страхуемся нейтральной записью.
type safetyStage struct{}

func (*safetyStage) Name() string { return "safety" }

func (*safetyStage) Run(_ context.Context, st *TurnState) error {
	if st.Reply == "" {
		st.Reply = fallbackReply
	}
	return nil
}
