// Package bank — Челнок: банк рутины литературной базы. Выбор записи из
// bank_pool по тегам сцены идёт детерминированным кодом, без вызова слота
// генерации (LT-08.3); банк хранит форму, мир подставляет факты (LT-08.5).
package bank

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"tns65/internal/db"
)

// Slot — слот генерации прозы (T-02-14). Челнок его не вызывает: выбор идёт
// кодом, без дрейфа модели; ссылка хранится, чтобы проверять «ноль вызовов».
type Slot interface {
	Generate(ctx context.Context, role, prompt string) (string, error)
}

// Clock — источник времени бюджета выборки; testkit.Clock удовлетворяет контракту.
type Clock interface {
	Now() time.Time
}

// Binder — подстановка фактов мира в форму записи банка (LT-08.5): банк хранит
// форму с токенами-заменителями, мир — факты (имена, цены, места).
type Binder interface {
	Bind(text string) string
}

// Budget — бюджет выборки записи банка (T-03-07, шаг 1): Челнок отвечает быстро,
// в отличие от Ткача.
const Budget = 100 * time.Millisecond

// recentWindow — сколько последних наборов тегов помнит банк для «повторяемого»
// (T-03-07, шаг 1): тот же набор тегов в пределах окна выдаёт ту же запись.
const recentWindow = 3

// Ошибки Челнока.
var (
	// ErrNoRecord — в банке нет записи, подходящей тегам сцены.
	ErrNoRecord = errors.New("bank: нет записи под теги сцены")
	// ErrNoReader — читатель литературной базы не задан.
	ErrNoReader = errors.New("bank: читатель литературной базы не задан")
	// ErrEmptyPattern — у узора сцены нет структуры: пусты участники, локация и тип.
	ErrEmptyPattern = errors.New("bank: пустой узор сцены")
)

// ScenePattern — структурный ключ сцены-узора (T-19-12): состав участников,
// локация и тип сцены из оседания. Одинаковый ключ описывает ту же сцену-узор.
type ScenePattern struct {
	Participants []string // состав участников; порядок и регистр не важны.
	Location     string   // локация сцены.
	SceneType    string   // тип сцены (намерение из оседания).
}

// Key — каноническая форма узора: участники без повторов в нижнем регистре по
// алфавиту, затем локация и тип сцены. Пустой ключ — узор без структуры.
func (p ScenePattern) Key() string {
	people := sortedKeys(tagSet(p.Participants))
	loc := normalizeField(p.Location)
	kind := normalizeField(p.SceneType)
	if len(people) == 0 && loc == "" && kind == "" {
		return ""
	}
	return strings.Join(people, "+") + "|" + loc + "|" + kind
}

// Tags — теги сцены для обычного выбора из bank_pool (T-03-07): участники,
// локация и тип сцены узора.
func (p ScenePattern) Tags() []string {
	tags := make([]string, 0, len(p.Participants)+2)
	tags = append(tags, p.Participants...)
	tags = append(tags, p.Location, p.SceneType)
	return sortedKeys(tagSet(tags))
}

// Record — запись пула банка bank_pool литературной базы.
type Record struct {
	ID   int64
	Kind string
	Text string
	Tags string
}

// recentPick — запомненный выбор по набору тегов для «повторяемого».
type recentPick struct {
	key string
	rec Record
}

// patternEntry — запись кэша узоров: форма записи банка и счётчик попаданий.
type patternEntry struct {
	rec  Record
	hits int
}

// PatternCache — кэш сцен-узоров Челнока (T-19-12): структурный ключ узора →
// форма записи банка. Кэш общий на процесс: Банк собирается на ход, а сцена-узор
// переживает ходы, поэтому повторный запрос отдаётся без слота генерации.
// Хранится форма без фактов мира — биндер подставляет их при каждой отдаче (LT-08.5).
type PatternCache struct {
	mu      sync.Mutex
	entries map[string]*patternEntry
}

// NewPatternCache — пустой кэш узоров.
func NewPatternCache() *PatternCache {
	return &PatternCache{entries: make(map[string]*patternEntry)}
}

// defaultPatternCache — кэш узоров для Банков, собранных без своего.
var defaultPatternCache = NewPatternCache()

// DefaultPatternCache — общий на процесс кэш сцен-узоров.
func DefaultPatternCache() *PatternCache { return defaultPatternCache }

// Lookup отдаёт форму записи узора; попадание растит счётчик узора.
func (c *PatternCache) Lookup(key string) (Record, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return Record{}, false
	}
	e.hits++
	return e.rec, true
}

