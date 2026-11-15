package deepread

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"tns65/internal/db"
	"tns65/internal/models"
)

// Staging кандидатов словарей (W-28, T-14-12): частые леммы чанков с высокой
// семантической привязкой к категории попадают в dict_staging литературной
// базы. Файлы пака разбор не пишет: промоушен выполняет только владелец правкой
// пака, статус строки код не меняет.

const (
	// DictAnchorTable — owner_table якорей категорий в vectors литературной
	// базы (шаг 1); owner_id — номер ключа в реестре CategoryKeys.
	DictAnchorTable = "dict_anchor"

	// Статусы строки dict_staging (W-28.1): код пишет только candidate,
	// promoted и rejected ставит владелец вручную (W-28.3).
	StagingStatusCandidate = "candidate"
	StagingStatusPromoted  = "promoted"
	StagingStatusRejected  = "rejected"

	// minCandidateFrequency — минимальная частота леммы в чанках-кандидатах
	// (шаг 2 T-14-12).
	minCandidateFrequency = 3

	// defaultStagingCosine — deepread.staging_cosine конфига по умолчанию.
	defaultStagingCosine = 0.8

	// anchorVectorModel — модель векторов разбора (T-14-02).
	anchorVectorModel = "deepread-1024"
	// anchorVectorLang — язык книги разбора (пак ru).
	anchorVectorLang = "ru"
)

// StagingConfig — порог косинуса кандидата в dict_staging из
// deepread.staging_cosine конфига (§3 плана волны 14).
type StagingConfig struct {
	Cosine float64
}

// DefaultStagingConfig возвращает порог, совпадающий со значением config.yaml.
func DefaultStagingConfig() StagingConfig {
	return StagingConfig{Cosine: defaultStagingCosine}
}

// LoadStagingConfig читает порог кандидатов из файла конфигурации.
func LoadStagingConfig(path string) (StagingConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return StagingConfig{}, err
	}
	return parseStagingSection(string(data))
}

// parseStagingSection разбирает ключ staging_cosine секции deepread; прочие
// ключи секции не читаются, отсутствие ключа оставляет дефолт.
func parseStagingSection(data string) (StagingConfig, error) {
	cfg := DefaultStagingConfig()

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
		if !ok || strings.TrimSpace(key) != "staging_cosine" {
			continue
		}
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return StagingConfig{}, fmt.Errorf("deepread: staging_cosine: %w", err)
		}
		cfg.Cosine = f
	}
	if err := sc.Err(); err != nil {
		return StagingConfig{}, err
	}
	if err := validateStagingConfig(cfg); err != nil {
		return StagingConfig{}, err
	}
	return cfg, nil
}

// validateStagingConfig проверяет порог: 0 ≤ staging_cosine ≤ 1.
func validateStagingConfig(cfg StagingConfig) error {
	if math.IsNaN(cfg.Cosine) || cfg.Cosine < 0 || cfg.Cosine > 1 {
		return fmt.Errorf("deepread: staging_cosine %g вне 0..1", cfg.Cosine)
	}
	return nil
}

// DictAnchor — якорь категории (шаг 1): центроид векторов confident-чанков,
// ключ категории и его номер в реестре CategoryKeys (owner_id в vectors).
type DictAnchor struct {
	Key      string
	OwnerID  int
	Centroid []float32
	Chunks   []int64
}

// StagingCandidate — кандидат словаря: пара (ключ, лемма), вес, основание-
// цитата и частота в чанках-кандидатах. В dict_staging пишется как candidate.
type StagingCandidate struct {
	DictKey   string
	Lemma     string
	Weight    int
	Evidence  string
	Frequency int
}

// StagingInput — вход staging: книга, чанки прогона с леммами, теги полос,
// словари категорий и порог косинуса.
type StagingInput struct {
	BookID int64
	Chunks []SalienceChunk
	Tags   []ChunkTags
	Dicts  map[string]*Dictionary
	Config StagingConfig
}

// StagingResult — итог staging: построенные якоря, кандидаты и счётчики строк
// dict_staging (новые и повторы без новой строки).
type StagingResult struct {
	Anchors    []DictAnchor
	Candidates []StagingCandidate
	Inserted   int
	Repeated   int
}

