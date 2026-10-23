# P0. Фундамент

Цель волны: честный станок без игры — статичный бинарник, журнал, карта писателей, три базы, порт-гейт, сетевой гейт, резиденты под надзором, запечённый сид. Пути — от `TNS65/`; шелл для всех команд: `export PATH="$HOME/.local/go/bin:$PATH"`. Гейты и рамки — см. 00-INDEX.

---
### T-01-01. Модуль, сборка, статичный бинарник
- **Атомы:** F-11.1, F-11.2, F-11.4 (инфраструктура; гейты сборки).
- **Выдержки атомов:** F-11.4 — гейты: `gofmt -l` пусто, `go vet` чисто, `go test -race` зелено.
- **Зависит от:** нет.
- **Создаётся:** `engine/go.mod`, `engine/cmd/tns65/main.go`, `engine/internal/version/version.go`, `engine/scripts/build.sh`, `engine/scripts/gates.sh`, `engine/scripts/runtests.sh`, `engine/.gitignore`.
- **Правится:** нет.
- **Писатель:** нет записи.
- **Шаги:**
  1. `go mod init tns65`, директива `go 1.27`.
  2. Переменные `Version` и `BuildDate` в `internal/version`; флаг `--version` в `cmd/tns65` печатает их в stdout, код 0.
  3. `scripts/build.sh`: цели `linux/amd64`, `windows/amd64`, `darwin/arm64`; флаги `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X tns65/internal/version.Version=$(git describe --tags --always)"`; результат в `engine/bin/tns65-<os>-<arch>[.exe]`; плюс копия для текущей ОС `engine/bin/tns65` и сборка `engine/bin/tns65-bake` той же версии — команды задач запускают именно их.
  4. `scripts/gates.sh`: `gofmt -l .` пусто, `go vet ./...`, `go test ./... -race -count=1`, grep-проверка `os\.DevNull|/dev/null` по `*.go` вне `*_test.go` — вывод пуст; любой красный статус — ненулевой код возврата.
  5. `scripts/runtests.sh <pkg> <TestName>…`: `go test -json -count=1` по имени; если хоть одно имя не дало `PASS` — падение с перечнем (защита от ложно-зелёного `-run` на несуществующем имени).
  6. В `.gitignore` внести `bin/`, `state/`, `*.db`, `*.db-wal`, `*.db-shm`.
- **Проверка:** `cd engine && ./scripts/build.sh && test -s bin/tns65-linux-amd64 && test -s bin/tns65-windows-amd64.exe && test -s bin/tns65-darwin-arm64 && test -s bin/tns65 && test -s bin/tns65-bake && file bin/tns65-linux-amd64 | grep -q "statically linked" && ./bin/tns65 --version` → exit 0, в выводе строка версии; `bash scripts/gates.sh` → exit 0.
- **Исполнитель:** код.

---
### T-01-02. Раскладка пакетов и направление импортов
- **Атомы:** F-12.1, F-12.6 (рамка слоёв).
- **Выдержки атомов:** F-12.6 — направление импортов `cmd/* → core/* → доменные пакеты → инфраструктура`; обратные импорты и циклы запрещены.
- **Зависит от:** T-01-01.
- **Создаётся:** `engine/internal/{journal,db,netgate,portgate,supervisor,swarm,langpack,lemmarpc,models,core,birth,workshop,paganel,book,webapi}/doc.go`, `engine/ARCHITECTURE.md`.
- **Правится:** нет.
- **Писатель:** нет записи.
- **Шаги:**
  1. Завести каталоги, в каждом `doc.go` — имя пакета и одно предложение назначения; в `ARCHITECTURE.md` описать три слоя (ядро, резиденты, рой), направление импортов `cmd/* → core/* → доменные пакеты → инфраструктура (db, journal, netgate, portgate)`, эталонный список каталогов `internal/` и реестр миграций `db/schema/` (номер → база → задача-владелец; каждая новая миграция вписывается задачей, которая её создаёт).
  2. Зафиксировать в `ARCHITECTURE.md` контракты общих пакетов: сигнатуры `journal.Logger`/`journal.Pipe`, `db.Open`/`db.Tx`, `netgate.Client` — следующие задачи реализуют их, не изобретая.
  3. Тест запрета циклов: по `go list -deps ./internal/...` проверить, что инфраструктурные пакеты (`db`, `journal`, `netgate`, `portgate`) не импортируют доменные.
