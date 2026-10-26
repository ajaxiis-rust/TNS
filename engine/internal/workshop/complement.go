package workshop

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"tns65/internal/db"
	"tns65/internal/version"
)

// DefaultArcsPath — путь к реестру драматических арок от корня engine/.
const DefaultArcsPath = "internal/birth/data/arcs.json"

// Arc — строка реестра arcs.json: требуемая арка и допустимые архетипы Юнга
// (B-12, T-05-02).
type Arc struct {
	Arc        string   `json:"arc"`
	Archetypes []string `json:"archetypes"`
}

// LoadArcs читает реестр драматических арок (12 арок с допустимыми архетипами).
func LoadArcs(path string) ([]Arc, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("workshop: читать arcs.json %s: %w", path, err)
	}
	var arcs []Arc
	if err := json.Unmarshal(raw, &arcs); err != nil {
		return nil, fmt.Errorf("workshop: arcs.json %s: %w", path, err)
	}
	return arcs, nil
}

// ComplementPayload — payload заказа complement (T-05-02, шаг 1): требуемая
// арка из arcs.json и сцена-источник спроса.
type ComplementPayload struct {
	Arc     string `json:"arc"`
	SceneID int64  `json:"scene_id"`
}

// EnsureComplementOrder ставит заказ complement (шаг 2), когда под требуемую
// арку нет подходящей карточки: подходящая — карточка characters, чей архетип
// входит в archetypes арки. Заказ ставится только на тяжёлой ступени паузы;
// повторная постановка того же спроса не создаёт дубль. Возвращает true, если
// заказ стоит в очереди после вызова.
func EnsureComplementOrder(h *db.Handle, arc Arc, sceneID int64, pause PauseStep) (bool, error) {
	if pause != PauseHeavy {
		return false, nil
	}
	ok, err := hasCardForArc(h, arc)
	if err != nil {
		return false, err
	}
	if ok {
		return false, nil
	}

	payload := ComplementPayload{Arc: arc.Arc, SceneID: sceneID}
	raw, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM workshop_queue WHERE kind = ? AND status = 'queued' AND payload_json = ?`,
		string(KindComplement), string(raw)).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	if err := Enqueue(h, KindComplement, StepHeavy, DefaultProvider, payload); err != nil {
		return false, err
	}
	return true, nil
}

// ReconcileComplement снимает заказы complement, чей спрос исчез (шаг 3):
// сцена-источник закрыта или арка покрыта подходящей карточкой. Снятый заказ
// переходит в status=cancelled и недокованная карточка в пачку не появляется.
func ReconcileComplement(h *db.Handle, arcs []Arc) error {
	rows, err := h.Query(`SELECT id, payload_json FROM workshop_queue WHERE kind = ? AND status = 'queued'`,
		string(KindComplement))
	if err != nil {
		return err
	}
	type pending struct {
		id      int64
		payload string
	}
	var pend []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.payload); err != nil {
			_ = rows.Close()
			return err
		}
		pend = append(pend, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, p := range pend {
		var payload ComplementPayload
		if err := json.Unmarshal([]byte(p.payload), &payload); err != nil {
			return err
		}
		active, err := sceneActive(h, payload.SceneID)
		if err != nil {
			return err
		}
		covered, err := arcCovered(h, arcs, payload.Arc)
		if err != nil {
			return err
		}
		if !active || covered {
			if _, err := h.Exec("workshop_queue",
				`UPDATE workshop_queue SET status = ? WHERE id = ?`,
				string(StatusCancelled), p.id); err != nil {
				return err
			}
		}
	}
	return nil
}

// sceneActive сообщает, активна ли сцена-источник спроса: активна только open;
// закрытая или отсутствующая сцена спроса не держит.
func sceneActive(h *db.Handle, sceneID int64) (bool, error) {
	var status string
	err := h.QueryRow(`SELECT status FROM scenes WHERE id = ?`, sceneID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status == "open", nil
}

// arcCovered сообщает, покрыта ли арка подходящей карточкой characters.
func arcCovered(h *db.Handle, arcs []Arc, arcName string) (bool, error) {
	for _, a := range arcs {
		if a.Arc == arcName {
			return hasCardForArc(h, a)
		}
	}
	return false, nil
}

// hasCardForArc сообщает, есть ли в characters карточка, чей архетип входит в
// archetypes арки.
func hasCardForArc(h *db.Handle, arc Arc) (bool, error) {
	rows, err := h.Query(`SELECT card_json FROM characters`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return false, err
		}
		var c struct {
			Archetype string `json:"archetype"`
		}
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return false, err
		}
		for _, a := range arc.Archetypes {
			if c.Archetype == a {
				return true, nil
			}
		}
	}
	return false, rows.Err()
}

// ForgeCardRole — роль слота задачи ковки карточки (tns65 swarm forge-card).
const ForgeCardRole = "forge-card"

// ForgeCardKind — kind записей artifact и forge_log о ковке карточки.
const ForgeCardKind = "forge-card"

// ForgeCardPrompt строит промпт задачи ковки карточки: требуемая арка и
// сцена-источник спроса. Оценка Пифии добавляется задачей T-05-03.
func ForgeCardPrompt(p ComplementPayload) string {
	raw, _ := json.Marshal(map[string]any{
		"task":     ForgeCardRole,
		"arc":      p.Arc,
		"scene_id": p.SceneID,
		"pythia":   "",
	})
	return string(raw)
}

// RunForgeCard — однозадачная задача ковки карточки (шаг 4): модель по арке и
// оценке Пифии варит карточку; результат — артефакт в литературной базе и строка
// forge_log (писатель forge). Карточка валидируется теми же правилами, что и
// пачка P1 (три слоя, T-02-06); карточка без слоя отвергается.
func RunForgeCard(literary *db.Handle, payload ComplementPayload) (string, error) {
	slot := slotForProvider(DefaultProvider)
	if slot == nil {
		return "", errors.New("workshop: слот ковки карточки не подключён")
	}
	out, err := slot.Generate(context.Background(), ForgeCardRole, ForgeCardPrompt(payload))
	if err != nil {
		return "", err
	}
	if err := validateCard(out); err != nil {
		return "", err
	}

	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(out)))
	now := nowUTC()
	err = literary.Tx(db.WriterForge, func(tx *db.Tx) error {
		if _, err := tx.Exec("artifact",
			`INSERT INTO artifact (kind, path, checksum) VALUES (?, ?, ?)`,
			ForgeCardKind, fmt.Sprintf("cards/complement-%d.json", payload.SceneID), checksum); err != nil {
			return err
		}
		_, err := tx.Exec("forge_log",
			`INSERT INTO forge_log (kind, task, status, started_at, finished_at, log_path, package_version) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ForgeCardKind, ForgeCardRole, "done", now, now, "", version.Version)
		return err
	})
	if err != nil {
		return "", err
	}
	return out, nil
}

