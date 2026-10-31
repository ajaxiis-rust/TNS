# P9 Интеграция devForge и структурная композиция — план правок движка

Волна P9. Источник — два новых документа (норма для этой волны):

- **SCR** — `fine/TNS-Structural-Composition-Requirements-v2.md` (v2, 2026-10-31):
  композиция из 14 семей, скоринг, дрейф, этапы 1–4, критерии приёмки §8.
- **TUN** — `fine/tuning.md` (2026-10-31): сверка печеных баз devForge с кодом
  движка; пункты TUN-1…TUN-9.

Формат задач — `fine/plan/_TEMPLATE.md`. Правила волны те же: атомы здесь —
пункты SCR/TUN (ссылки вида SCR-§3.1, TUN-2), гейты `cd engine && bash
scripts/gates.sh` в каждой задаче, статус — в `99-LEDGER.md`, писатели — по
карте T-01-07. Спецификация `fine/04-spec` не правится; расхождения — вопросом
в `99-LEDGER.md`.

## Порядок с приёмкой P8: сначала правки, потом приёмка

Волна P9 исполняется **до** приёмки P8 (`09-acceptance.md`), хотя номер у неё
позже: порядок задаёт «Зависит от», а не номер (правило 1 ведения плана).
Основание — состояние кода на 2026-10-31: движок ищет базу языка в
`world_db/base/base.<lang>.db` (`internal/birth/birth.go:134`), а базы
поставки лежат в `engine/base/`. **Сейчас движок с этими базами не работает**:
рождение мира на них невозможно, пока не сделаны правки этой волны.

Решение владельца 31.10 (после закрытия T-10-01): базы поставки
`base.<lang>.db` лежат в git рядом с квитенциями `*.sha256` — движок
выкладывается на GitHub целиком; `engine/.gitignore` впускает `base/*.db`
(остальные `*.db` и `*.db-wal`/`*.db-shm` остаются игнорированными). Строка
проверки T-10-01 «только `*.db` не отслежены» исполнена до этого решения и к
`engine/base/` далее не применяется.
Отсюда порядок:

1. Правки движка: T-10-02 (путь поставки), T-10-03 (язык), T-10-04
   (контрольная сумма), T-10-05 (эмбеддер) — движок начинает читать базы.
2. Проводка T-10-15: рождение мира на реальных `base.<lang>.db` всех языков —
   доказательство, что движок завёлся на базах поставки.
3. Только после этого — приёмка P8: сценарии INT-1 и INT-4 рождают миры на
   базах `engine/base/base.<lang>.db` и проверяют их поимённо.

Проверок вида «движок читает базы» до правок не назначается: сверка доставки
(ниже) — проверка файлов драйвером SQLite, не движком.

## Решение владельца о путях (2026-10-31)

Три базы — три места, поставка не смешивается с рабочими данными:

- **`engine/base/base.<lang>.db`** — базы языков: неизменяемый комплект,
  поставляется разработчиком в папке движка рядом с бинарниками `engine/bin/`
  (норма спеки D-01 «base.db — папка движка, поставляется разработчиком»,
  D-03 «заморожена, только чтение»). Движок читает и не пишет; рядом лежат
  `.sha256` и `archetypes.json` (реестр 14 семей — общий для всех языков,
  слаги в `patterns.kind` идентичны в каждой базе).
- **`world_db/worlds/<slug>/<slug>.db`** — оперативная база мира: в ней
  работает движок (жизненный цикл мира).
- **`world_db/worlds/<slug>/literary.db`** — литературная база игрока: его
  литература, заготовки и банки (D-05 «точечные заготовки», D-06).
- **`world_db/models/`** — модели; `world_db/` не содержит баз поставки.

Каталог данных `world_db/` создаётся рантаймом; базы поставки в него не
кладутся. Рабочий каталог процесса — `engine/` (все `engine/scripts/*.sh`
делают `cd engine/`), поэтому относительные пути `base` и `world_db`
разрешаются от корня движка.

## Что уже сделано до старта волны (доставка, TUN-1 — данные)

2026-10-31 базы скопированы из `devForge/work/` в `engine/base/`:

- `base.{en,ru,de,es,fr,ja,zh}.db` — 7 баз языков, по 1120–1121 нарративов;
- `base.<L>.db.sha256` — контрольные суммы (формат «голый hex + \n»);
- `archetypes.json` — словарь 14 семей `{name, gloss_en, keywords}`.

Проверка доставки (исполнитель: код):

```
cd engine/base && for L in en ru de es fr ja zh; do \
  [ "$(cat base.$L.db.sha256)" = "$(sha256sum base.$L.db | cut -d' ' -f1)" ] \
  && echo "$L OK" || echo "$L MISMATCH"; done
```

→ семь строк `OK`. Сверка данных драйвером SQLite в режиме только чтения
(`mode=ro`): `user_version=3`, 1121 вектор `dim=1024`, 14 видов
`patterns.kind`, 1380 имён `characters` — проверено 2026-10-31. Это
проверка файлов, не движка: рантайм прочитает базы после T-10-02, а
доказательство «движок завёлся на базах» — проводка T-10-15.

В git идут `archetypes.json` и 7 файлов `.sha256` (реестр и квитенции
целостности); `*.db` остаются вне git (`engine/.gitignore`, ~51 МБ данных).
Слитая легаси `base.db` из devForge не копируется — рантайм её не читает
(TUN-8). Промежуточная раскладка `world_db/base/` (первая попытка доставки)
разобрана 2026-10-31 тем же решением владельца: поставка — в папке движка.

## Диагноз (сверка с кодом, ветка `dry/fine-spec-v1`)

- Схема баз devForge байт-в-байт совпадает с миграциями движка; из base-базы
  рантайм читает только `vectors` (`internal/birth/anchor.go:
  readNarrativeVectors`); `narratives/verses/patterns/characters` — груз.
- Путь базы по умолчанию строится как `<worlds_root>/base/base.<lang>.db`
  (`internal/birth/birth.go:134`, `internal/webapi/handlers_atlas.go:144`) —
  это смешивает поставку с рабочими данными и противоречит D-01; правится
  задачей T-10-02.
- `anchor_narrative_id` пишется в `world_meta` и дальше никем не читается.
- `ancketa.Validate` поле `lang` не проверяет (TUN-2); `.sha256` при открытии
  не проверяется (TUN-9).
- `models.Client` постит `{"input"}` в корень `:5002`; devForge поднимает
  llama-server с контрактом `/v1/embeddings` (TUN-3).
