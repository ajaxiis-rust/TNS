package webapi

// Единственный пользователь и отсутствие аутентификации (T-08-04, шаги 1–2,
// UI-03): интерфейс открывается сразу, уровней доступа нет, скрытой
// админ-зоны нет; станочное доступно только через «Настройки» и «MCP».

// authModeNone — режим доступа webapi: без входа и без ролей.
const authModeNone = "none"

// accessLevels — пусто: уровней доступа нет, весь интерфейс доступен игроку.
var accessLevels = []string{}

// adminZonePaths — пусто: скрытой админ-зоны нет.
var adminZonePaths = []string{}

// routeRequiresAuth — false для любого маршрута: аутентификация не применяется.
func routeRequiresAuth(_, _ string) bool { return false }
