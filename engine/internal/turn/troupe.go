package turn

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"tns65/internal/cast"
	"tns65/internal/db"
	"tns65/internal/prompts"
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
	Archetype  string   `json:"archetype"`
	Voice      string   `json:"voice"`
	Appearance string   `json:"appearance"`
	Backstory  string   `json:"backstory"`
	Idiolect   idiolect `json:"idiolect"`
}

// idiolect — идиолект лица из карточки (B-15, T-19-08): лексикон, речевые
// привычки и табу. Поля совпадают с birth.Idiolect; табу хранятся основами.
type idiolect struct {
	Lexicon []string `json:"lexicon"`
	Habits  []string `json:"habits"`
	Taboo   []string `json:"taboo"`
}

// Метки тегов идиолекта (T-19-08, шаг 3): сцена различает словарь, привычки и
// табу лица по префиксу тега.
const (
	idiolectLexiconTag = "лексикон:"
	idiolectHabitTag   = "привычка:"
	idiolectTabooTag   = "табу:"
)

// idiolectTags раскладывает идиолект в теги лица: словарь, привычки и табу идут
// отдельными тегами, чтобы сцена получила речь лица целиком.
func idiolectTags(id idiolect) []string {
	var tags []string
	for _, w := range id.Lexicon {
		if w = strings.TrimSpace(w); w != "" {
			tags = append(tags, idiolectLexiconTag+w)
		}
	}
	for _, h := range id.Habits {
		if h = strings.TrimSpace(h); h != "" {
			tags = append(tags, idiolectHabitTag+h)
		}
	}
	for _, t := range id.Taboo {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, idiolectTabooTag+t)
		}
	}
	return tags
}