- Выбор драмы в живом ходе — теги `bank_pool` через `bank.Pick` (пересечение
  тегов), единственный «архетип» — юнгианский в карточках персонажей
  (`internal/birth/data/archetypes.json`, 12 имён) — это другой слой, SCR-§9
  запрещает его смешивать со структурным.
- Драматические артефакты живут в `literary.db → bank_pool (kind='drama')`,
  пишет `forge` (ковка/стиль/переплавка). Полей композиции нет.
- `patterns.kind` в базах = слаги 14 семей (проверено по base.ru.db) — канон
  для сопоставления нарративов между языками (TUN-6).
- Легаси `cmd/tns65-bake` пишет слитую базу по умолчанию в
  `world_db/base/base.db` (`cmd/tns65-bake/main.go:19`) — выводится из
  употребления задачей T-10-14.

## Карта задач

Порядок исполнения задаёт «Зависит от». Нумерация: `T-10-NN`.

- [ ] **T-10-01** Реестр 14 семей: пакет `internal/structural`, загрузчик `engine/base/archetypes.json`
- [ ] **T-10-02** Каталог поставки `engine/base/`: путь базы языка по умолчанию (развязка с `world_db/`)
- [ ] **T-10-03** Валидация языка анкеты по установленным базам (TUN-2)
- [ ] **T-10-04** Контрольная сумма базы языка при рождении (TUN-9)
- [ ] **T-10-05** Клиент эмбеддера `/v1/embeddings` (TUN-3)
- [ ] **T-10-06** Потребитель якоря: материализация сценарной рамки (TUN-4, TUN-6)
- [ ] **T-10-07** Назначение имён NPC из пула базы языка (TUN-5)
- [ ] **T-10-08** Миграция literary 0003: колонки композиции в `bank_pool` (SCR-§6.1, Этап 1)
- [ ] **T-10-09** Скоринг композиции: формула, жёсткие правила, конфиг (SCR-§3.1–3.2, Этап 1)
- [ ] **T-10-10** Миграция world 0012: счётчики дрейфа `family_usage` (SCR-§3.3, §6.2, Этап 2)
- [ ] **T-10-11** Стадия композиции в конвейере хода (SCR-§3, Этап 1–2; приёмка §8.1–3, §8.6–7)
- [ ] **T-10-12** Ковка с композицией: классификация семьи кодом + извлечение слотом (SCR-§5, Этап 3)
- [ ] **T-10-13** Разведение близких пар и фиксация набора 14 (SCR-§3.2.5, §7-Этап 4, §10)
- [ ] **T-10-14** Легаси `tns65-bake` и сообщения `base.<lang>.db` (TUN-7, TUN-8)
- [ ] **T-10-15** Приёмочная проводка: движок заводится на базах поставки — входное условие волны P8

Срезы: T-10-01…05 + T-10-15 — «поставка на месте и движок заводится на новых
базах» (T-10-15 — входной билет приёмки P8); T-10-06…07 — «база перестаёт
быть мёртвым грузом»; T-10-08…11 — Этапы 1–2 SCR (композиция + дрейф в живом
ходе); T-10-12 — Этап 3 (обогащение из книг); T-10-13–14 — Этап 4 и косметика.

---

### T-10-01. Реестр 14 семей: пакет internal/structural
- **Атомы:** SCR-§1.2 (набор 14), SCR-§8.8 (набор зафиксирован), TUN-6 (слаги — канон).
- **Выдержки:** «Текущий набор Family (14): escape_liberation … teaching_discourse»; «Набор Family зафиксирован на 14 (текущий список)»; «сравнивать по английским slug'ам как канону».
- **Зависит от:** —.
- **Читать:** `fine/TNS-Structural-Composition-Requirements-v2.md` §1.2, §10; `engine/base/archetypes.json`.
- **Создаётся:** `engine/internal/structural/doc.go`, `engine/internal/structural/family.go`, `engine/internal/structural/family_test.go`, `engine/internal/structural/testdata/archetypes.json` (копия боевого файла, 14 семей).
- **Правится:** `engine/ARCHITECTURE.md` (эталон каталогов — строка `structural`).
- **Писатель:** нет записи.
- **Шаги:**
  1. Завести пакет `internal/structural` с типом контракта: `type Family struct { Name, GlossEN string; Keywords []string }` и `func LoadFamilies(path string) ([]Family, error)`.
  2. Зафиксировать контракт валидации: ровно 14 записей; имена уникальны; `name` соответствует `^[a-z_]+$`; `keywords` непуст; резерв `none` семейством не является (словарная запись `none` при встрече — ошибка валидации).
  3. Зафиксировать канонический порядок = порядок файла; загрузчик не сортирует.
  4. Добавить `func FamiliesPath(engineRoot string) string` → `filepath.Join(engineRoot, "base", "archetypes.json")` — единая точка пути; `engineRoot` — тот же корень, от которого берётся `db.BaseRoot` (T-10-02).
  5. Вписать `structural` в эталон каталогов `ARCHITECTURE.md`.
  6. `git add base/archetypes.json base/*.sha256` — реестр и квитенции попадают в репозиторий (прецедент `seed/gazetteer.jsonl`, T-01-16).
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/structural TestLoadFamilies` → `PASS` (14 семей из testdata, `none` отвергнут, дубль имени отвергнут); `git status --short base/` → только `*.db` не отслежены. Гейты `bash scripts/gates.sh` → exit 0.
- **Исполнитель:** код.

### T-10-02. Каталог поставки engine/base/: путь базы языка по умолчанию
- **Атомы:** D-01 (данные: base.db — папка движка), D-03 (заморожена, только чтение); решение владельца 2026-10-03 «поставка рядом с бинарниками, не смешивать с world_db».
- **Выдержки:** «base.db — база движка: лежит в папке движка, поставляется разработчиком»; «Данные: base.db — папка движка; literary.db — папка мира; world(name).db — папка мира»; «после сборки base.db заморожена: записи, правки и пересчёт векторов в контурах игры запрещены».
- **Зависит от:** —.
- **Читать:** `fine/04-spec/01-data.md` D-01, D-03; `engine/internal/birth/birth.go` (шаг baseDBPath), `engine/internal/webapi/handlers_atlas.go` (`atlasBirthConfig`), `engine/internal/db/worldroot.go`.
- **Создаётся:** `engine/internal/db/baseroot.go` (по образцу `worldroot.go`), `engine/internal/db/baseroot_test.go`.
- **Правится:** `engine/internal/birth/birth.go`, `engine/internal/webapi/handlers_atlas.go`, `engine/internal/db/worldroot.go` (комментарий: `base/` — поставка, `world_db/` — рабочие данные), `engine/ARCHITECTURE.md` (раскладка: строка про `engine/base/`).
- **Писатель:** нет записи.
- **Шаги:**
  1. Контракт: `var BaseRoot = "base"` в `internal/db` — каталог поставки баз языков, относителен корню движка (рантайм-каталог процесса — `engine/`); параметризуется переменной для тестов, как `WorldsRoot`.
  2. `birth.go`: при пустом `cfg.BaseDBPath` путь базы — `filepath.Join(db.BaseRoot, "base."+a.Lang+".db")`; каталог `world_db/` базами не занимается.
  3. `handlers_atlas.go` (`atlasBirthConfig`): тот же источник пути — `db.BaseRoot`, а не `filepath.Join(worldsRoot, "base", …)`.
  4. Открытие — только `OpenReader` (query_only, D-03): проверить, что в коде нет `OpenWriter*` для базы языка (контракт-тест: попытка открыть writer-хэндлом base-базу не компилируется/отвергается картой писателей — все таблицы base уже принадлежат `bake`, живой писатель в карте один, запись из игры невозможна).
  5. Комментарии в `worldroot.go` и `baseroot.go` проговаривают развязку: `base/` — поставка разработчика рядом с `bin/`; `world_db/worlds/<slug>/` — оперативная и литературная базы мира; `world_db/models/` — модели.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/birth TestBirthBaseRootDefault` → `PASS`: пустой `BaseDBPath` + фикстура `base/base.<lang>.db` под `db.BaseRoot` → якорь прочитан из неё; под `WorldsRoot` каталог `base/` не создаётся. `grep -rn 'Join(root, "base"' internal/ | grep -v _test` → вывод пуст. Гейты → exit 0.
