package deepread

// run.go — оркестратор разбора книги (T-14-13): лемма-слой → спорные смыслы
// слоту → salience → персонажи → сцены → стиль → staging → спорные слоту.
// Порядок фиксирован, каждый слой идемпотентен и продолжает прогон по наличию
// своих строк; итог прогона пишется сводкой в forge_log kind='deepread'.

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"tns65/internal/db"
	"tns65/internal/journal"
	"tns65/internal/models"
	"tns65/internal/prompts"
	"tns65/internal/version"
)

// Рейтинги, которые предлагает сводка разбора (W-22.7): пороги
// deepread.rating_16/rating_18 в config.yaml.
const (
	Rating16 = "16+"
	Rating18 = "18+"
)

// defaultRating16 / defaultRating18 — пороги предложения рейтинга, совпадающие
// со значениями config.yaml.
const (
	defaultRating16 = 0.05
	defaultRating18 = 0.15
)

// RunChunk — чанк книги для разбора: строка карты book_chunk (ID) либо текст с
// номером ord. Без ID Run заводит строку book_chunk по (book_id, ord).
type RunChunk struct {
	ID      int64
	Ord     int
	Chapter string
	Text    string
}

// RunInput — вход прогона разбора книги: книга реестра, чанки, каталог пака,
// файл конфигурации, слот спорных вопросов и журнал. Пустые PackDir и
// ConfigPath разрешаются от корня движка. Embedder (T-20-03) подключает к
// смысловому слою векторный счёт смыслов; nil — только триггеры.
type RunInput struct {
	BookID     int64
	Author     string
	Chunks     []RunChunk
	PackDir    string
	ConfigPath string
	Slot       DisputeSlot
	Logger     *journal.Logger
	Now        string
	Embedder   models.Embedder
}

// Summary — сводка прогона разбора (шаг 4 T-14-13): число чанков, полосы тегов
// по категориям, сцены, персонажи, вызовы слота, кандидаты staging, плотность
// erotic/violence и предложенный рейтинг. Читается вкладкой MCP из forge_log.
type Summary struct {
	Chunks          int                       `json:"chunks"`
	Bands           map[string]map[string]int `json:"bands"`
	Scenes          int                       `json:"scenes"`
	Characters      int                       `json:"characters"`
	SlotCalls       int                       `json:"slot_calls"`
	Staging         int                       `json:"staging_candidates"`
	EroticDensity   float64                   `json:"erotic_density"`
	ViolenceDensity float64                   `json:"violence_density"`
	SuggestedRating string                    `json:"suggested_rating"`
}

// RunReport — исход прогона: пропуск по неизменному ключу либо сводка.
type RunReport struct {
	Skipped bool
	Reason  string
	Chunks  int
	Summary Summary
}

// Payload — заказ задачи роя deepread: книга реестра, файл-вход ковки и слаг
// мира. BookID ≤ 0 — книга ищется по имени заголовка входа.
type Payload struct {
	BookID    int64  `json:"book_id"`
	InputPath string `json:"input_path"`
	WorldSlug string `json:"world_slug"`
}

// RatingConfig — пороги предложения рейтинга из deepread.rating_16/rating_18
// (W-22.7): доля confident-чанков erotic/violence.
type RatingConfig struct {
	Rating16 float64
	Rating18 float64
}

// DefaultRatingConfig возвращает пороги, совпадающие со значениями config.yaml.
func DefaultRatingConfig() RatingConfig {
	return RatingConfig{Rating16: defaultRating16, Rating18: defaultRating18}
}

// LoadRatingConfig читает пороги рейтинга из файла конфигурации.
func LoadRatingConfig(path string) (RatingConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RatingConfig{}, err
	}
	return parseRatingSection(string(data))
}

// parseRatingSection разбирает ключи rating_16 и rating_18 секции deepread;
// прочие ключи секции не читаются.
func parseRatingSection(data string) (RatingConfig, error) {
	cfg := DefaultRatingConfig()

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
		if !inDeepread || indent != 2 {
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
		switch strings.TrimSpace(key) {
		case "rating_16":
			cfg.Rating16 = f
		case "rating_18":
			cfg.Rating18 = f
		}
	}
	if err := sc.Err(); err != nil {
		return RatingConfig{}, err
	}
	if err := validateRatingConfig(cfg); err != nil {
		return RatingConfig{}, err
	}
	return cfg, nil
}