- **Проверка:** `cd engine && go build ./... && go vet ./... && test "$(ls internal | wc -l)" -eq 16` → exit 0, вывод `go vet` пуст, каталогов в `internal/` ровно 16 (15 новых плюс `version` из T-01-01); `go test ./internal/... -count=1 -run TestNoImportCycles` → PASS.
- **Исполнитель:** код.

---
### T-01-03. Журнал станка
- **Атомы:** F-12.3, F-05.2, F-09.1.
- **Выдержки атомов:** F-12.3 — вывод резидентов пишется в журнал станка, молчаливый `/dev/null` запрещён.
- **Зависит от:** T-01-01.
- **Создаётся:** `engine/internal/journal/{journal.go,events.go,child.go,journal_test.go}`.
- **Правится:** нет.
- **Писатель:** пишет файл `engine/state/journal/engine.log`, в базы не пишет.
- **Шаги:**
  1. Строки JSON с полями `ts`, `level`, `source`, `event`, `fields`; одна строка — одно событие; API записи `Event(source, event string, fields ...Field)`; один писатель под мьютексом, `O_APPEND`, ротация по 50 МБ, хранить 5 файлов; после ротации текущий файл ≤ порога, архивов ≤ 5, старейший удалён.
  2. В `events.go` — константы имён событий (`db.foreign_writer`, `netgate.deny`, `portgate.deny`, `child.degraded`, `child.stopped`, `lemma.fail`, `swarm.version_mismatch`, `web.access`); все пакеты ссылаются на константы, строки-литералы в коде запрещены — тест на уникальность и отсутствие дублей.
  3. `Pipe(name string, w io.Writer) (io.Writer, error)` для вывода детей: построчная разметка, `source=child:<name>`; `w == nil` — ошибка `ErrNoWriter`, нечитаемые байты кодируются, а не отбрасываются.
  4. Вывод детей в `os.DevNull` запрещён: гейт `gates.sh` ищет `os\.DevNull|/dev/null` вне `*_test.go`.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/journal TestChildOutput TestRotation TestEventsUnique` → PASS: у обоих тестов `PASS`; после ротации архивов ровно 5, старейший удалён; события из `events.go` уникальны; `grep -rnE "os\.DevNull|/dev/null" --include='*.go' internal cmd | grep -v _test.go` → вывод пуст.
- **Исполнитель:** код.

---
### T-01-04. DDL base.db и мигратор
- **Атомы:** D-01, D-02, D-03, D-17, D-19.
- **Выдержки атомов:** D-02.4 — таблицы сида нейтральны (verses, patterns, characters); D-03.2 — после сборки base.db заморожена.
- **Зависит от:** T-01-01.
- **Создаётся:** `engine/internal/db/schema/base/0001_init.sql`, `engine/internal/db/schema/shared/0002_vectors.sql`, `engine/internal/db/{open.go,migrate.go,schema/base/tables.txt}`, `engine/internal/db/base_test.go`.
- **Правится:** нет.
- **Писатель:** `bake` — единственный писатель base.db при запекании (D-09.1); живому ходу, мастерской и игроку — только чтение.
- **Шаги:**
  1. `narratives`: `id`, `name`, `arc_summary`, `lang`, `created_at`; имена нейтральны, без упоминаний источника сида.
  2. `vectors`: `id`, `owner_table`, `owner_id`, `model`, `dim` с `CHECK (dim = 1024)`, `lang`, `source_text`, `vec BLOB NOT NULL`, `created_at`; индекс по `(owner_table, owner_id)`.
  3. Таблицы сида с нейтральными именами: `verses(id, narrative_id, text, lang, ord)`, `patterns(id, narrative_id, kind, text, lang)`, `characters(id, narrative_id, name, traits_json)`; `narrative_id` — внешний ключ на `narratives`.
  4. Мигратор: SQL из `embed.FS` по возрастанию номера, версия в `schema_migrations` и `PRAGMA user_version`; `tables.txt` — эталонный список таблиц для сверки в тесте; общая миграция `shared/0002_vectors.sql` подключается в каждую базу отдельным шагом мигратора (порядок: `0001_init` базы → `shared/0002`), номер и владелец вписаны в реестр миграций `ARCHITECTURE.md`.
  5. `open.go`: все базы открываются с `PRAGMA journal_mode=WAL`, `busy_timeout=5000`; запись сериализована одним соединением на писателя (`SetMaxOpenConns(1)`); чтение — с `_pragma=query_only(1)`, запись на read-only хэндле возвращает ошибку SQLite.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/db TestBaseSchema` → PASS: список таблиц равен `tables.txt`, `dim=1024` принимается, `dim=384` отвергается, запись в `narratives` на read-only хэндле даёт ошибку, у `verses`/`patterns`/`characters` колонки совпадают с шагом 3.
