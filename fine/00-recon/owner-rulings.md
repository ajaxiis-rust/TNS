# Owner rulings on AMBIG items — chat 17.10.2026

Source: Telegram chat with owner, 17 Oct 2026, message-by-message on the AMBIG list
(fine/00-recon/mimo-mechanics-registry.md, section (c)).

| AMBIG | Question | Ruling (owner, 17.10) | Status |
|---|---|---|---|
| AMBIG-1 | Six estates list missing (пачка = 16 psychotypes × 6 estates = 96 cards) | «Сословия брать из TNS35, нужен поиск — мимо+MCP» + 17.10 ruling: list given by owner | RESOLVED: 6 estates = военный/служащий, купец, аристократ, духовенство, ремесленник, крестьянин (owner list; TNS35 has no 6-estate enum, ROLE_BIAS 6 roles noted as heritage) |
| AMBIG-2 | 16 psychotypes vs 14-metric profiler | «ошибка, тоже поиск» | SEARCH DONE (Q2): no conflict — 4 axes / 14 stored metrics / 16 = implied type-code space (never enumerated) / 12 = separate narrative archetypes enum |
| AMBIG-3 | p.21 narrative granularity (per-verse vs per-plot) | «1А» | RESOLVED: narrative = whole plot, ONE embedding point per story, vector brewed at engine build from the arc summary (summary of the arc, not mean of verse vectors) |
| AMBIG-9 | «Догон с рассказом» vs п.13 pace 1:1 | «2А» + rationale | RESOLVED: grain #56 CANCELLED (contradicts п.13). Owner rationale recorded: the mechanic guarded against player cheating (freezing the world by silence to dodge timed consequences). Anti-abuse guarantee to bake into spec: under 1:1 the world KEEPS LIVING during absence at the same rate — NPC обсчёты run, deferred events fire, return = summary «пока тебя не было» + delayed news; pauses must NEVER be implemented as freeze. |
| AMBIG-4 | embeddingDimension default: UI 384 vs BGE-M3 1024 | «Ошибка — точно 1024» | RESOLVED: 1024. UI default 384 = stale. |
| AMBIG-5 | Анкета field count drift (4 core / 13 UI / 14 with режим походов) | «Подробней» → «Да 14 со слагом» | RESOLVED: canonical count = 14 (13 semantic fields + world slug as technical field); режим походов included in the 13 |
| AMBIG-6 | Режим походов: explicit field (canon) vs genre-derived (DOCS) | — | PENDING (canon wins by priority; DOCS fix queued) |
| AMBIG-7 | Ткач silence threshold ~45 s vs generation 35–55 s | «1 минута» | RESOLVED: threshold = 60 s |
| AMBIG-8 | Big Five vs Jung for NPC | «Не понял» | SEARCH DONE (Q3): mislabel confirmed — no Big Five/OCEAN in TNS35; hunger/anger are NOT TNS35 fields (only literary corpus text); NPC = state (health/mood/skills) + economy (6 stats, 8 vices, 4 humors) + l3 Jung psychotype, coexist by design; DOCS/MECHANICS.md #53 example = invented, flag stale |
| AMBIG-9 | «Догон с рассказом» vs п.13 pace 1:1 | — | PENDING |
| AMBIG-10 | T3 actual progress | — | PENDING (procedural, not spec-blocking) |

## Дополнительные решения 17.10 (вне списка AMBIG)

**UI: четыре вкладки, админ-зоны нет.** Владелец: «Игроку должно быть всё доступно, просто кнопок (вкладок) сверху 4: Атлас (мир/ы, графы и карты), История (Моя книга), MCP, Настройки. Это если из чата — главного меню — смотреть; по входу в одну из вкладок появляется вкладка чата (нормальное динамическое меню горизонтальное). А в настройках уже всё, что там как-то в админку попало». Следствие: разделение игрок/админ и скрытая админ-зона (канон п.14, R-017) ОТМЕНЕНЫ как архитектура доступа; единственный пользователь — игрок, весь интерфейс доступен; всё «админское» (конфигурация мира, роли, провайдеры, модели, дашборд, консоль наполнения) живёт во вкладке «Настройки», наполнение — вкладка «MCP». Канон п.14 в этой части устарел и подлежит правке владельцем (backlog). Состав окон и полей из банка решений `interface/` остаётся источником inventory (R-164) — меняется навигация и доступ, не сами формы.

Подтверждения 17.10 (ответы владельца на вопросы): 1) вкладка MCP = консоль наполнения — «верно»; 2) создание мира и его использование/переключение/переигровка в Атласе — «верно»; 3) «конфиг мира — ошибка, остальное верно»: страница world-config умирает, все настройки мира делаются сразу при рождении — иначе правка одного параметра ломает всю логику ковки внутри мира; остальное размещение в Настройках (роли, провайдеры, модели, мониторинг/дашборд) — верно. Итоговое правило неизменности мира: мировые параметры не редактируются после рождения вообще; R-149/R-150 помечены CANCELLED в реестре.

**Замечания 17.10 (финальные правки 07-ui):** 1) боковых меню нет — дизайн адаптивный, включая телефон; панель мир/герой/система боковым меню не переезжает; 2) аудита мира не существует — упоминания убраны; 3) все вопросы состава MCP решаются при дизайне самого MCP, не в спецификации. План владельца: далее переделывается дизайн UI, после него спецификация переписывается с нуля полностью.

**Библия и запекание сида.** Владелец: «Обработка библии для запекания в дефолтную начальную базу — механика вне игры. Она нужна на начальном этапе. Библейский нарратив делается, создаётся вне игры — это просто отдельный инструмент». Следствие, обязательное для спецификации: запекание библии в дефолтную начальную базу — внеигровой отдельный инструмент предстартовой подготовки; выполняется один раз на начальном этапе; библейский нарратив создаётся вне игры; движок, рождение мира, мастерская и живой контур никогда не запекают сырьё — игра получает уже готовую запечённую базу (base.db с нарративами + сид-банки языкового пакета). Термины: «запекание сида» = этот внеигровой инструмент; «запекание» в словаре мастерской = внутриигровой синоним ковки (канон п.18) — разные вещи, не смешивать.

Bake into spec sections: AMBIG-4 → data layer (embedding config); AMBIG-7 → live turn (Ткач insurance);
AMBIG-1/2/8 → world birth (пачка) after search verdicts; библия/запекание сида → data layer (§1.1, §3) и machine-birth (§6).