// Remember записывает новый узор; знакомому ключу обновляет форму, счётчик хранит.
func (c *PatternCache) Remember(key string, rec Record) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		e.rec = rec
		return
	}
	c.entries[key] = &patternEntry{rec: rec}
}

// Known — есть ли узор в кэше, без роста счётчика: вход расширения RuleRepeatable.
func (c *PatternCache) Known(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.entries[key]
	return ok
}

// Hits — счётчик попаданий узора для Селектора (T-19-12).
func (c *PatternCache) Hits(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		return e.hits
	}
	return 0
}

// Len — число узоров в кэше.
func (c *PatternCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Bank — Челнок: выбор записи из bank_pool по тегам сцены.
type Bank struct {
	reader *db.Handle // литературная база; только чтение (bank_pool пишет forge).
	clock  Clock
	slot   Slot // не вызывается; присутствует для проверки «ноль вызовов слота».
	binder Binder
	recent []recentPick
	cache  *PatternCache // кэш узоров (T-19-12); nil — общий на процесс.
}

// Option — настройка Челнока при сборке (T-19-12).
type Option func(*Bank)

// WithPatternCache подключает свой кэш узоров вместо общего на процесс.
func WithPatternCache(c *PatternCache) Option {
	return func(b *Bank) { b.cache = c }
}

// New собирает Челнок над литературной базой reader (чтение), часами clock,
// слотом генерации slot и биндером фактов binder. clock и binder могут быть nil:
// тогда время берётся реальное, а форма выдаётся без подстановки фактов.
func New(reader *db.Handle, clock Clock, slot Slot, binder Binder, opts ...Option) *Bank {
	b := &Bank{reader: reader, clock: clock, slot: slot, binder: binder}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Pick выбирает запись банка по тегам сцены tags и возвращает её с подставленными
// фактами. Выбор детерминированный, без вызова слота генерации, в бюджете Budget.
// Повторяемое: тот же набор тегов в пределах recentWindow последних ходов выдаёт
// ту же запись.
func (b *Bank) Pick(tags []string) (Record, error) {
	if b.reader == nil {
		return Record{}, ErrNoReader
	}
	start := b.now()

	rec, err := b.pick(tags)
	if err != nil {
		return Record{}, err
	}

	if elapsed := b.now().Sub(start); elapsed > Budget {
		return Record{}, fmt.Errorf("bank: выборка превысила бюджет: %v > %v", elapsed, Budget)
	}
	return rec, nil
}

// pick — выбор с памятью последних ходов: попадание в окно возвращает прошлый
// выбор без нового чтения; иначе — детерминированный выбор и запоминание.
func (b *Bank) pick(tags []string) (Record, error) {
	rec, err := b.pickForm(tags)
	if err != nil {
		return Record{}, err
	}
	return b.bind(rec), nil
}

// pickForm — выбор формы записи без подстановки фактов мира: в окне последних
// ходов и в кэше узоров хранится форма, факты подставляет биндер при отдаче.
func (b *Bank) pickForm(tags []string) (Record, error) {
	key := canonicalTags(tags)
	if i := b.recentIndex(key); i >= 0 {
		return b.recent[i].rec, nil
	}

	rec, err := b.selectRecord(tags)
	if err != nil {
		return Record{}, err
	}
	b.remember(key, rec)
	return rec, nil
}

// PickScene отдаёт заготовку по узору сцены без вызова слота генерации (T-19-12,
// расширение RuleRepeatable): попадание в кэш узоров возвращает ту же форму с
// подставленными фактами и растит счётчик; промах идёт обычным путём выбора и
// записывает новый узор. Бюджет выборки — тот же Budget.
func (b *Bank) PickScene(p ScenePattern) (Record, error) {
	if b.reader == nil {
		return Record{}, ErrNoReader
	}
	key := p.Key()
	if key == "" {
		return Record{}, ErrEmptyPattern
	}
	start := b.now()
	cache := b.patterns()

	rec, hit := cache.Lookup(key)
	if !hit {
		selected, err := b.pickForm(p.Tags())
		if err != nil {
			return Record{}, err
		}
		cache.Remember(key, selected)
		rec = selected
	}

	if elapsed := b.now().Sub(start); elapsed > Budget {
		return Record{}, fmt.Errorf("bank: выборка превысила бюджет: %v > %v", elapsed, Budget)
	}
	return b.bind(rec), nil
}

// PatternKnown — известен ли узор: вход Селектора для RuleRepeatable без слота.
func (b *Bank) PatternKnown(p ScenePattern) bool {
	key := p.Key()
	return key != "" && b.patterns().Known(key)
}

// PatternHits — сколько раз узор отдан из кэша: счётчик попаданий для Селектора.
func (b *Bank) PatternHits(p ScenePattern) int {
	key := p.Key()
	if key == "" {
		return 0
	}
	return b.patterns().Hits(key)
}

// patterns — кэш узоров Банка; nil — общий на процесс.
func (b *Bank) patterns() *PatternCache {
	if b.cache != nil {
		return b.cache
	}
	return defaultPatternCache
}

// selectRecord — детерминированный выбор записи: среди записей, чьи теги
// пересекаются с тегами сцены, побеждает запись с наибольшим числом совпадений;
// при равенстве — меньший id. Пересечения нет, но банк непуст — литературная
// запись всё равно отдаётся (selectDefault), иначе Челнок падал бы в заглушку
// на обычных рутинных фразах без словарных категорий.
func (b *Bank) selectRecord(tags []string) (Record, error) {
	rows, err := b.reader.Query(`SELECT id, kind, text, tags FROM bank_pool`)
	if err != nil {
		return Record{}, err
	}
	defer rows.Close()

	want := tagSet(tags)
	var best Record
	bestScore := -1
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ID, &rec.Kind, &rec.Text, &rec.Tags); err != nil {
			return Record{}, err
		}
		score := overlapScore(want, parseTags(rec.Tags))
		switch {
		case score > bestScore:
			bestScore = score
			best = rec
		case score == bestScore && bestScore > 0 && rec.ID < best.ID:
			best = rec
		}
	}
	if err := rows.Err(); err != nil {
		return Record{}, err
	}
	if bestScore > 0 {
		return best, nil
	}
	return b.selectDefault()
}

