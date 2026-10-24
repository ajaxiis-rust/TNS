#!/usr/bin/env bash
set -uo pipefail

# Resolve the engine root (this script lives in engine/scripts).
cd "$(dirname "$0")/.."

if [ "$#" -lt 2 ]; then
    echo "usage: $0 <pkg> <TestName>..." >&2
    exit 2
fi

pkg="$1"
shift
names=("$@")

# Build a -run regex anchored to each requested full test name.
run_regex=""
for n in "${names[@]}"; do
    if [ -z "$run_regex" ]; then
        run_regex="^${n}$"
    else
        run_regex="${run_regex}|^${n}$"
    fi
done

json_out="$(go test -json -count=1 -run "(${run_regex})" "$pkg" 2>&1)"
go_exit=$?

# Track whether each requested name produced a PASS event.
declare -A passed
for n in "${names[@]}"; do
    passed["$n"]=0
done

while IFS= read -r line; do
    action="$(sed -n 's/.*"Action":"\([^"]*\)".*/\1/p' <<< "$line")"
    test="$(sed -n 's/.*"Test":"\([^"]*\)".*/\1/p' <<< "$line")"
    if [ "$action" = "pass" ] && [ -n "$test" ]; then
        for n in "${names[@]}"; do
            if [ "$test" = "$n" ]; then
                passed["$n"]=1
            fi
        done
    fi
done <<< "$json_out"

if [ "$go_exit" -ne 0 ]; then
    echo "FAIL: go test exited with status $go_exit" >&2
    echo "$json_out" >&2
    exit "$go_exit"
fi

missing=()
for n in "${names[@]}"; do
    if [ "${passed["$n"]}" != "1" ]; then
        missing+=("$n")
    fi
done

if [ "${#missing[@]}" -gt 0 ]; then
    echo "FAIL: tests without PASS:" >&2
    for n in "${missing[@]}"; do
        echo "  $n" >&2
    done
    exit 1
fi

exit 0
