# Ф0. Фундамент

Цель волны: пустой, но честный станок — собирается в статичный бинарник, ведёт
журнал, держит карту писателей, знает три базы, умеет проверять порты и сеть,
поднимает и лечит резидентов, запекает сид. Никакой игры в этой волне нет:
после Ф0 ядро запускается и падает ровно там, где должно.

Пути — от `TNS65/`. Шелл для всех команд: `export PATH="$HOME/.local/go/bin:$PATH"`.

---

### T-01-01. Модуль, сборка, статичный бинарник
- **Атомы:** F-11.1, F-11.2, F-11.4.
- **Зависит от:** нет.
- **Создаётся:** `engine/go.mod`, `engine/cmd/tns65/main.go`, `engine/internal/version/version.go`, `engine/scripts/build.sh`, `engine/.gitignore`.
- **Правится:** нет.
- **Писатель:** нет записи.
- **Шаги:**
  1. `go mod init tns65`, директива `go 1.27`.
  2. Добавить зависимость `modernc.org/sqlite` (чистый Go), версию зафиксировать в `go.mod`.
  3. В `internal/version` завести переменные `Version` и `BuildDate`, подставляемые линковщиком.
  4. В `cmd/tns65` обработать флаг `--version`: печать версии и даты в stdout, код возврата 0.
  5. В `scripts/build.sh` собрать три цели: `linux/amd64`, `windows/amd64`, `darwin/arm64`; флаги `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X tns65/internal/version.Version=$(git describe --tags --always)"`; результат в `engine/bin/tns65-<os>-<arch>[.exe]`.
  6. В `.gitignore` внести `bin/`, `state/`, `*.db`, `*.db-wal`, `*.db-shm`.
- **Проверка:** `cd engine && ./scripts/build.sh && file bin/tns65-linux-amd64 | grep -q "statically linked" && ./bin/tns65-linux-amd64 --version` → exit 0, в выводе строка версии; `gofmt -l .` пусто.
- **Исполнитель:** код.

---

### T-01-02. Раскладка пакетов и направление импортов
- **Атомы:** F-12.1, F-12.6 (рамка слоёв).
- **Зависит от:** T-01-01.
- **Создаётся:** `engine/internal/{journal,db,netgate,portgate,supervisor,swarm,langpack,lemmarpc,models,core,birth,workshop,paganel,book,webapi}/doc.go`, `engine/ARCHITECTURE.md`.
- **Правится:** нет.
- **Писатель:** нет записи.
- **Шаги:**
  1. Завести перечисленные каталоги, в каждом `doc.go` с именем пакета и одним предложением назначения.
  2. В `ARCHITECTURE.md` описать три слоя (ядро, резиденты, рой) и направление импортов: `cmd/* → core/* → доменные пакеты → инфраструктура (db, journal, netgate, portgate)`; обратные импорты и циклы запрещены.
  3. Записать правило: инфраструктурные пакеты не импортируют доменные.
- **Проверка:** `cd engine && go build ./... && go vet ./...` → exit 0, вывод `go vet` пуст; `ls engine/internal | wc -l` → `15`.
- **Исполнитель:** код.

---

### T-01-03. Журнал станка
- **Атомы:** F-12.3, F-05.2, F-09.1.
- **Зависит от:** T-01-01.
- **Создаётся:** `engine/internal/journal/journal.go`, `engine/internal/journal/child.go`, `engine/internal/journal/journal_test.go`.
- **Правится:** нет.
- **Писатель:** пишет файл `engine/state/journal/engine.log`, в базы не пишет.
- **Шаги:**
  1. Реализовать запись строк JSON: `ts`, `level`, `source`, `event`, `fields`; одна строка — одно событие.
  2. Сделать единственный писатель файла под мьютексом, открытие с `O_APPEND`, ротация по 50 МБ, хранить 5 файлов.
  3. Реализовать `Pipe(name string) io.Writer` для вывода детей: построчная разметка, поле `source=child:<name>`, нечитаемые байты не отбрасываются, а кодируются.
  4. Запретить передачу вывода детей в `os.DevNull`: `Pipe` не принимает пустой писатель, при `nil` возвращает ошибку.
