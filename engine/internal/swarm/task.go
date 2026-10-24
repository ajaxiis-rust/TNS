package swarm

import "errors"

// EnvPackageVersion — имя переменной окружения, через которую ядро передаёт
// задаче свою версию перед запуском (T-01-15, шаг 1).
const EnvPackageVersion = "TNS65_PACKAGE_VERSION"

// ErrVersionMismatch — версия задачи из рукопожатия не совпала с версией ядра:
// задача гасится, работа не начинается (шаг 2).
var ErrVersionMismatch = errors.New("swarm: version mismatch")

// Handshake — первая строка stdout задачи: {"version":"…"} (шаг 1).
type Handshake struct {
	Version string `json:"version"`
}
