# Починка проводки эмбеддера (:5002) — 2026-11-25

План зарегистрирован в `fine/plan/99-LEDGER.md` (REG-EMB). Источник —
расследование сессии 25.11.2026: чтение кода + проверка живой базы
`TNS65-TEST/world_db/worlds/testforge`.

## Что установило расследование

Проектно эмбеддер (BGE-M3, `:5002`) участвует в горячем пути:

- резка и векторизация реплики — `T-03-02` (`fine/plan/03-live-turn.md`);
- двухголосие подтверждения — `T-03-04`;
- смысловой дедуп оседания — `T-04-04`;
- векторы чанков книги при варке — `P11`, `T-12-11`;
- центроиды персонажей/сцен/якорей при разборе — `P14`;
- центроиды смыслов омонимов — `T-15-03`.

В проде клиент `models.NewClient` создаётся только в двух местах:

- `internal/birth/birth.go:172-182` — якорь при рождении мира (клиент создаётся
  сам, когда `Config.Embed == nil`);
- `internal/bake/bake.go:104` — отдельная утилита `tns65-bake`.

Во всех остальных точках `Embedder == nil`:

- `turn.Pipeline` собирается в `webapi/handlers_chat.go:191` без поля
  `Embedder` → резка без векторов, векторный голос подтверждения выключен;
- `workshop.embedderForge()` nil → `writeBankVector` пропускает векторы чанков;
- `deepread.LoadSenseLayer` строит слой без `senses.VectorLayer` → смыслы только
  триггерным счётом;
- `sediment.Pipeline` не собирается в проде (отдельная проблема, см. T-20-04).

Живая проверка `testforge`: `testforge.db.vectors = []`,
`literary.db.vectors = []`. Быстрый ответ (Челнок, `internal/bank`) выбирает
записи по тегам и эмбеддера не требует — это вне объёма починки.

## Задачи

### T-20-01. Конфиг и клиент эмбеддера

- Расширить `internal/llmparams.Config`: поля `EmbedAddr`, `EmbedModel`,
  `EmbedTimeout`; в `ParseSettings` читать `llm.embed_addr` / `llm.embed_model` /
  `llm.embed_timeout`.
- Добавить построение клиента: пустой адрес → nil (деградация без падений);
  адрес вида `:5002` приводить к `http://127.0.0.1:5002` (как в проводке слота
  генерации); `embed_timeout` → тайм-аут HTTP-клиента. `models.NewClient`
  сейчас без тайм-аута — добавить вариант с тайм-аутом по образцу
  `NewWeaverTimeout`.
- Тест: парсинг трёх ключей, пустой адрес → nil, адрес+модель → клиент,
  тайм-аут применяется.

### T-20-02. Чат (turn): векторы реплики и векторный голос

- В `webapi` завести `chatEmbedder` и сеттер `SetChatEmbedder` (по образцу
  `SetChatWeaver` / `SetChatLemmatizer`).
- В `handlers_chat.go` в литерале `turn.Pipeline` добавить
  `Embedder: chatEmbedder`.
- В `cmd/tns65/main.go` `run()` собрать клиент из T-20-01 и вызвать
  `webapi.SetChatEmbedder(emb)`.
- Эффект: `cutStage` векторизует сегменты/целую реплику/ранние слова;
  `confirm` включает векторный голос.
- Тест: хендлер-тест, что конвейер получает эмбеддер при установленном сеттере.

### T-20-03. Варка и разбор книг

- Ядро: `workshop.SetEmbedder(emb)` — покрывает гороскоп-ветки и
  atlas-forge путь (`writeBankVector`).
- Ребёнок `forge-book` (`runForgeBookTask`): поднять клиент и
  `workshop.SetEmbedder(emb)` → `writeBankVector` пишет векторы чанков.
- `deepread`: добавить поле `Embedder` в `RunInput`; в `runLayers` собирать
  `senses.VectorLayer` через `senses.NewVectorLayer`, когда есть словарь,
  контексты и эмбеддер; `RunForgeBook` передаёт `embedderForge()` в
  `RunPipeline`; `runDeepreadTask` передаёт клиент в `RunInput.Embedder`.
- Эффект: векторы чанков/сцен/персонажей/якорей + векторный слой смыслов.
- Проверка: существующий `forge_book_test` покрывает векторы при
  `SetEmbedder`; после проводки — прогон ковки 1 книги на `testforge` и
  проверка, что `vectors` заполнены.

### T-20-04. Оседание (sediment) — blocked-dep

- Компоненты есть и закрыты задачами: воркер мастерской `workshop.Scheduler`
  (T-05-01, `scheduler.go`), исполнение заказа sediment
  `NewSedimentRunner`/`RunSedimentOrder` (T-05-16, `sediment_run.go`).
- Но в проде воркер не подключён: `workshop.NewScheduler`,
  `SetSedimentRunner`, `DefaultSedimentRun`, `MarkActive`, `SetChapterClosed`
  вызываются только в тестах/e2e (`int12_workshop_test.go`); в `main.go`,
  `webapi` и `turn.Pipeline` их нет. Документы это и закладывали:
  `02-birth.md` — «исполнитель очереди появится в P4»; `04-sedimentation.md`
  — «Дальше P4: мастерская»; `brief3.md` — «конвейер (T-04-05) нигде не
  запускается». Позже Э3 (`PROVIDER-ROUTING-PLAN`) закрыл только проводку
  *слота* мастерской (`workshop.SetSlot` в `main.go`), а не воркера.
- Предусловие: отдельная задача «проводка воркера мастерской в живой ход» —
  создать `workshop.Scheduler` на ядре/в `turn`, звать `Run` на тяжёлой
  ступени и в ночном окне, `MarkActive`/`SetChapterClosed` из хода, отдать
  `NewSedimentRunner` со слотом, цензором и эмбеддером. До неё эмбеддер
  оседания подключать некуда.
- Статус: `blocked-dep`.

## Порядок и проверка

- T-20-01 → T-20-02 (независимы от мастерской) → T-20-03; T-20-04 ждёт
  проводки исполнителя мастерской.
- Гейты: `cd engine && bash scripts/gates.sh`; независимый прогон тестов каждой
  задачи.
- Живая проверка: пересобрать движок, прогнать ковку 1 книги на `testforge`,
  убедиться, что `vectors` заполнены и есть векторы `bank_pool`, `book_scene`,
  `book_character`, `dict_anchor`; прогнать реплику в чате — векторный голос
  работает (при недоступном `:5002` — видимая деградация `embed.fail`, ход не
  валится).

## Вне объёма

- Быстрый ответ (Челнок, `internal/bank`) остаётся теговым — эмбеддер туда не
  добавляем.
- `webapi.atlasEmbed` — для функции не нужен: `birth.Birth` сам создаёт клиент
  при `Embed == nil`.