// WriteComplementCard — ядро пишет готовую карточку в characters оперативной
// базы (шаг 5). Имя NPC, как и в пачке, остаётся пустым (B-05.3). В живом ходе
// карточки не куются — постановку исполнения закрывает исполнитель очереди.
func WriteComplementCard(world *db.Handle, cardJSON string) error {
	var c forgeCard
	if err := json.Unmarshal([]byte(cardJSON), &c); err != nil {
		return fmt.Errorf("workshop: карточка не JSON: %w", err)
	}
	if c.Psychotype == "" || c.Estate == "" {
		return errors.New("workshop: карточка без психотипа или сословия")
	}
	_, err := world.Exec("characters",
		`INSERT INTO characters (name, psychotype, estate, card_json) VALUES (?, ?, ?, ?)`,
		"", c.Psychotype, c.Estate, cardJSON)
	return err
}

// forgeCard — карточка, сваренная задачей ковки; слои — контракт пачки P1 (B-07).
type forgeCard struct {
	Psychotype string     `json:"psychotype"`
	Estate     string     `json:"estate"`
	Archetype  string     `json:"archetype"`
	Voice      string     `json:"voice"`
	Appearance string     `json:"appearance"`
	Weight     float64    `json:"weight"`
	Backstory  string     `json:"backstory,omitempty"`
	Layers     cardLayers `json:"layers"`
}

type cardLayers struct {
	State   *cardStateLayer   `json:"state"`
	Economy *cardEconomyLayer `json:"economy"`
	Hidden  *cardHiddenLayer  `json:"hidden"`
}

type cardStateLayer struct {
	Health int            `json:"health"`
	Skills map[string]int `json:"skills"`
	Memory cardMemory     `json:"memory"`
}

type cardMemory struct {
	HalfLifeDays        float64 `json:"halfLifeDays"`
	SimilarityThreshold float64 `json:"similarityThreshold"`
	MaxMemoryEntries    int     `json:"maxMemoryEntries"`
}

type cardEconomyLayer struct {
	Stats       map[string]int     `json:"stats"`
	Vices       map[string]int     `json:"vices"`
	Temperament map[string]float64 `json:"temperament"`
}

type cardHiddenLayer struct {
	Psychotype string   `json:"psychotype"`
	Motives    []string `json:"motives"`
}

// validateCard проверяет карточку теми же правилами, что и пачка P1 (B-07,
// T-02-06): три слоя на месте, состояние 0–100 и девять навыков, шесть статов,
// восемь пороков, четыре гумора, память заполнена, скрытый слой с психотипом и
// явными пустыми мотивами. Карточка без слоя отвергается.
func validateCard(raw string) error {
	var c forgeCard
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return fmt.Errorf("workshop: карточка не JSON: %w", err)
	}
	if c.Archetype == "" {
		return errors.New("workshop: карточка без архетипа")
	}
	l := c.Layers
	if l.State == nil || l.Economy == nil || l.Hidden == nil {
		return errors.New("workshop: карточка без слоёв")
	}
	if l.State.Health < 0 || l.State.Health > 100 {
		return fmt.Errorf("workshop: здоровье вне 0–100: %d", l.State.Health)
	}
	if got := len(l.State.Skills); got != 9 {
		return fmt.Errorf("workshop: навыков %d, want 9", got)
	}
	if l.State.Memory.HalfLifeDays <= 0 || l.State.Memory.SimilarityThreshold <= 0 || l.State.Memory.MaxMemoryEntries <= 0 {
		return errors.New("workshop: параметры памяти не заполнены")
	}
	if got := len(l.Economy.Stats); got != 6 {
		return fmt.Errorf("workshop: статов %d, want 6", got)
	}
	if got := len(l.Economy.Vices); got != 8 {
		return fmt.Errorf("workshop: пороков %d, want 8", got)
	}
	if got := len(l.Economy.Temperament); got != 4 {
		return fmt.Errorf("workshop: гуморов %d, want 4", got)
	}
	if l.Hidden.Psychotype == "" {
		return errors.New("workshop: скрытый слой без психотипа")
	}
	if l.Hidden.Motives == nil {
		return errors.New("workshop: скрытый слой без мотивов")
	}
	return nil
}
