#!/usr/bin/env bash
set -euo pipefail

# --update-web-manifest — пересчитать манифест фронта (web/manifest.json).
# Обычная сборка манифест только сверяет: правка UI без обновления пинов падает
# с подсказкой, как packs/ru.
UPDATE_WEB_MANIFEST=0
case "${1:-}" in
    --update-web-manifest) UPDATE_WEB_MANIFEST=1 ;;
    "") ;;
    *) echo "build.sh: неизвестный флаг: $1" >&2; exit 2 ;;
esac

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
    nature.json             # словари разбора книги (W-22)
    architecture.json
    social.json
    action_dramatic.json
    erotic.json
    violence.json
    battle.json
    appearance.json         # словарь анатомии (IMP-9)
    styles.json             # канон ярлыков стиля (W-27.2)
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
# config.yaml, providers.toml и packs/ целиком. Выборочное копирование пака уже
# однажды ломало варку книг в проде.

# Веб-статика (IMP-12): гибрид «диск прежде embed». Фронт ездит каталогом web/
# рядом с бинарником — правка UI на станции подхватывается рестартом движка, без
# пересборки. Манифест пинит sha256 файлов фронта на момент сборки: его пишет
# этот скрипт, по нему движок в journal сигналит о разошедшемся файле.
WEB_ROOT="web"
REQUIRED_WEB_FILES=(
    index.html
    atlas.html
    history.html
    mcp.html
    settings.html
    wizard.html
    static/app.js
    manifest.json
)

generate_web_manifest() {
    local tmp first=1 f sha
    tmp="$(mktemp)"
    {
        printf '{\n  "id": "web",\n  "files": {\n'
        while IFS= read -r f; do
            sha="$(sha256sum "$WEB_ROOT/$f" | cut -d' ' -f1)"
            [ "$first" -eq 1 ] || printf ',\n'
            first=0
            printf '    "%s": "%s"' "$f" "$sha"
        done < <(cd "$WEB_ROOT" && find . -type f ! -name 'manifest.json' ! -name '*.go' \
                     | sed 's|^\./||' | LC_ALL=C sort)
        printf '\n  }\n}\n'
    } >"$tmp"
    mv "$tmp" "$WEB_ROOT/manifest.json"
}

if [ "$UPDATE_WEB_MANIFEST" -eq 1 ]; then
    generate_web_manifest
    echo "web/manifest.json обновлён"
fi

echo "verifying web"
for f in "${REQUIRED_WEB_FILES[@]}"; do
    if [ ! -f "$WEB_ROOT/$f" ]; then
        echo "FAIL: web/$f отсутствует — без него фронт поставки неполон, движок уйдёт на embed" >&2
        exit 1
    fi
done
while IFS=$'\t' read -r name sha; do
    [ -n "$name" ] || continue
    if [ ! -f "$WEB_ROOT/$name" ]; then
        echo "FAIL: web/$name из манифеста отсутствует" >&2
        exit 1
    fi
    got="$(sha256sum "$WEB_ROOT/$name" | cut -d' ' -f1)"
    if [ "$got" != "$sha" ]; then
        echo "FAIL: web/$name sha256 $got != манифест $sha — правка фронта без обновления манифеста" >&2
        echo "      пересчитай пины: bash scripts/build.sh --update-web-manifest" >&2
        exit 1
    fi
done < <(jq -r '.files | to_entries[] | "\(.key)\t\(.value)"' "$WEB_ROOT/manifest.json")
echo "web OK: фронт и манифест сходятся"

# Копия фронта в поставку: bin/web ложится рядом с bin/tns65, а rsync bin/ в
# папку станции кладёт web/ рядом с бинарником (гибрид IMP-12).
rm -rf bin/web
cp -R "$WEB_ROOT" bin/web
# В поставке только статика: embed.go — исходник движка, на станции не нужен.
find bin/web -name '*.go' -delete
echo "bin/web собран: $(find bin/web -type f | wc -l) файлов"