- **Проверка:** `cd engine && go test ./internal/journal -race -run 'TestChildOutput|TestRotation' -v` → оба теста PASS; `grep -rn "/dev/null" engine/internal engine/cmd` → вывод пуст.
- **Исполнитель:** код.

---

### T-01-04. DDL base.db и мигратор
- **Атомы:** D-01, D-02, D-03, D-17, D-19.
- **Зависит от:** T-01-01.
- **Создаётся:** `engine/internal/db/schema/base/0001_init.sql`, `engine/internal/db/schema/shared/0002_vectors.sql`, `engine/internal/db/{open.go,migrate.go,schema/base/tables.txt}`, `engine/internal/db/base_test.go`.
- **Правится:** нет.
- **Писатель:** `разработчик` при запекании сида; живому ходу, мастерской и игроку — только чтение (D-03.3).
- **Шаги:**
  1. Таблица `narratives`: `id`, `name`, `arc_summary`, `lang`, `created_at`; имя таблицы и поля нейтральны, без упоминаний источника сида.
  2. Общая таблица `vectors`: `id`, `owner_table`, `owner_id`, `model`, `dim` с `CHECK (dim = 1024)`, `lang`, `source_text`, `vec BLOB NOT NULL`, `created_at`; индекс по `(owner_table, owner_id)`.
  3. Таблицы сида с нейтральными именами: `verses`, `patterns`, `characters`.
  4. Мигратор: встроенные через `embed.FS` SQL-файлы применяются по возрастанию номера, версия пишется в `schema_migrations` и `PRAGMA user_version`.
  5. Открытие базы для чтения — с `_pragma=query_only(1)`; попытка записи возвращает ошибку SQLite.
  6. `tables.txt` — эталонный список таблиц базы для сверки в тесте.
- **Проверка:** `cd engine && go test ./internal/db -race -run TestBaseSchema -v` → PASS; тест проверяет: список таблиц равен `tables.txt`, `dim=1024` принимается, `dim=384` отвергается, запись в `narratives` на read-only хэндле даёт ошибку.
- **Исполнитель:** код.

---

### T-01-05. DDL литературной базы
- **Атомы:** D-05, D-06, D-14, D-19.
- **Зависит от:** T-01-04 (мигратор).
- **Создаётся:** `engine/internal/db/schema/literary/0001_init.sql`, `engine/internal/db/schema/literary/tables.txt`, `engine/internal/db/literary_test.go`.
- **Правится:** нет.
- **Писатель:** `Гефест/Forge` — пулы банка, шаблоны, гороскопы, журнал ковки, артефакты; `оседание` — осевшие сцены и пополнение пулов через Цензор. Живой ход и игрок — только чтение (D-06.3).
- **Шаги:**
  1. `bank_pool`: `id`, `kind` с `CHECK (kind IN ('routine','drama'))`, `text`, `lang`, `trust_level` (0 — снятый, 1 — проверенный), `origin`, `source_scene_id`, `created_at`.
  2. `lit_template`: `id`, `kind`, `text`, `lang`, `tags`.
  3. `sediment_scene`: `id`, `scene_id`, `cleaned_text`, `vector_id`, `trust_level`, `created_at`.
  4. `horoscope_issue`: `id`, `scene_id`, `issue_json`, `created_at`.
  5. `forge_log`: `id`, `task`, `status`, `started_at`, `finished_at`, `log_path`; `artifact`: `id`, `kind`, `path`, `checksum`.
  6. Зафиксировать `tables.txt`: словарей пакета в базе нет (D-14.2) — ни таблиц лексикона, ни запрет-списков, ни детекторов.
- **Проверка:** `cd engine && go test ./internal/db -race -run TestLiterarySchema -v` → PASS; список таблиц равен `tables.txt`; тест отдельно падает, если в списке появилось имя, содержащее `lexicon`, `stoplist`, `detector`.
- **Исполнитель:** код.

---

