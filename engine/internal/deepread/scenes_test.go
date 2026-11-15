package deepread_test

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"tns65/internal/db"
	"tns65/internal/deepread"
)

// TestScenes — гейт мест и сцен (W-26, приёмка T-14-08): места из словарей
// architecture/nature, границы по смене места и времени, склейка смежных
// чанков с общим профилем, запись book_place/book_scene и векторов сцен.
func TestScenes(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Fatal("корень движка не найден")
	}
	packDir := filepath.Join(root, "packs", "ru")

	dicts, err := deepread.LoadDictionaries(packDir)
	if err != nil {
		t.Fatalf("LoadDictionaries: %v", err)
	}
	mood, err := deepread.LoadEmotionDictionary(filepath.Join(packDir, "detectors_mood.json"))
	if err != nil {
		t.Fatalf("LoadEmotionDictionary: %v", err)
	}
	clock, err := deepread.LoadTimeLexicon(packDir)
	if err != nil {
		t.Fatalf("LoadTimeLexicon: %v", err)
	}

	t.Run("словарь времени", func(t *testing.T) {
		chunk := sceneChunk(1, "Спустя три дня Иван вернулся.", sceneWords)
		markers := clock.Markers(chunk)
		if len(markers) != 1 || markers[0] != "Спустя три дня" {
			t.Errorf("Markers = %v, want [Спустя три дня]", markers)
		}
		if got := clock.Markers(sceneChunk(2, "Иван молчал.", nil)); len(got) != 0 {
			t.Errorf("Markers без предлога = %v, want пусто", got)
		}

		dir := t.TempDir()
		if _, err := deepread.LoadTimeLexicon(dir); err == nil {
			t.Error("отсутствующий detectors_time.json должен вернуть ошибку")
		}
		empty := filepath.Join(dir, "detectors_time.json")
		if err := os.WriteFile(empty, []byte(`{"units":{},"numerals":{},"prepositions":[]}`), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := deepread.LoadTimeLexicon(dir); err == nil {
			t.Error("пустые единицы времени должны вернуть ошибку")
		}
	})

	t.Run("места и сцены", func(t *testing.T) {
		lit, bookID := newTestLiterary(t)
		world := newTestWorld(t)

		texts := []string{
			"Золотой дворец стоял на холме. Иван молчал от страха.",
			"Золотого дворца сияли окна. Марья ждала.",
			"Спустя день Золотой дворец опустел. Иван вышел.",
			"Золотой дворец затих. Марья слушала.",
			"Тёмный лес шумел вокруг. Иван бежал.",
			"Тёмный лес молчал. Марья стояла.",
		}
		chunkIDs := insertSceneChunks(t, lit, world, bookID, texts)

		chunks := make([]deepread.SalienceChunk, len(texts))
		for i, id := range chunkIDs {
			chunks[i] = sceneChunk(id, texts[i], sceneWords)
		}
		groups := []deepread.CharacterGroup{
			{ID: 1, Name: "Иван", Windows: sceneWindows(chunkIDs, 4)},
			{ID: 2, Name: "Марья", Windows: sceneWindows(chunkIDs, 3)},
		}

		set, err := deepread.BuildScenes(lit, world, deepread.SceneInput{
			BookID:     bookID,
			Chunks:     chunks,
			Tags:       sceneTags(chunkIDs),
			Characters: groups,
			Places:     dicts,
			Emotion:    mood,
			Time:       clock,
		})
		if err != nil {
			t.Fatalf("BuildScenes: %v", err)
		}

		// Места: «Золотой дворец» с алиасом повторного падежа и «Тёмный лес».
		if len(set.Places) != 2 {
			t.Fatalf("мест %d, want 2: %+v", len(set.Places), set.Places)
		}
		palace := findPlace(t, set.Places, "золотой дворец")
		if palace.Name != "Золотой дворец" || palace.Kind != deepread.KeyArchitecture {
			t.Errorf("место = %+v, want «Золотой дворец» architecture", palace)
		}
		if len(palace.Aliases) != 1 || palace.Aliases[0] != "Золотого дворца" {
			t.Errorf("алиасы места = %v, want [Золотого дворца]", palace.Aliases)
		}
		if palace.Mentions != 4 || palace.FirstChunk != chunkIDs[0] || palace.LastChunk != chunkIDs[3] {
			t.Errorf("место = mentions %d, чанки %d..%d, want 4, %d..%d",
				palace.Mentions, palace.FirstChunk, palace.LastChunk, chunkIDs[0], chunkIDs[3])
		}
		forest := findPlace(t, set.Places, "тёмный лес")
		if forest.Kind != deepread.KeyNature || forest.Mentions != 2 {
			t.Errorf("место = %+v, want «Тёмный лес» nature, 2 упоминания", forest)
		}

		// Три сцены: граница по времени в третьем чанке, по месту — в пятом.
		if len(set.Scenes) != 3 {
			t.Fatalf("сцен %d, want 3: %+v", len(set.Scenes), set.Scenes)
		}
		first, second, third := set.Scenes[0], set.Scenes[1], set.Scenes[2]
		if first.FromChunk != chunkIDs[0] || first.ToChunk != chunkIDs[1] {
			t.Errorf("сцена 1 = чанки %d..%d, want %d..%d (граница по времени)",
				first.FromChunk, first.ToChunk, chunkIDs[0], chunkIDs[1])
		}
		if second.FromChunk != chunkIDs[2] || second.ToChunk != chunkIDs[3] {
			t.Errorf("сцена 2 = чанки %d..%d, want %d..%d",
				second.FromChunk, second.ToChunk, chunkIDs[2], chunkIDs[3])
		}
		if third.FromChunk != chunkIDs[4] || third.ToChunk != chunkIDs[5] {
			t.Errorf("сцена 3 = чанки %d..%d, want %d..%d (граница по месту)",
				third.FromChunk, third.ToChunk, chunkIDs[4], chunkIDs[5])
		}
		for i := 1; i < len(set.Scenes); i++ {
			if set.Scenes[i].FromChunk <= set.Scenes[i-1].ToChunk {
				t.Errorf("сцены %d и %d пересекаются по чанкам", i, i+1)
			}
			if set.Scenes[i].Ord != i {
				t.Errorf("ord сцены %d = %d", i, set.Scenes[i].Ord)
			}
		}

		if first.TimeMarker != "" {
			t.Errorf("маркер времени первой сцены = %q, want пусто", first.TimeMarker)
		}
		if second.TimeMarker != "Спустя день" {
			t.Errorf("маркер времени второй сцены = %q, want «Спустя день»", second.TimeMarker)
		}
		if third.TimeMarker != "Спустя день" {
			t.Errorf("маркер третьей сцены = %q, want перенос «Спустя день»", third.TimeMarker)
		}
		if first.PlaceKey != palace.Key || second.PlaceKey != palace.Key || third.PlaceKey != forest.Key {
			t.Errorf("места сцен = %q, %q, %q", first.PlaceKey, second.PlaceKey, third.PlaceKey)
		}

		wantParticipants := []string{"Иван", "Марья"}
		if !sameStrings(first.Participants, wantParticipants) {
			t.Errorf("участники первой сцены = %v, want %v", first.Participants, wantParticipants)
		}
		if len(second.Participants) != 2 || len(third.Participants) != 2 {
			t.Errorf("участники сцен 2 и 3 = %v / %v, want по два персонажа",
				second.Participants, third.Participants)
		}

		if !sameStrings(first.Tags, []string{deepread.KeyArchitecture}) {
			t.Errorf("теги первой сцены = %v, want [architecture]", first.Tags)
		}
		if !sameStrings(third.Tags, []string{deepread.KeyNature}) {
			t.Errorf("теги третьей сцены = %v, want [nature]", third.Tags)
		}

		share, ok := first.Emotion[mood.Mood]
		if !ok || math.Abs(share-0.5) > 1e-9 {
			t.Errorf("эмоции первой сцены = %v, want %s ≈ 0.5", first.Emotion, mood.Mood)
		}
		if got := second.Emotion[mood.Mood]; got != 0 {
			t.Errorf("эмоции второй сцены = %v, want 0", second.Emotion)
		}

		for i, scene := range set.Scenes {
			if len(scene.Centroid) != 1024 {
				t.Errorf("центроид сцены %d: dim %d, want 1024", i, len(scene.Centroid))
			}
		}

		// Спорная граница: смена места без смены времени и состава (шаг 3).
		if len(set.Disputed) != 1 {
			t.Fatalf("спорных границ %d, want 1: %+v", len(set.Disputed), set.Disputed)
		}
		dispute := set.Disputed[0]
		if dispute.FromChunk != chunkIDs[3] || dispute.ToChunk != chunkIDs[4] {
			t.Errorf("спорная граница = %d→%d, want %d→%d",
				dispute.FromChunk, dispute.ToChunk, chunkIDs[3], chunkIDs[4])
		}

		if err := deepread.WriteScenes(lit, set, "2026-11-14T00:00:00Z"); err != nil {
			t.Fatalf("WriteScenes: %v", err)
		}
		if n := countScenes(t, lit, bookID); n != 3 {
			t.Errorf("строк book_scene %d, want 3", n)
		}
		if n := countPlaces(t, lit, bookID); n != 2 {
			t.Errorf("строк book_place %d, want 2", n)
		}
		if n := countSceneVectors(t, lit); n != 3 {
			t.Errorf("векторов book_scene %d, want 3", n)
		}

		rows := readSceneRows(t, lit, bookID)
		if len(rows) != 3 {
			t.Fatalf("строк сцен %d, want 3", len(rows))
		}
		writtenPalace := findPlace(t, set.Places, palace.Key)
		if rows[0].placeID == nil || *rows[0].placeID != writtenPalace.ID {
			t.Errorf("place_id первой сцены = %v, want %d", rows[0].placeID, writtenPalace.ID)
		}
		if rows[0].timeMarker != "" || rows[1].timeMarker != "Спустя день" {
			t.Errorf("time_marker = %q / %q", rows[0].timeMarker, rows[1].timeMarker)
		}
		var participants, tags []string
		if err := json.Unmarshal([]byte(rows[0].participants), &participants); err != nil {
			t.Fatalf("participants %q: %v", rows[0].participants, err)
		}
		if !sameStrings(participants, []string{"Иван", "Марья"}) {
			t.Errorf("participants в базе = %v", participants)
		}
		if err := json.Unmarshal([]byte(rows[0].tags), &tags); err != nil {
			t.Fatalf("tags %q: %v", rows[0].tags, err)
		}
		if !sameStrings(tags, []string{deepread.KeyArchitecture}) {
			t.Errorf("tags в базе = %v", tags)
		}
		var profile map[string]float64
		if err := json.Unmarshal([]byte(rows[0].emotion), &profile); err != nil {
			t.Fatalf("emotion %q: %v", rows[0].emotion, err)
		}
		if value := profile[mood.Mood]; math.Abs(value-0.5) > 1e-9 {
			t.Errorf("emotion в базе = %v, want %s ≈ 0.5", profile, mood.Mood)
		}

		blob, source := readSceneVector(t, lit, set.Scenes[0].ID)
		if !strings.Contains(source, "чанки") {
			t.Errorf("source_text вектора сцены = %q", source)
		}
		if len(blob) != 1024*4 {
			t.Fatalf("BLOB вектора %d байт, want %d", len(blob), 1024*4)
		}
		for i := 0; i < 1024; i++ {
			got := math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
			if got != first.Centroid[i] {
				t.Fatalf("компонент вектора %d = %g, want %g", i, got, first.Centroid[i])
			}
		}

		// Повторный прогон идемпотентен: строки и векторы не плодятся.
		if err := deepread.WriteScenes(lit, set, "2026-11-14T00:00:00Z"); err != nil {
			t.Fatalf("повторный WriteScenes: %v", err)
		}
		if n := countScenes(t, lit, bookID); n != 3 {
			t.Errorf("после повтора строк book_scene %d, want 3", n)
		}
		if n := countPlaces(t, lit, bookID); n != 2 {
			t.Errorf("после повтора строк book_place %d, want 2", n)
		}
		if n := countSceneVectors(t, lit); n != 3 {
			t.Errorf("после повтора векторов book_scene %d, want 3", n)
		}
	})

	t.Run("место без имени", func(t *testing.T) {
		lit, bookID := newTestLiterary(t)
		world := newTestWorld(t)

		texts := []string{
			"Дворец стоял на холме. Иван молчал.",
			"Лес шумел вокруг. Марья ждала.",
		}
		chunkIDs := insertSceneChunks(t, lit, world, bookID, texts)
		chunks := make([]deepread.SalienceChunk, len(texts))
		for i, id := range chunkIDs {
			chunks[i] = sceneChunk(id, texts[i], sceneWords)
		}

		set, err := deepread.BuildScenes(lit, world, deepread.SceneInput{
			BookID:  bookID,
			Chunks:  chunks,
			Places:  dicts,
			Emotion: mood,
			Time:    clock,
		})
		if err != nil {
			t.Fatalf("BuildScenes: %v", err)
		}
		if len(set.Places) != 0 {
			t.Errorf("мест без имени %d, want 0: %+v", len(set.Places), set.Places)
		}
		if len(set.Scenes) != 1 {
			t.Fatalf("сцен %d, want 1", len(set.Scenes))
		}
		if set.Scenes[0].PlaceKey != "" || set.Scenes[0].PlaceName != "" {
			t.Errorf("место сцены = %q/%q, want пусто", set.Scenes[0].PlaceKey, set.Scenes[0].PlaceName)
		}

		if err := deepread.WriteScenes(lit, set, "2026-11-14T00:00:00Z"); err != nil {
			t.Fatalf("WriteScenes: %v", err)
		}
		if n := countPlaces(t, lit, bookID); n != 0 {
			t.Errorf("строк book_place без имён %d, want 0", n)
		}
		rows := readSceneRows(t, lit, bookID)
		if len(rows) != 1 || rows[0].placeID != nil {
			t.Errorf("place_id сцены = %v, want NULL", rows[0].placeID)
		}
	})

	t.Run("смена состава", func(t *testing.T) {
		lit, bookID := newTestLiterary(t)
		world := newTestWorld(t)

		texts := []string{
			"Золотой дворец стоял. Иван молчал.",
			"Золотой дворец сиял. Иван ждал.",
			"Золотой дворец гудел. Пётр вышел.",
			"Золотой дворец затих. Пётр слушал.",
		}
		chunkIDs := insertSceneChunks(t, lit, world, bookID, texts)
		chunks := make([]deepread.SalienceChunk, len(texts))
		for i, id := range chunkIDs {
			chunks[i] = sceneChunk(id, texts[i], sceneWords)
		}
		groups := []deepread.CharacterGroup{
			{ID: 1, Name: "Иван", Windows: sceneWindows(chunkIDs, 4)},
			{ID: 2, Name: "Пётр", Windows: sceneWindows(chunkIDs[2:], 4)},
		}

		set, err := deepread.BuildScenes(lit, world, deepread.SceneInput{
			BookID:     bookID,
			Chunks:     chunks,
			Characters: groups,
			Places:     dicts,
			Emotion:    mood,
			Time:       clock,
		})
		if err != nil {
			t.Fatalf("BuildScenes: %v", err)
		}
		if len(set.Scenes) != 2 {
			t.Fatalf("сцен %d, want 2: %+v", len(set.Scenes), set.Scenes)
		}
		if set.Scenes[0].ToChunk != chunkIDs[1] || set.Scenes[1].FromChunk != chunkIDs[2] {
			t.Errorf("граница = %d→%d, want %d→%d (появление участника)",
				set.Scenes[0].ToChunk, set.Scenes[1].FromChunk, chunkIDs[1], chunkIDs[2])
		}
		if !sameStrings(set.Scenes[0].Participants, []string{"Иван"}) {
			t.Errorf("участники первой сцены = %v, want [Иван]", set.Scenes[0].Participants)
		}
		if !sameStrings(set.Scenes[1].Participants, []string{"Иван", "Пётр"}) {
			t.Errorf("участники второй сцены = %v, want [Иван Пётр]", set.Scenes[1].Participants)
		}
		if len(set.Disputed) != 0 {
			t.Errorf("спорных границ %d, want 0", len(set.Disputed))
		}
	})

	t.Run("ошибки", func(t *testing.T) {
		if _, err := deepread.BuildScenes(nil, nil, deepread.SceneInput{}); !errors.Is(err, deepread.ErrNoStore) {
			t.Errorf("BuildScenes(nil) = %v, want ErrNoStore", err)
		}
		if err := deepread.WriteScenes(nil, nil, ""); !errors.Is(err, deepread.ErrNoStore) {
			t.Errorf("WriteScenes(nil) = %v, want ErrNoStore", err)
		}

		lit, bookID := newTestLiterary(t)
		world := newTestWorld(t)
		base := deepread.SceneInput{
			BookID:  bookID,
			Places:  dicts,
			Emotion: mood,
			Time:    clock,
		}
		if _, err := deepread.BuildScenes(lit, world, deepread.SceneInput{Places: dicts, Time: clock}); err == nil {
			t.Error("BuildScenes без книги должен вернуть ошибку")
		}
		noTime := base
		noTime.Time = nil
		if _, err := deepread.BuildScenes(lit, world, noTime); err == nil {
			t.Error("BuildScenes без словаря времени должен вернуть ошибку")
		}
		noPlace := base
		noPlace.Places = nil
		if _, err := deepread.BuildScenes(lit, world, noPlace); err == nil {
			t.Error("BuildScenes без словарей места должен вернуть ошибку")
		}
		dup := base
		dup.Chunks = []deepread.SalienceChunk{{ID: 7}, {ID: 7}}
		if _, err := deepread.BuildScenes(lit, world, dup); err == nil {
			t.Error("повтор id чанков должен вернуть ошибку")
		}

		if err := deepread.WriteScenes(lit, &deepread.BookScenes{BookID: 0}, ""); err == nil {
			t.Error("WriteScenes без книги должен вернуть ошибку")
		}
		bad := &deepread.BookScenes{BookID: bookID, Scenes: []deepread.Scene{{Ord: 0, FromChunk: 5, ToChunk: 2}}}
		if err := deepread.WriteScenes(lit, bad, ""); err == nil {
			t.Error("сцена с обратным диапазоном чанков должна вернуть ошибку")
		}
		overlap := &deepread.BookScenes{BookID: bookID, Scenes: []deepread.Scene{
			{Ord: 0, FromChunk: 1, ToChunk: 3},
			{Ord: 1, FromChunk: 3, ToChunk: 5},
		}}
		if err := deepread.WriteScenes(lit, overlap, ""); err == nil {
			t.Error("пересечение сцен по чанкам должно вернуть ошибку")
		}
		missingPlace := &deepread.BookScenes{BookID: bookID, Scenes: []deepread.Scene{
			{Ord: 0, FromChunk: 1, ToChunk: 2, PlaceKey: "нет такого"},
		}}
		if err := deepread.WriteScenes(lit, missingPlace, ""); err == nil {
			t.Error("ссылка на отсутствующее место должна вернуть ошибку")
		}
	})
}

