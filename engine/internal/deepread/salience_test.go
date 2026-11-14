package deepread_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"tns65/internal/db"
	"tns65/internal/deepread"
)

// TestSalienceBands — гейт salience и тегов чанков (W-23.1–W-23.3, приёмка
// T-14-05): фикстура из пяти чанков (пейзажный, интерьерный, боевой,
// диалоговый и связный без лемм категорий) размечена полосами; пейзажный чанк
// получает nature=confident с якорем-подстрокой, связный — все ключи reject,
// порог 0.99 переводит пейзажный в uncertain, теги книги идемпотентно
// ложатся в book_chunk_tag.
func TestSalienceBands(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Fatal("корень движка не найден")
	}

	dicts, err := deepread.LoadDictionaries(filepath.Join(root, "packs", "ru"))
	if err != nil {
		t.Fatalf("LoadDictionaries: %v", err)
	}

	cfg, err := deepread.LoadSalienceConfig(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatalf("LoadSalienceConfig(engine/config.yaml): %v", err)
	}
	if cfg.Confident != 0.6 || cfg.Reject != 0.15 {
		t.Fatalf("пороги config.yaml = %+v, ожидались 0.6/0.15", cfg)
	}

	t.Run("idf", func(t *testing.T) {
		chunks := []deepread.SalienceChunk{
			{Lemmas: []deepread.Lemma{{Lemma: "лес"}, {Lemma: "лес"}, {Lemma: "река"}}},
			{Lemmas: []deepread.Lemma{{Lemma: "лес"}}},
		}
		idf := deepread.IDF(chunks)
		// N = 2: «лес» в обоих чанках (df=2), «река» в одном (df=1).
		if want := math.Log(1 + 2.0/3.0); math.Abs(idf["лес"]-want) > 1e-9 {
			t.Errorf("idf(лес) = %g, want %g", idf["лес"], want)
		}
		if want := math.Log(1 + 2.0/2.0); math.Abs(idf["река"]-want) > 1e-9 {
			t.Errorf("idf(река) = %g, want %g", idf["река"], want)
		}
	})

	chunks := salienceFixture()

	t.Run("полосы и якоря", func(t *testing.T) {
		tags, err := deepread.TagChunks(chunks, dicts, cfg)
		if err != nil {
			t.Fatalf("TagChunks: %v", err)
		}
		if len(tags) != len(chunks) {
			t.Fatalf("тегов чанков %d, ожидалось %d", len(tags), len(chunks))
		}
		for i, chunkTags := range tags {
			if chunkTags.ChunkID != chunks[i].ID {
				t.Errorf("чанк %d: ChunkID %d, want %d", i, chunkTags.ChunkID, chunks[i].ID)
			}
			if len(chunkTags.Tags) != len(deepread.DictionaryKeys) {
				t.Errorf("чанк %d: тегов %d, ожидалось %d", i, len(chunkTags.Tags), len(deepread.DictionaryKeys))
			}
			byKey := tagMap(t, chunkTags)
			mark := salienceMarks[i]
			for _, key := range deepread.DictionaryKeys {
				tag, ok := byKey[key]
				if !ok {
					t.Fatalf("чанк %s: нет тега ключа %q", mark.name, key)
				}
				want := deepread.BandReject
				switch {
				case containsKey(mark.confident, key):
					want = deepread.BandConfident
				case containsKey(mark.uncertain, key):
					want = deepread.BandUncertain
				}
				if tag.Band != want {
					t.Errorf("чанк %s, ключ %s: полоса %q (скор %.3f), want %q",
						mark.name, key, tag.Band, tag.Score, want)
				}
				if tag.Score < 0 || tag.Score > 1 {
					t.Errorf("чанк %s, ключ %s: скор %.3f вне 0..1", mark.name, key, tag.Score)
				}
				if want == deepread.BandReject {
					if len(tag.Anchors) != 0 {
						t.Errorf("чанк %s, ключ %s: у reject якоря %q", mark.name, key, tag.Anchors)
					}
					continue
				}
				if len(tag.Anchors) == 0 {
					t.Errorf("чанк %s, ключ %s: нет якорей", mark.name, key)
				}
				if len(tag.Anchors) > 3 {
					t.Errorf("чанк %s, ключ %s: якорей %d, предел 3", mark.name, key, len(tag.Anchors))
				}
				for _, anchor := range tag.Anchors {
					if !strings.Contains(chunks[i].Text, anchor) {
						t.Errorf("чанк %s, ключ %s: якорь не подстрока чанка: %q", mark.name, key, anchor)
					}
				}
			}
		}

		// Приёмка: пейзажный чанк — nature=confident с якорем вокруг «лес».
		nature := tagMap(t, tags[0])["nature"]
		if nature.Band != deepread.BandConfident {
			t.Fatalf("пейзажный nature: полоса %q, want confident", nature.Band)
		}
		if len(nature.Anchors) == 0 {
			t.Fatal("пейзажный nature: нет якоря")
		}
		if anchor := strings.ToLower(nature.Anchors[0]); !strings.Contains(anchor, "лес") {
			t.Errorf("якорь пейзажного чанка не содержит опорную лемму «лес»: %q", nature.Anchors[0])
		}

		// Приёмка: связный чанк — все ключи reject со скором 0.
		for _, tag := range tags[4].Tags {
			if tag.Band != deepread.BandReject {
				t.Errorf("связный чанк, ключ %s: полоса %q, want reject", tag.Key, tag.Band)
			}
			if tag.Score != 0 {
				t.Errorf("связный чанк, ключ %s: скор %g, want 0", tag.Key, tag.Score)
			}
		}

		// Смена порогов в конфиге меняет полосы: при confident=0.99 пейзажный
		// чанк падает в uncertain, reject-чанки не меняются.
		strict := cfg
		strict.Confident = 0.99
		strictTags, err := deepread.TagChunks(chunks, dicts, strict)
		if err != nil {
			t.Fatalf("TagChunks(порог 0.99): %v", err)
		}
		natureStrict := tagMap(t, strictTags[0])["nature"]
		if natureStrict.Band != deepread.BandUncertain {
			t.Errorf("порог 0.99: пейзажный nature = %q (скор %.3f), want uncertain",
				natureStrict.Band, natureStrict.Score)
		}
		if natureStrict.Score != nature.Score {
			t.Errorf("порог 0.99: скор nature изменился %g → %g", nature.Score, natureStrict.Score)
		}
		for _, tag := range strictTags[4].Tags {
			if tag.Band != deepread.BandReject {
				t.Errorf("порог 0.99: связный чанк, ключ %s: полоса %q, want reject", tag.Key, tag.Band)
			}
		}
	})

	t.Run("запись тегов", func(t *testing.T) {
		lit, bookID := newTestLiterary(t)
		ids := make([]int64, len(chunks))
		for i := range chunks {
			res, err := lit.Exec("book_chunk",
				`INSERT INTO book_chunk (book_id, ord, chapter, text_hash) VALUES (?, ?, '1', ?)`,
				bookID, i+1, fmt.Sprintf("h%d", i+1))
			if err != nil {
				t.Fatalf("INSERT book_chunk %d: %v", i, err)
			}
			if ids[i], err = res.LastInsertId(); err != nil {
				t.Fatalf("book_chunk LastInsertId: %v", err)
			}
		}

		tags, err := deepread.TagChunks(chunks, dicts, cfg)
		if err != nil {
			t.Fatalf("TagChunks: %v", err)
		}
		for i := range tags {
			tags[i].ChunkID = ids[i]
		}
		if err := deepread.WriteChunkTags(lit, tags); err != nil {
			t.Fatalf("WriteChunkTags: %v", err)
		}

		wantRows := len(chunks) * len(deepread.DictionaryKeys)
		if n := countChunkTags(t, lit); n != wantRows {
			t.Fatalf("строк book_chunk_tag %d, ожидалось %d", n, wantRows)
		}

		// Тег пейзажного чанка: полоса и якоря как JSON-массив строк.
		var score float64
		var band, anchors string
		if err := lit.QueryRow(
			`SELECT score, band, anchors FROM book_chunk_tag WHERE chunk_id = ? AND key = 'nature'`,
			ids[0]).Scan(&score, &band, &anchors); err != nil {
			t.Fatalf("SELECT nature: %v", err)
		}
		if band != deepread.BandConfident {
			t.Errorf("пейзажный nature в базе: полоса %q, want confident", band)
		}
		var decoded []string
		if err := json.Unmarshal([]byte(anchors), &decoded); err != nil {
			t.Fatalf("anchors JSON %q: %v", anchors, err)
		}
		if len(decoded) == 0 {
			t.Errorf("пейзажный nature в базе: якорей нет (%q)", anchors)
		}
		if want := tagMap(t, tags[0])["nature"].Anchors; len(decoded) != len(want) {
			t.Errorf("пейзажный nature в базе: якорей %d, want %d", len(decoded), len(want))
		}

		// Связный чанк: якоря — пустой JSON-массив, как дефолт колонки.
		if err := lit.QueryRow(
			`SELECT anchors FROM book_chunk_tag WHERE chunk_id = ? AND key = 'nature'`,
			ids[4]).Scan(&anchors); err != nil {
			t.Fatalf("SELECT связный nature: %v", err)
		}
		if anchors != "[]" {
			t.Errorf("связный чанк: anchors %q, want «[]»", anchors)
		}

		// Идемпотентность: повторная запись заменяет строки чанка, а не плодит.
		for i := range tags[0].Tags {
			if tags[0].Tags[i].Key == "nature" {
				tags[0].Tags[i].Score = 0.42
				tags[0].Tags[i].Band = deepread.BandUncertain
			}
		}
		if err := deepread.WriteChunkTags(lit, tags); err != nil {
			t.Fatalf("повторный WriteChunkTags: %v", err)
		}
		if n := countChunkTags(t, lit); n != wantRows {
			t.Fatalf("после повтора строк %d, ожидалось %d", n, wantRows)
		}
		if err := lit.QueryRow(
			`SELECT score, band FROM book_chunk_tag WHERE chunk_id = ? AND key = 'nature'`,
			ids[0]).Scan(&score, &band); err != nil {
			t.Fatalf("SELECT после повтора: %v", err)
		}
		if band != deepread.BandUncertain || math.Abs(score-0.42) > 1e-9 {
			t.Errorf("после повтора nature = (%g, %q), want (0.42, uncertain)", score, band)
		}

		// Ошибки записи: нет базы, полоса вне канона.
		if err := deepread.WriteChunkTags(nil, tags); !errors.Is(err, deepread.ErrNoStore) {
			t.Errorf("WriteChunkTags(nil) = %v, want ErrNoStore", err)
		}
		bad := []deepread.ChunkTags{{ChunkID: ids[0], Tags: []deepread.ChunkTag{{Key: "nature", Band: "xxx"}}}}
		if err := deepread.WriteChunkTags(lit, bad); err == nil {
			t.Error("WriteChunkTags с полосой xxx не вернул ошибку")
		}
	})
}

