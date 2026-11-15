package deepread_test

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"tns65/internal/db"
	"tns65/internal/deepread"
	"tns65/internal/workshop"
)

// styleFixture — fb2-фикстура стилевого гейта: автор в метаданных парсера и
// две главы — лирическая и боевая.
const styleFixture = `<?xml version="1.0" encoding="utf-8"?>
<FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0">
  <description>
    <title-info>
      <author>
        <first-name>Иван</first-name>
        <last-name>Тургенев</last-name>
      </author>
      <book-title>Стилевая проба</book-title>
    </title-info>
  </description>
  <body>
    <section>
      <title>Глава 1. Лирический вечер</title>
      <p>Вечер был тихий. Вечер был долгий. Вечер был светлый… Ветер стихал, и вечер был тихий.</p>
    </section>
    <section>
      <title>Глава 2. Бой</title>
      <p>Клинок блеснул! Враг упал! Кровь на снегу! Бой стих!</p>
    </section>
  </body>
</FictionBook>
`

// TestStyle — гейт стиля, драмы и автора (W-27.1–W-27.3, W-27.5, приёмка
// T-14-09): на фикстуре из лирической и боевой глав ярлыки берутся только из
// канона styles.json и различаются, драматичность лежит в 0..1 и выше у боевой
// главы, автор равен метаданным парсера, профиль идемпотентно ложится в
// book_style.
func TestStyle(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Fatal("корень движка не найден")
	}
	packDir := filepath.Join(root, "packs", "ru")
	styles, err := deepread.LoadStyles(packDir)
	if err != nil {
		t.Fatalf("LoadStyles: %v", err)
	}

	path := filepath.Join(t.TempDir(), "style.fb2")
	if err := os.WriteFile(path, []byte(styleFixture), 0o600); err != nil {
		t.Fatalf("запись фикстуры: %v", err)
	}
	book, err := workshop.ParseBookInput(path)
	if err != nil {
		t.Fatalf("ParseBookInput: %v", err)
	}
	if len(book.Chunks) != 2 {
		t.Fatalf("чанков фикстуры %d, ожидалось 2", len(book.Chunks))
	}
	lyricText := book.Chunks[0].Text
	battleText := book.Chunks[1].Text
	if !strings.Contains(lyricText, "Вечер") || !strings.Contains(battleText, "Клинок") {
		t.Fatalf("главы фикстуры не совпали с ожидаемыми: %q / %q", lyricText, battleText)
	}

	lyric, err := deepread.BuildBookStyle(deepread.StyleInput{
		Author: book.Meta.Author,
		Text:   lyricText,
		Lemmas: styleLemmas(lyricText),
		Chunks: []deepread.StyleChunk{{
			ID:       1,
			Text:     lyricText,
			Salience: map[string]float64{deepread.KeyActionDramatic: 0.05, deepread.KeyEmotion: 0.1},
		}},
		Styles: styles,
	})
	if err != nil {
		t.Fatalf("BuildBookStyle(лирика): %v", err)
	}
	battle, err := deepread.BuildBookStyle(deepread.StyleInput{
		Author: book.Meta.Author,
		Text:   battleText,
		Lemmas: styleLemmas(battleText),
		Chunks: []deepread.StyleChunk{{
			ID:   2,
			Text: battleText,
			Salience: map[string]float64{
				deepread.KeyActionDramatic: 0.9,
				deepread.KeyViolence:       0.8,
				deepread.KeyBattle:         0.85,
				deepread.KeyEmotion:        0.6,
			},
		}},
		Styles: styles,
	})
	if err != nil {
		t.Fatalf("BuildBookStyle(бой): %v", err)
	}

	t.Run("ярлыки канона", func(t *testing.T) {
		canon := make(map[string]struct{}, len(styles.Labels))
		for _, label := range styles.Labels {
			canon[label.Name] = struct{}{}
		}
		for _, style := range []*deepread.BookStyle{lyric, battle} {
			for _, label := range style.Labels {
				if _, ok := canon[label]; !ok {
					t.Errorf("ярлык %q вне styles.json", label)
				}
			}
		}
		if !hasStyleLabel(lyric.Labels, deepread.StyleLyrical) {
			t.Errorf("ярлыки лирики %v без %q", lyric.Labels, deepread.StyleLyrical)
		}
		if hasStyleLabel(lyric.Labels, deepread.StyleDynamic) {
			t.Errorf("ярлыки лирики %v содержат %q", lyric.Labels, deepread.StyleDynamic)
		}
		if !hasStyleLabel(battle.Labels, deepread.StyleDynamic) {
			t.Errorf("ярлыки боя %v без %q", battle.Labels, deepread.StyleDynamic)
		}
		if hasStyleLabel(battle.Labels, deepread.StyleLyrical) {
			t.Errorf("ярлыки боя %v содержат %q", battle.Labels, deepread.StyleLyrical)
		}
	})

	t.Run("драматичность", func(t *testing.T) {
		for _, style := range []*deepread.BookStyle{lyric, battle} {
			if len(style.Drama) != 1 {
				t.Fatalf("строк драматичности %d, ожидалась 1", len(style.Drama))
			}
			value := style.Drama[0].Drama
			if math.IsNaN(value) || value < 0 || value > 1 {
				t.Errorf("драматичность %g вне 0..1", value)
			}
			for feature, value := range style.Features {
				if math.IsNaN(value) || value < 0 {
					t.Errorf("признак %q = %g", feature, value)
				}
				if feature != deepread.FeatureAvgSentenceLen && value > 1 {
					t.Errorf("признак %q = %g вне 0..1", feature, value)
				}
			}
			for pos, share := range style.POS {
				if share < 0 || share > 1 {
					t.Errorf("частота части речи %q = %g вне 0..1", pos, share)
				}
			}
		}
		if battle.Drama[0].Drama <= lyric.Drama[0].Drama {
			t.Errorf("драматичность боя %g не выше лирики %g",
				battle.Drama[0].Drama, lyric.Drama[0].Drama)
		}
		if battle.Features[deepread.FeatureExclaimDensity] <= lyric.Features[deepread.FeatureExclaimDensity] {
			t.Errorf("плотность ! боя %g не выше лирики %g",
				battle.Features[deepread.FeatureExclaimDensity], lyric.Features[deepread.FeatureExclaimDensity])
		}
		if len(lyric.Histogram) == 0 || len(lyric.POS) == 0 {
			t.Errorf("профиль без гистограммы или частей речи: %v / %v", lyric.Histogram, lyric.POS)
		}
	})

	t.Run("автор из метаданных", func(t *testing.T) {
		if book.Meta.Author != "Иван Тургенев" {
			t.Fatalf("автор парсера %q, ожидался «Иван Тургенев»", book.Meta.Author)
		}
		if lyric.Author != book.Meta.Author || battle.Author != book.Meta.Author {
			t.Errorf("автор профиля %q/%q, метаданные %q", lyric.Author, battle.Author, book.Meta.Author)
		}
	})

	t.Run("запись book_style", func(t *testing.T) {
		lit, bookID := newTestLiterary(t)
		if err := deepread.WriteBookStyle(lit, bookID, battle, "2026-11-14T00:00:00Z"); err != nil {
			t.Fatalf("WriteBookStyle(бой): %v", err)
		}
		got := readBookStyle(t, lit, bookID)
		if got.author != battle.Author {
			t.Errorf("author = %q, ожидался %q", got.author, battle.Author)
		}
		var labels []string
		if err := json.Unmarshal([]byte(got.labels), &labels); err != nil {
			t.Fatalf("labels: %v", err)
		}
		if !hasStyleLabel(labels, deepread.StyleDynamic) {
			t.Errorf("labels = %v без %q", labels, deepread.StyleDynamic)
		}
		var drama []deepread.ChunkDrama
		if err := json.Unmarshal([]byte(got.drama), &drama); err != nil {
			t.Fatalf("drama_profile: %v", err)
		}
		if len(drama) != 1 || drama[0].Chunk != 2 {
			t.Fatalf("drama_profile = %+v, ожидался чанк 2", drama)
		}
		if math.Abs(drama[0].Drama-battle.Drama[0].Drama) > 1e-9 {
			t.Errorf("drama_profile = %g, ожидалось %g", drama[0].Drama, battle.Drama[0].Drama)
		}

		var features map[string]json.RawMessage
		if err := json.Unmarshal([]byte(got.features), &features); err != nil {
			t.Fatalf("features: %v", err)
		}
		var avg float64
		if err := json.Unmarshal(features[deepread.FeatureAvgSentenceLen], &avg); err != nil {
			t.Fatalf("features.avg_sentence_len: %v", err)
		}
		if math.Abs(avg-battle.Features[deepread.FeatureAvgSentenceLen]) > 1e-9 {
			t.Errorf("features.avg_sentence_len = %g, ожидалось %g",
				avg, battle.Features[deepread.FeatureAvgSentenceLen])
		}
		if _, ok := features["sentence_histogram"]; !ok {
			t.Errorf("features без sentence_histogram: %s", got.features)
		}
		if _, ok := features["pos_frequencies"]; !ok {
			t.Errorf("features без pos_frequencies: %s", got.features)
		}

		// Повторная запись не плодит строк: профиль книги обновляется.
		if err := deepread.WriteBookStyle(lit, bookID, lyric, ""); err != nil {
			t.Fatalf("WriteBookStyle(лирика): %v", err)
		}
		var rows int
		if err := lit.QueryRow(`SELECT COUNT(*) FROM book_style WHERE book_id = ?`, bookID).Scan(&rows); err != nil {
			t.Fatalf("COUNT book_style: %v", err)
		}
		if rows != 1 {
			t.Errorf("строк book_style %d, ожидалась 1", rows)
		}
		got = readBookStyle(t, lit, bookID)
		if err := json.Unmarshal([]byte(got.drama), &drama); err != nil {
			t.Fatalf("drama_profile после обновления: %v", err)
		}
		if len(drama) != 1 || drama[0].Chunk != 1 {
			t.Errorf("drama_profile после обновления = %+v, ожидался чанк 1", drama)
		}
	})

	t.Run("отказы записи", func(t *testing.T) {
		if err := deepread.WriteBookStyle(nil, 1, lyric, ""); !errors.Is(err, deepread.ErrNoStore) {
			t.Errorf("WriteBookStyle(nil) = %v, want ErrNoStore", err)
		}
		lit, bookID := newTestLiterary(t)
		if err := deepread.WriteBookStyle(lit, 0, lyric, ""); err == nil {
			t.Error("WriteBookStyle без книги должен вернуть ошибку")
		}
		badDrama := *lyric
		badDrama.Drama = []deepread.ChunkDrama{{Chunk: 1, Drama: 1.5}}
		if err := deepread.WriteBookStyle(lit, bookID, &badDrama, ""); err == nil {
			t.Error("драматичность 1.5 должна вернуть ошибку")
		}
		badLabel := *lyric
		badLabel.Labels = []string{"эпический"}
		if err := deepread.WriteBookStyle(lit, bookID, &badLabel, ""); err == nil {
			t.Error("ярлык вне канона должен вернуть ошибку")
		}
	})
}

