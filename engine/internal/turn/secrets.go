package turn

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

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

// heroStateLayer — слой состояния героя для чтения телесных статов (T-19-14);
// полный слой состояния рождается в birth, здесь читаются только восемь статов
// тела (без импорта пакета рождения).
type heroStateLayer struct {
	Health  int `json:"health"`
	Hunger  int `json:"hunger"`
	Thirst  int `json:"thirst"`
	Fatigue int `json:"fatigue"`
	Comfort int `json:"comfort"`
	Spirit  int `json:"spirit"`
	Bladder int `json:"bladder"`
	Bowel   int `json:"bowel"`
}

// HeroBodyStats — восемь телесных статов героя 0–100 (T-19-14, B-07.1);
// здоровье входит в набор.
type HeroBodyStats struct {
	Health  int
	Hunger  int
	Thirst  int
	Fatigue int
	Comfort int
	Spirit  int
	Bladder int
	Bowel   int
}

// stateLayer — имя слоя состояния в character_layers.
const stateLayer = "state"

// readHeroBodyStats читает восемь телесных статов героя из слоя состояния
// character_layers. false — статы неизвестны (слоя нет).
func readHeroBodyStats(h *db.Handle) (HeroBodyStats, bool, error) {
	if h == nil {
		return HeroBodyStats{}, false, nil
	}
	var raw string
	err := h.QueryRow(
		`SELECT payload_json FROM character_layers WHERE character_id = ? AND layer = ?`,
		heroCharacterID, stateLayer).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return HeroBodyStats{}, false, nil
	}
	if err != nil {
		return HeroBodyStats{}, false, err
	}
	var st heroStateLayer
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return HeroBodyStats{}, false, err
	}
	return HeroBodyStats{
		Health:  st.Health,
		Hunger:  st.Hunger,
		Thirst:  st.Thirst,
		Fatigue: st.Fatigue,
		Comfort: st.Comfort,
		Spirit:  st.Spirit,
		Bladder: st.Bladder,
		Bowel:   st.Bowel,
	}, true, nil
}

// readHeroHealth читает здоровье героя из того же слоя состояния, что и
// телесные статы. false — здоровье неизвестно (слоя нет): смерть не навязывается.
func readHeroHealth(h *db.Handle) (int, bool, error) {
	st, ok, err := readHeroBodyStats(h)
	if err != nil || !ok {
		return 0, false, err
	}
	return st.Health, true, nil
}

// willLayer — имя слоя завещания в character_layers (писатель core, карта T-01-07):
// завещание живёт слоем героя, отдельная таблица и миграция не нужны (T-19-03).
const willLayer = "will"

// Will — завещание героя (T-19-03, LT-16.7): текст последних слов со сцены
// прощания. Мир держит его крючком следующей сцены.
type Will struct {
	Text string `json:"text"`
}

// saveWill пишет завещание героя; пустой текст завещанием не считается.
// Последние слова перекрывают прежние: завещание — то, что сказано последним.
func saveWill(h *db.Handle, text string) error {
	if h == nil || strings.TrimSpace(text) == "" {
		return nil
	}
	raw, err := json.Marshal(Will{Text: text})
	if err != nil {
		return err
	}
	if _, err := h.Exec("character_layers",
		`DELETE FROM character_layers WHERE character_id = ? AND layer = ?`,
		heroCharacterID, willLayer); err != nil {
		return err
	}
	_, err = h.Exec("character_layers",
		`INSERT INTO character_layers (character_id, layer, payload_json) VALUES (?, ?, ?)`,
		heroCharacterID, willLayer, string(raw))
	return err
}

// readWill читает завещание героя; false — завещания нет, поведение мира прежнее.
func readWill(h *db.Handle) (Will, bool, error) {
	if h == nil {
		return Will{}, false, nil
	}
	var raw string
	err := h.QueryRow(
		`SELECT payload_json FROM character_layers WHERE character_id = ? AND layer = ?`,
		heroCharacterID, willLayer).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Will{}, false, nil
	}
	if err != nil {
		return Will{}, false, err
	}
	var w Will
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		return Will{}, false, err
	}
	return w, strings.TrimSpace(w.Text) != "", nil
}
