package workshop

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"tns65/internal/db"
	"tns65/internal/prompts"
	"tns65/internal/version"
)

// PythiaRole — роль слота ночного прогона Пифии.
const PythiaRole = "pythia"

// pythiaSource — источник строк карты игрока, пополненных ночным прогоном.
const pythiaSource = "pythia"

// Фасеты карты игрока в player_atlas.theme: какая грань профиля пополнена.
const (
	pythiaThemeInterest = "interest"
	pythiaThemeHabit    = "habit"
	pythiaThemeAvoid    = "avoid"
	// pythiaThemeManipulation — фасет карты: пометка сработавшего детекта (W-13.7).
	pythiaThemeManipulation = "manipulation"
)

// Детект манипуляции (W-13.7): окно реплик делится на блоки, резкий сдвиг доли
// паттерна между соседними блоками выше порога alpha — манипуляция. Порог —
// рабочее значение TNS35 (alpha 0.25).
const (
	manipAlpha   = 0.25
	manipBlocks  = 4
	manipVerdict = "поведение главнее слов"
	manipLogKind = "manipulation"
)

// pythiaMaxTurns — сколько последних реплик игрока уходит в промпт Пифии.
const pythiaMaxTurns = 100

// PythiaOutput — строгий JSON ночного прогона Пифии (W-12, шаг 2). Поле
// psychotype принимается и отбрасывается: в карту идёт психотип героя из пачки
// (W-12.4 — модель психотип не пересчитывает).
type PythiaOutput struct {
	Mood       string   `json:"mood"`
	Interests  []string `json:"interests"`
	Habits     []string `json:"habits"`
	Avoid      []string `json:"avoid"`
	Psychotype string   `json:"psychotype"`
}

// RunPythia — ночной прогон Пифии (шаги 1–4): по стартовым данным героя
// (world_meta.hero_birth), психотипу из пачки и паттернам последних реплик
// модель пополняет карту игрока. Новые строки карты пишутся в player_atlas
// хэндлом core, сводка ночного прогона — в forge_log хэндлом forge. Повторный
// прогон того же дня не дублирует записи (идемпотентность по дню).
func RunPythia(world *db.Handle, literary *db.Handle, day int) error {
	var n int
	if err := world.QueryRow(`SELECT COUNT(*) FROM player_atlas WHERE day = ?`, day).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	birth, err := heroBirth(world)
	if err != nil {
		return err
	}
	psychotype, err := playerPsychotype(world)
	if err != nil {
		return err
	}
	turns, err := recentPlayerTurns(world)
	if err != nil {
		return err
	}
	manip := DetectManipulation(turns, DefaultManipulationConfig())

	if slotForProvider(DefaultProvider) == nil {
		return errors.New("workshop: слот Пифии не подключён")
	}
	out, err := GenerateWithRetry(context.Background(), literary, GenerateOptions{
		Role:     PythiaRole,
		Lang:     "ru",
		Provider: DefaultProvider,
	}, func(_ int, _ string) string { return pythiaPrompt(birth, psychotype, turns) })
	if err != nil {
		return err
	}

	var po PythiaOutput
	if err := json.Unmarshal([]byte(out), &po); err != nil {
		return fmt.Errorf("workshop: выход Пифии не JSON: %w", err)
	}
	if err := validatePythiaOutput(po); err != nil {
		return err
	}

	if err := writePlayerAtlas(world, day, po, psychotype, manip); err != nil {
		return err
	}
	if err := writePythiaLog(literary); err != nil {
		return err
	}
	if manip.Detected {
		return writeManipulationLog(literary)
	}
	return nil
}

