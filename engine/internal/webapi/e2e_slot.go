//go:build e2e

package webapi

import (
	"strings"
	"unicode"

	"tns65/internal/lemmarpc"
	"tns65/internal/turn"
)

// SetE2ESlot подключает слот Ткача к подставному провайдеру (INT-3): адрес
// httptest-сервера теста, говорящего протоколом /completion.
func SetE2ESlot(s turn.ProseSlot) {
	chatWeaver = s
}

// SetE2ELemmatizer подключает подставной лемматизатор (INT-3): словоформы
// движения и речи сводятся к леммам лексикона, прочее — к нижнему регистру.
func SetE2ELemmatizer() {
	chatLemmatizer = e2eLemmatizer{}
}

type e2eLemmatizer struct{}

// e2eLemmaForms — словоформы движения, по которым предпуть строит гипотезу
// намерения «перемещение» без живого лемматизатора (INT-3).
var e2eLemmaForms = map[string]string{
	"иду": "идти", "идти": "идти", "идёт": "идти", "шёл": "идти",
	"еду": "ехать", "ехать": "ехать",
	"плыву": "плыть", "плыть": "плыть",
	"бегу": "бежать", "бежать": "бежать",
}

func (e2eLemmatizer) Lemmatize(text string) ([]lemmarpc.Token, error) {
	var toks []lemmarpc.Token
	for _, w := range strings.Fields(text) {
		key := strings.Map(func(r rune) rune {
			if unicode.IsPunct(r) {
				return -1
			}
			return r
		}, strings.ToLower(w))
		lemma := e2eLemmaForms[key]
		if lemma == "" {
			lemma = key
		}
		if lemma == "" {
			continue
		}
		toks = append(toks, lemmarpc.Token{Form: w, Lemma: lemma})
	}
	return toks, nil
}
