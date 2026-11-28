# План: единая линия main — перенос закладки дистилляции (T-22) и гигиена веток

Дата: 28.11.2026. Регистрация: REG-T22 (ledger). Источник-закладка: ветка
`dry/16-distillation-full` (верх `50235e7`), ответвление от `34929fb`.

## Контекст: почему «слить ветки» нельзя, а «перенести задачи» можно

- `main` и `dry/16-distillation-full` разошлись от `34929fb`: main +174 коммита
  (T-17/T-19/T-20/T-21), закладка +116 (T-14/T-16, МК-1…6, Шаги 1–5).
- Полный `git merge` закладки в main даёт **62 конфликтных файла** (merge-tree):
  `main.go`, `config.yaml`, почти весь `internal/workshop`, `webapi`, `turn`
  и половина `DOCS/`. Cherry-pick подряд ломается на 3-м коммите.
- При этом `git cherry`: **115 из 116 коммитов закладки уже в main по
  содержимому** (patch-id) — работа переносилась по частям (как REG-P14:
  T-15-01 перебазирован с закладки). Слепой мерж дважды пронесёт одно и то же.
- В main проверка №8 docs-check (линейность планов, T-16-08) уже живёт в
  эволюционировавшем виде (T-17-08: топологическая перенумерация, без изъятий) —
  переносить `02755d9` отдельно не нужно.
- Реально недостающее — 64 файла (+7 777/−207): пакеты `namepool`,
  `psychotype`, `birth/analysis.go`, `cast/build.go`, `deepread/window.go`,
  `workshop/{recorder,birth_orders,prompt_json}.go`, `turn/psychotype_drift.go`,
  `version.String()`, три миграции БД и проводки.

## Архитектурное решение

1. **main — единственная линия интеграции.** Волны живут в коротких ветках
   `dry/N-<тема>`, вливаются fast-forward после зелёного `gates.sh`.
   Долгоживущие параллельные линии запрещены: расхождение >50 коммитов
   считается аварийным сигналом.
2. **Закладки — тегами, не ветками.** `dry/16-distillation-full` помечается
   тегом `backup/distillation-full-20261121` и остаётся до закрытия волны T-22;
   после — ветка удаляется, тег хранит историю. Перенос работы из закладки —
   только задачами плана с источником-коммитом (как T-15-01/6d45356).
3. **Перенос = задача, не мерж.** Каждая задача: взять `git show
   dry/16-distillation-full:<файл>` за основу, вписать в main-версии общих
   файлов (main всегда база), принести тесты закладки, прогнать свой гейт.
   Мерж-конфликты общих файлов решаются вручную по правилу «main-проводки
   T-20/T-21 неприкосновенны, добавляется переносимое».
4. **Миграции перенумеровываются.** Номера закладки заняты в main другими
   миграциями (см. D-22-1). Перенос без перенумерации невозможен.

## Решения владельца до кода (рекомендации; отметка в ledger при старте волны)

- **D-22-1 (миграции).** literary: `0007_pool_names` (закладка) → **0009**,
  `0008_character_forms` → **0010**; world: `0013_player_profile` → **0014**.
  Рекомендация: да — номера 0007/0008/0013 в main заняты
  (`gefest_rename`, `bank_tags`, `player_atlas`).
- **D-22-2 (два психопрофиля).** `player_atlas` (main, T-17-04, пишет Пифия)
  остаётся без изменений; `player_profile` (закладка, пишет анализ рождения) —
  новая таблица рядом. Слияние таблиц не делается. Рекомендация: принять.
- **D-22-3 (анкета).** Поле `favorite_books` (T-19-06, рабочее в main)
  сохраняется; анализ двух текстов добавляет психотип и банк заявки, анкету
  не сужает. Рекомендация: сохранить оба источника.
- **D-22-4 (МК-6 общая литбаза).** Поэтапно: волна T-22 проводит namepool на
  per-world `literary.db` мира (без перекройки путей рождения/воркера/чата);
  общая база инстанса — отдельное решение после волны, если пул покажет
  ценность. Рекомендация: поэтапно.
- **D-22-5 (ветки).** Слитые ветки (ahead=0) удалить после тегов-меток;
  `dry/full-fixes` и `dry/16-distillation-full` — под тег закладки до закрытия
  T-22, затем удалить. Рекомендация: да (см. T-22-16).

