# TNS35 search: estates, Jung profiler metrics, NPC trait fields

RECON report. Sources: codebase-memory MCP (project `media-ajaxiis-ext3-TNS35`) + files on disk
under `/media/ajaxiis/ext3/TNS35`. All paths below are relative to that root. Read-only; nothing
invented; each claim carries file:line.

---

## Q1. ESTATES (сословия)

**VERDICT: NOT FOUND — TNS35 contains NO list/enum of exactly 6 social estates.** The word
«сословия» appears only in prose (design docs), never as an enumerated code/data structure.
What TNS35 actually has is (a) a 10-level feudal rank ladder, (b) per-social-system hierarchy
arrays of 0–5 levels, (c) two exactly-6-element sets that are the only plausible sources for a
"6 estates" batch axis: `ROLE_BIAS` roles and `FactionType`. Candidates listed below in order
of fitness for the пачка axis.

### Candidate A — ROLE_BIAS: exactly 6 social roles (BEST FIT for "6 estates × 16 psychotypes")
This is the only 6-element set of *social roles* in the code, and it is wired directly into the
Jung profiler (role → psychotype bias), matching the пачка structure 16 × 6:
- `app/src/services/jungian-profiler.ts:290-297` — `ROLE_BIAS` table, 6 roles, each with Jung
  axis priors:
  1. craftsman  (intuition 0.3, thinking 0.75, judging 0.7)  — line 291
  2. guard      (intuition 0.3, thinking 0.6,  judging 0.75) — line 292
  3. merchant   (intuition 0.35, thinking 0.7, judging 0.6)  — line 293
  4. scholar    (intuition 0.8, thinking 0.8,  judging 0.55) — line 294
  5. wanderer   (intuition 0.8, thinking 0.3,  judging 0.3)  — line 295
  6. healer     (intuition 0.55, thinking 0.35, judging 0.6) — line 296
- Consumed by `assignNpcPsychotype(role, faction?, worldSystem?, seed)` —
  `app/src/services/jungian-profiler.ts:309-340` (unknown role falls back to neutral 0.5/0.5/0.5,
  line 315; faction/world-system nudges, lines 322-328).

### Candidate B — FactionType: exactly 6 values (faction kinds, includes "neutral")
- `app/src/services/social-graph.ts:22` — `FactionType = "military" | "economic" | "religious" |
  "criminal" | "noble" | "neutral"` (6 values). Used by `FactionDetails` (social-graph.ts:26-30).
  Caveat: these are faction *types*, and "neutral" is not an estate.

### The actual rank ladder (10 levels, not 6) — feudal promotion ladder
- `app/src/models/rank.ts:8-19` — `enum RankType`: SLAVE, COMMONER, BARONET, BARON, VISCOUNT,
  COUNT, MARQUIS, DUKE, KING, EMPEROR (10 values).
- `app/src/rules/economy/ranks.json:18` — promotion order:
  `["slave", "commoner", "baronet", "baron", "viscount", "count", "marquis", "duke", "king", "emperor"]`;
  formula at `ranks.json:21-22` ("promotion: wealth >= next rank's wealthMin").
- Per-rank economy config (wealthMin, guards, baseTaxRate, foodConsumption, foodCost, salary,
  familyExpensesPct, canGiveBribes, canTakeBribes): `app/src/rules/economy/ranks.json:7-16`,
  typed at `app/src/models/rank.ts:21-32`; promotion logic `app/src/models/rank.ts:47-58`.

### Per-social-system hierarchies (rules/social/*.json, "hierarchy" at line 6 of each file)
10 world social systems, each with its own ladder — none has 6 levels:
- feudalism   `app/src/rules/social/feudalism.json:6`   — [king, duke, lord, knight, peasant] (5)
- mercantilism `app/src/rules/social/mercantilism.json:6` — [merchant_prince, guildmaster, merchant, artisan, laborer] (5)
- theocracy   `app/src/rules/social/theocracy.json:6`   — [high_priest, priest, acolyte, faithful, heathen] (5)
- tribalism   `app/src/rules/social/tribalism.json:6`   — [elder, warrior, hunter, gatherer, youth] (5)
- slavery     `app/src/rules/social/slavery.json:6`     — [master, overseer, freed, slave] (4)
- capitalism  `app/src/rules/social/capitalism.json:6`  — [bourgeoisie, middle_class, working_class, poor] (4)
- communism   `app/src/rules/social/communism.json:6`   — [party_secretary, cadre, citizen] (3)
- socialism   `app/src/rules/social/socialism.json:6`   — [party_official, citizen] (2)
- democracy   `app/src/rules/social/democracy.json:6`   — [citizen] (1)
- anarchy     `app/src/rules/social/anarchy.json:6`     — [] (0)
- plus economy variant: `app/src/rules/economy/silver-economy.json:6` — [silver_lord, merchant,
  artisan, laborer] (4).

