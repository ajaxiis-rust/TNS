#!/usr/bin/env bash
set -uo pipefail

# Сводный прогон статики интерфейса (T-08-14).
# Два режима:
#   release — статический линтер без Node; входит в gates.sh и выполняется всегда.
#   dev     — то же плюс node --check по каждому .js (только машина разработчика).
#
# На машине игрока Node нет и не появляется: release-режим node не вызывает.

usage() {
    echo "usage: $0 {release|dev}" >&2
    exit 2
}

[ "$#" -eq 1 ] || usage
mode="$1"
case "$mode" in
    release|dev) ;;
    *) usage ;;
esac

# Resolve the engine root (this script lives in engine/scripts).
cd "$(dirname "$0")/.."

fail=0

# Статический линтер по всему engine/web/ — без Node, общий для обоих режимов.
if ! bash scripts/ui-lint.sh; then
    echo "FAIL: scripts/ui-lint.sh"
    fail=1
fi

# Release-режим: статики достаточно, node не вызывается вовсе.
[ "$mode" = "release" ] && exit "$fail"

# Dev-режим: node --check для каждого .js; node нужен только разработчику.
echo "node --check: только машина разработчика"
if node_bin="$(command -v node 2>/dev/null)" && [ -n "$node_bin" ]; then
    while IFS= read -r -d '' js; do
        if ! node --check "$js"; then
            echo "FAIL: node --check $js"
            fail=1
        fi
    done < <(find web -name '*.js' -print0 | sort -z)
else
    echo "FAIL: node недоступен, а режим dev требует node --check" >&2
    fail=1
fi

exit "$fail"
