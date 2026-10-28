#!/usr/bin/env bash
set -euo pipefail

# Исходящий туннель cloudflared (F-04.3): станок сам подключается к краю и
# пробрасывает loopback-адрес веб-интерфейса; публичных слушающих портов нет.
# Запуск: scripts/tunnel.sh (из engine/).

# Resolve the engine root (this script lives in engine/scripts).
cd "$(dirname "$0")/.."

CONFIG="${1:-config.yaml}"
[ -f "$CONFIG" ] || { echo "tunnel.sh: config not found: $CONFIG" >&2; exit 1; }

# Край Cloudflare и пробрасываемый loopback-адрес читаются из секции tunnel.
EDGE="$(awk '
  $0 ~ /^tunnel:/ { in_tunnel = 1; next }
  in_tunnel && $0 ~ /^[a-z_]+:/ { in_tunnel = 0 }
  in_tunnel && $1 == "edge:" { sub(/^[^:]*:[[:space:]]*/, ""); sub(/[[:space:]]*#.*$/, ""); print; exit }
' "$CONFIG")"
LOCAL="$(awk '
  $0 ~ /^tunnel:/ { in_tunnel = 1; next }
  in_tunnel && $0 ~ /^[a-z_]+:/ { in_tunnel = 0 }
  in_tunnel && $1 == "local:" { sub(/^[^:]*:[[:space:]]*/, ""); sub(/[[:space:]]*#.*$/, ""); print; exit }
' "$CONFIG")"

EDGE="${EDGE:-region1.v2.argotunnel.com}"
LOCAL="${LOCAL:-http://127.0.0.1:8720}"

# Писатель T-08-03: событие подключения — в журнал станка (state/journal/engine.log).
log_connect() {
  local ts
  ts="$(date +%Y-%m-%dT%H:%M:%S%z)"
  mkdir -p state/journal
  printf '{"ts":"%s","level":"info","source":"tunnel","event":"tunnel.connect","fields":[{"key":"edge","value":"%s"},{"key":"local","value":"%s"}]}\n' \
    "$ts" "$EDGE" "$LOCAL" >> state/journal/engine.log
}

log_connect

# Только исходящее: --edge --url, без внешнего bind (F-04.3). Внешний слушающий
# адрес не задаётся — параметра bind у cloudflared не появляется.
exec cloudflared tunnel --edge "$EDGE" --url "$LOCAL"