- **Исполнитель:** код.

### T-10-03. Валидация языка анкеты по установленным базам
- **Атомы:** TUN-2 (блокирующее).
- **Выдержки:** «`birth/ancketa.Validate` поле `lang` не проверяет вовсе… Список допустимых языков = набор установленных баз (en, ru, de, es, fr, ja, zh)».
- **Зависит от:** T-10-02 (каталог поставки и путь по умолчанию).
- **Читать:** `fine/tuning.md` п.2; `engine/internal/birth/birth.go` (шаги 2–6), `engine/internal/birth/ancketa.go`.
- **Создаётся:** `engine/internal/birth/langbase.go`, `engine/internal/birth/langbase_test.go`.
- **Правится:** `engine/internal/birth/birth.go` (шаг цепочки до создания каталога мира), `engine/internal/birth/birth_test.go` (новый кейс).
- **Писатель:** нет записи (только чтение каталога; мир не создаётся).
- **Шаги:**
  1. Контракт: `func AvailableLangs() ([]string, error)` — имена файлов `base.<L>.db` в `db.BaseRoot`; легаси-слитая `base.db` в список не входит; сортировка по алфавиту.
  2. Контракт: `func ValidateLang(a *Ancketa) *FieldError` — пустой `lang` → `FieldError{Field:"lang", Reason:"язык не указан"}`; `lang` вне `AvailableLangs()` → `FieldError{Field:"lang", Reason:"язык не установлен: нет базы base.<lang>.db"}`.
  3. Встроить проверку в `Birth` первой (до `MkdirAll` мира): ошибка языка даёт `ExitError{Code: ExitInvalid}` с перечнем полей — тот же контракт кода 2, что у `Validate` (T-02-09).
  4. Если `cfg.BaseDBPath != ""` (явный путь, тесты/атлас) — вместо списка проверять существование этого файла: `os.Stat` → отказ `FieldError{Field:"lang", Reason:"база языка не найдена: <путь>"}`.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/birth TestValidateLang` → `PASS`: пустой lang и `klingon` дают код 2 с полем `lang` и не создают каталога; анкета с `en` при фикстуре `base/base.en.db` под `db.BaseRoot` проходит до шага pack. Гейты → exit 0.
- **Исполнитель:** код.

### T-10-04. Контрольная сумма базы языка при рождении
- **Атомы:** TUN-9 (дешёвая проверка целостности).
- **Выдержки:** «`.sha256` при открытии base-базы никто не проверяет — дешёвая проверка целостности при рождении мира защитила бы от недокопированных баз».
- **Зависит от:** T-10-03.
- **Читать:** `fine/tuning.md` п.9; `engine/internal/birth/anchor.go`.
- **Создаётся:** `engine/internal/birth/checksum.go`, `engine/internal/birth/checksum_test.go`.
- **Правится:** `engine/internal/birth/langbase.go` (вызов после проверки языка), `engine/internal/journal/events.go` (константа `birth.base_corrupt`).
- **Писатель:** нет записи.
- **Шаги:**
  1. Контракт: `func VerifyBaseChecksum(dbPath string) error` — читает `dbPath + ".sha256"` (формат: голый hex, возможен `\n`), считает SHA-256 файла базы, сверяет. Файл суммы отсутствует → ошибка `нет контрольной суммы базы <путь>`; не совпал → `база языка повреждена (sha256)`.
  2. Вызов в шаге langbase сразу после проверки существования базы; ошибка — отказ рождения кодом 2 (поле `lang`), каталог мира не создаётся.
  3. Событие `birth.base_corrupt` в журнал при несовпадении (литералы имён событий запрещены — только константа).
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/birth TestVerifyBaseChecksum` → `PASS`: подменённый байт в копии базы → отказ с `sha256` в причине и событием в журнале; чистая фикстура проходит. Гейты → exit 0.
- **Исполнитель:** код.

