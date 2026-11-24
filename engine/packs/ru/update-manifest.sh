#!/usr/bin/env bash
# Пересчёт sha256 пинов manifest.json этого языкового пакета.
# Кладётся рядом со словарями: владелец станции правит файл словаря руками,
# затем запускает скрипт, чтобы пересчитать пин. Без совпадения пина движок
# откажется грузить пак с ошибкой «langpack: broken files: [...]».
#
# Требует только sha256sum или shasum (Linux/macOS). Формат manifest.json
# ожидается машинный (2-пробельный отступ), как его пишет Go MarshalIndent.
set -euo pipefail

cd "$(dirname "$0")"

MANIFEST="manifest.json"

usage() {
    cat <<'EOF'
usage: ./update-manifest.sh [файл ...] [--check]

Пересчитывает sha256 пинов manifest.json этого языкового пакета.
Файл словаря правится руками, затем скрипт пересчитывает пин; без
совпадения пина движок откажется грузить пак (langpack: broken files).

  без аргументов      пересчитать все файлы, закреплённые в manifest.json
  файл ...            пересчитать только указанные файлы
  --check             только сверить, ничего не писать
  -h, --help          эта справка
EOF
    exit "${1:-0}"
}

# sha256 файла; нет sha256sum/shasum — понятная ошибка, код 2.
sha256_of() {
    local file="$1" actual
    if command -v sha256sum >/dev/null 2>&1; then
        actual="$(sha256sum "$file" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
        actual="$(shasum -a 256 "$file" | awk '{print $1}')"
    else
        echo "ошибка: не найдено sha256sum/shasum" >&2
        exit 2
    fi
    printf '%s' "$actual"
}

# Имена файлов, закреплённые в manifest.json: значение — 64-символьный hex sha.
pinned_names() {
    sed -n 's/^    "\([^"]*\)": "[0-9a-f]\{64\}",*$/\1/p' "$MANIFEST"
}

# Текущий пин файла в manifest.json (пусто, если файла в списке нет).
pin_of() {
    sed -n "s/^    \"$1\": \"\([0-9a-f]*\)\",*$/\1/p" "$MANIFEST"
}

# Записать новый пин, сохранив наличие/отсутствие хвостовой запятой.
write_pin() {
    local name="$1" sha="$2"
    if [ -z "$(pin_of "$name")" ]; then
        echo "ошибка: $name нет в $MANIFEST (секция files)" >&2
        return 1
    fi
    sed "s|^\(    \"$name\": \"\)[0-9a-f]\{1,\}\(\",*\)$|\1$sha\2|" \
        "$MANIFEST" > "$MANIFEST.tmp"
    mv "$MANIFEST.tmp" "$MANIFEST"
}

mode="write"
names=()
for arg in "$@"; do
    case "$arg" in
        -h|--help) usage 0 ;;
        --check) mode="check" ;;
        *) names+=("$arg") ;;
    esac
done

if [ "${#names[@]}" -eq 0 ]; then
    while IFS= read -r n; do
        [ -n "$n" ] && names+=("$n")
    done < <(pinned_names)
    [ "${#names[@]}" -gt 0 ] || {
        echo "ошибка: в $MANIFEST нет закреплённых файлов" >&2
        exit 1
    }
fi

status=0
for name in "${names[@]}"; do
    if [ ! -f "$name" ]; then
        echo "нет    $name (файла нет)" >&2
        status=1
        continue
    fi
    sha="$(sha256_of "$name")"
    old="$(pin_of "$name")"
    if [ -z "$old" ]; then
        echo "нет    $name (нет пина в manifest.json)" >&2
        status=1
        continue
    fi
    if [ "$mode" = "check" ]; then
        if [ "$old" = "$sha" ]; then
            echo "ok     $name"
        else
            echo "мимо   $name (sha не совпадает с манифестом)"
            status=1
        fi
    else
        write_pin "$name" "$sha"
        echo "пин    $name -> $sha"
    fi
done

exit "$status"