### T-01-06. DDL оперативной базы мира
- **Атомы:** D-07, D-08, D-09, D-10, D-17, D-18, W-06, W-17.
- **Зависит от:** T-01-04 (мигратор, общая таблица векторов).
- **Создаётся:** `engine/internal/db/schema/world/0001_init.sql`, `engine/internal/db/schema/world/tables.txt`, `engine/internal/db/world_test.go`.
- **Правится:** нет.
- **Писатель:** `ядро` — всё, кроме `places`, `paths`, `world_meta`; эти три — `Гефест/Forge` (D-09.7). Исключение зафиксировать в карте писателей задачей T-01-07.
- **Шаги:**
  1. Мир и персонажи: `world_meta(key,value)`, `characters(id, name, psychotype, estate, card_json)`, `character_layers(character_id, layer, payload_json)`, `character_state(character_id, place_id, mood, status)`.
  2. Память: `memory_topics(id, character_id, theme, weight, reinforced_at, vector_id)` — имена параметров памяти NPC живут в `world_meta` (`halfLifeDays`, `similarityThreshold`, `maxMemoryEntries`).
  3. Ход: `sessions(id, started_at, ended_at, mode)`, `turns(id, session_id, player_text, reply_text, path, selector_decision, saw_recommendation, trust_level, latency_ms, degraded)`, `scenes(id, session_id, day, place_id, status, prose, turn_id)`, `collisions(id, place_id, day, kind, payload_json)`, `events(id, day, kind, payload_json, source)`.
  4. Отложенные: `deferred_events(id, kind, payload_json, due_day, status, created_at)`.
  5. Геомодель: `places(id, name, kind, x, y, epoch_ref)`, `paths(id, from_place_id, to_place_id, type CHECK (type IN ('trakt','river','sea')), length_km, friction)`.
  6. Паганель: `rings(id, status, progress, budget_used, started_at, finished_at)`, `corridors(id, ring_id, from_place_id, to_place_id, status)`.
  7. Игрок: `player_atlas(id, day, theme, mood, interest, source)`, `player_map(place_id, known_at, drawn)`, `economy(id, day, resource, amount, actor_id)`, `social_graph(id, a_character_id, b_character_id, relation, weight, updated_at)`, `slot_bindings(id, slot, model_ref, provider, updated_at)`.
  8. Книга: `book_chapters(id, day, title, status, digest)`, `book_text(id, chapter_id, version, text, edited_by CHECK (edited_by IN ('player','editor')), published)`, `book_cuts(id, chapter_id, cut_text, reason, status)`.
  9. Подключить общий `0002_vectors.sql` (та же таблица `vectors`, размерность 1024).
  10. Зафиксировать `tables.txt`.
- **Проверка:** `cd engine && go test ./internal/db -race -run TestWorldSchema -v` → PASS; создаётся файл `world(<slug>).db` по слагу, список таблиц равен `tables.txt`, `paths.type` не принимает значение вне трёх типов.
- **Исполнитель:** код.

---

### T-01-07. Карта писателей и запрет второго писателя
- **Атомы:** D-09.7, F-12.2.
- **Зависит от:** T-01-04, T-01-05, T-01-06.
- **Создаётся:** `engine/internal/db/writers.go`, `engine/internal/db/writers_test.go`.
- **Правится:** `engine/internal/db/open.go` (хэндл получает тег писателя).
- **Писатель:** метаданные карты, записи в базы — по карте.
- **Шаги:**
  1. Завести три идентичности писателя: `core`, `forge`, `sediment`.
  2. Построить карту `таблица → допустимые писатели` по трём базам, включая исключение `places`/`paths`/`world_meta` за `forge`.
  3. Каждая запись проходит через `Exec(table, query, args)`: таблица сверяется с картой и с тегом хэндла.
  4. При нарушении — типизированная ошибка `ErrForeignWriter` и строка журнала `event=db.foreign_writer` с именем таблицы и писателя.
  5. Запись без указания таблицы запрещена API (нет метода «выполнить произвольный SQL на запись»).
- **Проверка:** `cd engine && go test ./internal/db -race -run TestWriterMap -v` → PASS: `core` пишет в `turns`, получает `ErrForeignWriter` на `narratives` и `bank_pool`; `forge` пишет в `places`, получает отказ на `turns`; в журнале две строки `db.foreign_writer`.
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
  2. Загрузчик проверяет наличие всех файлов, контрольные суммы и версию; при расхождении — отказ с перечнем битых файлов.
  3. Версии: каталог `packs/ru/versions/<version>/`, указатель `current`; откат переключает указатель на предыдущую версию и проверяет её суммы.
  4. Сборка пакета `build.go`: переводит базовые пулы сида на язык пакета одним проходом, одним голосом (D-15.1), пишет результат в каталог версии; шаг перевода живёт только в сборке пакета.
  5. Подкоманда `tns65 lang list` печатает пакеты, версии и текущую.
  6. Зафиксировать четыре составные части пакета (D-11): лемматизатор, лексикон намерений, запрет-списки, детекторы времени и настроения.
