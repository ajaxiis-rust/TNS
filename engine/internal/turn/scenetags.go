package turn

// scenetags.go — мост между языком сцены и словарём разбора книги: теги сцены
// для банка берутся из смыслов фразы (категории словарей пакета), а не только
// из имени намерения. Банк хранит записи с ключами book_chunk_tag (nature,
// architecture, appearance…), поэтому без этого моста пересечение тегов всегда
// пусто и Челнок уходит в fallback.

import (
	"sort"
	"strings"
	"sync"

	"tns65/internal/deepread"
)

// sceneTagPackDir — каталог языкового пакета для тегов сцены; пусто — пакет не
// подключён, остаются прежние теги намерения/настроения/семьи.
var sceneTagPackDir = "packs/ru"

// sceneTagLayers — словари категорий и смысловой слой пака, загруженные один
// раз на каталог. Пустые слои — пакет недоступен: теги сцены деградируют до
// намерения и настроения.
type sceneTagLayers struct {
	dicts  map[string]*deepread.Dictionary
	senses *deepread.SenseLayer
}

var sceneTagCache sync.Map // map[string]*sceneTagLayers

// sceneTagsFrom — теги сцены: намерение, настроение, семья композиции и ключи
// категорий разбора, найденные в словарях по разбору фразы.
func sceneTagsFrom(packDir string, st *TurnState) []string {
	var tags []string
	if st == nil {
		return tags
	}
	if st.Prepath.Intent.Name != "" {
		tags = append(tags, st.Prepath.Intent.Name)
	}
	if st.Prepath.Mood != "" {
		tags = append(tags, st.Prepath.Mood)
	}
	if st.Composition.FamilyPrimary != "" {
		tags = append(tags, st.Composition.FamilyPrimary)
	}
	return append(tags, sceneCategoryTags(packDir, st)...)
}

// sceneCategoryTags считает ключи категорий словарей пака по сегментам фразы.
// Смысловой слой снимает вклад омонима, решённого в другую категорию; запись
// берётся при любом ненулевом скоре — важна не полоса, а пересечение с тегами
// записи банка.
func sceneCategoryTags(packDir string, st *TurnState) []string {
	layers := loadSceneTagLayers(packDir)
	if layers == nil || len(layers.dicts) == 0 || len(st.Cut.Segments) == 0 {
		return nil
	}
	chunks := make([]deepread.SalienceChunk, 0, len(st.Cut.Segments))
	for i, seg := range st.Cut.Segments {
		lemmas := segmentLemmas(seg)
		if len(lemmas) == 0 {
			continue
		}
		text := seg.Text
		if strings.TrimSpace(text) == "" {
			text = strings.Join(seg.Lemmas, " ")
		}
		chunks = append(chunks, deepread.SalienceChunk{ID: int64(i + 1), Text: text, Lemmas: lemmas})
	}
	if len(chunks) == 0 {
		return nil
	}
	tagged, err := deepread.TagChunksWithSenses(chunks, layers.dicts, layers.senses, deepread.DefaultSalienceConfig())
	if err != nil {
		return nil
	}
	set := make(map[string]struct{})
	for _, chunk := range tagged {
		for _, tag := range chunk.Tags {
			if tag.Score > 0 {
				set[tag.Key] = struct{}{}
			}
		}
	}
	tags := make([]string, 0, len(set))
	for tag := range set {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// loadSceneTagLayers читает словари и смыслы пака; ошибка чтения кэшируется как
// пустой слой — ход не деградирует из-за отсутствия пакета.
func loadSceneTagLayers(packDir string) *sceneTagLayers {
	if packDir == "" {
		return &sceneTagLayers{}
	}
	if cached, ok := sceneTagCache.Load(packDir); ok {
		return cached.(*sceneTagLayers)
	}
	layers := &sceneTagLayers{}
	if dicts, err := deepread.LoadDictionaries(packDir); err == nil {
		layers.dicts = dicts
		if senses, err := deepread.LoadSenseLayer(packDir, deepread.DefaultSalienceConfig()); err == nil {
			layers.senses = senses
		}
	}
	actual, _ := sceneTagCache.LoadOrStore(packDir, layers)
	return actual.(*sceneTagLayers)
}

// segmentLemmas — разбор сегмента для словарей: токены лемматизатора с формами
// и частями речи; сегмент без токенов (тестовая фикстура) даёт леммы как есть.
func segmentLemmas(seg Segment) []deepread.Lemma {
	if len(seg.Tokens) > 0 {
		out := make([]deepread.Lemma, 0, len(seg.Tokens))
		for _, tok := range seg.Tokens {
			lemma := strings.ToLower(strings.TrimSpace(tok.Lemma))
			if lemma == "" {
				lemma = strings.ToLower(strings.TrimSpace(tok.Form))
			}
			if lemma == "" {
				continue
			}
			out = append(out, deepread.Lemma{
				Form:   tok.Form,
				Lemma:  lemma,
				POS:    tok.Pos,
				Parses: tok.AllParses(),
			})
		}
		return out
	}
	out := make([]deepread.Lemma, 0, len(seg.Lemmas))
	for _, raw := range seg.Lemmas {
		lemma := strings.ToLower(strings.TrimSpace(raw))
		if lemma == "" {
			continue
		}
		out = append(out, deepread.Lemma{Form: raw, Lemma: lemma})
	}
	return out
}