- **Исполнитель:** код.

---
### T-01-05. DDL литературной базы
- **Атомы:** D-05, D-06, D-14, D-19.
- **Выдержки атомов:** D-06.1 — записи пулов банка и гороскопы пишет Гефест; D-14.2 — таблицы literary.db словарями пакета не служат.
- **Зависит от:** T-01-04 (мигратор).
- **Создаётся:** `engine/internal/db/schema/literary/0001_init.sql`, `engine/internal/db/schema/literary/tables.txt`, `engine/internal/db/literary_test.go`.
- **Правится:** нет.
- **Писатель:** `forge` — пулы банка, шаблоны, гороскопы, журнал ковки, артефакты; `sediment` — осевшие сцены и их векторы. Живой ход и игрок — только чтение (D-06.3).
- **Шаги:**
  1. `bank_pool`: `id`, `kind` с `CHECK (kind IN ('routine','drama','fallback'))`, `text`, `lang`, `tags` (слова через запятую: приветствие, торг, сенсорика, …), `trust_level` с `CHECK (trust_level IN (0,1,2))` — шкала: 0 снятый, 1 непроверенный (оседание), 2 проверенный (сид и подъём Гефестом), `origin`, `source_scene_id`, `created_at`.
  2. `lit_template(id, kind, text, lang, tags)`; `sediment_scene(id, source_scene_id, cleaned_text, vector_id, verdict, trust_level, created_at)`; `horoscope_issue(id, scene_id, issue_json, created_at)`.
  3. `forge_log`: `id`, `kind`, `task`, `status`, `started_at`, `finished_at`, `log_path`, `package_version`; `artifact`: `id`, `kind`, `path`, `checksum`.
  4. `tables.txt`: словарей пакета в базе нет (D-14.2) — ни таблиц лексикона, ни запрет-списков, ни детекторов.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/db TestLiterarySchema` → PASS: список таблиц равен `tables.txt`; `kind='fallback'` и `trust_level=0..2` принимаются, `kind='poem'` и `trust_level=3` отвергаются; тест отдельно падает, если в списке появилось имя, содержащее `lexicon`, `stoplist`, `detector`.
- **Исполнитель:** код.

---
### T-01-06. DDL оперативной базы мира
- **Атомы:** D-07, D-08, D-09, D-10, D-17, D-18, W-06, W-17.
- **Выдержки атомов:** D-09.7 — одна таблица один писатель: Forge пишет места, пути и world_meta, живой ход — остальные; W-06.2 — дни пути никогда не хранятся.
- **Зависит от:** T-01-04 (мигратор, общая таблица векторов).
- **Создаётся:** `engine/internal/db/schema/world/0001_init.sql`, `engine/internal/db/schema/world/tables.txt`, `engine/internal/db/world_test.go`.
- **Правится:** нет.
- **Писатель:** `core` — все таблицы, кроме `places`, `paths`, `world_meta`; эти три — `forge` (D-09.7): их пишет контур рождения (B-03.5), мастерская и Паганель (записи `origin='paganel'`).
- **Шаги:**
  1. Мир и персонажи: `world_meta(key,value)`, `characters(id, name, psychotype, estate, card_json)`, `character_layers(character_id, layer, payload_json)`, `character_state(character_id, place_id, x REAL, y REAL, mood, status)` — координаты героя приблизительные, живут здесь, не в `places`; память — `memory_topics(id, character_id, theme, weight, reinforced_at, vector_id)`, её параметры в `world_meta` (`halfLifeDays`, `similarityThreshold`, `maxMemoryEntries`).
  2. Ход: `sessions(id, started_at, ended_at, mode)`, `turns(id, session_id, player_text, reply_text, path, selector_decision, saw_recommendation, trust_level, latency_ms, degraded)`, `scenes(id, session_id, day, place_id, status, prose, turn_id)`, `collisions(id, place_id, day, kind, payload_json)`, `events(id, day, kind, payload_json, source)`, `deferred_events(id, kind, payload_json, due_day, status, created_at)`.
  3. Геомодель: `places(id, name, kind, x, y, epoch_ref)`, `paths(id, from_place_id, to_place_id, type CHECK (type IN ('trakt','river','sea')), length_km, friction)` — поля с днями нет нигде; Паганель: `rings(id, status, progress, budget_used, started_at, finished_at)`, `corridors(id, ring_id, from_place_id, to_place_id, status)`.
  4. Игрок: `player_atlas(id, day, theme, mood, interest, source)` (профиль игрока), `player_map(place_id, known_at, drawn)` (карта мира игрока), `economy(id, day, resource, amount, actor_id)`, `social_graph(id, a_character_id, b_character_id, relation, weight, updated_at)`, `model_bindings(id, role, model_ref, provider, updated_at)` (слоты моделей), `token_bindings(id, scene_id, chapter_id, token, character_id, updated_at)` (токены-заменители биндера, живут сцену или главу — LT-03.7; пишет `core` на коммите хода).
  5. Книга: `book_chapters(id, day, title, status CHECK (status IN ('open','closed','processing','published')), kind CHECK (kind IN ('chapter','epilogue')), digest)`, `book_text(id, chapter_id, version, text, edited_by CHECK (edited_by IN ('player','editor') OR edited_by IS NULL), published)` — версия 1 собирается кодом с `edited_by=NULL`, `book_cuts(id, chapter_id, cut_text, reason, status)`.
  6. Подключить общий `0002_vectors.sql` отдельным шагом мигратора; зафиксировать `tables.txt`.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/db TestWorldSchema` → PASS: создаётся файл `world(<slug>).db` по слагу, список таблиц равен `tables.txt`, `paths.type` не принимает значение вне трёх типов, `book_chapters.status='published'` принимается, а `'draft'` отвергается, в `character_state` есть колонки `x`, `y`.
