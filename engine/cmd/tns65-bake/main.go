// Команда tns65-bake — легаси-пекарь (TUN-8): один раз готовит замороженную
// слитую base.db из сида. Заменён devForge — базы поставляются готовыми
// base.<lang>.db в engine/base/, рантайм слитую базу не читает. Совместимость
// сохранена: при старте печатается предупреждение, команда продолжает работу.
// Повторный запуск — код 5.
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
	fmt.Fprintln(os.Stderr, "заменён devForge: базы поставляются готовыми base.<lang>.db в engine/base/")
	dbPath := flag.String("db", "state/legacy-base.db", "путь к легаси-слитой base.db (данные станка вне поставки)")
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
