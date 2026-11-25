package deepread

// bookmap.go — карта книги-источника (Э6, PROVIDER-ROUTING-PLAN §6.5, §12.6):
// отдельный проход картографа по таблицам разбора. Вход — только book_place и
// book_scene, проза книги повторно не читается; роль выбирается режимом походов
// мира, провайдера даёт маршрут sources_map. Карта ложится в литературную базу
// строкой forge_log kind='book_map' — новых таблиц проход не заводит.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"tns65/internal/db"
	"tns65/internal/prompts"
	"tns65/internal/version"
)

// KindBookMap — kind строки forge_log о карте книги-источника (писатель forge).
const KindBookMap = "book_map"

// Роли картографа по режиму походов (§6.5, §6.4): Летописец, Картограф
// вымысла, Штурман. Промпт и температура остаются у роли, маршрут выбирает
// только провайдера и модель.
const (
	MapChroniclerRole = "map-chronicler" // Летописец — реальный мир.
	MapFictionRole    = "map-fiction"    // Картограф вымысла — вымышленный мир.
	MapNavigatorRole  = "map-navigator"  // Штурман — смешанный мир.
)

// mapInstruction — задание картографу: свести выписки разбора, прозы нет.
const mapInstruction = "Сведи места и времена книги в карту книги-источника: области, эпохи, ориентиры и порядок пути. Прозы книги нет — опирайся только на выписки разбора."

// MapMode — режим походов мира для выбора роли картографа. Значения совпадают
// с полем анкеты travel_mode (paganel.TravelMode): пакет разбора не берёт тип
// атласа, иначе цикл импортов paganel → workshop → deepread.
type MapMode string

// Режимы походов карты (§6.5): реальная, вымышленная, смешанная.
const (
	MapModeReal    MapMode = "реальная"
	MapModeFiction MapMode = "вымышленная"
	MapModeMixed   MapMode = "смешанная"
)

var (
	// ErrMapEmpty — карта не строится: в таблицах разбора нет мест и сцен.
	ErrMapEmpty = errors.New("deepread: карта книги: нет мест и сцен разбора")
	// ErrMapNoSlot — слот картографа не подключён: проход пропускается.
	ErrMapNoSlot = errors.New("deepread: слот картографа не подключён")
)

// MapRole — роль картографа по режиму походов (§6.5): реальная — Летописец,
// вымышленная — Картограф вымысла, смешанная — Штурман.
func MapRole(mode MapMode) (string, error) {
	switch mode {
	case MapModeReal:
		return MapChroniclerRole, nil
	case MapModeFiction:
		return MapFictionRole, nil
	case MapModeMixed:
		return MapNavigatorRole, nil
	}
	return "", fmt.Errorf("deepread: режим походов %q вне списка реальная|вымышленная|смешанная", string(mode))
}

// MapPlace — место книги в карте: имя, вид, алиасы, упоминания, отрезок чанков
// и число сцен, где место ведущее. Поля — из строки book_place.
type MapPlace struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
	Mentions   int64    `json:"mentions"`
	FirstChunk int64    `json:"first_chunk"`
	LastChunk  int64    `json:"last_chunk"`
	Scenes     int      `json:"scenes"`
}