- **Исполнитель:** код.

---
### T-01-07. Карта писателей и запрет второго писателя
- **Атомы:** D-09.7, F-12.2.
- **Выдержки атомов:** D-09.7 — одна таблица один писатель: Forge пишет места, пути и world_meta, живой ход — остальные; нарушение отклоняется.
- **Зависит от:** T-01-04, T-01-05, T-01-06.
- **Создаётся:** `engine/internal/db/writers.go`, `engine/internal/db/writers_test.go`.
- **Правится:** `engine/internal/db/open.go` (хэндл получает тег писателя).
- **Писатель:** метаданные карты, записи в базы — по карте.
- **Шаги:**
  1. Четыре идентичности писателя: `core` (живой ход, рождение таблиц хода), `forge` (места, пути, world_meta — контур рождения, мастерской и Паганеля), `sediment` (осевшие записи), `bake` (base.db при запекании).
  2. Карта `таблица → допустимый писатель` по трём базам, полным перечнем: base.db — все таблицы `bake`; literary.db — `bank_pool`, `lit_template`, `horoscope_issue`, `forge_log`, `artifact` → `forge`; `sediment_scene` и её `vectors` → `sediment`; world db — `places`, `paths`, `world_meta` → `forge`; `vectors`, `turns`, `scenes`, `events`, `sessions`, `characters`, `character_layers`, `character_state`, `memory_topics`, `token_bindings`, `model_bindings`, `workshop_queue`, `player_atlas`, `player_map`, `economy`, `social_graph`, `book_*`, `scene_signals`, `saw_state`, `scene_arc`, `wakeups`, `deep_reads`, `book_claim`, `book_peaks`, `world_lexicon` → `core`. Тест: каждая таблица из всех `tables.txt` присутствует в карте ровно один раз, и каждая запись карты существует в DDL.
  3. Запись только через `Exec(table, query, args)` и транзакционно через `Tx(writer, fn)`: таблица каждой записи сверяется с картой и с тегом хэндла; произвольного SQL на запись API не даёт; тест «откат при отказе второй записи» — ни одной строки первой таблицы.
  4. Нарушение — `ErrForeignWriter` и строка журнала `event=db.foreign_writer` с таблицей и писателем.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/db TestWriterMap TestWriterMapCoverage TestTxRollback` → PASS: `core` пишет в `turns`, получает `ErrForeignWriter` на `narratives`, `bank_pool` и `places`; `forge` пишет в `places` и `world_meta`, получает отказ на `turns`; в журнале строки `db.foreign_writer`; каждая таблица трёх баз покрыта картой ровно один раз; транзакция с отказом на второй записи не оставляет строк первой.
- **Исполнитель:** код.

---
### T-01-08. Языковой пакет: манифест, версии, откат, сборка
- **Атомы:** D-11, D-12, D-14, D-15, MD-10.
- **Зависит от:** T-01-01, T-01-02.
- **Создаётся:** `engine/internal/langpack/{manifest.go,load.go,versions.go,build.go,langpack_test.go}`, `engine/packs/ru/manifest.json`, `engine/packs/ru/lexicon_intents.jsonl`, `engine/packs/ru/stoplist.txt`, `engine/packs/ru/detectors_time.json`, `engine/packs/ru/detectors_mood.json`.
- **Правится:** `engine/cmd/tns65/main.go` (подкоманда `lang`).
- **Писатель:** нет записи в базы; читает каталог пакета.
- **Шаги:**
  1. Манифест: `id`, `name`, `lang`, `version`, `lemmatizer{kind, protocol}`, `files{путь → sha256}`.
  2. Загрузчик проверяет наличие файлов, суммы и версию; расхождение — отказ с перечнем битых файлов.
  3. Версии: каталог `packs/ru/versions/<version>/`, указатель `current`; откат переключает указатель на предыдущую версию и проверяет её суммы.
  4. `build.go`: перевод базовых пулов сида на язык пакета одним проходом, одним голосом (D-15.1), результат — в каталог версии; шаг перевода живёт только в сборке пакета.
  5. Подкоманда `tns65 lang list` печатает пакеты, версии и текущую; четыре составные части пакета по D-11 (лемматизатор, лексикон намерений, запрет-списки, детекторы времени и настроения — детекторы одной частью, файлами `detectors_time.json` и `detectors_mood.json`) зафиксированы; указатель `current` — текстовый файл с именем версии, не симлинк (Windows).
  6. Форматы файлов пакета — контракты, в шагах: `lexicon_intents.jsonl` — по строке `{"intent":"...","lemmas":["..."],"verbs":["..."]}`, намерение «перемещение» несёт глаголы движения; `stoplist.txt` — по лемме в строке; `detectors_time.json` — `{"units":{"час":"hours","день":"days"},"numerals":{"три":3},"prepositions":["через","спустя"]}`; `detectors_mood.json` — `{"mood":"...","lemmas":["..."]}`.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/langpack TestManifest TestRollback TestBuildHasTranslate` → PASS; `./bin/tns65 lang list` → строка `ru 1.0.0 (current)`; в конвейере хода шага перевода нет — тест ищет вызов перевода только в `internal/langpack/build.go`.
