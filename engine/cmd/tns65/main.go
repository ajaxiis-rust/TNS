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
	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/langpack"
	"tns65/internal/portgate"
	"tns65/internal/supervisor"
	"tns65/internal/swarm"
	"tns65/internal/version"
	"tns65/internal/workshop"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "-version" || args[0] == "--version") {
		fmt.Printf("Version: %s\nBuildDate: %s\n", version.Version, version.BuildDate)
		return
	}
	// Скрытая точка входа однозадачной задачи роя (T-01-15, T-05-02): задача
	// выполняется тем же бинарником tns65, отдельный бинарник не заводится.
	if len(args) > 0 && args[0] == "swarm-task" {
		os.Exit(runSwarmTask(args[1:]))
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
	case "swarm":
		if err := runSwarm(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
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
		return fmt.Errorf("tns65 world: missing command (birth, list)")
	}
	switch args[0] {
	case "birth":
		return runWorldBirth(args[1:])
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("tns65 world list: no arguments expected")
		}
		return runWorldList()
	default:
		return fmt.Errorf("tns65 world: unknown command %q", args[0])
	}
}

// runWorldList — подкоманда world list: таблица «слаг — заголовок — дата», по
// строке на готовый мир (T-02-10).
func runWorldList() error {
	worlds, err := birth.List("")
	if err != nil {
		return err
	}
	for _, w := range worlds {
		fmt.Printf("%s — %s — %s\n", w.Slug, w.Title, w.BirthDate.Format("2006-01-02"))
	}
	return nil
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

// runSwarm — точка запуска однозадачных задач роя (T-01-15, T-05-02):
// tns65 swarm <task>. Отдельный бинарник для задач не заводится — задача
// выполняется тем же бинарником tns65 перезапуском со скрытой подкомандой
// swarm-task; ядро сверяет версию по рукопожатию (swarm.Launch).
func runSwarm(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("tns65 swarm: missing task (forge-card)")
	}
	switch args[0] {
	case "forge-card":
		return launchSwarmTask(args[1:])
	default:
		return fmt.Errorf("tns65 swarm: unknown task %q", args[0])
	}
}

// launchSwarmTask запускает задачу ковки карточки роя: аргументы задачи —
// литературная база мира и payload заказа complement.
func launchSwarmTask(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("tns65 swarm forge-card: expected <literary_db> <payload_json>")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logger, err := journal.New("state/journal/swarm.log")
	if err != nil {
		return err
	}
	res, err := swarm.Launch(logger, swarm.Spec{
		Name: "forge-card",
		Cmd:  exe,
		Args: []string{"swarm-task", "forge-card", args[0], args[1]},
	}, version.Version)
	if err != nil {
		return err
	}
	fmt.Print(string(res.Output))
	return nil
}

// runSwarmTask — скрытая точка входа однозадачной задачи роя: первой строкой
// stdout печатает рукопожатие, затем выполняет работу и печатает результат.
func runSwarmTask(args []string) int {
	hs, _ := json.Marshal(swarm.Handshake{Version: version.Version})
	fmt.Println(string(hs))
	if len(args) != 2 || args[0] != "forge-card" {
		return 2
	}

	literary, err := db.OpenWriterAs(args[0], db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer literary.Close()

	var payload workshop.ComplementPayload
	if err := json.Unmarshal([]byte(args[1]), &payload); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	card, err := workshop.RunForgeCard(literary, payload)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Print(card)
	return 0
}
