package deepread

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"tns65/internal/db"
	"tns65/internal/models"
)

// Места и сцены книги (W-26, T-14-08): именные места словарей architecture и
// nature, границы сцен по смене места, времени или состава и профиль сцены
// (диапазон чанков, участники, доминирующие теги, эмоции, вектор-центроид).
// Сигналы границы считаются по леммам прогона: словари места, словарь времени
// пака и окна упоминаний карты P11.

const (
	// SceneVectorTable — owner_table центроидов сцен в vectors литературной
	// базы (W-26.3).
	SceneVectorTable = "book_scene"
	// castMinMentions — порог упоминаний персонажа для состава сцены (шаг 2).
	castMinMentions = 3
	// placeWindow — сколько токенов перед леммой-якорем осматривается на имя.
	placeWindow = 2
	// maxSceneTags — предел доминирующих тегов сцены (шаг 4).
	maxSceneTags = 3
	// dominantShare — доля максимума суммы скоров для доминирующего тега.
	dominantShare = 0.5
)

// placeKinds — словари, из которых берутся места (шаг 1).
var placeKinds = []string{KeyArchitecture, KeyNature}

// TimeLexicon — словарь времени пака detectors_time.json (W-22.2): единицы,
// числительные и предлоги. Маркер времени — «предлог (числительное) единица»;
// отдельной леммы времени без предлога словарь не задаёт.
type TimeLexicon struct {
	units        map[string]struct{}
	numerals     map[string]struct{}
	prepositions map[string]struct{}
}