### T-10-05. Клиент эмбеддера /v1/embeddings
- **Атомы:** TUN-3 (контракт эмбеддера, обязательное окружение).
- **Выдержки:** «devForge поднимает llama-server с OpenAI-контрактом `/v1/embeddings` (`{"input":[...],"model":"bge-m3"}` → `data[0].embedding`). Если на `:5002` встанет тот же llama-server без фасада — клиент движка адаптировать под `/v1/embeddings`. Модель обязана быть тот же bge-m3 (1024)».
- **Зависит от:** —.
- **Читать:** `fine/tuning.md` п.3; `engine/internal/models/embed.go`; `engine/config.yaml` (секция `llm`).
- **Создаётся:** `engine/internal/models/embed_test.go`.
- **Правится:** `engine/internal/models/embed.go`.
- **Писатель:** нет записи.
- **Шаги:**
  1. Контракт запроса: `POST <embed_addr>/v1/embeddings`, тело `{"input":["<текст>"],"model":"<модель>"}`; модель — из конфига `llm.embed_model`, пусто → `bge-m3`.
  2. Контракт ответа: `{"data":[{"embedding":[1024 float]}]}` — берётся `data[0].embedding`; размерность строго `EmbedDim=1024`, иначе ошибка `embedder returned dim N, want 1024` (существующий текст ошибки сохраняется).
  3. L2-нормализация, `embed.fail` в журнал, транспорт только `netgate.Local` — без изменений контракта.
  4. Обновить комментарий `Client` (контракт llama-server, а не фасад devForge).
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/models TestEmbedV1` → `PASS`: httptest-сервер требует путь `/v1/embeddings`, массив `input` и поле `model`; ответ `data[0].embedding` длиной 1024 нормируется; dim 384 → ошибка. Гейты → exit 0.
- **Исполнитель:** код.

### T-10-06. Потребитель якоря: материализация сценарной рамки
- **Атомы:** TUN-4, TUN-6; SCR-§4 (библейский хребет остаётся поставщиком).
- **Выдержки:** «По `anchor_narrative_id` никто не берёт ни биты (`verses`), ни дугу (`patterns`). Нужен шаг, превращающий якорный нарратив в сценарный материал»; «сравнивать по английским slug'ам как канону (`escape_liberation`, …)».
- **Зависит от:** T-10-04.
- **Читать:** `fine/tuning.md` п.4, п.6; `engine/internal/birth/anchor.go`, `engine/internal/birth/birth.go` (цепочка шагов).
- **Создаётся:** `engine/internal/birth/anchormat.go`, `engine/internal/birth/anchormat_test.go`.
- **Правится:** `engine/internal/birth/birth.go` (шаг после anchor).
- **Писатель:** `forge` (ключи `world_meta`).
- **Шаги:**
  1. Контракт чтения (база языка, `OpenReader`): `SELECT name, arc_summary FROM narratives WHERE id=?`; `SELECT text FROM verses WHERE narrative_id=? ORDER BY ord LIMIT ?`; `SELECT DISTINCT kind FROM patterns WHERE narrative_id=?`.
  2. Контракт записи — четыре ключа `world_meta` одним шагом хэндлом `forge`: `anchor_name` (TEXT), `anchor_summary` (TEXT), `anchor_verses` (JSON-массив строк, лимит 12 — константа `anchorVersesLimit`), `anchor_families` (JSON-массив слагов семей из `patterns.kind`, канон TUN-6).
  3. Деградация без отказа: пустые `verses`/`patterns` дают `[]` — рождение не падает (якорь всегда назначен, B-10).
  4. Отсутствие нарратива по id (битая база) — отказ рождения с внятной ошибкой `якорный нарратив <id> не найден в базе языка` (дефект поставки базы, тишины нет).
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/birth TestAnchorMaterial` → `PASS`: фикстура базы с 1 нарративом, 3 verses, 2 разными `kind` → в `world_meta` 4 ключа с ожидаемым содержимым; фикстура без patterns → `anchor_families` = `[]`; мир рождён. Гейты → exit 0.
- **Исполнитель:** код.

### T-10-07. Назначение имён NPC из пула базы языка
- **Атомы:** TUN-5; B-05.3 (имя назначается в живом ходе).
- **Выдержки:** «`characters` в base (1380 имён на язык, `lang`, `traits_json.significance`) не потребляется… Завести шаг назначения имён из `base.<lang>.db` — миграция `characters.lang` (0003) уже этого ждёт».
- **Зависит от:** T-10-06.
- **Читать:** `fine/tuning.md` п.5; `engine/internal/turn/troupe.go`; `engine/internal/turn/stages.go` (труппа, предпуть).
- **Создаётся:** `engine/internal/turn/namepool.go`, `engine/internal/turn/namepool_test.go`.
- **Правится:** `engine/internal/turn/troupe.go` (Spawn с пустым именем), `engine/internal/turn/pipeline.go` (проводка `BaseDBPath` и `Lang`), `engine/internal/webapi/handlers_chat.go` (сбор пути базы языка через `db.BaseRoot`).
- **Писатель:** `core` (`UPDATE characters SET name` оперативной базы мира).
- **Шаги:**
  1. Контракт пула: `func PickPoolName(basePath, lang string, taken map[string]bool) (string, error)` — читает `SELECT name, traits_json FROM characters WHERE lang=?`; детерминированный порядок: `significance='major'` раньше прочих, внутри групп — по `id`; имя из `taken` пропускается; пул исчерпан → пустая строка без ошибки (имя остаётся назначаться позже).
  2. `troupe.Spawn`: если имя карточки пусто (B-05.3) — взять из пула и записать `UPDATE characters SET name=? WHERE id=?` хэндлом `core` той же транзакцией, что и появление NPC.
  3. `Pipeline` получает поля `BaseDBPath`, `Lang` (пусто → шаг пула пропущен, деградация без имени — текущее поведение).
  4. webapi: путь базы языка — `filepath.Join(db.BaseRoot, "base."+lang+".db")`, `lang` — из `world_meta` ключа `lang` мира.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/turn TestNamePool` → `PASS`: фикстура базы с именами двух групп значимости → первым взято major-имя; занятые имена пропущены; два Spawn подряд дают разные имена; без базы (`BaseDBPath=""`) NPC появляется без имени, отказов нет. Гейты → exit 0.
- **Исполнитель:** код.

### T-10-08. Миграция literary 0003: колонки композиции в bank_pool
- **Атомы:** SCR-§6.1 (Этап 1: «ввести поля композиции в схему артефактов»), SCR-§4 (старые артефакты живут в деградации).
- **Выдержки:** «family_primary: string // обязательно, одна из 14; pattern_secondary: string?; technique: string?; moral_stance: string?; inversion_flag: bool // default false; usage_count: int».
- **Зависит от:** T-10-01.
- **Читать:** `fine/TNS-Structural-Composition-Requirements-v2.md` §6.1, §4; `engine/internal/db/schema/literary/0001_init.sql`.
- **Создаётся:** `engine/internal/db/schema/literary/0003_composition.sql`.
- **Правится:** `engine/internal/db/writers.go` (без изменений: bank_pool уже forge), `engine/ARCHITECTURE.md` (реестр миграций: `0003_composition | literary | T-10-08`), `engine/internal/db/literary_test.go`.
- **Писатель:** `forge` (bank_pool).
- **Шаги:**
  1. DDL-контракт (аддитивно, значения по умолчанию — деградация старых записей SCR-§4): `ALTER TABLE bank_pool ADD COLUMN family_primary TEXT NOT NULL DEFAULT ''`; `ADD COLUMN pattern_secondary TEXT`; `ADD COLUMN technique TEXT`; `ADD COLUMN moral_stance TEXT`; `ADD COLUMN inversion_flag INTEGER NOT NULL DEFAULT 0 CHECK (inversion_flag IN (0,1))`; `ADD COLUMN usage_count INTEGER NOT NULL DEFAULT 0`.
  2. CHECK списка 14 семей в DDL не делается: реестр — данные (`archetypes.json`), а не DDL; принадлежность семьи реестру проверяет писатель (T-10-12) и читатель (T-10-11).
  3. Комментарий в DDL: `usage_count` инкрементирует контур forge (ковка/переплавка/подъём Гефеста); счётчики использования в живом ходе живут в оперативной базе (T-10-10) — один писатель на таблицу.
  4. Реестр миграций и тест схемы: `user_version=3` литературной базы, новые колонки читаются/пишутся, старые записи после миграции имеют `family_primary=''`, `inversion_flag=0`.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/db TestLiterarySchema` → `PASS` (`user_version=3`, 6 новых колонок, деградационные дефолты на старой фикстуре). Гейты → exit 0.