// MapScene — сцена книги в карте: место, маркер времени, состав, теги и
// диапазон чанков. Поля — из строки book_scene.
type MapScene struct {
	Ord          int      `json:"ord"`
	Place        string   `json:"place,omitempty"`
	TimeMarker   string   `json:"time_marker,omitempty"`
	Participants []string `json:"participants,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	FromChunk    int64    `json:"from_chunk"`
	ToChunk      int64    `json:"to_chunk"`
}

// BookMap — карта книги-источника: выписки разбора (места и времена) и текст
// картографа. Хранится JSON-объектом в forge_log kind='book_map'.
type BookMap struct {
	BookID  int64      `json:"book_id"`
	Mode    MapMode    `json:"mode"`
	Role    string     `json:"role"`
	Places  []MapPlace `json:"places"`
	Scenes  []MapScene `json:"scenes"`
	Text    string     `json:"text"`
	BuiltAt string     `json:"built_at"`
}

// MapSource — выписки разбора для карты: места и сцены книги из book_place и
// book_scene. Других таблиц проход не читает.
type MapSource struct {
	Places []MapPlace
	Scenes []MapScene
}

// MapSlot — слот картографа: один запрос одной модели, роль — по режиму
// походов. Подходит workshop.Slot (RoutedWorkshopSlot на маршруте
// sources_map) и testkit.FakeSlot.
type MapSlot interface {
	Generate(ctx context.Context, role, prompt string) (string, error)
}

// MapInput — вход прохода картографа: книга, режим походов мира, слот маршрута
// sources_map и момент сборки карты.
type MapInput struct {
	BookID int64
	Mode   MapMode
	Slot   MapSlot
	Now    string
}

// mapPrompt — документ запроса картографу: только выписки разбора, прозы нет.
type mapPrompt struct {
	Task        string     `json:"task"`
	Instruction string     `json:"instruction"`
	BookID      int64      `json:"book_id"`
	Mode        MapMode    `json:"mode"`
	Places      []MapPlace `json:"places"`
	Scenes      []MapScene `json:"scenes"`
}

// BuildBookMap строит карту книги-источника отдельным проходом картографа
// (Э6, §6.5/§12.6): вход — строки book_place и book_scene, проза не читается;
// роль выбирается режимом походов, провайдера даёт маршрут sources_map с
// каскадом на sources. Пустые таблицы — ErrMapEmpty, отказ слота (нет ключей,
// все провайдеры отказали) уходит вызывающему: карта не пишется, разбор книги
// от пропуска не страдает.
func BuildBookMap(ctx context.Context, lit *db.Handle, in MapInput) (*BookMap, error) {
	if lit == nil {
		return nil, ErrNoStore
	}
	if in.BookID <= 0 {
		return nil, fmt.Errorf("deepread: карта книги: book_id %d", in.BookID)
	}
	role, err := MapRole(in.Mode)
	if err != nil {
		return nil, err
	}
	if in.Slot == nil {
		return nil, ErrMapNoSlot
	}
	src, err := LoadMapSource(lit, in.BookID)
	if err != nil {
		return nil, err
	}
	if len(src.Places) == 0 && len(src.Scenes) == 0 {
		return nil, ErrMapEmpty
	}
	prompt, err := mapPromptText(role, in.BookID, in.Mode, src)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	text, err := in.Slot.Generate(ctx, role, prompt)
	if err != nil {
		return nil, fmt.Errorf("deepread: карта книги %d: %w", in.BookID, err)
	}

	now := in.Now
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	bm := &BookMap{
		BookID:  in.BookID,
		Mode:    in.Mode,
		Role:    role,
		Places:  src.Places,
		Scenes:  src.Scenes,
		Text:    strings.TrimSpace(text),
		BuiltAt: now,
	}
	if err := writeBookMap(lit, bm); err != nil {
		return nil, err
	}
	return bm, nil
}

// mapPromptText собирает промпт картографа из выписок разбора: JSON-документ
// без текста книги.
func mapPromptText(role string, bookID int64, mode MapMode, src *MapSource) (string, error) {
	raw, err := json.Marshal(mapPrompt{
		Task:        "book-map",
		Instruction: prompts.ForRole(role, "deepread.bookmap", mapInstruction),
		BookID:      bookID,
		Mode:        mode,
		Places:      src.Places,
		Scenes:      src.Scenes,
	})
	if err != nil {
		return "", fmt.Errorf("deepread: промпт карты книги %d: %w", bookID, err)
	}
	return string(raw), nil
}

// LoadMapSource читает выписки разбора книги: места из book_place и сцены из
// book_scene. Прозы в литературной базе нет, и проход её не запрашивает.
func LoadMapSource(lit *db.Handle, bookID int64) (*MapSource, error) {
	if lit == nil {
		return nil, ErrNoStore
	}
	if bookID <= 0 {
		return nil, fmt.Errorf("deepread: карта книги: book_id %d", bookID)
	}
	out := &MapSource{}
	places, index, err := loadMapPlaces(lit, bookID)
	if err != nil {
		return nil, err
	}
	out.Places = places
	if err := fillMapSceneCounts(lit, bookID, out.Places, index); err != nil {
		return nil, err
	}
	names := make(map[int64]string, len(index))
	for id, i := range index {
		names[id] = out.Places[i].Name
	}
	scenes, err := loadMapScenes(lit, bookID, names)
	if err != nil {
		return nil, err
	}
	out.Scenes = scenes
	return out, nil
}

// loadMapPlaces читает места книги в порядке первого появления; index —
// id строки book_place → индекс в срезе.
func loadMapPlaces(lit *db.Handle, bookID int64) ([]MapPlace, map[int64]int, error) {
	rows, err := lit.Query(
		`SELECT id, name, aliases, kind, first_chunk, last_chunk, mentions FROM book_place WHERE book_id = ? ORDER BY first_chunk, id`,
		bookID)
	if err != nil {
		return nil, nil, fmt.Errorf("deepread: места карты книги %d: %w", bookID, err)
	}
	defer rows.Close()

	var places []MapPlace
	index := make(map[int64]int)
	for rows.Next() {
		var (
			id                              int64
			name, aliases, kind             string
			firstChunk, lastChunk, mentions int64
		)
		if err := rows.Scan(&id, &name, &aliases, &kind, &firstChunk, &lastChunk, &mentions); err != nil {
			return nil, nil, fmt.Errorf("deepread: места карты книги %d: %w", bookID, err)
		}
		place := MapPlace{
			Name:       name,
			Kind:       kind,
			Mentions:   mentions,
			FirstChunk: firstChunk,
			LastChunk:  lastChunk,
		}
		if aliases != "" {
			if err := json.Unmarshal([]byte(aliases), &place.Aliases); err != nil {
				return nil, nil, fmt.Errorf("deepread: алиасы места %q: %w", name, err)
			}
		}
		index[id] = len(places)
		places = append(places, place)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("deepread: места карты книги %d: %w", bookID, err)
	}
	return places, index, nil
}

// fillMapSceneCounts считает сцены каждого места по book_scene.
func fillMapSceneCounts(lit *db.Handle, bookID int64, places []MapPlace, index map[int64]int) error {
	rows, err := lit.Query(
		`SELECT place_id, COUNT(*) FROM book_scene WHERE book_id = ? AND place_id IS NOT NULL GROUP BY place_id`,
		bookID)
	if err != nil {
		return fmt.Errorf("deepread: сцены мест карты книги %d: %w", bookID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			placeID int64
			count   int
		)
		if err := rows.Scan(&placeID, &count); err != nil {
			return fmt.Errorf("deepread: сцены мест карты книги %d: %w", bookID, err)
		}
		if i, ok := index[placeID]; ok {
			places[i].Scenes = count
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("deepread: сцены мест карты книги %d: %w", bookID, err)
	}
	return nil
}

// loadMapScenes читает сцены книги по ord; имя места берётся из book_place.
// names — id строки book_place → имя.
func loadMapScenes(lit *db.Handle, bookID int64, names map[int64]string) ([]MapScene, error) {
	rows, err := lit.Query(
		`SELECT ord, from_chunk, to_chunk, place_id, time_marker, participants, tags FROM book_scene WHERE book_id = ? ORDER BY ord`,
		bookID)
	if err != nil {
		return nil, fmt.Errorf("deepread: сцены карты книги %d: %w", bookID, err)
	}
	defer rows.Close()

	var scenes []MapScene
	for rows.Next() {
		var (
			ord                int
			fromChunk, toChunk int64
			placeID            sql.NullInt64
			timeMarker         string
			participants, tags string
		)
		if err := rows.Scan(&ord, &fromChunk, &toChunk, &placeID, &timeMarker, &participants, &tags); err != nil {
			return nil, fmt.Errorf("deepread: сцены карты книги %d: %w", bookID, err)
		}
		scene := MapScene{
			Ord:        ord,
			TimeMarker: timeMarker,
			FromChunk:  fromChunk,
			ToChunk:    toChunk,
		}
		if placeID.Valid {
			// Ссылка на место другой книги — пустое имя, а не ошибка: строка
			// разбора остаётся, карта строится по доступным выпискам.
			scene.Place = names[placeID.Int64]
		}
		if err := unmarshalList(participants, &scene.Participants); err != nil {
			return nil, fmt.Errorf("deepread: участники сцены %d: %w", ord, err)
		}
		if err := unmarshalList(tags, &scene.Tags); err != nil {
			return nil, fmt.Errorf("deepread: теги сцены %d: %w", ord, err)
		}
		scenes = append(scenes, scene)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("deepread: сцены карты книги %d: %w", bookID, err)
	}
	return scenes, nil
}

// unmarshalList разбирает JSON-массив строк; пустая строка — пустой список.
func unmarshalList(raw string, out *[]string) error {
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), out)
}

// writeBookMap пишет карту в литературную базу строкой forge_log kind='book_map':
// JSON-объект в task, как запись прогона разбора.
func writeBookMap(lit *db.Handle, bm *BookMap) error {
	raw, err := json.Marshal(bm)
	if err != nil {
		return fmt.Errorf("deepread: запись карты книги %d: %w", bm.BookID, err)
	}
	err = lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		_, err := tx.Exec("forge_log",
			`INSERT INTO forge_log (kind, task, status, started_at, finished_at, log_path, package_version) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			KindBookMap, string(raw), statusDone, bm.BuiltAt, bm.BuiltAt, "", version.Version)
		return err
	})
	if err != nil {
		return fmt.Errorf("deepread: запись карты книги %d: %w", bm.BookID, err)
	}
	return nil
}

// LoadBookMap читает последнюю карту книги-источника: строки forge_log
// kind='book_map' читаются от новых к старым, берётся первая по book_id.
func LoadBookMap(lit *db.Handle, bookID int64) (*BookMap, bool, error) {
	if lit == nil {
		return nil, false, ErrNoStore
	}
	if bookID <= 0 {
		return nil, false, fmt.Errorf("deepread: карта книги: book_id %d", bookID)
	}
	rows, err := lit.Query(
		`SELECT task FROM forge_log WHERE kind = ? AND status = ? ORDER BY id DESC`,
		KindBookMap, statusDone)
	if err != nil {
		return nil, false, fmt.Errorf("deepread: карта книги %d: %w", bookID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var task string
		if err := rows.Scan(&task); err != nil {
			return nil, false, fmt.Errorf("deepread: карта книги %d: %w", bookID, err)
		}
		var bm BookMap
		if err := json.Unmarshal([]byte(task), &bm); err != nil {
			continue
		}
		if bm.BookID == bookID {
			return &bm, true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("deepread: карта книги %d: %w", bookID, err)
	}
	return nil, false, nil
}