// heroBirth читает стартовые данные карты игрока из world_meta.hero_birth
// (W-12.3); значение world_meta хранится JSON-строкой (T-02-03, шаг 3).
func heroBirth(h *db.Handle) (string, error) {
	var raw string
	err := h.QueryRow(`SELECT value FROM world_meta WHERE "key" = 'hero_birth'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("workshop: world_meta.hero_birth не найден")
	}
	if err != nil {
		return "", err
	}
	var birth string
	if err := json.Unmarshal([]byte(raw), &birth); err != nil {
		return "", fmt.Errorf("workshop: разбор hero_birth: %w", err)
	}
	return birth, nil
}

// playerPsychotype читает психотип героя из пачки: карточка героя — строка
// characters с id 0 (пачка NPC занимает 1..96, герой — 0). Психотип не
// пересчитывается (W-12.4): модель может вернуть свой психотип, но в базу он
// не пишется.
func playerPsychotype(h *db.Handle) (string, error) {
	var psychotype string
	err := h.QueryRow(`SELECT psychotype FROM characters WHERE id = 0`).Scan(&psychotype)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("workshop: карточка героя (characters id 0) не найдена")
	}
	if err != nil {
		return "", err
	}
	return psychotype, nil
}

// recentPlayerTurns собирает паттерны прошедших сессий: последние реплики
// игрока в порядке хода (W-12.4).
func recentPlayerTurns(h *db.Handle) ([]string, error) {
	rows, err := h.Query(`SELECT player_text FROM turns ORDER BY id DESC LIMIT ?`, pythiaMaxTurns)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var turns []string
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		turns = append(turns, text)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// ORDER BY id DESC собрал реплики от новой к старой; в промпте нужен порядок хода.
	for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
		turns[i], turns[j] = turns[j], turns[i]
	}
	return turns, nil
}

// defaultPythiaPrompt — дефолт промпта ночного прогона (без файла пакета):
// стартовые данные героя, психотип и паттерны последних реплик.
const defaultPythiaPrompt = `{"task":"pythia","hero_birth":"{hero_birth}","psychotype":"{psychotype}","turns":"{turns}"}`

// pythiaPrompt строит промпт ночного прогона: стартовые данные героя, психотип
// и паттерны последних реплик.
func pythiaPrompt(birth, psychotype string, turns []string) string {
	return prompts.Render(PythiaRole, "pythia", defaultPythiaPrompt, map[string]any{
		"hero_birth": birth,
		"psychotype": psychotype,
		"turns":      turns,
	})
}

// validatePythiaOutput проверяет строгий JSON Пифии: настроение и все грани
// карты на месте. psychotype не проверяется — поле принимается и отбрасывается
// (W-12.4).
func validatePythiaOutput(po PythiaOutput) error {
	if po.Mood == "" {
		return errors.New("workshop: выход Пифии без настроения")
	}
	if po.Interests == nil || po.Habits == nil || po.Avoid == nil {
		return errors.New("workshop: выход Пифии без граней карты")
	}
	return nil
}

// writePlayerAtlas пишет новые строки карты игрока в player_atlas хэндлом core
// (append-only: новая строка на прогон, старые не перезаписываются). Фасет грани
// идёт в theme, значение — в interest; привычка дублируется в habits. Психотип
// карты пишется при первом вычислении (W-12.4) и дальше не пересчитывается:
// строки прогона несут его, только пока в карте психотипа нет. Пометка детекта
// манипуляции (W-13.7) идёт первой строкой прогона и без психотипа: это метка
// поведения, а не грань психотипа.
func writePlayerAtlas(world *db.Handle, day int, po PythiaOutput, psychotype string, manip ManipulationReport) error {
	var known int
	if err := world.QueryRow(`SELECT COUNT(*) FROM player_atlas WHERE psychotype <> ''`).Scan(&known); err != nil {
		return err
	}
	if known > 0 {
		psychotype = ""
	}
	return world.Tx(db.WriterCore, func(tx *db.Tx) error {
		if manip.Detected {
			if _, err := tx.Exec("player_atlas",
				`INSERT INTO player_atlas (day, theme, mood, interest, source) VALUES (?, ?, ?, ?, ?)`,
				day, pythiaThemeManipulation, po.Mood, manipulationMark(manip), pythiaSource); err != nil {
				return err
			}
		}
		for _, v := range po.Interests {
			if _, err := tx.Exec("player_atlas",
				`INSERT INTO player_atlas (day, theme, mood, interest, source, psychotype) VALUES (?, ?, ?, ?, ?, ?)`,
				day, pythiaThemeInterest, po.Mood, v, pythiaSource, psychotype); err != nil {
				return err
			}
		}
		for _, v := range po.Habits {
			if _, err := tx.Exec("player_atlas",
				`INSERT INTO player_atlas (day, theme, mood, interest, source, psychotype, habits) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				day, pythiaThemeHabit, po.Mood, v, pythiaSource, psychotype, v); err != nil {
				return err
			}
		}
		for _, v := range po.Avoid {
			if _, err := tx.Exec("player_atlas",
				`INSERT INTO player_atlas (day, theme, mood, interest, source, psychotype) VALUES (?, ?, ?, ?, ?, ?)`,
				day, pythiaThemeAvoid, po.Mood, v, pythiaSource, psychotype); err != nil {
				return err
			}
		}
		return nil
	})
}