- **Исполнитель:** код.
---
### T-01-09. Протокол лемматизатора и клиент
- **Атомы:** D-13.2, D-13.3, MD-03.3.
- **Выдержки атомов:** D-13.2 — контракт stdio/JSON: лемма, часть речи, метки; D-13.3 — «не» отдельной леммой.
- **Зависит от:** T-01-08.
- **Создаётся:** `engine/internal/lemmarpc/{protocol.go,client.go,client_test.go,testdata/fake_lemmatizer/main.go}`.
- **Правится:** нет.
- **Писатель:** нет записи.
- **Шаги:**
  1. Протокол stdio/JSON построчно: запрос `{"id":<n>,"text":"..."}`, ответ `{"id":<n>,"tokens":[{"form","lemma","pos","tags":[]}]}`; «не» приходит отдельным токеном с леммой `не` (D-13.3).
  2. Клиент: запуск по пути из манифеста пакета, тайм-аут ответа 200 мс, перезапуск после трёх подряд тайм-аутов, ошибки — `event=lemma.fail`.
  3. Тестовый сервер `testdata/fake_lemmatizer` на Go: отвечает по протоколу, умеет имитировать задержку; клиент не зависит от реализации.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/lemmarpc TestProtocol TestClient` → PASS: «не хочу» → токены `не` (лемма `не`) и `хотеть`; тайм-аут 200 мс даёт ошибку, а не зависание.
- **Исполнитель:** код.

---
### T-01-10. Русский лемматизатор: интеграция gomorphy
- **Атомы:** D-13.1, D-13.4, MD-03.3, F-11.2.
- **Выдержки атомов:** D-13.1 — реализация для русского пакета AlexMaxy/gomorphy, код MIT, словарь OpenCorpora CC BY-SA 4.0 отдельным артефактом; D-13.4 — резидентен в памяти ~100 МБ, отвечает за миллисекунды.
- **Зависит от:** T-01-09 (протокол и клиент).
- **Создаётся:** `engine/cmd/tns65-lemmaru/main.go` (резидент по протоколу T-01-09), `engine/packs/ru/lemmatizer/NOTICE` (уведомление OpenCorpora CC BY-SA 4.0).
- **Правится:** `engine/packs/ru/manifest.json` (поле `lemmatizer{kind:"gomorphy", binary:"tns65-lemmaru", dict:"…"}`), `engine/scripts/build.sh` (сборка резидента).
- **Писатель:** нет записи.
- **Шаги:**
  1. Подключить `AlexMaxy/gomorphy` (решение B-1 от 20.10: MIT-код); словарь OpenCorpora — отдельным артефактом со своим уведомлением `NOTICE`, не внутри кода.
  2. Резидент `tns65-lemmaru` читает словарь при старте (бюджет памяти ~100 МБ, D-13.4) и обслуживает протокол T-01-09; отрицание «не» — отдельной леммой.
  3. Прогнать золотой набор `lemmabench-golang-20261020` (формат: файл строк, протокол замера — как в замере от 20.10) через резидента.
- **Проверка:** `cd engine && ./scripts/build.sh && go test ./internal/lemmarpc -count=1 -run TestGoldenLemmaru -v` → PASS: доля верного первого разбора ≥ 89.18 % на золотом наборе; регресс больше 2 п.п. против 89.19 % — стоп и вопрос владельцу; память резидента ≤ 200 МБ.
- **Исполнитель:** код.

---
### T-01-11. Единый сетевой гейт
- **Атомы:** F-05, F-04.1, F-03.4, MD-09.2.
- **Выдержки атомов:** F-05 — единственный компонент, через который проходит наружный трафик; адрес вне списка блокируется с записью в журнал.
- **Зависит от:** T-01-03.
- **Создаётся:** `engine/internal/netgate/{gate.go,gate_test.go}`, `engine/config.yaml` (секция `netgate.allowed_hosts`).
- **Правится:** `engine/internal/netgate` подключается к `models` и `paganel` в следующих волнах.
- **Писатель:** нет записи в базы; отказы — в журнал.
- **Шаги:**
  1. Белый список хостов в конфигурации: внешний API слота, HF для менеджера моделей, край туннеля; контракт списка — по hostname (запись `host` или `host/cidr`); проверка выполняется по hostname до установки соединения, DNS-ответ отдельно не сверяется; редирект за пределы списка запрещён (клиент не следует); DNS-ошибка — отказ с причиной.
  2. `netgate.Client(host)`: HTTP-клиент с `DialContext`, сверка хоста с белым списком до установки соединения; `netgate.Local(addr)` — явный хук для loopback-адресов (`:5001`, `:5002`, httptest в тестах): локальные адреса списком не занимаются.
  3. Нарушение — `ErrDeniedHost` и строка журнала `event=netgate.deny` с хостом и вызывающим.
  4. Прямые вызовы вне гейта запрещены: тест статически проверяет, что `http.Get`, `http.Post`, `net.Dial` есть только в пакете `netgate`.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/netgate TestDeny TestOnlyGateDials TestLocalHook TestNoRedirect` → PASS: запрос к хосту вне списка отклонён, в журнале строка `netgate.deny`; loopback-адрес проходит через `netgate.Local`; редирект на хост вне списка не выполняется; поиск прямых вызовов по остальным пакетам пуст.
