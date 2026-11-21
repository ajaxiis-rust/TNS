#!/usr/bin/env bash
set -uo pipefail

# Гейт удержания на запись живых *.md (T-16-07), ступени A, B и C.
# Ступень A — буквальный поиск запрещённых синонимов из dev/GLOSSARY-TERMS.tsv
# (без учёта регистра). Ступень B — лемма-сверка того же текста резидентным
# tns65-lemmaru: ловит словоформы, которые буквальный поиск пропускает;
# недоступный демон ступень пропускает, запись не блокируется — только заметка
# в журнал. Ступень C — журнал кандидатов-чеканок (существительные вне словаря
# и стоп-слов), ничего не блокирует.
#
# Запуск: PostToolUse-хук на write|search_replace (.grok/settings.json и
# .grok/hooks/term-gate.json), stdin — JSON события; нарушение уходит модели
# замечанием в том же ходу.
# Ручной прогон: bash scripts/term-gate.sh --file DOCS/x.md
#   exit 0 — чисто, 1 — нарушения, 2 — ошибка запуска.
# Самопроверка: bash scripts/term-gate.sh --self-test.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
ROOT_DEFAULT="$(cd "$SCRIPT_DIR/../.." && pwd)"

ROOT="$ROOT_DEFAULT"
MODE=hook
FILE=""
SELF_TEST=0

usage() {
  echo "term-gate: [--file <путь>] [--root <каталог>] [--self-test]" >&2
  exit 2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --file)
      [ "$#" -ge 2 ] || usage
      MODE=file
      FILE="$2"
      shift 2
      ;;
    --root)
      [ "$#" -ge 2 ] || usage
      ROOT="$2"
      shift 2
      ;;
    --self-test)
      SELF_TEST=1
      shift
      ;;
    -h|--help)
      usage
      ;;
    *)
      echo "term-gate: неизвестный флаг: $1" >&2
      usage
      ;;
  esac
done

ROOT="$(realpath -m -- "$ROOT")"
TSV="${TNS65_TERM_GATE_TSV:-$ROOT/dev/GLOSSARY-TERMS.tsv}"
LEMMARU="${TNS65_LEMMARU_BIN:-$ROOT/engine/packs/ru/lemmatizer/tns65-lemmaru}"
JOURNAL="${TNS65_TERM_GATE_JOURNAL:-$ROOT/engine/state/journal/term-gate.log}"

# Стоп-слова ступени C: частые существительные, не кандидаты в чеканки.
STOP_WORDS="
год время человек день дело жизнь слово место работа вопрос рука лицо
файл строка текст код путь шаг задача документ таблица список набор число
версия команда тест проверка правка запись событие поле значение вызов случай
часть конец начало вид форма тип пример результат процесс система состояние
уровень данные имя роль сторона цель правило метод модель сеть точка линия
группа сервис служба запрос ответ ошибка выбор ход смысл ключ признак порядок
очередь основа сцена книга мир игрок ночь утро вечер память заявка карта
отчёт срок причина следствие итог способ размер банк якорь канон реестр
"

WORK=""
cleanup() { [ -n "$WORK" ] && rm -rf "$WORK"; }
trap cleanup EXIT
WORK="$(mktemp -d)"

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g' | tr '\n\r\t' '   '
}

# journal_event <событие> [ключ значение]... — строка журнала станка.
journal_event() {
  local event="$1" fields="" k v ts
  shift
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  while [ "$#" -ge 2 ]; do
    k="$1"
    v="$2"
    shift 2
    fields="${fields}${fields:+,}{\"key\":\"$(json_escape "$k")\",\"value\":\"$(json_escape "$v")\"}"
  done
  mkdir -p "$(dirname "$JOURNAL")" 2>/dev/null || return 0
  printf '{"ts":"%s","level":"info","source":"term-gate","event":"%s","fields":[%s]}\n' \
    "$ts" "$(json_escape "$event")" "$fields" >>"$JOURNAL" 2>/dev/null || true
}