// writePythiaLog пишет сводку ночного прогона в forge_log хэндлом forge.
func writePythiaLog(literary *db.Handle) error {
	now := nowUTC()
	return literary.Tx(db.WriterForge, func(tx *db.Tx) error {
		_, err := tx.Exec("forge_log",
			`INSERT INTO forge_log (kind, task, status, started_at, finished_at, log_path, package_version) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			PythiaRole, PythiaRole, "done", now, now, "", version.Version)
		return err
	})
}

// writeManipulationLog пишет сработавший детект в журнал Пифии хэндлом forge:
// запись метит прогон, в котором поведение разошлось с самопрезентацией.
func writeManipulationLog(literary *db.Handle) error {
	now := nowUTC()
	return literary.Tx(db.WriterForge, func(tx *db.Tx) error {
		_, err := tx.Exec("forge_log",
			`INSERT INTO forge_log (kind, task, status, started_at, finished_at, log_path, package_version) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			manipLogKind, PythiaRole, "done", now, now, "", version.Version)
		return err
	})
}

// ManipulationConfig — пороги детекта манипуляции (W-13.7): Alpha — минимальная
// резкость сдвига доли паттерна между соседними блоками окна, Blocks — число
// блоков, на которые делится окно реплик.
type ManipulationConfig struct {
	Alpha  float64
	Blocks int
}

// DefaultManipulationConfig возвращает порог alpha по умолчанию (0.25) и четыре
// блока окна.
func DefaultManipulationConfig() ManipulationConfig {
	return ManipulationConfig{Alpha: manipAlpha, Blocks: manipBlocks}
}

// ManipulationReport — итог детекта: MaxShift — резкость сдвига по окну, Pattern
// — грань, давшая сдвиг, Claimed и Behavior — самопрезентация окна и поведение
// последнего блока, BehaviorWins — поведение главнее слов.
type ManipulationReport struct {
	Detected     bool
	MaxShift     float64
	Alpha        float64
	Pattern      string
	Claimed      string
	Behavior     string
	BehaviorWins bool
}

// manipulationPattern — грань поведения: имя для отчёта и основы слов реплик.
type manipulationPattern struct {
	name  string
	stems []string
}

// manipulationPatterns — грани поведения, по долям которых считается сдвиг.
var manipulationPatterns = []manipulationPattern{
	{name: "техника", stems: []string{"техник", "технократ", "механизм", "схем", "чертёж", "чертеж", "инженер", "алгоритм", "датчик", "двигател", "расчёт", "расчет", "систем"}},
	{name: "сенсорика", stems: []string{"стих", "поэт", "запах", "аромат", "шёлк", "шелк", "бархат", "нежн", "трепет", "шёпот", "шепот"}},
	{name: "быт", stems: []string{"хлеб", "ужин", "печь", "двор", "торг", "деньг", "хозяйств", "огород", "рынок"}},
}

// manipulationClaimMarkers — обороты самопрезентации: заявление о себе, а не
// действие (W-13.7: поведение главнее слов).
var manipulationClaimMarkers = []string{"я —", "я -", "я есть", "по натуре", "считаю себя", "себя считаю", "я человек", "во мне", "обо мне", "про меня"}