- **Проверка:** `cd engine && go test ./internal/langpack -race -run 'TestManifest|TestRollback|TestBuildHasTranslate' -v` → PASS; `./bin/tns65 lang list` → строка `ru 1.0.0 (current)`; в конвейере хода шага перевода нет — тест ищет вызов перевода только в `internal/langpack/build.go`.
- **Исполнитель:** код.

---

### T-01-09. Протокол лемматизатора и клиент
- **Атомы:** D-13.2, D-13.3, MD-03.3.
- **Зависит от:** T-01-08.
- **Создаётся:** `engine/internal/lemmarpc/{protocol.go,client.go,client_test.go,testdata/fake_lemmatizer/main.go}`.
- **Правится:** нет.
- **Писатель:** нет записи.
- **Шаги:**
  1. Описать протокол stdio/JSON построчно: запрос `{"id":<n>,"text":"..."}`, ответ `{"id":<n>,"tokens":[{"form","lemma","pos","tags":[]}]}`.
  2. Зафиксировать правило отрицания: «не» приходит отдельным токеном с леммой `не` (D-13.3).
  3. Реализовать клиент: запуск процесса по пути из манифеста пакета, тайм-аут ответа 200 мс, перезапуск процесса после трёх подряд тайм-аутов, ошибки — в журнал `event=lemma.fail`.
  4. Сделать тестовый сервер `testdata/fake_lemmatizer` на Go: отвечает по протоколу, умеет имитировать задержку.
  5. Проверить отсутствие зависимости клиента от реализации: клиент работает с любым сервером протокола.
- **Проверка:** `cd engine && go test ./internal/lemmarpc -race -v` → PASS, среди кейсов: «не хочу» → токены `не` (лемма `не`) и `хотеть`; тайм-аут 200 мс даёт ошибку, а не зависание.
- **Исполнитель:** код.

---

### T-01-10. Реализация русского лемматизатора — **blocked, вилка В-1**
- **Атомы:** D-13.1, D-13.4, MD-03.3, F-11.2.
- **Зависит от:** T-01-09 и решения владельца по В-1.
- **Создаётся:** зависит от варианта: (1) `engine/internal/lemmago/` + `engine/packs/ru/lemmatizer/dict/`; (2) `engine/packs/ru/lemmatizer/server.py`; (3) как (2) плюс пометка «только машина разработчика».
- **Правится:** `engine/packs/ru/manifest.json` (поле `lemmatizer.kind`).
- **Писатель:** нет записи.
- **Шаги (после решения):**
  1. Зафиксировать выбранный вариант в `99-LEDGER.md` строкой решения владельца с датой.
  2. Вариант 1 — порт словаря pymorphy в Go: словарь отдельным артефактом со своим уведомлением CC BY-SA 3.0, движок под MIT.
  3. Вариант 2 — Python-резидент по протоколу T-01-09: правка F-11.2 фиксируется отдельным решением.
  4. Прогнать золотой набор `lemmabench-20261018` через реализацию.
- **Проверка:** вариант 1 и 2 — доля верного первого разбора не ниже 89.18 % на золотом наборе (`go test ./internal/lemmago -race -run TestGolden` или прогон сервера тем же набором); регресс больше 2 п.п. — стоп и вопрос владельцу. Вариант 3 — проверки нет, задача остаётся открытой до релиза.
- **Исполнитель:** код.

---

