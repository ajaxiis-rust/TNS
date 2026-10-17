#!/usr/bin/env python3
"""verify-registry.py — mechanical fact-checker for the TNS65 recon registry.

Hard checks (any FAIL -> exit 1):
  F1 parse     every '- **R-NNN | ...' line yields number/name/source
  F2 pointer   cited file exists (abs path, or relative to TNS65 root, or known short name)
  F3 range     line range valid (start<=end, end within file)
  F4 dup       R-numbers unique
  F5 encoding  file decodes as UTF-8; no mojibake signatures (double-encoded Cyrillic), no U+FFFD
Soft checks (printed, do not fail the run):
  S1 anchor    >=1 distinctive token of the entry text appears within cited lines +/-10
               (0-hit entries go to the REVIEW bucket = invention suspects)
  S2 coverage  items п.1..п.21 each referenced somewhere in the registry
  S3 sections  expected source sections present among '### ' headers
  S4 status    unknown Status words listed
  S5 tail      AMBIG list and coverage table present

Usage:
  python3 verify-registry.py <registry.md>
  python3 verify-registry.py --selftest
"""
import os
import re
import sys
import tempfile

TNS = "/media/ajaxiis/ext3/TNS65"
SHORT = {
    "canon": TNS + "/TNS65-PARADIGM.md",
    "paradigm": TNS + "/TNS65-PARADIGM.md",
    "docs": TNS + "/DOCS",
    "arcs": TNS + "/basically-arcs-20261011.md",
    "base": TNS + "/RAW/base.md",
    "glossary": TNS + "/GLOSSARY.md",
    "repair": TNS + "/repair-card-20261012.md",
    "interface": TNS + "/interface",
    "mechanics": TNS + "/AGENT-MECHANICS.md",
    "index": TNS + "/RAW/INDEX.md",
}
ROLES = ["Пила", "Ткач", "Челнок", "Селектор", "Цензор", "Редактор",
         "Труппа", "Толмач", "Паганель", "Гефест", "Пифия"]
KNOWN_STATUS = {"CANON", "PLAN", "UI", "STALE", "OPEN", "AMBIG"}

ENTRY_RE = re.compile(
    r"^- \*\*R-(\d+)\s*\|\s*(?P<name>.+?)\*\*\s*[—-]\s*(?P<body>.+)$")
SRC_RE = re.compile(r"Source:\s*(?P<src>[^\s:]+)(?::(?P<a>\d+)(?:-(?P<b>\d+))?)?(?P<rest>[\s.,]|$)")
STATUS_RE = re.compile(r"Status:\s*([A-Za-z]+)")


def load_lines(path):
    with open(path, encoding="utf-8", errors="strict") as f:
        return f.read().splitlines()


def resolve_path(token):
    token = token.strip().strip("`.\"'")
    if token.startswith("/"):
        base = token
    else:
        low = token.lower()
        if low in SHORT:
            return SHORT[low]
        base = os.path.join(TNS, token)
    if not os.path.exists(base):
        for ext in (".md", ".html"):
            cand = base + ext
            if os.path.exists(cand):
                return cand
    return base


def anchor_tokens(text):
    words = re.findall(r"[A-Za-zА-Яа-яЁё0-9]{5,}", text.lower())
    words = [w for w in words if not re.fullmatch(r"\d+", w)]
    words.sort(key=len, reverse=True)
    return words[:4]


def mojibake_hits(text):
    sigs = re.findall(r"Ð[\x80-\xBF\u0400-\u04FF]|â€|Ð¤|ï¿½", text)
    return len(sigs)