// selectDefault — дефолтная запись для сцены без пересечения тегов: рутина
// (kind='routine') с наименьшим usage_count, при равенстве — меньший id. Рутины
// нет, но банк непуст — берётся любая запись по тому же порядку: заглушка
// fallback остаётся только для пустого банка.
func (b *Bank) selectDefault() (Record, error) {
	var rec Record
	err := b.reader.QueryRow(
		`SELECT id, kind, text, tags FROM bank_pool
		 ORDER BY CASE WHEN kind = 'routine' THEN 0 ELSE 1 END, usage_count ASC, id ASC
		 LIMIT 1`).Scan(&rec.ID, &rec.Kind, &rec.Text, &rec.Tags)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, ErrNoRecord
	}
	if err != nil {
		return Record{}, err
	}
	return rec, nil
}

// bind подставляет факты мира в форму записи биндером; без биндера — форма как есть.
func (b *Bank) bind(rec Record) Record {
	if b.binder != nil {
		rec.Text = b.binder.Bind(rec.Text)
	}
	return rec
}

// now — текущее время часов; nil — реальное время.
func (b *Bank) now() time.Time {
	if b.clock != nil {
		return b.clock.Now()
	}
	return time.Now()
}

// recentIndex — номер в окне памяти по каноническому ключу; −1 — нет.
func (b *Bank) recentIndex(key string) int {
	for i := len(b.recent) - 1; i >= 0; i-- {
		if b.recent[i].key == key {
			return i
		}
	}
	return -1
}

// remember кладёт выбор в окно памяти и держит его длиной не более recentWindow.
func (b *Bank) remember(key string, rec Record) {
	b.recent = append(b.recent, recentPick{key: key, rec: rec})
	if n := len(b.recent); n > recentWindow {
		b.recent = b.recent[n-recentWindow:]
	}
}

// canonicalTags — каноническая форма набора тегов для сравнения «повторяемого».
func canonicalTags(tags []string) string {
	return strings.Join(sortedKeys(tagSet(tags)), ",")
}

// normalizeField — поле структурного ключа: нижний регистр без пробелов по краям.
func normalizeField(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// sortedKeys — ключи набора по алфавиту: канонический порядок тегов и узоров.
func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// tagSet — нормализованный набор тегов: без пустых, в нижнем регистре.
func tagSet(tags []string) map[string]bool {
	out := make(map[string]bool)
	for _, t := range tags {
		if t = strings.TrimSpace(strings.ToLower(t)); t != "" {
			out[t] = true
		}
	}
	return out
}

// parseTags — теги записи банка: поле tags — список через запятую.
func parseTags(s string) map[string]bool {
	return tagSet(strings.Split(s, ","))
}

// overlapScore — число тегов сцены, встретившихся в тегах записи.
func overlapScore(want, have map[string]bool) int {
	n := 0
	for k := range want {
		if have[k] {
			n++
		}
	}
	return n
}
