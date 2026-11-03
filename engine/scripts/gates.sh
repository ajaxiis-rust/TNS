#!/usr/bin/env bash
set -uo pipefail

# Resolve the engine root (this script lives in engine/scripts).
cd "$(dirname "$0")/.."

fail=0

# 1. gofmt -l must be empty, and gofmt itself must run cleanly.
fmt_out="$(gofmt -l .)"
fmt_exit=$?
if [ "$fmt_exit" -ne 0 ]; then
    echo "FAIL: gofmt -l errored (exit $fmt_exit)"
    fail=1
elif [ -n "$fmt_out" ]; then
    echo "FAIL: gofmt -l found unformatted files:"
    echo "$fmt_out"
    fail=1
fi

# 2. go vet ./... must pass.
if ! go vet ./...; then
    echo "FAIL: go vet ./..."
    fail=1
fi

# 3. go test ./... -race -count=1 must be green.
if ! go test ./... -race -count=1; then
    echo "FAIL: go test ./... -race -count=1"
    fail=1
fi

# 4. Forbidden silent null (os.DevNull | /dev/null) in non-test Go files.
null_out="$(grep -rnE 'os\.DevNull|/dev/null' --include='*.go' --exclude='*_test.go' . 2>/dev/null || true)"
if [ -n "$null_out" ]; then
    echo "FAIL: forbidden silent null (os.DevNull|/dev/null) found:"
    echo "$null_out"
    fail=1
fi

exit "$fail"