// sceneWord — лемма и часть речи слова фикстуры.
type sceneWord struct {
	lemma string
	pos   string
}

// sceneWords — разбор слов фикстуры: падежи места, прилагательные имён,
// предлог и единица времени, лемма эмоции.
var sceneWords = map[string]sceneWord{
	"золотой":  {lemma: "золотой", pos: "ADJ"},
	"золотого": {lemma: "золотой", pos: "ADJ"},
	"дворец":   {lemma: "дворец", pos: "NOUN"},
	"дворца":   {lemma: "дворец", pos: "NOUN"},
	"тёмный":   {lemma: "тёмный", pos: "ADJ"},
	"лес":      {lemma: "лес", pos: "NOUN"},
	"спустя":   {lemma: "спустя", pos: "PREP"},
	"день":     {lemma: "день", pos: "NOUN"},
	"дня":      {lemma: "день", pos: "NOUN"},
	"страха":   {lemma: "страх", pos: "NOUN"},
}

// sceneChunk собирает чанк фикстуры: слова текста получают леммы и части речи
// из words, неизвестные слова остаются существительными.
func sceneChunk(id int64, text string, words map[string]sceneWord) deepread.SalienceChunk {
	chunk := deepread.SalienceChunk{ID: id, Text: text}
	index := 0
	for lineNo, line := range strings.Split(text, "\n") {
		for _, raw := range strings.Fields(line) {
			form := strings.TrimFunc(raw, func(r rune) bool {
				return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
			})
			if form == "" {
				continue
			}
			word := words[strings.ToLower(form)]
			if word.lemma == "" {
				word.lemma = strings.ToLower(form)
			}
			if word.pos == "" {
				word.pos = "NOUN"
			}
			chunk.Lemmas = append(chunk.Lemmas, deepread.Lemma{
				Form:  form,
				Lemma: word.lemma,
				POS:   word.pos,
				Line:  lineNo + 1,
				Index: index,
			})
			index++
		}
	}
	return chunk
}