- **Исполнитель:** код.

---
### T-01-12. Порт-гейт при старте
- **Атомы:** F-06, F-04.2.
- **Выдержки атомов:** F-06 — при старте ядро сверяет слушающие сокеты с белым списком; чужой слушающий сокет — отказ старта.
- **Зависит от:** T-01-03.
- **Создаётся:** `engine/internal/portgate/{portgate.go,portgate_linux.go,portgate_windows.go,portgate_test.go}`, `engine/config.yaml` (секция `ports.whitelist`).
- **Правится:** `engine/cmd/tns65/main.go` (порядок старта: замок T-01-14 → порт-гейт → надзиратель T-01-13; вызов порт-гейта при старте).
- **Писатель:** нет записи; дефект — в журнал и код возврата.
- **Шаги:**
  1. Читать слушающие сокеты процесса ядра и его детей (Linux — `/proc/net/tcp` и `/proc/net/tcp6` с разбором hex-адресов и привязкой к процессу через `/proc/<pid>/fd` inode; Windows — `netstat -ano`) и сверять с белым списком `127.0.0.1:8720`, `127.0.0.1:5001`, `127.0.0.1:5002`; `::1` относится к loopback.
  2. Сокет не на loopback или порт вне списка — отказ старта, код возврата 3, строка `event=portgate.deny`; сокеты чужих процессов (`sshd` и прочие) гейт не считают — фильтр по процессам ядра.
  3. Приёмка на живой машине — отдельной командой `ss -H -lntp`; её вывод код не разбирает.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/portgate TestFixtureExternal TestFixtureLoopback TestV6Loopback` → PASS: на фикстурах `/proc/net/tcp`/`tcp6` внешний сокет → отказ (код 3), только loopback → старт, `::1` — loopback; живая проверка `ss -H -lntp | grep tns65 | awk '{print $4}' | grep -v '^127.0.0.1:'` → вывод пуст.
- **Исполнитель:** код.

---
### T-01-13. Ядро-надзиратель: запуск, лечение, остановка резидентов
- **Атомы:** F-12.1, F-12.3, F-12.6, MD-03.
- **Выдержки атомов:** F-12.3 — вывод резидентов пишется в журнал станка, молчаливый `/dev/null` запрещён.
- **Зависит от:** T-01-03, T-01-11, T-01-12.
- **Создаётся:** `engine/internal/supervisor/{supervisor.go,child.go,health.go,supervisor_test.go}`, `engine/config.yaml` (секция `children`).
- **Правится:** `engine/cmd/tns65/main.go` (старт надзирателя).
- **Писатель:** нет записи в базы; деградации — в журнал.
- **Шаги:**
  1. Ребёнок в конфигурации: `name`, `cmd`, `args`, `port`, `health_path`, `ready_timeout` (по умолчанию 600 с), `poll_interval` (по умолчанию 2 с) — интервал и тайм-аут инжектируются, тесты масштабируют время.
  2. Запуск: stdout и stderr — через `journal.Pipe(name)`, прямой `os.Stdout` ребёнка запрещён; готовность: опрос health-эндпоинта каждые `poll_interval` до `ready_timeout`, короткого фиксированного бюджета нет — загрузка модели идёт минутами.
  3. Падение: перезапуск с паузой 5, 10, 20, 30, далее 60 с; каждая неудача — `event=child.degraded` и рост счётчика деградаций.
  4. Остановка: по SIGTERM и SIGINT дети гасятся в обратном порядке, ожидание выхода, `event=child.stopped`; иных путей запуска детей в коде нет.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/supervisor TestRestart TestNoShortReadyBudget TestStopOrder` → PASS на тестовом ребёнке с подставными часами: после аварийного выхода появился перезапуск, в журнале строка `child.degraded`; ребёнок с задержкой готовности 900 «секунд» на масштабированных часах признаётся готовым, а не убитым; реального ожидания в тесте нет.
