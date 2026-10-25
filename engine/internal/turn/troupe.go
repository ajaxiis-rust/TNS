package turn

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"tns65/internal/db"
)

// Труппа (T-03-11, LT-16): персонаж рождается в живом ходе без истории и копит
// память реакций; Чеховское ружьё — политика: всё показанное игроку
// зарегистрировано. Лица труппы приходят из карточек characters оперативной базы
// мира (архетип, голос, внешность), предыстория до второго обращения может
// отсутствовать.

// card — поля карточки, которые Труппа читает из characters.card_json при
// появлении лица. Имена совпадают с карточкой пачки (birth.Card), но Труппа
// разбирает только то, что показывается игроку, без импорта пакета рождения.
type card struct {
	Archetype  string `json:"archetype"`
	Voice      string `json:"voice"`
	Appearance string `json:"appearance"`
	Backstory  string `json:"backstory"`
}

// Реакции игрока на встречу (LT-16.2): что мир фиксирует в памяти реакций.
const (
	ReactionFed     = "подал"
	ReactionChased  = "прогнал"
	ReactionIgnored = "не заметил"
)

// reactionWeight — вес реакции в полосе отношения: подал — к себе, прогнал —
// от себя, не заметил — нейтрально.
func reactionWeight(reaction string) float64 {
	switch reaction {
	case ReactionFed:
		return 1
	case ReactionChased:
		return -1
	default:
		return 0
	}
}

// Чеховское ружьё (LT-16.3): событие регистрации показанного элемента в events.
const (
	gunKind     = "gun_registered"
	gunSource   = "troupe"
	gunLabelNew = "new_element"
)

// gun — показанный элемент (ружьё) и его заряд.
type gun struct {
	CharacterID int64  `json:"character_id,omitempty"`
	Label       string `json:"label"`
	Fired       bool   `json:"fired"`
}

// Troupe — лица мира в живом ходе. Запись идёт хэндлом core: память реакций в
// memory_topics, реестр ружей — событиями events (карта писателей T-01-07).
type Troupe struct {
	db *db.Handle
}

// NewTroupe возвращает Труппу над оперативной базой мира h (писатель core).
func NewTroupe(h *db.Handle) *Troupe {
	return &Troupe{db: h}
}

// LoadRoster собирает труппу из карточек characters: по лицу на строку с именем
// или с готовой к появлению карточкой. Лицо без имени не называется по имени до
// появления; предыстория берётся из карточки, если уже есть.
func (t *Troupe) LoadRoster() (Roster, error) {
	if t.db == nil {
		return Roster{}, nil
	}
	rows, err := t.db.Query(`SELECT id, name, card_json FROM characters ORDER BY id`)
	if err != nil {
		return Roster{}, err
	}
	defer rows.Close()

	var r Roster
	for rows.Next() {
		var id int64
		var name, raw string
		if err := rows.Scan(&id, &name, &raw); err != nil {
			return Roster{}, err
		}
		var c card
		if raw != "" {
			_ = json.Unmarshal([]byte(raw), &c)
		}
		r.NPCs = append(r.NPCs, NPC{
			ID:        id,
			Name:      name,
			Role:      nonEmpty(c.Archetype, c.Voice),
			Group:     c.Appearance,
			Tags:      []string{c.Archetype, c.Voice, c.Appearance},
			Backstory: c.Backstory,
		})
	}
	return r, rows.Err()
}

// Spawn — мгновенное появление персонажа из книги (LT-16.1): архетип, голос и
// внешность из карточки, предыстория пустая до второго обращения. Имя при первом
// появлении не задаётся: назначение имени — следующий шаг живого хода.
func (t *Troupe) Spawn(characterID int64) (NPC, error) {
	var raw string
	if err := t.db.QueryRow(`SELECT card_json FROM characters WHERE id = ?`, characterID).Scan(&raw); err != nil {
		return NPC{}, fmt.Errorf("turn: карточка лица %d: %w", characterID, err)
	}
	var c card
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return NPC{}, fmt.Errorf("turn: карточка лица %d: %w", characterID, err)
	}
	return NPC{
		ID:        characterID,
		Role:      nonEmpty(c.Archetype, c.Voice),
		Group:     c.Appearance,
		Tags:      []string{c.Archetype, c.Voice, c.Appearance},
		Backstory: "", // предыстории при первом появлении нет (LT-16.1).
	}, nil
}

// RecordMeeting пишет память реакций (LT-16.2): при каждой встрече мир фиксирует,
// как игрок отреагировал на лицо — подал, прогнал, не заметил. Это механика
// (memory_topics), не генерация; повторная встреча читает вес в полосе отношения.
func (t *Troupe) RecordMeeting(characterID int64, reaction string) error {
	_, err := t.db.Exec("memory_topics",
		`INSERT INTO memory_topics (character_id, theme, weight, reinforced_at) VALUES (?, ?, ?, ?)`,
		characterID, reaction, reactionWeight(reaction), time.Now().UTC().Format(time.RFC3339))
	return err
}

// RegisterGun регистрирует показанный элемент (ружьё) событием events
// (LT-16.3). Всё показанное игроку попадает в реестр до ввода нового элемента.
func (t *Troupe) RegisterGun(g gun) error {
	payload, err := json.Marshal(g)
	if err != nil {
		return err
	}
	_, err = t.db.Exec("events",
		`INSERT INTO events (day, kind, payload_json, source) VALUES (0, ?, ?, ?)`,
		gunKind, string(payload), gunSource)
	return err
}

// FirstUnfiredGun возвращает первое незаряженное ружьё реестра; false, если все
// показанные элементы уже заряжены (или реестр пуст).
func (t *Troupe) FirstUnfiredGun() (gun, bool, error) {
	rows, err := t.db.Query(`SELECT payload_json FROM events WHERE kind = ? ORDER BY id`, gunKind)
	if err != nil {
		return gun{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return gun{}, false, err
		}
		var g gun
		if err := json.Unmarshal([]byte(raw), &g); err != nil {
			return gun{}, false, err
		}
		if !g.Fired {
			return g, true, nil
		}
	}
	return gun{}, false, rows.Err()
}

// NextElement — планирование нового элемента (LT-16.3): Селектор сначала ищет
// незаряженное ружьё и лишь потом вводит новый элемент. Возвращает показанное
// ружьё (переиспользовано) или, если свободных нет, регистрирует и возвращает
// новый элемент.
func (t *Troupe) NextElement(label string) (gun, error) {
	if label == "" {
		label = gunLabelNew
	}
	if g, ok, err := t.FirstUnfiredGun(); err != nil {
		return gun{}, err
	} else if ok {
		return g, nil
	}
	g := gun{Label: label}
	return g, t.RegisterGun(g)
}

// detectMentions — имена труппы, названные в реплике (по регистронезависимому
// совпадению имени). Пустое имя не совпадает: лицо без имени ещё не появилось.
func detectMentions(roster Roster, text string) []NPC {
	lower := strings.ToLower(text)
	var out []NPC
	for i := range roster.NPCs {
		n := roster.NPCs[i]
		if n.Name != "" && strings.Contains(lower, strings.ToLower(n.Name)) {
			out = append(out, n)
		}
	}
	return out
}