// sceneWindows — окна упоминаний персонажа по всем чанкам фикстуры.
func sceneWindows(chunkIDs []int64, mentions int64) []deepread.CharacterWindow {
	out := make([]deepread.CharacterWindow, len(chunkIDs))
	for i, id := range chunkIDs {
		out[i] = deepread.CharacterWindow{ChunkID: id, Mentions: mentions}
	}
	return out
}

// sceneTags — теги чанков фикстуры: архитектура в первых четырёх, природа в
// двух последних.
func sceneTags(chunkIDs []int64) []deepread.ChunkTags {
	out := make([]deepread.ChunkTags, len(chunkIDs))
	for i, id := range chunkIDs {
		var tags []deepread.ChunkTag
		if i < len(chunkIDs)-2 {
			tags = []deepread.ChunkTag{
				{Key: deepread.KeyArchitecture, Score: 0.8, Band: deepread.BandConfident},
				{Key: deepread.KeyNature, Score: 0.05, Band: deepread.BandReject},
			}
		} else {
			tags = []deepread.ChunkTag{
				{Key: deepread.KeyNature, Score: 0.9, Band: deepread.BandConfident},
				{Key: deepread.KeyArchitecture, Score: 0.02, Band: deepread.BandReject},
			}
		}
		out[i] = deepread.ChunkTags{ChunkID: id, Tags: tags}
	}
	return out
}

