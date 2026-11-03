package turn

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/structural"
)

// compositionBudget — бюджет выбора композиции (T-10-11, шаг 8): как у Челнока —
// одна выборка SQL и скоринг в памяти, без слота генерации.
const compositionBudget = 100 * time.Millisecond

// fanficBookBoost — множитель скоринга для записей привязанной книги
// (book_id = source_book_id, P11 §5.8): без него универсальный нарратив
// перебивает фандом.
const fanficBookBoost = 3.0

// compositionStage — стадия композиции (T-10-11): живой ход выбирает композицию
// драмы из литературного банка, а не одиночный архетип. Кандидаты — строки
// bank_pool kind='drama' с непустым family_primary; скоринг — structural.Score
// с дрейфом family_usage (T-10-10). Отказ хода запрещён: без кандидатов
// композиция нулевая, ход идёт прежним теговым путём (SCR-§4).
type compositionStage struct {
	lit    *db.Handle          // литературная база: bank_pool (только чтение).
	world  *db.Handle          // оперативная база мира: family_usage, world_meta, player_atlas.
	fams   []structural.Family // реестр семей; nil — загрузить дефолт.
	cfg    structural.Config   // веса скоринга; нулевой — DefaultConfig.
	logger *journal.Logger
	clock  ClockSource // часы бюджета; nil — реальное время.
}

func (*compositionStage) Name() string { return "composition" }

func (s *compositionStage) Run(_ context.Context, st *TurnState) error {
	if s.lit == nil {
		return nil
	}
	fams := s.fams
	if fams == nil {
		fams = defaultCompositionFamilies()
	}

	start := s.now()
	comp, score, seq, err := pickComposition(st, s.lit, s.world, fams, s.cfg)
	if err != nil {
		return nil // деградация: отказ хода запрещён (SCR-§4).
	}
	if elapsed := s.now().Sub(start); elapsed > compositionBudget {
		return nil // деградация: выборка не уложилась в бюджет.
	}
	if comp.FamilyPrimary == "" {
		return nil
	}

	comp.Mood = st.Prepath.Mood
	st.Composition = comp
	s.logComposition(comp, score, seq)
	return nil
}

func (s *compositionStage) now() time.Time {
	if s.clock != nil {
		return s.clock.Now()
	}
	return time.Now()
}

// defaultCompositionFamilies — реестр семей из базы поставки base/archetypes.json
// (T-10-01), загружается один раз за процесс. Ошибка загрузки — пустой реестр:
// композиция не выбирается (деградация без отказа).
var defaultCompositionFamilies = sync.OnceValue(func() []structural.Family {
	root := findEngineRoot()
	if root == "" {
		return nil
	}
	fams, err := structural.LoadFamilies(structural.FamiliesPath(root))
	if err != nil {
		return nil
	}
	return fams
})

// findEngineRoot поднимается от рабочего каталога до каталога с go.mod — корень
// движка, от которого считается base/archetypes.json.
func findEngineRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// dramaCandidate — кандидат композиции из bank_pool с числами дрейфа.
type dramaCandidate struct {
	structural.Candidate
	id        int64
	bookID    int64 // bank_pool.book_id; 0 — сид/старая запись без книги.
	usageBook int
}

// pickComposition читает кандидатов одним SQL, применяет дрейф и скоринг и
// возвращает выбранную композицию, её скоринг и порядковый номер выбора (seq).
func pickComposition(st *TurnState, lit, world *db.Handle, fams []structural.Family, cfg structural.Config) (structural.Composition, float64, int, error) {
	cands, err := readDramaCandidates(lit, fams)
	if err != nil {
		return structural.Composition{}, 0, 0, err
	}
	if len(cands) == 0 {
		return structural.Composition{}, 0, 0, nil
	}

	usageChapter := readUsage(world, structural.ScopeChapter)
	usageBook := readUsage(world, structural.ScopeBook)
	// Порядковый номер выбора равен сумме book-счётчиков плюс один: каждый выбор
	// инкрементирует ровно один book-счётчик на коммите.
	seq := totalUsage(usageBook) + 1

	for i := range cands {
		cands[i].UsageChapter = usageChapter[cands[i].FamilyPrimary]
		cands[i].usageBook = usageBook[cands[i].FamilyPrimary]
	}

	q := buildCompositionQuery(st, world)
	q.HasRicher = hasRicherComposition(cands)
	sourceBookID := readSourceBookID(world)

	var chosen dramaCandidate
	if seq%structural.DefaultForceEvery == 0 {
		chosen = forceCompositionPick(cands)
	} else {
		chosen = scoreCompositionPick(cands, q, cfg, usageChapter, sourceBookID)
	}
	return chosen.Composition, compositionScore(chosen, q, cfg, sourceBookID), seq, nil
}

