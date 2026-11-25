# Проводка параметров генерации и внешний файл промптов — план починки

Дата: 2026-11-25. Ветка: `dry/20-llama-dicts` (продолжение), либо своя ветка `dry/21-gen-params`.
Аудит кода: 2026-11-25, ветка `dry/20-llama-dicts` @ `b442d9a`.

## 1. Проблема (описание кода)

### 1.1. Настройки LLM не доходят до запросов (мёртвые провода)

Парсятся из `config.yaml`, пишутся через `PUT /api/settings/config`, показываются
в «Настройках → LLM», описаны в `DOCS/10-STATION-SETTINGS.md` — и не читаются
ни одним клиентом генерации:

| Настройка | Парсинг | UI | Запрос модели |
|---|---|---|---|
| `gen_max_tokens` | `handlers_settings.go:1025` | да | нет |
| `gen_temperature` | `handlers_settings.go:1029` | да | нет |
| `gen_timeout` | `handlers_settings.go:1023` | да | нет (клиент без Timeout) |
| `gen_retries` | есть | да | нет |
| `gen_concurrency` | есть | да | нет |
| `sampling` | есть | да | нет |

Тела запросов клиентов:

- локальный weaver `models/weaver.go:66` — `{"prompt": …}` на `/completion`
  (без `n_predict`, `temperature`, `top_p`, `top_k`, `stop`);
- OpenAI `models/openai.go:73` — `{model, messages, stream}`
  (без `max_tokens`, `temperature`, `top_p`, `reasoning_effort`);
- Gemini `models/provider.go:242` — только `contents`
  (без `generationConfig`: `maxOutputTokens`, `temperature`, `topP`, `topK`,
  `thinkingConfig`).

Следствие: модель генерирует до EOS. Замер на ковке 3 книг (2026-11-24/25):
чанк «Последнего дела Холмса» шёл 2 ч 15 мин — скрытые ретраи при мусорном JSON
и безлимитная генерация abliterated-модели с thinking-выкидами.

### 1.2. Нет таймаута HTTP-клиента слота

`netgate.Local` (`netgate/gate.go:159`) возвращает `http.Client{}` без `Timeout`
(только `dialTimeout` 30s на установку соединения). Долгая генерация ждётся
бесконечно; `gen_timeout` к клиенту не применяется.

### 1.3. Промпты зашиты в код Go

`WeaverInstruction` (`models/weaver.go:48`), `composePrompt`/`ladderPrompt`
(`workshop/compose.go:189`, `workshop/generate.go:117`), промпты споров deepread
(`senseDisputePrompt`, `dispute.go`), `castRolePrompt`, horoscope/pythia/editor —
всё это константы и функции в Go. Правка промпта требует пересборки.
Внешних файлов промптов нет: `world_db/models/<role>/<model>.gguf.json` —
манифест установки, `packs/ru/refusals.txt` — паттерны цензора.

### 1.4. Состав роли и привязки — тоже заготовки

- `model_bindings` (role → model_ref, provider) читают только webapi и
  `workshop/queue.go` (для записи строки); генерация модель по роли не выбирает.
- `roleConfig` (`world_meta` ключ `role.<роль>`: temperature, prompts, priority)
  пишется/читается только в `handlers_settings.go`; до запроса не доходит.

## 2. Цель

1. **Каждый запрос генерации несёт поля**: `temperature`, `reasoning`
   (рассуждения), `gen_max_tokens`, `top_p`, `top_k`. Отсутствующий параметр
   в файле настроек = поле не отправляется = дефолт самой модели/сервера.
2. **Внешний файл промптов**: тексты промптов переносятся из Go в файл пакета;
   Go хранит дефолт на случай отсутствия файла.
3. **`gen_timeout` применяется** к HTTP-клиентам слота.

## 3. Маппинг полей по протоколам

| Поле запроса | weaver `/completion` (llama.cpp) | OpenAI `/chat/completions` | Gemini `generateContent` |
|---|---|---|---|
| генерация | `n_predict` | `max_tokens` | `generationConfig.maxOutputTokens` |
| температура | `temperature` | `temperature` | `generationConfig.temperature` |
| top_p | `top_p` | `top_p` | `generationConfig.topP` |
| top_k | `top_k` | `top_k` (не стандарт, у совместимых серверов есть) | `generationConfig.topK` |
| рассуждения | `reasoning_budget` (thinking-модели llama.cpp; проверить на установленном `llama-server`, запасной путь — `chat_template_kwargs.enable_thinking`) | `reasoning_effort` (у providers.toml уже есть поле) | `generationConfig.thinkingConfig.thinkingBudget` |

Источники значений (приоритет сверху вниз):

1. состав роли `world_meta role.<роль>` (`roleConfig`) — если задано;
2. `config.yaml` `llm:` (`gen_max_tokens`, `gen_temperature`, новые
   `gen_top_p`, `gen_top_k`, `gen_reasoning`);
3. ничего не отправлять — дефолт модели.

Точка «0 — без ограничения» из `DOCS/10-STATION-SETTINGS.md` сохраняется:
`gen_max_tokens: 0` и пустое значение = поле не отправляется.

## 4. Этапы

### П1. Тела запросов клиентов (модельный слой)

Файлы: `engine/internal/models/weaver.go`, `openai.go`, `provider.go`.

- Ввести `models.GenerateParams` (Temperature, MaxTokens, TopP, TopK, Reasoning —
  все указатели/`omitempty`; nil = не отправлять поле).