def check(registry_path, report):
    fails, warns, review = [], [], []
    try:
        raw = open(registry_path, "rb").read()
        text = raw.decode("utf-8")
    except UnicodeDecodeError as e:
        print("[FAIL F5] not valid UTF-8: %s" % e)
        return 1
    if "\ufffd" in text:
        fails.append("F5 U+FFFD replacement chars present")
    mh = mojibake_hits(text)
    if mh > 5:
        fails.append("F5 mojibake signatures: %d (double-encoded Cyrillic?)" % mh)

    lines = text.splitlines()
    entries = []
    for i, ln in enumerate(lines, 1):
        m = ENTRY_RE.match(ln)
        if not m:
            continue
        num = int(m.group(1))
        srcm = SRC_RE.search(m.group("body"))
        stm = STATUS_RE.search(m.group("body"))
        entries.append({
            "line": i, "num": num, "name": m.group("name").strip(),
            "body": m.group("body"),
            "src": srcm.group("src") if srcm else None,
            "a": int(srcm.group("a")) if srcm and srcm.group("a") else None,
            "b": int(srcm.group("b")) if srcm and srcm.group("b") else None,
            "status": stm.group(1) if stm else None,
        })
        if not srcm:
            fails.append("F1 R-%03d (line %d): no Source field" % (num, i))

    # F4 duplicates
    seen = {}
    for e in entries:
        seen.setdefault(e["num"], []).append(e["line"])
    for num, ls in sorted(seen.items()):
        if len(ls) > 1:
            fails.append("F4 duplicate R-%03d at lines %s" % (num, ls))

    # F2/F3 pointers + S1 anchors
    cache = {}
    for e in entries:
        if not e["src"]:
            continue
        path = resolve_path(e["src"])
        if os.path.isdir(path):
            continue  # directory citation: no line/anchor check possible
        if not os.path.isfile(path):
            fails.append("F2 R-%03d: source file not found: %s" % (e["num"], e["src"]))
            continue
        if path not in cache:
            try:
                cache[path] = load_lines(path)
            except Exception as ex:
                fails.append("F2 R-%03d: cannot read %s (%s)" % (e["num"], path, ex))
                cache[path] = []
                continue
        flines = cache[path]
        n = len(flines)
        if e["a"] is None:
            continue  # item-pointer (п.N) or dir citation; covered by S2/notes
        a, b = e["a"], e["b"] or e["a"]
        if not (1 <= a <= b <= n):
            fails.append("F3 R-%03d: line range %d-%d invalid for %s (%d lines)"
                         % (e["num"], a, b, e["src"], n))
            continue
        window = "\n".join(flines[max(1, a - 10) - 1:min(n, b + 10)]).lower()
        toks = anchor_tokens(e["name"] + " " + e["body"])
        if toks and not any(t in window for t in toks):
            review.append("S1 R-%03d (%s:%d-%d): 0/%d anchor tokens hit [%s]"
                          % (e["num"], e["src"], a, b, len(toks), ", ".join(toks)))

    # S2 п.1..21 coverage
    missing_items = []
    for n in range(1, 22):
        if not re.search(r"[пp]\.\s*%d(?!\d)" % n, text):
            missing_items.append(n)
    if missing_items:
        warns.append("S2 canon items not referenced anywhere: %s" % missing_items)

    # S3 sections
    headers = [l for l in lines if l.startswith("### ")]
    hlow = " ".join(headers).lower()
    for key in ("canon", "docs", "arcs", "glossary", "interface"):
        if key not in hlow:
            warns.append("S3 no '### ' section mentioning '%s'" % key)

    # S4 statuses
    bad_status = sorted({e["status"] for e in entries
                         if e["status"] and e["status"] not in KNOWN_STATUS})
    if bad_status:
        warns.append("S4 unknown status words: %s" % bad_status)
    nostatus = [e["num"] for e in entries if not e["status"]]
    if nostatus:
        warns.append("S4 entries without Status: %s" % nostatus)

    # S5 tail
    if "AMBIG" not in text:
        warns.append("S5 no AMBIG list at the end")
    if not re.search(r"coverage|покрыти", text, re.I):
        warns.append("S5 no coverage table")

    roles_ok = [r for r in ROLES if r in text]
    report.append("entries=%d sections=%d roles_seen=%d/%d"
                  % (len(entries), len(headers), len(roles_ok), len(ROLES)))
    for w in warns:
        report.append("[warn] " + w)
    for r in review:
        report.append("[review] " + r)
    for f in fails:
        report.append("[FAIL] " + f)
    report.append("verdict: %s (fails=%d review=%d warns=%d)"
                  % ("PASS" if not fails else "FAIL", len(fails), len(review), len(warns)))
    return 0 if not fails else 1


def selftest():
    tmp = tempfile.mkdtemp(prefix="verify-reg-")
    src = os.path.join(tmp, "src.txt")
    with open(src, "w", encoding="utf-8") as f:
        f.write("verifikacionnyi-marker line one\nsecond line filtre-mekanik\nthird\nfourth\n")
    reg = os.path.join(tmp, "reg.md")
    good = ("- **R-001 | good entry** — essence about verifikacionnyi-marker. "
            "Source: %s:1-2. Status: CANON\n" % src)
    badrange = ("- **R-002 | bad range** — essence. "
                "Source: %s:900-950. Status: CANON\n" % src)
    invented = ("- **R-003 | invented anchor** — essence about izobretennyi-token. "
                "Source: %s:1-4. Status: CANON\n" % src)
    dup = ("- **R-002 | duplicate number** — essence about filtre-mekanik. "
           "Source: %s:2-3. Status: PLAN\n" % src)
    missing_src = "- **R-004 | no source field** — essence.\n"
    moji = "- **R-005 | mojibake Ð¿Ð¸Ð»Ð° name** — essence ÐµÑÐ»Ð¸ ÑÑÐ¾. Source: %s:1-1. Status: UI\n" % src
    with open(reg, "w", encoding="utf-8") as f:
        f.write("# test registry\n### canon\n")
        f.write(good + badrange + invented + dup + missing_src + moji)
        f.write("\n## coverage table\n| canon | 5 |\nAMBIG: none parsed\n")
    report = []
    rc = check(reg, report)
    out = "\n".join(report)
    print(out)
    expectations = [
        ("F3 R-002: line range 900-950", "bad range not caught"),
        ("[review] S1 R-003", "invented anchor not flagged for review"),
        ("F4 duplicate R-002", "duplicate not caught"),
        ("F1 R-004", "missing Source not caught"),
        ("F5 mojibake", "mojibake not caught"),
    ]
    ok = True
    for needle, why in expectations:
        if needle not in out:
            print("SELFTEST MISS: %s (%s)" % (needle, why))
            ok = False
    good_review_absent = "[review] S1 R-001" in out
    if good_review_absent:
        print("SELFTEST MISS: good entry R-001 wrongly flagged")
        ok = False
    print("SELFTEST %s (rc=%d)" % ("OK" if ok and rc == 1 else "BROKEN", rc))
    return 0 if ok and rc == 1 else 2


def main():
    if len(sys.argv) == 2 and sys.argv[1] == "--selftest":
        return selftest()
    if len(sys.argv) != 2:
        print(__doc__)
        return 2
    report = []
    rc = check(sys.argv[1], report)
    print("\n".join(report))
    return rc


if __name__ == "__main__":
    sys.exit(main())