## Волна 1. Чистые переносы (новые файлы, общие файлы почти не трогаем)

### T-22-01. `version.String()`: строка сборки в бинарнике

- Суть: `tns65 --version` и журнал станции показывают «tns65 <версия> сборка
  <дата>»; значения прошиваются `-ldflags` через scripts/build.sh.
- Закладка: `3e66779` (T-14-31).
- Файлы: `engine/internal/version/version.go` (+~15 строк), `version_test.go`,
  `engine/scripts/build.sh`.
- Проверка: `cd engine && go test ./internal/version -count=1` — PASS;
  `go run ./cmd/tns65 --version` печатает строку сборки.

### T-22-02. Пакет `internal/psychotype`: весовая матрица хода

- Суть: детерминированный пересчёт профиля психотипа (метрики ∈ [−1,1]) в веса
  хода ∈ [0,1]; хранение в `world_meta.psychotype_matrix`. Ноль LLM (механика
  №15). Потребители подключаются в волне 5.
- Закладка: `ada986e` (Шаг 4).
- Файлы: `engine/internal/psychotype/psychotype.go` (61), `psychotype_test.go` (115).
- Проверка: `cd engine && go test ./internal/psychotype -count=1` — PASS
  (детерминизм, дефолт 0.5, чтение из world_meta, битый JSON).

### T-22-03. `workshop/prompt_json`: строгий JSON-контракт промптов

- Суть: `JSONContract(fields)` строит контракт в промпте, `ParseJSONOutput`
  устойчиво разбирает ответ (markdown-ограждения, проза по краям). Переиспользуется
  `compose`/`complement` (промпт-контракт compose уже частично в main, 93bc2c0 —
  свести разбор на общий помощник).
- Закладка: `4d1a6bf` (МК-4.1).
- Файлы: `engine/internal/workshop/prompt_json.go` (96); правки вызовов в
  `compose.go`, `complement.go` (разбор ответа через `ParseJSONOutput`).
- Проверка: `cd engine && go test ./internal/workshop -run 'TestCompose|TestComplement' -count=1`
  — PASS; новые юниты контракта — PASS.

### T-22-04. `workshop/recorder`: счётчик вызовов/отказов слота + карточка MCP

- Суть: обёртка `SlotRecorder` вокруг слота мастерской считает вызовы и отказы
  (роль, сообщение, время); карточка книги в MCP показывает «ключ сохранён,
  а ковка стоит» (`slotErrorStats`). Проводка: `wireWorkshopSlot` возвращает
  рекордер; каскад на воркер и swarm-задачи — по типу возвращаемого значения.
- Закладка: `afdd922`, `7df7f67` (МК-1).
- Файлы: `engine/internal/workshop/recorder.go` (74), `recorder_test.go` (85),
  `engine/cmd/tns65/main.go` (`wireWorkshopSlot`), `engine/internal/webapi/handlers_mcp.go`.
- Проверка: `cd engine && go test ./internal/workshop -run TestSlotRecorder -count=1`
  — PASS; `go vet ./...` чисто; e2e Verne не упал.

## Волна 2. Пул имён героев книг (namepool, per-world)

### T-22-05. Миграция literary 0009: `pool_names` + писатель

- Суть: таблица `pool_names` (UNIQUE character_id, FK `book_character` ON DELETE
  CASCADE, индексы culture/desc_rating), перенумерована из закладочной 0007
  (D-22-1). Регистрация в реестре миграций `engine/ARCHITECTURE.md` (гейт №4).
- Закладка: `d597188` (МК-5).
- Файлы: `engine/internal/db/schema/literary/0009_pool_names.sql`,
  `literary/tables.txt`, `engine/internal/db/writers.go`
  (`{DBLiterary, "pool_names"}: WriterForge`), `engine/ARCHITECTURE.md`.
- Проверка: `cd engine && go test ./internal/db -count=1` — PASS;
  `bash scripts/docs-check.sh` — без DRIFT (реестр миграций).

### T-22-06. Пакет `internal/namepool`: сбор и выбор имён

- Суть: `Fill(lit)` — героев книг литбазы в `pool_names` (идемпотентный upsert);
  `Pick(lit, culture, taken)` — имя по культуре мира с приоритетом полного
  описания; `DescRating`, `NameCulture`, `WorldCulture`.
