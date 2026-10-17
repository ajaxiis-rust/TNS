# RECON: TNS65 mechanics registry (fact extraction)

Role: RECON (fact extractor). Project: TrueNeverStory TNS65 — a text narrative engine.
Goal: a COMPLETE registry of design decisions from the written sources, feeding an engineering specification later. You extract facts; decisions and synthesis happen elsewhere.

## Read these sources (absolute paths, in this order)

1. `/media/ajaxiis/ext3/TNS65/TNS65-PARADIGM.md` — THE canon (462 lines, numbered items p.1–21, six turn-logic parts). PRIMARY source; everything here is CANON unless marked otherwise.
2. `/media/ajaxiis/ext3/TNS65/DOCS/` — read `00-START-HERE.md` first, then 01–09 and `MECHANICS.md`. NOTE: partially OUTDATED (e.g. the world-birth form in 01-PARADIGM is older than the canon). Mark entries from here STALE where the canon contradicts them; otherwise PLAN/SUPPORT.
3. `/media/ajaxiis/ext3/TNS65/basically-arcs-20261011.md` — arcs bake plan (de-anonymization, slots, name pool, censorship).
4. `/media/ajaxiis/ext3/TNS65/RAW/base.md` — seed base fill plan.
5. `/media/ajaxiis/ext3/TNS65/RAW/INDEX.md` — index of raw sources.
6. `/media/ajaxiis/ext3/TNS65/GLOSSARY.md` — wording canon (terms).
7. `/media/ajaxiis/ext3/TNS65/repair-card-20261012.md` — repair card (open).
8. `/media/ajaxiis/ext3/TNS65/interface/` — decision bank (UI copy from the old house; server calls already stripped). Read `world-config.html`, `settings.html`, `index.html`, `worlds.html`, `models.html`: extract FIELD INVENTORIES (form field names, options, toggles, tabs) — each is a made UI decision.
9. `/media/ajaxiis/ext3/TNS65/AGENT-MECHANICS.md` — the section "TNS project state" (~lines 80–102) plus "TNS65 Паганель data schema" paragraph.
10. List `/media/ajaxiis/ext3/TNS65/work/`, `/media/ajaxiis/ext3/TNS65/map/`, `/media/ajaxiis/ext3/TNS65/Levant/` via search_files (pattern `*`, target files). Read small text files; for big/binary ones note name + size only.

## Output — ONE file

write_file → `/media/ajaxiis/ext3/TNS65/fine/00-recon/mimo-mechanics-registry.md`
Full overwrite, UTF-8, LF, plain markdown. English text; Russian terms (Пила, Ткач, Челнок, Селектор, Цензор, Редактор, Труппа, Толмач, Паганель, Гефест, Пифия, анкета, варка, оседание and similar) quoted EXACTLY as literal bytes — never transliterate, never translate them.

Format — numbered registry grouped by source:

```
### <source short name>
- **R-NNN | <short name>** — essence in 1–3 telegraphic lines (what exactly is fixed: numbers, thresholds, lists, fields, who executes). Source: <file>:<line range or item no>. Status: CANON | PLAN | UI | STALE | OPEN.
```

If the registry grows past ~500 lines, compress entries to 1–2 lines each — completeness beats prose.

## Hard rules

- ZERO invention: only what is actually written in the sources above. A source silent on a topic = the topic does not exist for you.
- Every entry carries a pointer (file + line range or item number). No pointer → do not include the entry.
- Cover ALL numbered items p.1–21 of the canon and all six turn-logic parts — none skipped.
- Make sure these families are covered IF present in sources: world-birth form fields; NPC memory knobs; Паганель map/travel/calibration rules; language pack / Толмач duties; forge (варка) pipeline; databases (based.db, literary.db, world db — who writes, who reads); UI decisions (modal, tabs, toggles); naming rules; genre/строй lists; world/rebirth rules; NPC batch generation; luck field; expedition mode; Big-Six-style role chain of the turn.
- End the file with: (a) coverage table source → entries count + lines read; (b) list of sources/parts you could NOT parse and why; (c) top-10 ambiguities noticed (marked AMBIG, described, NOT resolved).
- Read-only everywhere EXCEPT the one output file. Never modify, move or delete any source file.
- No external network, no git commands.
