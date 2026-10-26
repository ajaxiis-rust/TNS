package sediment

import "errors"

// Token — слот-токен оседания (GLO-1): placeholder в записи банка в фигурных скобках.
type Token string

// Закрытый набор слот-токенов оседания (T-04-01, шаг 3).
const (
	TokenName  Token = "{NAME}"
	TokenThing Token = "{THING}"
	TokenPrice Token = "{PRICE}"
	TokenPlace Token = "{PLACE}"
)

// slotBinding — строка реестра соответствия: слот-токен ↔ токен-заменитель биндера.
type slotBinding struct {
	Slot   Token
	Binder string
}

// slotBindings — единый реестр соответствия слот-токенов оседания токенам-заменителям
// биндера (T-04-01, шаг 3; GLO-1). Пополнение — только правкой этого перечня:
// новый слот-токен добавляется сюда, а не конструктором.
var slotBindings = []slotBinding{
	{Slot: TokenName, Binder: "[NPCname]"},
	{Slot: TokenThing, Binder: "[THING]"},
	{Slot: TokenPrice, Binder: "[PRICE]"},
	{Slot: TokenPlace, Binder: "[PLACE]"},
}

// ErrUnknownSlotToken — токен вне закрытого набора слот-токенов.
var ErrUnknownSlotToken = errors.New("sediment: слот-токен вне набора")

// ParseToken принимает токен только из закрытого набора; иначе ErrUnknownSlotToken.
func ParseToken(s string) (Token, error) {
	for _, b := range slotBindings {
		if string(b.Slot) == s {
			return b.Slot, nil
		}
	}
	return "", ErrUnknownSlotToken
}

// SlotTokens — слот-токены закрытого набора в порядке реестра.
func SlotTokens() []Token {
	out := make([]Token, 0, len(slotBindings))
	for _, b := range slotBindings {
		out = append(out, b.Slot)
	}
	return out
}

// BinderToken возвращает токен-заменитель биндера для слот-токена из реестра.
// Для токена вне набора возвращает пустую строку.
func BinderToken(t Token) string {
	for _, b := range slotBindings {
		if b.Slot == t {
			return b.Binder
		}
	}
	return ""
}
