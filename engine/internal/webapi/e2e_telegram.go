//go:build e2e

package webapi

import "strings"

// SetE2ETelegramBase подменяет базу Bot API локальным фейком (INT-11): харнесс
// поднимает httptest-сервер и настраивает канал через настройки, живой Telegram
// не нужен.
func SetE2ETelegramBase(base string) {
	telegramBaseURL = strings.TrimRight(base, "/")
}
