#!/usr/bin/env bash
set -euo pipefail

# Собирает dev/GLOSSARY-TERMS.tsv из якорных словарей (T-16-04).
# Запуск: cd engine && bash scripts/glossary-terms.sh.
# --self-test — строит таблицу дважды во временном каталоге и сверяет:
#               побайтовое совпадение, пять колонок, все термины якорей на месте.
# Таблица генерируется, руками не редактируется (GLO-1.8).
# Колонки: термин | пакет/идентификатор | файлы | раздел канона | запрещённые синонимы.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUT="$ROOT/dev/GLOSSARY-TERMS.tsv"

ANCHORS=(
  "$ROOT/GLOSSARY.md"
  "$ROOT/dev/GLOSSARY.md"
  "$ROOT/fine/04-spec/00-glossary.md"
)

SELF_TEST=0
case "${1:-}" in
  --self-test) SELF_TEST=1 ;;
  "") ;;
  *)
    echo "glossary-terms: неизвестный флаг: $1" >&2
    exit 2
    ;;
esac

for f in "${ANCHORS[@]}"; do
  if [ ! -f "$f" ]; then
    echo "glossary-terms: нет якоря: ${f#"$ROOT"/}" >&2
    exit 2
  fi
done

# build <out> — строки таблицы якорей, слитые по термину.
build() {
  local out="$1" body
  body="$(awk '
function trim(s) { gsub(/^[ \t]+|[ \t]+$/, "", s); return s }
function clean(s) { gsub(/`/, "", s); gsub(/\*\*/, "", s); return trim(s) }
function add(hist, seen, term, value,    key) {
  if (value == "" || value == "-" || value == "—" || value == "нет") return
  key = term SUBSEP value
  if (key in seen) return
  seen[key] = 1
  hist[term] = (term in hist) ? hist[term] ";" value : value
}
function val(hist, term) { return (term in hist) ? hist[term] : "—" }
BEGIN { FS = "|" }
/^\|/ {
  if (NF != 7) next
  term = clean($2)
  ids = clean($4)
  forb = clean($5)
  canon = clean($6)
  if (term == "" || term == "термин" || term ~ /^-+$/) next
  terms[term] = 1

  n = split(ids, parts, ";")
  for (i = 1; i <= n; i++) {
    e = trim(parts[i])
    if (match(e, /[A-Za-z0-9_.\/-]+\.go/)) add(files, fileseen, term, substr(e, RSTART, RLENGTH))
    else add(pkgs, pkgseen, term, e)
  }

  n = split(canon, cparts, ";")
  for (i = 1; i <= n; i++) add(canons, canonseen, term, trim(cparts[i]))

  n = split(forb, fparts, ",")
  for (i = 1; i <= n; i++) add(forbids, forbidseen, term, trim(fparts[i]))
}
END {
  for (t in terms)
    printf "%s\t%s\t%s\t%s\t%s\n", t, val(pkgs, t), val(files, t), val(canons, t), val(forbids, t)
}
' "${ANCHORS[@]}")"
  {
    printf 'термин\tпакет/идентификатор\tфайлы\tраздел канона\tзапрещённые синонимы\n'
    printf '%s\n' "$body" | LC_ALL=C sort
  } >"$out"
}

# anchor_terms — термины колонки «термин» из якорей, по одному в строке.
anchor_terms() {
  awk '
function trim(s) { gsub(/^[ \t]+|[ \t]+$/, "", s); return s }
BEGIN { FS = "|" }
/^\|/ {
  if (NF != 7) next
  t = $2
  gsub(/`/, "", t); gsub(/\*\*/, "", t); t = trim(t)
  if (t == "" || t == "термин" || t ~ /^-+$/) next
  print t
}
' "${ANCHORS[@]}" | LC_ALL=C sort -u
}

if [ "$SELF_TEST" -eq 1 ]; then
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  build "$tmp/first.tsv"
  build "$tmp/second.tsv"
  if ! cmp -s "$tmp/first.tsv" "$tmp/second.tsv"; then
    echo "self-test: FAIL (таблица не воспроизводима)" >&2
    exit 1
  fi
  if ! awk -F'\t' 'NR > 1 && NF != 5 { exit 1 }' "$tmp/first.tsv"; then
    echo "self-test: FAIL (строка без пяти колонок)" >&2
    exit 1
  fi
  if [ -z "$(head -n 1 "$tmp/first.tsv")" ]; then
    echo "self-test: FAIL (нет шапки таблицы)" >&2
    exit 1
  fi
  anchor_terms >"$tmp/anchor-terms.txt"
  cut -f1 "$tmp/first.tsv" | tail -n +2 | LC_ALL=C sort -u >"$tmp/table-terms.txt"
  if ! comm -23 "$tmp/anchor-terms.txt" "$tmp/table-terms.txt" | grep -q .; then
    :
  else
    echo "self-test: FAIL (термины якорей пропали из таблицы):" >&2
    comm -23 "$tmp/anchor-terms.txt" "$tmp/table-terms.txt" >&2
    exit 1
  fi
  echo "self-test: ok ($(($(wc -l <"$tmp/first.tsv") - 1)) терминов, воспроизводима)"
  exit 0
fi

build "$OUT"
echo "glossary-terms: $(($(wc -l <"$OUT") - 1)) терминов → ${OUT#"$ROOT"/}"