# is_live <путь> — файл входит в живой набор T-16-06; never-edit исключены.
is_live() {
  local abs rel base
  abs="$(realpath -m -- "$1")"
  case "$abs" in
    "$ROOT"/*) rel="${abs#"$ROOT"/}" ;;
    *) return 1 ;;
  esac
  case "$rel" in
    *.md) ;;
    *) return 1 ;;
  esac
  case "/$rel" in
    */RAW/*) return 1 ;;
  esac
  base="$(basename -- "$rel")"
  [ "$base" = "brief3.md" ] && return 1
  [ "$rel" = "fine/plan/99-LEDGER.md" ] && return 1
  case "$rel" in
    DOCS/*|TNS65-PARADIGM.md|GLOSSARY.md|dev/GLOSSARY.md|fine/04-spec/*|fine/03-design/*|work/*) return 0 ;;
    fine/plan/16-distillation.md) return 0 ;;
  esac
  return 1
}

# stage_a <файл текста> — буквальные совпадения; печатает строки отчёта.
stage_a() {
  local textfile="$1" phrase term line
  if ! grep -qiFw -f "$WORK/forbidden.txt" "$textfile" 2>/dev/null; then
    return 0
  fi
  while IFS=$'\t' read -r phrase term; do
    [ -n "$phrase" ] || continue
    line="$(grep -niFw -m1 -- "$phrase" "$textfile" 2>/dev/null | cut -d: -f1)"
    [ -n "$line" ] && printf '«%s» → «%s» (строка %s)\n' "$phrase" "$term" "$line"
  done <"$WORK/forbidden.tsv"
}

# lemmatize_file <запрос> <ответ> — один вызов резидента; 1 — демон недоступен.
lemmatize_file() {
  local req="$1" resp="$2" st=0
  [ -x "$LEMMARU" ] || return 1
  timeout 5 "$LEMMARU" <"$req" >"$resp" 2>"$WORK/lemmaru.err" || st=$?
  [ "$st" -eq 0 ] || return 1
  [ -s "$resp" ] || return 1
  head -n 1 "$resp" | grep -q '"tokens"' || return 1
}

# stage_b <файл текста> <путь файла> — лемма-сверка; 1 — демон недоступен.
# Один вызов резидента обслуживает три пакета запросов: запрещённые синонимы,
# текст записи и словарь ступени C.
stage_b() {
  local textfile="$1" file="$2" p tp
  : >"$WORK/stage_b.txt"
  : >"$WORK/tokens.tsv"
  : >"$WORK/nouns.txt"
  : >"$WORK/known_lemmas.txt"
  if ! command -v jq >/dev/null 2>&1; then
    journal_event term.gate.skip file "$file" reason "нет jq для ступеней B и C"
    return 1
  fi
  sed 's/[^[:alnum:][:space:]-]/ /g' "$textfile" >"$WORK/lemma.txt" 2>/dev/null || : >"$WORK/lemma.txt"
  p="$(wc -l <"$WORK/pattern.req" 2>/dev/null || echo 0)"
  tp=$((p + 1))
  jq -cn --rawfile t "$WORK/lemma.txt" --argjson id "$tp" '{id:$id,text:$t}' >"$WORK/text.req" 2>/dev/null || return 1
  awk -F'\t' -v base="$tp" '
    function esc(s) { gsub(/\\/, "\\\\", s); gsub(/"/, "\\\"", s); return s }
    length($0) > 0 { n++; printf "{\"id\":%d,\"text\":\"%s\"}\n", base + n, esc($1) }
  ' "$WORK/known.txt" >"$WORK/known.req" 2>/dev/null || : >"$WORK/known.req"
  cat "$WORK/pattern.req" "$WORK/text.req" "$WORK/known.req" >"$WORK/all.req" 2>/dev/null || return 1
  if ! lemmatize_file "$WORK/all.req" "$WORK/all.resp"; then
    journal_event term.gate.skip file "$file" reason "демон tns65-lemmaru недоступен"
    return 1
  fi

  jq -r --argjson p "$p" --argjson tp "$tp" '
    if .id == $tp then
      .tokens[] | ["T", .form, (.lemma // ""),
        ([.parses[]?.lemma] | unique | join("|")),
        (if ((.parses|length) > 0)
         then ([.parses[] | select(.pos == "NOUN") | .lemma] | unique | join("|"))
         else (if .pos == "NOUN" then .lemma else "" end) end)] | @tsv
    elif .id < $tp then
      ["P", (.id|tostring), (.tokens | map(.parses[0].lemma // .lemma) | join(" ")),
        (if (.tokens|length) == 1 then ([.tokens[0].parses[]?.lemma] | unique | join("|")) else "" end)] | @tsv
    else
      .tokens[] | ((.parses[]?.lemma // empty), (.lemma // empty)) as $l | ["K", $l] | @tsv
    end' "$WORK/all.resp" 2>/dev/null >"$WORK/all.tsv" || : >"$WORK/all.tsv"

  if [ "$p" -gt 0 ] && grep -q '^P' "$WORK/all.tsv" 2>/dev/null; then
    awk -F'\t' '$1 == "P" { print $2 "\t" $3 "\t" $4 }' "$WORK/all.tsv" >"$WORK/pattern_raw.tsv"
    awk -F'\t' 'NR==FNR { term[FNR]=$2; phrase[FNR]=$1; next } { print $1 "\t" term[$1] "\t" phrase[$1] "\t" $2 "\t" $3 }' \
      "$WORK/forbidden.tsv" "$WORK/pattern_raw.tsv" >"$WORK/pattern.tsv" 2>/dev/null || : >"$WORK/pattern.tsv"
  else
    : >"$WORK/pattern.tsv"
  fi

  awk -F'\t' '$1 == "T" { print $2 "\t" $3 "\t" $4 }' "$WORK/all.tsv" >"$WORK/tokens.tsv"
  awk -F'\t' '$1 == "T" { n = split($5, a, "|"); for (i = 1; i <= n; i++) if (a[i] != "") print a[i] }' \
    "$WORK/all.tsv" | LC_ALL=C sort -u >"$WORK/nouns.txt"
  awk -F'\t' '$1 == "K" && $2 != "" { print $2 }' "$WORK/all.tsv" |
    LC_ALL=C sort -u >"$WORK/known_lemmas.txt"

  [ -s "$WORK/pattern.tsv" ] || return 0
  [ -s "$WORK/tokens.tsv" ] || return 0

  awk -F'\t' '
    function note(form, lemma, term, phrase) {
      key = form SUBSEP term
      if (key in seen) return
      seen[key] = 1
      printf "«%s» (лемма «%s») → «%s»\n", form, lemma, term
      hits++
    }
    NR == FNR {
      seq = $4
      if (seq == "") next
      n = split(seq, w, " ")
      if (n > 1) {
        multi++
        multiTerm[multi] = $2
        multiPhrase[multi] = $3
        multiN[multi] = n
        for (i = 1; i <= n; i++) multiWord[multi, i] = w[i]
        firstIdx[w[1]] = firstIdx[w[1]] " " multi
      } else {
        split($5 != "" ? $5 : $4, alt, "|")
        for (i in alt) if (!(alt[i] in single)) single[alt[i]] = $2
      }
      next
    }
    {
      form[FNR] = $1
      lemmas[FNR] = $3
      count = FNR
    }
    END {
      for (i = 1; i <= count; i++) {
        m = split(lemmas[i], alt, "|")
        for (j = 1; j <= m; j++) {
          if (alt[j] in single) note(form[i], alt[j], single[alt[j]], "")
        }
      }
      for (i = 1; i <= count; i++) {
        m = split(lemmas[i], alt, "|")
        for (j = 1; j <= m; j++) {
          if (!(alt[j] in firstIdx)) continue
          pn = split(firstIdx[alt[j]], plist, " ")
          for (q = 1; q <= pn; q++) {
            p = plist[q]
            if (i + multiN[p] - 1 > count) continue
            ok = 1
            for (j2 = 2; j2 <= multiN[p]; j2++) {
              nn = split(lemmas[i + j2 - 1], l2, "|")
              hit = 0
              for (k = 1; k <= nn; k++) if (l2[k] == multiWord[p, j2]) hit = 1
              if (!hit) { ok = 0; break }
            }
            if (ok) note(form[i], multiWord[p, 1], multiTerm[p], multiPhrase[p])
          }
        }
      }
      if (hits > 20) printf "… и ещё %d\n", hits - 20
    }
  ' "$WORK/pattern.tsv" "$WORK/tokens.tsv" >"$WORK/stage_b.txt" 2>/dev/null || true
  return 0
}

# stage_c <файл> — кандидаты-чеканки: существительные вне словаря и стоп-слов.
stage_c() {
  local file="$1" words
  cat "$WORK/known.txt" "$WORK/known_lemmas.txt" 2>/dev/null |
    awk 'length($0) > 0' | LC_ALL=C sort -u >"$WORK/known_all.txt"
  [ -s "$WORK/known_all.txt" ] || cp "$WORK/known.txt" "$WORK/known_all.txt"
  words="$(
    awk -F'\t' 'NR==FNR { known[$1]=1; next } tolower($0) in known { next } { print }' \
      "$WORK/known_all.txt" "$WORK/nouns.txt" 2>/dev/null |
      awk 'length($0) >= 3' || true
  )"
  [ -n "$words" ] || return 0
  journal_event term.gate.coinage file "$file" words "$(printf '%s' "$words" | tr '\n' ';' | cut -c1-400)"
}

# build_tables — плоский список запрещённых синонимов и известных слов.
build_tables() {
  awk -F'\t' '
    NR == 1 { next }
    {
      n = split($5, a, ";")
      for (i = 1; i <= n; i++) {
        gsub(/^[ \t]+|[ \t]+$/, "", a[i])
        if (a[i] == "" || a[i] == "—") continue
        printf "%s\t%s\n", a[i], $1
      }
    }
  ' "$TSV" | awk -F'\t' '!seen[$1]++' >"$WORK/forbidden.tsv" || return 1
  cut -f1 "$WORK/forbidden.tsv" >"$WORK/forbidden.txt" || return 1

  {
    cut -f1 "$TSV" | tail -n +2
    cut -f1 "$WORK/forbidden.tsv"
    printf '%s\n' "$STOP_WORDS"
  } | awk '{ print tolower($0) }' | tr '[:space:]' '\n' |
    sed 's/ё/е/g; s/[^[:alnum:]-]//g' |
    awk 'length($0) > 0' | LC_ALL=C sort -u >"$WORK/known.txt" || return 1

  : >"$WORK/pattern.req"
  awk -F'\t' '
    function esc(s) { gsub(/\\/, "\\\\", s); gsub(/"/, "\\\"", s); return s }
    {
      n++
      printf "{\"id\":%d,\"text\":\"%s\"}\n", n, esc($1)
    }
  ' "$WORK/forbidden.tsv" >"$WORK/pattern.req" || return 1
}

# check_text <текст-файл> <путь файла> — общий прогон ступеней A/B/C.
# Возвращает 0 — нарушений нет, 1 — есть.
check_text() {
  local textfile="$1" file="$2" a_out b_out a_n b_n report
  a_out="$(stage_a "$textfile")"
  stage_b "$textfile" "$file" || true
  [ -s "$WORK/nouns.txt" ] && stage_c "$file"
  b_out="$WORK/stage_b.txt"
  if [ -s "$b_out" ] && [ -n "$a_out" ]; then
    printf '%s\n' "$a_out" | sed -n 's/^«\([^»]*\)».*/\1/p' | awk '{ print tolower($0) }' >"$WORK/a_forms.txt"
    awk 'NR==FNR { skip[tolower($0)] = 1; next }
      { split($0, p, "»"); f = p[1]; sub(/^«/, "", f); if (tolower(f) in skip) next; print }' \
      "$WORK/a_forms.txt" "$b_out" >"$WORK/stage_b_dedup.txt"
    b_out="$WORK/stage_b_dedup.txt"
  fi
  a_n=0
  b_n=0
  [ -n "$a_out" ] && a_n="$(printf '%s\n' "$a_out" | grep -c .)"
  [ -s "$b_out" ] && b_n="$(grep -v '^…' "$b_out" | grep -c .)"

  if [ "$a_n" -eq 0 ] && [ "$b_n" -eq 0 ]; then
    return 0
  fi
  report="Гейт удержания T-16-07: ${file#"$ROOT"/} — нарушений: $((a_n + b_n))."
  [ "$a_n" -gt 0 ] && report="$report
Ступень A (буквальная): $a_out"
  [ "$b_n" -gt 0 ] && report="$report
Ступень B (леммы): $(head -n 20 "$b_out")"
  report="$report
Замени по таблице dev/GLOSSARY-TERMS.tsv и перепиши файл тем же ходом."
  REPORT="$report"
  return 1
}

