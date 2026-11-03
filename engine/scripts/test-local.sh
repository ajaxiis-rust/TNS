#!/usr/bin/env bash
# Локальный тест станка (T-12-28, T-12-29): запускает тесты с тегом local против
# реальной книги и локальных моделей :5001/:5002. Ветка загрузки (без флагов)
# проверяет BGE-M3 Q4 и генеративную 3B, явно скачивает недостающее со сверкой
# sha256, поднимает llama-server на :5001/:5002 и гонит тест. `--check` только
# перечисляет отсутствующее и ничего не качает. Без поднятых моделей тесты сами
# пропускаются (SKIP) с сообщением «локальная модель не поднята, см.
# scripts/test-local.sh».
set -euo pipefail

# Корень движка (скрипт лежит в engine/scripts).
cd "$(dirname "$0")/.."
ENGINE_ROOT="$(pwd)"

# Закреплённые модели локального теста (T-12-29). Каждая строка: роль|файл|sha256.
# sha256 — oid LFS файла на Hugging Face; смена модели — правка этой таблицы и
# source_for ниже.
BGE_M3_Q4_FILE="models/bge-m3-Q4_K_M.gguf"
BGE_M3_Q4_SRC="https://huggingface.co/gpustack/bge-m3-GGUF/resolve/main/bge-m3-Q4_K_M.gguf"
BGE_M3_Q4_SHA256="6d39681b26c61279ac1f82db35a04a05009e94c415b51c858ff571489a82fc06"

GEN_3B_FILE="models/Llama-3.2-3B-Instruct-Q4_K_M.gguf"
GEN_3B_SRC="https://huggingface.co/bartowski/Llama-3.2-3B-Instruct-GGUF/resolve/main/Llama-3.2-3B-Instruct-Q4_K_M.gguf"
GEN_3B_SHA256="6c1a2b41161032677be168d354123594c0e6e67d2b9227c84f296ad037c728ff"

MODELS="
embed|${BGE_M3_Q4_FILE}|${BGE_M3_Q4_SHA256}
gen|${GEN_3B_FILE}|${GEN_3B_SHA256}
"

# Источник модели по роли (embed/gen).
source_for() {
    case "$1" in
        embed) printf '%s' "$BGE_M3_Q4_SRC" ;;
        gen)   printf '%s' "$GEN_3B_SRC" ;;
        *)     return 2 ;;
    esac
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

# Состояние модели: ok, missing или mismatch <actual>.
model_state() {
    local file="$1" expected="$2" actual
    if [ ! -f "$ENGINE_ROOT/$file" ]; then
        printf 'missing'
        return 0
    fi
    actual="$(sha256_of "$ENGINE_ROOT/$file")"
    if [ "$actual" = "$expected" ]; then
        printf 'ok'
    else
        printf 'mismatch %s' "$actual"
    fi
}

usage() {
    cat <<EOF
usage: $0 [--check]

Запускает локальный тест станка: проверяет модели (BGE-M3 Q4 и генеративную 3B),
скачивает недостающее со сверкой sha256, поднимает llama-server на :5001/:5002 и
гонит go test -tags=local.

  --check       только перечислить отсутствующее/повреждённое, ничего не качать
  -h, --help    эта справка
EOF
    exit "${1:-0}"
}

# Скачивает модель в engine/models/ с проверкой sha256 (явная загрузка, T-12-29).
download_model() {
    local role="$1" file="$2" sha="$3" src tmp actual
    src="$(source_for "$role")" || return 2
    tmp="${ENGINE_ROOT}/${file}.tmp.$$"
    mkdir -p "$(dirname "$ENGINE_ROOT/$file")"
    echo "скачиваю ${src}"
    curl -fSL --retry 3 -o "$tmp" "$src"
    echo "проверяю sha256"
    actual="$(sha256_of "$tmp")"
    if [ "$actual" != "$sha" ]; then
        echo "ошибка: sha256 не совпал" >&2
        echo "  ожидалось: $sha" >&2
        echo "  получено:  $actual" >&2
        rm -f "$tmp"
        return 1
    fi
    mv "$tmp" "$ENGINE_ROOT/$file"
    echo "готово: ${file}"
}