// validateRatingConfig проверяет порядок порогов: 0 ≤ rating_16 ≤ rating_18 ≤ 1.
func validateRatingConfig(cfg RatingConfig) error {
	if math.IsNaN(cfg.Rating16) || math.IsNaN(cfg.Rating18) ||
		cfg.Rating16 < 0 || cfg.Rating18 > 1 || cfg.Rating16 > cfg.Rating18 {
		return fmt.Errorf("deepread: пороги рейтинга вне 0..1: rating_16=%g rating_18=%g",
			cfg.Rating16, cfg.Rating18)
	}
	return nil
}

// RunPipeline прогоняет разбор книги по ключу пересчёта (D-20.3): слои идут в
// порядке лемма → salience → персонажи → сцены → стиль → staging → спорные
// слоту; неизменный ключ прогон пропускает, завершённый — пишет сводку в
// forge_log. Имя Run занято лемма-слоем (lemmas.go), оркестратор — RunPipeline.
func RunPipeline(lit, world *db.Handle, in RunInput) (RunReport, error) {
	if lit == nil || world == nil {
		return RunReport{}, ErrNoStore
	}
	if in.BookID <= 0 {
		return RunReport{}, fmt.Errorf("deepread: разбор книги: book_id %d", in.BookID)
	}
	if len(in.Chunks) == 0 {
		return RunReport{}, fmt.Errorf("deepread: разбор книги %d: нет чанков", in.BookID)
	}

	packDir, configPath, err := resolveRunPaths(in.PackDir, in.ConfigPath)
	if err != nil {
		return RunReport{}, err
	}
	// Промпты пакета: правка packs/<lang>/prompts.json доезжает до запросов
	// разбора без пересборки; файла нет — работают дефолты Go.
	if err := prompts.Load(packDir); err != nil {
		return RunReport{}, err
	}
	chunks, err := resolveChunks(lit, in.BookID, in.Chunks)
	if err != nil {
		return RunReport{}, err
	}

	now := in.Now
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	key, err := NewKey(packDir, configPath, joinChunkTexts(chunks))
	if err != nil {
		return RunReport{}, err
	}

	var summary Summary
	outcome, err := RunOnce(lit, key, func() (int, error) {
		s, err := runLayers(lit, world, in, chunks, packDir, configPath, key, now)
		if err != nil {
			return 0, err
		}
		summary = s
		if err := writeSummary(lit, s, now); err != nil {
			return 0, err
		}
		return len(chunks), nil
	})
	if err != nil {
		return RunReport{}, err
	}
	return RunReport{
		Skipped: outcome.Skipped,
		Reason:  outcome.Reason,
		Chunks:  outcome.Chunks,
		Summary: summary,
	}, nil
}

