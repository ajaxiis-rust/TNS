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
# Бинарник кладётся внутрь языкового пакета рядом со словарём — пакет
# самодостаточен, и движок находит лемматизатор без go.mod (см. fine/04-spec D-13).
mkdir -p packs/ru/lemmatizer
echo "building ${cur_os}/${cur_arch} -> packs/ru/lemmatizer/tns65-lemmaru"
CGO_ENABLED=0 GOOS="$cur_os" GOARCH="$cur_arch" \
    go build -trimpath -ldflags "${LDFLAGS}" -o packs/ru/lemmatizer/tns65-lemmaru ./cmd/tns65-lemmaru

# Поставка языкового пакета в прод: пакет ездит ЦЕЛИКОМ, не выборочно.
# Урок TNS-GAME (2026-11-04): прод со старой заглушкой stoplist.txt (7 служебных
# слов от T-01-08) и без stoplist_16.txt резал любую варку книги цензором —
# разбор в TNS-GAME/probe-out/REPORT.md. manifest.json пинит sha не всех файлов
# цензора, поэтому обязательный список требуем явно, а пинимое сверяем по sha256.
REQUIRED_PACK_FILES=(
    stoplist.txt            # базовый словарь грубых слов — цензор при любом рейтинге
    stoplist_16.txt         # 16+: второй словарь (оскорбления)
    refusals.txt            # фразы-маркеры отказов LLM
    descriptions.json
    detectors_mood.json
    detectors_time.json
    lexicon_intents.jsonl
    manifest.json
)

echo "verifying packs/ru"
for f in "${REQUIRED_PACK_FILES[@]}"; do
    if [ ! -f "packs/ru/$f" ]; then
        echo "FAIL: packs/ru/$f отсутствует — без него цензор не варит книги в проде" >&2
        exit 1
    fi
done
while IFS=$'\t' read -r name sha; do
    [ -n "$name" ] || continue
    got="$(sha256sum "packs/ru/$name" | cut -d' ' -f1)"
    if [ "$got" != "$sha" ]; then
        echo "FAIL: packs/ru/$name sha256 $got != манифест $sha" >&2
        exit 1
    fi
done < <(jq -r '.files | to_entries[] | "\(.key)\t\(.value)"' packs/ru/manifest.json)
echo "packs/ru OK: словари цензора и маркеры отказов в поставке"

# Сборка папки поставки (прод): копировать из корня движка bin/, base/, seed/,
# config.yaml и packs/ целиком. Выборочное копирование пака уже однажды
# ломало варку книг в проде.
