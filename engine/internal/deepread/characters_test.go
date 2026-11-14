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

	"tns65/internal/db"
	"tns65/internal/deepread"
)

// TestCharacterCentroids — гейт персонажей (W-25, приёмка T-14-07): окна
// упоминаний из карты P11, центроид векторов окон в literary.vectors с
// owner_table='book_character', слияние алиасов по косинусу центроидов, профиль
// эмоций с долями в 0..1 и сохранение существующих данных строки карты.
func TestCharacterCentroids(t *testing.T) {
	root := findEngineRoot()
	if root == "" {
		t.Fatal("корень движка не найден")
	}

	mood, err := deepread.LoadEmotionDictionary(filepath.Join(root, "packs", "ru", "detectors_mood.json"))
	if err != nil {
		t.Fatalf("LoadEmotionDictionary: %v", err)
	}
	if mood.Mood == "" || len(mood.Lemmas) == 0 {
		t.Fatalf("словарь emotion пуст: %+v", mood)
	}

	cfg, err := deepread.LoadCharacterConfig(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatalf("LoadCharacterConfig(engine/config.yaml): %v", err)
	}
	if math.Abs(cfg.AliasCosine-0.85) > 1e-9 {
		t.Fatalf("alias_cosine config.yaml = %g, ожидалось 0.85", cfg.AliasCosine)
	}

	t.Run("порог и словарь", func(t *testing.T) {
		if def := deepread.DefaultCharacterConfig(); math.Abs(def.AliasCosine-0.85) > 1e-9 {
			t.Errorf("DefaultCharacterConfig = %g, want 0.85", def.AliasCosine)
		}

		dir := t.TempDir()
		bad := filepath.Join(dir, "bad.yaml")
		if err := os.WriteFile(bad, []byte("deepread:\n  alias_cosine: 1.5\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := deepread.LoadCharacterConfig(bad); err == nil {
			t.Error("alias_cosine 1.5 должен вернуть ошибку")
		}
		if _, err := deepread.LoadCharacterConfig(filepath.Join(dir, "none.yaml")); err == nil {
			t.Error("отсутствующий конфиг должен вернуть ошибку")
		}

		empty := filepath.Join(dir, "mood.json")
		if err := os.WriteFile(empty, []byte(`{"mood":"","lemmas":[]}`), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := deepread.LoadEmotionDictionary(empty); err == nil {
			t.Error("пустой detectors_mood.json должен вернуть ошибку")
		}
	})

	t.Run("центроид окон", func(t *testing.T) {
		windows := []deepread.CharacterWindow{
			{ChunkID: 1, Vector: []float32{1, 2}},
			{ChunkID: 2, Vector: []float32{3, 4}},
			{ChunkID: 3},
		}
		got := deepread.Centroid(windows)
		if len(got) != 2 || math.Abs(float64(got[0])-2) > 1e-6 || math.Abs(float64(got[1])-3) > 1e-6 {
			t.Errorf("Centroid = %v, want [2 3]", got)
		}
		if deepread.Centroid(nil) != nil {
			t.Error("Centroid без векторов должен быть nil")
		}
		if c := deepread.Cosine([]float32{1, 0}, []float32{0, 1}); math.Abs(c) > 1e-9 {
			t.Errorf("Cosine ортогональных = %g, want 0", c)
		}
		if c := deepread.Cosine([]float32{1, 0}, []float32{2, 0}); math.Abs(c-1) > 1e-9 {
			t.Errorf("Cosine коллинеарных = %g, want 1", c)
		}
	})

	t.Run("слияние и запись", func(t *testing.T) {
		lit, bookID := newTestLiterary(t)
		world := newTestWorld(t)
		fix := newCharacterFixture(t, lit, world, bookID)

		groups, disputed, err := deepread.BuildCharacters(lit, world, deepread.CharacterInput{
			BookID:  bookID,
			Chunks:  fix.chunks,
			Emotion: mood,
			Config:  cfg,
		})
		if err != nil {
			t.Fatalf("BuildCharacters: %v", err)
		}
		if len(disputed) != 0 {
			t.Errorf("спорных пар %d, ожидалось 0: %+v", len(disputed), disputed)
		}
		if len(groups) != 2 {
			t.Fatalf("групп %d, ожидалось 2: %+v", len(groups), groups)
		}

		// Старшая запись — «Снейп»: больше упоминаний, алиас — имя младшей.
		merged := groups[0]
		if merged.ID != fix.character["Снейп"] || merged.Name != "Снейп" {
			t.Errorf("старшая запись = (%d, %q), want Снейп", merged.ID, merged.Name)
		}
		if len(merged.Aliases) != 1 || !strings.EqualFold(merged.Aliases[0], "профессор") {
			t.Errorf("алиасы слитой записи = %v, want [профессор]", merged.Aliases)
		}
		if len(merged.Members) != 2 {
			t.Errorf("участников %d, want 2", len(merged.Members))
		}
		if len(merged.Windows) != 3 {
			t.Errorf("окон %d, want 3 (общие чанки без повторов)", len(merged.Windows))
		}
		if len(merged.Centroid) != 1024 {
			t.Errorf("центроид dim %d, want 1024", len(merged.Centroid))
		}

		toponym := groups[1]
		if toponym.ID != fix.character["Хогвартс"] || len(toponym.Members) != 1 {
			t.Errorf("вторая группа = %+v, want одиночный «Хогвартс»", toponym)
		}

		// Эмоциональный профиль: «Страх» только в первом окне слитой записи.
		share, ok := merged.Emotion[mood.Mood]
		if !ok {
			t.Fatalf("в профиле нет категории %q: %v", mood.Mood, merged.Emotion)
		}
		if want := 2.0 / 7.0; math.Abs(share-want) > 1e-6 {
			t.Errorf("доля эмоции = %g, want %g", share, want)
		}
		for category, value := range merged.Emotion {
			if value < 0 || value > 1 {
				t.Errorf("доля категории %q = %g вне 0..1", category, value)
			}
		}
		for category, value := range toponym.Emotion {
			if value < 0 || value > 1 {
				t.Errorf("доля категории %q топонима = %g вне 0..1", category, value)
			}
		}

		// Векторы: ровно один центроид на слитую запись, у младшей — ни одного.
		if err := deepread.WriteCharacterCentroids(lit, groups, "2026-11-14T00:00:00Z"); err != nil {
			t.Fatalf("WriteCharacterCentroids: %v", err)
		}
		if n := countCharacterVectors(t, lit); n != 2 {
			t.Fatalf("векторов book_character %d, ожидалось 2", n)
		}
		if n := countCharacterVectors(t, lit, merged.ID); n != 1 {
			t.Errorf("векторов старшей записи %d, want 1", n)
		}
		if n := countCharacterVectors(t, lit, fix.character["профессор"]); n != 0 {
			t.Errorf("векторов младшей записи %d, want 0 (вектор имени не строится)", n)
		}

		var source string
		var blob []byte
		if err := lit.QueryRow(
			`SELECT source_text, vec FROM vectors WHERE owner_table = ? AND owner_id = ?`,
			deepread.CharacterVectorTable, merged.ID).Scan(&source, &blob); err != nil {
			t.Fatalf("SELECT вектор персонажа: %v", err)
		}
		for _, name := range []string{"Снейп", "профессор", "Хогвартс"} {
			if strings.Contains(source, name) {
				t.Errorf("source_text вектора содержит имя %q: %q", name, source)
			}
		}
		if !strings.Contains(source, "чанки") {
			t.Errorf("source_text вектора не описывает окна: %q", source)
		}
		if len(blob) != 1024*4 {
			t.Fatalf("BLOB вектора %d байт, want %d", len(blob), 1024*4)
		}
		for i := 0; i < 1024; i++ {
			got := math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
			if got != merged.Centroid[i] {
				t.Fatalf("компонент вектора %d = %g, want %g", i, got, merged.Centroid[i])
			}
		}

		// Внешность «Хогвартса» заполнена заранее — профиль её не затирает.
		if _, err := lit.Exec("book_character",
			`UPDATE book_character SET appearance = ? WHERE id = ?`, "Каменный замок", toponym.ID); err != nil {
			t.Fatalf("UPDATE appearance: %v", err)
		}
		if err := deepread.WriteCharacterProfiles(lit, groups); err != nil {
			t.Fatalf("WriteCharacterProfiles: %v", err)
		}

		row := readCharacterRow(t, lit, merged.ID)
		var aliases []string
		if err := json.Unmarshal([]byte(row.aliases), &aliases); err != nil {
			t.Fatalf("алиасы в базе %q: %v", row.aliases, err)
		}
		if len(aliases) != 1 || aliases[0] != "профессор" {
			t.Errorf("алиасы в базе = %v, want [профессор]", aliases)
		}
		if row.mentions != 10 || row.first != 1 || row.last != 3 {
			t.Errorf("строка карты = mentions %d, чанки %d..%d, want 10, 1..3",
				row.mentions, row.first, row.last)
		}
		var profile map[string]float64
		if err := json.Unmarshal([]byte(row.appearance), &profile); err != nil {
			t.Fatalf("профиль в appearance %q: %v", row.appearance, err)
		}
		if value, ok := profile[mood.Mood]; !ok || math.Abs(value-2.0/7.0) > 1e-6 {
			t.Errorf("профиль в базе = %v, want %s ≈ %g", profile, mood.Mood, 2.0/7.0)
		}
		if value := profile[mood.Mood]; value < 0 || value > 1 {
			t.Errorf("доля эмоции в базе %g вне 0..1", value)
		}

		if got := readCharacterRow(t, lit, toponym.ID); got.appearance != "Каменный замок" {
			t.Errorf("внешность топонима затёрта: %q", got.appearance)
		}
		if got := readCharacterRow(t, lit, fix.character["профессор"]); got.appearance != "" {
			t.Errorf("младшая запись тронута: appearance %q", got.appearance)
		}

		// Повторный проход идемпотентен: векторы и алиасы не плодятся.
		if err := deepread.WriteCharacterCentroids(lit, groups, "2026-11-14T00:00:00Z"); err != nil {
			t.Fatalf("повторный WriteCharacterCentroids: %v", err)
		}
		if err := deepread.WriteCharacterProfiles(lit, groups); err != nil {
			t.Fatalf("повторный WriteCharacterProfiles: %v", err)
		}
		if n := countCharacterVectors(t, lit); n != 2 {
			t.Errorf("после повтора векторов %d, want 2", n)
		}
		again := readCharacterRow(t, lit, merged.ID)
		if again.aliases != row.aliases || again.appearance != row.appearance {
			t.Errorf("повтор изменил строку: %+v → %+v", row, again)
		}
	})

	t.Run("смена порога разъединяет", func(t *testing.T) {
		lit, bookID := newTestLiterary(t)
		world := newTestWorld(t)
		fix := newCharacterFixture(t, lit, world, bookID)

		strict := deepread.CharacterConfig{AliasCosine: 0.99}
		groups, disputed, err := deepread.BuildCharacters(lit, world, deepread.CharacterInput{
			BookID:  bookID,
			Chunks:  fix.chunks,
			Emotion: mood,
			Config:  strict,
		})
		if err != nil {
			t.Fatalf("BuildCharacters(0.99): %v", err)
		}
		if len(groups) != 3 {
			t.Errorf("при пороге 0.99 групп %d, want 3", len(groups))
		}
		if len(disputed) != 0 {
			t.Errorf("при пороге 0.99 спорных пар %d, want 0", len(disputed))
		}
	})

	t.Run("пограничная пара", func(t *testing.T) {
		near := func(cos float64) []float32 {
			return []float32{float32(cos), float32(math.Sqrt(1 - cos*cos)), 0}
		}
		cands := []deepread.CharacterCandidate{
			{ID: 1, Name: "А", Mentions: 3, Windows: []deepread.CharacterWindow{{ChunkID: 10, Mentions: 3, Vector: near(1)}}},
			{ID: 2, Name: "Б", Mentions: 2, Windows: []deepread.CharacterWindow{{ChunkID: 10, Mentions: 2, Vector: near(0.88)}}},
			{ID: 3, Name: "В", Mentions: 1, Windows: []deepread.CharacterWindow{{ChunkID: 10, Mentions: 1, Vector: near(0.3)}}},
		}

		groups, disputed, err := deepread.MergeCharacters(cands, mood, cfg)
		if err != nil {
			t.Fatalf("MergeCharacters: %v", err)
		}
		if len(groups) != 3 {
			t.Errorf("групп %d, want 3 (пограничная пара не слита)", len(groups))
		}
		if len(disputed) != 1 {
			t.Fatalf("спорных пар %d, want 1: %+v", len(disputed), disputed)
		}
		if disputed[0].SeniorID != 1 || disputed[0].JuniorID != 2 {
			t.Errorf("спорная пара = %+v, want 1→2", disputed[0])
		}
		if math.Abs(disputed[0].Cosine-0.88) > 1e-6 {
			t.Errorf("косинус спорной пары = %g, want 0.88", disputed[0].Cosine)
		}

		// Порог ниже косинуса вне полосы ±0.05 — пара сливается.
		loose := deepread.CharacterConfig{AliasCosine: 0.8}
		groups, disputed, err = deepread.MergeCharacters(cands, mood, loose)
		if err != nil {
			t.Fatalf("MergeCharacters(0.8): %v", err)
		}
		if len(groups) != 2 {
			t.Errorf("при пороге 0.8 групп %d, want 2", len(groups))
		}
		if len(disputed) != 0 {
			t.Errorf("при пороге 0.8 спорных пар %d, want 0", len(disputed))
		}
	})

	t.Run("ошибки", func(t *testing.T) {
		if err := deepread.WriteCharacterCentroids(nil, nil, ""); !errors.Is(err, deepread.ErrNoStore) {
			t.Errorf("WriteCharacterCentroids(nil) = %v, want ErrNoStore", err)
		}
		if err := deepread.WriteCharacterProfiles(nil, nil); !errors.Is(err, deepread.ErrNoStore) {
			t.Errorf("WriteCharacterProfiles(nil) = %v, want ErrNoStore", err)
		}
		if _, err := deepread.LoadCharacterCandidates(nil, 1); !errors.Is(err, deepread.ErrNoStore) {
			t.Errorf("LoadCharacterCandidates(nil) = %v, want ErrNoStore", err)
		}

		lit, _ := newTestLiterary(t)
		if _, err := deepread.LoadCharacterCandidates(lit, 0); err == nil {
			t.Error("LoadCharacterCandidates без книги должен вернуть ошибку")
		}

		badCentroid := []deepread.CharacterGroup{{
			ID: 1, Name: "А", Centroid: []float32{1, 2, 3},
		}}
		if err := deepread.WriteCharacterCentroids(lit, badCentroid, ""); err == nil {
			t.Error("центроид dim=3 должен вернуть ошибку")
		}
		if err := deepread.WriteCharacterProfiles(lit, []deepread.CharacterGroup{{ID: 999}}); err == nil {
			t.Error("профиль отсутствующего персонажа должен вернуть ошибку")
		}

		dup := []deepread.CharacterCandidate{{ID: 1, Name: "А"}, {ID: 1, Name: "Б"}}
		if _, _, err := deepread.MergeCharacters(dup, mood, cfg); err == nil {
			t.Error("повтор id кандидатов должен вернуть ошибку")
		}
		noID := []deepread.CharacterCandidate{{Name: "А"}}
		if _, _, err := deepread.MergeCharacters(noID, mood, cfg); err == nil {
			t.Error("кандидат без id должен вернуть ошибку")
		}
	})
}

// characterFixture — книга из четырёх чанков с картой P11 и векторами ковки.
type characterFixture struct {
	bookID    int64
	chunks    []deepread.SalienceChunk
	character map[string]int64
}

// newCharacterFixture раскладывает чанки, записи банка ковки, векторы чанков в
// оперативной базе и трёх кандидатов карты: «Снейп» и «профессор» — одно лицо в
// общих чанках, «Хогвартс» — топоним с низким косинусом.
func newCharacterFixture(t *testing.T, lit, world *db.Handle, bookID int64) characterFixture {
	t.Helper()

	texts := []string{
		"Снейп вошёл в класс.\nСтрах сковал учеников.",
		"Профессор долго смотрел на карту.\nСнейп молчал.",
		"Профессор стоял у окна.\nХогвартс спал.",
		"Хогвартс шумел до утра.\nДвери хлопали.",
	}
	vectors := [][]float32{
		testVector(map[int]float32{0: 1, 1: 0.1}),
		testVector(map[int]float32{0: 1, 1: 0.12, 2: 0.05}),
		testVector(map[int]float32{0: 0.15, 1: 1, 2: 0.05}),
		testVector(map[int]float32{0: 0.1, 1: 0.12, 4: 1}),
	}

	fix := characterFixture{bookID: bookID, character: map[string]int64{}}
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
		fix.chunks = append(fix.chunks, fixtureChunk(chunkIDs[i], text, nil))

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
		if _, err := world.Exec("vectors",
			`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at) VALUES ('bank_pool', ?, 'bge-m3', 1024, 'ru', ?, ?, '2026-01-01T00:00:00Z')`,
			bankID, text, testVectorBlob(vectors[i])); err != nil {
			t.Fatalf("INSERT vectors чанка %d: %v", i, err)
		}
	}

	persons := []struct {
		name     string
		mentions map[int]int64
	}{
		{name: "Снейп", mentions: map[int]int64{0: 2, 1: 3}},
		{name: "профессор", mentions: map[int]int64{0: 1, 1: 2, 2: 2}},
		{name: "Хогвартс", mentions: map[int]int64{2: 4, 3: 1}},
	}
	for _, person := range persons {
		var first, last, total int64
		for ord, count := range person.mentions {
			if first == 0 || chunkIDs[ord] < first {
				first = chunkIDs[ord]
			}
			if chunkIDs[ord] > last {
				last = chunkIDs[ord]
			}
			total += count
		}
		res, err := lit.Exec("book_character",
			`INSERT INTO book_character (book_id, name, aliases, confidence, first_chunk, last_chunk, mentions, status, created_at) VALUES (?, ?, '[]', 0.5, ?, ?, ?, 'candidate', '2026-01-01T00:00:00Z')`,
			bookID, person.name, first, last, total)
		if err != nil {
			t.Fatalf("INSERT book_character %s: %v", person.name, err)
		}
		charID, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("book_character LastInsertId: %v", err)
		}
		fix.character[person.name] = charID
		for ord, count := range person.mentions {
			if err := insertMention(lit, charID, chunkIDs[ord], count); err != nil {
				t.Fatalf("INSERT mention %s: %v", person.name, err)
			}
		}
	}
	return fix
}

