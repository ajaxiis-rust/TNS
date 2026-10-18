#!/usr/bin/env python3
"""Build MiMo writer briefs from 04-spec/SKELETON.md. Emits brief-NN.md + sections.json."""
import json, re, sys

ROOT = "/media/ajaxiis/ext3/TNS65/fine/04-spec"
SKEL = f"{ROOT}/SKELETON.md"
TPL = open(f"{ROOT}/_pipeline/writer-brief.tpl.md", encoding="utf-8").read()

text = open(SKEL, encoding="utf-8").read()
# split into (header, body) chunks at every '## ' heading
parts = re.split(r"(?m)^## ", text)
sections = {}
for p in parts[1:]:
    lines = p.splitlines()
    m = re.match(r"(\d\d)-([a-z-]+)\.md — (.+?) \(([A-Z]+)\)$", lines[0].strip())
    if not m:
        continue  # non-section headings (format rules, pipeline etc.)
    nn, slug, title, prefix = m.groups()
    body = "\n".join(lines[1:]).strip("\n")
    ids = re.findall(r"^- ([A-Z]{1,3}-\d{1,2})\b", body, re.M)
    assert ids, f"no atom ids parsed in {nn}"
    sections[nn] = {"file": f"{nn}-{slug}.md", "title": title, "prefix": prefix, "ids": ids, "body": body}

assert len(sections) == 10, f"expected 10 sections, got {len(sections)}: {sorted(sections)}"
meta = {k: {kk: v[kk] for kk in ("file", "title", "prefix", "ids")} for k, v in sections.items()}
json.dump(meta, open(f"{ROOT}/_pipeline/sections.json", "w", encoding="utf-8"), ensure_ascii=False, indent=1)

for nn, s in sorted(sections.items()):
    brief = TPL.replace("{FILE_NAME}", s["file"]).replace("{SECTION_TITLE}", f"{nn} — {s['title']}").replace("{SECTION_BLOCK}", s["body"])
    open(f"{ROOT}/_pipeline/brief-{nn}.md", "w", encoding="utf-8").write(brief)
    print(f"{nn}: {s['file']} atoms={len(s['ids'])} [{', '.join(s['ids'][:3])}..{s['ids'][-1]}]")
print("OK: 10 briefs + sections.json")
