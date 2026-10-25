package turn

import "strings"

// Токены-заменители биндера слотов (T-03-03, шаг 2). Закрываются биндером по
// труппе/пачке и склонению лица; литералы имён событий в журнале запрещены,
// токены — контракт шаблона, живут здесь одной строкой.
const (
	TokenNPCName        = "[NPCname]"
	TokenNPCNameActive  = "[NPCnameActive]"
	TokenNPCNamePassive = "[NPCnamePassive]"
	TokenNPCRole        = "[NPCrole]"
	TokenNPCRoleDeity   = "[NPCroleDeity]"
	TokenNPCGroup       = "[NPCgroup]"
	TokenPlace          = "[PLACE]"
)

// NPC — лицо труппы/пачки для биндера (Труппа T-03-11 наполняет её из карточек).
// Deity помечает влиятельную сущность мира, стихию, закон или судьбу — не смертного.
type NPC struct {
	ID          int64
	Name        string
	NameActive  string // форма активного падежа (склонение из карточки NPC).
	NamePassive string // форма пассивного падежа.
	Role        string
	Group       string
	Deity       bool
	Tags        []string
	Backstory   string // предыстория; до второго обращения может отсутствовать (LT-16.1).
}

// Place — топоним для биндера.
type Place struct {
	ID   int64
	Name string
}

// Roster — труппа и места, доступные биндеру на сцену.
type Roster struct {
	NPCs   []NPC
	Places []Place
}

// Binding — привязка токена к лицу или месту. До подтверждения живёт в памяти
// хода; на коммите пишется строкой token_bindings (scene_id, chapter_id, token,
// character_id). Номер привязки ID отличает одинаковый токен в разных арках.
type Binding struct {
	ID          int64
	SceneID     int64
	ChapterID   int64
	Token       string
	CharacterID int64 // 0 для места.
	PlaceID     int64 // 0 для лица.
	Value       string
}

// Mention — сущность, названная в реплике: лицо труппы или топоним.
type Mention struct {
	NPC   *NPC
	Place *Place
}

// SlotBinder — биндер слотов: закрывает токены шаблона по труппе/пачке и
// склонению лица, привязки живут сцену или главу (T-03-03, шаги 2–3).
type SlotBinder struct {
	roster Roster
	seq    int64
}

// NewSlotBinder возвращает биндер над труппой и местами roster.
func NewSlotBinder(r Roster) *SlotBinder {
	return &SlotBinder{roster: r}
}

// Roster возвращает труппу и места биндера.
func (b *SlotBinder) Roster() Roster { return b.roster }

// DetectMentions находит в реплике названные лица труппы и топонимы по имени.
func (b *SlotBinder) DetectMentions(text string) []Mention {
	lower := strings.ToLower(text)
	var out []Mention
	for i := range b.roster.NPCs {
		n := &b.roster.NPCs[i]
		if n.Name != "" && strings.Contains(lower, strings.ToLower(n.Name)) {
			out = append(out, Mention{NPC: n})
		}
	}
	for i := range b.roster.Places {
		p := &b.roster.Places[i]
		if p.Name != "" && strings.Contains(lower, strings.ToLower(p.Name)) {
			out = append(out, Mention{Place: p})
		}
	}
	return out
}

// Bind закрывает токены шаблона по названным в реплике сущностям и возвращает
// текст без заменителей вместе с привязками. Каждая привязка получает новый
// номер: одинаковый токен в разных арках даёт разные привязки.
func (b *SlotBinder) Bind(template string, mentions []Mention, sceneID, chapterID int64) (string, []Binding) {
	npc := firstMentionedNPC(mentions)
	place := firstMentionedPlace(mentions)
	deity := b.deity()

	res := template
	var bindings []Binding

	add := func(token, value string, charID, placeID int64) {
		res = strings.ReplaceAll(res, token, value)
		b.seq++
		bindings = append(bindings, Binding{
			ID:          b.seq,
			SceneID:     sceneID,
			ChapterID:   chapterID,
			Token:       token,
			CharacterID: charID,
			PlaceID:     placeID,
			Value:       value,
		})
	}

	if npc != nil {
		add(TokenNPCName, npc.Name, npc.ID, 0)
		add(TokenNPCNameActive, nonEmpty(npc.NameActive, npc.Name), npc.ID, 0)
		add(TokenNPCNamePassive, nonEmpty(npc.NamePassive, npc.Name), npc.ID, 0)
		add(TokenNPCRole, nonEmpty(npc.Role, npc.Name), npc.ID, 0)
		add(TokenNPCGroup, nonEmpty(npc.Group, npc.Name), npc.ID, 0)
	}
	// Роль исхода силы привязывается только к влиятельной сущности — не к смертному.
	if deity != nil {
		add(TokenNPCRoleDeity, nonEmpty(deity.Role, deity.Name), deity.ID, 0)
	}
	if place != nil {
		add(TokenPlace, place.Name, 0, place.ID)
	}
	return res, bindings
}

// deity — первая влиятельная сущность труппы (стихия/закон/судьба), не смертный.
func (b *SlotBinder) deity() *NPC {
	for i := range b.roster.NPCs {
		if b.roster.NPCs[i].Deity {
			return &b.roster.NPCs[i]
		}
	}
	return nil
}

func firstMentionedNPC(mentions []Mention) *NPC {
	for i := range mentions {
		if mentions[i].NPC != nil {
			return mentions[i].NPC
		}
	}
	return nil
}

func firstMentionedPlace(mentions []Mention) *Place {
	for i := range mentions {
		if mentions[i].Place != nil {
			return mentions[i].Place
		}
	}
	return nil
}

func nonEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