// tabooHit возвращает табу-основу, встреченную в реплике отдельным словом;
// пусто — табу нет (T-19-08). Сверка по началу слова, регистр и «ё» не важны.
func tabooHit(taboo []string, speech string) string {
	norm := func(s string) string {
		return strings.ToLower(strings.NewReplacer("ё", "е").Replace(strings.TrimSpace(s)))
	}
	words := strings.FieldsFunc(norm(speech), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, t := range taboo {
		t = norm(t)
		if t == "" {
			continue
		}
		for _, w := range words {
			if strings.HasPrefix(w, t) {
				return t
			}
		}
	}
	return ""
}

// tagValues — значения тегов лица с префиксом: идиолект лежит в общих тегах
// метками «лексикон:», «привычка:», «табу:» (T-19-08).
func tagValues(tags []string, prefix string) []string {
	var out []string
	for _, t := range tags {
		if v, ok := strings.CutPrefix(t, prefix); ok {
			if v = strings.TrimSpace(v); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

// idiolectPrompt — идиолект лица строкой промпта сцены (B-15, правило 4):
// словарь, привычки и табу из тегов лица. Пусто — идиолекта нет.
func idiolectPrompt(tags []string) string {
	var parts []string
	if v := tagValues(tags, idiolectLexiconTag); len(v) > 0 {
		parts = append(parts, "лексикон — "+strings.Join(v, ", "))
	}
	if v := tagValues(tags, idiolectHabitTag); len(v) > 0 {
		parts = append(parts, "привычки — "+strings.Join(v, "; "))
	}
	if v := tagValues(tags, idiolectTabooTag); len(v) > 0 {
		parts = append(parts, "табу — "+strings.Join(v, "; "))
	}
	if len(parts) == 0 {
		return ""
	}
	return prompts.Render("", "idiolect", defaultIdiolectPrompt, map[string]any{"parts": strings.Join(parts, "; ")})
}

// defaultIdiolectPrompt — дефолт строки идиолекта лица в промпте сцены (B-15).
const defaultIdiolectPrompt = "Идиолект: {parts}."

// SpeechAllowed — словарный гейт прямой речи (T-19-08, шаг 4): табу идиолекта
// лица не попадает в его реплику. Табу читаются из тегов лица, которые Труппа
// кладёт из карточки, поэтому гейт работает на сцене без чтения карточки.
func SpeechAllowed(tags []string, speech string) bool {
	return tabooHit(tagValues(tags, idiolectTabooTag), speech) == ""
}

// speechWithoutTaboo отдаёт сцену без реплик лиц с табу-основой в прямой речи
// (T-19-08, шаг 4). Реплика сверяется с табу того лица труппы, которое названо
// в ней самой или в соседней строке; реплика без опознанного лица не
// отклоняется — гейт держит речь лиц карточек.
func speechWithoutTaboo(roster Roster, scene string) string {
	lines := strings.Split(scene, "\n")
	kept := make([]string, 0, len(lines))
	for i, line := range lines {
		if !hasDirectSpeech(line) {
			kept = append(kept, line)
			continue
		}
		if npc := speechOwner(roster, lines, i); npc != nil && !SpeechAllowed(npc.Tags, line) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// speechWithoutTabooBy отдаёт текст без реплик с табу-основой названного лица
// (T-19-08, шаг 4): сцена вести — речь одного лица, поэтому гейт идёт по его
// тегам, а не по именам в строках.
func speechWithoutTabooBy(tags []string, scene string) string {
	if len(tagValues(tags, idiolectTabooTag)) == 0 {
		return scene
	}
	lines := strings.Split(scene, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if hasDirectSpeech(line) && !SpeechAllowed(tags, line) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// speechOwner — лицо труппы, которому принадлежит реплика с номером i: имя лица
// ищется в самой реплике и в соседних строках — ремарке «— сказал Старик» или
// строке действия перед репликой.
func speechOwner(roster Roster, lines []string, i int) *NPC {
	for _, j := range [...]int{i, i - 1, i + 1} {
		if j < 0 || j >= len(lines) {
			continue
		}
		lower := strings.ToLower(lines[j])
		for k := range roster.NPCs {
			n := &roster.NPCs[k]
			if n.Name != "" && strings.Contains(lower, strings.ToLower(n.Name)) {
				return n
			}
		}
	}
	return nil
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
	db         *db.Handle
	baseDBPath string     // путь базы языка base.<lang>.db для пула имён (TUN-5).
	lang       string     // язык мира для выбора имён; пусто — пул пропущен.
	litDB      *db.Handle // литературная база мира для описаний по имени (T-12-25).
}

// NewTroupe возвращает Труппу над оперативной базой мира h (писатель core).
func NewTroupe(h *db.Handle) *Troupe {
	return &Troupe{db: h}
}

// WithNamePool подключает пул имён из базы языка (TUN-5): пустое имя карточки
// назначается из base.<lang>.db при появлении. Пустые baseDBPath или lang
// оставляют деградацию без имени — текущее поведение.
func (t *Troupe) WithNamePool(baseDBPath, lang string) *Troupe {
	t.baseDBPath = baseDBPath
	t.lang = lang
	return t
}

// WithLiterary подключает литературную базу мира (T-12-25): пустая карточка
// отдаёт описание героя из книги по имени (LT-06.2, P12 §5.9 п.7). nil — описания
// не подставляются.
func (t *Troupe) WithLiterary(lit *db.Handle) *Troupe {
	t.litDB = lit
	return t
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
			Tags:      append([]string{c.Archetype, c.Voice, c.Appearance}, idiolectTags(c.Idiolect)...),
			Backstory: c.Backstory,
		})
	}
	return r, rows.Err()
}

// Spawn — мгновенное появление персонажа из книги (LT-16.1): архетип, голос и
// внешность из карточки, предыстория пустая до второго обращения. Пустое имя
// карточки (B-05.3) назначается из пула базы языка, если он подключён.
func (t *Troupe) Spawn(characterID int64) (NPC, error) {
	var name, raw string
	if err := t.db.QueryRow(`SELECT name, card_json FROM characters WHERE id = ?`, characterID).Scan(&name, &raw); err != nil {
		return NPC{}, fmt.Errorf("turn: карточка лица %d: %w", characterID, err)
	}
	var c card
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return NPC{}, fmt.Errorf("turn: карточка лица %d: %w", characterID, err)
	}
	if name == "" {
		picked, err := t.assignPoolName(characterID)
		if err != nil {
			return NPC{}, err
		}
		name = picked
	}
	appearance := c.Appearance
	if appearance == "" && name != "" {
		// Пустая карточка по имени (T-12-25): описания в карточке нет —
		// подставляется описание героя из книги (LT-06.2).
		if desc, ok := t.BookDescription(name); ok {
			appearance = desc
		}
	}
	return NPC{
		ID:        characterID,
		Name:      name,
		Role:      nonEmpty(c.Archetype, c.Voice),
		Group:     appearance,
		Tags:      append([]string{c.Archetype, c.Voice, appearance}, idiolectTags(c.Idiolect)...),
		Backstory: "", // предыстории при первом появлении нет (LT-16.1).
	}, nil
}

// BookDescription возвращает описание героя книги по имени (регистронезависимо)
// из литературной базы мира; литература не подключена или героя нет — ("", false).
// Подстановка декоративная: ошибка чтения гасится в false (LT-06.2).
func (t *Troupe) BookDescription(name string) (string, bool) {
	return cast.DescriptionByName(t.litDB, name)
}

// assignPoolName назначает пустое имя карточки из пула базы языка и пишет его в
// characters хэндлом core (TUN-5, шаг 2). Пустой пул или неполная проводка
// оставляют имя пустым — имя остаётся назначаться позже.
func (t *Troupe) assignPoolName(characterID int64) (string, error) {
	if t.baseDBPath == "" || t.lang == "" {
		return "", nil
	}
	taken, err := t.takenNames()
	if err != nil {
		return "", err
	}
	name, err := PickPoolName(t.baseDBPath, t.lang, taken)
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", nil
	}
	if err := t.db.Tx(db.WriterCore, func(tx *db.Tx) error {
		_, err := tx.Exec("characters", `UPDATE characters SET name = ? WHERE id = ?`, name, characterID)
		return err
	}); err != nil {
		return "", err
	}
	return name, nil
}

// takenNames собирает занятые имена карточек мира, чтобы пул не выдал дубль.
func (t *Troupe) takenNames() (map[string]bool, error) {
	rows, err := t.db.Query(`SELECT name FROM characters WHERE name != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	taken := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		taken[name] = true
	}
	return taken, rows.Err()
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
