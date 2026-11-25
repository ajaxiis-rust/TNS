# План: таймеры, хуки и подключение мастерской (Пифия/Гефест)

Волна эмбеддера T-20-01…T-20-04 закрыта и запушена (`dry/21-gen-params`,
`dc4be41..f942b62`). Расследования выявили: код мастерской готов и покрыт
тестами, но в проде не подключён; два теста падают от окружения (таймер/хук);
есть расхождения с механиками. План чинит таймеры и хуки, подключает блоки и
дореализовывает механики.

## Справочная: цензор при варке — история решения

- **Что решили и когда.** 15.11.2026, план IMP-13 (коммит `c123ad5`), решение
  владельца, ревизия 2: «зачем цензор вообще для ковки книг? он должен
  контролировать только при ходе» — цензор из ковки книг убрать.
- **Почему.** Цензор убивал ковку целиком: `ErrCensorReject` на «тварь»
  (ч.0 «Флибустьер») и «дурак» (ч.42 «Шаг во времени») возвращал ошибку наверх
  — умирала вся книга. Дополнительно: рейтинг 21+ разрешает все слова (пустой
  `stoplist_21`), «дурак»/«тварь» убраны из `stoplist_16` как шум.
- **Текущее расхождение.** `forge_pipeline.go:76-77` «Цензора при ковке нет
  (IMP-13)», но `DOCS/04-WORKSHOP.md` §6, `DOCS/01-PARADIGM.md` §18,
  `fine/04-spec/05-workshop.md` W-14.4 и тест `forge_pipeline_test.go:105`
  всё ещё требуют цензуру. На гороскопе (W-13.9) и эпохе (W-19) цензор
  остаётся — это их спеки, не ковка книг.
- **Направление.** Выровнять доки/спеку/тест под IMP-13: банки книг — без
  цензора при ковке; гороскоп и эпоха — с цензором по их спекам.

## Волна 1. Таймеры и хуки

### T-21-01. `TestNoShortReadyBudget`: тест не зависит от занятого порта 5001

> ✅ Выполнено 2026-11-26 (коммит 88e4dd9): порт-чек подменён через WithPortCheck-фейк, тест независим от порта 5001.

- Суть: юнит супервизора использует `Port: 5001` и заводской `CheckPortFree`;
  на рабочей машине на 5001 стоит `llama-server`, фейк-ребёнок не стартует.
  Убрать зависимость: `Port: 0` (порт-чек пропускается) или `WithCheckPort`-фейк.
- Файлы: `engine/internal/supervisor/supervisor_test.go`.
- Проверка: `cd engine && go test ./internal/supervisor -run TestNoShortReadyBudget -count=1` — PASS.

### T-21-02. `TestVerne`: ядро харнесса не упирается в фиксированный порт 8720

> ✅ Выполнено 2026-11-26 (коммит b02d8db): свободный порт через net.Listen(:0) в копии config.yaml, повтор старта при чужом процессе.

- Суть: `TestVerne` стартует ядро на фиксированном `127.0.0.1:8720`; левый
  процесс `./tns65` (pid 1924735, cwd `/home/ajaxii/PRJ/TNS65-TEST`) занимает
  порт — `/api/health` отвечает левым процессом, `/api/turn` отдаёт 404.
  Сделать порт динамическим (свободный порт через `net.Listen(:0)` и проброс в
  ядро) либо до старта проверять, что порт свободен, и падать с понятным
  сообщением. Перед проверкой убить левый процесс 1924735.
- Файлы: `engine/e2e/verne_test.go`; при пробросе порта — возможно
  `engine/cmd/tns65/hooks_e2e.go`.
- Проверка: `cd engine && go test ./e2e -run TestVerne -count=1` — PASS;
  повторный прогон — PASS.

## Волна 2. Воркер мастерской

### T-21-03. Статусы заказов: running → done/failed, без переисполнения на тике

> ✅ Выполнено 2026-11-26 (коммит cf318d2): жизненный цикл заказа running→done/failed с чекпойнтом и попытками, журнал сбоев, прогон идёт дальше.