- Закладка: `d597188` (МК-5).
- Файлы: `engine/internal/namepool/{pool,pick,culture}.go` (308) + тесты (280).
- Проверка: `cd engine && go test ./internal/namepool -count=1` — PASS
  (Fill идемпотентность, культура, занятые, исчерпание, nil-хэндл).

### T-22-07. Проводка пула: ковка заполняет, труппа выбирает

- Суть: `RunForgeBook` вызывает `namepool.Fill` после ковки книги; в
  `turn/troupe.go` `assignPoolName` для мира без книги-источника берёт
  `namepool.Pick` (fallback-цепочка «пул книг → base.<lang>.db», культура мира
  из `worldNaming()` по title/description/synopsis). Per-world литбаза (D-22-4).
- Закладка: `d597188` (МК-5), проводка `5a8d620` без МК-6.
- Файлы: `engine/internal/workshop/forge_book.go`, `engine/internal/turn/troupe.go`,
  `engine/internal/turn/pipeline.go` (`troupeFor`).
- Проверка: `cd engine && go test ./internal/turn ./internal/workshop -count=1`
  — PASS; новый юнит fallback-цепочки имён — PASS.

## Волна 3. Окна упоминаний deepread (окнные векторы)

### T-22-08. `deepread/window` + `configkeys` + поле Forms

- Суть: окно упоминания персонажа = 1–2 абзаца чанка (≤600 рун), а не весь
  чанк — чинит одинаковые векторы героев одного чанка (косинус 1.0 → ложное
  слияние алиасов, W-25.1). `configkeys.go` — строгий реестр ключей секции
  `deepread` с fail-fast на опечатки (5 парсеров). Поле `Candidate.Forms`
  (словоформы скана) + `CharacterConfig.WindowChars/WindowParagraphs` +
  миграция literary **0010** `character_forms` (ALTER `book_character` ADD
  `forms`, из закладочной 0008; D-22-1).
- Закладка: `0c8da91` (T-14-14), `d5c7b3a` (T-14-15), `c209ace` (T-14-19),
  `13bdbde` (T-14-32).
- Файлы: `engine/internal/deepread/{window,configkeys}.go` (216) + тесты (474),
  правки `characters.go` (Forms, Window*), `engine/internal/db/schema/literary/0010_character_forms.sql`,
  `literary/tables.txt`, `engine/ARCHITECTURE.md`.
- Проверка: `cd engine && go test ./internal/deepread -count=1` — PASS
  (окно по абзацам, гард упоминаний, fallback-вектор чанка, сплитSameChunk);
  docs-check — без DRIFT.

### T-22-09. Врезка окон в `RunPipeline` с сохранением смыслового слоя

- Суть: вектор персонажа строится по тексту окна через эмбеддер; main-овый
  смысловой слой (`senses.go`, `senses_dispute.go`, T-15-02/T-20-03) и поле
  `RunInput.Embedder` (векторный счёт смыслов) сохраняются — семантику поля не
  подменять, окно векторизуется тем же эмбеддером отдельным вызовом. Фазы
  `Stage` в `forge_log` для карточки MCP.
- Закладка: `0c8da91`, `e6e5931`.
- Файлы: `engine/internal/deepread/run.go` (врезка, main за базу),
  `characters.go` (`AttachWindowVectors`).
- Проверка: `cd engine && go test ./internal/deepread ./internal/db -count=1`
  — PASS; смысловые юниты (`TestSenseDispute` и др.) не упали.

## Волна 4. Карта книги (cast): наполнитель, которого в main нет

### T-22-10. Волна `cast/scan`: словоформы, семьи имён, полные имена

- Суть: скан карты: склейка словоформ (`FoldNameForms`), словарь семей имён
  `packs/ru/names.txt` (T-14-28), полные имена «Имя Фамилия» — одна карточка,
  отчество не отдельный герой, нарицательные/топонимы не герои, стем 3 руны,
  census. `aliases` = только алиасы слияния, словоформы скана — в `forms`.
- Закладка: `62b6525`, `822bf4c`, `42f006f`, `3c0f562`, `e7a6995`, `57e2a04`,
  `c209ace` (T-14-16…19, 23, 25–28).