// runLayers проходит слои разбора по порядку и возвращает сводку по строкам
// литературной базы. Слой, чьи строки уже на месте, повторно не пишется —
// прогон после обрыва продолжается с первого недописанного слоя.
func runLayers(lit, world *db.Handle, in RunInput, chunks []runChunk, packDir, configPath string, key Key, now string) (Summary, error) {
	dicts, err := LoadDictionaries(packDir)
	if err != nil {
		return Summary{}, err
	}
	mood, err := LoadEmotionDictionary(filepath.Join(packDir, "detectors_mood.json"))
	if err != nil {
		return Summary{}, err
	}
	timeLex, err := LoadTimeLexicon(packDir)
	if err != nil {
		return Summary{}, err
	}
	styles, err := LoadStyles(packDir)
	if err != nil {
		return Summary{}, err
	}
	salCfg, err := LoadSalienceConfig(configPath)
	if err != nil {
		return Summary{}, err
	}
	charCfg, err := LoadCharacterConfig(configPath)
	if err != nil {
		return Summary{}, err
	}
	stagingCfg, err := LoadStagingConfig(configPath)
	if err != nil {
		return Summary{}, err
	}
	disputeCfg, err := LoadDisputeConfig(configPath)
	if err != nil {
		return Summary{}, err
	}
	ratingCfg, err := LoadRatingConfig(configPath)
	if err != nil {
		return Summary{}, err
	}
	senseLayer, err := LoadSenseLayer(packDir, salCfg, in.Embedder)
	if err != nil {
		return Summary{}, err
	}
	senseCatalog, err := LoadSenseCatalog(packDir)
	if err != nil {
		return Summary{}, err
	}

	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = chunk.Text
	}
	lemmaRun, err := OpenRun(packDir)
	if err != nil {
		return Summary{}, err
	}
	chunkLemmas := lemmaRun.ChunkLemmas(texts)
	if err := lemmaRun.Err(); err != nil {
		_ = lemmaRun.Close()
		return Summary{}, fmt.Errorf("deepread: лемма-слой: %w", err)
	}
	if err := lemmaRun.Close(); err != nil {
		return Summary{}, fmt.Errorf("deepread: лемма-слой: %w", err)
	}

	salChunks := make([]SalienceChunk, len(chunks))
	styleChunks := make([]StyleChunk, len(chunks))
	var allLemmas []Lemma
	for i, chunk := range chunks {
		salChunks[i] = SalienceChunk{ID: chunk.ID, Text: chunk.Text, Lemmas: chunkLemmas[i]}
		styleChunks[i] = StyleChunk{ID: chunk.ID, Text: chunk.Text}
		allLemmas = append(allLemmas, chunkLemmas[i]...)
	}

	counter := &callCounter{inner: in.Slot}
	var slot DisputeSlot
	if in.Slot != nil {
		slot = counter
	}
	senseQuestions, err := BuildSenseDisputes(SenseDisputeInput{
		Chunks:  salChunks,
		Layer:   senseLayer,
		Catalog: senseCatalog,
	})
	if err != nil {
		return Summary{}, err
	}
	senseOutcomes, err := AskSenseDisputes(lit, key, senseQuestions, disputeCfg, slot, in.Logger, now)
	if err != nil {
		return Summary{}, err
	}

	tags, err := TagChunksWithSenses(salChunks, SenseDictionaries(dicts, senseOutcomes), senseLayer, salCfg)
	if err != nil {
		return Summary{}, err
	}
	tagsDone, err := chunkTagsComplete(lit, in.BookID, len(salChunks)*len(dicts))
	if err != nil {
		return Summary{}, err
	}
	if !tagsDone {
		if err := WriteChunkTags(lit, tags); err != nil {
			return Summary{}, err
		}
	}

	groups, disputedAliases, err := BuildCharacters(lit, world, CharacterInput{
		BookID:  in.BookID,
		Chunks:  salChunks,
		Emotion: mood,
		Config:  charCfg,
	})
	if err != nil {
		return Summary{}, err
	}
	charsDone, err := characterCentroidsComplete(lit, groups)
	if err != nil {
		return Summary{}, err
	}
	if !charsDone {
		if err := WriteCharacterCentroids(lit, groups, now); err != nil {
			return Summary{}, err
		}
		if err := WriteCharacterProfiles(lit, groups); err != nil {
			return Summary{}, err
		}
	}

	scenes, err := BuildScenes(lit, world, SceneInput{
		BookID:     in.BookID,
		Chunks:     salChunks,
		Tags:       tags,
		Characters: groups,
		Places:     dicts,
		Emotion:    mood,
		Time:       timeLex,
	})
	if err != nil {
		return Summary{}, err
	}
	scenesDone, err := scenesComplete(lit, in.BookID, len(scenes.Scenes))
	if err != nil {
		return Summary{}, err
	}
	if !scenesDone {
		if err := WriteScenes(lit, scenes, now); err != nil {
			return Summary{}, err
		}
	}

	salienceByChunk := salienceScores(tags)
	for i := range styleChunks {
		styleChunks[i].Salience = salienceByChunk[styleChunks[i].ID]
	}
	style, err := BuildBookStyle(StyleInput{
		Author: in.Author,
		Text:   joinChunkTexts(chunks),
		Lemmas: allLemmas,
		Chunks: styleChunks,
		Styles: styles,
	})
	if err != nil {
		return Summary{}, err
	}
	styleDone, err := styleComplete(lit, in.BookID)
	if err != nil {
		return Summary{}, err
	}
	if !styleDone {
		if err := WriteBookStyle(lit, in.BookID, style, now); err != nil {
			return Summary{}, err
		}
	}

	if _, err := StageDictionaries(lit, world, StagingInput{
		BookID: in.BookID,
		Chunks: salChunks,
		Tags:   tags,
		Dicts:  dicts,
		Config: stagingCfg,
	}); err != nil {
		return Summary{}, err
	}

	windows := make(map[int64]string, len(salChunks))
	for _, chunk := range salChunks {
		windows[chunk.ID] = chunk.Text
	}
	questions, err := BuildDisputes(DisputeInput{
		BookID:     in.BookID,
		Tags:       tags,
		Merges:     disputedAliases,
		Boundaries: scenes.Disputed,
		Windows:    windows,
		TagKeys:    DictionaryKeys,
	})
	if err != nil {
		return Summary{}, err
	}

	outcomes, err := ResolveDisputes(questions, RemainingDisputeConfig(disputeCfg, len(senseQuestions)), slot, in.Logger)
	if err != nil {
		return Summary{}, err
	}

	for _, out := range outcomes {
		if err := ApplyDecision(lit, out); err != nil {
			return Summary{}, err
		}
	}
	updated := applyCategoryDecisions(tags, outcomes)
	if len(updated) > 0 {
		if err := WriteChunkTags(lit, updated); err != nil {
			return Summary{}, err
		}
	}
	boundaryChanged := hasBoundaryJoin(outcomes)
	if err := ApplyBoundaries(scenes, outcomes); err != nil {
		return Summary{}, err
	}
	if boundaryChanged {
		if err := WriteScenes(lit, scenes, now); err != nil {
			return Summary{}, err
		}
	}

	return summarize(lit, in.BookID, counter.calls, ratingCfg)
}

