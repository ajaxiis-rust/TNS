#!/usr/bin/env bash
# Поставка llama-server (T-12-18): закреплённый релиз llama.cpp, sha256,
# установка в engine/bin/ (не в PATH). Бинарник и общие библиотеки кладутся
# плоско в bin/, чтобы RUNPATH $ORIGIN находил libllama*.so рядом с бинарником.
set -euo pipefail

# Корень движка (скрипт лежит в engine/scripts).
cd "$(dirname "$0")/.."
ENGINE_ROOT="$(pwd)"

# Закреплённый релиз llama.cpp. При смене версии обязательна правка таблицы
# sha256 ниже (считается от официальных ассетов релиза на GitHub).
LLAMA_RELEASE="b12063"
BASE_URL="https://github.com/ggml-org/llama.cpp/releases/download/${LLAMA_RELEASE}"

# sha256 официальных ассетов релиза b12063.
SHA_UBUNTU_X64="b57e6b6df9b5af6a6ace2a0648bceda447ad95eca36b1cb2cef4a944b0ab4a76"
SHA_UBUNTU_ARM64="d1d42b3732ce8042d9e54fc22e47842bb5310bba2bd16dc8362cc1b27312a0b3"
SHA_MACOS_X64="17a87c9c64646652ada68cc7fb745114262d46a12499869db39d2960803d43f6"
SHA_MACOS_ARM64="892b2823184bedbba35c7cbc52a368e2d83310631d25ebd950f5a5a48bfa131e"
SHA_WIN_X64="4e3766c32a517cdd66ea28f57c862d6b83582ba5d1354f9db585b34f7ad8cf62"
SHA_WIN_ARM64="f43d15a78ebac6d7ce96cb16a75469c094cfc328684b3889396bac26447749d4"

usage() {
    cat <<EOF
usage: $0 [--check] [--dry-run] [--bin-dir DIR]

Скачивает закреплённый релиз llama.cpp ($LLAMA_RELEASE), сверяет sha256 и
ставит llama-server в DIR (по умолчанию engine/bin/).

  --check        сухая проверка без сети: код 0 — llama-server уже стоит
  --dry-run      только показать план, ничего не скачивать
  --bin-dir DIR  каталог установки (по умолчанию engine/bin/)
  -h, --help     эта справка
EOF
    exit "${1:-0}"
}

# Печатает "<ассет> <sha256>" для текущей платформы; код 2 — не поддержана.
resolve_asset() {
    local os arch
    os="$(uname -s)"
    arch="$(uname -m)"
    case "$os" in
        Linux)
            case "$arch" in
                x86_64|amd64)   echo "llama-${LLAMA_RELEASE}-bin-ubuntu-x64.tar.gz"    "$SHA_UBUNTU_X64" ;;
                arm64|aarch64)  echo "llama-${LLAMA_RELEASE}-bin-ubuntu-arm64.tar.gz"  "$SHA_UBUNTU_ARM64" ;;
                *) return 2 ;;
            esac ;;
        Darwin)
            case "$arch" in
                x86_64|amd64)   echo "llama-${LLAMA_RELEASE}-bin-macos-x64.tar.gz"    "$SHA_MACOS_X64" ;;
                arm64|aarch64)  echo "llama-${LLAMA_RELEASE}-bin-macos-arm64.tar.gz"  "$SHA_MACOS_ARM64" ;;
                *) return 2 ;;
            esac ;;
        *MINGW*|*MSYS*|*CYGWIN*)
            case "$arch" in
                x86_64|amd64)   echo "llama-${LLAMA_RELEASE}-bin-win-cpu-x64.zip"     "$SHA_WIN_X64" ;;
                arm64|aarch64)  echo "llama-${LLAMA_RELEASE}-bin-win-cpu-arm64.zip"   "$SHA_WIN_ARM64" ;;
                *) return 2 ;;
            esac ;;
        *) return 2 ;;
    esac
}

# Сверяет sha256 файла с ожидаемым; несовпадение — понятная ошибка, код 1.
sha256_check() {
    local file="$1" expected="$2" actual
    if command -v sha256sum >/dev/null 2>&1; then
        actual="$(sha256sum "$file" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
        actual="$(shasum -a 256 "$file" | awk '{print $1}')"
    else
        echo "ошибка: не найдено sha256sum/shasum" >&2
        exit 2
    fi
    if [ "$actual" != "$expected" ]; then
        echo "ошибка: sha256 не совпал" >&2
        echo "  ожидалось: $expected" >&2
        echo "  получено:  $actual" >&2
        exit 1
    fi
}

main() {
    local dry=0 check=0 bin_dir="$ENGINE_ROOT/bin"
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --check) check=1 ;;
            --dry-run) dry=1 ;;
            --bin-dir) [ "$#" -ge 2 ] || usage 2; bin_dir="$2"; shift ;;
            -h|--help) usage 0 ;;
            *) usage 2 ;;
        esac
        shift
    done

    local asset sha bin_name
    if ! read -r asset sha < <(resolve_asset); then
        echo "ошибка: платформа не поддержана ($(uname -s)/$(uname -m))" >&2
        exit 2
    fi
    bin_name="llama-server"
    [ "${asset##*.}" = "zip" ] && bin_name="llama-server.exe"

    if [ "$check" = 1 ]; then
        if [ -x "${bin_dir}/${bin_name}" ]; then
            echo "llama-server на месте: ${bin_dir}/${bin_name}"
            "${bin_dir}/${bin_name}" --version || true
            exit 0
        fi
        echo "llama-server не установлен: ${bin_dir}/${bin_name}"
        exit 1
    fi

    if [ "$dry" = 1 ]; then
        echo "dry-run: релиз $LLAMA_RELEASE"
        echo "  платформа: $(uname -s)/$(uname -m)"
        echo "  источник:  ${BASE_URL}/${asset}"
        echo "  sha256:    $sha"
        echo "  установка: ${bin_dir}/${bin_name}"
        exit 0
    fi

    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT

    mkdir -p "$bin_dir"
    echo "скачиваю ${BASE_URL}/${asset}"
    curl -fsSL --retry 3 -o "$tmpdir/archive" "${BASE_URL}/${asset}"

    echo "проверяю sha256"
    sha256_check "$tmpdir/archive" "$sha"

    mkdir -p "$tmpdir/extract"
    if [ "${asset##*.}" = "zip" ]; then
        command -v unzip >/dev/null 2>&1 || { echo "ошибка: нужен unzip" >&2; exit 2; }
        unzip -q "$tmpdir/archive" -d "$tmpdir/extract"
        # zip раскладывается плоско (dll + llama-server.exe + LICENSE).
        cp -a "$tmpdir/extract"/. "$bin_dir"/
        echo "готово: ${bin_dir}/${bin_name}"
        "${bin_dir}/${bin_name}" --version || true
        exit 0
    fi

    tar -xzf "$tmpdir/archive" -C "$tmpdir/extract"
    local src="$tmpdir/extract/llama-${LLAMA_RELEASE}"

    install -m 0755 "$src/llama-server" "$bin_dir/llama-server"
    # Общие библиотеки рядом с бинарником (RUNPATH $ORIGIN), включая CPU-бэкенды.
    cp -a "$src"/lib*.so* "$bin_dir"/
    [ -f "$src/LICENSE" ] && install -m 0644 "$src/LICENSE" "$bin_dir/llama-LICENSE"

    echo "готово: ${bin_dir}/${bin_name}"
    "${bin_dir}/${bin_name}" --version || true
}

main "$@"
