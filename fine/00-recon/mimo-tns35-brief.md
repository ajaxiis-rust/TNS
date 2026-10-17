# SEARCH: TNS35 facts — estates, profiler metrics, NPC trait fields

Role: RECON. Answer THREE questions from the TNS35 codebase. Sources: codebase-memory MCP
(project name: `media-ajaxiis-ext3-TNS35`, index fresh) + files on disk under `/media/ajaxiis/ext3/TNS35`.
Prefer targeted search_code / search_graph / grep over get_architecture dumps.

## Q1. ESTATES (сословия)

TNS65 canon: character batch (пачка) = 16 Jung psychotypes × 6 estates = 96 cards.
The 6 estates are NOT enumerated in any TNS65 source. Owner ruling 17.10: take them from TNS35.
FIND: the list/enum/table of social estates / rank ladder / classes used by the TNS35 simulation.
Search words (RU + EN): сослови, ранг, ladder, estate, class, caste, rank, social, страт.
Report the exact list (values, levels, names) with file:line citations for every element.

## Q2. JUNG PROFILER METRICS

TNS65 sources disagree: «16 психотипов Юнга» (batch axis) vs «юнгианский профилировщик TNS35 (14 метрик)».
FIND in TNS35: the Jung profiler implementation — exact metric count and their names
(axes E/I N/S T/F J/P? plus what?), and any enum of psychotypes (how many distinct types defined).
Verdict needed: is the real number 14 metrics, 16 types, or other — with evidence (file:line).

## Q3. NPC TRAIT FIELDS

TNS65 DOCS/MECHANICS.md (#53) describes NPC snapshots with values like hunger=30, anger=60
(labelled "Big Five-style" by the recon registry — suspect mislabel).
FIND in TNS35: what fields an NPC snapshot/profile actually carries — physiology values
(hunger, health, anger...), personality axes, psychotype, stats. List the field groups with file:line.
Verdict needed: are hunger/anger physiology/state (coexisting with Jung psychotype by design),
or a separate personality model?

## Rules

- Read-only. Zero invention. Every claim = file:line (path under TNS35 + line).
- If something is NOT found: write NOT FOUND + the exact queries you tried.
- No file writes except the single output file below.

## Output

write_file → `/media/ajaxiis/ext3/TNS65/fine/00-recon/mimo-tns35-search.md`
English, three sections (Q1/Q2/Q3), each: verdict first, then evidence list.