### Other 5-element groupings (dialogue factions, not estates)
- `app/src/services/dialogue-context.ts:5` and `app/src/services/dialogue-manager.ts:444` —
  FACTIONS = [guards, thieves, merchants, nobles, peasants] (5).

### Prose mentions of «сословия» (no enumeration)
- `devs/PARADIGM.md:12` — world lives by its laws «(экономика, сословия, семьи, налоги, вера)».
- `devs/PARADIGM.md:45` — «Веер реакций — сословия отвечают по своим интересам (Церковь — ересь,
  купец — цена, цех — угроза)» — 3 example estates named (Церковь, купец, цех), no list.
- `devs/SPEC.md:159`, `devs/SPEC.md:301` — «циклы, дилеммы сословий» (economy tables migration).
- `app/docs/01-concepts.md:9,20,98` — «сословия» in paradigm prose (world laws; «дилеммы
  сословий» in the economy service row).

### Queries tried for a 6-estate enum (negative verification)
- codebase-memory search_code regex `сослови|сословие|estate|caste` (72 raw matches — all prose).
- fff grep `estate`; fff multi_grep `rank|ladder|caste|сослов|ранг`.
- ripgrep over TNS35: `сослов|estate|Estate|страт` and `ESTATES|SOCIAL_CLASS|SocialClass|Estates\b|сослови[еяй]`
  — no enum, only the prose above.
- `grep '"hierarchy"'` over `app/src/rules/**` (full list above).
- Conclusion: a fixed list of exactly 6 estates does not exist in TNS35; if the пачка axis needs
  6, the owner must either take Candidate A (ROLE_BIAS roles, mechanically paired with the Jung
  profiler) or coin the list. The feudal *ladder* of TNS35 is 10 ranks (Candidate "rank ladder").

---

## Q2. JUNG PROFILER METRICS

**VERDICT: "14 metrics" and "16 types" are both real but describe DIFFERENT things — no
contradiction.** The profiler core = 4 Jung axes (E/I, N/S, T/F, J/P). "14 metrics" = the 14
`jungian_*` values stored per player in `player_profile.metrics` (8 axis values = 4 axes ×
preference/range, + 1 confidence, + 4 axis confidences, + 1 source). "16 types" = the MBTI-style
type space *implied* by `deriveType`'s 4 binary axes (2^4 = 16 when every axis is decisive) — but
**no explicit 16-type enum exists anywhere in TNS35**; `deriveType` can also emit 'X' for
ambivalent axes. Separately there is a 12-element `JUNGIAN_ARCHETYPES` enum — that is a *narrative
archetype* floor (personas), not psychotypes; do not conflate.

### The 4 axes and the profile object
- `app/src/services/jungian-profiler.ts:10-18` — `JungianProfile`: extraversion, intuition,
  thinking, judging — each `AxisProfile {preference, range}` (lines 5-8) — plus `confidence`,
  `axisConfidence` (per-axis), `source: 'text'|'metrics'|'blended'|'default'`.
- `app/src/services/jungian-profiler.ts:33-39` — `deriveType`: 4-letter code, each letter
  E|I|X, N|S|X, T|F|X, J|P|X with thresholds 0.55/0.45. Decisive axes → the classical 16 MBTI
  codes; any axis in [0.45, 0.55] → 'X' (so the full output space is larger than 16).

### The 14 metrics (exact keys)
- `app/src/lib/player-profile-store.ts:190-210` — `upsertJungianProfile` writes exactly 14 keys:
  1-2 `jungian_extraversion_pref` / `jungian_extraversion_range` (lines 194-195)
  3-4 `jungian_intuition_pref` / `jungian_intuition_range` (196-197)
  5-6 `jungian_thinking_pref` / `jungian_thinking_range` (198-199)
  7-8 `jungian_judging_pref` / `jungian_judging_range` (200-201)
  9   `jungian_confidence` (202)
  10-13 `jungian_conf_extraversion`, `jungian_conf_intuition`, `jungian_conf_thinking`,
        `jungian_conf_judging` (203-206)
  14  `jungian_source` (207)
- Schema comment confirming the count: `app/src/lib/create-world.sql:145` — «Psychotype and stats
  of the player (14 metrics...)»; `app/src/lib/create-world.sql:150` — `metrics TEXT ... JSON:
  14 metrics of the player's style`.
- Docs confirming: `app/docs/03-data.md:52` — «player_profile (единственная строка: юнгианский
  психотип + 14 метрик стиля + статы...)».