// LoadTimeLexicon читает detectors_time.json из каталога пака. Пустой набор
// единиц — ошибка: тихий пустой словарь выключил бы сигнал времени.
func LoadTimeLexicon(packDir string) (*TimeLexicon, error) {
	raw, err := os.ReadFile(filepath.Join(packDir, "detectors_time.json"))
	if err != nil {
		return nil, err
	}
	var file struct {
		Units        map[string]string `json:"units"`
		Numerals     map[string]int64  `json:"numerals"`
		Prepositions []string          `json:"prepositions"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("deepread: detectors_time.json: %w", err)
	}

	lex := &TimeLexicon{
		units:        make(map[string]struct{}, len(file.Units)),
		numerals:     make(map[string]struct{}, len(file.Numerals)),
		prepositions: make(map[string]struct{}, len(file.Prepositions)),
	}
	for word := range file.Units {
		if lemma := strings.ToLower(strings.TrimSpace(word)); lemma != "" {
			lex.units[lemma] = struct{}{}
		}
	}
	if len(lex.units) == 0 {
		return nil, fmt.Errorf("deepread: detectors_time.json: нет единиц времени")
	}
	for word := range file.Numerals {
		if lemma := strings.ToLower(strings.TrimSpace(word)); lemma != "" {
			lex.numerals[lemma] = struct{}{}
		}
	}
	for _, word := range file.Prepositions {
		if lemma := strings.ToLower(strings.TrimSpace(word)); lemma != "" {
			lex.prepositions[lemma] = struct{}{}
		}
	}
	return lex, nil
}

// Markers возвращает маркеры времени чанка в порядке появления: предлог,
// необязательное числительное и единица времени на одной строке текста.
func (t *TimeLexicon) Markers(chunk SalienceChunk) []string {
	if t == nil {
		return nil
	}
	var out []string
	lemmas := chunk.Lemmas
	for i := 0; i < len(lemmas); i++ {
		if _, ok := t.prepositions[lemmas[i].Lemma]; !ok {
			continue
		}
		forms := []string{lemmas[i].Form}
		j := i + 1
		if j >= len(lemmas) || lemmas[j].Line != lemmas[i].Line {
			continue
		}
		if _, ok := t.numerals[lemmas[j].Lemma]; ok {
			forms = append(forms, lemmas[j].Form)
			j++
			if j >= len(lemmas) || lemmas[j].Line != lemmas[i].Line {
				continue
			}
		}
		if _, ok := t.units[lemmas[j].Lemma]; !ok {
			continue
		}
		forms = append(forms, lemmas[j].Form)
		out = append(out, strings.Join(forms, " "))
		i = j
	}
	return out
}

// Place — место книги (W-26.4): именная группа при лемме-якоре словаря
// architecture/nature, алиасы по повторам и границы упоминаний.
type Place struct {
	ID         int64
	Key        string
	Name       string
	Aliases    []string
	Kind       string
	FirstChunk int64
	LastChunk  int64
	Mentions   int64
}

// Scene — сцена книги (W-26.3): диапазон чанков, место, маркер времени,
// участники, доминирующие теги, профиль эмоций и вектор-центроид чанков.
type Scene struct {
	ID           int64
	Ord          int
	FromChunk    int64
	ToChunk      int64
	PlaceKey     string
	PlaceName    string
	PlaceID      int64
	TimeMarker   string
	Participants []string
	Tags         []string
	Emotion      map[string]float64
	Centroid     []float32
}

// DisputedBoundary — спорная граница сцен (шаг 3): смена места без смены
// времени и состава; до ответа слота граница ставится по месту, случай уходит
// в очередь T-14-10.
type DisputedBoundary struct {
	FromChunk int64
	ToChunk   int64
	Signals   []string
}

// SceneInput — вход разбора сцен: чанки в порядке ord, их теги, группы
// персонажей, словари места, времени и эмоций.
type SceneInput struct {
	BookID     int64
	Chunks     []SalienceChunk
	Tags       []ChunkTags
	Characters []CharacterGroup
	Places     map[string]*Dictionary
	Emotion    *EmotionDictionary
	Time       *TimeLexicon
}

// BookScenes — результат разбора сцен книги: места, сцены и спорные границы.
type BookScenes struct {
	BookID   int64
	Places   []Place
	Scenes   []Scene
	Disputed []DisputedBoundary
}

// placeMention — именное упоминание места в чанке.
type placeMention struct {
	key  string
	name string
	kind string
}

// BuildScenes проходит шаги 1–5 без записи в базу: места по именным группам,
// границы по смене места/времени/состава, склейка смежных чанков с общим
// профилем (место+время+состав) в одну сцену. Чанки обязаны идти в порядке
// book_chunk.ord: первая итерация режет внутри главы по чанкам. Векторы
// центроидов берутся из оперативной базы (ковка чанков, T-14-07).
func BuildScenes(lit, world *db.Handle, in SceneInput) (*BookScenes, error) {
	if lit == nil || world == nil {
		return nil, ErrNoStore
	}
	if in.BookID <= 0 {
		return nil, fmt.Errorf("deepread: сцены: book_id %d", in.BookID)
	}
	if in.Time == nil {
		return nil, fmt.Errorf("deepread: сцены: нет словаря времени detectors_time.json")
	}
	for _, key := range placeKinds {
		if in.Places[key] == nil {
			return nil, fmt.Errorf("deepread: сцены: нет словаря места %q", key)
		}
	}

	seen := make(map[int64]struct{}, len(in.Chunks))
	ids := make([]int64, 0, len(in.Chunks))
	for _, chunk := range in.Chunks {
		if chunk.ID <= 0 {
			return nil, fmt.Errorf("deepread: сцены: чанк без id")
		}
		if _, dup := seen[chunk.ID]; dup {
			return nil, fmt.Errorf("deepread: сцены: чанк %d повторяется", chunk.ID)
		}
		seen[chunk.ID] = struct{}{}
		ids = append(ids, chunk.ID)
	}

	set := &BookScenes{BookID: in.BookID}
	if len(in.Chunks) == 0 {
		return set, nil
	}

	vectors, err := chunkVectors(lit, world, in.BookID, ids)
	if err != nil {
		return nil, err
	}

	mentions := make([][]placeMention, len(in.Chunks))
	placeKeys := make([]string, len(in.Chunks))
	for i, chunk := range in.Chunks {
		mentions[i] = placeMentions(chunk, in.Places)
		placeKeys[i] = dominantKey(countPlaceMentions(mentions[i]))
	}
	set.Places = buildPlaces(in.Chunks, mentions)
	placeNames := make(map[string]string, len(set.Places))
	for _, place := range set.Places {
		placeNames[place.Key] = place.Name
	}

	markers := make([]string, len(in.Chunks))
	for i, chunk := range in.Chunks {
		if found := in.Time.Markers(chunk); len(found) > 0 {
			markers[i] = found[len(found)-1]
		}
	}
	tags := tagsByChunk(in.Tags)
	cast := castByChunk(in.Characters)
	names := characterNames(in.Characters)

	var (
		scenes   []Scene
		disputed []DisputedBoundary
		acc      = newSceneAcc()
		carried  string
		start    = 0
	)
	for i := range in.Chunks {
		if i > 0 {
			from, to := in.Chunks[i-1].ID, in.Chunks[i].ID
			placeChanged := placeKeys[i] != "" && placeKeys[i-1] != "" && placeKeys[i] != placeKeys[i-1]
			timeChanged := markers[i] != "" && markers[i] != carried
			castChanged := castShift(cast[from], cast[to])
			if placeChanged || timeChanged || castChanged {
				if placeChanged && !timeChanged && !castChanged {
					disputed = append(disputed, DisputedBoundary{FromChunk: from, ToChunk: to, Signals: []string{"place"}})
				}
				scenes = append(scenes, acc.scene(len(scenes), in.Chunks[start].ID, from, carried, placeNames, names, in.Emotion))
				acc = newSceneAcc()
				start = i
			}
		}
		acc.add(in.Chunks[i], mentions[i], tags[in.Chunks[i].ID], cast[in.Chunks[i].ID], vectors[in.Chunks[i].ID], in.Emotion)
		if markers[i] != "" {
			carried = markers[i]
		}
	}
	scenes = append(scenes, acc.scene(len(scenes), in.Chunks[start].ID, in.Chunks[len(in.Chunks)-1].ID, carried, placeNames, names, in.Emotion))

	set.Scenes = scenes
	set.Disputed = disputed
	return set, nil
}

// placeMentions находит именные упоминания мест чанка: лемма-якорь словаря
// architecture/nature с согласованной именной группой перед ней. Якорь без
// имени места не создаёт (W-26.4).
func placeMentions(chunk SalienceChunk, dicts map[string]*Dictionary) []placeMention {
	var out []placeMention
	for i, token := range chunk.Lemmas {
		for _, key := range placeKinds {
			dict := dicts[key]
			if dict == nil {
				continue
			}
			if _, ok := dict.Lemmas[token.Lemma]; !ok {
				continue
			}
			mods := placeModifiers(chunk.Lemmas, i)
			if len(mods) == 0 {
				continue
			}
			forms := make([]string, 0, len(mods)+1)
			lemmas := make([]string, 0, len(mods)+1)
			for _, mod := range mods {
				forms = append(forms, mod.Form)
				lemmas = append(lemmas, mod.Lemma)
			}
			forms = append(forms, token.Form)
			lemmas = append(lemmas, token.Lemma)
			out = append(out, placeMention{
				key:  strings.ToLower(strings.Join(lemmas, " ")),
				name: strings.Join(forms, " "),
				kind: key,
			})
			break
		}
	}
	return out
}

// placeModifiers собирает идущие подряд перед якорем определения: имя
// собственное или прилагательное. Согласование по граммемам прогон не отдаёт,
// поэтому опора — часть речи и заглавная буква не в начале строки.
func placeModifiers(lemmas []Lemma, anchor int) []Lemma {
	var mods []Lemma
	for j := anchor - 1; j >= 0 && len(mods) < placeWindow; j-- {
		if lemmas[j].Line != lemmas[anchor].Line || !nameLike(lemmas[j]) {
			break
		}
		mods = append([]Lemma{lemmas[j]}, mods...)
	}
	return mods
}

// nameLike — токен может быть определением при якоре места.
func nameLike(token Lemma) bool {
	switch token.POS {
	case "ADJ", "COMP":
		return true
	}
	runes := []rune(token.Form)
	return len(runes) > 0 && unicode.IsUpper(runes[0]) && token.Index > 0
}

// countPlaceMentions считает упоминания мест чанка по ключу.
func countPlaceMentions(mentions []placeMention) map[string]int {
	counts := make(map[string]int, len(mentions))
	for _, m := range mentions {
		counts[m.key]++
	}
	return counts
}

// dominantKey выбирает место с наибольшим числом упоминаний; при равенстве —
// первое по порядку ключей (детерминированный обход отсортированных ключей).
func dominantKey(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	best, bestN := "", 0
	for _, key := range keys {
		if counts[key] > bestN {
			best, bestN = key, counts[key]
		}
	}
	return best
}

// buildPlaces сводит упоминания книги в места: ключ (леммы имени) — одно
// место, повторные поверхности имени становятся алиасами (шаг 1).
func buildPlaces(chunks []SalienceChunk, mentions [][]placeMention) []Place {
	var (
		out    []Place
		index  = make(map[string]int)
		aliasm = make(map[string]map[string]struct{})
	)
	for i, chunk := range chunks {
		for _, m := range mentions[i] {
			pos, ok := index[m.key]
			if !ok {
				out = append(out, Place{
					Key:        m.key,
					Name:       m.name,
					Kind:       m.kind,
					FirstChunk: chunk.ID,
					LastChunk:  chunk.ID,
				})
				pos = len(out) - 1
				index[m.key] = pos
				aliasm[m.key] = map[string]struct{}{m.name: {}}
			}
			place := &out[pos]
			if chunk.ID < place.FirstChunk {
				place.FirstChunk = chunk.ID
			}
			if chunk.ID > place.LastChunk {
				place.LastChunk = chunk.ID
			}
			place.Mentions++
			if _, dup := aliasm[m.key][m.name]; dup {
				continue
			}
			aliasm[m.key][m.name] = struct{}{}
			if m.name != place.Name {
				place.Aliases = append(place.Aliases, m.name)
			}
		}
	}
	return out
}

// chunkCast — состав чанка по карте P11: все упоминания, состав (≥ порога) и
// присутствие персонажа.
type chunkCast struct {
	present  map[int64]struct{}
	cast     map[int64]struct{}
	mentions map[int64]int64
}

// castByChunk раскладывает окна групп персонажей по чанкам.
func castByChunk(groups []CharacterGroup) map[int64]*chunkCast {
	out := make(map[int64]*chunkCast)
	for _, group := range groups {
		for _, window := range group.Windows {
			if window.ChunkID <= 0 {
				continue
			}
			entry := out[window.ChunkID]
			if entry == nil {
				entry = &chunkCast{
					present:  make(map[int64]struct{}),
					cast:     make(map[int64]struct{}),
					mentions: make(map[int64]int64),
				}
				out[window.ChunkID] = entry
			}
			entry.present[group.ID] = struct{}{}
			entry.mentions[group.ID] += int64(windowMentions(window))
			if entry.mentions[group.ID] >= castMinMentions {
				entry.cast[group.ID] = struct{}{}
			}
		}
	}
	return out
}

// castShift — смена состава между чанками (шаг 2): появление персонажа с ≥ 3
// упоминаниями или исчезновение участника прежнего состава. Чанк без
// упоминаний сигналом не считается — сомнение трактуется консервативно.
func castShift(prev, cur *chunkCast) bool {
	if prev == nil || cur == nil || len(prev.present) == 0 || len(cur.present) == 0 {
		return false
	}
	for id := range cur.cast {
		if _, ok := prev.present[id]; !ok {
			return true
		}
	}
	for id := range prev.cast {
		if _, ok := cur.present[id]; !ok {
			return true
		}
	}
	return false
}

// characterNames — имена групп персонажей по id старшей записи карты.
func characterNames(groups []CharacterGroup) map[int64]string {
	out := make(map[int64]string, len(groups))
	for _, group := range groups {
		out[group.ID] = group.Name
	}
	return out
}

// tagsByChunk раскладывает теги чанков по id чанка.
func tagsByChunk(tags []ChunkTags) map[int64][]ChunkTag {
	out := make(map[int64][]ChunkTag, len(tags))
	for _, chunk := range tags {
		out[chunk.ChunkID] = chunk.Tags
	}
	return out
}

// sceneAcc — накопитель профиля сцены до закрытия диапазона чанков.
type sceneAcc struct {
	chunks       int
	placeCounts  map[string]int
	tagSums      map[string]float64
	charMentions map[int64]int64
	emotionHits  int
	vectors      [][]float32
}

// newSceneAcc заводит пустой накопитель сцены.
func newSceneAcc() *sceneAcc {
	return &sceneAcc{
		placeCounts:  make(map[string]int),
		tagSums:      make(map[string]float64),
		charMentions: make(map[int64]int64),
	}
}

// add включает чанк в сцену: места, теги, упоминания персонажей, попадание
// emotion и вектор чанка.
func (a *sceneAcc) add(chunk SalienceChunk, mentions []placeMention, tags []ChunkTag, cast *chunkCast, vector []float32, emotion *EmotionDictionary) {
	a.chunks++
	for _, m := range mentions {
		a.placeCounts[m.key]++
	}
	for _, tag := range tags {
		a.tagSums[tag.Key] += tag.Score
	}
	if cast != nil {
		for id, count := range cast.mentions {
			a.charMentions[id] += count
		}
	}
	if emotion.hits(chunk.Lemmas) {
		a.emotionHits++
	}
	if len(vector) > 0 {
		a.vectors = append(a.vectors, vector)
	}
}

// scene закрывает сцену: место — доминирующее по упоминаниям, маркер времени —
// последний якорь на момент конца сцены, участники — персонажи с ≥ 3
// упоминаниями в диапазоне, теги — доминирующие, эмоции — доля чанков с
// попаданием, вектор — центроид чанков.
func (a *sceneAcc) scene(ord int, from, to int64, marker string, placeNames map[string]string, names map[int64]string, emotion *EmotionDictionary) Scene {
	key := dominantKey(a.placeCounts)
	scene := Scene{
		Ord:          ord,
		FromChunk:    from,
		ToChunk:      to,
		PlaceKey:     key,
		PlaceName:    placeNames[key],
		TimeMarker:   marker,
		Participants: participantsOf(a.charMentions, names),
		Tags:         dominantTags(a.tagSums),
		Emotion:      make(map[string]float64),
		Centroid:     centroidVectors(a.vectors),
	}
	if emotion != nil && a.chunks > 0 {
		scene.Emotion[emotion.Mood] = float64(a.emotionHits) / float64(a.chunks)
	}
	return scene
}

// participantsOf отбирает участников сцены по числу упоминаний в её чанках.
func participantsOf(mentions map[int64]int64, names map[int64]string) []string {
	type participant struct {
		name     string
		mentions int64
	}
	var list []participant
	for id, count := range mentions {
		if count < castMinMentions {
			continue
		}
		list = append(list, participant{name: names[id], mentions: count})
	}
	sort.Slice(list, func(a, b int) bool {
		if list[a].mentions != list[b].mentions {
			return list[a].mentions > list[b].mentions
		}
		return list[a].name < list[b].name
	})
	out := make([]string, len(list))
	for i, p := range list {
		out[i] = p.name
	}
	return out
}

// dominantTags выбирает доминирующие теги сцены: ключи с суммой скоров не ниже
// половины максимума, не больше трёх, по убыванию суммы.
func dominantTags(sums map[string]float64) []string {
	max := 0.0
	for _, sum := range sums {
		if sum > max {
			max = sum
		}
	}
	if max <= 0 {
		return nil
	}
	keys := make([]string, 0, len(sums))
	for key, sum := range sums {
		if sum >= max*dominantShare {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(a, b int) bool {
		if sums[keys[a]] != sums[keys[b]] {
			return sums[keys[a]] > sums[keys[b]]
		}
		return keys[a] < keys[b]
	})
	if len(keys) > maxSceneTags {
		keys = keys[:maxSceneTags]
	}
	return keys
}

// centroidVectors — среднее векторов чанков; векторы другой размерности в
// среднее не входят.
func centroidVectors(vectors [][]float32) []float32 {
	var dim int
	for _, vector := range vectors {
		if len(vector) > 0 {
			dim = len(vector)
			break
		}
	}
	if dim == 0 {
		return nil
	}
	sum := make([]float64, dim)
	n := 0
	for _, vector := range vectors {
		if len(vector) != dim {
			continue
		}
		for i, value := range vector {
			sum[i] += float64(value)
		}
		n++
	}
	if n == 0 {
		return nil
	}
	out := make([]float32, dim)
	for i, value := range sum {
		out[i] = float32(value / float64(n))
	}
	return out
}

// WriteScenes пишет места и сцены книги в литературную базу (шаг 5):
// book_place и book_scene заменяются целиком, повторы строк не плодятся,
// центроиды сцен идут в vectors с owner_table='book_scene'.
func WriteScenes(lit *db.Handle, set *BookScenes, now string) error {
	if lit == nil {
		return ErrNoStore
	}
	if set == nil || set.BookID <= 0 {
		return fmt.Errorf("deepread: запись сцен: нет книги")
	}
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	if err := validateScenes(set); err != nil {
		return err
	}

	oldIDs, err := sceneIDs(lit, set.BookID)
	if err != nil {
		return err
	}

	placeIDs := make(map[string]int64, len(set.Places))
	err = lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		if _, err := tx.Exec("book_scene",
			`DELETE FROM book_scene WHERE book_id = ?`, set.BookID); err != nil {
			return fmt.Errorf("deepread: сцены книги %d: %w", set.BookID, err)
		}
		if _, err := tx.Exec("book_place",
			`DELETE FROM book_place WHERE book_id = ?`, set.BookID); err != nil {
			return fmt.Errorf("deepread: места книги %d: %w", set.BookID, err)
		}

		for i := range set.Places {
			place := &set.Places[i]
			aliases, err := jsonList(place.Aliases)
			if err != nil {
				return fmt.Errorf("deepread: алиасы места %q: %w", place.Name, err)
			}
			res, err := tx.Exec("book_place",
				`INSERT INTO book_place (book_id, name, aliases, kind, first_chunk, last_chunk, mentions) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				set.BookID, place.Name, aliases, place.Kind, place.FirstChunk, place.LastChunk, place.Mentions)
			if err != nil {
				return fmt.Errorf("deepread: место %q: %w", place.Name, err)
			}
			id, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("deepread: место %q: %w", place.Name, err)
			}
			place.ID = id
			placeIDs[place.Key] = id
		}

		for i := range set.Scenes {
			scene := &set.Scenes[i]
			var placeID any
			if scene.PlaceKey != "" {
				id, ok := placeIDs[scene.PlaceKey]
				if !ok {
					return fmt.Errorf("deepread: сцена %d: место %q не найдено", scene.Ord, scene.PlaceKey)
				}
				placeID = id
				scene.PlaceID = id
			}
			participants, err := jsonList(scene.Participants)
			if err != nil {
				return fmt.Errorf("deepread: сцена %d: участники: %w", scene.Ord, err)
			}
			tags, err := jsonList(scene.Tags)
			if err != nil {
				return fmt.Errorf("deepread: сцена %d: теги: %w", scene.Ord, err)
			}
			emotion := scene.Emotion
			if emotion == nil {
				emotion = map[string]float64{}
			}
			rawEmotion, err := json.Marshal(emotion)
			if err != nil {
				return fmt.Errorf("deepread: сцена %d: эмоции: %w", scene.Ord, err)
			}
			res, err := tx.Exec("book_scene",
				`INSERT INTO book_scene (book_id, ord, from_chunk, to_chunk, place_id, time_marker, participants, tags, emotion) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				set.BookID, scene.Ord, scene.FromChunk, scene.ToChunk, placeID,
				scene.TimeMarker, participants, tags, string(rawEmotion))
			if err != nil {
				return fmt.Errorf("deepread: сцена %d: %w", scene.Ord, err)
			}
			id, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("deepread: сцена %d: %w", scene.Ord, err)
			}
			scene.ID = id
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, owner := range oldIDs {
		if err := execLiteraryVectors(lit,
			`DELETE FROM vectors WHERE owner_table = ? AND owner_id = ?`,
			SceneVectorTable, owner); err != nil {
			return fmt.Errorf("deepread: вектор сцены %d: %w", owner, err)
		}
	}
	for _, scene := range set.Scenes {
		if len(scene.Centroid) == 0 {
			continue
		}
		if len(scene.Centroid) != models.EmbedDim {
			return fmt.Errorf("deepread: центроид сцены %d: dim %d, want %d",
				scene.Ord, len(scene.Centroid), models.EmbedDim)
		}
		if err := execLiteraryVectors(lit,
			`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			SceneVectorTable, scene.ID, characterVectorModel, models.EmbedDim,
			characterVectorLang, sceneSource(scene), vectorBlob(scene.Centroid), now); err != nil {
			return fmt.Errorf("deepread: вектор сцены %d: %w", scene.Ord, err)
		}
	}
	return nil
}

