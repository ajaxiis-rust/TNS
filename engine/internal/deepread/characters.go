package deepread

import (
	"bufio"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"tns65/internal/db"
	"tns65/internal/models"
)

// Персонажи книги (W-25, T-14-07): центроид векторов окон упоминаний, слияние
// алиасов по косинусу центроидов и эмоциональный профиль. Карта персонажей P11
// (book_character*) остаётся владельцем строк: волна дополняет поля и векторы,
// новых строк не создаёт.

const (
	// CharacterVectorTable — owner_table центроидов персонажей в vectors
	// литературной базы (W-25.1).
	CharacterVectorTable = "book_character"
	// characterVectorModel — модель векторов разбора (T-14-02).
	characterVectorModel = "deepread-1024"
	// characterVectorLang — язык книги разбора (пак ru).
	characterVectorLang = "ru"
	// aliasCosineMargin — полуширина полосы спорных пар вокруг порога (шаг 4).
	aliasCosineMargin = 0.05
	// defaultAliasCosine — deepread.alias_cosine конфига по умолчанию (W-25.2).
	defaultAliasCosine = 0.85
)

// CharacterConfig — порог слияния кандидатов персонажей (шаг 4): косинус
// центроидов окон из deepread.alias_cosine конфига.
type CharacterConfig struct {
	AliasCosine float64
}

// DefaultCharacterConfig возвращает порог, совпадающий со значением config.yaml.
func DefaultCharacterConfig() CharacterConfig {
	return CharacterConfig{AliasCosine: defaultAliasCosine}
}

// LoadCharacterConfig читает порог слияния из файла конфигурации.
func LoadCharacterConfig(path string) (CharacterConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CharacterConfig{}, err
	}
	return parseCharacterSection(string(data))
}

// parseCharacterSection разбирает ключ alias_cosine секции deepread; прочие
// ключи секции не читаются, отсутствие ключа оставляет дефолт.
func parseCharacterSection(data string) (CharacterConfig, error) {
	cfg := DefaultCharacterConfig()

	inDeepread := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			inDeepread = trimmed == "deepread:"
			continue
		}
		if !inDeepread || indent > 2 {
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok || strings.TrimSpace(key) != "alias_cosine" {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return CharacterConfig{}, fmt.Errorf("deepread: alias_cosine: %w", err)
		}
		cfg.AliasCosine = f
	}
	if err := sc.Err(); err != nil {
		return CharacterConfig{}, err
	}
	if err := validateCharacterConfig(cfg); err != nil {
		return CharacterConfig{}, err
	}
	return cfg, nil
}

// validateCharacterConfig проверяет порог слияния: 0 ≤ alias_cosine ≤ 1.
func validateCharacterConfig(cfg CharacterConfig) error {
	if math.IsNaN(cfg.AliasCosine) || cfg.AliasCosine < 0 || cfg.AliasCosine > 1 {
		return fmt.Errorf("deepread: alias_cosine %g вне 0..1", cfg.AliasCosine)
	}
	return nil
}

// EmotionDictionary — словарь emotion пака detectors_mood.json (W-22.2,
// W-25.5): категория настроения и её леммы.
type EmotionDictionary struct {
	Mood   string
	Lemmas map[string]struct{}
}

