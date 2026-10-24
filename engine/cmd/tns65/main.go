package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"tns65/internal/birth"
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
	case "world":
		if err := runWorld(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			var ee *birth.ExitError
			if errors.As(err, &ee) {
				os.Exit(ee.Code)
			}
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "tns65: unknown command %q\n", args[0])
		os.Exit(2)
	}
}

// run — старт ядра. Порядок: замок одного экземпляра (T-01-14) → порт-гейт
// (T-01-12) → надзиратель резидентов (T-01-13). Ядро держит резидентов до
// SIGTERM/SIGINT; замок снимается при выходе из run.
func run() int {
	// T-01-14: захват замка до старта детей. Второй запуск — код 4.
	lock, err := supervisor.Acquire("state/tns65.lock")
	if err != nil {
		if errors.Is(err, supervisor.ErrAlreadyRunning) {
			fmt.Fprintln(os.Stderr, "tns65: core is already running")
			return 4
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer lock.Release()

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

func runWorld(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("tns65 world: missing command (birth)")
	}
	switch args[0] {
	case "birth":
		return runWorldBirth(args[1:])
	default:
		return fmt.Errorf("tns65 world: unknown command %q", args[0])
	}
}

// runWorldBirth — подкоманда world birth --ancketa <файл>: рождение мира по
// анкете без интерфейса (T-02-03). Код выхода: 2 — невалидная анкета,
// 6 — слаг уже существует.
func runWorldBirth(args []string) error {
	fs := flag.NewFlagSet("world birth", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	ancketaPath := fs.String("ancketa", "", "путь к файлу анкеты (JSON)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ancketaPath == "" {
		return fmt.Errorf("tns65 world birth: флаг --ancketa обязателен")
	}

	data, err := os.ReadFile(*ancketaPath)
	if err != nil {
		return fmt.Errorf("tns65 world birth: %w", err)
	}
	var a birth.Ancketa
	if err := json.Unmarshal(data, &a); err != nil {
		return fmt.Errorf("tns65 world birth: чтение анкеты: %w", err)
	}

	res, err := birth.Birth(&a, birth.Config{})
	if err != nil {
		return err
	}
	fmt.Printf("мир %q создан: %s\n", res.Slug, res.Dir)
	return nil
}
