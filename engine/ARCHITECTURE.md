# ARCHITECTURE

## 1. Три слоя станка

- **Ядро-надзиратель** — единственный процесс-хозяин: держит конвейер живого хода, очередь мастерской, таймеры отложенных событий и приоритет хода игрока; порождает, лечит и останавливает остальные процессы станка. Код: cmd/tns65, internal/core, internal/supervisor.
- **Резидентные сервисы горячего пути** — постоянные дети ядра: генерация llama.cpp :5001, эмбеддинги BGE-M3 :5002, лемматизатор tns65-lemmaru; запуск, лечение и остановка — обязанность ядра, готовность проверяется health-запросом без фиксированного короткого бюджета. Клиенты в коде ядра: internal/models, internal/lemmarpc.
- **Рой задач** — каждая ковка, доукомплектация, коллизия, оседание сцены — однозадачный запуск с началом и концом; завершился — результат в базу, процесс умер; между паузами в памяти ничего не висит. Код: internal/swarm, internal/workshop.

## 2. Направление импортов

Направление: `cmd/* → core/* → доменные пакеты → инфраструктура (db, journal, netgate, portgate)`.
Обратные импорты и циклы запрещены. Контроль — тест TestNoImportCycles и ревью.

## 3. Эталон каталогов internal/

| Каталог | Назначение |
| --- | --- |
| version | пакет версии ядра и роя — Version и BuildDate, переопределяемые через -ldflags (F-12.5) |
| journal | журнал станка — события ядра и вывод резидентов, одна JSON-строка — одно событие (T-01-03) |
| db | хранилище трёх баз SQLite — открытие, мигратор и карта писателей (T-01-04…T-01-07) |
| netgate | единый сетевой гейт — весь наружный трафик только через белый список хостов (T-01-11) |
| portgate | порт-гейт при старте — слушающие сокеты ядра сверяются с белым списком портов (T-01-12) |
| supervisor | ядро-надзиратель — запуск, лечение и остановка детей-резидентов (T-01-13, T-01-14) |
| swarm | рой однозадачных задач — контракт задачи и версионная сверка с ядром (T-01-15) |
| langpack | языковой пакет — манифест, версии, откат и сборка (T-01-08) |
| lemmarpc | протокол лемматизатора stdio/JSON и его клиент (T-01-09) |
| models | слоты моделей станка — эмбеддинги :5002, генерация :5001 и внешний провайдер |
| embedserver | локальный эмбеддер :5002 — флаги llama-server, health и проверка пулинга (T-12-17) |
| censor | единый словарный фильтр пакета — леммы против словаря запрета (T-04-11) |
| core | ядро станка — конвейер живого хода, планировщик приоритета хода, очередь мастерской и таймеры (F-12.1) |
| birth | рождение мира — анкета, пачка карточек и якорь сценарной рамки (P1) |
| structural | реестр 14 семей структурной композиции и загрузчик archetypes.json (T-10-01) |
| workshop | фоновая мастерская — очередь, ступени паузы, ночное окно и ковка (P4) |
| paganel | геомодель мира — места, пути, кольца и разведка карты (P5) |
| book | книга игрока — главы, публикация, дайджест и экспорт (P6) |
| cast | карта книги — имена, описания, характеры и роли действующих лиц (P12) |
| library | глобальная полка книг — импорт txt/fb2, манифест и статусы (P11, T-12-09) |
| webapi | JSON/WS-интерфейс на loopback для веба игрока (P7) |

Новый каталог добавляется только задачей с правкой этого списка. Будущие каталоги: bake (T-01-16), testkit (T-02-15), turn и selector (P2), sediment (P3), geo (T-05-13), bank (P2).

## 4. Реестр миграций db/schema/

| Номер | База | Задача-владелец |
| --- | --- | --- |
| 0001_init | base | T-01-04 |
| 0003_characters_lang | base | E-05 |
| 0002_vectors | shared | T-01-04 |
| 0001_init | literary | T-01-05 |
| 0002_sediment_idempotency | literary | T-04-10 |
| 0003_composition | literary | T-10-08 |
| 0004_book | literary | T-12-10 |
| 0005_cast | literary | T-12-21 |
| 0001_init | world | T-01-06 |
| 0003_workshop_queue | world | T-02-11 |
| 0004_saw | world | T-03-06 |
| 0005_scene_signals | world | T-04-06 |
| 0006_paganel | world | T-06-05 |
| 0007_book_wakeup | world | T-07-02 |
| 0008_book_deepread | world | T-07-03 |
| 0009_book_claim | world | T-07-09 |
| 0010_book_epilogue | world | T-07-13 |
| 0011_world_lexicon | world | T-07-05 |
| 0012_structural_drift | world | T-10-10 |

Каждая новая миграция вписывается задачей, которая её создаёт.

Поставка баз — devForge в `engine/base/`: базы `base.<lang>.db` приходят
готовыми, движок их не печёт. Слитая `base.db` легаси-пекаря `cmd/tns65-bake`
рантаймом не читается (TUN-8, T-10-14).

## 5. Контракты общих пакетов

Следующие задачи реализуют эти сигнатуры, не изобретая.

```go
// journal — реализуется T-01-03
type Field struct{ Key, Value string }
type Logger struct{ /* один писатель: мьютекс, O_APPEND, ротация 50 МБ, хранить 5 файлов */ }
func New(path string) (*Logger, error)
func (l *Logger) Event(source, event string, fields ...Field) error
func Pipe(name string, w io.Writer) (io.Writer, error) // w == nil → ErrNoWriter
var ErrNoWriter = errors.New("journal: no writer")

// db — реализуется T-01-04 и T-01-07
type Writer string // "core" | "forge" | "sediment" | "bake"
func Open(path string, w Writer) (*DB, error)  // писатель: WAL, busy_timeout=5000, SetMaxOpenConns(1)
func OpenRead(path string) (*DB, error)        // чтение: _pragma=query_only(1); запись — ошибка SQLite
func (d *DB) Tx(w Writer, fn func(tx *Tx) error) error // w обязан совпадать с тегом хэндла
func (tx *Tx) Exec(table, query string, args ...any) (sql.Result, error) // таблица сверяется с картой писателей
var ErrForeignWriter = errors.New("db: foreign writer")

// netgate — реализуется T-01-11
func Client(host string) *http.Client // единственный путь наружного HTTP; сверка хоста с белым списком до соединения
func Local(addr string) *http.Client  // явный хук loopback (:5001, :5002, httptest)
var ErrDeniedHost = errors.New("netgate: denied host")
```

## 6. Раскладка каталогов engine/

| Каталог | Назначение |
| --- | --- |
| bin/ | бинарники станка |
| base/ | поставка баз языков base.<lang>.db и реестра archetypes.json — рядом с бинарниками, заморожена, только чтение (D-01, D-03, T-10-02) |
| world_db/worlds/<slug>/ | рабочие данные мира: оперативная и литературная базы |
| world_db/library/ | глобальная полка книг: сырьё txt/fb2 и манифест library.json (P11, T-12-09) |
| world_db/models/ | модели менеджера моделей (MD-09.5) |
| seed/ | сид-нарративы и газетир |
| packs/ | языковые пакеты |
| web/ | статика интерфейса игрока |

Настройки станка — раскладка по `config.yaml`, `settings.toml`, `world_meta` и настройка Telegram/туннеля — описаны в `DOCS/10-STATION-SETTINGS.md` (от корня репозитория).

