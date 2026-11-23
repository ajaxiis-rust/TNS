# P14. Омонимы: смыслы слов, триггеры, спорные случаи

> Восстановлена 23.11.2026 (REG-P14, решение владельца «восстановить
> обязательно») после отката 21.11: план снят с калибрационной станции
> `tns65-calib-2026-11-17/calib-src`, T-15-01 перебазирован с закладки
> `dry/16-distillation-full` (6d45356 → 17dfd27, ветка `dry/14-senses`).

Волна закрывает различение внутричастеречных омонимов разбора: «зáмок — замóк»,
«коса — коса — коса». Лемматизатор различает части речи и даёт несколько
разборов, но два существительных одной леммы для него одно слово; смысл
различает контекст. Решение владельца 17.11.2026: триггерные словари весов,
затем смысл в salience, затем центроиды смыслов на эмбеддингах, спорные случаи —
слоту. Порядок задач ведёт «Зависит от».

Норма данных — пак `engine/packs/ru`; код — пакет `internal/senses` и связка с
`internal/deepread`. Сборка пинит sha256 файлов пака в `manifest.json`.

---

### T-15-01. senses.json и триггерный матчер
- **Атомы:** нет (вне спеки, решение владельца 17.11).
- **Зависит от:** нет.
- **Читать:** `engine/internal/deepread/lemmas.go` (`Lemma`: Form/Lemma/POS/Tags/Line/Index), `engine/internal/deepread/dicts.go` (чтение словарей пака), `engine/packs/ru/manifest.json`.
- **Создаётся:** `engine/internal/senses/senses.go`, `engine/internal/senses/senses_test.go`, `engine/packs/ru/senses.json`.
- **Правится:** `engine/packs/ru/manifest.json` (пин sha256 нового файла), `engine/internal/langpack/langpack_test.go` (счёт файлов манифеста 13→14 под пин `senses.json` — как у IMP-9).
- **Писатель:** нет (пакет в памяти; в базу ничего не пишется).
- **Шаги:**
  1. Схема `senses.json`: `{"version":1,"senses":[{"lemma":"замок","pos":"NOUN","sense":"строение","category":"architecture","triggers":[{"lemma":"башня","weight":1.0}]}]}`. `category` — ключ словаря разбора (`architecture`, `appearance`, `nature`, `social`, `action_dramatic`, `erotic`, `violence`, `battle`) либо пустая строка, если смысл в категории не попадает.
  2. Данные: 12 лемм с внутричастеречными омонимами — замок, ключ, коса, кисть, ножка, пол, батарея, ласка, свет, звезда, гребень, кран. По 2–3 смысла на лемму, у каждого смысла 5–10 триггеров-лемм с весом 0.5–1.5. Триггеры — бытовые слова контекста («башня», «ворота», «ключ», «дверь»); слова, которые сами стоят в словаре категории этого смысла, триггерами не берутся.
  3. Пакет `internal/senses`: `Load(path)` читает и валидирует файл; типы `Token{Form,Lemma,POS,Line,Index}`, `Trigger{Lemma,Weight}`, `Sense{Lemma,POS,Sense,Category,Triggers}`, `Decision{Index,Sense,Category,Score,Margin,Evidence,Decided}`; `(*Dictionary).Resolve(tokens []Token) []Decision` рассматривает только токены, у которых (лемма, часть речи) имеет ≥2 смысла.
  4. Окно контекста — ±12 токенов от омонима в пределах того же абзаца (`Line`); триггер учитывается один раз независимо от числа вхождений; сам токен-омоним триггером не считается. Смысл решён, если лучший вес ≥ `MinScore` (1.0) и отрыв от второго места ≥ `MinMargin` (0.5); иначе `Decided=false`, `Sense` пустой.
  5. Тесты: `TestResolveSenses` — по одному предложению на каждый смысл всех 12 лемм, смысл решён верно; `TestResolveAmbiguous` — предложение без триггеров даёт `Decided=false`; `TestLoadSenses` — ошибки разбора, дубликат ключа (lemma,pos,sense), смысл с менее чем тремя триггерами; `TestSensesFile` — реальный `packs/ru/senses.json`: 12 лемм, у каждой ≥2 смысла, у каждого смысла ≥3 триггера, веса в 0.5–1.5, категории из списка.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/senses TestResolveSenses TestResolveAmbiguous TestLoadSenses TestSensesFile` → PASS по всем четырём; `cd engine && bash scripts/build.sh 2>&1 | tail -5` → строка `packs/ru OK`.
- **Документы:** нет.
- **Исполнитель:** код.

> ✅ закрыт 2026-11-17 — коммит 6d45356; перебазирован 2026-11-23 — коммит 17dfd27 (senses.json: 12 омонимов, триггерный матчер; ревью 2/3; конфликт количества файлов пака разрешён: 15 с учётом sarcasm.json P17).

---

### T-15-02. Смысл в salience: золотые чанки омонимов и порог
- **Атомы:** W-22, W-23 (скор категорий и полосы).
- **Зависит от:** T-15-01.
- **Читать:** `engine/internal/deepread/salience.go` (`TagChunks`, словарный счёт), `engine/internal/deepread/golden_test.go`.
- **Создаётся:** `engine/internal/deepread/senses.go`, `engine/internal/deepread/senses_test.go`, `fine/golden/ru-homonyms/golden.json`, `fine/golden/ru-homonyms/README.md`.
- **Правится:** `engine/internal/deepread/salience.go` (счёт омонимов через смысл), `engine/config.yaml` (`deepread.senses_margin`).
- **Писатель:** `forge` — `book_chunk_tag` как сейчас.
- **Шаги:**
  1. При загрузке прогона `senses.json` читается рядом со словарями категорий; нет файла — разбор работает как раньше.
  2. Для токена, чья лемма стоит в словаре категории и чей (лемма, часть речи) есть в senses: считается только смысл с `Category`, равной ключу словаря; смысл решён и в другую категорию — вклад 0; смысл не решён — вклад как сейчас (консервативно).
  3. Золотой отрывок `fine/golden/ru-homonyms/`: 12–20 чанков, по одному-два на каждую лемму волны, с ручной разметкой ожидаемых категорий; гейт `TestGoldenPrecision` обходит `fine/golden/ru*` и берёт отрывок по `approved: true`.
  4. Порог `deepread.senses_margin` — отрыв лучшего смысла от второго; при `0` действует константа T-15-01.
  5. Тесты: омоним-ловушка не даёт ложного тега категории; золотой отрывок проходит гейт; точность прежних трёх отрывков не падает.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/deepread TestGoldenPrecision TestSenseSalience` → PASS; `go test ./internal/deepread -run TestGoldenPrecision -count=1` по всем отрывкам ≥ `golden_min_precision`.
