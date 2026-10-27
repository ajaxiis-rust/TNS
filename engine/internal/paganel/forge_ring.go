package paganel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"

	"tns65/internal/censor"
	"tns65/internal/db"
	"tns65/internal/geo"
)

// ForgeRole — роль слота ковки кольца Паганеля (tns65 swarm forge-ring).
const ForgeRole = "forge_ring"

// forgeLang — язык заводского Цензора ковки кольца.
const forgeLang = "ru"

// RingRadiusDays — радиус кольца для блужданий (W-05.6): около недели пути.
// Промоушен отклоняет место, дальше которого от героя больше этого радиуса.
const RingRadiusDays = 7.0

// Статусы записей ковки (W-05.9/W-05.10): черновик, испечённое, в перековку.
const (
	placeStatusDraft   = "draft"
	placeStatusBaked   = "baked"
	placeStatusReforge = "reforge"
)

// placeOriginPaganel — происхождение записей Паганеля (W-05.9).
const placeOriginPaganel = "paganel"

// forgedPlaceKind — род места для черновиков Паганеля: в схеме выхода модели
// рода нет, места Паганеля пишутся родом «место».
const forgedPlaceKind = "место"

// Slot — слот генерации ковки: по роли и промпту возвращает текст ответа.
// В тестах подменяется testkit.FakeSlot через SetSlot; в производстве до
// подключения генератора слот отсутствует.
type Slot interface {
	Generate(ctx context.Context, role, prompt string) (string, error)
}

// Censor — словарный фильтр ковки (T-04-11); в проде censor.Check, в тестах
// подменяется через SetCensor.
type Censor interface {
	Check(text string) (bool, []string)
}

// censorAdapter — заводской Цензор ковки: единый фильтр пакета по словарю ru.
type censorAdapter struct{}

func (censorAdapter) Check(text string) (bool, []string) {
	return censor.Check(text, forgeLang)
}

var (
	slotMu   sync.RWMutex
	curSlot  Slot
	censorMu sync.RWMutex
	curCen   Censor
)

// SetSlot подменяет слот ковки кольца (тестовая точка входа); nil — заводской.
func SetSlot(s Slot) {
	slotMu.Lock()
	defer slotMu.Unlock()
	curSlot = s
}

// slotForge возвращает слот ковки кольца; nil — слот не подключён.
func slotForge() Slot {
	slotMu.RLock()
	defer slotMu.RUnlock()
	return curSlot
}

// SetCensor подменяет Цензора ковки кольца (тестовая точка входа); nil — заводской.
func SetCensor(c Censor) {
	censorMu.Lock()
	defer censorMu.Unlock()
	curCen = c
}

// censorForge возвращает Цензора ковки; nil заменяется заводским.
func censorForge() Censor {
	censorMu.RLock()
	defer censorMu.RUnlock()
	if curCen != nil {
		return curCen
	}
	return censorAdapter{}
}

// ForgedPlace — место в строгом JSON ковки кольца (T-06-08, шаг 1): имя, строка
// сути, хозяин, известность, направление словами и дистанция числом дней.
// Координаты назначает код от положения героя (принцип «код считает», W-20).
type ForgedPlace struct {
	Name         string  `json:"name"`
	Essence      string  `json:"essence"`
	Owner        string  `json:"owner"`
	Prominence   string  `json:"prominence"`
	Bearing      string  `json:"bearing"`
	DistanceDays float64 `json:"distance_days"`
}

// ForgedPath — путь в строгом JSON ковки кольца: концы именами мест, тип —
// тракт/река/море. Длину и трение считает код (W-20).
type ForgedPath struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// ForgeOutput — строгий JSON ковки кольца (T-06-08, шаг 1).
type ForgeOutput struct {
	Places []ForgedPlace `json:"places"`
	Paths  []ForgedPath  `json:"paths"`
}

// PromotionResult — итог промоушена draft → baked: сколько записей испечено,
// сколько отправлено в перековку (reforge).
type PromotionResult struct {
	Promoted int64 `json:"promoted"`
	Reforged int64 `json:"reforged"`
}

