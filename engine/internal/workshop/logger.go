// Журнал станка мастерской (FIX-5): модельные шаги Редактора и отказы Цензора
// правок пишутся событиями журнала. Прод-сборка задаёт журнал один раз (как
// netgate.SetLogger); без журнала события не пишутся.
package workshop

import (
	"sync"

	"tns65/internal/journal"
)

var (
	logMu     sync.RWMutex
	curLogger *journal.Logger
)

// SetLogger задаёт журнал станка для событий мастерской; nil — без событий.
func SetLogger(l *journal.Logger) {
	logMu.Lock()
	defer logMu.Unlock()
	curLogger = l
}

// loggerOf возвращает журнал станка мастерской; nil — события не пишутся.
func loggerOf() *journal.Logger {
	logMu.RLock()
	defer logMu.RUnlock()
	return curLogger
}
