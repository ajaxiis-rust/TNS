package workshop

import (
	"encoding/json"

	"tns65/internal/swarm"
)

// ForgeBookRole — имя однозадачной задачи роя ковки книги: tns65 swarm
// forge-book (T-01-15). Каркас ковки книги наследуется целиком: одна ковка —
// один запуск, начало и конец, результат в базу, процесс умер (W-14.2, шаг 4).
const ForgeBookRole = "forge-book"

// ForgeBookKind — kind записей forge_log и artifact о ковке книги. На шаге
// T-05-06 запись в базу не выполняется (шаг 5); kind закрепляет каркас для
// конвейера ковки (T-05-07, писатель forge).
const ForgeBookKind = "forge-book"

// ForgeBookPayload — payload задачи ковки книги: путь к сырому файлу (txt/fb2)
// и слаг мира, в литературную базу которого ляжет выкованное.
type ForgeBookPayload struct {
	InputPath string `json:"input_path"`
	WorldSlug string `json:"world_slug"`
}

// ForgeBookSpec собирает Spec однозадачного запуска ковки книги через рой
// (шаг 4): тот же бинарник tns65, скрытая подкоманда swarm-task, как у задачи
// forge-card (T-05-02). Спеку подставляет в swarm.Launch ядро.
func ForgeBookSpec(exe string, p ForgeBookPayload) swarm.Spec {
	raw, _ := json.Marshal(p)
	return swarm.Spec{
		Name: ForgeBookRole,
		Cmd:  exe,
		Args: []string{"swarm-task", ForgeBookRole, string(raw)},
	}
}