// hasStyleLabel — есть ли ярлык в списке.
func hasStyleLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}

// styleLemmas — леммы фикстуры без резидента: форма токена служит его леммой,
// части речи заданы условно для частот профиля.
func styleLemmas(text string) []deepread.Lemma {
	var out []deepread.Lemma
	var b strings.Builder
	flush := func(line int) {
		if b.Len() == 0 {
			return
		}
		form := b.String()
		b.Reset()
		out = append(out, deepread.Lemma{
			Form:  form,
			Lemma: form,
			POS:   "NOUN",
			Line:  line,
			Index: len(out),
		})
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush(1)
	}
	flush(1)
	return out
}

// bookStyleRow — обновляемые поля строки book_style.
type bookStyleRow struct {
	author   string
	features string
	labels   string
	drama    string
}

// readBookStyle читает строку профиля книги.
func readBookStyle(t *testing.T, lit *db.Handle, bookID int64) bookStyleRow {
	t.Helper()
	var row bookStyleRow
	if err := lit.QueryRow(
		`SELECT author, features, labels, drama_profile FROM book_style WHERE book_id = ?`, bookID).
		Scan(&row.author, &row.features, &row.labels, &row.drama); err != nil {
		t.Fatalf("SELECT book_style: %v", err)
	}
	return row
}