REPORT=""

# --- самопроверка ---------------------------------------------------------
if [ "$SELF_TEST" -eq 1 ]; then
  SELF="$SCRIPT_DIR/$(basename "${BASH_SOURCE[0]:-$0}")"
  TMP="$(mktemp -d)"
  trap 'rm -rf "$TMP" "$WORK"' EXIT
  fails=0
  checks=0
  pass() { checks=$((checks + 1)); }
  fail() { checks=$((checks + 1)); fails=$((fails + 1)); echo "self-test: FAIL — $*" >&2; }

  mkdir -p "$TMP/dev" "$TMP/DOCS" "$TMP/RAW" "$TMP/fine/plan" "$TMP/engine" "$TMP/work"
  cp "$ROOT_DEFAULT/dev/GLOSSARY-TERMS.tsv" "$TMP/dev/"

  gate() { TNS65_TERM_GATE_JOURNAL="$TMP/journal.log" bash "$SELF" --root "$TMP" "$@"; }

  printf 'Здесь пайплайн и всё.\n' >"$TMP/DOCS/lit.md"
  out="$(gate --file "$TMP/DOCS/lit.md")"
  code=$?
  if [ "$code" -eq 1 ] && [[ "$out" == *"Ступень A"* ]] && [[ "$out" == *"Алгоритм"* ]]; then
    pass
  else
    fail "ступень A не поймала запрещённое слово (exit $code)"
  fi

  printf 'Живой ход: якорь, банк, мастерская, Ткач.\n' >"$TMP/DOCS/clean.md"
  out="$(gate --file "$TMP/DOCS/clean.md")"
  code=$?
  if [ "$code" -eq 0 ] && [[ "$out" == *"term-gate: ok"* ]]; then pass; else fail "чистая запись не прошла (exit $code)"; fi

  printf 'пайплайн\n' >"$TMP/RAW/raw.md"
  printf 'пайплайн\n' >"$TMP/fine/plan/99-LEDGER.md"
  printf 'пайплайн\n' >"$TMP/fine/plan/brief3.md"
  printf 'пайплайн\n' >"$TMP/engine/README.md"
  for f in RAW/raw.md fine/plan/99-LEDGER.md fine/plan/brief3.md engine/README.md; do
    gate --file "$TMP/$f" >/dev/null 2>&1
    code=$?
    if [ "$code" -eq 0 ]; then pass; else fail "never-edit/не живой путь проверялся: $f"; fi
  done

  printf '{"tool_name":"write","tool_input":{"file_path":"%s/DOCS/lit.md","content":"пайплайн тут"}}' "$TMP" |
    TNS65_TERM_GATE_JOURNAL="$TMP/journal.log" bash "$SELF" --root "$TMP" >"$TMP/hook.out" 2>/dev/null
  code=$?
  if [ "$code" -eq 0 ] && [ "$(jq -r '.decision // empty' "$TMP/hook.out" 2>/dev/null)" = "block" ] &&
    [[ "$(jq -r '.reason // empty' "$TMP/hook.out")" == *"Ступень A"* ]]; then
    pass
  else
    fail "хук не вернул замечание модели"
  fi

  printf '{"tool_name":"write","tool_input":{"file_path":"%s/DOCS/clean.md","content":"живой ход"}}' "$TMP" |
    TNS65_TERM_GATE_JOURNAL="$TMP/journal.log" bash "$SELF" --root "$TMP" >"$TMP/hook-clean.out" 2>/dev/null
  if [ ! -s "$TMP/hook-clean.out" ]; then pass; else fail "чистый хук что-то напечатал"; fi

  printf '{"tool_name":"search_replace","tool_input":{"file_path":"%s/DOCS/lit.md","old_string":"x","new_string":"тут пайплайн"}}' "$TMP" |
    TNS65_TERM_GATE_JOURNAL="$TMP/journal.log" bash "$SELF" --root "$TMP" >"$TMP/hook-replace.out" 2>/dev/null
  if [ "$(jq -r '.decision // empty' "$TMP/hook-replace.out" 2>/dev/null)" = "block" ]; then
    pass
  else
    fail "хук не проверил new_string у search_replace"
  fi

  rm -f "$TMP/journal.log"
  gate --file "$TMP/DOCS/clean.md" >/dev/null 2>&1
  if grep -q 'term.gate.skip' "$TMP/journal.log" 2>/dev/null; then
    pass
  else
    fail "нет заметки журнала о недоступном демоне"
  fi

  start="$(date +%s%N)"
  gate --file "$TMP/DOCS/clean.md" >/dev/null 2>&1
  ms=$((($(date +%s%N) - start) / 1000000))
  if [ "$ms" -le 1000 ]; then pass; else fail "чистая запись заняла ${ms} мс"; fi

  REAL_BIN="$ROOT_DEFAULT/engine/packs/ru/lemmatizer/tns65-lemmaru"
  if [ -x "$REAL_BIN" ]; then
    big_doc="$TMP/DOCS/big.md"
    awk 'BEGIN { s = "Живой ход: якорь, банк, мастерская, Ткач, Селектор, Редактор. "; for (i = 0; i < 1000; i++) printf "%s", s; print "" }' >"$big_doc"
    start="$(date +%s%N)"
    TNS65_LEMMARU_BIN="$REAL_BIN" TNS65_TERM_GATE_JOURNAL="$TMP/journal.log" \
      bash "$SELF" --root "$TMP" --file "$big_doc" >/dev/null 2>&1
    ms=$((($(date +%s%N) - start) / 1000000))
    if [ "$ms" -le 1000 ]; then pass; else fail "чистая запись 108 КБ с демоном заняла ${ms} мс"; fi

    printf 'Вот пайплайны, конвейера ковки и шаблоны.\n' >"$TMP/DOCS/morph.md"
    out="$(TNS65_LEMMARU_BIN="$REAL_BIN" TNS65_TERM_GATE_JOURNAL="$TMP/journal.log" \
      bash "$SELF" --root "$TMP" --file "$TMP/DOCS/morph.md")"
    code=$?
    if [ "$code" -eq 1 ] && [[ "$out" == *"Ступень B"* ]]; then
      pass
    else
      fail "ступень B не поймала словоформы (exit $code)"
    fi

    rm -f "$TMP/journal.log"
    printf 'Здесь чеканка терминов и живой ход.\n' >"$TMP/DOCS/coin.md"
    TNS65_LEMMARU_BIN="$REAL_BIN" TNS65_TERM_GATE_JOURNAL="$TMP/journal.log" \
      bash "$SELF" --root "$TMP" --file "$TMP/DOCS/coin.md" >/dev/null 2>&1
    if grep -q 'term.gate.coinage' "$TMP/journal.log" 2>/dev/null &&
      grep -q 'чеканка' "$TMP/journal.log" 2>/dev/null; then
      pass
    else
      fail "ступень C не записала кандидата-чеканку"
    fi
  else
    echo "self-test: ступени B и C пропущены — нет $REAL_BIN" >&2
  fi

  if [ "$fails" -eq 0 ]; then
    echo "term-gate: self-test ok ($checks проверок)"
    exit 0
  fi
  echo "term-gate: self-test FAIL ($fails из $checks)" >&2
  exit 1