// DetectManipulation ищет манипуляцию в окне реплик игрока (W-13.7): окно
// делится на Blocks последовательных блоков, и резкий сдвиг доли паттерна между
// соседними блоками выше Alpha считается попыткой переиграть систему. Плавный
// дрейф внутри окна не детектируется: важна резкость шага, а не расхождение
// концов. Самопрезентация окна сверяется с поведением последнего блока —
// поведение главнее слов; психотип детект не трогает (W-12.4).
func DetectManipulation(turns []string, cfg ManipulationConfig) ManipulationReport {
	if cfg.Blocks <= 0 {
		cfg.Blocks = manipBlocks
	}
	if cfg.Alpha <= 0 {
		cfg.Alpha = manipAlpha
	}
	rep := ManipulationReport{Alpha: cfg.Alpha}
	if len(turns) < cfg.Blocks {
		return rep
	}

	shares := manipulationBlockShares(turns, cfg.Blocks)
	for b := 1; b < len(shares); b++ {
		for i := range manipulationPatterns {
			if d := math.Abs(shares[b][i] - shares[b-1][i]); d > rep.MaxShift {
				rep.MaxShift = d
				rep.Pattern = manipulationPatterns[i].name
			}
		}
	}
	rep.Detected = rep.MaxShift > rep.Alpha

	lastBlock := turns[(cfg.Blocks-1)*(len(turns)/cfg.Blocks):]
	rep.Behavior = manipulationDominant(lastBlock)
	rep.Claimed = manipulationClaimed(turns)
	rep.BehaviorWins = rep.Claimed != "" && rep.Claimed != rep.Behavior
	return rep
}

// manipulationBlockShares делит окно на Blocks равных блоков (остаток уходит
// последнему) и возвращает долю реплик каждой грани по блокам.
func manipulationBlockShares(turns []string, blocks int) [][]float64 {
	size := len(turns) / blocks
	shares := make([][]float64, blocks)
	for b := range shares {
		start := b * size
		end := start + size
		if b == blocks-1 {
			end = len(turns)
		}
		shares[b] = manipulationShares(turns[start:end])
	}
	return shares
}

// manipulationShares считает доли граней в блоке реплик: реплика входит в грань,
// если несёт хоть одну её основу.
func manipulationShares(block []string) []float64 {
	shares := make([]float64, len(manipulationPatterns))
	if len(block) == 0 {
		return shares
	}
	for _, text := range block {
		low := strings.ToLower(text)
		for i, p := range manipulationPatterns {
			if manipulationMatches(low, p) {
				shares[i]++
			}
		}
	}
	for i := range shares {
		shares[i] /= float64(len(block))
	}
	return shares
}

// manipulationMatches — реплика несёт грань, если содержит одну из её основ.
func manipulationMatches(low string, p manipulationPattern) bool {
	for _, stem := range p.stems {
		if strings.Contains(low, stem) {
			return true
		}
	}
	return false
}

// manipulationDominant — грань с наибольшей долей в блоке; при равенстве
// выигрывает первая по порядку реестра, пустой блок даёт пустую грань.
func manipulationDominant(block []string) string {
	shares := manipulationShares(block)
	best := -1
	for i, s := range shares {
		if s > 0 && (best < 0 || s > shares[best]) {
			best = i
		}
	}
	if best < 0 {
		return ""
	}
	return manipulationPatterns[best].name
}

// manipulationClaimed — грань самопрезентации окна: грани, о которых игрок
// заявляет оборотами «я — …», «по натуре …»; пусто — заявлений нет.
func manipulationClaimed(turns []string) string {
	counts := make([]int, len(manipulationPatterns))
	for _, text := range turns {
		low := strings.ToLower(text)
		if !manipulationIsClaim(low) {
			continue
		}
		for i, p := range manipulationPatterns {
			if manipulationMatches(low, p) {
				counts[i]++
			}
		}
	}
	best := -1
	for i, c := range counts {
		if c > 0 && (best < 0 || c > counts[best]) {
			best = i
		}
	}
	if best < 0 {
		return ""
	}
	return manipulationPatterns[best].name
}

// manipulationIsClaim — реплика заявляет о себе, а не описывает действие.
func manipulationIsClaim(low string) bool {
	for _, marker := range manipulationClaimMarkers {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

// manipulationMark — текст пометки карты игрока: интерпретация «поведение
// главнее слов» с метрикой сдвига (W-13.7).
func manipulationMark(rep ManipulationReport) string {
	mark := fmt.Sprintf("%s: сдвиг «%s» %.2f (порог %.2f), поведение — «%s»",
		manipVerdict, rep.Pattern, rep.MaxShift, rep.Alpha, rep.Behavior)
	if rep.Claimed != "" {
		mark += fmt.Sprintf(", заявлено — «%s»", rep.Claimed)
	}
	return mark
}