- **Исполнитель:** код.

---
### T-01-14. Один экземпляр ядра
- **Атомы:** F-12.1, приёмка F-12 (второй запуск не порождает дубликатов резидентов).
- **Зависит от:** T-01-13.
- **Создаётся:** `engine/internal/supervisor/lock.go`, `engine/internal/supervisor/lock_test.go`.
- **Правится:** `engine/cmd/tns65/main.go` (захват замка до старта детей).
- **Писатель:** нет записи; событие в журнал.
- **Шаги:**
  1. Замок на файле `engine/state/tns65.lock`: Linux — `flock`, Windows — `LockFileEx`; второй запуск — код 4 с сообщением о работающем ядре, без старта детей.
  2. Замок снимается при штатной остановке; после аварийного выхода не мешает старту.
- **Проверка:** `cd engine && go test ./internal/supervisor -race -run TestSecondInstance -v` → PASS: два процесса, второй код 4, число запущенных детей не удвоилось.
- **Исполнитель:** код.

---
### T-01-15. Версии пакета ядра и роя
- **Атомы:** F-12.5.
- **Выдержки атомов:** F-12.5 — задача старой версии с новым ядром не запускается.
- **Зависит от:** T-01-14.
- **Создаётся:** `engine/internal/swarm/{task.go,launch.go,swarm_test.go}`.
- **Правится:** нет (точку запуска задач роя `tns65 swarm <task>` заводит T-05-02; здесь — контракт задачи и версионная сверка).
- **Писатель:** нет записи в базы; задача пишет результат через ядро.
- **Шаги:**
  1. Версия из `internal/version` передаётся задаче переменной окружения `TNS65_PACKAGE_VERSION`; задача первым действием печатает рукопожатие `{"version":"..."}` в stdout, ядро сверяет версию.
  2. Расхождение — задача гасится, строка `event=swarm.version_mismatch`, работа не начинается.
  3. Задача однозадачная: получила работу, отдала результат, вышла; после выхода нет ни процесса, ни файла-состояния.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/swarm TestVersionMismatch TestTaskLifecycle` → PASS: несовпадение версии гасит задачу до работы; после штатного завершения тест не находит процесс по сохранённому pid и список файлов каталога состояния не изменился.
- **Исполнитель:** код.

---
### T-01-16. Запекание сида: base.db и заморозка
- **Атомы:** D-04, D-02, D-03, D-15.
- **Выдержки атомов:** D-04.1 — запекание выполняется вне игры, один раз; D-03.2 — после сборки base.db заморожена.
- **Зависит от:** T-01-04, T-01-08, T-01-07 (запись через `bake`).
- **Создаётся:** `engine/cmd/tns65-bake/main.go`, `engine/internal/bake/{bake.go,bake_test.go}`, `engine/seed/narratives/` (файлы сида), `engine/seed/gazetteer.jsonl` (справочник реальных мест: `{"name":"…","x":…,"y":…,"epoch_ref":"…","kind":"city"}` — читает T-02-16), `engine/seed/verses.txt`.
- **Правится:** нет (сборка — `build.sh`; запекание в сборку не входит).
- **Писатель:** `bake` — единственный писатель base.db (D-09.1).
- **Шаги:**
  1. Файл сида: имя нарратива, резюме дуги, список стихов; проверка, что у нарратива ровно одно резюме дуги; нарративов ≥ 1 — иначе отказ с причиной (якорь без нарративов невозможен, B-10).
  2. Вектор — по полю «резюме дуги», не среднее векторов частей (D-02.3), через `--embed-url` (по умолчанию `http://127.0.0.1:5002`); тест запускает подставной эмбеддер на `httptest.Server` — живой `:5002` не нужен.
  3. Запись в `world_db/base/base.db`: нарративы, ровно один вектор на нарратив, стихи, паттерны, персонажи сида; заморозка: `PRAGMA user_version`, открытие далее только `query_only`, рядом файл `base.db.sha256`; повторный запуск отказывает с кодом 5 и сообщением «сид уже запечён».
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/bake TestBakeFreezes TestBakeIdempotent` → PASS: на подставном эмбеддере у каждого нарратива ровно один вектор `dim=1024`; повторный запуск — код 5; после заморозки запись в `narratives` через хэндл чтения возвращает ошибку; сумма `base.db.sha256` совпадает.
- **Исполнитель:** код.

---
## Выход волны P0
Все 16 задач → бинарники ядра (три платформы, копия текущей ОС, `tns65-bake`, `tns65-lemmaru`) одной версии, ядро стартует и держит резидентов, базы создаются, писатели разграничены картой с транзакционным API, сеть и порты под гейтами, сид запечён и заморожен. Дальше P1: анкета, рождение мира, карточки, якорь по векторам.