// salienceChunkMark — разметка фикстуры: чанк и полосы confident/uncertain;
// прочие ключи размечены reject.
type salienceChunkMark struct {
	name      string
	confident []string
	uncertain []string
}

// salienceMarks — разметка пяти чанков фикстуры (шаг 6 T-14-05).
var salienceMarks = []salienceChunkMark{
	{name: "пейзажный", confident: []string{deepread.KeyNature}},
	{name: "интерьерный", confident: []string{deepread.KeyArchitecture}},
	{name: "боевой", confident: []string{deepread.KeyBattle}},
	{name: "диалоговый", uncertain: []string{deepread.KeySocial}},
	{name: "связный"},
}

// salienceFixture — пять чанков разметки: линии чанка — предложения, словоформы
// словарей помечены леммами.
func salienceFixture() []deepread.SalienceChunk {
	return []deepread.SalienceChunk{
		fixtureChunk(1, strings.Join([]string{
			"Лес стоял тихо на берегу.",
			"Река блестела под солнцем.",
			"Дальше начиналось поле.",
			"Небо было ясным.",
			"Трава шелестела у дороги.",
			"Птица пролетела над водой.",
		}, "\n"), mark{"берегу": "берег", "солнцем": "солнце"}),
		fixtureChunk(2, strings.Join([]string{
			"Стена была холодной, окно смотрело во двор.",
			"Дверь в комнату оставалась открытой.",
			"В коридоре горела свеча.",
		}, "\n"), mark{"комнату": "комната", "коридоре": "коридор"}),
		fixtureChunk(3, strings.Join([]string{
			"Битва шла весь день.",
			"Войско стояло в строю.",
			"Отряд пошёл в атаку.",
			"Залп гремел долго.",
		}, "\n"), mark{"строю": "строй", "атаку": "атака"}),
		fixtureChunk(4, strings.Join([]string{
			"Господин вошёл и поклонился.",
			"Князь ответил ему коротко.",
			"Гость молчал.",
		}, "\n"), nil),
		fixtureChunk(5, strings.Join([]string{
			"Он кивнул и вышел.",
			"Потом он посмотрел в сторону.",
		}, "\n"), nil),
	}
}

