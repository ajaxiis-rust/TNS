package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"tns65/internal/journal"
	"tns65/internal/langpack"
	"tns65/internal/portgate"
	"tns65/internal/supervisor"
	"tns65/internal/version"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "-version" || args[0] == "--version") {
		fmt.Printf("Version: %s\nBuildDate: %s\n", version.Version, version.BuildDate)
		return
	}
	if len(args) == 0 {
		os.Exit(run())
	}

	switch args[0] {
	case "lang":
		if err := runLang(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "tns65: unknown command %q\n", args[0])
		os.Exit(2)
	}
}

// run — старт ядра. Порядок: замок одного экземпляра (T-01-14) → порт-гейт
// (T-01-12) → надзиратель резидентов (T-01-13). Замок заводит следующая
// задача; здесь ядро сверяет слушающие сокеты с белым списком и держит
// резидентов до SIGTERM/SIGINT.
func run() int {
	logger, err := journal.New("state/journal/engine.log")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	wl, err := portgate.LoadWhitelist("config.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if code := portgate.Run(wl, logger); code != 0 {
		return code
	}

	// T-01-13: ядро-надзиратель запускает, лечит и останавливает резидентов.
	children, err := supervisor.Load("config.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sup := supervisor.New(logger, children)
	sup.Run()

	// T-01-13, шаг 4: по SIGTERM и SIGINT дети гасятся в обратном порядке.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	sup.Stop()
	return 0
}

func runLang(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("tns65 lang: missing command (list)")
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("tns65 lang list: no arguments expected")
		}
		return listPacks()
	default:
		return fmt.Errorf("tns65 lang: unknown command %q", args[0])
	}
}

// listPacks печатает пакеты, версии и текущую (T-01-08, шаг 5).
func listPacks() error {
	items, err := langpack.List("packs")
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.Current {
			fmt.Printf("%s %s (current)\n", it.ID, it.Version)
		} else {
			fmt.Printf("%s %s\n", it.ID, it.Version)
		}
	}
	return nil
}