// insertMention пишет упоминание персонажа в чанке карты.
func insertMention(lit *db.Handle, characterID, chunkID, count int64) error {
	_, err := lit.Exec("book_character_mention",
		`INSERT INTO book_character_mention (character_id, chunk_id, count) VALUES (?, ?, ?)`,
		characterID, chunkID, count)
	return err
}

// testVector собирает вектор размерности 1024 из ненулевых компонент.
func testVector(base map[int]float32) []float32 {
	vec := make([]float32, 1024)
	for i, v := range base {
		vec[i] = v
	}
	return vec
}

// testVectorBlob кодирует вектор в BLOB Float32 little-endian (D-17.2).
func testVectorBlob(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, f := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// newTestWorld создаёт оперативную базу с таблицей vectors и хэндлом core.
func newTestWorld(t *testing.T) *db.Handle {
	t.Helper()
	path := filepath.Join(t.TempDir(), "world.db")
	raw, err := db.OpenWriter(path)
	if err != nil {
		t.Fatalf("OpenWriter(world): %v", err)
	}
	if err := db.Migrate(raw, "world", "shared"); err != nil {
		_ = raw.Close()
		t.Fatalf("Migrate(world, shared): %v", err)
	}
	_ = raw.Close()

	world, err := db.OpenWriterAs(path, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("OpenWriterAs(world): %v", err)
	}
	t.Cleanup(func() { _ = world.Close() })
	return world
}

// characterRow — поля карты персонажа, которые обновляет разбор.
type characterRow struct {
	aliases    string
	appearance string
	mentions   int64
	first      int64
	last       int64
}

// readCharacterRow читает обновляемые поля строки карты.
func readCharacterRow(t *testing.T, lit *db.Handle, characterID int64) characterRow {
	t.Helper()
	var row characterRow
	if err := lit.QueryRow(
		`SELECT aliases, appearance, mentions, first_chunk, last_chunk FROM book_character WHERE id = ?`,
		characterID).Scan(&row.aliases, &row.appearance, &row.mentions, &row.first, &row.last); err != nil {
		t.Fatalf("SELECT book_character %d: %v", characterID, err)
	}
	return row
}

// countCharacterVectors считает векторы owner_table='book_character'; ownerID > 0
// — только векторы одного персонажа.
func countCharacterVectors(t *testing.T, lit *db.Handle, ownerID ...int64) int {
	t.Helper()
	query := `SELECT COUNT(*) FROM vectors WHERE owner_table = ?`
	args := []any{deepread.CharacterVectorTable}
	if len(ownerID) > 0 {
		query += ` AND owner_id = ?`
		args = append(args, ownerID[0])
	}
	var n int
	if err := lit.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("COUNT vectors: %v", err)
	}
	return n
}