// insertSceneChunks раскладывает чанки книги, записи банка ковки и векторы
// чанков в оперативной базе.
func insertSceneChunks(t *testing.T, lit, world *db.Handle, bookID int64, texts []string) []int64 {
	t.Helper()
	chunkIDs := make([]int64, len(texts))
	for i, text := range texts {
		res, err := lit.Exec("book_chunk",
			`INSERT INTO book_chunk (book_id, ord, chapter, text_hash) VALUES (?, ?, '1', ?)`,
			bookID, i, fmt.Sprintf("h%d", i))
		if err != nil {
			t.Fatalf("INSERT book_chunk %d: %v", i, err)
		}
		if chunkIDs[i], err = res.LastInsertId(); err != nil {
			t.Fatalf("book_chunk LastInsertId: %v", err)
		}

		res, err = lit.Exec("bank_pool",
			`INSERT INTO bank_pool (kind, text, lang, tags, trust_level, origin, created_at, book_id) VALUES ('routine', ?, 'ru', 't', 2, ?, '2026-01-01T00:00:00Z', ?)`,
			text, fmt.Sprintf("forge-book:%d:%d", bookID, i), bookID)
		if err != nil {
			t.Fatalf("INSERT bank_pool %d: %v", i, err)
		}
		bankID, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("bank_pool LastInsertId: %v", err)
		}
		vector := testVector(map[int]float32{0: float32(i+1) / 10, 1: 1})
		if _, err := world.Exec("vectors",
			`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES ('bank_pool', ?, 'bge-m3', 1024, 'ru', ?, ?, '2026-01-01T00:00:00Z')`,
			bankID, text, testVectorBlob(vector)); err != nil {
			t.Fatalf("INSERT vectors чанка %d: %v", i, err)
		}
	}
	return chunkIDs
}