// callCounter считает вызовы слота спорных вопросов для сводки.
type callCounter struct {
	inner DisputeSlot
	calls int
}

// Generate выполняет запрос слота и считает вызов.
func (c *callCounter) Generate(ctx context.Context, role, prompt string) (string, error) {
	c.calls++
	return c.inner.Generate(ctx, role, prompt)
}

// runChunk — разрешённый чанк прогона: строка карты и текст.
type runChunk struct {
	ID      int64
	Ord     int
	Chapter string
	Text    string
}

// resolveChunks ставит чанки на строки book_chunk по (book_id, ord): готовый ID
// берётся как есть, иначе строка ищется или заводится с хешем текста. Порядок
// чанков — по ord: сцены режутся в порядке карты.
func resolveChunks(lit *db.Handle, bookID int64, chunks []RunChunk) ([]runChunk, error) {
	out := make([]runChunk, len(chunks))
	for i, chunk := range chunks {
		if chunk.Text == "" {
			return nil, fmt.Errorf("deepread: разбор книги %d: чанк %d без текста", bookID, i)
		}
		if chunk.Ord < 0 {
			return nil, fmt.Errorf("deepread: разбор книги %d: ord %d", bookID, chunk.Ord)
		}
		out[i] = runChunk{ID: chunk.ID, Ord: chunk.Ord, Chapter: chunk.Chapter, Text: chunk.Text}
		if out[i].ID > 0 {
			continue
		}
		id, err := chunkRowID(lit, bookID, chunk.Ord, chunk.Text, chunk.Chapter)
		if err != nil {
			return nil, err
		}
		out[i].ID = id
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Ord < out[b].Ord })
	return out, nil
}

// chunkRowID находит строку book_chunk по (book_id, ord) или заводит её; хеш
// текста хранится в строке, сам текст в базе не дублируется.
func chunkRowID(lit *db.Handle, bookID int64, ord int, text, chapter string) (int64, error) {
	var id int64
	err := lit.QueryRow(
		`SELECT id FROM book_chunk WHERE book_id = ? AND ord = ?`, bookID, ord).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("deepread: строка чанка %d/%d: %w", bookID, ord, err)
	}
	res, err := lit.Exec("book_chunk",
		`INSERT INTO book_chunk (book_id, ord, chapter, text_hash) VALUES (?, ?, ?, ?)`,
		bookID, ord, chapter, TextHash(text))
	if err != nil {
		return 0, fmt.Errorf("deepread: строка чанка %d/%d: %w", bookID, ord, err)
	}
	id, err = res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("deepread: строка чанка %d/%d: %w", bookID, ord, err)
	}
	return id, nil
}

// joinChunkTexts склеивает текст книги из чанков: ключ пересчёта и стилевой
// профиль считаются по всей книге.
func joinChunkTexts(chunks []runChunk) string {
	parts := make([]string, len(chunks))
	for i, chunk := range chunks {
		parts[i] = chunk.Text
	}
	return strings.Join(parts, "\n\n")
}

