package turn

import "strings"

// Command — распознанная слеш-команда реплики.
type Command struct {
	Name string // имя команды со слешем, например "/статус".
	Args string // остаток реплики после имени (может быть пуст).
}

// knownCommands — реестр слеш-команд (T-03-01, шаг 4).
var knownCommands = []string{"/статус", "/осмотр", "/квесты", "/помощь"}

// parseCommand распознаёт слеш-команду в начале реплики: реплика равна команде
// или начинается с «команды + пробел». Всё, что не похоже на команду, остаётся
// прозой — прозаический путь не отключается.
func parseCommand(text string) (Command, bool) {
	t := strings.TrimSpace(text)
	for _, name := range knownCommands {
		if t == name {
			return Command{Name: name}, true
		}
		if strings.HasPrefix(t, name+" ") {
			return Command{Name: name, Args: strings.TrimSpace(t[len(name):])}, true
		}
	}
	return Command{}, false
}

// commandReply — служебный ответ слеш-команды (заглушка до контента стадий P2).
func commandReply(name string) string {
	switch name {
	case "/статус":
		return "Мир жив, и ты в нём. День ещё длится."
	case "/осмотр":
		return "Ты осматриваешься: вокруг знакомые очертания места."
	case "/квесты":
		return "Заданий пока нет."
	case "/помощь":
		return "Команды: /статус, /осмотр, /квесты, /помощь."
	default:
		return fallbackReply
	}
}
