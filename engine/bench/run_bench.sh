#!/usr/bin/env bash
# run_bench.sh — замер скорости моделей станции через llama-bench (сторонний
# инструмент из llama.cpp) и подбор оптимальных чисел потоков.
#
# Ищет все *.gguf в --models-dir (по умолчанию — world_db/models станции),
# для каждой модели меряет prompt processing (pp) и text generation (tg)
# при разных -t, пишет сырой JSON в results/<метка>-<режим>/ и сводку
# summary.md с рекомендациями под config.yaml (gen_threads, embed_threads).
#
# Режимы:
#   smoke — одна модель, 2 потока, малые размеры: проверка самого стенда (~секунды);
#   quick — все модели, потоки 1,2,3,4,6,8, pp 256 / tg 32, 2 повтора, CPU;
#   full  — все модели, потоки 1..8, pp 1024 / tg 128, 5 повторов, CPU и Vulkan.
#
# Быстрый запуск:  bash engine/bench/run_bench.sh
# Полный прогон:  bash engine/bench/run_bench.sh --mode full
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODELS_DIR="/home/ajaxii/PRJ/TNS-GOLD/world_db/models"
MODE="quick"
REPS=""
N_PROMPT=""
N_GEN=""
THREADS=""
NGLS=""
OUT_ROOT="${SCRIPT_DIR}/results"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --models-dir) MODELS_DIR="$2"; shift 2 ;;
        --mode)       MODE="$2"; shift 2 ;;
        --out)        OUT_ROOT="$2"; shift 2 ;;
        -h|--help)    sed -n '2,20p' "$0"; exit 0 ;;
        *) echo "неизвестный флаг: $1" >&2; exit 2 ;;
    esac
done

if ! command -v llama-bench >/dev/null 2>&1; then
    echo "FAIL: llama-bench не найден в PATH (ожидается ~/.local/bin/llama-bench)" >&2
    exit 1
fi
if [[ ! -d "$MODELS_DIR" ]]; then
    echo "FAIL: папка моделей не найдена: $MODELS_DIR" >&2
    exit 1
fi

case "$MODE" in
    smoke) THREADS="1,4";     N_PROMPT=64;   N_GEN=16;  REPS=1; NGLS="0" ;;
    quick) THREADS="1,2,3,4,6,8"; N_PROMPT=256;  N_GEN=32;  REPS=2; NGLS="0" ;;
    full)  THREADS="1,2,3,4,5,6,7,8"; N_PROMPT=1024; N_GEN=128; REPS=5; NGLS="0,999" ;;
    *) echo "FAIL: неизвестный режим: $MODE (допустимы smoke|quick|full)" >&2; exit 2 ;;
esac

mapfile -t MODELS < <(find "$MODELS_DIR" -name '*.gguf' | sort)
if [[ ${#MODELS[@]} -eq 0 ]]; then
    echo "FAIL: в $MODELS_DIR нет *.gguf" >&2
    exit 1
fi
if [[ "$MODE" == "smoke" ]]; then
    # самая лёгкая модель — быстрая проверка стенда
    MODELS=("$(ls -S "${MODELS[@]}" | tail -n 1)")
fi

STAMP="$(date +%Y%m%d-%H%M%S)"
OUT_DIR="${OUT_ROOT}/${STAMP}-${MODE}"
mkdir -p "$OUT_DIR"

echo "=== llama-bench: режим $MODE, моделей ${#MODELS[@]}, потоки $THREADS ==="
echo "результаты: $OUT_DIR"

FAILED=()
for MODEL in "${MODELS[@]}"; do
    ROLE="$(basename "$(dirname "$MODEL")")"
    BASE="$(basename "$MODEL" .gguf)"
    # Эмбеддер — только prompt processing (-n 0), у генеративных pp и tg.
    GEN_FLAGS=""
    if [[ "$ROLE" == "embed" ]]; then
        GEN_FLAGS="-embd 1 -n 0"
    else
        GEN_FLAGS="-n ${N_GEN}"
    fi
    for NGL in ${NGLS//,/ }; do
        OUT_FILE="${OUT_DIR}/${ROLE}__${BASE}__ngl${NGL}.json"
        echo "--- $ROLE/$BASE (ngl=$NGL) ---"
        # Одна битая модель не должна обрывать прогон: пишем сбой в список и
        # идём дальше — в сводке он покажется отдельной строкой.
        # shellcheck disable=SC2086
        if ! llama-bench -m "$MODEL" $GEN_FLAGS -p "$N_PROMPT" -t "$THREADS" \
            -r "$REPS" -ngl "$NGL" -o json > "$OUT_FILE" 2>>"${OUT_DIR}/llama-bench.log"; then
            echo "    !! llama-bench не смог замерить $MODEL (см. llama-bench.log)"
            FAILED+=("${ROLE}__${BASE}__ngl${NGL}")
            continue
        fi
        echo "    -> $(basename "$OUT_FILE")"
    done
done

FAILED_JSON="[]"
if [[ ${#FAILED[@]} -gt 0 ]]; then
    FAILED_JSON="$(printf '%s\n' "${FAILED[@]}" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read().split()))')"
fi

cat > "${OUT_DIR}/manifest.json" <<EOF
{
  "mode": "${MODE}",
  "models_dir": "${MODELS_DIR}",
  "threads": "${THREADS}",
  "n_prompt": ${N_PROMPT},
  "n_gen": ${N_GEN},
  "reps": ${REPS},
  "ngls": "${NGLS}",
  "cpu_info": "$(grep -m1 'model name' /proc/cpuinfo | cut -d: -f2 | sed 's/^ //')",
  "nproc": "$(nproc)",
  "mem_total_kb": "$(awk '/MemTotal/{print $2}' /proc/meminfo)",
  "host": "$(hostname)",
  "date": "$(date -Iseconds)",
  "failed": ${FAILED_JSON}
}
EOF

python3 "${SCRIPT_DIR}/summarize.py" "$OUT_DIR" | tee "${OUT_DIR}/summary.md"
echo
echo "Готово: ${OUT_DIR}/summary.md"
