package workshop

import (
	"bufio"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"tns65/internal/db"
)

// HoroscopeMatchConfig — порог сверки ранних векторов набора игрока с векторами
// веток гороскопа (W-13.7, config.yaml: секция horoscope).
type HoroscopeMatchConfig struct {
	MatchThreshold float64 // косинус ≥ порога — готовая ветка.
}

// DefaultHoroscopeMatchConfig возвращает порог по умолчанию, совпадающий со
// значением config.yaml.
func DefaultHoroscopeMatchConfig() HoroscopeMatchConfig {
	return HoroscopeMatchConfig{MatchThreshold: 0.75}
}

// LoadHoroscopeMatchConfig читает секцию horoscope из файла конфигурации.
func LoadHoroscopeMatchConfig(path string) (HoroscopeMatchConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return HoroscopeMatchConfig{}, err
	}
	return parseHoroscopeSection(string(data))
}

// parseHoroscopeSection разбирает топ-ключ `horoscope:` со скалярным полем
// match_threshold; отсутствующая секция сохраняет значение по умолчанию.
func parseHoroscopeSection(data string) (HoroscopeMatchConfig, error) {
	cfg := DefaultHoroscopeMatchConfig()

	inHoroscope := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inHoroscope = trimmed == "horoscope:"
			continue
		}
		if !inHoroscope {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}
		if strings.TrimSpace(key) == "match_threshold" {
			cfg.MatchThreshold = f
		}
	}
	if err := sc.Err(); err != nil {
		return HoroscopeMatchConfig{}, err
	}
	return cfg, nil
}

// HoroscopeMatch — готовая ветка гороскопа, к которой игрок пришёл сам (W-13.7).
// Ветка живёт только в памяти: матчер в базы не пишет (W-13.5).
type HoroscopeMatch struct {
	SceneID int64
	IssueID int64
	Branch  HoroscopeBranch
	Cosine  float64
}

// MatchHoroscope сверяет ранние векторы набора игрока с векторами веток
// гороскопа сцены (W-13.7): совпадение (косинус ≥ порога) возвращает готовую
// ветку мгновенно, без генерации на сцене; промах возвращает nil (W-13.6 — промах
// не форсируется). Матчер только читает обе базы: ни одна таблица не пишется,
// слот генерации не вызывается (W-13.5 — неподтверждённая ветка ничего не пишет).
func MatchHoroscope(world, literary *db.Handle, sceneID int64, early [][]float32, cfg HoroscopeMatchConfig) (*HoroscopeMatch, error) {
	if world == nil || literary == nil {
		return nil, errors.New("workshop: матчер гороскопа без хэндла базы")
	}
	if cfg.MatchThreshold <= 0 {
		cfg.MatchThreshold = DefaultHoroscopeMatchConfig().MatchThreshold
	}
	if len(early) == 0 {
		return nil, nil
	}

	// Гороскоп выпускается под сцену и живёт до перековки: активен последний выпуск.
	var issueID int64
	var raw string
	err := literary.QueryRow(
		`SELECT id, issue_json FROM horoscope_issue WHERE scene_id = ? ORDER BY id DESC LIMIT 1`,
		sceneID).Scan(&issueID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var ho HoroscopeOutput
	if err := json.Unmarshal([]byte(raw), &ho); err != nil {
		return nil, fmt.Errorf("workshop: issue_json гороскопа не JSON: %w", err)
	}

	// Векторы веток: source_text хранит probe_text ветки (T-05-04).
	vecByProbe := map[string][]float32{}
	rows, err := world.Query(
		`SELECT source_text, vec FROM vectors WHERE owner_table = ? AND owner_id = ?`,
		horoscopeOwnerTable, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var probe string
		var blob []byte
		if err := rows.Scan(&probe, &blob); err != nil {
			return nil, err
		}
		if v := horoscopeVecFromBlob(blob); v != nil {
			vecByProbe[probe] = v
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var best *HoroscopeMatch
	bestCosine := -1.0
	for _, b := range ho.Branches {
		bv, ok := vecByProbe[b.ProbeText]
		if !ok {
			continue
		}
		for _, ev := range early {
			c := cosine(ev, bv)
			if c > bestCosine {
				bestCosine = c
				best = &HoroscopeMatch{SceneID: sceneID, IssueID: issueID, Branch: b, Cosine: c}
			}
		}
	}
	if best == nil || bestCosine < cfg.MatchThreshold {
		return nil, nil
	}
	return best, nil
}

// horoscopeVecFromBlob декодирует BLOB вектора ветки в Float32 little-endian
// (D-17.2); битый BLOB (не кратен 4 байтам) даёт nil.
func horoscopeVecFromBlob(blob []byte) []float32 {
	if len(blob)%4 != 0 {
		return nil
	}
	vec := make([]float32, len(blob)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return vec
}

// cosine — косинусная близость двух векторов; расхождение размерности или
// нулевая норма защищены — дают 0.
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		na += ai * ai
		nb += bi * bi
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