### T-01-11. Единый сетевой гейт
- **Атомы:** F-05, F-04.1, F-03.4, MD-09.2.
- **Зависит от:** T-01-03.
- **Создаётся:** `engine/internal/netgate/{gate.go,gate_test.go}`, `engine/config.yaml` (секция `netgate.allowed_hosts`).
- **Правится:** `engine/internal/netgate` подключается к `models` и `paganel` в следующих волнах.
- **Писатель:** нет записи в базы; отказы — в журнал.
- **Шаги:**
  1. Белый список хостов в конфигурации: внешний API слота, HF для менеджера моделей, край туннеля.
  2. Реализовать `netgate.Client(host)` — HTTP-клиент с `DialContext`, который сверяет хост и адрес с белым списком до установки соединения.
  3. Нарушение — ошибка `ErrDeniedHost` и строка журнала `event=netgate.deny` с хостом и вызывающим.
  4. Запретить прямые сетевые вызовы вне гейта: статическая проверка в тесте, что `http.Get`, `http.Post`, `net.Dial` встречаются только в пакете `netgate`.
- **Проверка:** `cd engine && go test ./internal/netgate -race -run 'TestDeny|TestOnlyGateDials' -v` → PASS: запрос к хосту вне списка отклонён, в журнале строка `netgate.deny`; поиск прямых вызовов по остальным пакетам пуст.
- **Исполнитель:** код.

---

### T-01-12. Порт-гейт при старте
- **Атомы:** F-06, F-04.2.
- **Зависит от:** T-01-03.
- **Создаётся:** `engine/internal/portgate/{portgate.go,portgate_linux.go,portgate_windows.go,portgate_test.go}`, `engine/config.yaml` (секция `ports.whitelist`).
- **Правится:** нет.
- **Писатель:** нет записи; дефект — в журнал и код возврата.
- **Шаги:**
  1. Читать таблицу слушающих сокетов: Linux — `/proc/net/tcp` и `/proc/net/tcp6`, Windows — `netstat -ano`.
  2. Сверять с белым списком: `127.0.0.1:8720`, `127.0.0.1:5001`, `127.0.0.1:5002`.
  3. Любой слушающий сокет не на loopback или порт вне списка — отказ старта, код возврата 3, строка `event=portgate.deny`.
  4. Приёмка на живой машине описана отдельной командой `ss -lntp`; разбор её вывода в код не входит.
- **Проверка:** `cd engine && go test ./internal/portgate -race -v` → PASS на фикстурах `/proc/net/tcp` (внешний сокет → отказ, только loopback → старт); живая проверка `ss -lntp | awk '{print $4}' | grep -v '^127.0.0.1' ` → вывод пуст.
- **Исполнитель:** код.

---

### T-01-13. Ядро-надзиратель: запуск, лечение, остановка резидентов
- **Атомы:** F-12.1, F-12.3, F-12.6, MD-03.
- **Зависит от:** T-01-03, T-01-11, T-01-12.
- **Создаётся:** `engine/internal/supervisor/{supervisor.go,child.go,health.go,supervisor_test.go}`, `engine/config.yaml` (секция `children`).
- **Правится:** `engine/cmd/tns65/main.go` (старт надзирателя).
- **Писатель:** нет записи в базы; деградации — в журнал.
- **Шаги:**
  1. Описать ребёнка конфигурацией: `name`, `cmd`, `args`, `port`, `health_path`, `ready_timeout` (по умолчанию 600 с).
  2. Запуск ребёнка: вывод stdout и stderr через `journal.Pipe(name)`; прямой `os.Stdout` ребёнка запрещён.
  3. Готовность: опрос health-эндпоинта каждые 2 с до `ready_timeout`; короткого фиксированного бюджета нет — загрузка модели идёт минутами.
  4. Падение: перезапуск с паузой 5, 10, 20, 30, далее 60 с; каждая неудача — `event=child.degraded` и рост счётчика деградаций.
  5. Остановка: по SIGTERM и SIGINT ядро гасит детей в обратном порядке, ждёт выхода и пишет `event=child.stopped`.
  6. Дети запускаются только надзирателем: иных путей запуска в коде нет.
- **Проверка:** `cd engine && go test ./internal/supervisor -race -run 'TestRestart|TestNoShortReadyBudget|TestStopOrder' -v` → PASS на тестовом ребёнке: после аварийного выхода появился перезапуск, в журнале строка `child.degraded`; ребёнок с задержкой готовности 90 с признаётся готовым, а не убитым.
- **Исполнитель:** код.

---

