#!/usr/bin/env bash
set -uo pipefail

# Сверка документов с кодом (T-12-31). Запуск: cd engine && bash scripts/docs-check.sh.
# Печатает строки DRIFT: на каждое расхождение; exit 0 — чисто, 1 — есть расхождения,
# 2 — ошибка запуска (нет jq, нет файла канона, неизвестный флаг).
# --root <dir> — корень репозитория (по умолчанию вычисляется из места скрипта).
# --self-test  — копирует документы во временный каталог, вносит расхождение и
#                ожидает, что сверка вернёт код 1 (дрейф обнаружен).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
REPO_DEFAULT="$(cd "$SCRIPT_DIR/../.." && pwd)"

ROOT="$REPO_DEFAULT"
SELF_TEST=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --root)
      [ "$#" -ge 2 ] || { echo "docs-check: --root требует аргумент" >&2; exit 2; }
      ROOT="$2"
      shift 2
      ;;
    --self-test)
      SELF_TEST=1
      shift
      ;;
    *)
      echo "docs-check: неизвестный флаг: $1" >&2
      exit 2
      ;;
  esac
done

ENGINE="$ROOT/engine"
DRIFT_COUNT=0

drift() {
  echo "DRIFT: $*"
  DRIFT_COUNT=$((DRIFT_COUNT + 1))
}

# file_contains <file> <phrase> — подстрока с нормализацией «ё»→«е» и пробелов.
file_contains() {
  local file="$1" needle="$2" content nneedle
  content="$(sed 's/ё/е/g; s/Ё/Е/g; s/[[:space:]]\+/ /g' "$file" 2>/dev/null)"
  nneedle="$(printf '%s' "$needle" | sed 's/ё/е/g; s/Ё/Е/g; s/[[:space:]]\+/ /g')"
  [[ "$content" == *"$nneedle"* ]]
}