### Behavioral metrics feeding the axes (the "profiler pipeline")
- `app/src/services/metrics-collector.ts:6-17` — `RawAggregates`: 10 counters (dialogueInitiated,
  dialogueCount, dialogueTotalWords, avoidedDialogues, explorationActions, riskTakingActions,
  planningActions, combatInitiated, inputTotalChars, expressiveActions).
- `app/src/services/metrics-collector.ts:19-30` — `DerivedMetrics`: 10 derived values.
- `app/src/services/metrics-collector.ts:32-37` — `AxisSignals`: 4 signals (the axes).
- `app/src/services/metrics-collector.ts:142-159` (`deriveMetrics`) and `:177-208`
  (`inferFromMetrics`) — behavioral metrics → 4 axis signals. Blending into the profile:
  `jungian-profiler.ts:93-114` (`blendBehavioralSignals`), config `jungian-profiler.ts:52-58`.

### Psychotype enums actually defined
- NO 16-type enum. Only 4 named type hints in `app/src/services/agents/actor.ts:193-198`
  (ISTP, ESFJ, ENFP, INTJ) with a generic fallback string for every other code
  (`actor.ts:199`, `${name} (${t}): consistent with a ${t} temperament`). Tests exercise ESTP
  (`app/tests/jungian-profiler.smoke.test.ts:49`) and ISTP (`app/tests/actor.test.ts:15`).
- Distinct "Jung" enum (different axis — narrative archetypes, «этаж Юнга»):
  `app/src/domain/archetypes.ts:22-35` — `JUNGIAN_ARCHETYPES`, 12 values: Self, Persona, Shadow,
  Anima, Animus, Wise Old Man, Great Mother, Trickster, Hero, Child, Maiden, Wise Old Woman;
  `app/src/domain/archetypes.ts:6-20` — `SCENE_ARCHETYPES`, 13 values. Header `archetypes.ts:2`:
  «Two-floor archetype enums ... SPEC §3». Used for personas' `archetype` («этаж Юнга»,
  `app/docs/03-data.md:32`) and the SPEC section-3 archetype FK check
  (`app/src/forge/brew.ts:3`, consumption `brew.ts:811`, `:943`, `:994`).
- Schema comment `app/src/lib/create-world.sql:149` — `archetype TEXT -- Jungian psychotype
  (section 3 enum)`. Note the column is also written with a closest-author name
  (`app/src/lib/player-profile-store.ts:235-239`), so "archetype" there is loosely used.

### Bottom line for TNS65
- «16 психотипов Юнга» = the derived type-code space of `deriveType` (4 binary axes; 16 decisive
  combinations) — plausible, but TNS35 never enumerates 16.
- «юнгианский профилировщик TNS35 (14 метрик)» = correct, but 14 = stored profile metric count,
  NOT the number of axes. The axis count is 4.
- If a doc needs one number: profiler = 4 axes; persisted style metrics = 14; type codes = 16
  (plus X-variants); narrative Jung archetypes = 12 (a separate enum).

---

## Q3. NPC TRAIT FIELDS

**VERDICT: hunger/anger are NOT NPC fields in TNS35 — the "Big Five-style" label in the TNS65
recon registry is a mislabel (no Big Five/OCEAN model exists in TNS35 at all).** Real NPC data is
three coexisting groups by design: (1) runtime state (`NPCProfile`: health, mood, memories,
skills), (2) economy (`NPCEconomyState`: stats, 8 vices, 4-humor temperament, loyalty, family),
(3) layered entity profile l1/l2/l3 where l3 carries the Jung psychotype + hidden motivations.
So psychotype and physiology-ish state coexist by design as separate models. Closest analogues to
the TNS65 numbers: `health` (numeric 0-100 state), `vices.wrath` (a vice, not a state),
`mood` (a string). hunger=30 / anger=60 do NOT correspond to any TNS35 field.

### Group 1 — runtime state: `NPCProfile` (`app/src/models/npc-state.ts`)
- Fields, `app/src/models/npc-state.ts:29-42`: name, uid, shortTerm[], longTermEpisodic[],
  location, health, mood, goals[], inventory[], tags (Record), updatedAt, skills.
- `EpisodicMemory` (`npc-state.ts:6-15`): id, timestamp, description, importance, emotion,
  involvedEntities[], location, consolidated.
- `NPCSkills` (`npc-state.ts:17-27`): strength, dexterity, charisma, intelligence, wisdom, luck,
  combat_skill, persuasion, stealth (9 RPG-style stats, default 0.5 — `npc-state.ts:57-67`).
