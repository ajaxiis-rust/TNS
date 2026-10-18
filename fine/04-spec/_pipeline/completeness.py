#!/usr/bin/env python3
"""Final completeness: every CANON/UI registry mechanic covered by >=1 atom; assemble _coverage.md.
Exit 0 = nothing missing; 1 = missing list non-empty (written to completeness-report.txt)."""
import glob, json, re, sys

ROOT = "/media/ajaxiis/ext3/TNS65/fine/04-spec"
PIPE = f"{ROOT}/_pipeline"
REG = "/media/ajaxiis/ext3/TNS65/fine/00-recon/mimo-mechanics-registry.md"

reg = open(REG, encoding="utf-8").read()
entries = {}
for m in re.finditer(r"- \*\*(R-\d{3}) \| ([^*]+)\*\* ([^\n]*)", reg):
    rid, name, rest = m.groups()
    st = re.search(r"Status:\s*(\w+)", rest)
    entries[rid] = {"name": name.strip(), "status": st.group(1) if st else "?", "text": rest}
CANCELLED_BY_OWNER = {"R-149", "R-150"}  # owner 17.10: world-config cancelled
# EXEMPT (18.10, owner "close the holes yourself"): document-meta mechanics, no product norms.
EXEMPT_META = {
    "R-002": "history of migration 55->65 - about the paradigm document, not the product",
    "R-007": "logic of the paradigm document (six parts, stable numbering) - not the product",
}

live = {r for r, e in entries.items() if e["status"] in ("CANON", "UI")
        and "CANCELLED" not in e["text"] and r not in CANCELLED_BY_OWNER}
live -= set(EXEMPT_META)
plan = {r for r, e in entries.items() if e["status"] == "PLAN"}

cov = {}   # atom -> refs
for f in sorted(glob.glob(f"{PIPE}/coverage-*.txt")):
    for line in open(f, encoding="utf-8"):
        m = re.match(r"([A-Z]{1,3}-\d{1,2}):\s*(.+)", line.strip())
        if m:
            cov[m.group(1)] = m.group(2).strip()

r2a = {}
for atom, refs in cov.items():
    for r in re.findall(r"R-\d{3}", refs):
        r2a.setdefault(r, []).append(atom)

covered = set(r2a)
missing = sorted(live - covered)
plan_uncovered = sorted(plan - covered)

lines = ["# Покрытие реестра атомами спецификации (генерируется completeness.py, не пишется руками)", "",
         f"Покрыто: {len(covered & live)}/{len(live)} живых (CANON/UI). Отменено владельцем: {len(CANCELLED_BY_OWNER)}. Вне продукта (EXEMPT): {len(EXEMPT_META)}.", ""]
for r, why in EXEMPT_META.items():
    lines.append(f"- {r} EXEMPT — {entries[r]['name']}: {why}")
for r in sorted(live & covered):
    e = entries[r]
    lines.append(f"- {r} ({e['status']}) {e['name']} → {', '.join(sorted(r2a[r]))}")
if missing:
    lines += ["", f"## НЕ ПОКРЫТО ({len(missing)})", ""]
    for r in missing:
        lines.append(f"- {r} ({entries[r]['status']}) {entries[r]['name']}")
lines += ["", f"PLAN вне покрытия (справочно, {len(plan_uncovered)}): {', '.join(plan_uncovered) if plan_uncovered else '—'}", ""]
open(f"{ROOT}/_coverage.md", "w", encoding="utf-8").write("\n".join(lines))
open(f"{PIPE}/completeness-report.txt", "w", encoding="utf-8").write(
    "MISSING:\n" + ("\n".join(missing) if missing else "none") + "\n")
print(f"live covered {len(covered & live)}/{len(live)}; missing {len(missing)}; plan uncovered {len(plan_uncovered)}")
sys.exit(1 if missing else 0)