- **Исполнитель:** код.

### T-10-09. Скоринг композиции: формула, жёсткие правила, конфиг
- **Атомы:** SCR-§2.2–2.3 (композиция, техники, моральные позиции), SCR-§3.1–3.2 (Этап 1: «логика выбора на композиционный скоринг»).
- **Выдержки:** формула `score = 0.28*family_match + 0.18*secondary_match + 0.15*technique_fit + 0.12*moral_stance_fit + 0.10*mood_match + 0.10*quality + 0.07*freshness_penalty`; «Вес primary family не должен превышать ~0.30»; жёсткие правила 1–5 (§3.2).
- **Зависит от:** T-10-01, T-10-08.
- **Читать:** SCR §2, §3; `engine/internal/selector/selector.go` (стиль таблицы решений); `engine/config.yaml` (секции `confirm`, `saw` — образец конфига).
- **Создаётся:** `engine/internal/structural/composition.go`, `engine/internal/structural/score.go`, `engine/internal/structural/score_test.go`.
- **Правится:** `engine/config.yaml` (секция `composition`).
- **Писатель:** нет записи (чистые функции).
- **Шаги:**
  1. Контракт типов: `type Composition struct { FamilyPrimary, PatternSecondary, Technique, MoralStance string; Inversion bool; Mood string }`; канонические закрытые списки `Techniques` (10 значений SCR-§2.3: slow_revelation … didactic_exhortation) и `MoralStances` (6: redemptive, tragic, ironic, ambivalent, cynical, moralistic); значение вне списка — ошибка валидации, маппинга «к ближайшему» нет.
  2. Контракт формулы (все веса — из конфига, дефолты SCR): `w_family=0.28, w_secondary=0.18, w_technique=0.15, w_moral=0.12, w_mood=0.10, w_quality=0.10, w_freshness=0.07`; `w_family ≤ 0.30` — инвариант загрузки конфига, нарушение — ошибка старта.
  3. Контракт слагаемых (детерминированные, без LLM): `family_match` — пересечение ключевых слов запроса с `Keywords` семьи кандидата + бонус 0.2, если семья входит в `anchor_families` мира (T-10-06), кламп [0,1]; `secondary_match` — 1.0 при совпадении с семейными ожиданиями запроса, 0.25 при наличии любого корректного secondary, 0 при пустом; `technique_fit`/`moral_stance_fit` — 1.0/0.5/0 (в списке ожиданий / задан / пуст); `mood_match` — 1.0 при совпадении с настроением запроса, иначе 0.5; `quality` — `trust_level/2` записи банка; `freshness` — `1/(1+usage_chapter[family])`.
  4. Жёсткие правила SCR-§3.2 как код-контракт: primary обязателен (пустая → запись не кандидат драмы); secondary ≠ primary; family-only штраф `family_only_penalty` (дефолт 0.10) при наличии в пуле richer-кандидатов; `inversion=true` даёт бонус `inversion_boost` (дефолт 0.10) только при флаге «ломать ожидание»; близкие пары — задача T-10-13.
  5. Секция `composition` в `config.yaml` с этими ключами и комментариями-ссылками на SCR-§3.
  6. Золотой пример-контракт в тесте: фиксированные вход/выход с точностью 1e-9 (числа фиксирует тест, не прогон).
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/structural TestScoreComposition` → `PASS` (золотой пример, все жёсткие правила, инвариант весов, порог 0.30). Гейты → exit 0.
- **Исполнитель:** код.

### T-10-10. Миграция world 0012: счётчики дрейфа family_usage
- **Атомы:** SCR-§3.3, SCR-§6.2 (Этап 2: «счётчики использования семей», «Сброс chapter-счётчиков — при начале новой главы»).
- **Выдержки:** «В `world(name).db` ведутся счётчики: `family_usage_chapter[family]`, `family_usage_book[family]`; «Чем выше счётчик семьи в текущей главе — тем сильнее штраф»; «После N использований одной семьи в главе (рекомендуется N = 3–4) система почти запрещает брать её снова»; «Механика полностью детерминированная».
- **Зависит от:** T-10-01.
- **Читать:** SCR §3.3, §6.2; `engine/internal/db/schema/world/0011_world_lexicon.sql`; `engine/internal/book/wake.go` (`OpenChapter`).
- **Создаётся:** `engine/internal/db/schema/world/0012_structural_drift.sql`, `engine/internal/structural/drift.go`, `engine/internal/structural/drift_test.go`.
- **Правится:** `engine/internal/db/schema/world/tables.txt` (+`family_usage`), `engine/internal/db/writers.go` (`family_usage → core`), `engine/internal/db/world_test.go` (`user_version=12`), `engine/internal/book/wake.go` (сброс chapter-счётчиков в `OpenChapter`), `engine/internal/book/wake_test.go`, `engine/ARCHITECTURE.md` (реестр миграций).
- **Писатель:** `core`.
- **Шаги:**
  1. DDL-контракт: `CREATE TABLE family_usage (scope TEXT NOT NULL CHECK (scope IN ('chapter','book')), family TEXT NOT NULL, count INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL, PRIMARY KEY (scope, family))`.
  2. Контракт функций: `BumpUsage(h *db.Handle, scope, family string, now string) error` (апсерт +1); `Usage(h *db.Handle, scope string) (map[string]int, error)`; `ResetScope(h *db.Handle, scope string, now string) error` — удаляет строки scope; всё через хэндл `core`, мимо карты писателей — `ErrForeignWriter`.
  3. `book.OpenChapter` вызывает `ResetScope(h, "chapter", now)` — граница главы SCR-§6.2; book-счётчики не сбрасываются.
  4. Правило «почти запрещает» — контракт параметра `drift_forbid_after` (дефолт 3) в секции `composition`: при `usage_chapter[family] ≥ N` семья исключается из кандидатов, если есть хоть один кандидат другой семьи; если нет — берётся наименее используемая (отказ хода запрещён).
  5. Правило «периодически обязана» — контракт `force_every` (дефолт 5): счётчик выборов `world_meta` ключ `composition_pick_seq` (писатель core); при `seq % force_every == 0` выбор обязан взять кандидата с `inversion=true` или наименьший `usage_book` — детерминированно.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/structural TestDriftCounters` → `PASS` (bump/апсерт, reset только chapter, `user_version=12`, писатель core, чужой писатель — отказ); `bash scripts/runtests.sh ./internal/book TestOpenChapterResetsDrift` → `PASS`. Гейты → exit 0.