// ForgePrompt собирает промпт ковки кольца: модель извлекает места и пути из
// голоса книги, называет направление словами и дистанцию числом дней; координаты
// модель не считает (W-20.2).
func ForgePrompt(hero HeroPosition, h *db.Handle) string {
	placeName := ""
	if hero.PlaceID != 0 && h != nil {
		_ = h.QueryRow(`SELECT name FROM places WHERE id = ?`, hero.PlaceID).Scan(&placeName)
	}
	var b strings.Builder
	b.WriteString("Ты — Паганель, ковка карты мира. Извлеки из голоса книги места и пути, включая неявные («к вечеру добрались до…»). ")
	if placeName != "" {
		b.WriteString("Герой сейчас у места «")
		b.WriteString(placeName)
		b.WriteString("». ")
	}
	b.WriteString("Назови направление словами (север, северо-восток, восток, юго-восток, юг, юго-запад, запад, северо-запад) и дистанцию числом дней пути от героя; координаты не считай. ")
	b.WriteString("Ответь строгим JSON без пояснений: ")
	b.WriteString(`{"places":[{"name":"…","essence":"…","owner":"…","prominence":"…","bearing":"северо-запад","distance_days":2.5}],"paths":[{"from":"…","to":"…","type":"trakt|river|sea"}]}`)
	return b.String()
}

// ForgeRing — ковка кольца (T-06-08, шаги 1–2): модель (слот) возвращает
// извлечённые места и пути строгим JSON, код назначает координаты от положения
// героя и пишет draft-записи в places/paths хэндлом forge (origin='paganel',
// status='draft'). Черновик сразу виден рантайму как состояние мира (W-05.9);
// литературную базу ковка не трогает. Мусорный ответ модели — отказ до записи.
func ForgeRing(forge *db.Handle) (ForgeOutput, error) {
	if forge == nil {
		return ForgeOutput{}, errors.New("paganel: хэндл ковки не задан")
	}
	slot := slotForge()
	if slot == nil {
		return ForgeOutput{}, errors.New("paganel: слот ковки кольца не подключён")
	}
	hero, err := ReadHeroPosition(forge)
	if err != nil {
		return ForgeOutput{}, err
	}
	norm, err := ReadDayNorm(forge)
	if err != nil {
		return ForgeOutput{}, err
	}

	raw, err := slot.Generate(context.Background(), ForgeRole, ForgePrompt(hero, forge))
	if err != nil {
		return ForgeOutput{}, err
	}
	var fo ForgeOutput
	if err := json.Unmarshal([]byte(raw), &fo); err != nil {
		return ForgeOutput{}, fmt.Errorf("paganel: выход ковки кольца не JSON: %w", err)
	}
	if err := validateForgeOutput(fo); err != nil {
		return ForgeOutput{}, err
	}
	if err := writeDrafts(forge, fo, hero, norm); err != nil {
		return ForgeOutput{}, err
	}
	return fo, nil
}

// validateForgeOutput проверяет строгий JSON ковки: у места имя, допустимое
// направление и неотрицательная дистанция; у пути концы и допустимый тип.
// Мусорный ответ — отказ (00-INDEX, «отказы модельных шагов»).
func validateForgeOutput(fo ForgeOutput) error {
	for _, p := range fo.Places {
		if strings.TrimSpace(p.Name) == "" {
			return errors.New("paganel: место без имени в выходе ковки")
		}
		if _, err := parseBearing(p.Bearing); err != nil {
			return err
		}
		if p.DistanceDays < 0 {
			return fmt.Errorf("paganel: место %q: distance_days = %v вне допустимого", p.Name, p.DistanceDays)
		}
	}
	for _, path := range fo.Paths {
		if strings.TrimSpace(path.From) == "" || strings.TrimSpace(path.To) == "" {
			return errors.New("paganel: путь без концов в выходе ковки")
		}
		if err := ValidPathType(path.Type); err != nil {
			return err
		}
	}
	return nil
}