// validateScenes проверяет набор перед записью: ключи мест уникальны, сцены
// идут по ord без пересечения диапазонов и ссылаются на существующее место.
func validateScenes(set *BookScenes) error {
	keys := make(map[string]struct{}, len(set.Places))
	for _, place := range set.Places {
		if place.Key == "" || strings.TrimSpace(place.Name) == "" {
			return fmt.Errorf("deepread: место без ключа или имени")
		}
		if _, dup := keys[place.Key]; dup {
			return fmt.Errorf("deepread: место %q повторяется", place.Key)
		}
		keys[place.Key] = struct{}{}
		if place.FirstChunk <= 0 || place.LastChunk < place.FirstChunk {
			return fmt.Errorf("deepread: место %q: диапазон чанков %d..%d",
				place.Name, place.FirstChunk, place.LastChunk)
		}
	}
	for i, scene := range set.Scenes {
		if scene.Ord != i {
			return fmt.Errorf("deepread: сцена %d: ord %d", i, scene.Ord)
		}
		if scene.FromChunk <= 0 || scene.ToChunk < scene.FromChunk {
			return fmt.Errorf("deepread: сцена %d: диапазон чанков %d..%d",
				i, scene.FromChunk, scene.ToChunk)
		}
		if i > 0 && scene.FromChunk <= set.Scenes[i-1].ToChunk {
			return fmt.Errorf("deepread: сцены %d и %d пересекаются по чанкам", i-1, i)
		}
		if scene.PlaceKey != "" {
			if _, ok := keys[scene.PlaceKey]; !ok {
				return fmt.Errorf("deepread: сцена %d: место %q не найдено", i, scene.PlaceKey)
			}
		}
	}
	return nil
}

// sceneIDs возвращает текущие id сцен книги: их векторы снимаются после
// успешной записи новых строк.
func sceneIDs(lit *db.Handle, bookID int64) ([]int64, error) {
	rows, err := lit.Query(`SELECT id FROM book_scene WHERE book_id = ? ORDER BY id`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

// jsonList кодирует список строк JSON-массивом; пустой список — «[]», как в
// дефолтах колонок DDL.
func jsonList(items []string) (string, error) {
	if len(items) == 0 {
		return "[]", nil
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// sceneSource описывает диапазон чанков центроида сцены.
func sceneSource(scene Scene) string {
	return fmt.Sprintf("центроид сцены: чанки %d..%d", scene.FromChunk, scene.ToChunk)
}