fi

# --- ручной прогон --------------------------------------------------------
if [ "$MODE" = "file" ]; then
  [ -f "$FILE" ] || { echo "term-gate: нет файла: $FILE" >&2; exit 2; }
  [ -f "$TSV" ] || { echo "term-gate: нет таблицы: $TSV" >&2; exit 2; }
  if ! is_live "$FILE"; then
    echo "term-gate: ok ${FILE#"$ROOT"/} (вне живого набора)"
    exit 0
  fi
  build_tables || { echo "term-gate: таблица не прочитана: $TSV" >&2; exit 2; }
  if check_text "$FILE" "$FILE"; then
    echo "term-gate: ok ${FILE#"$ROOT"/}"
    exit 0
  fi
  printf '%s\n' "$REPORT"
  exit 1
fi

# --- хук PostToolUse ------------------------------------------------------
if ! command -v jq >/dev/null 2>&1; then
  exit 0
fi
payload="$(cat)"
path="$(printf '%s' "$payload" | jq -r '.tool_input.file_path // .tool_input.path // empty' 2>/dev/null)"
text="$(printf '%s' "$payload" | jq -r '.tool_input.content // .tool_input.contents // .tool_input.new_string // .tool_input.new_str // empty' 2>/dev/null)"
[ -n "$path" ] || exit 0
[ -n "$text" ] || exit 0
path="$(realpath -m -- "$path")"
is_live "$path" || exit 0
[ -f "$TSV" ] || exit 0
printf '%s\n' "$text" >"$WORK/text.md"
build_tables || exit 0
if ! check_text "$WORK/text.md" "$path"; then
  jq -cn --arg reason "$REPORT" '{decision:"block",reason:$reason}' 2>/dev/null || true
fi
exit 0
