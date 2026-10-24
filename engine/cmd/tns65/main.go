package main

import (
	"fmt"
	"os"

	"tns65/internal/langpack"
	"tns65/internal/version"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "-version" || args[0] == "--version") {
		fmt.Printf("Version: %s\nBuildDate: %s\n", version.Version, version.BuildDate)
		return
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: tns65 [--version] [lang list]")
		os.Exit(2)
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