- `Weaver.Stream(ctx, prompt)` → `StreamParams(ctx, prompt, params)`; в
  `completionRequest` добавить `n_predict`, `temperature`, `top_p`, `top_k`,
  `reasoning_budget`.
- `openAIChatRequest` добавить `max_tokens`, `temperature`, `top_p`, `top_k`,
  `reasoning_effort`.
- `geminiGenerateRequest` добавить `generationConfig` (+`thinkingConfig`).
- Сохранить обратную совместимость: старый `Stream(ctx, prompt)` зовёт новый
  с пустыми params.

Тесты: тело запроса каждого клиента (httptest-сервер ловит JSON):
есть поля при заданных params; полей нет при пустых params.

### П2. Проброска params через слоты

Файлы: `workshop/queue.go` (`Slot`), `workshop/routed_slot.go`,
`workshop/generate.go`, `providerroute/slot.go`.

- Контракт слота: `Generate(ctx, role, prompt)` → `Generate(ctx, role, prompt,
  params)` (или params в `GenerateOptions` — уже есть в workshop).
- `workshop.GenerateOptions` расширяется полями params; `GenerateWithRetry` и
  `ladder` передают их в `slot.Generate`.
- `RoutedWorkshopSlot` и `providerroute.RoutedProseSlot.Stream` прокидывают params
  в клиент (`turn.ProviderSlot` → клиент).
- Чтение источника значений (П3-конфиг): `roleConfig` из `world_meta` (роль) →
  `config.yaml llm:` (станок). Чтение `roleConfig` вынести из webapi в общий
  пакет (например `internal/llmparams`), webapi остаётся потребителем.

Тесты: роль с temperature в `world_meta` доходит до тела запроса (fake-клиент
ловит params); роль без состава берёт `llm:`-дефолт; пустые оба — поля нет.

### П3. Конфиг: новые ключи и таймаут

Файлы: `engine/config.yaml`, `webapi/handlers_settings.go` (парсинг уже частично
есть), `DOCS/10-STATION-SETTINGS.md`.

- Новые ключи `llm:`: `gen_top_p`, `gen_top_k`, `gen_reasoning`
  (int budget либо строка effort — формат зафиксировать в плане реализации:
  предлагается `gen_reasoning: -1` = не отправлять, `0` = выключить рассуждения,
  `>0` = бюджет; для OpenAI-маршрута маппится в `reasoning_effort` по таблице).
- `gen_timeout` применять в `netgate.Local`/клиентах слота как `http.Client.Timeout`.
- UI «Настройки → LLM»: новые поля (top_p, top_k, reasoning).

Тесты: парсинг новых ключей; клиент слота обрывает генерацию по таймауту.

### П4. Внешний файл промптов

Файлы: `engine/packs/ru/prompts.json` (новый), `engine/packs/ru/manifest.json`
(sha256, как у словарей), загрузчик в `workshop`/`deepread`, вызывающие точки.

- Структура `prompts.json` — ключи по ролям и шагам:
  `weaver_instruction`, `compose`, `ladder.step1|step2|step3`, `cast`,
  `horoscope`, `pythia`, `editor`, `editor_digest`, `style`, `deepread.*`,
  `sense_dispute`, `dispute` (список закрыть по инвентаризации grep'ом
  `Prompt(`/`prompt` в Go).
- Загрузка: при старте пакета; текст из файла, при отсутствии ключа/файла —
  дефолт из Go (текущие константы остаются как fallback).
- Приоритет: `world_meta role.<роль>.prompts` (своя правка) → `prompts.json` →
  дефолт Go. Это же доводит до дела `roleConfig.Prompts`.
- Рендеринг: текущая форма промптов (JSON-обёртка `composePrompt`, ступени
  `ladderPrompt`) сохраняется; шаблонные места (`{chunk}`, `{text}`, `{step}`)
  выносятся в файл.

Тесты: загрузка файла; fallback без файла; override из `world_meta`; манифест
(sha256) как у словарей; `go test ./...` зелёный.

### П5. Сборка, деплой, замер

- Сборка станции, перезапуск `llama-server` не требуется (правки на стороне
  движка); перезапуск ядра.
- Smoke: `curl :5001/completion` с `n_predict=32` обрывает генерацию; ковка
  одного чанка fb2 — время падает с ~2 ч до минут.
- Правка промпта в `prompts.json` без пересборки — подтверждается на ковке.

## 5. Порядок коммитов

1. `fix(llm): параметры запроса генерации — temperature/reasoning/max_tokens/top_p/top_k` (П1+П2)
2. `fix(llm): gen_timeout в HTTP-клиентах слота + новые ключи llm:` (П3)
3. `feat(pack): внешний файл промптов packs/ru/prompts.json` (П4)
4. `docs(ledger): журнал + DOCS/10-STATION-SETTINGS.md` (П3/П5)

Каждый коммит — с описанием проблемы в теле сообщения. Ветка отдельная,
по конвенции «Сделано = зелёная проверка + коммит + строка в LEDGER».

## 6. Риски и ограничения

- `reasoning_budget` поддержан не всеми версиями llama-server — проверить
  `bin/llama-server --version` станции; при отсутствии — `chat_template_kwargs`
  либо пропуск поля (дефолт модели).
- `top_k` у внешних OpenAI-провайдеров может игнорироваться — поле безвредно.
- Двухпроходная генерация (`TwoPass`, факты+проза) получает params обеими
  проходами; лимит на проход — суммарно не более `gen_ctx`.
- Промпты в `prompts.json` попадают в sha256-манифест пакета — обновлять
  `packs/ru/manifest.json` скриптом `update-manifest.sh`.