- Файлы: `engine/internal/cast/{scan,lang,store}.go` (+632/+101 строк),
  `engine/packs/ru/names.txt` (123), `packs/ru/manifest.json` (sha256),
  тесты `cast/*_test.go`.
- Проверка: `cd engine && go test ./internal/cast -count=1` — PASS; фикстура
  приёмки алиасов `testdata/alias-acceptance.json` — PASS.

### T-22-11. `cast/build`: оркестратор карты книги

- Суть: `Build(lit, opts)`: куски → Scan → Describe → ChunkRoles → строки
  литбазы с журналом (`EventCast*` — новые константы журнала) и ключом
  пересчёта `text_hash+dict_version` в `forge_log kind='cast'`; подтверждение
  героев по порогу упоминаний ≥3 (МК-5.1); версия словаря в forge_log.
- Закладка: `fdc5100` (МК-2), `afdcf04` (МК-5.1).
- Файлы: `engine/internal/cast/build.go` (549) + `build_test.go` (380),
  `engine/internal/journal/events.go` (+6 констант), `store.go` (Forms).
- Проверка: `cd engine && go test ./internal/cast -count=1` — PASS
  (карта синтетической книги, подтверждение, forms≠aliases).

### T-22-12. Проводка карты в `RunForgeBook` + фазы прогресса

- Суть: `buildBookCast` вызывается между циклом чанков и deepread; лемматизатор
  — через адаптер `castLemmatizer`; `Stage`-фазы прогресса ковки в карточке MCP.
  main-овые `forge_plan`/`order_runner`/`bank_tags`/`ParamsSlot` не трогать.
- Закладка: `fdc5100`, `afdd922`.
- Файлы: `engine/internal/workshop/forge_book.go` (main за базой),
  `engine/internal/webapi/handlers_mcp.go`.
- Проверка: `cd engine && go test ./internal/workshop ./e2e -count=1` — PASS;
  живая ковка малой книги пишет строки `book_character`/`forge_log kind='cast'`.

## Волна 5. Психотип: анализ рождения и дрейф хода

### T-22-13. `birth/analysis` + миграция world 0014

- Суть: строгий разбор JSON-ответа внешней LLM (психотип: 4 оси + 14 метрик из
  `data/metrics.json`, код из 16, голос мира; банк заявки/крючка/обещания) и
  запись: профиль → `player_profile`, банк → `book_claim.kind`, матрица →
  `world_meta`. Миграция world **0014** из закладочной 0013 (D-22-1/-2/-3:
  favorite_books остаётся, player_atlas не трогается).
- Закладка: `ada986e` (Шаг 4).
- Файлы: `engine/internal/birth/analysis.go` (292) + `analysis_test.go` (253),
  `engine/internal/db/schema/world/0014_player_profile.sql`,
  `world/tables.txt`, `engine/internal/db/writers.go`, `engine/ARCHITECTURE.md`.
- Проверка: `cd engine && go test ./internal/birth ./internal/db -count=1` —
  PASS; docs-check — без DRIFT.

### T-22-14. `workshop/birth_orders`: исполнение заказов рождения по модели T-21

- Суть: `ExecuteBirthOrders` — исполнитель заказов `analysis`/`forge_plan`.
  Встраивается в main-модель: статусы ведёт воркер (T-21-03 running→done/failed),
  исполнитель регистрируется по Kind в реестре T-21-04, а не немедленное
  исполнение при рождении. Промпт из пролога/описания/синопсиса, ответ — в
  checkpoint заказа и `world_meta`.
- Закладка: `ada986e` (Шаг 4), сплит `enqueue(..., attemptNow)` из `8e27246`.
- Файлы: `engine/internal/workshop/birth_orders.go` (200) + тесты (227),
  `queue.go`, `scheduler.go` (реестр исполнителей), `birth.go` (постановка).
- Проверка: `cd engine && go test ./internal/workshop ./internal/birth -count=1`
  — PASS (заказ queued→running→done, отказ растит attempts, без слота — queued).

### T-22-15. `turn/psychotype_drift`: пассивная подстройка психотипа

- Суть: механика №39: после commit-стадии реплика игрока с шансом 0.15 двигает
  1–2 метрики на 0.05 (таблица 7 признаков: вопрос/приказ/план/…) и пересчитывает
  матрицу. Стадия вставляется в main-реестр 18+ стадий одной строкой (после
  commit, перед audit); сбой записи ход не валит.