run_checks() {
  local root="$1"
  ROOT="$root"
  ENGINE="$ROOT/engine"
  DRIFT_COUNT=0

  local birth_spec="$ROOT/fine/04-spec/03-birth.md"
  local db_doc="$ROOT/DOCS/05-DATABASES.md"
  local qr_doc="$ROOT/DOCS/09-QUICK-REFERENCE.md"
  local ancketa="$ENGINE/internal/birth/data/ancketa.json"
  local estates="$ENGINE/internal/birth/data/estates.json"
  local consumers="$ENGINE/internal/birth/data/consumers.json"
  local arch="$ENGINE/ARCHITECTURE.md"
  local schema_dir="$ENGINE/internal/db/schema"
  local ledger="$ROOT/fine/plan/99-LEDGER.md"
  local plan_dir="$ROOT/fine/plan"
  local docs_dir="$ROOT/DOCS"

  # 1. Канон из кода.
  command -v jq >/dev/null 2>&1 || { echo "docs-check: нужен jq для чтения канона" >&2; exit 2; }
  for f in "$ancketa" "$estates" "$consumers"; do
    [ -f "$f" ] || { echo "docs-check: нет файла канона: $f" >&2; exit 2; }
  done
  local genres orders econ fields modes ratings langs estates_n
  genres="$(jq '.genres | length' "$ancketa")"
  orders="$(jq '.social_orders | length' "$ancketa")"
  econ="$(jq '.economy_mods | length' "$ancketa")"
  fields="$(jq '.fields | keys | length' "$consumers")"
  modes="$(jq '.travel_modes | length' "$ancketa")"
  ratings="$(jq '.ratings | length' "$ancketa")"
  langs="$(jq '.langs | length' "$ancketa")"
  estates_n="$(jq 'length' "$estates")"
  for n in "$genres" "$orders" "$econ" "$fields" "$modes" "$ratings" "$langs" "$estates_n"; do
    case "$n" in ''|*[!0-9]*) echo "docs-check: канон не распарсился: $ancketa" >&2; exit 2;; esac
  done

  # 2. Старые и новые числа в трёх документах.
  local old_phrases=("10 жанров" "11 строёв" "4 экономических" "14 полей")
  local new_phrases=("${genres} жанров" "${orders} строёв" "${econ} экономик" "${fields} полей")
  local doc p
  for doc in "$birth_spec" "$db_doc" "$qr_doc"; do
    if [ ! -f "$doc" ]; then
      drift "нет файла для сверки чисел: ${doc#"$ROOT"/}"
      continue
    fi
    for p in "${old_phrases[@]}"; do
      file_contains "$doc" "$p" && drift "${doc#"$ROOT"/}: старое число «$p»"
    done
    for p in "${new_phrases[@]}"; do
      file_contains "$doc" "$p" || drift "${doc#"$ROOT"/}: нет нового числа «$p»"
    done
  done

  # 3. Маршруты: ROUTES.md обязан совпадать с роутером.
  if [ -f "$ENGINE/go.mod" ]; then
    local go_out rc
    go_out="$(cd "$ENGINE" && go test ./internal/webapi -run TestRouteTable -count=1 2>&1)"
    rc=$?
    if [ "$rc" -ne 0 ]; then
      drift "ROUTES.md расходится с роутером (TestRouteTable, exit $rc):"
      printf '%s\n' "$go_out" | tail -n 20 | sed 's/^/  /' >&2
    fi
  fi

  # 4. Миграции и реестр ARCHITECTURE.md.
  if [ -f "$arch" ]; then
    local mf name
    while IFS= read -r mf; do
      name="$(basename "$mf" .sql)"
      grep -qF "| ${name} |" "$arch" || drift "миграция ${mf#"$ROOT"/} без строки в реестре ARCHITECTURE.md"
    done < <(find "$schema_dir" -type f -name '*.sql' 2>/dev/null | sort)

    local line reg_name reg_base
    while IFS= read -r line; do
      reg_name="$(printf '%s\n' "$line" | sed -E 's/^\| ([0-9]{4}_[a-zA-Z0-9_]+) \|.*/\1/')"
      reg_base="$(printf '%s\n' "$line" | sed -E 's/^\| [0-9]{4}_[a-zA-Z0-9_]+ \| ([a-z]+) \|.*/\1/')"
      if [ ! -f "$schema_dir/$reg_base/$reg_name.sql" ]; then
        drift "реестр указывает на отсутствующий файл internal/db/schema/$reg_base/$reg_name.sql"
      fi
    done < <(grep -E '^\| [0-9]{4}_[a-zA-Z0-9_]+ \|' "$arch")
  else
    drift "нет engine/ARCHITECTURE.md"
  fi

  # 5. Планы: строка задачи в журнале ↔ заголовок в fine/plan/*.md.
  local ledger_ids id plan base ids
  ledger_ids="$(grep -oE '^\| T-[0-9]{2}-[0-9]{2} \|' "$ledger" | grep -oE 'T-[0-9]{2}-[0-9]{2}' | sort -u)"
  declare -A id_count
  for plan in "$plan_dir"/*.md; do
    base="$(basename "$plan")"
    [ "$base" = "99-LEDGER.md" ] && continue
    [ "$base" = "_TEMPLATE.md" ] && continue
    ids="$(grep -oE '^(###|\*\*) ?T-[0-9]{2}-[0-9]{2}\.' "$plan" | grep -oE 'T-[0-9]{2}-[0-9]{2}' | sort -u)"
    for id in $ids; do
      id_count[$id]=$(( ${id_count[$id]:-0} + 1 ))
    done
  done
  for id in $ledger_ids; do
    [ -n "${id_count[$id]:-}" ] || drift "задача $id в 99-LEDGER.md без заголовка в fine/plan/*.md"
  done
  for id in "${!id_count[@]}"; do
    [ "${id_count[$id]}" -le 1 ] || drift "ID $id повторяется между файлами плана"
  done

  # 6. Имена: полка, базы миров, неизменяемость base.db.
  local living=()
  for f in "$docs_dir"/*.md; do living+=("$f"); done
  for f in "$ROOT"/fine/04-spec/*.md; do living+=("$f"); done
  living+=("$arch")

  local f rel n1 n2 shelf=0 imm=0
  for f in "${living[@]}"; do
    [ -f "$f" ] || continue
    rel="${f#"$ROOT"/}"
    n1="$(grep -oF 'world(name).db' "$f" | wc -l | tr -d ' ')"
    n2="$(grep -oF '<slug>.db' "$f" | wc -l | tr -d ' ')"
    [ "$n1" -eq 0 ] || drift "$rel: устаревшее имя world(name).db (×$n1)"
    [ "$n2" -eq 0 ] || drift "$rel: устаревшее имя <slug>.db (×$n2)"
    grep -qF 'world_db/library/' "$f" && shelf=1
    grep -qE 'base\.db.*(неизмен|заморож|immutable|read-?only|readonly|на чтение|только читает)' "$f" && imm=1
  done
  [ "$shelf" -eq 1 ] || drift "полка world_db/library/ не описана в живых документах"
  [ "$imm" -eq 1 ] || drift "base.db не описан как неизменяемый в живых документах"

  # 7. Вход в документацию: каждый DOCS/NN-*.md в таблице DOCS/00-START-HERE.md.
  local start_here="$docs_dir/00-START-HERE.md"
  if [ -f "$start_here" ]; then
    for f in "$docs_dir"/[0-9][0-9]-*.md; do
      base="$(basename "$f")"
      [ "$base" = "00-START-HERE.md" ] && continue
      grep -qF "$base" "$start_here" || drift "$base: нет строки в таблице DOCS/00-START-HERE.md"
    done
  else
    drift "нет DOCS/00-START-HERE.md"
  fi
}

if [ "$SELF_TEST" -eq 1 ]; then
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  mkdir -p "$tmp/fine"
  cp -r "$REPO_DEFAULT/fine/plan" "$tmp/fine/plan"
  cp -r "$REPO_DEFAULT/fine/04-spec" "$tmp/fine/04-spec"
  cp -r "$REPO_DEFAULT/DOCS" "$tmp/DOCS"
  mkdir -p "$tmp/engine/internal/birth/data" "$tmp/engine/internal/db/schema"
  cp "$REPO_DEFAULT/engine/ARCHITECTURE.md" "$tmp/engine/ARCHITECTURE.md"
  cp -r "$REPO_DEFAULT/engine/internal/birth/data/." "$tmp/engine/internal/birth/data/"
  cp -r "$REPO_DEFAULT/engine/internal/db/schema/." "$tmp/engine/internal/db/schema/"

  # Вносим гарантированно обнаруживаемое расхождение: старое число в зеркале жанров.
  printf '\n10 жанров\n' >> "$tmp/fine/04-spec/03-birth.md"

  run_checks "$tmp"
  if [ "$DRIFT_COUNT" -gt 0 ]; then
    echo "self-test: ok (drift detected)"
    exit 0
  fi
  echo "self-test: FAIL (drift not detected)" >&2
  exit 1
fi

run_checks "$ROOT"
if [ "$DRIFT_COUNT" -gt 0 ]; then
  echo "docs-check: $DRIFT_COUNT расхождений" >&2
  exit 1
fi
echo "docs-check: ok"
exit 0