- **Исполнитель:** код.

### T-10-11. Стадия композиции в конвейере хода
- **Атомы:** SCR-§3 (Этапы 1–2), SCR-§8.1–3, §8.6–7 (критерии приёмки: выбирается композиция; drift влияет; повторяемость падает; скорость живого хода не деградирует).
- **Выдержки:** «В живом ходе выбирается композиция, а не одиночный архетип (кроме случаев деградации)»; «Structural Drift реально влияет на выбор»; «Живой ход не получает заметной деградации по скорости»; «тяжёлая работа — в мастерской».
- **Зависит от:** T-10-06, T-10-08, T-10-09, T-10-10.
- **Читать:** SCR §3; `engine/internal/turn/stages.go` (реестр 107–122), `engine/internal/turn/stages.go` (`sceneTags`), `engine/internal/selector/selector.go`; `engine/internal/turn/weaver_path.go`.
- **Создаётся:** `engine/internal/turn/composition.go`, `engine/internal/turn/composition_test.go`.
- **Правится:** `engine/internal/turn/stages.go` (реестр + `sceneTags`), `engine/internal/turn/pipeline.go` (поля `LitDB`, `DriftDB`), `engine/internal/journal/events.go` (константа `turn.composition`), `engine/internal/webapi/handlers_chat.go` (проводка хэндлов).
- **Писатель:** `core` (family_usage, world_meta `composition_pick_seq`); банк не пишется.
- **Шаги:**
  1. Новая стадия `compositionStage` в реестре между `prepathStage` и `confirmStage`: кандидаты — `bank_pool` строки `kind='drama'` с непустым `family_primary` (валидной по реестру); запрос — из `Prepath` (намерение/настроение) + `anchor_families` + карта Пифии (`player_atlas`, mood/interest) — всё чтением, без LLM.
  2. Скоринг — `structural.Score` (T-10-09) с дрейфом (T-10-10); выбранная композиция — в `TurnState.Composition` (family_primary, secondary, technique, moral_stance, inversion, mood).
  3. Деградация SCR-§4: нет кандидатов с композицией (все family-only/старые) → `TurnState.Composition` нулевая, ход идёт прежним теговым путём; отказ хода запрещён.
  4. `sceneTags` дополняется слагом выбранной семьи — Челнок по-прежнему выбирает по тегам, теперь с семейным тегом; путь рутины не меняется.
  5. Путь Ткача: в инструкцию промпта (`BuildPrompt` не меняет сигнатуру) добавляется строка-руководство выбранных значений — текст-контракт фиксирован в задаче: одна строка `приём: <technique>; позиция: <moral_stance>; слом ожидания: да/нет`.
  6. Коммит: `commitStage` после успешного хода делает `BumpUsage(chapter)` и `BumpUsage(book)` выбранной семьи + инкремент `composition_pick_seq` — одной транзакцией core.
  7. Событие `turn.composition` в журнал: family, secondary, technique, moral_stance, inversion, score, seq (наблюдаемость риска №4 — решение видно).
  8. Бюджет скорости: выборка кандидатов одним SQL (`WHERE kind='drama' AND family_primary != ''`), скоринг в памяти; тест фиксирует бюджет как у Челнока — 100 мс на инжектированных часах.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/turn TestCompositionStage` → `PASS`: (а) пул из 3 драм-записей — выбрана лучшая по золотой формуле, событие в журнале, счётчики выросли; (б) та же семья 3 раза подряд → на 4-м ходе выбран кандидат другой семьи; (в) `seq % 5 == 0` → взят инвертированный/редкий кандидат; (г) пул без композиции → нулевая композиция, ход доставлен, счётчики не тронуты; (д) бюджет ≤ 100 мс на часах testkit. Гейты → exit 0.
- **Исполнитель:** код.

### T-10-12. Ковка с композицией: семья кодом, слоты извлекают остальное
- **Атомы:** SCR-§5 (Этап 3: «расширить извлечение в мастерской», «начать ковать полноценные композиции»), SCR-§8.5 («новые артефакты из мастерской содержат technique и moral_stance»).
- **Выдержки:** «При обработке книг мастерская должна извлекать и сохранять: 1. Привязку к Family (из 14). 2. Secondary Pattern / micro-pattern. 3. Technique. 4. MoralStance. 5. Признаки inversion»; «Даже слабая локальная модель достаточна для классификации и извлечения паттернов».
- **Зависит от:** T-10-01, T-10-08, T-10-09.
- **Читать:** SCR §5; `engine/internal/workshop/forge_pipeline.go` (`ForgeMaterial`, `ForgeBankRecord`, `writeBankRecord`), `engine/internal/workshop/style.go`, `engine/internal/workshop/forge_input.go`; образец строгого JSON — `engine/internal/workshop/pythia.go` (`PythiaOutput`).
- **Создаётся:** `engine/internal/workshop/compose.go`, `engine/internal/workshop/compose_test.go`.
- **Правится:** `engine/internal/workshop/forge_pipeline.go` (поля материала, запись колонок), `engine/internal/workshop/style.go` (стилизация пишет композицию, если извлечена).
- **Писатель:** `forge` (bank_pool, включая `usage_count` при ковке/переплавке).
- **Шаги:**
  1. Классификация семьи — кодом, без LLM (как `classify` в devForge): `func ClassifyFamily(text string, fams []Family) string` — поиск по границам слова по `Keywords`, победитель по числу попаданий; ничья/ноль → пустая строка (деградация family-only, тайного фолбэка нет — строка `forge_log`).
  2. Расширение `ForgeMaterial` полями композиции (контракт: family задан кодом шага 1; secondary/technique/moral_stance/inversion — слотом).
  3. Новый слот-шаг `compose` (роль `compose`, слот локальный `:5001`): промпт чанка книги → строгий JSON `{"family_primary": "...", "pattern_secondary": "...", "technique": "...", "moral_stance": "...", "inversion": bool}`; валидация по реестру и закрытым спискам; мусор/тайм-аут — ≤2 повторов, затем запись без композиции + `forge_log` (гейт «отказы модельных шагов», ход игрока не затронут).
  4. `family_primary` слота обязан совпасть с классификацией кода шага 1 либо быть `none`-пустым; расхождение — берётся кодовое значение, событие в `forge_log` (код главнее, как словарь в T-03-04).
  5. `writeBankRecord` пишет все 6 колонок; `secondary ≠ primary` и принадлежность реестру проверяются до записи — нарушение → отказ ковки с причиной.
  6. Идемпотентность сохраняется: тот же `origin` — та же запись, повторная ковка поля не перезаписывает.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/workshop TestForgeComposition` → `PASS`: чанк с ключевыми словами `escape/flee/deliver` → family `escape_liberation` кодом; валидный JSON слота → все колонки записаны; secondary==primary → отказ; мусор слота дважды → запись family-only + строка forge_log; повтор origin — без перезаписи. Гейты → exit 0.