- Закладка: `e0d873e` (Шаг 5), `ada986e`.
- Файлы: `engine/internal/turn/psychotype_drift.go` (228) + тесты (547),
  `stages.go` (строка реестра + `TurnState.Psychotype`), `pipeline.go`
  (`ForgeDB`, `Draw`).
- Проверка: `cd engine && go test ./internal/turn -count=1` — PASS
  (движение метрик, аккумуляция, шанс, не блокирует ход).

## Волна 6. Гигиена веток и закрепление

### T-22-16. Теги закладок и чистка слитых веток

- Суть: тег `backup/distillation-full-20261121` → `dry/16-distillation-full`,
  `backup/full-fixes-20261121` → `dry/full-fixes`; слитые ветки (ahead=0:
  `dry/16-distillation`, `dry/17-hole-curing`, `dry/anchor-threshold`,
  `dry/fine-spec-v1`, `fix/live-test-2`, `fix/live-test-20261124`,
  `fix/live-test-3`, `golden/edit`, `dry/20-llama-dicts`, `dry/21-gen-params`)
  удалить локально и на origin (по подтверждению владельца, D-22-5). Ветки
  закладок — после закрытия T-22-18.
- Файлы: только git-операции; запись в ledger.
- Проверка: `git branch -vv` — только `main` и активные волны; теги
  `git tag -l 'backup/*'` на месте; `git log backup/distillation-full-20261121 -1`.

### T-22-17. Правило ветвления в канон разработки

- Суть: одна линия интеграции (`main`), волны `dry/N-<тема>` короткоживущие,
  fast-forward после зелёного `gates.sh`; закладки — теги `backup/*`, перенос
  из закладки — задачами плана с источником-коммитом; расхождение >50 коммитов
  — тревога. Регистрация в `fine/plan/00-INDEX.md` (нормы) и
  `DOCS/08-DEVELOPMENT-PLAN.md`.
- Файлы: `fine/plan/00-INDEX.md`, `DOCS/08-DEVELOPMENT-PLAN.md`.
- Проверка: `bash scripts/docs-check.sh` — без DRIFT.

### T-22-18. Финал волны: доки, гейт, переиндексация

- Суть: DOCS/02-ARCHITECTURE (новые пакеты namepool/psychotype/recorder),
  DOCS/05-DATABASES (таблицы pool_names, character_forms, player_profile,
  book_claim.kind), реестр миграций `engine/ARCHITECTURE.md`; полный
  `gates.sh`; переиндексация codebase-memory (`index_repository`) и gograph;
  закрытие задач в ledger, чеки реализации в плане.
- Файлы: `DOCS/02-ARCHITECTURE.md`, `DOCS/05-DATABASES.md`,
  `engine/ARCHITECTURE.md`, `fine/plan/99-LEDGER.md`, этот план (чеки).
- Проверка: `bash scripts/gates.sh` — GATES_EXIT=0; docs-check — без DRIFT;
  MCP-индекс перестроен (список проектов подтверждает свежую дату).

## DoD волны

- Все задачи T-22-01…T-22-18 закрыты, чеки реализации стоят, ledger обновлён.
- `bash scripts/gates.sh` зелёный; e2e Verne и воркшоп-тесты зелёные.
- Живая ковка малой книги наполняет `book_character` (карта), окна дают
  разные векторы героев одного чанка, пул имён работает на мире без книги.
- В репозитории одна линия интеграции; закладки — теги; правило ветвления
  в каноне разработки.

## Порядок и зависимости

Линейно по номерам: 1→2→3→4→5→6. Внутри волн: 01–04 независимы (можно
параллельно); 05→06→07; 08→09; 10→11→12; 13→14→15; 16–17 после всех, 18
финал. Миграции: 0009 (T-22-05) → 0010 (T-22-08) → 0014 (T-22-13).

## Выполнение

Волна исполняется воркфлоу `tns65-task` (кодер+ревью), ветка
`dry/22-distill-port`, fast-forward в `main` после зелёного `gates.sh`.
Источник кода — `git show dry/16-distillation-full:<путь>` (или тег закладки);
правки общих файлов — от main-версии, проводки T-20/T-21 сохраняются.