- Суть: `Scheduler.Run`/`runOne` для не-sediment заказов статус не меняет
  (`scheduler.go:152-153`, `:175-193`); skip-карта живёт один прогон — с
  тикером в минуту заказы переисполняются. Добавить жизненный цикл: перед
  исполнением `running`, после успеха `done` (+checkpoint), при ошибке
  `failed` (+attempts). `Run` не валится на первом сбое — логирует и идёт
  дальше.
- Файлы: `engine/internal/workshop/scheduler.go`,
  тест `engine/internal/workshop/scheduler_test.go`.
- Проверка: `cd engine && go test ./internal/workshop -count=1` — PASS;
  юнит: running→done, при ошибке failed, повторный тик не исполняет done/failed.

### T-21-04. Диспетчер видов: исполнитель по `Kind` вместо общего `slot.Generate`

- Суть: `runOne` спец-обрабатывает только `KindSediment`; остальные виды уходят
  в `slot.Generate`. Завести реестр исполнителей по виду и зарегистрировать:
  `horoscope` → `RunHoroscopeForge`, `pythia` → `RunPythia`,
  `epoch` → `ForgeEpoch`, `style` → `StylizeClassic`,
  `complement` → `RunForgeCard`+`ReconcileComplement`, `edit` → `RunEditorTask`;
  `analysis`/`forge_plan` — через слот.
- Файлы: `engine/internal/workshop/scheduler.go`,
  `engine/internal/workshop/order_runner.go` (новый),
  `engine/cmd/tns65/main.go` (регистрация),
  тест `engine/internal/workshop/scheduler_test.go`.
- Проверка: `cd engine && go test ./internal/workshop -count=1 && go build ./cmd/tns65` — PASS;
  юнит: заказ каждого вида исполняется своим runner и закрывается статусом.

### T-21-05. Пауза набегает от последней активности, а не от старта ядра

- Суть: `NewPauseTimer` начинает молчание с момента создания (`pause.go:76-78`);
  до первой реплики игрока ступень набегает от запуска. Инициализировать
  таймер от последней активности мира (читая `chatLastActive` или последнюю
  строку `turns.created_at`) при открытии мира воркером.
- Файлы: `engine/cmd/tns65/main.go` (`openWorld`),
  `engine/internal/webapi/handlers_chat.go` (getter),
  при необходимости `engine/internal/workshop/pause.go`, тест.
- Проверка: `cd engine && go test ./internal/workshop/ ./internal/webapi/ -count=1 && go build ./cmd/tns65` — PASS;
  юнит: инициализация от 1 ч назад даёт ступень от прошлого, а не от старта.

## Волна 3. Пифия

### T-21-06. Ночной прогон `RunPythia` из воркера

- Суть: `RunPythia` вызывается только в тестах. На ночной ступени (`PauseNight`)
  воркер вызывает `workshop.RunPythia(world, lit, day)` (идемпотентность по дню
  уже внутри).
- Файлы: `engine/cmd/tns65/main.go` (`tick`), тест `engine/internal/workshop/scheduler_test.go`.
- Проверка: `cd engine && go test ./internal/workshop -count=1` — PASS;
  юнит: на ночной ступени RunPythia вызван, на лёгкой — нет.

### T-21-07. Гороскоп по смене темы: постановка заказа Пифии

- Суть: «гороскоп живёт до смены темы, затем перековывается» (W-13.1). В
  обработчике хода определить смену темы и вызвать
  `workshop.EnsureHoroscopeOrder(world, payload)`; повтор без смены заказ не
  дублирует.
- Файлы: `engine/internal/webapi/handlers_chat.go`,
  `engine/internal/workshop/horoscope.go`, тест `engine/internal/webapi/handlers_chat_test.go`.
- Проверка: `cd engine && go test ./internal/webapi/ ./internal/workshop/ -count=1` — PASS;
  юнит: смена темы ставит один заказ horoscope, без смены — ноль.

### T-21-08. Сверка гороскопа на сцене + промах в кандидаты оседания

