#!/usr/bin/env bash
set -euo pipefail

# Resolve the engine root (this script lives in engine/scripts).
cd "$(dirname "$0")/.."
ENGINE_ROOT="$(pwd)"
REPO_ROOT="$(cd "$ENGINE_ROOT/.." && pwd)"

VERSION="$(git -C "$REPO_ROOT" describe --tags --always)"
LDFLAGS="-s -w -X tns65/internal/version.Version=${VERSION}"

mkdir -p bin

build_target() {
    local os="$1" arch="$2" out="$3"
    echo "building ${os}/${arch} -> bin/${out}"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
        go build -trimpath -ldflags "${LDFLAGS}" -o "bin/${out}" ./cmd/tns65
}

build_target linux   amd64 tns65-linux-amd64
build_target windows amd64 tns65-windows-amd64.exe
build_target darwin  arm64 tns65-darwin-arm64

# Current-OS convenience copies.
case "$(uname -s)" in
    Linux)           cur_os=linux ;;
    Darwin)          cur_os=darwin ;;
    *MINGW*|*MSYS*|*CYGWIN*) cur_os=windows ;;
    *)               cur_os=linux ;;
esac
case "$(uname -m)" in
    x86_64|amd64)  cur_arch=amd64 ;;
    arm64|aarch64) cur_arch=arm64 ;;
    *)             cur_arch=amd64 ;;
esac

if [ "$cur_os" = "windows" ]; then
    src="bin/tns65-${cur_os}-${cur_arch}.exe"
else
    src="bin/tns65-${cur_os}-${cur_arch}"
fi

cp "$src" bin/tns65
cp "$src" bin/tns65-bake

# LeMMARu resident (T-01-10): русский лемматизатор, отдельная команда.
echo "building ${cur_os}/${cur_arch} -> bin/tns65-lemmaru"
CGO_ENABLED=0 GOOS="$cur_os" GOARCH="$cur_arch" \
    go build -trimpath -ldflags "${LDFLAGS}" -o bin/tns65-lemmaru ./cmd/tns65-lemmaru