// LoadEmotionDictionary читает detectors_mood.json. Пустая категория или пустой
// набор лемм — ошибка: тихий пустой словарь обнулил бы профиль эмоций.
func LoadEmotionDictionary(path string) (*EmotionDictionary, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var file struct {
		Mood   string   `json:"mood"`
		Lemmas []string `json:"lemmas"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("deepread: detectors_mood.json: %w", err)
	}

	mood := strings.TrimSpace(file.Mood)
	if mood == "" {
		return nil, fmt.Errorf("deepread: detectors_mood.json: пустая категория emotion")
	}
	dict := &EmotionDictionary{Mood: mood, Lemmas: make(map[string]struct{}, len(file.Lemmas))}
	for _, lemma := range file.Lemmas {
		lemma = strings.ToLower(strings.TrimSpace(lemma))
		if lemma == "" {
			continue
		}
		dict.Lemmas[lemma] = struct{}{}
	}
	if len(dict.Lemmas) == 0 {
		return nil, fmt.Errorf("deepread: detectors_mood.json: у категории %q нет лемм", mood)
	}
	return dict, nil
}

// hits сообщает, есть ли в леммах окна лемма категории emotion.
func (d *EmotionDictionary) hits(lemmas []Lemma) bool {
	if d == nil {
		return false
	}
	for _, lemma := range lemmas {
		if _, ok := d.Lemmas[strings.ToLower(lemma.Lemma)]; ok {
			return true
		}
	}
	return false
}

// CharacterWindow — окно упоминания персонажа (W-25.1): чанк карты, счётчик
// упоминаний в нём, вектор чанка ковки и леммы чанка.
type CharacterWindow struct {
	ChunkID  int64
	Mentions int64
	Vector   []float32
	Lemmas   []Lemma
}

// CharacterCandidate — кандидат карты P11 (строка book_character) с окнами
// упоминаний из book_character_mention.
type CharacterCandidate struct {
	ID         int64
	Name       string
	Aliases    []string
	FirstChunk int64
	LastChunk  int64
	Mentions   int64
	Windows    []CharacterWindow
}

// EmotionProfile — доли категорий emotion по окнам упоминаний (W-25.5):
// числитель — упоминания окон с леммой категории, знаменатель — все упоминания
// окон группы; значения в 0..1.
type EmotionProfile map[string]float64

// CharacterGroup — слитая запись персонажа (шаги 3–5): старшая строка карты,
// алиасы младших, окна группы без повторов чанков, центроид и профиль эмоций.
type CharacterGroup struct {
	ID         int64
	Name       string
	Aliases    []string
	Members    []int64
	FirstChunk int64
	LastChunk  int64
	Mentions   int64
	Windows    []CharacterWindow
	Centroid   []float32
	Emotion    EmotionProfile
}

// DisputedAlias — пограничная пара кандидатов (шаг 4): косинус центроидов в
// пределах ±0.05 от порога, решение за слотом (T-14-10).
type DisputedAlias struct {
	SeniorID   int64
	JuniorID   int64
	SeniorName string
	JuniorName string
	Cosine     float64
}

// Centroid — центроид окон (шаг 3): среднее векторов окон; окна без вектора и
// окна другой размерности в среднее не входят. nil — векторов в окнах нет.
func Centroid(windows []CharacterWindow) []float32 {
	var sum []float32
	n := 0
	for _, window := range windows {
		if len(window.Vector) == 0 {
			continue
		}
		if sum == nil {
			sum = make([]float32, len(window.Vector))
		}
		if len(window.Vector) != len(sum) {
			continue
		}
		for i, v := range window.Vector {
			sum[i] += v
		}
		n++
	}
	if n == 0 {
		return nil
	}
	for i := range sum {
		sum[i] /= float32(n)
	}
	return sum
}

// Cosine — косинусная близость двух векторов; расхождение размерности и
// нулевая норма дают 0.
func Cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
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

// MergeCharacters строит группы персонажей (шаги 3–5): центроид кандидата —
// среднее векторов его окон; пара с общим чанком упоминаний и косинусом
// центроидов ≥ alias_cosine сливается в старшую запись (алиас — имя младшей),
// косинус в пределах ±0.05 от порога уходит в спорные пары (T-14-10), ниже
// порога пара не сливается.
func MergeCharacters(cands []CharacterCandidate, dict *EmotionDictionary, cfg CharacterConfig) ([]CharacterGroup, []DisputedAlias, error) {
	if cfg == (CharacterConfig{}) {
		cfg = DefaultCharacterConfig()
	}
	if err := validateCharacterConfig(cfg); err != nil {
		return nil, nil, err
	}
	if len(cands) == 0 {
		return nil, nil, nil
	}

	seen := make(map[int64]struct{}, len(cands))
	for _, cand := range cands {
		if cand.ID <= 0 {
			return nil, nil, fmt.Errorf("deepread: персонаж без id карты")
		}
		if _, dup := seen[cand.ID]; dup {
			return nil, nil, fmt.Errorf("deepread: персонаж %d повторяется", cand.ID)
		}
		seen[cand.ID] = struct{}{}
	}

	centroids := make([][]float32, len(cands))
	chunkSets := make([]map[int64]struct{}, len(cands))
	for i, cand := range cands {
		centroids[i] = Centroid(cand.Windows)
		chunkSets[i] = make(map[int64]struct{}, len(cand.Windows))
		for _, window := range cand.Windows {
			chunkSets[i][window.ChunkID] = struct{}{}
		}
	}

	parent := make([]int, len(cands))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}

	type borderPair struct {
		left, right int
		cosine      float64
	}
	var borders []borderPair
	for i := 0; i < len(cands); i++ {
		for j := i + 1; j < len(cands); j++ {
			if !shareWindow(chunkSets[i], chunkSets[j]) {
				continue
			}
			cosine := Cosine(centroids[i], centroids[j])
			switch {
			case cosine >= cfg.AliasCosine+aliasCosineMargin:
				union(i, j)
			case cosine >= cfg.AliasCosine-aliasCosineMargin:
				borders = append(borders, borderPair{left: i, right: j, cosine: cosine})
			}
		}
	}

	var disputed []DisputedAlias
	for _, pair := range borders {
		if find(pair.left) == find(pair.right) {
			continue
		}
		senior, junior := cands[pair.left], cands[pair.right]
		if candidateSenior(junior, senior) {
			senior, junior = junior, senior
		}
		disputed = append(disputed, DisputedAlias{
			SeniorID:   senior.ID,
			JuniorID:   junior.ID,
			SeniorName: senior.Name,
			JuniorName: junior.Name,
			Cosine:     pair.cosine,
		})
	}
	sort.Slice(disputed, func(a, b int) bool {
		if disputed[a].Cosine != disputed[b].Cosine {
			return disputed[a].Cosine > disputed[b].Cosine
		}
		if disputed[a].SeniorID != disputed[b].SeniorID {
			return disputed[a].SeniorID < disputed[b].SeniorID
		}
		return disputed[a].JuniorID < disputed[b].JuniorID
	})

	byRoot := make(map[int][]int, len(cands))
	for i := range cands {
		root := find(i)
		byRoot[root] = append(byRoot[root], i)
	}

	groups := make([]CharacterGroup, 0, len(byRoot))
	for _, members := range byRoot {
		sort.Slice(members, func(a, b int) bool {
			return candidateSenior(cands[members[a]], cands[members[b]])
		})
		groups = append(groups, buildGroup(cands, members, dict))
	}
	sort.Slice(groups, func(a, b int) bool {
		if groups[a].Mentions != groups[b].Mentions {
			return groups[a].Mentions > groups[b].Mentions
		}
		if groups[a].FirstChunk != groups[b].FirstChunk {
			return groups[a].FirstChunk < groups[b].FirstChunk
		}
		return groups[a].ID < groups[b].ID
	})
	return groups, disputed, nil
}

// buildGroup собирает группу по старшинству: members отсортированы старшей
// записью вперёд, окна группы не повторяют чанк, центроид — среднее окон.
func buildGroup(cands []CharacterCandidate, members []int, dict *EmotionDictionary) CharacterGroup {
	senior := cands[members[0]]
	group := CharacterGroup{
		ID:      senior.ID,
		Name:    senior.Name,
		Aliases: append([]string(nil), senior.Aliases...),
		Members: make([]int64, 0, len(members)),
	}

	seenChunks := make(map[int64]struct{})
	var first, last int64
	edges := false
	for _, idx := range members {
		cand := cands[idx]
		group.Members = append(group.Members, cand.ID)
		if cand.ID != senior.ID {
			group.Aliases = appendAlias(group.Aliases, cand.Name)
		}

		mentions := cand.Mentions
		if mentions <= 0 {
			for _, window := range cand.Windows {
				mentions += int64(windowMentions(window))
			}
		}
		group.Mentions += mentions

		for _, edge := range []int64{cand.FirstChunk, cand.LastChunk} {
			if edge <= 0 {
				continue
			}
			edges = true
			if first == 0 || edge < first {
				first = edge
			}
			if edge > last {
				last = edge
			}
		}

		for _, window := range cand.Windows {
			if _, dup := seenChunks[window.ChunkID]; dup {
				continue
			}
			seenChunks[window.ChunkID] = struct{}{}
			group.Windows = append(group.Windows, window)
		}
	}

	if !edges {
		for _, window := range group.Windows {
			if window.ChunkID <= 0 {
				continue
			}
			if first == 0 || window.ChunkID < first {
				first = window.ChunkID
			}
			if window.ChunkID > last {
				last = window.ChunkID
			}
		}
	}
	group.FirstChunk, group.LastChunk = first, last
	group.Centroid = Centroid(group.Windows)
	group.Emotion = emotionProfile(group.Windows, dict)
	return group
}

// emotionProfile считает доли категорий emotion по окнам группы (W-25.5):
// окно без счётчика упоминаний считается одним упоминанием, чтобы знаменатель
// не обнулялся.
func emotionProfile(windows []CharacterWindow, dict *EmotionDictionary) EmotionProfile {
	if dict == nil {
		return EmotionProfile{}
	}
	total, hit := 0, 0
	for _, window := range windows {
		mentions := windowMentions(window)
		total += mentions
		if dict.hits(window.Lemmas) {
			hit += mentions
		}
	}
	share := 0.0
	if total > 0 {
		share = float64(hit) / float64(total)
	}
	return EmotionProfile{dict.Mood: share}
}

// windowMentions — вес окна в профиле: счётчик карты, при нуле — одно
// упоминание.
func windowMentions(window CharacterWindow) int {
	if window.Mentions > 0 {
		return int(window.Mentions)
	}
	return 1
}

// shareWindow сообщает, есть ли у двух кандидатов общий чанк упоминаний
// (W-25.2: общая вершина упоминаний).
func shareWindow(a, b map[int64]struct{}) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	for chunkID := range a {
		if _, ok := b[chunkID]; ok {
			return true
		}
	}
	return false
}

// candidateSenior — порядок старшинства: больше упоминаний, раньше первый
// чанк, меньше id — старше.
func candidateSenior(a, b CharacterCandidate) bool {
	if a.Mentions != b.Mentions {
		return a.Mentions > b.Mentions
	}
	if af, bf := candidateFirstChunk(a), candidateFirstChunk(b); af != bf {
		return af < bf
	}
	return a.ID < b.ID
}

// candidateFirstChunk — первый чанк кандидата для порядка старшинства: поле
// карты, при нуле — минимальный чанк окон.
func candidateFirstChunk(cand CharacterCandidate) int64 {
	if cand.FirstChunk > 0 {
		return cand.FirstChunk
	}
	first := int64(0)
	for _, window := range cand.Windows {
		if window.ChunkID <= 0 {
			continue
		}
		if first == 0 || window.ChunkID < first {
			first = window.ChunkID
		}
	}
	if first == 0 {
		return math.MaxInt64
	}
	return first
}

// appendAlias добавляет алиас без пустых значений и повторов.
func appendAlias(aliases []string, alias string) []string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return aliases
	}
	for _, have := range aliases {
		if strings.EqualFold(strings.TrimSpace(have), alias) {
			return aliases
		}
	}
	return append(aliases, alias)
}

// LoadCharacterCandidates читает кандидатов книги и их окна из карты P11 (шаг
// 1): строки book_character и упоминания book_character_mention.
func LoadCharacterCandidates(lit *db.Handle, bookID int64) ([]CharacterCandidate, error) {
	if lit == nil {
		return nil, ErrNoStore
	}
	if bookID <= 0 {
		return nil, fmt.Errorf("deepread: персонажи: book_id %d", bookID)
	}

	rows, err := lit.Query(
		`SELECT c.id, c.name, c.aliases, c.first_chunk, c.last_chunk, c.mentions,
		        m.chunk_id, m.count
		 FROM book_character c
		 LEFT JOIN book_character_mention m ON m.character_id = c.id
		 WHERE c.book_id = ?
		 ORDER BY c.id, m.chunk_id`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CharacterCandidate
	index := make(map[int64]int)
	for rows.Next() {
		var (
			id                    int64
			name, aliases         string
			first, last, mentions int64
			chunkID, count        sql.NullInt64
		)
		if err := rows.Scan(&id, &name, &aliases, &first, &last, &mentions, &chunkID, &count); err != nil {
			return nil, err
		}

		pos, ok := index[id]
		if !ok {
			parsed, err := parseAliases(id, aliases)
			if err != nil {
				return nil, err
			}
			out = append(out, CharacterCandidate{
				ID:         id,
				Name:       name,
				Aliases:    parsed,
				FirstChunk: first,
				LastChunk:  last,
				Mentions:   mentions,
			})
			pos = len(out) - 1
			index[id] = pos
		}
		if chunkID.Valid {
			out[pos].Windows = append(out[pos].Windows, CharacterWindow{
				ChunkID:  chunkID.Int64,
				Mentions: count.Int64,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// parseAliases разбирает JSON-массив алиасов карты; пустое поле — нет алиасов.
func parseAliases(characterID int64, raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var aliases []string
	if err := json.Unmarshal([]byte(raw), &aliases); err != nil {
		return nil, fmt.Errorf("deepread: алиасы героя %d: %w", characterID, err)
	}
	return aliases, nil
}

// AttachWindowVectors подставляет векторы окон из оперативной базы (шаг 2):
// вектор чанка посчитан ковкой (writeBankVector) и лежит в vectors с
// owner_table='bank_pool'; запись банка чанка находится по origin
// forge-book:<book_id>:<ord>, где ord — номер чанка карты book_chunk.
func AttachWindowVectors(lit, world *db.Handle, bookID int64, cands []CharacterCandidate) error {
	if lit == nil || world == nil {
		return ErrNoStore
	}
	if bookID <= 0 {
		return fmt.Errorf("deepread: векторы окон: book_id %d", bookID)
	}

	seen := make(map[int64]struct{})
	ids := make([]int64, 0)
	for _, cand := range cands {
		for _, window := range cand.Windows {
			if _, dup := seen[window.ChunkID]; dup {
				continue
			}
			seen[window.ChunkID] = struct{}{}
			ids = append(ids, window.ChunkID)
		}
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })

	vectors, err := chunkVectors(lit, world, bookID, ids)
	if err != nil {
		return err
	}
	for ci := range cands {
		for wi := range cands[ci].Windows {
			if vec, ok := vectors[cands[ci].Windows[wi].ChunkID]; ok {
				cands[ci].Windows[wi].Vector = vec
			}
		}
	}
	return nil
}

// chunkVectors строит карту «id чанка карты → вектор чанка»: ord чанка →
// origin записи банка ковки (forge-book:<book_id>:<ord>) → vectors оперативной
// базы. Чанк без записи банка или без вектора в карту не попадает.
func chunkVectors(lit, world *db.Handle, bookID int64, chunkIDs []int64) (map[int64][]float32, error) {
	out := make(map[int64][]float32, len(chunkIDs))
	if len(chunkIDs) == 0 {
		return out, nil
	}

	ords := make(map[int64]int64, len(chunkIDs))
	rows, err := lit.Query(`SELECT id, ord FROM book_chunk WHERE book_id = ?`, bookID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, ord int64
		if err := rows.Scan(&id, &ord); err != nil {
			rows.Close()
			return nil, err
		}
		ords[id] = ord
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	prefix := fmt.Sprintf("forge-book:%d:", bookID)
	bankByOrd := make(map[int64]int64)
	rows, err = lit.Query(
		`SELECT id, origin FROM bank_pool WHERE book_id = ? AND origin LIKE ?`, bookID, prefix+"%")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var origin string
		if err := rows.Scan(&id, &origin); err != nil {
			rows.Close()
			return nil, err
		}
		ord, ok := parseForgeOrigin(origin, prefix)
		if !ok {
			continue
		}
		bankByOrd[ord] = id
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for _, chunkID := range chunkIDs {
		ord, ok := ords[chunkID]
		if !ok {
			continue
		}
		bankID, ok := bankByOrd[ord]
		if !ok {
			continue
		}

		var blob []byte
		err := world.QueryRow(
			`SELECT vec FROM vectors WHERE owner_table = 'bank_pool' AND owner_id = ? ORDER BY id DESC LIMIT 1`,
			bankID).Scan(&blob)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		vec, err := vectorFromBlob(blob)
		if err != nil {
			return nil, fmt.Errorf("deepread: вектор чанка %d: %w", chunkID, err)
		}
		out[chunkID] = vec
	}
	return out, nil
}

// parseForgeOrigin разбирает номер чанка ковки из origin записи банка.
func parseForgeOrigin(origin, prefix string) (int64, bool) {
	if !strings.HasPrefix(origin, prefix) {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimPrefix(origin, prefix), 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// vectorFromBlob разбирает BLOB Float32 little-endian (D-17.2); длина не
// кратная 4 — битый BLOB.
func vectorFromBlob(blob []byte) ([]float32, error) {
	if len(blob)%4 != 0 {
		return nil, fmt.Errorf("длина BLOB %d не кратна 4", len(blob))
	}
	vec := make([]float32, len(blob)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return vec, nil
}

// vectorBlob кодирует вектор в BLOB Float32 little-endian (D-17.2).
func vectorBlob(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, f := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// AttachWindowLemmas подставляет леммы чанков прогона (T-14-04) в окна
// упоминаний: окно — чанк упоминания, леммы нужны профилю эмоций (W-25.5).
func AttachWindowLemmas(cands []CharacterCandidate, chunks []SalienceChunk) {
	byChunk := make(map[int64][]Lemma, len(chunks))
	for _, chunk := range chunks {
		byChunk[chunk.ID] = chunk.Lemmas
	}
	for ci := range cands {
		for wi := range cands[ci].Windows {
			if lemmas, ok := byChunk[cands[ci].Windows[wi].ChunkID]; ok {
				cands[ci].Windows[wi].Lemmas = lemmas
			}
		}
	}
}

// CharacterInput — вход разбора персонажей: книга, чанки прогона с леммами,
// словарь эмоций и порог слияния.
type CharacterInput struct {
	BookID  int64
	Chunks  []SalienceChunk
	Emotion *EmotionDictionary
	Config  CharacterConfig
}

// BuildCharacters проходит шаги 1–5 без записи в базу: карта P11 → векторы
// окон → центроиды и слияние алиасов → профили эмоций.
func BuildCharacters(lit, world *db.Handle, in CharacterInput) ([]CharacterGroup, []DisputedAlias, error) {
	cands, err := LoadCharacterCandidates(lit, in.BookID)
	if err != nil {
		return nil, nil, err
	}
	if err := AttachWindowVectors(lit, world, in.BookID, cands); err != nil {
		return nil, nil, err
	}
	AttachWindowLemmas(cands, in.Chunks)
	return MergeCharacters(cands, in.Emotion, in.Config)
}

// WriteCharacterCentroids пишет центроиды групп в vectors литературной базы
// (шаг 3): owner_table='book_character', owner_id — id старшей строки карты.
// Прежний вектор персонажа заменяется, повторный прогон строк не плодит; вектор
// имени не строится — source_text описывает окна упоминаний.
func WriteCharacterCentroids(lit *db.Handle, groups []CharacterGroup, now string) error {
	if lit == nil {
		return ErrNoStore
	}
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	for _, group := range groups {
		if group.ID <= 0 {
			return fmt.Errorf("deepread: центроид: персонаж без id карты")
		}
		if len(group.Centroid) == 0 {
			continue
		}
		if len(group.Centroid) != models.EmbedDim {
			return fmt.Errorf("deepread: центроид персонажа %d: dim %d, want %d",
				group.ID, len(group.Centroid), models.EmbedDim)
		}

		if err := execLiteraryVectors(lit,
			`DELETE FROM vectors WHERE owner_table = ? AND owner_id = ?`,
			CharacterVectorTable, group.ID); err != nil {
			return fmt.Errorf("deepread: вектор персонажа %d: %w", group.ID, err)
		}
		if err := execLiteraryVectors(lit,
			`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			CharacterVectorTable, group.ID, characterVectorModel, models.EmbedDim,
			characterVectorLang, centroidSource(group), vectorBlob(group.Centroid), now); err != nil {
			return fmt.Errorf("deepread: вектор персонажа %d: %w", group.ID, err)
		}
	}
	return nil
}

