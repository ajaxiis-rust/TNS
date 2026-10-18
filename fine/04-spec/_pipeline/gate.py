#!/usr/bin/env python3
"""Gate for MiMo writer output. Validates format/IDs/forbidden patterns/coverage.
Usage:
  gate.py <NN> <rawfile>                 # validate raw output for section NN; on PASS writes ../<file> + coverage-NN.txt
  gate.py --selftest                     # negative self-test: must catch all planted defects
Exit codes: 0 = PASS, 1 = FAIL (errors on stdout), 2 = selftest failed.
"""
import json, re, sys

ROOT = "/media/ajaxiis/ext3/TNS65/fine/04-spec"
PIPE = f"{ROOT}/_pipeline"
MARK = "===COVERAGE==="
FORBIDDEN = [
    (r"R-\d{2,3}", "R-reference in spec text"),
    (r"решени[ея] владельца", "owner-ruling reference"),
    (r"ТРЕБУЕТСЯ ДИЗАЙН", "design marker"),
    (r"AMBIG", "AMBIG reference"),
    (r"\d{2}\.\d{2}\.20\d{2}", "date"),
    (r"канон п\.", "canon pointer"),
    (r"Источник", "source line copied"),
    (r"http", "URL"),
]
MOJIBAKE = [r"\ufffd", r"Ð[\x80-\xBF]", r"Ñ[\x80-\xBF]"]


def check(spec, cov_lines, ids, reg_ids, min_len=1500, max_len=90000):
    errs = []
    lines = spec.splitlines()
    first = next((l for l in lines if l.strip()), "")
    if not first.startswith("###"):
        errs.append(f"first line is not an atom header: {first[:60]!r}")
    heads = re.findall(r"^### ([A-Z]{1,3}-\d{1,2})\.", spec, re.M)
    if sorted(heads) != sorted(ids):
        miss = set(ids) - set(heads); extra = set(heads) - set(ids)
        if miss: errs.append(f"missing atoms: {sorted(miss)}")
        if extra: errs.append(f"extra atoms: {sorted(extra)}")
    if len(heads) != len(set(heads)):
        dup = [h for h in set(heads) if heads.count(h) > 1]
        errs.append(f"duplicate atoms: {sorted(dup)}")
    # order follows skeleton
    seq = [h for h in heads if h in set(ids)]
    if seq and seq != ids:
        errs.append(f"atom order differs from skeleton: {seq}")
    blocks = re.split(r"(?m)^### ", spec)
    for b in blocks[1:]:
        aid = re.match(r"([A-Z]{1,3}-\d{1,2})\.", b)
        if not aid: continue
        for fld in (r"Суть:", r"Правила:", r"Приёмка:"):
            if not re.search(rf"(?m)^{fld}", b):
                errs.append(f"{aid.group(1)}: missing field {fld}")
    for pat, why in FORBIDDEN:
        for m in re.finditer(pat, spec):
            ln = spec[:m.start()].count("\n") + 1
            errs.append(f"line {ln}: forbidden ({why}): ...{spec[max(0,m.start()-25):m.end()+25]}...".replace("\n", " "))
            break  # one hit per pattern is enough
    for pat in MOJIBAKE:
        if re.search(pat, spec):
            errs.append(f"mojibake pattern {pat!r} in spec text")
    if len(spec) < min_len: errs.append(f"spec too short: {len(spec)} chars")
    if len(spec) > max_len: errs.append(f"spec too long: {len(spec)} chars")
    cov = {}
    for l in cov_lines:
        m = re.match(r"([A-Z]{1,3}-\d{1,2}):\s*(.+)", l.strip())
        if not m:
            if l.strip(): errs.append(f"bad coverage line: {l.strip()[:60]!r}")
            continue
        cov[m.group(1)] = m.group(2).strip()
    for aid in ids:
        if aid not in cov:
            errs.append(f"no coverage line for {aid}")
    for aid, refs in cov.items():
        for r in re.findall(r"R-\d{2,3}", refs):
            if r not in reg_ids:
                errs.append(f"{aid}: coverage cites unknown {r}")
    return errs


def sanitize(spec, ids):
    """Strip narration preamble (before first skeleton atom header) and trailing chatter
    (disallowed lines inside the LAST atom block). Atoms content is never touched."""
    idset = set(ids)
    starts = [m.start() for m in re.finditer(r"(?m)^### ([A-Z]{1,3}-\d{1,2})\.", spec)
              if m.group(1) in idset]
    if not starts:
        return spec, ["no skeleton atom header found at all"]
    spec = spec[starts[0]:]
    last = list(re.finditer(r"(?m)^### ", spec))[-1].start()
    block = spec[last:]
    allowed = re.compile(r"^(### |Суть:|Правила:|Данные:|Связи:|Приёмка:|\d+\.|- |\s*$)")
    lines = block.splitlines(keepends=True)
    for i, ln in enumerate(lines):
        if not allowed.match(ln):
            spec = spec[: last] + "".join(lines[:i])
            break
    return spec.rstrip() + "\n", []