// resolveRunPaths разрешает каталог пака и файл конфигурации: пустые значения
// считаются от корня движка (каталог с go.mod, для развёрнутой поставки —
// рабочий каталог).
func resolveRunPaths(packDir, configPath string) (string, string, error) {
	if packDir == "" || configPath == "" {
		root := engineRoot()
		if packDir == "" {
			packDir = filepath.Join(root, "packs", "ru")
		}
		if configPath == "" {
			configPath = filepath.Join(root, "config.yaml")
		}
	}
	if _, err := os.Stat(filepath.Join(packDir, "manifest.json")); err != nil {
		return "", "", fmt.Errorf("deepread: пак разбора %s: %w", packDir, err)
	}
	return packDir, configPath, nil
}

// engineRoot поднимается от рабочего каталога до каталога с go.mod; найденного
// нет — рабочий каталог (поставка без go.mod).
func engineRoot() string {
	start, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start
		}
		dir = parent
	}
}

// chunkTagsComplete — теги всех чанков книги уже разложены: одна строка на
// пару (чанк, ключ словаря).
func chunkTagsComplete(lit *db.Handle, bookID int64, want int) (bool, error) {
	n, err := countRows(lit,
		`SELECT COUNT(*) FROM book_chunk_tag t JOIN book_chunk c ON c.id = t.chunk_id WHERE c.book_id = ?`,
		bookID)
	if err != nil {
		return false, err
	}
	return want > 0 && n >= want, nil
}

// characterCentroidsComplete — по центроиду на каждую слитую запись, у которой
// центроид есть.
func characterCentroidsComplete(lit *db.Handle, groups []CharacterGroup) (bool, error) {
	for _, group := range groups {
		if len(group.Centroid) == 0 {
			continue
		}
		n, err := countRows(lit,
			`SELECT COUNT(*) FROM vectors WHERE owner_table = ? AND owner_id = ?`,
			CharacterVectorTable, group.ID)
		if err != nil {
			return false, err
		}
		if n == 0 {
			return false, nil
		}
	}
	return true, nil
}

// scenesComplete — строки book_scene книги совпадают по числу с построенным
// набором сцен.
func scenesComplete(lit *db.Handle, bookID int64, want int) (bool, error) {
	n, err := countRows(lit, `SELECT COUNT(*) FROM book_scene WHERE book_id = ?`, bookID)
	if err != nil {
		return false, err
	}
	return want > 0 && n == want, nil
}