// mark — лемма словаря для словоформы фикстуры.
type mark map[string]string

// fixtureChunk собирает чанк фикстуры: каждая строка — предложение, её слова
// становятся токенами с номером строки и позицией, как их вернул бы резидент;
// словоформы вне mark получают лемму, равную себе.
func fixtureChunk(id int64, text string, lemmaOf mark) deepread.SalienceChunk {
	chunk := deepread.SalienceChunk{ID: id, Text: text}
	for lineNo, line := range strings.Split(text, "\n") {
		index := 0
		for _, word := range wordsOf(line) {
			lemma, ok := lemmaOf[word]
			if !ok {
				lemma = word
			}
			chunk.Lemmas = append(chunk.Lemmas, deepread.Lemma{
				Form: word, Lemma: lemma, POS: "NOUN", Line: lineNo + 1, Index: index,
			})
			index++
		}
	}
	return chunk
}

// wordsOf выделяет слова строки в нижнем регистре.
func wordsOf(line string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range line {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return out
}

// tagMap переводит теги чанка в карту по ключу.
func tagMap(t *testing.T, chunkTags deepread.ChunkTags) map[string]deepread.ChunkTag {
	t.Helper()
	out := make(map[string]deepread.ChunkTag, len(chunkTags.Tags))
	for _, tag := range chunkTags.Tags {
		out[tag.Key] = tag
	}
	return out
}

// containsKey проверяет ключ в списке.
func containsKey(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

// newTestLiterary создаёт литературную базу с книгой и возвращает хэндл forge.
func newTestLiterary(t *testing.T) (*db.Handle, int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "literary.db")
	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter(literary): %v", err)
	}
	if err := db.Migrate(raw, "literary", "shared"); err != nil {
		_ = raw.Close()
		t.Fatalf("Migrate(literary, shared): %v", err)
	}
	_ = raw.Close()

	lit, err := db.OpenWriterAs(path, db.DBLiterary, db.WriterForge, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(literary): %v", err)
	}
	t.Cleanup(func() { _ = lit.Close() })

	res, err := lit.Exec("book",
		`INSERT INTO book (shelf_id, name, lang, genres, rating, chapters, status, created_at) VALUES ('p13', 'Разметка salience', 'ru', '', '', 5, 'ready', '2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("INSERT book: %v", err)
	}
	bookID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("book LastInsertId: %v", err)
	}
	return lit, bookID
}

// countChunkTags — число строк book_chunk_tag.
func countChunkTags(t *testing.T, lit *db.Handle) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM book_chunk_tag`).Scan(&n); err != nil {
		t.Fatalf("COUNT book_chunk_tag: %v", err)
	}
	return n
}