- Defaults: health = 100 (`npc-state.ts:51`), mood = "neutral" (`npc-state.ts:52`).
- Persistence: world.db `npc_state` (`app/src/services/npc-runtime.ts:6,63,76`); the only
  physiology mutator is `adjustHealth(name, delta)` (`npc-runtime.ts:177-186`).

### Group 2 — economy: `NPCEconomyState` (`app/src/models/npc-stats.ts`)
- `app/src/models/npc-stats.ts:44-55`: stats, vices, age, temperament, loyalty, income, taxRate,
  totalBribes, familyExpenses, children.
- `NPCStats` (`npc-stats.ts:10-17`): wealth, power, popularity, health, experience, intrigue.
- `Vices` (`npc-stats.ts:19-28`): gluttony, drunkenness, greed, lust, sloth, wrath, pride, envy
  (8, default 0 — `npc-stats.ts:64-75`). `wrath` is the nearest thing to "anger" — a slowly
  evolving vice, not a per-turn state value.
- `Temperament` (`npc-stats.ts:9`): choleric | sanguine | melancholic | phlegmatic (4 humors).
- `FamilyExpenses` (`npc-stats.ts:30-35`): wife, children, food, clothing;
  `Inheritance` (`npc-stats.ts:37-42`): wealth, profession, traits[], vices.
- `stamina` appears only as an `ageDecay` input (`npc-stats.ts:108-111`,
  `peak = 25 + stamina*10`) — age-driven stat decay, not a hunger/fatigue meter.
- Food is rank-level economy, not an NPC trait: `app/src/rules/economy/ranks.json:7-16`
  (foodConsumption/foodCost per rank); `economy_npc` table carries food
  (`app/docs/03-data.md:51`).

### Group 3 — layered entity profile (l1/l2/l3): `LayeredProfile` (`app/src/models/entity.ts`)
- Shape: `app/src/models/entity.ts:28-41` — `LayeredProfile { l1, l2, l3 }`, each an open
  `LayerDict` (key-value bag); defaults `{ l1: {}, l2: {}, l3: {} }` (`app/src/store/entity-store.ts:158`).
- l1 (visible): name, type, summary, tags, relationships — `entity.ts:43-64`.
- l2 (known): current_location (`app/src/services/npc-runtime.ts:91`), description
  (`app/src/services/roleplay/handlers/command-handler.ts:26`), personality (free text —
  `app/src/services/agents/actor.ts:120`), inventory (`app/src/services/context-builder.ts:241`),
  status (quests — `context-builder.ts:170`).
- l3 (hidden): psychotype (`JungianProfile` — `app/src/services/roleplay-engine.ts:421`),
  hiddenMotivation (`actor.ts:67`, `app/src/services/simulation-engine.ts:97`),
  secretGoals (`actor.ts:68`).
- Every NPC can carry a Jung psychotype: `assignNpcPsychotype` → `JungianProfile`
  (`app/src/services/jungian-profiler.ts:309-340`), consumed via `n.profile.l3.psychotype`
  (`roleplay-engine.ts:421`). This is the designed coexistence: state (l1/l2 + NPCProfile +
  economy) vs personality (l3 Jung profile + temperament + l2.personality text).

### hunger / anger / Big Five — negative verification
- ripgrep `\b(hunger|anger|satiety|fatigue|stamina)\b` over the whole TNS35 tree: code hits only
  `stamina` (`app/src/models/npc-stats.ts:108,111`); hunger/anger/fatigue hits are exclusively
  literary corpus text (`artifacts/shelf-2026-09-19/{alice,sherlock,yankee}/source.txt`,
  `scratch/shelf-src/verne-20000.txt`) — no field, interface, or column anywhere.
- Earlier `anger` greps hit only substrings of "danger/stranger/Danger" (e.g.
  `app/src/models/intent.ts:24` risk_level) — no anger field behind them.
- ripgrep `Big Five|bigfive|big-five|OCEAN|пятифактор` over TNS35: zero code/data hits (only
  "PATIENCE"/"INDIAN OCEAN" substrings in literature). There is no Big Five personality model.

### Bottom line for TNS65
- DOCS/MECHANICS.md #53's "hunger=30, anger=60" is NOT a TNS35 snapshot field; TNS35 NPCs carry
  health (0-100), mood (string), 8 vices (incl. wrath), 6 economy stats, 4-humor temperament,
  9 skills, memories, and a l3 Jung psychotype + hiddenMotivation/secretGoals.
- So: personality (Jung psychotype + temperament) and physiological/state values (health, mood,
  vices, economy) coexist by design as separate models. The "Big Five-style" label should be
  corrected to: state + economy + layered Jung profile.
