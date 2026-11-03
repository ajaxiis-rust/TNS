#!/usr/bin/env bash
# Локальный тест станка (T-12-28): запускает тесты с тегом local против реальной
# книги и локальных моделей :5001/:5002. Проверка и загрузка моделей — T-12-29;
# без поднятых моделей тесты сами пропускаются (SKIP) с сообщением
# «локальная модель не поднята, см. scripts/test-local.sh».
set -euo pipefail

# Корень движка (скрипт лежит в engine/scripts).
cd "$(dirname "$0")/.."
ENGINE_ROOT="$(pwd)"

# Станок обязан быть поставлен: llama-server кладётся в bin/ (install-llama.sh).
if [ ! -x "$ENGINE_ROOT/bin/llama-server" ]; then
    echo "test-local: bin/llama-server не найден — сначала engine/scripts/install-llama.sh" >&2
    exit 2
fi

echo "test-local: книги из ${TNS65_FB2_DIR:-/media/ajaxiis/ext3/fb2}"

# Тесты с тегом local пропускаются (SKIP), если :5001/:5002 не отвечают.
exec go test -tags=local ./internal/cast ./internal/workshop -count=1