// placeRef — место с координатами для разрешения концов пути.
type placeRef struct {
	id int64
	x  float64
	y  float64
}

// writeDrafts пишет draft-места и draft-пути хэндлом forge (W-05.9). Место с
// уже существующим именем не создаётся повторно — идемпотентность повторной
// ковки (T-06-08, «Проверка»); путь без известных концов не пишется.
func writeDrafts(forge *db.Handle, fo ForgeOutput, hero HeroPosition, norm float64) error {
	refs := make(map[string]placeRef, len(fo.Places))
	for _, p := range fo.Places {
		if id, ok, err := placeIDByName(forge, p.Name); err != nil {
			return err
		} else if ok {
			x, y, err := readPlaceCoords(forge, id)
			if err != nil {
				return err
			}
			refs[p.Name] = placeRef{id: id, x: x, y: y}
			continue
		}

		bearingDeg, _ := parseBearing(p.Bearing) // уже проверена validateForgeOutput
		x, y := placeCoords(hero.X, hero.Y, bearingDeg, p.DistanceDays, norm)
		res, err := forge.Exec("places",
			`INSERT INTO places (name, kind, x, y, origin, status) VALUES (?, ?, ?, ?, ?, ?)`,
			p.Name, forgedPlaceKind, x, y, placeOriginPaganel, placeStatusDraft)
		if err != nil {
			return fmt.Errorf("paganel: записать место %q: %w", p.Name, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("paganel: id места %q: %w", p.Name, err)
		}
		refs[p.Name] = placeRef{id: id, x: x, y: y}
	}

	for _, path := range fo.Paths {
		from, okFrom := refs[path.From]
		to, okTo := refs[path.To]
		if !okFrom || !okTo {
			continue
		}
		length := geo.Distance(geo.Place{X: from.x, Y: from.y}, geo.Place{X: to.x, Y: to.y})
		if _, err := forge.Exec("paths",
			`INSERT INTO paths (from_place_id, to_place_id, type, length_km, friction, origin, status)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			from.id, to.id, path.Type, length, TypeFriction(path.Type), placeOriginPaganel, placeStatusDraft); err != nil {
			return fmt.Errorf("paganel: записать путь %q→%q: %w", path.From, path.To, err)
		}
	}
	return nil
}

// PromoteDrafts — промоушен draft → baked (T-06-08, шаг 3, W-05.10): кодом при
// следующей ковке. Место проходит словарную цензуру (T-04-11), проверку
// уникальности имени и проверку координат в пределах кольца; отклонённое
// получает status='reforge' и уходит в перековку. Пути (тип проверен при записи)
// продвигаются сразу. Хэндл — forge: пишет только places/paths, литературную
// базу не трогает.
func PromoteDrafts(forge *db.Handle) (PromotionResult, error) {
	if forge == nil {
		return PromotionResult{}, errors.New("paganel: хэндл ковки не задан")
	}
	hero, err := ReadHeroPosition(forge)
	if err != nil {
		return PromotionResult{}, err
	}
	norm, err := ReadDayNorm(forge)
	if err != nil {
		return PromotionResult{}, err
	}
	cen := censorForge()

	var result PromotionResult

	rows, err := forge.Query(`SELECT id, name, x, y FROM places WHERE status = ?`, placeStatusDraft)
	if err != nil {
		return result, fmt.Errorf("paganel: черновики мест: %w", err)
	}
	type draftPlace struct {
		id   int64
		name string
		x    float64
		y    float64
	}
	var drafts []draftPlace
	for rows.Next() {
		var d draftPlace
		var x, y sql.NullFloat64
		if err := rows.Scan(&d.id, &d.name, &x, &y); err != nil {
			rows.Close()
			return result, fmt.Errorf("paganel: строка черновика места: %w", err)
		}
		d.x, d.y = x.Float64, y.Float64
		drafts = append(drafts, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("paganel: черновики мест: %w", err)
	}

	for _, d := range drafts {
		status := placeStatusBaked
		switch {
		case !censorPass(cen, d.name):
			status = placeStatusReforge
		case nameDuplicate(forge, d.id, d.name):
			status = placeStatusReforge
		case daysToHero(hero, d.x, d.y, norm) > RingRadiusDays:
			status = placeStatusReforge
		}
		if _, err := forge.Exec("places",
			`UPDATE places SET status = ? WHERE id = ?`, status, d.id); err != nil {
			return result, fmt.Errorf("paganel: продвинуть место %d: %w", d.id, err)
		}
		if status == placeStatusReforge {
			result.Reforged++
		} else {
			result.Promoted++
		}
	}

	res, err := forge.Exec("paths",
		`UPDATE paths SET status = ? WHERE status = ?`, placeStatusBaked, placeStatusDraft)
	if err != nil {
		return result, fmt.Errorf("paganel: продвинуть пути: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		result.Promoted += n
	}
	return result, nil
}

// censorPass — словарная цензура текста (T-04-11): fail-closed, запретная лемма
// отклоняет запись.
func censorPass(cen Censor, text string) bool {
	if cen == nil {
		return true
	}
	ok, _ := cen.Check(text)
	return ok
}

// nameDuplicate сообщает, есть ли другое место с тем же именем (уникальность
// places.name, идемпотентность повторной ковки). Индекса UNIQUE в схеме нет,
// поэтому проверка выполняется кодом.
func nameDuplicate(h *db.Handle, id int64, name string) bool {
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM places WHERE name = ? AND id != ?`, name, id).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// placeIDByName возвращает id места по имени; false — места нет.
func placeIDByName(h *db.Handle, name string) (int64, bool, error) {
	var id int64
	err := h.QueryRow(`SELECT id FROM places WHERE name = ? LIMIT 1`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("paganel: читать место %q: %w", name, err)
	}
	return id, true, nil
}

// daysToHero — дни пути от героя до точки по геодезии и норме дня (оценка без
// известного пути, W-06.3).
func daysToHero(hero HeroPosition, x, y, norm float64) float64 {
	return geo.Distance(geo.Place{X: hero.X, Y: hero.Y}, geo.Place{X: x, Y: y}) / norm
}

// placeCoords назначает координаты места от положения героя по направлению и
// дистанции в днях (T-06-08, шаг 1, W-20): дистанция переводится в километры
// нормой дня, направление — азимут в градусах; долгота масштабируется косинусом
// широты. Дни в базу не пишутся.
func placeCoords(heroX, heroY, bearingDeg, distanceDays, norm float64) (float64, float64) {
	km := distanceDays * norm
	rad := bearingDeg * math.Pi / 180
	latRad := heroY * math.Pi / 180

	const kmPerDegLat = 110.574
	kmPerDegLon := 111.320 * math.Cos(latRad)

	dxDeg := km * math.Sin(rad) / kmPerDegLon
	dyDeg := km * math.Cos(rad) / kmPerDegLat
	return heroX + dxDeg, heroY + dyDeg
}

// bearingDeg — азимут русских слов направлений (0 — север, по часовой).
var bearingDeg = map[string]float64{
	"север":        0,
	"северовосток": 45,
	"восток":       90,
	"юговосток":    135,
	"юг":           180,
	"югозапад":     225,
	"запад":        270,
	"северозапад":  315,
}

// parseBearing разбирает русское направление модели (словами) в азимут градусов;
// вне списка восьми направлений — отказ.
func parseBearing(s string) (float64, error) {
	key := strings.ToLower(strings.TrimSpace(s))
	key = strings.ReplaceAll(key, "-", "")
	key = strings.ReplaceAll(key, " ", "")
	if deg, ok := bearingDeg[key]; ok {
		return deg, nil
	}
	return 0, fmt.Errorf("paganel: направление %q вне списка (север/восток/юг/запад и промежуточные)", s)
}
