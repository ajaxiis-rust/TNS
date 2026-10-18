#!/usr/bin/env bash
# One MiMo spec-writer run for section NN: run -> gate -> (one retry) -> gate.
set -u
NN="$1"
MIMO=/home/ajaxiis/.mimocode/bin/mimo
BASE=/media/ajaxiis/ext3
W=$BASE/TNS65/fine/04-spec/_pipeline
BRIEF=$W/brief-$NN.md
RAW=$W/raw
mkdir -p "$RAW"
SRC="-f $BASE/TNS65/fine/04-spec/SKELETON.md -f $BASE/TNS65/fine/00-recon/mimo-mechanics-registry.md -f $BASE/TNS65/TNS65-PARADIGM.md -f $BASE/TNS65/fine/00-recon/owner-rulings.md -f $BASE/TNS65/fine/03-design/01-mcp.md -f $BASE/TNS65/fine/03-design/02-monitoring.md -f $BASE/TNS65/fine/03-design/03-my-book.md"

log() { echo "[$(date +%H:%M:%S)] $*" >> "$W/progress.log"; }

attempt() {  # $1=outfile $2=extra -f
  timeout 890 "$MIMO" run --dir "$BASE" -m xiaomi/mimo-v2.6-pro \
    --title "spec-writer-$NN" "$(cat "$BRIEF")" $SRC $2 > "$1" 2> "${1%.out}.err"
  echo $?
}

log "$NN START"
RC=$(attempt "$RAW/$NN.out" "")
GATE=$(python3 "$W/gate.py" "$NN" "$RAW/$NN.out" 2>&1); GR=$?
if [ $GR -eq 0 ]; then log "$NN DONE gate=PASS attempt=1"; exit 0; fi

# mechanically proven cause -> one fix, one retry
log "$NN RETRY reason: $(echo "$GATE" | head -2 | tr '\n' ' ')"
python3 - "$NN" "$W" "$GATE" <<'PYEOF'
import sys
nn, w, gate = sys.argv[1], sys.argv[2], sys.argv[3]
brief = open(f"{w}/brief-{nn}.md", encoding="utf-8").read()
retry = ("Твой предыдущий ответ не прошёл автоматическую проверку. Список ошибок:\n\n"
         + gate + "\n\nИсправь ВСЕ ошибки и выдай файл ЦЕЛИКОМ заново, "
         "по тем же правилам и в том же формате (атомы + ===COVERAGE=== + построчное покрытие).\n\n"
         + brief)
open(f"{w}/retry-brief-{nn}.md", "w", encoding="utf-8").write(retry)
PYEOF
BRIEF=$W/retry-brief-$NN.md
RC=$(attempt "$RAW/$NN.retry.out" "-f $RAW/$NN.out")
GATE=$(python3 "$W/gate.py" "$NN" "$RAW/$NN.retry.out" 2>&1); GR=$?
if [ $GR -eq 0 ]; then log "$NN DONE gate=PASS attempt=2"; exit 0; fi
log "$NN FAIL attempts=2 last: $(echo "$GATE" | head -3 | tr '\n' ' ')"
exit 1