- **Документы:** нет.
- **Исполнитель:** код.

---

### T-15-03. Центроиды смыслов: слой эмбеддингов
- **Атомы:** W-25 (контекстные векторы), MD-03.
- **Зависит от:** T-15-02.
- **Читать:** `engine/internal/deepread/characters.go` (`AttachWindowVectors`, окно T-14-14), `engine/internal/models/embed.go`.
- **Создаётся:** `engine/internal/senses/vectors.go`, `engine/internal/senses/vectors_test.go`, `engine/packs/ru/senses_contexts.json`.
- **Правится:** `engine/internal/senses/senses.go` (слияние триггерного и векторного счёта), `engine/config.yaml` (`deepread.senses_embed_weight`), `engine/packs/ru/manifest.json`.
- **Писатель:** нет (векторы смыслов — в памяти прогона; кэш не заводится).
- **Шаги:**
  1. `senses_contexts.json`: по 5–10 коротких контекстов (одно-два предложения) на каждый смысл из `senses.json`; контексты без имён собственных, с триггерами смысла.
  2. Центроид смысла — среднее нормализованных векторов его контекстов; считается лениво на прогон и переиспользуется для всех чанков книги.
  3. Итоговый счёт смысла = `trigger_score + senses_embed_weight × cosine(окно, центроид)`; при недоступном эмбеддере остаётся только триггерный счёт, прогон не падает.
  4. Окно — контекст T-15-01 (±12 токенов, тот же абзац), текст окна собирается из `RunChunk.Text` по номерам строк и позициям токенов.
  5. Тесты: фейковый эмбеддер `testkit.NewFakeEmbed` — смысл выбирается по близости контекста при пустых триггерах; недоступный эмбеддер не меняет триггерное решение; вес `0` возвращает поведение T-15-02.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/senses TestSenseVectors TestSenseVectorsFallback` → PASS.
- **Документы:** нет.
- **Исполнитель:** код.

---

### T-15-04. Спорные смыслы — слоту
- **Атомы:** W-24 (слот спорных), T-14-10 (очередь спорных).
- **Зависит от:** T-15-02.
- **Читать:** `engine/internal/deepread/dispute.go`, `engine/internal/deepread/run.go` (`DisputeSlot`).
- **Создаётся:** `engine/internal/deepread/senses_dispute.go`, `engine/internal/deepread/senses_dispute_test.go`.
- **Правится:** `engine/internal/deepread/run.go` (очередь спорных смыслов), `fine/04-spec/05-workshop.md` (атом W-24: смысловой вопрос).
- **Писатель:** `forge` — решения в `forge_log`, как у спорных мест.
- **Шаги:**
  1. Спорный смысл — омоним, у которого `Decided=false`, но есть ≥1 сработавший триггер, либо отрыв в пределах `senses_margin × 2`.
  2. Вопрос слоту несёт лемму, часть речи, список смыслов с триггерами, цитату-подстроку чанка и номер токена; пакет вопросов — в общем бюджете `deepread.slot_budget`.
  3. Ответ применяется к тегам текущего прогона и пишется в `forge_log`; неверная цитата — вопрос отклоняется, тег остаётся консервативным.
  4. Повторный прогон с тем же ключом пересчёта вопросов не повторяет.
  5. Тесты: спорный омоним уходит в очередь с цитатой; ответ меняет тег; отклонённая цитата тег не меняет; бюджет и повторный прогон.
- **Проверка:** `cd engine && bash scripts/runtests.sh ./internal/deepread TestSenseDispute TestSenseDisputeBudget` → PASS.
- **Документы:** `engine/ARCHITECTURE.md`.
- **Исполнитель:** код.
