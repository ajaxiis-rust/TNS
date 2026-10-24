// Команда tns65-bake — внеигровое запекание сида (T-01-16, D-04): один раз
// готовит замороженную base.db. Повторный запуск — код 5.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"tns65/internal/bake"
)

func main() {
	os.Exit(run())
}

func run() int {
	dbPath := flag.String("db", "world_db/base/base.db", "путь к base.db")
	seedDir := flag.String("seed", "engine/seed", "каталог сида (narratives/, verses.txt)")
	embedURL := flag.String("embed-url", "http://127.0.0.1:5002", "адрес эмбеддера")
	flag.Parse()

	err := bake.Bake(bake.Config{
		DBPath:   *dbPath,
		SeedDir:  *seedDir,
		EmbedURL: *embedURL,
	})
	if err != nil {
		if errors.Is(err, bake.ErrAlreadyBaked) {
			fmt.Fprintln(os.Stderr, "сид уже запечён")
			return 5
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