// findPlace ищет место по ключу.
func findPlace(t *testing.T, places []deepread.Place, key string) deepread.Place {
	t.Helper()
	for _, place := range places {
		if place.Key == key {
			return place
		}
	}
	t.Fatalf("место %q не найдено: %+v", key, places)
	return deepread.Place{}
}

// sameStrings сравнивает списки строк.
func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// sceneRow — поля строки book_scene, которые проверяет гейт.
type sceneRow struct {
	ord          int
	fromChunk    int64
	toChunk      int64
	placeID      *int64
	timeMarker   string
	participants string
	tags         string
	emotion      string
}

// readSceneRows читает сцены книги в порядке ord.
func readSceneRows(t *testing.T, lit *db.Handle, bookID int64) []sceneRow {
	t.Helper()
	rows, err := lit.Query(
		`SELECT ord, from_chunk, to_chunk, place_id, time_marker, participants, tags, emotion
		 FROM book_scene WHERE book_id = ? ORDER BY ord`, bookID)
	if err != nil {
		t.Fatalf("SELECT book_scene: %v", err)
	}
	defer rows.Close()

	var out []sceneRow
	for rows.Next() {
		var row sceneRow
		if err := rows.Scan(&row.ord, &row.fromChunk, &row.toChunk, &row.placeID,
			&row.timeMarker, &row.participants, &row.tags, &row.emotion); err != nil {
			t.Fatalf("Scan book_scene: %v", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows book_scene: %v", err)
	}
	return out
}

// countScenes считает сцены книги.
func countScenes(t *testing.T, lit *db.Handle, bookID int64) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM book_scene WHERE book_id = ?`, bookID).Scan(&n); err != nil {
		t.Fatalf("COUNT book_scene: %v", err)
	}
	return n
}

// countPlaces считает места книги.
func countPlaces(t *testing.T, lit *db.Handle, bookID int64) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM book_place WHERE book_id = ?`, bookID).Scan(&n); err != nil {
		t.Fatalf("COUNT book_place: %v", err)
	}
	return n
}

// countSceneVectors считает векторы owner_table='book_scene'.
func countSceneVectors(t *testing.T, lit *db.Handle) int {
	t.Helper()
	var n int
	if err := lit.QueryRow(`SELECT COUNT(*) FROM vectors WHERE owner_table = ?`,
		deepread.SceneVectorTable).Scan(&n); err != nil {
		t.Fatalf("COUNT vectors сцен: %v", err)
	}
	return n
}

// readSceneVector читает BLOB и source_text вектора сцены.
func readSceneVector(t *testing.T, lit *db.Handle, sceneID int64) ([]byte, string) {
	t.Helper()
	var blob []byte
	var source string
	if err := lit.QueryRow(
		`SELECT source_text, vec FROM vectors WHERE owner_table = ? AND owner_id = ?`,
		deepread.SceneVectorTable, sceneID).Scan(&source, &blob); err != nil {
		t.Fatalf("SELECT вектор сцены %d: %v", sceneID, err)
	}
	return blob, source
}