// readDramaCandidates читает кандидатов композиции одним SQL: строки bank_pool
// kind='drama' с непустым family_primary, принадлежащим реестру; битые записи
// пропускаются (деградация до family-only остаётся в банке, но не в выборе).
func readDramaCandidates(lit *db.Handle, fams []structural.Family) ([]dramaCandidate, error) {
	famBySlug := make(map[string]structural.Family, len(fams))
	for _, f := range fams {
		famBySlug[f.Name] = f
	}

	rows, err := lit.Query(`SELECT id, book_id, family_primary, pattern_secondary, technique, moral_stance, inversion_flag, trust_level
		FROM bank_pool WHERE kind = 'drama' AND family_primary != '' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []dramaCandidate
	for rows.Next() {
		var id int64
		var bookID sql.NullInt64
		var inversionFlag, trustLevel int
		var familyPrimary string
		var secondary, technique, moralStance sql.NullString
		if err := rows.Scan(&id, &bookID, &familyPrimary, &secondary, &technique, &moralStance, &inversionFlag, &trustLevel); err != nil {
			return nil, err
		}
		fam, ok := famBySlug[familyPrimary]
		if !ok {
			continue
		}
		c := dramaCandidate{
			Candidate: structural.Candidate{
				Composition: structural.Composition{
					FamilyPrimary:    familyPrimary,
					PatternSecondary: secondary.String,
					Technique:        technique.String,
					MoralStance:      moralStance.String,
					Inversion:        inversionFlag == 1,
				},
				FamilyKeywords: fam.Keywords,
				TrustLevel:     trustLevel,
			},
			id:     id,
			bookID: bookID.Int64,
		}
		if err := c.Composition.Validate(); err != nil {
			continue
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// readUsage — счётчики дрейфа области scope; отсутствие хэндла или таблицы —
// пустая карта (деградация без отказа).
func readUsage(h *db.Handle, scope string) map[string]int {
	if h == nil {
		return map[string]int{}
	}
	usage, err := structural.Usage(h, scope)
	if err != nil {
		return map[string]int{}
	}
	return usage
}

func totalUsage(usage map[string]int) int {
	total := 0
	for _, c := range usage {
		total += c
	}
	return total
}

// buildCompositionQuery собирает запрос хода из предпути, anchor-семей мира и
// последнего интереса карты Пифии — всё чтением, без LLM.
func buildCompositionQuery(st *TurnState, world *db.Handle) structural.Query {
	q := structural.Query{
		Mood:           st.Prepath.Mood,
		AnchorFamilies: readAnchorFamilies(world),
	}
	if st.Prepath.Intent.Name != "" {
		q.Keywords = append(q.Keywords, st.Prepath.Intent.Name)
	}
	if st.Prepath.Mood != "" {
		q.Keywords = append(q.Keywords, st.Prepath.Mood)
	}
	if interest := readPlayerInterest(world); interest != "" {
		q.Keywords = append(q.Keywords, interest)
	}
	return q
}

// readAnchorFamilies читает anchor_families из world_meta (JSON-массив слагов,
// T-10-06).
func readAnchorFamilies(h *db.Handle) []string {
	if h == nil {
		return nil
	}
	var raw string
	if err := h.QueryRow(`SELECT value FROM world_meta WHERE "key" = 'anchor_families'`).Scan(&raw); err != nil {
		return nil
	}
	var fams []string
	if err := json.Unmarshal([]byte(raw), &fams); err != nil {
		return nil
	}
	return fams
}

// readPlayerInterest читает последний интерес карты Пифии (player_atlas).
func readPlayerInterest(h *db.Handle) string {
	if h == nil {
		return ""
	}
	var s string
	if err := h.QueryRow(`SELECT interest FROM player_atlas ORDER BY id DESC LIMIT 1`).Scan(&s); err != nil {
		return ""
	}
	return s
}

// hasRicherComposition — в пуле есть richer-кандидаты (с secondary/техникой/
// позицией): для family-only штрафа SCR-§3.2 правило 3.
func hasRicherComposition(cands []dramaCandidate) bool {
	for _, c := range cands {
		if c.PatternSecondary != "" || c.Technique != "" || c.MoralStance != "" {
			return true
		}
	}
	return false
}

// scoreCompositionPick — выбор наибольшего скоринга после запрета переиспользования
// семьи (Structural Drift, SCR-§3.3): семья с usage_chapter ≥ N исключается,
// если есть кандидат другой семьи; при пустом пуле запрет не применяется.
func scoreCompositionPick(cands []dramaCandidate, q structural.Query, cfg structural.Config, usageChapter map[string]int, sourceBookID int64) dramaCandidate {
	allowed := make([]dramaCandidate, 0, len(cands))
	for _, c := range cands {
		if compositionForbidden(c, cands, usageChapter) {
			continue
		}
		allowed = append(allowed, c)
	}
	if len(allowed) == 0 {
		allowed = cands
	}

	best := allowed[0]
	bestScore := compositionScore(best, q, cfg, sourceBookID)
	for _, c := range allowed[1:] {
		sc := compositionScore(c, q, cfg, sourceBookID)
		if sc > bestScore || (sc == bestScore && c.id < best.id) {
			best, bestScore = c, sc
		}
	}
	return best
}

// compositionScore — структурный скоринг с бустом привязанной книги
// (P11 §5.8): записи с book_id = source_book_id умножаются на fanficBookBoost.
func compositionScore(c dramaCandidate, q structural.Query, cfg structural.Config, sourceBookID int64) float64 {
	sc := structural.Score(c.Candidate, q, cfg)
	if sourceBookID != 0 && c.bookID == sourceBookID {
		sc *= fanficBookBoost
	}
	return sc
}

// readSourceBookID читает привязку фанфика из world_meta (T-12-13 запишет
// source_book_id при рождении); 0 — ключа нет или значение не число.
func readSourceBookID(h *db.Handle) int64 {
	if h == nil {
		return 0
	}
	var raw string
	if err := h.QueryRow(`SELECT value FROM world_meta WHERE "key" = 'source_book_id'`).Scan(&raw); err != nil {
		return 0
	}
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func compositionForbidden(c dramaCandidate, all []dramaCandidate, usageChapter map[string]int) bool {
	if usageChapter[c.FamilyPrimary] < structural.DefaultDriftForbidAfter {
		return false
	}
	for _, o := range all {
		if o.FamilyPrimary != c.FamilyPrimary {
			return true
		}
	}
	return false
}

// forceCompositionPick — принудительный выбор (seq % force_every == 0): кандидат
// с inversion, иначе наименьший usage_book; при равенстве — меньший id.
func forceCompositionPick(cands []dramaCandidate) dramaCandidate {
	best := cands[0]
	for _, c := range cands[1:] {
		if compositionForceLess(c, best) {
			best = c
		}
	}
	return best
}

func compositionForceLess(a, b dramaCandidate) bool {
	if a.Inversion != b.Inversion {
		return a.Inversion
	}
	if a.usageBook != b.usageBook {
		return a.usageBook < b.usageBook
	}
	return a.id < b.id
}

func (s *compositionStage) logComposition(c structural.Composition, score float64, seq int) {
	if s.logger == nil {
		return
	}
	_ = s.logger.Event("turn", journal.EventTurnComposition,
		journal.Field{Key: "family", Value: c.FamilyPrimary},
		journal.Field{Key: "secondary", Value: c.PatternSecondary},
		journal.Field{Key: "technique", Value: c.Technique},
		journal.Field{Key: "moral_stance", Value: c.MoralStance},
		journal.Field{Key: "inversion", Value: strconv.FormatBool(c.Inversion)},
		journal.Field{Key: "score", Value: strconv.FormatFloat(score, 'f', 4, 64)},
		journal.Field{Key: "seq", Value: strconv.Itoa(seq)})
}

// compositionGuide — строка-руководство выбранной композиции для промпта Ткача
// (T-10-11, шаг 5). Пустая композиция — руководства нет.
func compositionGuide(c structural.Composition) string {
	if c.FamilyPrimary == "" {
		return ""
	}
	breakExpectation := "нет"
	if c.Inversion {
		breakExpectation = "да"
	}
	return fmt.Sprintf("приём: %s; позиция: %s; слом ожидания: %s", c.Technique, c.MoralStance, breakExpectation)
}

// commitCompositionUsage фиксирует использование выбранной семьи одной
// транзакцией core: бумп chapter- и book-счётчиков family_usage (T-10-11, шаг 6).
// Нулевая композиция — записи нет.
func commitCompositionUsage(h *db.Handle, c structural.Composition, now string) error {
	if h == nil || c.FamilyPrimary == "" {
		return nil
	}
	return h.Tx(db.WriterCore, func(tx *db.Tx) error {
		for _, scope := range []string{structural.ScopeChapter, structural.ScopeBook} {
			if _, err := tx.Exec("family_usage",
				`INSERT INTO family_usage (scope, family, count, updated_at) VALUES (?, ?, 1, ?)
				 ON CONFLICT(scope, family) DO UPDATE SET count = family_usage.count + 1, updated_at = excluded.updated_at`,
				scope, c.FamilyPrimary, now); err != nil {
				return err
			}
		}
		return nil
	})
}