// execLiteraryVectors пишет строку vectors литературной базы. Карта писателей
// (T-01-07) не знает эту таблицу: она пришла общей миграцией shared/0002
// (T-14-02), а карта строится по tables.txt литературной базы, где vectors нет;
// Handle.Exec отклонил бы запись как чужую. Расширение карты — вне файлов
// задачи T-14-07, поэтому запись идёт запросом хэндла.
func execLiteraryVectors(lit *db.Handle, query string, args ...any) error {
	rows, err := lit.Query(query, args...)
	if err != nil {
		return err
	}
	return rows.Close()
}

// centroidSource описывает окна центроида по id чанков (W-25.1: вектор имени
// не строится).
func centroidSource(group CharacterGroup) string {
	ids := make([]int64, 0, len(group.Windows))
	for _, window := range group.Windows {
		if window.ChunkID > 0 {
			ids = append(ids, window.ChunkID)
		}
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })

	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return "центроид окон упоминаний: чанки " + strings.Join(parts, ",")
}

// WriteCharacterProfiles обновляет строки карты P11 по группам (шаг 5): алиасы
// слияния, диапазон чанков, число упоминаний и профиль эмоций. Существующие
// данные строки сохраняются: описание, внешность и прочие колонки не
// затираются, профиль пишется в пустое appearance, занятая внешность остаётся.
func WriteCharacterProfiles(lit *db.Handle, groups []CharacterGroup) error {
	if lit == nil {
		return ErrNoStore
	}
	for _, group := range groups {
		if group.ID <= 0 {
			return fmt.Errorf("deepread: профиль: персонаж без id карты")
		}

		aliases := []byte("[]")
		if len(group.Aliases) > 0 {
			raw, err := json.Marshal(group.Aliases)
			if err != nil {
				return fmt.Errorf("deepread: алиасы персонажа %d: %w", group.ID, err)
			}
			aliases = raw
		}

		var appearance string
		err := lit.QueryRow(`SELECT appearance FROM book_character WHERE id = ?`, group.ID).Scan(&appearance)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("deepread: профиль: персонаж %d не найден в карте", group.ID)
		}
		if err != nil {
			return err
		}

		next := appearance
		if strings.TrimSpace(appearance) == "" && len(group.Emotion) > 0 {
			raw, err := json.Marshal(group.Emotion)
			if err != nil {
				return fmt.Errorf("deepread: профиль эмоций персонажа %d: %w", group.ID, err)
			}
			next = string(raw)
		}

		if _, err := lit.Exec("book_character",
			`UPDATE book_character SET aliases = ?, first_chunk = ?, last_chunk = ?, mentions = ?, appearance = ? WHERE id = ?`,
			string(aliases), group.FirstChunk, group.LastChunk, group.Mentions, next, group.ID); err != nil {
			return fmt.Errorf("deepread: профиль персонажа %d: %w", group.ID, err)
		}
	}
	return nil
}