// styleComplete — строка book_style книги уже есть.
func styleComplete(lit *db.Handle, bookID int64) (bool, error) {
	n, err := countRows(lit, `SELECT COUNT(*) FROM book_style WHERE book_id = ?`, bookID)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// countRows — число строк запроса.
func countRows(lit *db.Handle, query string, args ...any) (int, error) {
	var n int
	if err := lit.QueryRow(query, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// salienceScores раскладывает скоры тегов по чанкам для драматичности.
func salienceScores(tags []ChunkTags) map[int64]map[string]float64 {
	out := make(map[int64]map[string]float64, len(tags))
	for _, chunkTags := range tags {
		scores := make(map[string]float64, len(chunkTags.Tags))
		for _, tag := range chunkTags.Tags {
			scores[tag.Key] = tag.Score
		}
		out[chunkTags.ChunkID] = scores
	}
	return out
}

// applyCategoryDecisions применяет решения слота о категории к тегам чанков:
// выбранный ключ получает полосу confident, цитата становится якорем. Возвращает
// теги затронутых чанков для частичной перезаписи.
func applyCategoryDecisions(tags []ChunkTags, outcomes []DisputeOutcome) []ChunkTags {
	byChunk := make(map[int64]int, len(tags))
	for i, chunkTags := range tags {
		byChunk[chunkTags.ChunkID] = i
	}
	touched := make(map[int64]struct{})
	for _, out := range outcomes {
		tag, ok := out.AppliedTag()
		if !ok {
			continue
		}
		idx, found := byChunk[out.Question.ChunkID]
		if !found {
			continue
		}
		replaced := false
		for i := range tags[idx].Tags {
			if tags[idx].Tags[i].Key != tag.Key {
				continue
			}
			tags[idx].Tags[i] = tag
			replaced = true
			break
		}
		if replaced {
			touched[out.Question.ChunkID] = struct{}{}
		}
	}
	if len(touched) == 0 {
		return nil
	}
	out := make([]ChunkTags, 0, len(touched))
	for _, chunkTags := range tags {
		if _, ok := touched[chunkTags.ChunkID]; ok {
			out = append(out, chunkTags)
		}
	}
	return out
}

// hasBoundaryJoin — среди решений есть склейка сцен: набор сцен после решения
// пишется заново.
func hasBoundaryJoin(outcomes []DisputeOutcome) bool {
	for _, out := range outcomes {
		if out.State == DisputeAnswered && out.Question.Kind == DisputeBoundary &&
			out.Value == AnswerBoundaryJoin {
			return true
		}
	}
	return false
}

// summarize собирает сводку по строкам литературной базы: счётчики берутся из
// таблиц, поэтому сводка сходится с их содержимым.
func summarize(lit *db.Handle, bookID int64, calls int, ratingCfg RatingConfig) (Summary, error) {
	s := Summary{
		Bands:     make(map[string]map[string]int, len(CategoryKeys)),
		SlotCalls: calls,
	}
	for _, key := range CategoryKeys {
		s.Bands[key] = map[string]int{BandConfident: 0, BandUncertain: 0, BandReject: 0}
	}

	var err error
	if s.Chunks, err = countRows(lit, `SELECT COUNT(*) FROM book_chunk WHERE book_id = ?`, bookID); err != nil {
		return Summary{}, fmt.Errorf("deepread: сводка: чанки: %w", err)
	}
	if s.Scenes, err = countRows(lit, `SELECT COUNT(*) FROM book_scene WHERE book_id = ?`, bookID); err != nil {
		return Summary{}, fmt.Errorf("deepread: сводка: сцены: %w", err)
	}
	if s.Characters, err = countRows(lit, `SELECT COUNT(*) FROM book_character WHERE book_id = ?`, bookID); err != nil {
		return Summary{}, fmt.Errorf("deepread: сводка: персонажи: %w", err)
	}
	if s.Staging, err = countRows(lit, `SELECT COUNT(*) FROM dict_staging WHERE book_id = ?`, bookID); err != nil {
		return Summary{}, fmt.Errorf("deepread: сводка: staging: %w", err)
	}

	rows, err := lit.Query(
		`SELECT t.key, t.band, COUNT(*) FROM book_chunk_tag t JOIN book_chunk c ON c.id = t.chunk_id WHERE c.book_id = ? GROUP BY t.key, t.band`,
		bookID)
	if err != nil {
		return Summary{}, fmt.Errorf("deepread: сводка: полосы: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, band string
		var n int
		if err := rows.Scan(&key, &band, &n); err != nil {
			return Summary{}, fmt.Errorf("deepread: сводка: полосы: %w", err)
		}
		counts := s.Bands[key]
		if counts == nil {
			counts = map[string]int{BandConfident: 0, BandUncertain: 0, BandReject: 0}
			s.Bands[key] = counts
		}
		counts[band] = n
	}
	if err := rows.Err(); err != nil {
		return Summary{}, fmt.Errorf("deepread: сводка: полосы: %w", err)
	}

	if s.Chunks > 0 {
		s.EroticDensity = float64(s.Bands[KeyErotic][BandConfident]) / float64(s.Chunks)
		s.ViolenceDensity = float64(s.Bands[KeyViolence][BandConfident]) / float64(s.Chunks)
	}
	s.SuggestedRating = ratingFor(s, ratingCfg)
	return s, nil
}

// ratingFor предлагает рейтинг по плотности erotic/violence (W-22.7): порог
// 18+ важнее 16+, ниже младшего порога рейтинг не предлагается.
func ratingFor(s Summary, cfg RatingConfig) string {
	density := math.Max(s.EroticDensity, s.ViolenceDensity)
	switch {
	case density >= cfg.Rating18:
		return Rating18
	case density >= cfg.Rating16:
		return Rating16
	}
	return ""
}

// writeSummary пишет сводку прогона в forge_log kind='deepread' (шаг 4): строка
// читается вкладкой MCP, отдельный маршрут не заводится.
func writeSummary(lit *db.Handle, s Summary, now string) error {
	if lit == nil {
		return ErrNoStore
	}
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	task, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("deepread: сводка разбора: %w", err)
	}
	return lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		_, err := tx.Exec("forge_log",
			`INSERT INTO forge_log (kind, task, status, started_at, finished_at, log_path, package_version) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			KindDeepread, string(task), statusDone, now, now, "", version.Version)
		return err
	})
}
