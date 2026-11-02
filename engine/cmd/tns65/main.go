package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"tns65/internal/birth"
	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/langpack"
	"tns65/internal/netgate"
	"tns65/internal/portgate"
	"tns65/internal/supervisor"
	"tns65/internal/swarm"
	"tns65/internal/version"
	"tns65/internal/webapi"
	"tns65/internal/workshop"
)

// e2eBirthHooks — хуки world birth (--fake-clock/--fake-embed/--fake-slot),
// доступные только в сборке с тегом e2e. В релизной сборке addE2EBirthFlags
// возвращает nil, и флаги не регистрируются (hooks_release.go).
type e2eBirthHooks interface {
	config() birth.Config
	finish(io.Writer)
}

// botMu защищает botStop — отмену текущего Telegram-канала. Канал
// перезапускается из Настроек (PUT /api/settings/channel), поэтому ссылка может
// меняться после старта ядра.
var (
	botMu   sync.Mutex
	botStop context.CancelFunc
)

// startTelegramBot поднимает Telegram-канал из текущих config.yaml/settings.toml,
// гася прежний канал. Ненастроенный канал — не ошибка: бот просто не запускается.
func startTelegramBot(logger *journal.Logger) error {
	cfg, err := webapi.LoadBotConfig("config.yaml", "settings.toml")
	if err != nil {
		return err
	}
	ctx, stop := context.WithCancel(context.Background())
	botMu.Lock()
	if botStop != nil {
		botStop()
	}
	botStop = stop
	botMu.Unlock()

	if err := webapi.StartTelegram(ctx, cfg, logger); err != nil && !errors.Is(err, webapi.ErrTelegramNotConfigured) {
		return err
	}
	return nil
}

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
	if isE2ECoreFlag(args[0]) {
		os.Exit(runE2ECore(args))
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
	case "tunnel":
		if err := runTunnel(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "tns65: unknown command %q\n", args[0])
		os.Exit(2)
	}
}

// run — старт ядра. Порядок: замок одного экземпляра (T-01-14) → порт-гейт
// (T-01-12) → веб-сервер (T-08-02) → надзиратель резидентов (T-01-13). Ядро
// держит резидентов до SIGTERM/SIGINT; замок снимается при выходе из run.
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

	// T-01-11: белый список наружных хостов — единственный путь внешнего HTTP
	// (провайдер, HF, Telegram). Без загрузки список пуст и любой выход наружу
	// получает netgate.deny.
	if err := netgate.Load("config.yaml"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	netgate.SetLogger(logger)

	wl, err := portgate.LoadWhitelist("config.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if code := portgate.Run(wl, logger); code != 0 {
		return code
	}

	// T-08-02: веб-сервер на loopback — статика из embed и JSON/WS-эндпоинты.
	webCfg, err := webapi.LoadConfig("config.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	webCfg.Logger = logger
	webSrv := webapi.New(webCfg)
	if err := webSrv.Listen(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	go func() {
		if err := webSrv.Serve(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "tns65: web server: %v\n", err)
		}
	}()

	// T-08-03/F-04.4: Telegram-бот стартует только когда канал настроен (есть
	// токен и chat_id). Без настройки старт не блокируется и не падает. Хуки
	// перезапуска выставляются после первого старта — их вызывает PUT /api/settings/channel.
	if err := startTelegramBot(logger); err != nil {
		fmt.Fprintf(os.Stderr, "tns65: telegram: %v\n", err)
	}
	webapi.StopTelegram = func() {
		botMu.Lock()
		defer botMu.Unlock()
		if botStop != nil {
			botStop()
			botStop = nil
		}
	}
	webapi.TelegramRestart = func(_ context.Context) error {
		return startTelegramBot(logger)
	}

	// T-01-13: ядро-надзиратель запускает, лечит и останавливает резидентов.
	children, err := supervisor.Load("config.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		webapi.StopTelegram()
		return 1
	}
	sup := supervisor.New(logger, children)
	sup.Run()

	// T-01-13, шаг 4: по SIGTERM и SIGINT дети гасятся в обратном порядке.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	webapi.StopTelegram()
	webSrv.Close()
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
	hooks := addE2EBirthFlags(fs)
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

	cfg := birth.Config{}
	if hooks != nil {
		cfg = hooks.config()
	}
	res, err := birth.Birth(&a, cfg)
	if err != nil {
		return err
	}
	if hooks != nil {
		hooks.finish(os.Stdout)
	}
	fmt.Printf("мир %q создан: %s\n", res.Slug, res.Dir)
	return nil
}

// runTunnel — подкоманда tunnel (T-08-03): читает и проверяет конфигурацию
// исходящего туннеля cloudflared. Без флага --run печатает команду запуска; с
// флагом --run запускает scripts/tunnel.sh, пробрасывая stdout/stderr и
// пересылая SIGINT/SIGTERM дочернему cloudflared. Внешний слушающий адрес
// отвергается (F-04.3).
func runTunnel(args []string) error {
	fs := flag.NewFlagSet("tunnel", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	run := fs.Bool("run", false, "запустить scripts/tunnel.sh")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("tns65 tunnel: unexpected argument %q", fs.Arg(0))
	}

	cfg, err := webapi.LoadBotConfig("config.yaml", "settings.toml")
	if err != nil {
		return fmt.Errorf("tns65 tunnel: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("tns65 tunnel: %w", err)
	}

	if *run {
		return runTunnelScript()
	}

	logger, err := journal.New("state/journal/engine.log")
	if err != nil {
		return err
	}
	webapi.LogTunnelConnect(logger, cfg)
	if cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
		webapi.LogTelegramConnect(logger, cfg)
	}
	fmt.Printf("tunnel: edge=%s local=%s\n", cfg.Edge, cfg.Local)
	if cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
		fmt.Printf("telegram: chat_id=%s mode=long_poll\n", cfg.TelegramChatID)
	} else {
		fmt.Printf("telegram: not configured\n")
	}
	fmt.Printf("run: scripts/tunnel.sh\n")
	return nil
}

// runTunnelScript запускает scripts/tunnel.sh через os/exec: stdout/stderr
// пробрасываются в терминал, SIGINT/SIGTERM пересылаются дочернему процессу
// (после exec это cloudflared). Возврат по завершении скрипта или по сигналу.
func runTunnelScript() error {
	cmd := exec.Command("scripts/tunnel.sh")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("tns65 tunnel: start scripts/tunnel.sh: %w", err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("tns65 tunnel: scripts/tunnel.sh: %w", err)
		}
		return nil
	case s := <-sig:
		_ = cmd.Process.Signal(s)
		<-done
		return nil
	}
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
