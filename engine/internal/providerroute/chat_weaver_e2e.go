//go:build e2e

package providerroute

import (
	"tns65/internal/journal"
	"tns65/internal/turn"
)

// DefaultChatWeaver в e2e-сборке слот не подключает: подставной слот ставит
// хук --fake-slot (SetE2ESlot) до старта ядра, а SetChatWeaver не перезаписывает
// уже подключённый слот.
func DefaultChatWeaver(_ *journal.Logger) turn.ProseSlot {
	return nil
}
