package turn

import (
	"database/sql"
	"encoding/json"
	"errors"

	"tns65/internal/db"
)

// Тайны и порог смерти (T-03-19, LT-16.4–LT-16.7): истинная роль персонажа при
// рождении не фиксируется — тайна назначается позже из «полотна тайн» при
// драматургической выгоде. У каждой тайны два флага — «намекать» и «раскрывать»;
// в живом ходе флаги не меняются, смена — распоряжение Селектора при планировании.

// Secret — тайна полотна: имя и два флага.
type Secret struct {
	Name   string `json:"name"`
	Hint   bool   `json:"hint"`
	Reveal bool   `json:"reveal"`
}

// AssignedSecret — назначенная тайна лица в памяти хода.
type AssignedSecret struct {
	CharacterID int64
	Secret      Secret
}

// secretLayer — имя слоя тайны в character_layers (писатель core, карта T-01-07).
const secretLayer = "secret"

// Полотно тайн (LT-16.4): реестр заранее сваренных тайн. Флаги заданы заводски
// и в живом ходе не меняются.
var secretCanvas = []Secret{
	{Name: "переодетый принц", Hint: true, Reveal: false},
	{Name: "беглый каторжник", Hint: true, Reveal: false},
	{Name: "агент фракции", Hint: true, Reveal: false},
}

// SecretFor возвращает тайну полотна по имени; false — такой тайны нет.
func SecretFor(name string) (Secret, bool) {
	for _, s := range secretCanvas {
		if s.Name == name {
			return s, true
		}
	}
	return Secret{}, false
}

// Secrets — назначение и чтение тайн в оперативной базе мира (писатель core).
type Secrets struct {
	db *db.Handle
}

// NewSecrets возвращает держателя тайн над оперативной базой h.
func NewSecrets(h *db.Handle) *Secrets {
	return &Secrets{db: h}
}

// Assign назначает тайну персонажу. Новая тайна переосмысливает показанное, но
// не противоречит ему (LT-16.4): у лица одна тайна — прежняя запись заменяется.
func (s *Secrets) Assign(characterID int64, sec Secret) error {
	if s.db == nil {
		return nil
	}
	if _, err := s.db.Exec("character_layers",
		`DELETE FROM character_layers WHERE character_id = ? AND layer = ?`,
		characterID, secretLayer); err != nil {
		return err
	}
	raw, err := json.Marshal(sec)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("character_layers",
		`INSERT INTO character_layers (character_id, layer, payload_json) VALUES (?, ?, ?)`,
		characterID, secretLayer, string(raw))
	return err
}

// Assigned возвращает назначенную тайну лица; false — тайны ещё нет.
func (s *Secrets) Assigned(characterID int64) (Secret, bool, error) {
	if s.db == nil {
		return Secret{}, false, nil
	}
	var raw string
	err := s.db.QueryRow(
		`SELECT payload_json FROM character_layers WHERE character_id = ? AND layer = ?`,
		characterID, secretLayer).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Secret{}, false, nil
	}
	if err != nil {
		return Secret{}, false, err
	}
	var sec Secret
	if err := json.Unmarshal([]byte(raw), &sec); err != nil {
		return Secret{}, false, err
	}
	return sec, true, nil
}

// deathHealthThreshold — порог смерти (LT-16.7): при падении здоровья героя до 2%
// движок гарантирует сцену прощания; до порога смерть не навязывается.
const deathHealthThreshold = 2

// farewellReply — заготовка сцены прощания (LT-16.7): время на диалог и завещание.
const farewellReply = "Настало время прощания: мир ждёт твоих последних слов и завещания."

// deathThresholdHit — здоровье героя упало до порога смерти.
func deathThresholdHit(health int) bool {
	return health <= deathHealthThreshold
}

// heroStateLayer — минимальный слой состояния героя для чтения здоровья; полный
// слой состояния рождается в birth, здесь читается только health (без импорта
// пакета рождения).
type heroStateLayer struct {
	Health int `json:"health"`
}

// stateLayer — имя слоя состояния в character_layers.
const stateLayer = "state"

// readHeroHealth читает здоровье героя из слоя состояния character_layers.
// false — здоровье неизвестно (слоя нет): смерть не навязывается.
func readHeroHealth(h *db.Handle) (int, bool, error) {
	if h == nil {
		return 0, false, nil
	}
	var raw string
	err := h.QueryRow(
		`SELECT payload_json FROM character_layers WHERE character_id = ? AND layer = ?`,
		heroCharacterID, stateLayer).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	var st heroStateLayer
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return 0, false, err
	}
	return st.Health, true, nil
}
