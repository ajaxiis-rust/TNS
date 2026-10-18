#!/usr/bin/env bash
# Watchdog: report NEW progress lines since last check; silent when nothing changed.
W=/media/ajaxiis/ext3/TNS65/fine/04-spec/_pipeline
LOG=$W/progress.log
STATE=$W/.watchdog-state
[ -f "$LOG" ] || exit 0
LAST=$(cat "$STATE" 2>/dev/null || echo 0)
CUR=$(wc -l < "$LOG")
if [ "$CUR" -le "$LAST" ]; then exit 0; fi
sed -n "$((LAST + 1)),${CUR}p" "$LOG"
DONE=$(grep -c "DONE gate=PASS" "$LOG" || true)
FAIL=$(grep -c "FAIL" "$LOG" || true)
FIN=$(grep -c "BATCH-FINISHED" "$LOG" || true)
echo "--- pass=$DONE fail=$FAIL finished=$FIN (of 10 sections)"
echo "$CUR" > "$STATE"
# non-zero exit on any FAIL so the failure is never silent
[ "$FAIL" -eq 0 ] || exit 1