- **Исполнитель:** код (валидация), модель (извлечение — по фейковому слоту testkit).

### T-10-13. Разведение близких пар и фиксация набора 14
- **Атомы:** SCR-§3.2.5, SCR-§7-Этап 4 («ужесточить разведение близких пар»), SCR-§10 (замечания по словарю), SCR-§8.8.
- **Выдержки:** «Близкие пары (`escape_liberation`/`rescue`, `wisdom_counsel`/`teaching_discourse`) не должны часто выступать как primary + secondary друг друга без явной причины»; «Набор Family зафиксирован на 14 (текущий список)»; «Gloss для `rise_fall_rise` лучше сделать полным».
- **Зависит от:** T-10-09, T-10-12.
- **Читать:** SCR §3.2, §7, §10; `engine/internal/structural/score.go`.
- **Создаётся:** `engine/internal/structural/adjacent_test.go`.
- **Правится:** `engine/internal/structural/score.go`, `engine/internal/structural/family.go` (хеш реестра), `engine/config.yaml` (ключ `composition.adjacent_penalty`), `engine/internal/birth/birth.go` (запись отпечатка реестра).
- **Писатель:** `forge` (`world_meta` ключ `family_registry_sha256` при рождении).
- **Шаги:**
  1. Контракт близких пар в пакете (не в конфиге — состав пар часть канона SCR): `AdjacentPairs = [escape_liberation↔rescue, wisdom_counsel↔teaching_discourse]`.
  2. Штраф `adjacent_penalty` (дефолт −0.15, конфиг) в скоринге, когда primary+secondary образуют близкую пару; «явная причина» = режим «ломать ожидание» (флаг стадии) — тогда штраф не применяется.
  3. Отпечаток реестра: `func RegistrySHA256(fams []Family) string` — sha256 канонического JSON; пишется в `world_meta` ключ `family_registry_sha256` при рождении; при несовпадении с текущим реестром в живом ходе — событие `turn.composition` с полем `registry_changed=true` (набор зафиксирован, тихая смена словаря видна).
  4. Правки gloss (`Rise, Fall and Rise`) и границ семей — вне движка: артефакт devForge `work/archetypes.json` + перепек баз; в план не входят, фиксируются вопросом в `99-LEDGER.md` на владельца devForge (набор 14 при этом не меняется).
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/structural TestAdjacentPairs` → `PASS` (пара штрафуется, независимая пара нет, режим слома ожидания снимает штраф, хеш реестра стабилен). Гейты → exit 0.
- **Исполнитель:** код; п.4 — владелец.

### T-10-14. Легаси tns65-bake и сообщения base.<lang>.db
- **Атомы:** TUN-7, TUN-8 (косметика и легаси).
- **Выдержки:** «Сообщения/комментарии `birth/anchor` говорят «base.db» — обновить формулировки»; «Легаси-пекарь `cmd/tns65-bake` + `internal/bake` (сид → слитая base.db) заменён devForge — вывести из употребления или пометить легаси».
- **Зависит от:** T-10-02.
- **Читать:** `fine/tuning.md` п.7–8; `engine/internal/birth/anchor.go`; `engine/cmd/tns65-bake/main.go` (дефолт `world_db/base/base.db`); `engine/ARCHITECTURE.md`.
- **Создаётся:** нет.
- **Правится:** `engine/internal/birth/anchor.go`, `engine/internal/db/worldroot.go` (комментарии), `engine/cmd/tns65-bake/main.go`, `engine/ARCHITECTURE.md`.
- **Писатель:** нет записи.
- **Шаги:**
  1. Обновить формулировки ошибок/комментариев: «base.db» → «база языка base.<lang>.db» (`anchor.go`: пустая база, открытие базы; `worldroot.go`: комментарий про замороженную базу).
  2. `tns65-bake` при старте печатает предупреждение «заменён devForge: базы поставляются готовыми base.<lang>.db в engine/base/» и работает дальше (совместимость); код не удаляется в этой волне; дефолтный путь вывода больше не `world_db/base/` (поставка туда не пишется), флаг `-db` обязателен или дефолт `state/legacy-base.db` — выбор фиксируется в задаче по правилу «данные станка вне поставки».
  3. `ARCHITECTURE.md`: пометка в реестре миграций, что поставка баз — devForge в `engine/base/` (слитая base.db — легаси, рантайм не читает).
- **Проверка:** `cd engine && grep -rn 'base\.db' internal/birth/anchor.go internal/db/worldroot.go | grep -v 'base\.<lang>\|base\.<L>'` → вывод пуст; `go build ./cmd/tns65-bake && ./tns65-bake 2>&1 | head -1` → строка про легаси; гейты → exit 0.
- **Исполнитель:** код.

### T-10-15. Приёмочная проводка: движок заводится на базах поставки
- **Атомы:** D-01 (данные: base.db — папка движка), D-03 (заморожена, только чтение), TUN-1 (доставка баз, блокирующее), TUN-9 (контрольная сумма в реальном прогоне).
- **Выдержки:** «base.db — база движка: лежит в папке движка, поставляется разработчиком»; «после сборки base.db заморожена: записи, правки и пересчёт векторов в контурах игры запрещены»; «Движок ждёт файлы в `world_db/base/base.<lang>.db` … Ни загрузчика, ни процедуры копирования (+ `.sha256`) нет. Без этого новые базы не заводятся» (TUN-1 — закрывается T-10-02 и этой проводкой).
- **Зависит от:** T-10-02, T-10-03, T-10-04.
- **Читать:** `fine/tuning.md` п.1; `engine/internal/birth/birth.go`; `engine/internal/testkit` (фиктивный эмбеддер).
- **Создаётся:** `engine/internal/birth/basesmoke_test.go`.
- **Правится:** нет.
- **Писатель:** `core` (рождает миры во временных каталогах теста; базы поставки не пишет).
- **Шаги:**
  1. Охрана теста: если в `db.BaseRoot` нет `base.<L>.db` хотя бы для одного языка из `en ru de es fr ja zh` — `t.Skip` («поставка баз языков не установлена»); тест базы не копирует и не создаёт (`*.db` вне git).
  2. Для каждого из 7 языков: рождение мира с `WorldsRoot` в `t.TempDir()`, фиктивный детерминированный эмбеддер 1024 из testkit, анкета с `lang=<L>`; путь базы не задаётся явно — берётся путь по умолчанию из T-10-02.
  3. Проверить на каждый язык: якорь назначен (`anchor_narrative_id` непуст), каталог мира создан, внутри `WorldsRoot` каталога `base/` нет (поставка не попадает в рабочие данные).
  4. Проверить неизменность поставки: SHA-256 каждой `base.<L>.db` до и после прогона совпадают (D-03 — движок читает, не пишет).
  5. Живой прогон с эмбеддером llama-server `:5002` (контракт T-10-05) в тест не входит: проводка на стенде фиксируется отдельной строкой в `99-LEDGER.md` и не блокирует волну.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/birth TestBirthOnShippedBases` → `PASS`: 7 языков, в каждом якорь назначен, суммы баз неизменны, `base/` в `WorldsRoot` нет; без установленной поставки → `SKIP` с причиной. Гейты → exit 0.