# Ждёт /health на порту; сервер умер или таймаут — код 1.
wait_health() {
    local port="$1" pid="$2" tries="${3:-300}" i=0
    while [ "$i" -lt "$tries" ]; do
        if ! kill -0 "$pid" 2>/dev/null; then
            return 1
        fi
        if curl -fsS "http://127.0.0.1:${port}/health" >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
        i=$((i+1))
    done
    return 1
}

GEN_PID=""
EMBED_PID=""

cleanup() {
    [ -n "$EMBED_PID" ] && kill "$EMBED_PID" 2>/dev/null || true
    [ -n "$GEN_PID" ] && kill "$GEN_PID" 2>/dev/null || true
    wait 2>/dev/null || true
}

main() {
    local mode="run"
    case "${1:-}" in
        "") mode="run" ;;
        --check) mode="check" ;;
        -h|--help) usage 0 ;;
        *) usage 2 ;;
    esac

    # Станок обязан быть поставлен: llama-server кладётся в bin/ (install-llama.sh).
    if [ ! -x "$ENGINE_ROOT/bin/llama-server" ]; then
        if [ "$mode" = "check" ]; then
            echo "нет    llama-server  bin/llama-server (engine/scripts/install-llama.sh)"
        else
            echo "test-local: bin/llama-server не найден — сначала engine/scripts/install-llama.sh" >&2
            exit 2
        fi
    fi

    local missing=0 role file sha state
    local -a todo=()
    while IFS='|' read -r role file sha; do
        [ -z "$role" ] && continue
        state="$(model_state "$file" "$sha")"
        if [ "$state" = "ok" ]; then
            echo "ok     ${role}  ${file}"
        else
            echo "нет    ${role}  ${file}  (${state})"
            missing=1
            todo+=("${role}|${file}|${sha}")
        fi
    done <<< "$MODELS"

    # --check только перечисляет отсутствующее/повреждённое и ничего не качает.
    if [ "$mode" = "check" ]; then
        if [ "$missing" = 0 ]; then
            echo "все модели на месте"
        fi
        exit "$missing"
    fi

    # Ветка загрузки: явно докачать недостающее/повреждённое.
    for m in "${todo[@]}"; do
        IFS='|' read -r role file sha <<< "$m"
        download_model "$role" "$file" "$sha"
    done

    # Подъём серверов: :5001 генерация, :5002 эмбеддинги BGE-M3.
    echo "поднимаю llama-server :5001 (генерация) и :5002 (эмбеддинги)"
    "$ENGINE_ROOT/bin/llama-server" \
        --model "$ENGINE_ROOT/$GEN_3B_FILE" --host 127.0.0.1 --port 5001 \
        >"$ENGINE_ROOT/state/llama-gen.log" 2>&1 &
    GEN_PID=$!
    "$ENGINE_ROOT/bin/llama-server" \
        --model "$ENGINE_ROOT/$BGE_M3_Q4_FILE" --host 127.0.0.1 --port 5002 \
        --embeddings --pooling cls -c 8192 \
        >"$ENGINE_ROOT/state/llama-embed.log" 2>&1 &
    EMBED_PID=$!

    trap cleanup EXIT

    echo "жду готовности :5001"
    wait_health 5001 "$GEN_PID" 600 || {
        echo "ошибка: :5001 не поднялся (см. state/llama-gen.log)" >&2
        exit 1
    }
    echo "жду готовности :5002"
    wait_health 5002 "$EMBED_PID" 600 || {
        echo "ошибка: :5002 не поднялся (см. state/llama-embed.log)" >&2
        exit 1
    }

    echo "test-local: книги из ${TNS65_FB2_DIR:-/media/ajaxiis/ext3/fb2}"

    # Тесты с тегом local пропускаются (SKIP), если :5001/:5002 не отвечают.
    go test -tags=local ./internal/cast ./internal/workshop -count=1
}

main "$@"