// BuildDictAnchors строит якоря категорий (шаг 1): центроид векторов чанков с
// полосой confident по ключу. Чанк без вектора в центроид не входит; ключ без
// confident-чанков якоря не получает. Порядок якорей — реестр CategoryKeys.
func BuildDictAnchors(chunks []SalienceChunk, vectors map[int64][]float32,
	tags []ChunkTags, dicts map[string]*Dictionary) []DictAnchor {
	confident := make(map[int64]map[string]struct{})
	for _, chunkTags := range tags {
		for _, tag := range chunkTags.Tags {
			if tag.Band != BandConfident {
				continue
			}
			if _, ok := dicts[tag.Key]; !ok {
				continue
			}
			keys := confident[chunkTags.ChunkID]
			if keys == nil {
				keys = make(map[string]struct{})
				confident[chunkTags.ChunkID] = keys
			}
			keys[tag.Key] = struct{}{}
		}
	}

	keys := make([]string, 0, len(dicts))
	for key, dict := range dicts {
		if dict == nil || CategoryID(key) < 0 {
			continue
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(a, b int) bool { return CategoryID(keys[a]) < CategoryID(keys[b]) })

	var out []DictAnchor
	for _, key := range keys {
		var vecs [][]float32
		var ids []int64
		for _, chunk := range chunks {
			if _, ok := confident[chunk.ID][key]; !ok {
				continue
			}
			vec, ok := vectors[chunk.ID]
			if !ok || len(vec) == 0 {
				continue
			}
			vecs = append(vecs, vec)
			ids = append(ids, chunk.ID)
		}
		centroid := centroidVectors(vecs)
		if centroid == nil {
			continue
		}
		out = append(out, DictAnchor{
			Key:      key,
			OwnerID:  CategoryID(key),
			Centroid: centroid,
			Chunks:   ids,
		})
	}
	return out
}

// BuildStagingCandidates строит кандидатов (шаг 2): лемма вне всех словарей с
// частотой ≥ 3 в чанках, чей косинус к якорю категории ≥ порога; имя
// собственное кандидатом не является. Кандидаты упорядочены по ключу реестра и
// лемме.
func BuildStagingCandidates(chunks []SalienceChunk, vectors map[int64][]float32,
	anchors []DictAnchor, dicts map[string]*Dictionary, cfg StagingConfig) []StagingCandidate {
	if cfg == (StagingConfig{}) {
		cfg = DefaultStagingConfig()
	}

	known := make(map[string]struct{})
	for _, dict := range dicts {
		if dict == nil {
			continue
		}
		for lemma := range dict.Lemmas {
			known[lemma] = struct{}{}
		}
	}

	var out []StagingCandidate
	for _, anchor := range anchors {
		type stat struct {
			count      int
			allProper  bool
			forms      []string
			formSeen   map[string]struct{}
			evidenceAt string
		}
		stats := make(map[string]*stat)
		for _, chunk := range chunks {
			vec, ok := vectors[chunk.ID]
			if !ok || len(vec) == 0 {
				continue
			}
			if Cosine(vec, anchor.Centroid) < cfg.Cosine {
				continue
			}
			for _, token := range chunk.Lemmas {
				lemma := strings.TrimSpace(token.Lemma)
				if lemma == "" {
					continue
				}
				s := stats[lemma]
				if s == nil {
					s = &stat{allProper: true, formSeen: make(map[string]struct{})}
					stats[lemma] = s
				}
				s.count++
				if s.evidenceAt == "" {
					s.evidenceAt = chunk.Text
				}
				form := strings.TrimSpace(token.Form)
				if form != "" {
					key := strings.ToLower(form)
					if _, dup := s.formSeen[key]; !dup {
						s.formSeen[key] = struct{}{}
						s.forms = append(s.forms, form)
					}
				}
				if !properNounToken(token) {
					s.allProper = false
				}
			}
		}

		lemmas := make([]string, 0, len(stats))
		for lemma := range stats {
			lemmas = append(lemmas, lemma)
		}
		sort.Strings(lemmas)
		for _, lemma := range lemmas {
			s := stats[lemma]
			if s.count < minCandidateFrequency {
				continue
			}
			if _, ok := known[lemma]; ok {
				continue
			}
			if s.allProper {
				continue
			}
			evidence := quoteSentence(s.evidenceAt, s.forms)
			if evidence == "" {
				continue
			}
			weight := s.count
			if weight > maxLemmaWeight {
				weight = maxLemmaWeight
			}
			out = append(out, StagingCandidate{
				DictKey:   anchor.Key,
				Lemma:     lemma,
				Weight:    weight,
				Evidence:  evidence,
				Frequency: s.count,
			})
		}
	}
	return out
}

// properNounToken — токен похож на имя собственное: форма с заглавной и часть
// речи не нарицательное (шаг 2); имя собственным кандидатом не является.
func properNounToken(token Lemma) bool {
	form := strings.TrimSpace(token.Form)
	if form == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(form)
	if !unicode.IsUpper(first) {
		return false
	}
	return !strings.EqualFold(strings.TrimSpace(token.POS), "NOUN")
}

// quoteSentence возвращает предложение чанка с первым вхождением формы леммы —
// основание кандидата (шаг 3). Пустая строка — формы в тексте не нашлись.
func quoteSentence(text string, forms []string) string {
	if strings.TrimSpace(text) == "" || len(forms) == 0 {
		return ""
	}
	want := make(map[string]struct{}, len(forms))
	for _, form := range forms {
		want[strings.ToLower(strings.TrimSpace(form))] = struct{}{}
	}

	runes := []rune(text)
	spans, _ := splitSentences(runes)
	for _, span := range spans {
		for _, word := range wordsOf(runes[span.start:span.end]) {
			if _, ok := want[word]; !ok {
				continue
			}
			quote := strings.TrimSpace(string(runes[span.start:span.end]))
			if quote != "" {
				return quote
			}
		}
	}
	return ""
}

// WriteDictAnchors пишет якоря категорий в vectors литературной базы (шаг 1):
// owner_table='dict_anchor', owner_id — номер ключа в реестре CategoryKeys.
// Прежний якорь ключа заменяется, повторный прогон строк не плодит.
func WriteDictAnchors(lit *db.Handle, anchors []DictAnchor, now string) error {
	if lit == nil {
		return ErrNoStore
	}
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	for _, anchor := range anchors {
		if anchor.OwnerID < 0 || CategoryID(anchor.Key) != anchor.OwnerID {
			return fmt.Errorf("deepread: якорь %q: owner_id %d вне реестра ключей", anchor.Key, anchor.OwnerID)
		}
		if len(anchor.Centroid) != models.EmbedDim {
			return fmt.Errorf("deepread: якорь %q: dim %d, want %d",
				anchor.Key, len(anchor.Centroid), models.EmbedDim)
		}

		if err := execLiteraryVectors(lit,
			`DELETE FROM vectors WHERE owner_table = ? AND owner_id = ?`,
			DictAnchorTable, anchor.OwnerID); err != nil {
			return fmt.Errorf("deepread: якорь %q: %w", anchor.Key, err)
		}
		if err := execLiteraryVectors(lit,
			`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			DictAnchorTable, anchor.OwnerID, anchorVectorModel, models.EmbedDim,
			anchorVectorLang, anchorSource(anchor), vectorBlob(anchor.Centroid), now); err != nil {
			return fmt.Errorf("deepread: якорь %q: %w", anchor.Key, err)
		}
	}
	return nil
}

// anchorSource описывает чанки якоря для source_text вектора.
func anchorSource(anchor DictAnchor) string {
	parts := make([]string, len(anchor.Chunks))
	for i, id := range anchor.Chunks {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return "якорь категории " + anchor.Key + ": чанки " + strings.Join(parts, ",")
}

// WriteStagingCandidates пишет кандидатов в dict_staging (шаг 3): новая пара
// (dict_key, lemma) — строка candidate, повтор поднимает repeats на 1 без новой
// строки. Статус существующей строки код не меняет (промоушен — владелец).
func WriteStagingCandidates(lit *db.Handle, bookID int64, cands []StagingCandidate, now string) (int, int, error) {
	if lit == nil {
		return 0, 0, ErrNoStore
	}
	if bookID <= 0 {
		return 0, 0, fmt.Errorf("deepread: staging: book_id %d", bookID)
	}
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}

	seen := make(map[string]struct{}, len(cands))
	inserted, repeated := 0, 0
	err := lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		for _, cand := range cands {
			if CategoryID(cand.DictKey) < 0 {
				return fmt.Errorf("deepread: staging: ключ %q вне реестра категорий", cand.DictKey)
			}
			if strings.TrimSpace(cand.Lemma) == "" {
				return fmt.Errorf("deepread: staging: пустая лемма для ключа %q", cand.DictKey)
			}
			if cand.Weight < minLemmaWeight || cand.Weight > maxLemmaWeight {
				return fmt.Errorf("deepread: staging: лемма %q: вес %d вне %d..%d",
					cand.Lemma, cand.Weight, minLemmaWeight, maxLemmaWeight)
			}
			if strings.TrimSpace(cand.Evidence) == "" {
				return fmt.Errorf("deepread: staging: лемма %q: нет основания-цитаты", cand.Lemma)
			}

			pair := cand.DictKey + "\x00" + cand.Lemma
			if _, dup := seen[pair]; dup {
				return fmt.Errorf("deepread: staging: кандидат (%q, %q) повторяется",
					cand.DictKey, cand.Lemma)
			}
			seen[pair] = struct{}{}

			res, err := tx.Exec("dict_staging",
				`UPDATE dict_staging SET repeats = repeats + 1 WHERE dict_key = ? AND lemma = ?`,
				cand.DictKey, cand.Lemma)
			if err != nil {
				return fmt.Errorf("deepread: staging: лемма %q: %w", cand.Lemma, err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("deepread: staging: лемма %q: %w", cand.Lemma, err)
			}
			if n > 0 {
				repeated++
				continue
			}
			if _, err := tx.Exec("dict_staging",
				`INSERT INTO dict_staging (dict_key, lemma, weight, evidence, book_id, status, repeats, created_at) VALUES (?, ?, ?, ?, ?, ?, 1, ?)`,
				cand.DictKey, cand.Lemma, cand.Weight, cand.Evidence, bookID,
				StagingStatusCandidate, now); err != nil {
				return fmt.Errorf("deepread: staging: лемма %q: %w", cand.Lemma, err)
			}
			inserted++
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return inserted, repeated, nil
}

// StageDictionaries прогоняет staging целиком (шаги 1–3): якоря категорий из
// confident-чанков пишутся в vectors, кандидаты — в dict_staging. Словари
// приходят загруженными вызывающим: пак разбор не читает и не пишет.
func StageDictionaries(lit, world *db.Handle, in StagingInput) (*StagingResult, error) {
	if lit == nil || world == nil {
		return nil, ErrNoStore
	}
	if in.BookID <= 0 {
		return nil, fmt.Errorf("deepread: staging: book_id %d", in.BookID)
	}
	cfg := in.Config
	if cfg == (StagingConfig{}) {
		cfg = DefaultStagingConfig()
	}
	if err := validateStagingConfig(cfg); err != nil {
		return nil, err
	}
	if len(in.Dicts) == 0 {
		return nil, fmt.Errorf("deepread: staging: нет словарей категорий")
	}

	ids := make([]int64, 0, len(in.Chunks))
	for _, chunk := range in.Chunks {
		if chunk.ID > 0 {
			ids = append(ids, chunk.ID)
		}
	}
	vectors, err := chunkVectors(lit, world, in.BookID, ids)
	if err != nil {
		return nil, err
	}

	anchors := BuildDictAnchors(in.Chunks, vectors, in.Tags, in.Dicts)
	if err := WriteDictAnchors(lit, anchors, ""); err != nil {
		return nil, err
	}
	cands := BuildStagingCandidates(in.Chunks, vectors, anchors, in.Dicts, cfg)
	inserted, repeated, err := WriteStagingCandidates(lit, in.BookID, cands, "")
	if err != nil {
		return nil, err
	}
	return &StagingResult{
		Anchors:    anchors,
		Candidates: cands,
		Inserted:   inserted,
		Repeated:   repeated,
	}, nil
}