- **Исполнитель:** код; п.5 — владелец.

---

## Чек-лист приёмки волны (SCR-§8 + TUN)

- [ ] 1. Живой ход выбирает композицию, а не одиночный архетип (деградация — только при пустом пуле) — T-10-11.
- [ ] 2. Secondary pattern всегда из другой семьи — T-10-09/T-10-12 (валидация писателя и читателя).
- [ ] 3. Structural Drift реально влияет на выбор (счётчики, N=3, форс редких/инверсий каждые 5) — T-10-10/T-10-11.
- [ ] 4. Старые артефакты работают без перекомпиляции Библии (дефолты колонок, family-only режим) — T-10-08/T-10-11.
- [ ] 5. Новые артефакты мастерской содержат technique и moral_stance — T-10-12.
- [ ] 6. Повторяемость семей в главе заметно снижается — приёмочный прогон T-10-11 (тест (б)).
- [ ] 7. Живой ход не замедляется (бюджет 100 мс, чтение одним SQL) — T-10-11 (тест (д)).
- [ ] 8. Набор Family зафиксирован на 14 — T-10-01/T-10-13 (валидация загрузчика + отпечаток реестра).
- [ ] 9. TUN-1: базы доставлены в `engine/base/` (папка движка, рядом с `bin/`), sha256 сверены (выполнено до старта волны).
- [ ] 10. D-01/D-03: база языка берётся из `engine/base/`, только чтение; в `world_db/` базы поставки не попадают — T-10-02.
- [ ] 11. TUN-2/TUN-9: неверный язык и битая база — понятный отказ до создания мира — T-10-03/T-10-04.
- [ ] 12. TUN-3: движок работает с llama-server `/v1/embeddings` напрямую — T-10-05.
- [ ] 13. TUN-4/TUN-5/TUN-6: якорь материализован, имена назначаются, `patterns.kind` — канон сопоставления — T-10-06/T-10-07.
- [ ] 14. TUN-7/TUN-8: сообщения про `base.<lang>.db`, пекарь помечен легаси — T-10-14.
- [ ] 15. Движок заводится на базах поставки: рождение мира на реальной `base.<lang>.db` всех 7 языков, суммы неизменны, в `world_db/` поставка не попадает — T-10-15. Входное условие старта приёмки P8: до этой проводки сценарии INT-1/INT-4 не гоняются.

## Риски и открытые вопросы (в `99-LEDGER.md` статусом `blocked-owner` не ставятся, пока не подтверждены владельцем)

1. **Рабочий каталог процесса.** `db.WorldsRoot = "world_db"` и новый `db.BaseRoot = "base"` относительны CWD; все штатные скрипты делают `cd engine/`. Запуск бинарника из другого каталога молча создаст новый `world_db/` и не найдёт базы. Кандидат в отдельную задачу (абсолютизация путей от флага/конфига/исполняемого файла) — вне P9, зафиксировать вопросом.
2. **Расхождение de/ja на 1 главу** (1120 против 1121) — движку безразлично, выравнивание — отдельный прогон devForge (TUN, «Примечание по балансу языков»).
3. **Gloss `rise_fall_rise` и границы близких пар** — правка словаря devForge + перепек; решение за владельцем devForge (T-10-13, п.4).
4. **`usage_count` в bank_pool** пишется только контуром forge; живой ход свои счётчики ведёт в `family_usage` (один писатель на таблицу). Если владелец захочет инкремент из хода — это сознательное расширение карты писателей, отдельный вопрос.
5. **Technique/mood-маппинг** (mood→техники) в T-10-09 фиксирован детерминированной таблицей в коде; при желании вынести в языковой пакет — вопрос к владельцу (потенциальная локализация).
6. **Состав релизного пакета** (F-11, T-08-14): бинарники + `engine/base/` + `engine/packs/` едут вместе; сборку пакета под новую раскладку оформить задачей P7/P8 при активации — вне рамок P9.
