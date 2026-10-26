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
| core | ядро станка — конвейер живого хода, планировщик приоритета хода, очередь мастерской и таймеры (F-12.1) |
| birth | рождение мира — анкета, пачка карточек и якорь сценарной рамки (P1) |
| workshop | фоновая мастерская — очередь, ступени паузы, ночное окно и ковка (P4) |
| paganel | геомодель мира — места, пути, кольца и разведка карты (P5) |
| book | книга игрока — главы, публикация, дайджест и экспорт (P6) |
| webapi | JSON/WS-интерфейс на loopback для веба игрока (P7) |

Новый каталог добавляется только задачей с правкой этого списка. Будущие каталоги: bake (T-01-16), testkit (T-02-15), turn и selector (P2), sediment (P3), geo (T-05-13), bank (P2).

## 4. Реестр миграций db/schema/

| Номер | База | Задача-владелец |
| --- | --- | --- |
| 0001_init | base | T-01-04 |
| 0002_vectors | shared | T-01-04 |
| 0001_init | literary | T-01-05 |
| 0001_init | world | T-01-06 |
| 0003_workshop_queue | world | T-02-11 |
| 0004_saw | world | T-03-06 |
| 0005_scene_signals | world | T-04-06 |

Каждая новая миграция вписывается задачей, которая её создаёт.

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
