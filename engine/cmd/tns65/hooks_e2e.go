//go:build e2e

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"tns65/internal/birth"
	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/testkit"
	"tns65/internal/webapi"
)

// e2eBirthHooksImpl — реализация хуков world birth (T-09-01): детерминированный
// эмбеддер, счётчик вызовов слота прозы и журнал для пометки принудительного
// якоря. Флаги --fake-* регистрируются только в сборке с тегом e2e.
type e2eBirthHooksImpl struct {
	fakeEmbed    *bool
	fakeSlot     *bool
	proseCalls   int
	anchorForced bool
}

func addE2EBirthFlags(fs *flag.FlagSet) e2eBirthHooks {
	h := &e2eBirthHooksImpl{}
	h.fakeEmbed = fs.Bool("fake-embed", false, "детерминированный эмбеддер 1024 (e2e)")
	h.fakeSlot = fs.Bool("fake-slot", false, "счётчик вызовов слота прозы (e2e)")
	_ = fs.Bool("fake-clock", false, "фейковые часы (e2e)")
	return h
}

// config строит конфигурацию рождения после разбора флагов: подменяет эмбеддер
// и подключает счётчик вызовов слота прозы (гвардия B-04.1). Помётка
// принудительного якоря (B-10) пишется только в журнал рождения — подписываемся
// на событие, файл журнала кладём в корень рабочих данных мира.
func (h *e2eBirthHooksImpl) config() birth.Config {
	cfg := birth.Config{}
	if *h.fakeEmbed {
		cfg.Embed = testkit.NewFakeEmbed()
	}
	if *h.fakeSlot {
		cfg.ProseCalls = &h.proseCalls
	}
	if *h.fakeEmbed {
		if logger, err := journal.New(filepath.Join(db.WorldsRoot, "e2e-birth.log")); err == nil {
			logger.OnEvent(func(event string) {
				if event == journal.EventAnchorForced {
					h.anchorForced = true
				}
			})
			cfg.Logger = logger
		}
	}
	return cfg
}

// finish печатает машиночитаемый итог рождения в stdout — его читает e2e-тест.
func (h *e2eBirthHooksImpl) finish(w io.Writer) {
	_ = json.NewEncoder(w).Encode(e2eBirthReport{
		LLMCalls:     h.proseCalls,
		AnchorForced: h.anchorForced,
	})
}

// e2eBirthReport — итог рождения с хуками (T-09-01, «Проверка»).
type e2eBirthReport struct {
	LLMCalls     int  `json:"llm_calls"`
	AnchorForced bool `json:"anchor_forced"`
}

func isE2ECoreFlag(arg string) bool {
	switch arg {
	case "--fake-clock", "--fake-embed", "--fake-slot":
		return true
	}
	return false
}

// runE2ECore принимает хуки ядра и стартует его штатно. Фейковые часы
// подключаются к серверу хода и контрольному эндпоинту /api/e2e/clock
// (T-09-02): время движется только вызовом эндпоинта, реального ожидания нет.
func runE2ECore(args []string) int {
	fs := flag.NewFlagSet("tns65", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fakeClock := fs.Bool("fake-clock", false, "фейковые часы (e2e)")
	_ = fs.Bool("fake-embed", false, "детерминированный эмбеддер 1024 (e2e)")
	_ = fs.Bool("fake-slot", false, "счётчик вызовов слота прозы (e2e)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *fakeClock {
		webapi.SetE2EClock(testkit.NewClock())
	}
	return run()
}