### T-01-14. Один экземпляр ядра
- **Атомы:** F-12.1, приёмка F-12 (второй запуск не порождает дубликатов резидентов).
- **Зависит от:** T-01-13.
- **Создаётся:** `engine/internal/supervisor/lock.go`, `engine/internal/supervisor/lock_test.go`.
- **Правится:** `engine/cmd/tns65/main.go` (захват замка до старта детей).
- **Писатель:** нет записи; событие в журнал.
- **Шаги:**
  1. Замок на файле `engine/state/tns65.lock`: Linux — `flock`, Windows — `LockFileEx`.
  2. Второй запуск завершается кодом 4 с сообщением о работающем ядре, без старта детей.
  3. Замок снимается при штатной остановке; после аварийного выхода замок не мешает старту.
- **Проверка:** `cd engine && go test ./internal/supervisor -race -run TestSecondInstance -v` → PASS: два процесса, второй код 4, число запущенных детей не удвоилось.
- **Исполнитель:** код.

---

### T-01-15. Версии пакета ядра и роя
- **Атомы:** F-12.5.
- **Зависит от:** T-01-14.
- **Создаётся:** `engine/internal/swarm/{task.go,launch.go,swarm_test.go}`.
- **Правится:** `engine/cmd/tns65-bake/main.go` (если создан раньше — иначе в T-01-16).
- **Писатель:** нет записи в базы; задача пишет результат через ядро.
- **Шаги:**
  1. Версия пакета берётся из `internal/version` и передаётся задаче переменной окружения `TNS65_PACKAGE_VERSION`.
  2. Задача первым действием печатает рукопожатие `{"version":"..."}` в stdout; ядро сверяет версию.
  3. Расхождение — задача гасится, строка `event=swarm.version_mismatch`, работа не начинается.
  4. Задача однозадачная: получила работу, отдала результат, вышла; после выхода не остаётся ни процесса, ни файла-состояния.
- **Проверка:** `cd engine && go test ./internal/swarm -race -v` → PASS: несовпадение версии гасит задачу до работы; после штатного завершения в `ps` нет дочернего процесса, каталог состояния не вырос.
- **Исполнитель:** код.

---

### T-01-16. Запекание сида: base.db и заморозка
- **Атомы:** D-04, D-02, D-03, D-15.
- **Зависит от:** T-01-04, T-01-08.
- **Создаётся:** `engine/cmd/tns65-bake/main.go`, `engine/internal/bake/{bake.go,bake_test.go}`, `engine/seed/narratives/` (файлы сида), `engine/seed/verses.txt`.
- **Правится:** `engine/scripts/build.sh` (вызов запекания после сборки).
- **Писатель:** `разработчик` — единственный писатель base.db (D-09.1).
- **Шаги:**
  1. Формат файла сида: имя нарратива, резюме дуги, список стихов; проверка, что у нарратива ровно одно резюме дуги.
  2. Вектор считается по полю «резюме дуги», а не как среднее векторов частей (D-02.3), через `--embed-url` (по умолчанию `http://127.0.0.1:5002`).
  3. Запись в `world_db/base/base.db`: нарративы, ровно один вектор на нарратив, стихи, паттерны, персонажи сида.
  4. Заморозка: `PRAGMA user_version`, файл `base.db` переводится в режим 0444, рядом кладётся `base.db.sha256`.
  5. Повторный запуск на замороженной базе отказывает с кодом 5 и сообщением «сид уже запечён».
- **Проверка:** `cd engine && ./bin/tns65-bake --seed seed/narratives --out world_db/base/base.db --embed-url http://127.0.0.1:5002` → exit 0; `sqlite3 world_db/base/base.db "select count(*) from narratives n left join vectors v on v.owner_table='narratives' and v.owner_id=n.id where v.id is null"` → `0`; повторный запуск → код 5; `sqlite3 world_db/base/base.db "insert into narratives(name,arc_summary,lang) values('x','y','ru')"` → ошибка «attempt to write a readonly database».
- **Исполнитель:** код.

---

## Выход волны Ф0

Закрыты все 16 задач → собраны три бинарника, ядро стартует и держит резидентов,
базы создаются, писатели разграничены, сеть и порты под гейтами, сид запечён.

Дальше Ф1: анкета, рождение мира, пачка карточек, якорь по векторам.