- Суть: ранние векторы сверяются с гороскопом (W-13.7); промах уходит в
  кандидаты оседания (W-13.6). В ходе после ранних векторов вызвать
  `workshop.MatchHoroscope`; при совпадении отдать `bank_ref`-ветку, при
  промахе поставить `SedimentCandidate` через `queueSedimentCandidates`.
- Файлы: `engine/internal/webapi/handlers_chat.go`,
  `engine/internal/workshop/horoscope_match.go`,
  `engine/internal/turn/stream.go`,
  тесты `handlers_chat_test.go`/`horoscope_match_test.go`.
- Проверка: `cd engine && go test ./internal/webapi/ ./internal/workshop/ -count=1` — PASS;
  юнит: совпадение отдаёт ветку без кандидата; промах ставит кандидата.

## Волна 4. Гефест

### T-21-09. Переплавка осевшего в форму со слотами после оседания

- Суть: после успешного оседания сцены с высоким доверием вызвать
  `RemeltScene(lit, sceneID, tags...)` (bank_pool drama trust=2).
- Файлы: `engine/internal/workshop/sediment_run.go`,
  `engine/internal/workshop/forge_pipeline.go`, тест `engine/internal/workshop/sediment_run_test.go`.
- Проверка: `cd engine && go test ./internal/workshop -count=1` — PASS;
  юнит: высокое доверие создаёт bank_pool drama trust=2, низкое — нет.

### T-21-10. Доковка персонажей пачки на тяжёлой ступени

- Суть: на тяжёлой ступени воркер по аркам (`birth/data/arcs.json`) ставит
  `complement`-заказ; диспетчер исполняет `RunForgeCard`+`ReconcileComplement`.
- Файлы: `engine/cmd/tns65/main.go`, `engine/internal/workshop/complement.go`,
  тест `engine/internal/workshop/complement_test.go`.
- Проверка: `cd engine && go test ./internal/workshop -count=1` — PASS;
  юнит: тяжёлая ступень ставит complement-заказ, лёгкая — нет.

### T-21-11. Цензор-согласование: доки/спека/тест под IMP-13

- Суть: выровнять описания с решением 15.11. Банки книг — без цензора при
  ковке; гороскоп (W-13.9) и эпоха (W-19) — с цензором. Поправить
  `DOCS/04-WORKSHOP.md` §6, `DOCS/01-PARADIGM.md` §18, W-14.4 (и W-02.7/W-02.8,
  если противоречат), тест `forge_pipeline_test.go:105`.
- Файлы: `DOCS/01-PARADIGM.md`, `DOCS/04-WORKSHOP.md`,
  `fine/04-spec/05-workshop.md`, `engine/internal/workshop/forge_pipeline_test.go`.
- Проверка: `cd engine && bash scripts/gates.sh` (docs-check) — PASS;
  `go test ./internal/workshop -run TestForge -count=1` — PASS.

## Волна 5. Варка по релевантности

### T-21-12. План ковки по релевантности пути (W-02)

- Суть: «ковать только релевантное текущему пути»: днём — вечер, ночью — утро
  и завтрашний день; 2–5 заготовок/сутки; дальняя — только при свободной
  очереди; ненужная снимается или уходит в оседание. Воркер выбирает заказы по
  плану ковки (тема/пики из гороскопа и `forge_plan`).
- Файлы: `engine/cmd/tns65/main.go`,
  `engine/internal/workshop/forge_plan.go` (новый),
  `engine/internal/workshop/queue.go`, тест `engine/internal/workshop/forge_plan_test.go`.
- Проверка: `cd engine && go test ./internal/workshop -count=1` — PASS;
  юнит: днём выбирается вечер, ночью — утро/завтра; дальняя — при свободной очереди.

## Порядок и проверка

- Волны по порядку 1 → 5; каждая задача — через workflow `tns65-task`.
- Финальный прогон: `cd engine && bash scripts/gates.sh`. После волны 1 ждём
  зелёные `TestNoShortReadyBudget` и `TestVerne`; `TestSwitchModelRollback` —
  флейк.
- Переиндексация `codebase-memory-mcp` и пуш после зелёного gates.