def load_reg_ids():
    t = open("/media/ajaxiis/ext3/TNS65/fine/00-recon/mimo-mechanics-registry.md", encoding="utf-8").read()
    return set(re.findall(r"\*\*(R-\d{3})\b", t))


def main():
    if sys.argv[1] == "--selftest":
        ids = ["T-1", "T-2"]
        reg = {"R-001", "R-002"}
        clean_spec = "### T-1. Первый\nСуть: норма одна.\nПравила:\n1. Требование.\nПриёмка: файл есть.\n\n### T-2. Второй\nСуть: норма две.\nПравила:\n1. Требование.\nПриёмка: файл есть.\n"
        clean_cov = ["T-1: R-001", "T-2: R-002"]
        cases = []  # (name, spec, cov, expect_err_substring)
        s = clean_spec.replace("### T-2. Второй\nСуть: норма две.\nПравила:\n1. Требование.\nПриёмка: файл есть.\n", "")
        cases.append(("missing atom", s, clean_cov, "missing atoms"))
        cases.append(("extra atom", clean_spec + "\n### T-9. Лишний\nСуть: x.\nПриёмка: y.\n", clean_cov + ["T-9: R-001"], "extra atoms"))
        s = clean_spec.replace("Приёмка: файл есть.", "Итог: файл есть.", 1)
        cases.append(("no acceptance", s, clean_cov, "missing field Приёмка:"))
        cases.append(("R-ref in text", clean_spec.replace("норма одна", "норма R-001 одна"), clean_cov, "R-reference"))
        cases.append(("mojibake", clean_spec.replace("норма", "но��рма", 1), clean_cov, "mojibake"))
        cases.append(("no coverage", clean_spec, ["T-1: R-001"], "no coverage line"))
        cases.append(("bad reg cite", clean_spec, ["T-1: R-999", "T-2: R-002"], "unknown R-999"))
        caught = 0
        for name, spec, cov, sub in cases:
            errs = check(spec, cov, ids, reg, min_len=50)
            hit = any(sub in e for e in errs)
            print(f"  [{ 'OK ' if hit else 'MISS'}] {name}" + ("" if hit else f" — errors were: {errs}"))
            caught += hit
        errs = check(clean_spec, clean_cov, ids, reg, min_len=50)
        print(f"  [{'OK ' if not errs else 'MISS'}] clean passes" + ("" if not errs else f" — errors were: {errs}"))
        # sanitizer: preamble stripped, trailing chatter cut, atoms intact
        dirty = "Осмотрюсь и прочитаю реестр.\n" + clean_spec.rstrip() + "\nФайл готов, проверяй.\n"
        san, serr = sanitize(dirty, ids)
        ok_san = (not serr) and san.startswith("### T-1.") and san.rstrip().endswith("Приёмка: файл есть.") and "Осмотрюсь" not in san and "Файл готов" not in san
        print(f"  [{'OK ' if ok_san else 'MISS'}] sanitizer strips preamble+tail")
        if not ok_san: print(f"       sanitized head: {san[:40]!r} tail: {san[-40:]!r}")
        ok = caught == len(cases) and not errs and ok_san
        print(f"SELFTEST: {caught}/{len(cases)} defects caught; {'PASS' if ok else 'FAIL'}")
        sys.exit(0 if ok else 2)

    nn, rawfile = sys.argv[1], sys.argv[2]
    meta = json.load(open(f"{PIPE}/sections.json", encoding="utf-8"))
    s = meta[nn]
    raw = open(rawfile, encoding="utf-8").read().strip()
    if not raw:
        print("FAIL: empty output"); sys.exit(1)
    if MARK not in raw:
        print(f"FAIL: no {MARK} separator"); sys.exit(1)
    spec, cov = raw.rsplit(MARK, 1)
    spec = spec.strip("\n")
    spec, serr = sanitize(spec, s["ids"])
    if serr:
        print(f"FAIL ({s['file']}):")
        for e in serr: print(f"  - {e}")
        sys.exit(1)
    cov_lines = cov.splitlines()
    errs = check(spec, cov_lines, s["ids"], load_reg_ids())
    if errs:
        print(f"FAIL ({s['file']}):")
        for e in errs: print(f"  - {e}")
        sys.exit(1)
    open(f"{ROOT}/{s['file']}", "w", encoding="utf-8").write(spec + "\n")
    open(f"{PIPE}/coverage-{nn}.txt", "w", encoding="utf-8").write("\n".join(cov_lines) + "\n")
    print(f"PASS {s['file']}: {len(s['ids'])} atoms, {len(spec)} chars")


if __name__ == "__main__":
    main()
